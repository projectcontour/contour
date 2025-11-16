// Copyright Project Contour Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package dag

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/ptr"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/annotation"
	"github.com/projectcontour/contour/internal/k8s"
	"github.com/projectcontour/contour/internal/status"
	"github.com/projectcontour/contour/internal/timeout"
)

// defaultMaxRequestBytes specifies default value maxRequestBytes for AuthorizationServer
const defaultMaxRequestBytes uint32 = 1024

// defaultExtensionRef populates the unset fields in ref with default values.
func defaultExtensionRef(ref contour_v1.ExtensionServiceReference) contour_v1.ExtensionServiceReference {
	if ref.APIVersion == "" {
		ref.APIVersion = contour_v1alpha1.GroupVersion.String()
	}

	return ref
}

// HTTPProxyProcessor translates HTTPProxies into DAG
// objects and adds them to the DAG.
type HTTPProxyProcessor struct {
	dag      *DAG
	source   *KubernetesCache
	orphaned map[types.NamespacedName]bool

	// DisablePermitInsecure disables the use of the
	// permitInsecure field in HTTPProxy.
	DisablePermitInsecure bool

	// FallbackCertificate is the optional identifier of the
	// TLS secret to use by default when SNI is not set on a
	// request.
	FallbackCertificate *types.NamespacedName

	// EnableExternalNameService allows processing of ExternalNameServices
	// This is normally disabled for security reasons.
	// See https://github.com/projectcontour/contour/security/advisories/GHSA-5ph6-qq5x-7jwc for details.
	EnableExternalNameService bool

	// DNSLookupFamily defines how external names are looked up
	// When configured as V4, the DNS resolver will only perform a lookup
	// for addresses in the IPv4 family. If V6 is configured, the DNS resolver
	// will only perform a lookup for addresses in the IPv6 family.
	// If AUTO is configured, the DNS resolver will first perform a lookup
	// for addresses in the IPv6 family and fallback to a lookup for addresses
	// in the IPv4 family. If ALL is specified, the DNS resolver will perform a lookup for
	// both IPv4 and IPv6 families, and return all resolved addresses.
	// When this is used, Happy Eyeballs will be enabled for upstream connections.
	// Refer to Happy Eyeballs Support for more information.
	// Note: This only applies to externalName clusters.
	DNSLookupFamily contour_v1alpha1.ClusterDNSFamilyType

	// ClientCertificate is the optional identifier of the TLS secret containing client certificate and
	// private key to be used when establishing TLS connection to upstream cluster.
	ClientCertificate *types.NamespacedName

	// Request headers that will be set on all routes (optional).
	RequestHeadersPolicy *HeadersPolicy

	// Response headers that will be set on all routes (optional).
	ResponseHeadersPolicy *HeadersPolicy

	// GlobalExternalAuthorization defines how requests will be authorized.
	GlobalExternalAuthorization *contour_v1.AuthorizationServer

	// ConnectTimeout defines how long the proxy should wait when establishing connection to upstream service.
	ConnectTimeout time.Duration

	// MaxRequestsPerConnection defines the maximum number of requests per connection to the upstream before it is closed.
	MaxRequestsPerConnection *uint32

	// PerConnectionBufferLimitBytes defines the soft limit on size of the listener’s new connection read and write buffers.
	PerConnectionBufferLimitBytes *uint32

	// GlobalRateLimitService defines Envoy's Global RateLimit Service configuration.
	GlobalRateLimitService *contour_v1alpha1.RateLimitServiceConfig

	// SetSourceMetadataOnRoutes defines whether to set the Kind,
	// Namespace and Name fields on generated DAG routes. This is
	// configurable and off by default in order to support the feature
	// without requiring all existing test cases to change.
	SetSourceMetadataOnRoutes bool

	// Whether to set StatPrefix on envoy routes or not
	EnableStatPrefix bool

	// GlobalCircuitBreakerDefaults defines global circuit breaker defaults.
	GlobalCircuitBreakerDefaults *contour_v1alpha1.CircuitBreakers

	// UpstreamTLS defines the TLS settings like min/max version
	// and cipher suites for upstream connections.
	UpstreamTLS *UpstreamTLS
}

// Run translates HTTPProxies into DAG objects and
// adds them to the DAG.
func (p *HTTPProxyProcessor) Run(dag *DAG, source *KubernetesCache) {
	p.dag = dag
	p.source = source
	p.orphaned = make(map[types.NamespacedName]bool, len(p.orphaned))

	// reset the processor when we're done
	defer func() {
		p.dag = nil
		p.source = nil
		p.orphaned = nil
	}()

	for _, proxy := range p.validHTTPProxies() {
		p.computeHTTPProxy(proxy)
	}

	for meta := range p.orphaned {
		proxy, ok := p.source.httpproxies[meta]
		if ok {
			pa, commit := p.dag.StatusCache.ProxyAccessor(proxy)
			pa.ConditionFor(status.ValidCondition).AddError(contour_v1.ConditionTypeOrphanedError,
				"Orphaned",
				"this HTTPProxy is not part of a delegation chain from a root HTTPProxy")
			commit()
		}
	}
}

func (p *HTTPProxyProcessor) computeHTTPProxy(proxy *contour_v1.HTTPProxy) {
	pa, commit := p.dag.StatusCache.ProxyAccessor(proxy)
	validCond := pa.ConditionFor(status.ValidCondition)

	defer commit()

	var defaultJWTProvider string

	if proxy.Spec.VirtualHost == nil {
		// mark HTTPProxy as orphaned.
		p.orphaned[k8s.NamespacedNameOf(proxy)] = true
		return
	}

	host := proxy.Spec.VirtualHost.Fqdn
	if isBlank(host) {
		validCond.AddError(contour_v1.ConditionTypeVirtualHostError, "FQDNNotSpecified",
			"Spec.VirtualHost.Fqdn must be specified")
		return
	}

	pa.Vhost = host

	// Ensure root httpproxy lives in allowed namespace.
	// This check must be after we can determine the vhost in order to be able to calculate metrics correctly.
	if !p.rootAllowed(proxy.Namespace) {
		validCond.AddError(contour_v1.ConditionTypeRootNamespaceError, "RootProxyNotAllowedInNamespace",
			"root HTTPProxy cannot be defined in this namespace")
		return
	}

	if len(proxy.Spec.Routes) == 0 && len(proxy.Spec.Includes) == 0 && proxy.Spec.TCPProxy == nil {
		validCond.AddError(contour_v1.ConditionTypeSpecError, "NothingDefined",
			"HTTPProxy.Spec must have at least one Route, Include, or a TCPProxy")
		return
	}

	if len(proxy.Spec.VirtualHost.JWTProviders) > 0 {
		if proxy.Spec.VirtualHost.TLS == nil || len(proxy.Spec.VirtualHost.TLS.SecretName) == 0 {
			validCond.AddError(contour_v1.ConditionTypeJWTVerificationError, "JWTVerificationNotPermitted",
				"Spec.VirtualHost.JWTProviders can only be defined for root HTTPProxies that terminate TLS")
			return
		}
	}

	if proxy.Spec.VirtualHost.TLS == nil && proxy.Spec.VirtualHost.Authorization != nil && len(proxy.Spec.VirtualHost.Authorization.ExtensionServiceRef.Name) > 0 {
		validCond.AddError(contour_v1.ConditionTypeAuthError, "AuthNotPermitted",
			"Spec.VirtualHost.Authorization.ExtensionServiceRef can only be defined for root HTTPProxies that terminate TLS")
		return
	}

	if len(proxy.Spec.VirtualHost.IPAllowFilterPolicy) > 0 && len(proxy.Spec.VirtualHost.IPDenyFilterPolicy) > 0 {
		validCond.AddError(contour_v1.ConditionTypeIPFilterError, "IncompatibleIPAddressFilters",
			"Spec.VirtualHost.IPAllowFilterPolicy and Spec.VirtualHost.IPDepnyFilterPolicy cannot both be defined.")
		return
	}

	var tlsEnabled bool
	if tls := proxy.Spec.VirtualHost.TLS; tls != nil {
		if tls.Passthrough && tls.EnableFallbackCertificate {
			validCond.AddError(contour_v1.ConditionTypeTLSError, "TLSIncompatibleFeatures",
				"Spec.VirtualHost.TLS: both Passthrough and enableFallbackCertificate were specified")
		}
		if !isBlank(tls.SecretName) && tls.Passthrough {
			validCond.AddError(contour_v1.ConditionTypeTLSError, "TLSConfigNotValid",
				"Spec.VirtualHost.TLS: both Passthrough and SecretName were specified")
			return
		}

		if isBlank(tls.SecretName) && !tls.Passthrough {
			validCond.AddError(contour_v1.ConditionTypeTLSError, "TLSConfigNotValid",
				"Spec.VirtualHost.TLS: neither Passthrough nor SecretName were specified")
			return
		}

		if tls.Passthrough && tls.ClientValidation != nil {
			validCond.AddError(contour_v1.ConditionTypeTLSError, "TLSIncompatibleFeatures",
				"Spec.VirtualHost.TLS passthrough cannot be combined with tls.clientValidation")
			return
		}

		tlsEnabled = true

		// Attach secrets to TLS enabled vhosts.
		if !tls.Passthrough {
			secretName := k8s.NamespacedNameFrom(tls.SecretName, k8s.DefaultNamespace(proxy.Namespace))
			sec, err := p.source.LookupTLSSecret(secretName, proxy.Namespace)
			if err != nil {
				if _, ok := err.(DelegationNotPermittedError); ok {
					validCond.AddErrorf(contour_v1.ConditionTypeTLSError, "DelegationNotPermitted",
						"Spec.VirtualHost.TLS Secret %q certificate delegation not permitted", tls.SecretName)
				} else {
					validCond.AddErrorf(contour_v1.ConditionTypeTLSError, "SecretNotValid",
						"Spec.VirtualHost.TLS Secret %q is invalid: %s", tls.SecretName, err)
				}
				return
			}

			listener, err := p.dag.GetSingleListener("https")
			if err != nil {
				validCond.AddError(contour_v1.ConditionTypeListenerError, "ErrorIdentifyingListener", err.Error())
				return
			}

			// default to a minimum TLS version of 1.2 if it's not specified
			minTLSVer := annotation.TLSVersion(tls.MinimumProtocolVersion, "1.2")

			// default to a maximum TLS version of 1.3 if it's not specified
			maxTLSVer := annotation.TLSVersion(tls.MaximumProtocolVersion, "1.3")
			if maxTLSVer < minTLSVer {
				validCond.AddError(contour_v1.ConditionTypeTLSError, "TLSConfigNotValid",
					"Spec.Virtualhost.TLS the minimum protocol version is greater than the maximum protocol version")
				return
			}

			svhost := p.dag.EnsureSecureVirtualHost(listener.Name, host)
			svhost.Secret = sec
			svhost.MinTLSVersion = minTLSVer
			svhost.MaxTLSVersion = maxTLSVer
			svhost.HTTPVersions = p.getSortedHTTPVersions(proxy)

			// Check if FallbackCertificate && ClientValidation are both enabled in the same vhost
			if tls.EnableFallbackCertificate && tls.ClientValidation != nil {
				validCond.AddError(contour_v1.ConditionTypeTLSError, "TLSIncompatibleFeatures",
					"Spec.Virtualhost.TLS fallback & client validation are incompatible")
				return
			}

			// Fallback certificates and authorization are
			// incompatible because fallback installs the routes on
			// a separate HTTPConnectionManager. We can't have the
			// same routes installed on multiple managers with
			// inconsistent authorization settings.
			if tls.EnableFallbackCertificate && proxy.Spec.VirtualHost.AuthorizationConfigured() {
				validCond.AddError(contour_v1.ConditionTypeTLSError, "TLSIncompatibleFeatures",
					"Spec.Virtualhost.TLS fallback & client authorization are incompatible")
				return
			}

			// If FallbackCertificate is enabled, but no cert passed, set error
			if tls.EnableFallbackCertificate {
				if p.FallbackCertificate == nil {
					validCond.AddError(contour_v1.ConditionTypeTLSError, "FallbackNotPresent",
						"Spec.Virtualhost.TLS enabled fallback but the fallback Certificate Secret is not configured in Contour configuration file")
					return
				}

				sec, err = p.source.LookupTLSSecret(*p.FallbackCertificate, proxy.Namespace)
				if err != nil {
					if _, ok := err.(DelegationNotPermittedError); ok {
						validCond.AddErrorf(contour_v1.ConditionTypeTLSError, "FallbackNotDelegated",
							"Spec.VirtualHost.TLS Secret %q is not configured for certificate delegation", p.FallbackCertificate)
					} else {
						validCond.AddErrorf(contour_v1.ConditionTypeTLSError, "FallbackNotValid",
							"Spec.Virtualhost.TLS Secret %q fallback certificate is invalid: %s", p.FallbackCertificate, err)
					}
					return
				}

				svhost.FallbackCertificate = sec
			}

			// Fill in DownstreamValidation when external client validation is enabled.
			if tls.ClientValidation != nil {
				dv := &PeerValidationContext{
					SkipClientCertValidation:  tls.ClientValidation.SkipClientCertValidation,
					OptionalClientCertificate: tls.ClientValidation.OptionalClientCertificate,
				}
				if tls.ClientValidation.ForwardClientCertificate != nil {
					dv.ForwardClientCertificate = &ClientCertificateDetails{
						Subject: tls.ClientValidation.ForwardClientCertificate.Subject,
						Cert:    tls.ClientValidation.ForwardClientCertificate.Cert,
						Chain:   tls.ClientValidation.ForwardClientCertificate.Chain,
						DNS:     tls.ClientValidation.ForwardClientCertificate.DNS,
						URI:     tls.ClientValidation.ForwardClientCertificate.URI,
					}
				}
				if tls.ClientValidation.CACertificate != "" {
					secretName := k8s.NamespacedNameFrom(tls.ClientValidation.CACertificate, k8s.DefaultNamespace(proxy.Namespace))
					cacert, err := p.source.LookupCASecret(secretName, proxy.Namespace)
					if err != nil {
						if _, ok := err.(DelegationNotPermittedError); ok {
							validCond.AddErrorf(contour_v1.ConditionTypeTLSError, "DelegationNotPermitted",
								"Spec.VirtualHost.TLS CA Secret %q is invalid: %s", tls.ClientValidation.CACertificate, err)
						} else {
							// PeerValidationContext is requested, but cert is missing or not configured.
							validCond.AddErrorf(contour_v1.ConditionTypeTLSError, "ClientValidationInvalid",
								"Spec.VirtualHost.TLS client validation is invalid: invalid CA Secret %q: %s", secretName, err)
						}
						return
					}
					dv.CACertificates = []*Secret{
						cacert,
					}
				} else if !tls.ClientValidation.SkipClientCertValidation {
					validCond.AddErrorf(contour_v1.ConditionTypeTLSError, "ClientValidationInvalid",
						"Spec.VirtualHost.TLS client validation is invalid: CA Secret must be specified")
				}
				if tls.ClientValidation.CertificateRevocationList != "" {
					secretName := k8s.NamespacedNameFrom(tls.ClientValidation.CertificateRevocationList, k8s.DefaultNamespace(proxy.Namespace))
					crl, err := p.source.LookupCRLSecret(secretName, proxy.Namespace)
					if err != nil {
						if _, ok := err.(DelegationNotPermittedError); ok {
							validCond.AddErrorf(contour_v1.ConditionTypeTLSError, "DelegationNotPermitted",
								"Spec.VirtualHost.TLS CRL Secret %q is invalid: %s", tls.ClientValidation.CertificateRevocationList, err)
						} else {
							// CRL is missing or not configured.
							validCond.AddErrorf(contour_v1.ConditionTypeTLSError, "ClientValidationInvalid",
								"Spec.VirtualHost.TLS client validation is invalid: invalid CRL Secret %q: %s", secretName, err)
						}
						return
					}
					dv.CRL = crl
					dv.OnlyVerifyLeafCertCrl = tls.ClientValidation.OnlyVerifyLeafCertCrl
				}
				svhost.DownstreamValidation = dv
			}

			if !p.computeSecureVirtualHostAuthorization(validCond, proxy, svhost) {
				return
			}

			providerNames := sets.NewString()
			for _, jwtProvider := range proxy.Spec.VirtualHost.JWTProviders {
				if providerNames.Has(jwtProvider.Name) {
					validCond.AddErrorf(contour_v1.ConditionTypeJWTVerificationError, "DuplicateProviderName",
						"Spec.VirtualHost.JWTProviders is invalid: duplicate name %s", jwtProvider.Name)
					return
				}
				providerNames.Insert(jwtProvider.Name)

				if jwtProvider.Default {
					if len(defaultJWTProvider) > 0 {
						validCond.AddErrorf(contour_v1.ConditionTypeJWTVerificationError, "MultipleDefaultProvidersSpecified",
							"Spec.VirtualHost.JWTProviders is invalid: at most one provider can be set as the default")
						return
					}
					defaultJWTProvider = jwtProvider.Name
				}

				jwksURL, err := url.Parse(jwtProvider.RemoteJWKS.URI)
				if err != nil {
					validCond.AddErrorf(contour_v1.ConditionTypeJWTVerificationError, "RemoteJWKSURIInvalid",
						"Spec.VirtualHost.JWTProviders.RemoteJWKS.URI is invalid: %s", err)
					return
				}

				if jwksURL.Scheme != "http" && jwksURL.Scheme != "https" {
					validCond.AddErrorf(contour_v1.ConditionTypeJWTVerificationError, "RemoteJWKSSchemeInvalid",
						"Spec.VirtualHost.JWTProviders.RemoteJWKS.URI has invalid scheme %q, must be http or https", jwksURL.Scheme)
					return
				}

				var uv *PeerValidationContext

				if jwtProvider.RemoteJWKS.UpstreamValidation != nil {
					if jwksURL.Scheme == "http" {
						validCond.AddErrorf(contour_v1.ConditionTypeJWTVerificationError, "RemoteJWKSUpstreamValidationInvalid",
							"Spec.VirtualHost.JWTProviders.RemoteJWKS.UpstreamValidation must not be specified when URI scheme is http.")
						return
					}

					caCertNamespacedName := k8s.NamespacedNameFrom(jwtProvider.RemoteJWKS.UpstreamValidation.CACertificate, k8s.DefaultNamespace(proxy.Namespace))
					uv, err = p.source.LookupUpstreamValidation(jwtProvider.RemoteJWKS.UpstreamValidation, caCertNamespacedName, proxy.Namespace)
					if err != nil {
						if _, ok := err.(DelegationNotPermittedError); ok {
							validCond.AddErrorf(contour_v1.ConditionTypeJWTVerificationError, "RemoteJWKSCACertificateNotDelegated",
								"Spec.VirtualHost.JWTProviders.RemoteJWKS.UpstreamValidation.CACertificate Secret %q is not configured for certificate delegation", caCertNamespacedName)
						} else {
							validCond.AddErrorf(contour_v1.ConditionTypeJWTVerificationError, "RemoteJWKSUpstreamValidationInvalid",
								"Spec.VirtualHost.JWTProviders.RemoteJWKS.UpstreamValidation is invalid: %s", err)
						}
						return
					}
				}

				jwksTimeout := time.Second
				if len(jwtProvider.RemoteJWKS.Timeout) > 0 {
					res, err := time.ParseDuration(jwtProvider.RemoteJWKS.Timeout)
					if err != nil {
						validCond.AddErrorf(contour_v1.ConditionTypeJWTVerificationError, "RemoteJWKSTimeoutInvalid",
							"Spec.VirtualHost.JWTProviders.RemoteJWKS.Timeout is invalid: %s", err)
						return
					}

					jwksTimeout = res
				}

				var cacheDuration *time.Duration
				if len(jwtProvider.RemoteJWKS.CacheDuration) > 0 {
					res, err := time.ParseDuration(jwtProvider.RemoteJWKS.CacheDuration)
					if err != nil {
						validCond.AddErrorf(contour_v1.ConditionTypeJWTVerificationError, "RemoteJWKSCacheDurationInvalid",
							"Spec.VirtualHost.JWTProviders.RemoteJWKS.CacheDuration is invalid: %s", err)
						return
					}

					cacheDuration = &res
				}

				// Check for a specified port and use it, else use the
				// standard ports by scheme.
				var port int
				switch {
				case len(jwksURL.Port()) > 0:
					p, err := strconv.Atoi(jwksURL.Port())
					if err != nil {
						// This theoretically shouldn't be possible as jwksURL.Port() will
						// only return a value if it's numeric, but we need to convert to
						// int anyway so handle the error.
						validCond.AddErrorf(contour_v1.ConditionTypeJWTVerificationError, "RemoteJWKSPortInvalid",
							"Spec.VirtualHost.JWTProviders.RemoteJWKS.URI has an invalid port: %s", err)
						return
					}
					port = p
				case jwksURL.Scheme == "http":
					port = 80
				case jwksURL.Scheme == "https":
					port = 443
				}

				// Get the DNS lookup family if specified, otherwise
				// default to the Contour-wide setting.
				dnsLookupFamily := ""
				switch jwtProvider.RemoteJWKS.DNSLookupFamily {
				case "auto", "v4", "v6", "all":
					dnsLookupFamily = jwtProvider.RemoteJWKS.DNSLookupFamily
				case "":
					dnsLookupFamily = string(p.DNSLookupFamily)
				default:
					validCond.AddErrorf(contour_v1.ConditionTypeJWTVerificationError, "RemoteJWKSDNSLookupFamilyInvalid",
						"Spec.VirtualHost.JWTProviders.RemoteJWKS.DNSLookupFamily has an invalid value %q, must be auto, all, v4 or v6", jwtProvider.RemoteJWKS.DNSLookupFamily)
					return
				}

				svhost.JWTProviders = append(svhost.JWTProviders, JWTProvider{
					Name:      jwtProvider.Name,
					Issuer:    jwtProvider.Issuer,
					Audiences: jwtProvider.Audiences,
					RemoteJWKS: RemoteJWKS{
						URI:     jwtProvider.RemoteJWKS.URI,
						Timeout: jwksTimeout,
						Cluster: DNSNameCluster{
							Address:            jwksURL.Hostname(),
							Scheme:             jwksURL.Scheme,
							Port:               port,
							DNSLookupFamily:    dnsLookupFamily,
							UpstreamValidation: uv,
							UpstreamTLS:        p.UpstreamTLS,
						},
						CacheDuration: cacheDuration,
					},
					ForwardJWT: jwtProvider.ForwardJWT,
				})
			}
		}
	}

	if proxy.Spec.TCPProxy != nil {
		if !tlsEnabled {
			validCond.AddError(contour_v1.ConditionTypeTCPProxyError, "TLSMustBeConfigured",
				"Spec.TCPProxy requires that either Spec.TLS.Passthrough or Spec.TLS.SecretName be set")
			return
		}
		if !p.processHTTPProxyTCPProxy(validCond, proxy, nil, host) {
			return
		}
	}

	routes := p.computeRoutes(validCond, proxy, proxy, nil, nil, tlsEnabled, defaultJWTProvider)

	listener, err := p.dag.GetSingleListener("http")
	if err != nil {
		validCond.AddError(contour_v1.ConditionTypeListenerError, "ErrorIdentifyingListener", err.Error())
		return
	}

	insecure := p.dag.EnsureVirtualHost(listener.Name, host)

	cp, err := toCORSPolicy(proxy.Spec.VirtualHost.CORSPolicy)
	if err != nil {
		validCond.AddErrorf(contour_v1.ConditionTypeCORSError, "PolicyDidNotParse",
			"Spec.VirtualHost.CORSPolicy: %s", err)
		return
	}
	insecure.CORSPolicy = cp

	var isValidRLP bool
	insecure.RateLimitPolicy, isValidRLP = computeVirtualHostRateLimitPolicy(proxy, p.GlobalRateLimitService, validCond)
	if !isValidRLP {
		return
	}

	if p.GlobalExternalAuthorization != nil && !proxy.Spec.VirtualHost.DisableAuthorization() {
		p.computeVirtualHostAuthorization(p.GlobalExternalAuthorization, validCond, proxy)
	}

	insecure.IPFilterAllow, insecure.IPFilterRules, err = toIPFilterRules(proxy.Spec.VirtualHost.IPAllowFilterPolicy, proxy.Spec.VirtualHost.IPDenyFilterPolicy, validCond)
	if err != nil {
		validCond.AddErrorf(contour_v1.ConditionTypeIPFilterError, "IPFilterPolicyNotValid",
			"Spec.VirtualHost.IPAllowFilterPolicy or Spec.VirtualHost.IPDenyFilterPolicy is invalid: %s", err)
		return
	}

	addRoutes(insecure, routes)

	// if TLS is enabled for this virtual host and there is no tcp proxy defined,
	// then add routes to the secure virtualhost definition.
	if tlsEnabled && proxy.Spec.TCPProxy == nil {
		listener, err := p.dag.GetSingleListener("https")
		if err != nil {
			validCond.AddError(contour_v1.ConditionTypeListenerError, "ErrorIdentifyingListener", err.Error())
			return
		}

		secure := p.dag.EnsureSecureVirtualHost(listener.Name, host)
		secure.CORSPolicy = cp

		secure.RateLimitPolicy, isValidRLP = computeVirtualHostRateLimitPolicy(proxy, p.GlobalRateLimitService, validCond)
		if !isValidRLP {
			return
		}

		secure.IPFilterAllow, secure.IPFilterRules, err = toIPFilterRules(proxy.Spec.VirtualHost.IPAllowFilterPolicy, proxy.Spec.VirtualHost.IPDenyFilterPolicy, validCond)
		if err != nil {
			validCond.AddErrorf(contour_v1.ConditionTypeIPFilterError, "IPFilterPolicyNotValid",
				"Spec.VirtualHost.IPAllowFilterPolicy or Spec.VirtualHost.IPDenyFilterPolicy is invalid: %s", err)
			return
		}

		addRoutes(secure, routes)

		// Process JWT verification requirements.
		for _, route := range routes {
			// JWT verification not enabled for the vhost: error if the route
			// specifies a JWT provider.
			if len(secure.JWTProviders) == 0 {
				if len(route.JWTProvider) == 0 {
					continue
				}

				validCond.AddErrorf(contour_v1.ConditionTypeJWTVerificationError, "JWTProviderNotDefined",
					"Route references an undefined JWT provider %q", route.JWTProvider)
				return
			}

			// JWT verification enabled for the vhost: error if the route
			// specifies a JWT provider that does not exist.
			if len(route.JWTProvider) > 0 {
				var found bool
				for _, provider := range secure.JWTProviders {
					if provider.Name == route.JWTProvider {
						found = true
						break
					}
				}

				if !found {
					validCond.AddErrorf(contour_v1.ConditionTypeJWTVerificationError, "JWTProviderNotDefined",
						"Route references an undefined JWT provider %q", route.JWTProvider)
					return
				}
			}
		}
	}
}

type vhost interface {
	AddRoute(*Route)
}

// addRoutes adds all routes to the vhost supplied.
func addRoutes(vhost vhost, routes []*Route) {
	for _, route := range routes {
		vhost.AddRoute(route)
	}
}

func (p *HTTPProxyProcessor) addStatusBadGatewayRoute(routes []*Route, conds []contour_v1.MatchCondition, proxy *contour_v1.HTTPProxy) []*Route {
	if len(conds) > 0 {
		route := &Route{
			PathMatchCondition:        mergePathMatchConditions(conds),
			HeaderMatchConditions:     mergeHeaderMatchConditions(conds),
			QueryParamMatchConditions: mergeQueryParamMatchConditions(conds),
			DirectResponse:            directResponse(http.StatusBadGateway, ""),
		}

		if p.SetSourceMetadataOnRoutes {
			route.Kind = "HTTPProxy"
			route.Namespace = proxy.Namespace
			route.Name = proxy.Name
		}

		if p.EnableStatPrefix {
			route.StatPrefix = ptr.To(fmt.Sprintf("%s_%s", proxy.Namespace, proxy.Name))
		}

		routes = append(routes, route)
	}
	return routes
}

func (p *HTTPProxyProcessor) computeRoutes(
	validCond *contour_v1.DetailedCondition,
	rootProxy *contour_v1.HTTPProxy,
	proxy *contour_v1.HTTPProxy,
	conditions []contour_v1.MatchCondition,
	visited []*contour_v1.HTTPProxy,
	enforceTLS bool,
	defaultJWTProvider string,
) []*Route {
	for _, v := range visited {
		// ensure we are not following an edge that produces a cycle
		var path []string
		for _, vir := range visited {
			path = append(path, fmt.Sprintf("%s/%s", vir.Namespace, vir.Name))
		}
		if v.Name == proxy.Name && v.Namespace == proxy.Namespace {
			path = append(path, fmt.Sprintf("%s/%s", proxy.Namespace, proxy.Name))
			validCond.AddErrorf(contour_v1.ConditionTypeIncludeError, "IncludeCreatesCycle",
				"include creates an include cycle: %s", strings.Join(path, " -> "))
			return nil
		}
	}

	visited = append(visited, proxy)
	var routes []*Route

	// Loop over and process all includes, including checking for duplicate conditions.
	seenConds := map[string][]matchConditionAggregate{}
	for _, include := range proxy.Spec.Includes {
		namespace := include.Namespace
		if namespace == "" {
			namespace = proxy.Namespace
		}

		if err := includeMatchConditionsValid(include.Conditions); err != nil {
			validCond.AddErrorf(contour_v1.ConditionTypeIncludeError, "PathMatchConditionsNotValid",
				"include: %s", err)
			continue
		}

		if err := pathMatchConditionsValid(include.Conditions); err != nil {
			validCond.AddErrorf(contour_v1.ConditionTypeIncludeError, "PathMatchConditionsNotValid",
				"include: %s", err)
			continue
		}

		if err := headerMatchConditionsValid(include.Conditions); err != nil {
			validCond.AddError(contour_v1.ConditionTypeRouteError, "HeaderMatchConditionsNotValid",
				err.Error())
			continue
		}

		if err := queryParameterMatchConditionsValid(include.Conditions); err != nil {
			validCond.AddError(contour_v1.ConditionTypeRouteError, "QueryParameterMatchConditionsNotValid",
				err.Error())
			continue
		}

		// Check to see if we have any duplicate include conditions.
		if includeMatchConditionsIdentical(include.Conditions, seenConds) {
			validCond.AddError(contour_v1.ConditionTypeIncludeError, "DuplicateMatchConditions",
				"duplicate conditions defined on an include")
			continue
		}

		includedProxy, ok := p.source.httpproxies[types.NamespacedName{Name: include.Name, Namespace: namespace}]
		if !ok {
			validCond.AddErrorf(contour_v1.ConditionTypeIncludeError, "IncludeNotFound",
				"include %s/%s not found", namespace, include.Name)

			// Set 502 response when include was not found but include condition was valid.
			routes = p.addStatusBadGatewayRoute(routes, include.Conditions, proxy)
			continue
		}

		if includedProxy.Spec.VirtualHost != nil {
			validCond.AddErrorf(contour_v1.ConditionTypeIncludeError, "RootIncludesRoot",
				"root httpproxy cannot include another root httpproxy (%s/%s)", includedProxy.Namespace, includedProxy.Name)
			// Set 502 response if include references another root
			routes = p.addStatusBadGatewayRoute(routes, include.Conditions, proxy)
			continue
		}

		inc, incCommit := p.dag.StatusCache.ProxyAccessor(includedProxy)
		incValidCond := inc.ConditionFor(status.ValidCondition)
		routes = append(routes, p.computeRoutes(incValidCond, rootProxy, includedProxy, append(conditions, include.Conditions...), visited, enforceTLS, defaultJWTProvider)...)
		incCommit()

		// dest is not an orphaned httpproxy, as there is an httpproxy that points to it
		delete(p.orphaned, types.NamespacedName{Name: includedProxy.Name, Namespace: includedProxy.Namespace})
	}

	dynamicHeaders := map[string]string{
		"CONTOUR_NAMESPACE": proxy.Namespace,
	}

	for _, route := range proxy.Spec.Routes {
		if err := routeActionCountValid(route); err != nil {
			validCond.AddError(contour_v1.ConditionTypeRouteError, "RouteActionCountNotValid", err.Error())
			return nil
		}

		if err := pathMatchConditionsValid(route.Conditions); err != nil {
			validCond.AddErrorf(contour_v1.ConditionTypeRouteError, "PathMatchConditionsNotValid",
				"route: %s", err)
			return nil
		}

		routeConditions := conditions
		routeConditions = append(routeConditions, route.Conditions...)

		// Look for invalid header conditions on this route
		if err := headerMatchConditionsValid(routeConditions); err != nil {
			validCond.AddError(contour_v1.ConditionTypeRouteError, "HeaderMatchConditionsNotValid",
				err.Error())
			return nil
		}

		// Look for invalid query parameter conditions on this route
		if err := queryParameterMatchConditionsValid(routeConditions); err != nil {
			validCond.AddError(contour_v1.ConditionTypeRouteError, "QueryParameterMatchConditionsNotValid",
				err.Error())
			return nil
		}

		reqHP, err := headersPolicyRoute(route.RequestHeadersPolicy, true /* allow Host */, dynamicHeaders)
		if err != nil {
			validCond.AddErrorf(contour_v1.ConditionTypeRouteError, "RequestHeadersPolicyInvalid",
				"%s on request headers", err)
			return nil
		}

		respHP, err := headersPolicyRoute(route.ResponseHeadersPolicy, false /* disallow Host */, dynamicHeaders)
		if err != nil {
			validCond.AddErrorf(contour_v1.ConditionTypeRouteError, "ResponseHeaderPolicyInvalid",
				"%s on response headers", err)
			return nil
		}

		cookieRP, err := cookieRewritePolicies(route.CookieRewritePolicies)
		if err != nil {
			validCond.AddErrorf(contour_v1.ConditionTypeRouteError, "CookieRewritePoliciesInvalid",
				"%s on route cookie rewrite rules", err)
			return nil
		}

		rtp, ctp, err := timeoutPolicy(route.TimeoutPolicy, p.ConnectTimeout)
		if err != nil {
			validCond.AddErrorf(contour_v1.ConditionTypeRouteError, "TimeoutPolicyNotValid",
				"route.timeoutPolicy failed to parse: %s", err)
			return nil
		}

		rlp, err := rateLimitPolicy(route.RateLimitPolicy)
		if err != nil {
			validCond.AddErrorf(contour_v1.ConditionTypeRouteError, "RateLimitPolicyNotValid",
				"route.rateLimitPolicy is invalid: %s", err)
			return nil
		}

		vrl := rateLimitPerRoute(route.RateLimitPolicy)

		requestHashPolicies, lbPolicy := loadBalancerRequestHashPolicies(route.LoadBalancerPolicy, validCond)

		redirectPolicy, err := redirectRoutePolicy(route.RequestRedirectPolicy)
		if err != nil {
			validCond.AddErrorf(contour_v1.ConditionTypeRouteError, "RequestRedirectPolicy",
				"route.requestRedirectPolicy is invalid: %s", err)
			return nil
		}

		irp := internalRedirectPolicy(route.InternalRedirectPolicy)

		directPolicy := directResponsePolicy(route.DirectResponsePolicy)

		r := &Route{
			PathMatchCondition:        mergePathMatchConditions(routeConditions),
			HeaderMatchConditions:     mergeHeaderMatchConditions(routeConditions),
			QueryParamMatchConditions: mergeQueryParamMatchConditions(routeConditions),
			Websocket:                 route.EnableWebsockets,
			HTTPSUpgrade:              routeEnforceTLS(enforceTLS, route.PermitInsecure && !p.DisablePermitInsecure),
			TimeoutPolicy:             rtp,
			RetryPolicy:               retryPolicy(route.RetryPolicy),
			RequestHeadersPolicy:      reqHP,
			ResponseHeadersPolicy:     respHP,
			CookieRewritePolicies:     cookieRP,
			RateLimitPolicy:           rlp,
			RateLimitPerRoute:         vrl,
			RequestHashPolicies:       requestHashPolicies,
			Redirect:                  redirectPolicy,
			DirectResponse:            directPolicy,
			InternalRedirectPolicy:    irp,
		}

		if p.SetSourceMetadataOnRoutes {
			r.Kind = "HTTPProxy"
			r.Namespace = proxy.Namespace
			r.Name = proxy.Name
		}

		if p.EnableStatPrefix {
			path := "_"
			if len(route.Conditions) > 0 && route.Conditions[0].Prefix != "" {
				path = strings.TrimPrefix(route.Conditions[0].Prefix, "/")
			}
			r.StatPrefix = ptr.To(fmt.Sprintf("%s_httpproxy_%s_path_%s", proxy.Namespace, proxy.Name, path))
		}

		// If the enclosing root proxy enabled authorization,
		// enable it on the route and propagate defaults
		// downwards.
		if rootProxy.Spec.VirtualHost.AuthorizationConfigured() || p.GlobalExternalAuthorization != nil {
			// Global external or vhost-level authorization is enabled by default
			// unless an AuthPolicy explicitly disables it. By default, `disabled`
			// is set to false, meaning authorization is active. This global setting
			// can be overridden by vhost-level AuthPolicy, which can further be
			// overridden by route-specific AuthPolicy.
			// Therefore, the final authorization state is determined by the
			// most specific policy applied at the route level.
			disabled := false

			if p.GlobalExternalAuthorization != nil && p.GlobalExternalAuthorization.AuthPolicy != nil {
				disabled = p.GlobalExternalAuthorization.AuthPolicy.Disabled
			}

			// When the ext_authz filter is added to a
			// vhost, it is in enabled state, but we can
			// disable it per route. We emulate disabling
			// it at the vhost layer by defaulting the state
			// from the root proxy.
			if rootProxy.Spec.VirtualHost.AuthorizationConfigured() {
				disabled = rootProxy.Spec.VirtualHost.DisableAuthorization()
			}

			// Take the default for enabling authorization
			// from the virtualhost/global-extauth. If this
			// route has a policy, let that override.
			if route.AuthPolicy != nil {
				disabled = route.AuthPolicy.Disabled
			}

			r.AuthDisabled = disabled

			if rootProxy.Spec.VirtualHost.AuthorizationConfigured() {
				r.AuthContext = route.AuthorizationContext(rootProxy.Spec.VirtualHost.AuthorizationContext())
			} else if p.GlobalExternalAuthorization != nil {
				r.AuthContext = route.AuthorizationContext(p.GlobalAuthorizationContext())
			}
		}

		if len(route.GetPrefixReplacements()) > 0 {
			if !r.HasPathPrefix() {
				validCond.AddError(contour_v1.ConditionTypePrefixReplaceError, "MustHavePrefix",
					"cannot specify prefix replacements without a prefix condition")
				return nil
			}

			if reason, err := prefixReplacementsAreValid(route.GetPrefixReplacements()); err != nil {
				validCond.AddError(contour_v1.ConditionTypePrefixReplaceError, reason, err.Error())
				return nil
			}

			// Note that we are guaranteed to always have a prefix
			// condition. Even if the CRD user didn't specify a
			// prefix condition, mergePathConditions() guarantees
			// a prefix of '/'.
			routingPrefix := r.PathMatchCondition.(*PrefixMatchCondition).Prefix

			// First, try to apply an exact prefix match.
			for _, prefix := range route.GetPrefixReplacements() {
				if len(prefix.Prefix) > 0 && routingPrefix == prefix.Prefix {
					r.PathRewritePolicy = &PathRewritePolicy{
						PrefixRewrite: prefix.Replacement,
					}
					break
				}
			}

			// If there wasn't a match, we can apply the default replacement.
			if r.PathRewritePolicy == nil {
				for _, prefix := range route.GetPrefixReplacements() {
					if len(prefix.Prefix) == 0 {
						r.PathRewritePolicy = &PathRewritePolicy{
							PrefixRewrite: prefix.Replacement,
						}
						break
					}
				}
			}
		}

		healthPolicy, err := httpHealthCheckPolicy(route.HealthCheckPolicy)
		if err != nil {
			validCond.AddError(contour_v1.ConditionTypeRouteError, "HealthCheckPolicyInvalid", err.Error())
			return nil
		}

		for _, service := range route.Services {
			if service.Port < 1 || service.Port > 65535 {
				validCond.AddErrorf(contour_v1.ConditionTypeServiceError, "ServicePortInvalid",
					"service %q: port must be in the range 1-65535", service.Name)
				return nil
			}

			var healthPort int
			if healthPolicy != nil && service.HealthPort > 0 {
				healthPort = service.HealthPort
			} else {
				healthPort = service.Port
			}

			m := types.NamespacedName{Name: service.Name, Namespace: proxy.Namespace}
			s, err := p.dag.EnsureService(m, service.Port, healthPort, p.source, p.EnableExternalNameService)
			if err != nil {
				validCond.AddErrorf(contour_v1.ConditionTypeServiceError, "ServiceUnresolvedReference",
					"Spec.Routes unresolved service reference: %s", err)
				continue
			}
			s = serviceCircuitBreakerPolicy(s, p.GlobalCircuitBreakerDefaults)

			// Determine the protocol to use to speak to this Cluster.
			protocol, err := getProtocol(service, s)
			if err != nil {
				validCond.AddError(contour_v1.ConditionTypeServiceError, "UnsupportedProtocol", err.Error())
				return nil
			}

			var uv *PeerValidationContext
			if (protocol == "tls" || protocol == "h2") && service.UpstreamValidation != nil {
				caCertNamespacedName := k8s.NamespacedNameFrom(service.UpstreamValidation.CACertificate, k8s.DefaultNamespace(proxy.Namespace))
				// we can only validate TLS connections to services that talk TLS
				uv, err = p.source.LookupUpstreamValidation(service.UpstreamValidation, caCertNamespacedName, proxy.Namespace)
				if err != nil {
					if _, ok := err.(DelegationNotPermittedError); ok {
						validCond.AddErrorf(contour_v1.ConditionTypeTLSError, "CACertificateNotDelegated",
							"service.UpstreamValidation.CACertificate Secret %q is not configured for certificate delegation", caCertNamespacedName)
					} else {
						validCond.AddErrorf(contour_v1.ConditionTypeServiceError, "TLSUpstreamValidation",
							"Service [%s:%d] TLS upstream validation policy error: %s", service.Name, service.Port, err)
					}
					return nil
				}
			}

			dynamicHeaders["CONTOUR_SERVICE_NAME"] = service.Name
			dynamicHeaders["CONTOUR_SERVICE_PORT"] = strconv.Itoa(service.Port)

			reqHP, err := headersPolicyService(p.RequestHeadersPolicy, service.RequestHeadersPolicy, true, dynamicHeaders)
			if err != nil {
				validCond.AddErrorf(contour_v1.ConditionTypeServiceError, "RequestHeadersPolicyInvalid",
					"%s on request headers", err)
				return nil
			}
			respHP, err := headersPolicyService(p.ResponseHeadersPolicy, service.ResponseHeadersPolicy, false, dynamicHeaders)
			if err != nil {
				validCond.AddErrorf(contour_v1.ConditionTypeServiceError, "ResponseHeadersPolicyInvalid",
					"%s on response headers", err)
				return nil
			}

			cookieRP, err := cookieRewritePolicies(service.CookieRewritePolicies)
			if err != nil {
				validCond.AddErrorf(contour_v1.ConditionTypeRouteError, "CookieRewritePoliciesInvalid",
					"%s on service cookie rewrite rules", err)
				return nil
			}

			var clientCertSecret *Secret
			if p.ClientCertificate != nil {
				// Since the client certificate is configured by admin, explicit delegation is not required.
				clientCertSecret, err = p.source.LookupTLSSecretInsecure(*p.ClientCertificate)
				if err != nil {
					validCond.AddErrorf(contour_v1.ConditionTypeTLSError, "SecretNotValid",
						"tls.envoy-client-certificate Secret %q is invalid: %s", p.ClientCertificate, err)
					return nil
				}
			}

			var slowStart *SlowStartConfig
			if service.SlowStartPolicy != nil {
				// Currently Envoy implements slow start only for RoundRobin and WeightedLeastRequest LB strategies.
				if lbPolicy != "" && lbPolicy != LoadBalancerPolicyRoundRobin && lbPolicy != LoadBalancerPolicyWeightedLeastRequest {
					validCond.AddErrorf(contour_v1.ConditionTypeServiceError, "SlowStartInvalid",
						"slow start is only supported with RoundRobin or WeightedLeastRequest load balancer strategy")
					return nil
				}

				slowStart, err = slowStartConfig(service.SlowStartPolicy)
				if err != nil {
					validCond.AddErrorf(contour_v1.ConditionTypeServiceError, "SlowStartInvalid",
						"%s on slow start", err)
					return nil
				}
			}

			c := &Cluster{
				Upstream:                      s,
				LoadBalancerPolicy:            lbPolicy,
				Weight:                        uint32(service.Weight), //nolint:gosec // disable G115
				HTTPHealthCheckPolicy:         healthPolicy,
				UpstreamValidation:            uv,
				RequestHeadersPolicy:          reqHP,
				ResponseHeadersPolicy:         respHP,
				CookieRewritePolicies:         cookieRP,
				Protocol:                      protocol,
				SNI:                           determineSNI(r.RequestHeadersPolicy, reqHP, s),
				DNSLookupFamily:               string(p.DNSLookupFamily),
				ClientCertificate:             clientCertSecret,
				TimeoutPolicy:                 ctp,
				SlowStartConfig:               slowStart,
				MaxRequestsPerConnection:      p.MaxRequestsPerConnection,
				PerConnectionBufferLimitBytes: p.PerConnectionBufferLimitBytes,
				UpstreamTLS:                   p.UpstreamTLS,
			}
			if service.Mirror && len(r.MirrorPolicies) > 0 {
				validCond.AddError(contour_v1.ConditionTypeServiceError, "OnlyOneMirror",
					"only one service per route may be nominated as mirror")
				return nil
			}
			if service.Mirror {
				// Legal Weight values are 1-100. The default value of a float64 if Weight is omitted
				// is 0. To retain backwards compatibility omitted weights will be treated as 100% mirroring.
				// EDGE CASE: This means that explicitly setting Weight to 0 will also result in 100%
				// mirroring. The Mirror field must be set to false or removed to disable the mirror.
				if service.Weight == 0 {
					r.MirrorPolicies = []*MirrorPolicy{{
						Cluster: c,
						Weight:  100,
					}}
				} else {
					r.MirrorPolicies = []*MirrorPolicy{{
						Cluster: c,
						Weight:  service.Weight,
					}}
				}
			} else {
				r.Clusters = append(r.Clusters, c)
			}
		}
		if len(r.Clusters) == 0 && route.RequestRedirectPolicy == nil && route.DirectResponsePolicy == nil {
			r.DirectResponse = directResponse(http.StatusServiceUnavailable, "")
		}

		// If we have a wildcard match, add a header match regex rule to match the
		// hostname so we can be sure to only match one DNS label. This is required
		// as Envoy's virtualhost hostname wildcard matching can match multiple
		// labels. This match ignores a port in the hostname in case it is present.
		if strings.HasPrefix(rootProxy.Spec.VirtualHost.Fqdn, "*.") {
			r.HeaderMatchConditions = append(r.HeaderMatchConditions, wildcardDomainHeaderMatch(rootProxy.Spec.VirtualHost.Fqdn))
		}

		jwt := route.JWTVerificationPolicy
		switch {
		case jwt != nil && len(route.JWTVerificationPolicy.Require) > 0 && route.JWTVerificationPolicy.Disabled:
			validCond.AddError(contour_v1.ConditionTypeJWTVerificationError, "InvalidJWTVerificationPolicy",
				"route's JWT verification policy cannot specify both require and disabled")
			return nil
		case jwt != nil && len(route.JWTVerificationPolicy.Require) > 0:
			r.JWTProvider = jwt.Require
		case jwt != nil && jwt.Disabled:
			r.JWTProvider = ""
		default:
			r.JWTProvider = defaultJWTProvider
		}

		r.IPFilterAllow, r.IPFilterRules, err = toIPFilterRules(route.IPAllowFilterPolicy, route.IPDenyFilterPolicy, validCond)
		if err != nil {
			return nil
		}

		routes = append(routes, r)
	}

	routes = expandPrefixMatches(routes)

	return routes
}

// toIPFilterRules converts ip filter settings from the api into the
// dag representation
func toIPFilterRules(allowPolicy, denyPolicy []contour_v1.IPFilterPolicy, validCond *contour_v1.DetailedCondition) (allow bool, filters []IPFilterRule, err error) {
	var ipPolicies []contour_v1.IPFilterPolicy
	switch {
	case len(allowPolicy) > 0 && len(denyPolicy) > 0:
		validCond.AddError(contour_v1.ConditionTypeIPFilterError, "IncompatibleIPAddressFilters",
			"cannot specify both `ipAllowPolicy` and `ipDenyPolicy`")
		err = fmt.Errorf("invalid ip filter")
		return
	case len(allowPolicy) > 0:
		allow = true
		ipPolicies = allowPolicy
	case len(denyPolicy) > 0:
		allow = false
		ipPolicies = denyPolicy
	}
	if ipPolicies == nil {
		return
	}
	filters = make([]IPFilterRule, 0, len(ipPolicies))
	for _, p := range ipPolicies {
		// convert bare IPs to CIDRs
		unparsedCIDR := p.CIDR
		if !strings.Contains(unparsedCIDR, "/") {
			if strings.Contains(unparsedCIDR, ":") {
				unparsedCIDR += "/128"
			} else {
				unparsedCIDR += "/32"
			}
		}
		var cidr *net.IPNet
		_, cidr, err = net.ParseCIDR(unparsedCIDR)
		if err != nil {
			validCond.AddErrorf(contour_v1.ConditionTypeIPFilterError, "InvalidCIDR",
				"%s failed to parse: %s", p.CIDR, err)
			continue
		}
		filters = append(filters, IPFilterRule{
			Remote: p.Source == contour_v1.IPFilterSourceRemote,
			CIDR:   *cidr,
		})
	}
	if err != nil {
		allow = false
		filters = nil
	}
	return
}

// processHTTPProxyTCPProxy processes the spec.tcpproxy stanza in a HTTPProxy document
// following the chain of spec.tcpproxy.include references. It returns true if processing
// was successful, otherwise false if an error was encountered. The details of the error
// will be recorded on the status of the relevant HTTPProxy object,
func (p *HTTPProxyProcessor) processHTTPProxyTCPProxy(validCond *contour_v1.DetailedCondition, httpproxy *contour_v1.HTTPProxy, visited []*contour_v1.HTTPProxy, host string) bool {
	tcpproxy := httpproxy.Spec.TCPProxy
	if tcpproxy == nil {
		// nothing to do
		return true
	}

	visited = append(visited, httpproxy)

	// #2218 Allow support for both plural and singular "Include" for TCPProxy for the v1 API Spec
	// Prefer configurations for singular over the plural version
	tcpProxyInclude := tcpproxy.Include
	if tcpproxy.Include == nil {
		tcpProxyInclude = tcpproxy.IncludesDeprecated
	}

	if len(tcpproxy.Services) > 0 && tcpProxyInclude != nil {
		validCond.AddError(contour_v1.ConditionTypeTCPProxyError, "NoServicesAndInclude",
			"cannot specify services and include in the same httpproxy")
		return false
	}

	lbPolicy := loadBalancerPolicy(tcpproxy.LoadBalancerPolicy)
	switch lbPolicy {
	case LoadBalancerPolicyCookie, LoadBalancerPolicyRequestHash:
		validCond.AddWarningf(contour_v1.ConditionTypeTCPProxyError, "IgnoredField",
			"ignoring field %q; %s load balancer policy is not supported for TCPProxies",
			"Spec.TCPProxy.LoadBalancerPolicy", lbPolicy)
		// Reset load balancer policy to ensure the default.
		lbPolicy = ""
	}

	if len(tcpproxy.Services) > 0 {
		var proxy TCPProxy
		for _, service := range httpproxy.Spec.TCPProxy.Services {
			var healthPort int
			healthPolicy := tcpHealthCheckPolicy(tcpproxy.HealthCheckPolicy)
			if healthPolicy != nil && service.HealthPort > 0 {
				healthPort = service.HealthPort
			} else {
				healthPort = service.Port
			}

			m := types.NamespacedName{Name: service.Name, Namespace: httpproxy.Namespace}
			s, err := p.dag.EnsureService(m, service.Port, healthPort, p.source, p.EnableExternalNameService)
			if err != nil {
				validCond.AddErrorf(contour_v1.ConditionTypeTCPProxyError, "ServiceUnresolvedReference",
					"Spec.TCPProxy unresolved service reference: %s", err)
				return false
			}

			// Determine the protocol to use to speak to this Cluster.
			protocol, err := getProtocol(service, s)
			if err != nil {
				validCond.AddError(contour_v1.ConditionTypeServiceError, "UnsupportedProtocol", err.Error())
				return false
			}

			var uv *PeerValidationContext
			if (protocol == "tls" || protocol == "h2") && service.UpstreamValidation != nil {
				uv = p.peerValidationContext(validCond, httpproxy, service)
				if uv == nil {
					return false
				}
			}

			var clientCertSecret *Secret
			if p.ClientCertificate != nil {
				// Since the client certificate is configured by admin, explicit delegation is not required.
				clientCertSecret, err = p.source.LookupTLSSecretInsecure(*p.ClientCertificate)
				if err != nil {
					validCond.AddErrorf(contour_v1.ConditionTypeTLSError, "SecretNotValid",
						"tls.envoy-client-certificate Secret %q is invalid: %s", p.ClientCertificate, err)
					return false
				}
			}

			proxy.Clusters = append(proxy.Clusters, &Cluster{
				Upstream:             s,
				Weight:               uint32(service.Weight), //nolint:gosec // disable G115
				Protocol:             protocol,
				LoadBalancerPolicy:   lbPolicy,
				TCPHealthCheckPolicy: healthPolicy,
				SNI:                  s.ExternalName,
				TimeoutPolicy:        ClusterTimeoutPolicy{ConnectTimeout: p.ConnectTimeout},
				UpstreamTLS:          p.UpstreamTLS,
				UpstreamValidation:   uv,
				ClientCertificate:    clientCertSecret,
			})
		}

		listener, err := p.dag.GetSingleListener("https")
		if err != nil {
			validCond.AddError(contour_v1.ConditionTypeListenerError, "ErrorIdentifyingListener", err.Error())
			return false
		}

		secure := p.dag.EnsureSecureVirtualHost(listener.Name, host)
		secure.TCPProxy = &proxy

		return true
	}

	if tcpProxyInclude == nil {
		// We don't allow an empty TCPProxy object.
		validCond.AddError(contour_v1.ConditionTypeTCPProxyError, "NothingDefined",
			"either services or inclusion must be specified")
		return false
	}

	namespace := tcpProxyInclude.Namespace
	if namespace == "" {
		// we are delegating to another HTTPProxy in the same namespace
		namespace = httpproxy.Namespace
	}

	m := types.NamespacedName{Name: tcpProxyInclude.Name, Namespace: namespace}
	dest, ok := p.source.httpproxies[m]
	if !ok {
		validCond.AddErrorf(contour_v1.ConditionTypeTCPProxyIncludeError, "IncludeNotFound",
			"include %s/%s not found", m.Namespace, m.Name)
		return false
	}

	if dest.Spec.VirtualHost != nil {

		validCond.AddErrorf(contour_v1.ConditionTypeTCPProxyIncludeError, "RootIncludesRoot",
			"root httpproxy cannot include another root httpproxy (%s/%s)", dest.Namespace, dest.Name)
		return false
	}

	// dest is no longer an orphan
	delete(p.orphaned, k8s.NamespacedNameOf(dest))

	// ensure we are not following an edge that produces a cycle
	var path []string
	for _, hp := range visited {
		path = append(path, fmt.Sprintf("%s/%s", hp.Namespace, hp.Name))
	}
	for _, hp := range visited {
		if dest.Name == hp.Name && dest.Namespace == hp.Namespace {
			path = append(path, fmt.Sprintf("%s/%s", dest.Namespace, dest.Name))
			validCond.AddErrorf(contour_v1.ConditionTypeTCPProxyIncludeError, "IncludeCreatesCycle",
				"include creates a cycle: %s", strings.Join(path, " -> "))
			return false
		}
	}

	// follow the link and process the target tcpproxy
	inc, commit := p.dag.StatusCache.ProxyAccessor(dest)
	incValidCond := inc.ConditionFor(status.ValidCondition)
	defer commit()
	ok = p.processHTTPProxyTCPProxy(incValidCond, dest, visited, host)
	return ok
}

// validHTTPProxies returns a slice of *contour_v1.HTTPProxy objects.
// invalid HTTPProxy objects are excluded from the slice and their status
// updated accordingly.
func (p *HTTPProxyProcessor) validHTTPProxies() []*contour_v1.HTTPProxy {
	// ensure that a given fqdn is only referenced in a single HTTPProxy resource
	var valid []*contour_v1.HTTPProxy
	fqdnHTTPProxies := make(map[string][]*contour_v1.HTTPProxy)
	for _, proxy := range p.source.httpproxies {
		if proxy.Spec.VirtualHost == nil {
			valid = append(valid, proxy)
			continue
		}
		fqdn := strings.ToLower(proxy.Spec.VirtualHost.Fqdn)
		fqdnHTTPProxies[fqdn] = append(fqdnHTTPProxies[fqdn], proxy)
	}

	for fqdn, proxies := range fqdnHTTPProxies {
		switch len(proxies) {
		case 1:
			valid = append(valid, proxies[0])
		default:
			// multiple proxies use the same fqdn. mark them as invalid.
			var conflicting []string
			for _, proxy := range proxies {
				conflicting = append(conflicting, proxy.Namespace+"/"+proxy.Name)
			}
			sort.Strings(conflicting) // sort for test stability
			msg := fmt.Sprintf("fqdn %q is used in multiple HTTPProxies: %s", fqdn, strings.Join(conflicting, ", "))
			for _, proxy := range proxies {
				pa, commit := p.dag.StatusCache.ProxyAccessor(proxy)
				pa.Vhost = fqdn
				pa.ConditionFor(status.ValidCondition).AddError(contour_v1.ConditionTypeVirtualHostError,
					"DuplicateVhost",
					msg)
				commit()
			}
		}
	}
	return valid
}

// rootAllowed returns true if the HTTPProxy lives in a permitted root namespace.
func (p *HTTPProxyProcessor) rootAllowed(namespace string) bool {
	if len(p.source.RootNamespaces) == 0 {
		return true
	}
	for _, ns := range p.source.RootNamespaces {
		if ns == namespace {
			return true
		}
	}
	return false
}

func (p *HTTPProxyProcessor) computeVirtualHostAuthorization(auth *contour_v1.AuthorizationServer, validCond *contour_v1.DetailedCondition, httpproxy *contour_v1.HTTPProxy) *ExternalAuthorization {
	ok, ext := validateExternalAuthExtensionService(defaultExtensionRef(auth.ExtensionServiceRef),
		validCond,
		httpproxy,
		p.dag.GetExtensionCluster,
	)
	if !ok {
		return nil
	}

	ok, respTimeout := determineExternalAuthTimeout(auth.ResponseTimeout, validCond, ext)
	if !ok {
		return nil
	}

	globalExternalAuthorization := &ExternalAuthorization{
		AuthorizationService:         ext,
		AuthorizationFailOpen:        auth.FailOpen,
		AuthorizationResponseTimeout: *respTimeout,
	}

	switch auth.ServiceAPIType {
	case contour_v1.AuthorizationGRPCService:
		globalExternalAuthorization.ServiceAPIType = contour_v1.AuthorizationGRPCService
	case contour_v1.AuthorizationHTTPService:
		globalExternalAuthorization.ServiceAPIType = contour_v1.AuthorizationHTTPService

		if auth.HTTPServerSettings != nil {
			globalExternalAuthorization.HTTPPathPrefix = auth.HTTPServerSettings.PathPrefix

			// globalExternalAuthorization.HttpServerURI = auth.HttpServerSettings.ServerURI

			if len(auth.HTTPServerSettings.AllowedAuthorizationHeaders) > 0 {
				if err := ExternalAuthAllowedHeadersValid(auth.HTTPServerSettings.AllowedAuthorizationHeaders); err != nil {
					validCond.AddErrorf(contour_v1.ConditionTypeAuthError, "AuthBadAllowedHeader",
						"allowed authorization headers validation failed: %v", err)

					return nil
				}

				globalExternalAuthorization.HTTPAllowedAuthorizationHeaders = auth.HTTPServerSettings.AllowedAuthorizationHeaders
			}

			if len(auth.HTTPServerSettings.AllowedUpstreamHeaders) > 0 {
				if err := ExternalAuthAllowedHeadersValid(auth.HTTPServerSettings.AllowedUpstreamHeaders); err != nil {
					validCond.AddErrorf(contour_v1.ConditionTypeAuthError, "AuthBadAllowedHeader",
						"allowed upstream headers validation failed: %v", err)

					return nil
				}

				globalExternalAuthorization.HTTPAllowedUpstreamHeaders = auth.HTTPServerSettings.AllowedUpstreamHeaders
			}
		}
	}

	if auth.WithRequestBody != nil {
		maxRequestBytes := defaultMaxRequestBytes
		if auth.WithRequestBody.MaxRequestBytes != 0 {
			maxRequestBytes = auth.WithRequestBody.MaxRequestBytes
		}
		globalExternalAuthorization.AuthorizationServerWithRequestBody = &AuthorizationServerBufferSettings{
			MaxRequestBytes:     maxRequestBytes,
			AllowPartialMessage: auth.WithRequestBody.AllowPartialMessage,
			PackAsBytes:         auth.WithRequestBody.PackAsBytes,
		}
	}
	return globalExternalAuthorization
}

func validateExternalAuthExtensionService(ref contour_v1.ExtensionServiceReference, validCond *contour_v1.DetailedCondition, httpproxy *contour_v1.HTTPProxy, getExtensionCluster func(name string) *ExtensionCluster) (bool, *ExtensionCluster) {
	if ref.APIVersion != contour_v1alpha1.GroupVersion.String() {
		validCond.AddErrorf(contour_v1.ConditionTypeAuthError, "AuthBadResourceVersion",
			"Spec.Virtualhost.Authorization.extensionRef specifies an unsupported resource version %q", ref.APIVersion)
		return false, nil
	}

	// Lookup the extension service reference.
	extensionName := types.NamespacedName{
		Name:      ref.Name,
		Namespace: stringOrDefault(ref.Namespace, httpproxy.Namespace),
	}

	ext := getExtensionCluster(ExtensionClusterName(extensionName))
	if ext == nil {
		validCond.AddErrorf(contour_v1.ConditionTypeAuthError, "ExtensionServiceNotFound",
			"Spec.Virtualhost.Authorization.ServiceRef extension service %q not found", extensionName)
		return false, ext
	}

	return true, ext
}

func determineExternalAuthTimeout(responseTimeout string, validCond *contour_v1.DetailedCondition, ext *ExtensionCluster) (bool, *timeout.Setting) {
	tout, err := timeout.Parse(responseTimeout)
	if err != nil {
		validCond.AddErrorf(contour_v1.ConditionTypeAuthError, "AuthResponseTimeoutInvalid",
			"Spec.Virtualhost.Authorization.ResponseTimeout is invalid: %s", err)
		return false, nil
	}

	if tout.UseDefault() {
		return true, &ext.RouteTimeoutPolicy.ResponseTimeout
	}

	return true, &tout
}

func (p *HTTPProxyProcessor) computeSecureVirtualHostAuthorization(validCond *contour_v1.DetailedCondition, httpproxy *contour_v1.HTTPProxy, svhost *SecureVirtualHost) bool {
	if httpproxy.Spec.VirtualHost.AuthorizationConfigured() && !httpproxy.Spec.VirtualHost.DisableAuthorization() && httpproxy.Spec.VirtualHost.Authorization.ExtensionServiceRef.IsConfigured() {
		authorization := p.computeVirtualHostAuthorization(httpproxy.Spec.VirtualHost.Authorization, validCond, httpproxy)
		if authorization == nil {
			return false
		}

		svhost.ExternalAuthorization = authorization
	} else if p.GlobalExternalAuthorization != nil {
		globalAuthorization := p.computeVirtualHostAuthorization(p.GlobalExternalAuthorization, validCond, httpproxy)
		if globalAuthorization == nil {
			return false
		}

		svhost.ExternalAuthorization = globalAuthorization
	}

	return true
}

func computeVirtualHostRateLimitPolicy(proxy *contour_v1.HTTPProxy, rls *contour_v1alpha1.RateLimitServiceConfig, validCond *contour_v1.DetailedCondition) (*RateLimitPolicy, bool) {
	rlp, err := rateLimitPolicy(proxy.Spec.VirtualHost.RateLimitPolicy)
	if err != nil {
		validCond.AddErrorf(contour_v1.ConditionTypeVirtualHostError, "RateLimitPolicyNotValid",
			"Spec.VirtualHost.RateLimitPolicy is invalid: %s", err)
		return nil, false
	}

	// If HTTPProxy defines its own Global RateLimit Policy, no need to check the default rate limit policy.
	if rlp != nil && rlp.Global != nil {
		return rlp, true
	}

	// Set the global rateLimit policy from the default global rateLimit policy
	// if HTTPProxy is not opted out explicitly, or it defines its own global rate limit policy.
	optedOut := proxy.Spec.VirtualHost.RateLimitPolicy != nil &&
		proxy.Spec.VirtualHost.RateLimitPolicy.Global != nil &&
		proxy.Spec.VirtualHost.RateLimitPolicy.Global.Disabled

	// Attach the default global rate limit policy if the rate limit service defines one.
	if rls != nil && rls.DefaultGlobalRateLimitPolicy != nil && !optedOut {
		if rlp == nil {
			rlp = &RateLimitPolicy{}
		}
		rlp.Global, err = globalRateLimitPolicy(rls.DefaultGlobalRateLimitPolicy)
		if err != nil {
			validCond.AddErrorf(contour_v1.ConditionTypeVirtualHostError, "RateLimitPolicyNotValid",
				"Default Global RateLimit Policy is invalid: %s", err)
			return nil, false
		}
	}

	return rlp, true
}

func (p *HTTPProxyProcessor) GlobalAuthorizationConfigured() bool {
	return p.GlobalExternalAuthorization != nil
}

// GlobalAuthorizationContext returns the authorization policy context (if present).
func (p *HTTPProxyProcessor) GlobalAuthorizationContext() map[string]string {
	if p.GlobalAuthorizationConfigured() && p.GlobalExternalAuthorization.AuthPolicy != nil {
		return p.GlobalExternalAuthorization.AuthPolicy.Context
	}
	return nil
}

func (p *HTTPProxyProcessor) peerValidationContext(validCond *contour_v1.DetailedCondition, httpproxy *contour_v1.HTTPProxy, service contour_v1.Service) *PeerValidationContext {
	caCertNamespacedName := k8s.NamespacedNameFrom(service.UpstreamValidation.CACertificate, k8s.DefaultNamespace(httpproxy.Namespace))
	// we can only validate TLS connections to services that talk TLS
	uv, err := p.source.LookupUpstreamValidation(service.UpstreamValidation, caCertNamespacedName, httpproxy.Namespace)
	if err != nil {
		if _, ok := err.(DelegationNotPermittedError); ok {
			validCond.AddErrorf(contour_v1.ConditionTypeTLSError, "CACertificateNotDelegated",
				"service.UpstreamValidation.CACertificate Secret %q is not configured for certificate delegation", caCertNamespacedName)
		} else {
			validCond.AddErrorf(contour_v1.ConditionTypeServiceError, "TLSUpstreamValidation",
				"Service [%s:%d] TLS upstream validation policy error: %s", service.Name, service.Port, err)
		}
		return nil
	}
	return uv
}

// getSortedHTTPVersions returns and empty slice or ["h2", "http/1.1"] or ["http/1.1"].
// This is done to conform with how envoy expects AlpnProtocols in tlsv3.CommonTlsContext.
func (p *HTTPProxyProcessor) getSortedHTTPVersions(proxy *contour_v1.HTTPProxy) []string {
	proxyHTTPVersions := proxy.Spec.HTTPVersions
	if len(proxyHTTPVersions) == 0 {
		return nil
	}
	for _, httpVersion := range proxyHTTPVersions {
		if httpVersion == "h2" {
			return []string{"h2", "http/1.1"}
		}
	}
	return []string{"http/1.1"}
}

// expandPrefixMatches adds new Routes to account for the difference
// between prefix replacement when matching on '/foo' and '/foo/'.
//
// The table below shows the behavior of Envoy prefix rewrite. If we
// match on only `/foo` or `/foo/`, then the unwanted rewrites marked
// with X can result. This means that we need to generate separate
// prefix matches (and replacements) for these cases.
//
// | Matching Prefix | Replacement | Client Path | Rewritten Path |
// |-----------------|-------------|-------------|----------------|
// | `/foo`          | `/bar`      | `/foosball` |   `/barsball`  |
// | `/foo`          | `/`         | `/foo/v1`   | X `//v1`       |
// | `/foo/`         | `/bar`      | `/foo/type` | X `/bartype`   |
// | `/foo`          | `/bar/`     | `/foosball` | X `/bar/sball` |
// | `/foo/`         | `/bar/`     | `/foo/type` |   `/bar/type`  |
func expandPrefixMatches(routes []*Route) []*Route {
	prefixedRoutes := map[string][]*Route{}

	expandedRoutes := []*Route{}

	// First, we group the Routes by their slash-consistent prefix match condition.
	for _, r := range routes {
		// If there is no path prefix, we won't do any expansion, so skip it.
		if !r.HasPathPrefix() {
			expandedRoutes = append(expandedRoutes, r)
		}

		// Skip for exact path match conditions
		if r.HasPathExact() || r.HasPathRegex() {
			continue
		}

		routingPrefix := r.PathMatchCondition.(*PrefixMatchCondition).Prefix

		if routingPrefix != "/" {
			routingPrefix = strings.TrimRight(routingPrefix, "/")
		}

		prefixedRoutes[routingPrefix] = append(prefixedRoutes[routingPrefix], r)
	}

	for prefix, routes := range prefixedRoutes {
		// Propagate the Routes into the expanded set. Since
		// we have a slice of pointers, we can propagate here
		// prior to any Route modifications.
		expandedRoutes = append(expandedRoutes, routes...)

		switch len(routes) {
		case 1:
			// Don't modify if we are not doing a replacement.
			if routes[0].PathRewritePolicy == nil {
				continue
			}

			routingPrefix := routes[0].PathMatchCondition.(*PrefixMatchCondition).Prefix

			// There's no alternate forms for '/' :)
			if routingPrefix == "/" {
				continue
			}

			// Shallow copy the Route. TODO(jpeach) deep copying would be more robust.
			newRoute := *routes[0]

			// Now, make the original route handle '/foo' and the new route handle '/foo'.
			routes[0].PathRewritePolicy.PrefixRewrite = strings.TrimRight(routes[0].PathRewritePolicy.PrefixRewrite, "/")
			routes[0].PathMatchCondition = &PrefixMatchCondition{Prefix: prefix}

			// Replace the entire PathRewritePolicy since we didn't deep-copy the route.
			newRoute.PathRewritePolicy = &PathRewritePolicy{
				PrefixRewrite: routes[0].PathRewritePolicy.PrefixRewrite + "/",
			}
			newRoute.PathMatchCondition = &PrefixMatchCondition{Prefix: prefix + "/"}

			// Since we trimmed trailing '/', it's possible that
			// we made the replacement empty. There's no such
			// thing as an empty rewrite; it's the same as
			// rewriting to '/'.
			if len(routes[0].PathRewritePolicy.PrefixRewrite) == 0 {
				routes[0].PathRewritePolicy.PrefixRewrite = "/"
			}

			expandedRoutes = append(expandedRoutes, &newRoute)
		case 2:
			// This group routes on both '/foo' and
			// '/foo/' so we can't add any implicit prefix
			// matches. This is why we didn't filter out
			// routes that don't have replacements earlier.
			continue
		default:
			// This can't happen unless there are routes
			// with duplicate prefix paths.
		}

	}

	return expandedRoutes
}

func getProtocol(service contour_v1.Service, s *Service) (string, error) {
	// Determine the protocol to use to speak to this Cluster.
	var protocol string
	if service.Protocol != nil {
		protocol = *service.Protocol
		switch protocol {
		case "h2c", "h2", "tls":
		default:
			return "", fmt.Errorf("unsupported protocol: %v", protocol)
		}
	} else {
		protocol = s.Protocol
	}

	return protocol, nil
}

// determineSNI decides what the SNI should be on the request. It is configured via RequestHeadersPolicy.Host key.
// Policies set on service are used before policies set on a route. Otherwise the value of the externalService
// is used if the route is configured to proxy to an externalService type.
func determineSNI(routeRequestHeaders, clusterRequestHeaders *HeadersPolicy, service *Service) string {
	// Service RequestHeadersPolicy take precedence
	if clusterRequestHeaders != nil {
		if clusterRequestHeaders.HostRewrite != "" {
			return clusterRequestHeaders.HostRewrite
		}
	}

	// Route RequestHeadersPolicy take precedence after service
	if routeRequestHeaders != nil {
		if routeRequestHeaders.HostRewrite != "" {
			return routeRequestHeaders.HostRewrite
		}
	}

	return service.ExternalName
}

func toCORSPolicy(policy *contour_v1.CORSPolicy) (*CORSPolicy, error) {
	if policy == nil {
		return nil, nil
	}

	if len(policy.AllowOrigin) == 0 {
		return nil, errors.New("invalid allowed origin configuration with length 0")
	}
	allowOriginMatches := make([]CORSAllowOriginMatch, 0, len(policy.AllowOrigin))
	toAllowOriginMatch := func(ao string) (CORSAllowOriginMatch, error) {
		// Short circuit common case.
		if ao == "*" {
			return CORSAllowOriginMatch{
				Type:  CORSAllowOriginMatchExact,
				Value: ao,
			}, nil
		}

		// Parse allowed origin as URL, to check if it should be an
		// exact match or regex.
		// If there is a parsing error, or we don't have a properly
		// formatted exact Origin header, then try to parse as a regex.
		// Exact Origin headers should only be allowed as scheme://host[:port]
		// See: https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Origin
		parsedURL, parseURLErr := url.Parse(ao)
		if parseURLErr != nil ||
			parsedURL.Scheme == "" || parsedURL.Host == "" ||
			parsedURL.Scheme+"://"+parsedURL.Host != ao {
			_, regexErr := regexp.Compile(ao)
			if regexErr != nil {
				return CORSAllowOriginMatch{}, errors.New("allowed origin is invalid exact match and invalid regex match")
			}
			return CORSAllowOriginMatch{
				Type:  CORSAllowOriginMatchRegex,
				Value: ao,
			}, nil
		}

		return CORSAllowOriginMatch{
			Type:  CORSAllowOriginMatchExact,
			Value: ao,
		}, nil
	}
	for _, ao := range policy.AllowOrigin {
		match, err := toAllowOriginMatch(ao)
		if err != nil {
			return nil, fmt.Errorf("invalid allowed origin %q: %w", ao, err)
		}
		allowOriginMatches = append(allowOriginMatches, match)
	}

	if len(policy.AllowMethods) == 0 {
		return nil, errors.New("invalid allowed methods configuration with length 0")
	}

	maxAge, err := timeout.ParseMaxAge(policy.MaxAge)
	if err != nil {
		return nil, err
	}
	if maxAge.Duration().Seconds() < 0 {
		return nil, fmt.Errorf("invalid max age value %q", policy.MaxAge)
	}
	return &CORSPolicy{
		AllowCredentials:    policy.AllowCredentials,
		AllowHeaders:        toStringSlice(policy.AllowHeaders),
		AllowMethods:        toStringSlice(policy.AllowMethods),
		AllowOrigin:         allowOriginMatches,
		ExposeHeaders:       toStringSlice(policy.ExposeHeaders),
		MaxAge:              maxAge,
		AllowPrivateNetwork: policy.AllowPrivateNetwork,
	}, nil
}

func toStringSlice(hvs []contour_v1.CORSHeaderValue) []string {
	s := make([]string, len(hvs))
	for i, v := range hvs {
		s[i] = string(v)
	}
	return s
}

// matchConditionAggregate is used to compare collections of match conditions
type matchConditionAggregate struct {
	headerConds     []HeaderMatchCondition
	queryParamConds []QueryParamMatchCondition
}

func includeMatchConditionsIdentical(includeConds []contour_v1.MatchCondition, seenConds map[string][]matchConditionAggregate) bool {
	pathPrefix := ""

	switch pathPrefixRef := mergePathMatchConditions(includeConds).(type) {
	case *PrefixMatchCondition:
		pathPrefix = pathPrefixRef.Prefix
	default:
		// This can never happen because include match conditions only have prefix match conditions.
		// Validations before this step ensure this, so it is safe to mark this validation failed
		// for anything else except a prefix condition.
		return true
	}

	includeHeaderConds := mergeHeaderMatchConditions(includeConds)
	includeQueryParamConds := mergeQueryParamMatchConditions(includeConds)

	// Note: this is stop-gap that we intend to change.
	// This means that an empty set of conditions or a lone path prefix match on "/"
	// are not considered duplicates. Previously, validation of
	// includes was implemented in such a way that users could be relying on this
	// behavior to set up their include tree.
	// It is unlikely that there is much usage of duplicate non-default include
	// conditions, so we think this special case is safe.
	if pathPrefix == "/" && len(includeHeaderConds) == 0 && len(includeQueryParamConds) == 0 {
		return false
	}

	// Sort header conditions so we can compare them to another slice.
	sort.SliceStable(includeHeaderConds, func(i, j int) bool {
		if includeHeaderConds[i].MatchType != includeHeaderConds[j].MatchType {
			return includeHeaderConds[i].MatchType < includeHeaderConds[j].MatchType
		}
		if includeHeaderConds[i].IgnoreCase != includeHeaderConds[j].IgnoreCase {
			return includeHeaderConds[i].IgnoreCase
		}
		if includeHeaderConds[i].Invert != includeHeaderConds[j].Invert {
			return includeHeaderConds[i].Invert
		}
		if includeHeaderConds[i].TreatMissingAsEmpty != includeHeaderConds[j].TreatMissingAsEmpty {
			return includeHeaderConds[i].TreatMissingAsEmpty
		}
		if includeHeaderConds[i].Name != includeHeaderConds[j].Name {
			return includeHeaderConds[i].Name < includeHeaderConds[j].Name
		}
		if includeHeaderConds[i].Value != includeHeaderConds[j].Value {
			return includeHeaderConds[i].Value < includeHeaderConds[j].Value
		}
		return false
	})

	// Sort query conditions so we can compare them to another slice.
	sort.SliceStable(includeQueryParamConds, func(i, j int) bool {
		if includeQueryParamConds[i].MatchType != includeQueryParamConds[j].MatchType {
			return includeQueryParamConds[i].MatchType < includeQueryParamConds[j].MatchType
		}
		if includeQueryParamConds[i].IgnoreCase != includeQueryParamConds[j].IgnoreCase {
			return includeQueryParamConds[i].IgnoreCase
		}
		if includeQueryParamConds[i].Name != includeQueryParamConds[j].Name {
			return includeQueryParamConds[i].Name < includeQueryParamConds[j].Name
		}
		if includeQueryParamConds[i].Value != includeQueryParamConds[j].Value {
			return includeQueryParamConds[i].Value < includeQueryParamConds[j].Value
		}
		return false
	})

	// Check if we have seen this path before.
	// If so, get all the collections of header and query params we
	// have seen with it.
	condAggregates, pathSeen := seenConds[pathPrefix]
	if !pathSeen {
		seenConds[pathPrefix] = []matchConditionAggregate{{
			headerConds:     includeHeaderConds,
			queryParamConds: includeQueryParamConds,
		}}
		return false
	}

	for _, ag := range condAggregates {
		// Quick check to see if lengths of either header or query param
		// condition lists are mismatched.
		// If so, we can skip the rest of the checks.
		if len(ag.headerConds) != len(includeHeaderConds) ||
			len(ag.queryParamConds) != len(includeQueryParamConds) {
			continue
		}

		// Now compare (sorted) header conditions element-by-element.
		// If any mismatch, we can skip the rest of the checks.
		headerCondsIdentical := true
		for i := range ag.headerConds {
			if ag.headerConds[i] != includeHeaderConds[i] {
				headerCondsIdentical = false
				break
			}
		}
		if !headerCondsIdentical {
			continue
		}

		// Now compare (sorted) query param conditions element-by-element.
		// If any mismatch, we can return early.
		queryParamCondsIdentical := true
		for i := range ag.queryParamConds {
			if ag.queryParamConds[i] != includeQueryParamConds[i] {
				queryParamCondsIdentical = false
				break
			}
		}
		if !queryParamCondsIdentical {
			continue
		}
		// If we get here, all header and query param conditions
		// must be equal.
		return true
	}

	// Save the seen path and header/query conditions.
	seenConds[pathPrefix] = append(seenConds[pathPrefix], matchConditionAggregate{
		headerConds:     includeHeaderConds,
		queryParamConds: includeQueryParamConds,
	})

	return false
}

// isBlank indicates if a string contains nothing but blank characters.
func isBlank(s string) bool {
	return len(strings.TrimSpace(s)) == 0
}

// routeEnforceTLS determines if the route should redirect the user to a secure TLS listener
func routeEnforceTLS(enforceTLS, permitInsecure bool) bool {
	return enforceTLS && !permitInsecure
}

func directResponse(statusCode uint32, body string) *DirectResponse {
	return &DirectResponse{
		StatusCode: statusCode,
		Body:       body,
	}
}

// routeActionCountValid  only one of route.services, route.requestRedirectPolicy, or route.directResponsePolicy can be specified
func routeActionCountValid(route contour_v1.Route) error {
	var routeActionCount int
	if len(route.Services) > 0 {
		routeActionCount++
	}

	if route.RequestRedirectPolicy != nil {
		routeActionCount++
	}

	if route.DirectResponsePolicy != nil {
		routeActionCount++
	}

	if routeActionCount != 1 {
		return errors.New("must set exactly one of route.services or route.requestRedirectPolicy or route.directResponsePolicy")
	}
	return nil
}

// redirectRoutePolicy builds a *dag.Redirect for the supplied redirect policy.
func redirectRoutePolicy(redirect *contour_v1.HTTPRequestRedirectPolicy) (*Redirect, error) {
	if redirect == nil {
		return nil, nil
	}

	var hostname string
	if redirect.Hostname != nil {
		hostname = *redirect.Hostname
	}

	var portNumber uint32
	if redirect.Port != nil {
		portNumber = uint32(*redirect.Port) //nolint:gosec // disable G115
	}

	var scheme string
	if redirect.Scheme != nil {
		scheme = *redirect.Scheme
	}

	var statusCode int
	if redirect.StatusCode != nil {
		statusCode = *redirect.StatusCode
	}

	if redirect.Path != nil && redirect.Prefix != nil {
		return nil, fmt.Errorf("cannot specify both redirect path and redirect prefix")
	}

	var pathRewritePolicy *PathRewritePolicy
	if redirect.Path != nil {
		if pathRewritePolicy == nil {
			pathRewritePolicy = &PathRewritePolicy{}
		}
		pathRewritePolicy.FullPathRewrite = *redirect.Path
	}
	if redirect.Prefix != nil {
		if pathRewritePolicy == nil {
			pathRewritePolicy = &PathRewritePolicy{}
		}
		pathRewritePolicy.PrefixRewrite = *redirect.Prefix
	}

	return &Redirect{
		Hostname:          hostname,
		Scheme:            scheme,
		PortNumber:        portNumber,
		StatusCode:        statusCode,
		PathRewritePolicy: pathRewritePolicy,
	}, nil
}

func directResponsePolicy(direct *contour_v1.HTTPDirectResponsePolicy) *DirectResponse {
	if direct == nil {
		return nil
	}

	return directResponse(uint32(direct.StatusCode), direct.Body) //nolint:gosec // disable G115
}

func internalRedirectPolicy(internal *contour_v1.HTTPInternalRedirectPolicy) *InternalRedirectPolicy {
	if internal == nil {
		return nil
	}

	redirectResponseCodes := make([]uint32, len(internal.RedirectResponseCodes))
	for i, responseCode := range internal.RedirectResponseCodes {
		redirectResponseCodes[i] = uint32(responseCode)
	}

	policy := &InternalRedirectPolicy{
		MaxInternalRedirects:      internal.MaxInternalRedirects,
		RedirectResponseCodes:     redirectResponseCodes,
		DenyRepeatedRouteRedirect: internal.DenyRepeatedRouteRedirect,
	}
	switch internal.AllowCrossSchemeRedirect {
	case "Always":
		policy.AllowCrossSchemeRedirect = InternalRedirectCrossSchemeAlways
	case "SafeOnly":
		policy.AllowCrossSchemeRedirect = InternalRedirectCrossSchemeSafeOnly
	default:
		// Value already checked by the generated validator, assume this is Never.
		policy.AllowCrossSchemeRedirect = InternalRedirectCrossSchemeNever
	}
	return policy
}

func slowStartConfig(slowStart *contour_v1.SlowStartPolicy) (*SlowStartConfig, error) {
	window, err := time.ParseDuration(slowStart.Window)
	if err != nil {
		return nil, fmt.Errorf("error parsing window: %s", err)
	}

	aggression := float64(1.0)
	if slowStart.Aggression != "" {
		aggression, err = strconv.ParseFloat(slowStart.Aggression, 64)
		if err != nil {
			return nil, fmt.Errorf("error parsing aggression: \"%s\" is not a decimal number", slowStart.Aggression)
		}
	}

	return &SlowStartConfig{
		Window:           window,
		Aggression:       aggression,
		MinWeightPercent: slowStart.MinimumWeightPercent,
	}, nil
}

func rateLimitPerRoute(in *contour_v1.RateLimitPolicy) *RateLimitPerRoute {
	// Ignore the virtual host global rate limit policy if disabled is true
	if in != nil && in.Global != nil && in.Global.Disabled {
		return &RateLimitPerRoute{
			VhRateLimits: VhRateLimitsIgnore,
		}
	}

	return nil
}
