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

//go:build e2e

package httpproxy

import (
	"context"
	"fmt"
	"strings"
	"testing"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	certmanagermetav1 "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/stretchr/testify/require"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/pkg/config"
	"github.com/projectcontour/contour/test/e2e"
)

var f = e2e.NewFramework(false)

func TestHTTPProxy(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "HTTPProxy tests")
}

var _ = BeforeSuite(func() {
	require.NoError(f.T(), f.Deployment.EnsureResourcesForLocalContour())
})

var _ = AfterSuite(func() {
	// Delete resources individually instead of deleting the entire contour
	// namespace as a performance optimization, because deleting non-empty
	// namespaces can take up to a couple minutes to complete.
	require.NoError(f.T(), f.Deployment.DeleteResourcesForLocalContour())
	gexec.CleanupBuildArtifacts()
})

// Contains specs that test that kubebuilder API validations
// work as expected, and do not require a Contour instance to
// be running.
var _ = Describe("HTTPProxy API validation", func() {
	f.NamespacedTest("httpproxy-required-field-validation", testRequiredFieldValidation)

	f.NamespacedTest("httpproxy-cel-validation", testCELValidation)

	f.NamespacedTest("httpproxy-invalid-wildcard-fqdn", testWildcardFQDN)

	f.NamespacedTest("invalid-cookie-rewrite-fields", testInvalidCookieRewriteFields)
})

var _ = Describe("HTTPProxy", func() {
	var (
		contourCmd            *gexec.Session
		contourConfig         *config.Parameters
		contourConfiguration  *contour_v1alpha1.ContourConfiguration
		contourConfigFile     string
		additionalContourArgs []string
	)

	BeforeEach(func() {
		// Contour config file contents, can be modified in nested
		// BeforeEach.
		contourConfig = e2e.DefaultContourConfigFileParams()

		// Contour configuration crd, can be modified in nested
		// BeforeEach.
		contourConfiguration = e2e.DefaultContourConfiguration()

		// Default contour serve command line arguments can be appended to in
		// nested BeforeEach.
		additionalContourArgs = []string{}
	})

	// JustBeforeEach is called after each of the nested BeforeEach are
	// called, so it is a final setup step before running a test.
	// A nested BeforeEach may have modified Contour config, so we wait
	// until here to start Contour.
	JustBeforeEach(func() {
		var err error
		contourCmd, contourConfigFile, err = f.Deployment.StartLocalContour(contourConfig, contourConfiguration, additionalContourArgs...)
		require.NoError(f.T(), err)

		// Wait for Envoy to be healthy.
		require.NoError(f.T(), f.Deployment.WaitForEnvoyUpdated())
	})

	AfterEach(func() {
		require.NoError(f.T(), f.Deployment.StopLocalContour(contourCmd, contourConfigFile))
	})
	f.NamespacedTest("httpproxy-direct-response-policy", testDirectResponseRule)

	f.NamespacedTest("httpproxy-request-redirect-policy-nosvc", testRequestRedirectRuleNoService)
	f.NamespacedTest("httpproxy-request-redirect-policy-invalid", testRequestRedirectRuleInvalid)

	f.NamespacedTest("httpproxy-internal-redirect-validation", testInternalRedirectValidation)
	f.NamespacedTest("httpproxy-internal-redirect-policy", func(namespace string) {
		Context("with ExternalName Services enabled", func() {
			BeforeEach(func() {
				contourConfig.EnableExternalNameService = true
				contourConfiguration.Spec.EnableExternalNameService = ptr.To(true)
			})
			testInternalRedirectPolicy(namespace)
		})
	})

	f.NamespacedTest("httpproxy-header-condition-match", testHeaderConditionMatch)

	f.NamespacedTest("httpproxy-query-parameter-condition-match", testQueryParameterConditionMatch)

	f.NamespacedTest("httpproxy-query-parameter-condition-multiple", testQueryParameterConditionMultiple)

	f.NamespacedTest("httpproxy-path-condition-match", testPathConditionMatch)

	f.NamespacedTest("httpproxy-path-prefix-rewrite", testPathPrefixRewrite)

	f.NamespacedTest("httpproxy-https-sni-enforcement", testHTTPSSNIEnforcement)

	f.NamespacedTest("httpproxy-pod-restart", testPodRestart)

	Context("disableMergeSlashes option", func() {
		Context("default value of false", func() {
			f.NamespacedTest("httpproxy-enable-merge-slashes", testDisableMergeSlashes(false))
		})

		Context("set to true", func() {
			BeforeEach(func() {
				contourConfig.DisableMergeSlashes = true
				contourConfiguration.Spec.Envoy.Listener.DisableMergeSlashes = ptr.To(true)
			})

			f.NamespacedTest("httpproxy-disable-merge-slashes", testDisableMergeSlashes(true))
		})
	})

	f.NamespacedTest("httpproxy-client-cert-auth", testClientCertAuth)

	f.NamespacedTest("httpproxy-tcproute-https-termination", testTCPRouteHTTPSTermination)

	f.NamespacedTest("httpproxy-https-misdirected-request", testHTTPSMisdirectedRequest)

	f.NamespacedTest("httpproxy-include-prefix-condition", testIncludePrefixCondition)

	f.NamespacedTest("httpproxy-include-exact-condition", testIncludeExactCondition)

	f.NamespacedTest("httpproxy-exact-path-condition-app", testExactPathCondition)

	f.NamespacedTest("httpproxy-regex-condition", testRegexPathCondition)

	f.NamespacedTest("httpproxy-include-regex-condition", testIncludeRegexCondition)

	f.NamespacedTest("httpproxy-retry-policy-validation", testRetryPolicyValidation)

	f.NamespacedTest("httpproxy-wildcard-subdomain-fqdn", testWildcardSubdomainFQDN)

	f.NamespacedTest("httpproxy-ingress-wildcard-override", testIngressWildcardSubdomainFQDN)

	f.NamespacedTest("httpproxy-https-fallback-certificate", func(namespace string) {
		Context("with fallback certificate", func() {
			BeforeEach(func() {
				contourConfig.TLS = config.TLSParameters{
					FallbackCertificate: config.NamespacedName{
						Name:      "fallback-cert",
						Namespace: namespace,
					},
				}
				contourConfiguration.Spec.HTTPProxy.FallbackCertificate = &contour_v1alpha1.NamespacedName{
					Name:      "fallback-cert",
					Namespace: namespace,
				}

				f.Certs.CreateSelfSignedCert(namespace, "fallback-cert", "fallback-cert", "fallback.projectcontour.io")
			})

			testHTTPSFallbackCertificate(namespace)
		})
	})

	f.NamespacedTest("httpproxy-backend-tls", func(namespace string) {
		Context("with backend tls", func() {
			BeforeEach(func() {
				// Top level issuer.
				selfSignedIssuer := &certmanagerv1.Issuer{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace: namespace,
						Name:      "selfsigned",
					},
					Spec: certmanagerv1.IssuerSpec{
						IssuerConfig: certmanagerv1.IssuerConfig{
							SelfSigned: &certmanagerv1.SelfSignedIssuer{},
						},
					},
				}
				require.NoError(f.T(), f.Client.Create(context.TODO(), selfSignedIssuer))

				// CA to sign backend certs with.
				caCertificate := &certmanagerv1.Certificate{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace: namespace,
						Name:      "ca-cert",
					},
					Spec: certmanagerv1.CertificateSpec{
						IsCA: true,
						Usages: []certmanagerv1.KeyUsage{
							certmanagerv1.UsageSigning,
							certmanagerv1.UsageCertSign,
						},
						CommonName: "ca-cert",
						SecretName: "ca-cert",
						IssuerRef: certmanagermetav1.ObjectReference{
							Name: "selfsigned",
						},
					},
				}
				require.NoError(f.T(), f.Client.Create(context.TODO(), caCertificate))

				// Issuer based on CA to generate new certs with.
				basedOnCAIssuer := &certmanagerv1.Issuer{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace: namespace,
						Name:      "ca-issuer",
					},
					Spec: certmanagerv1.IssuerSpec{
						IssuerConfig: certmanagerv1.IssuerConfig{
							CA: &certmanagerv1.CAIssuer{
								SecretName: "ca-cert",
							},
						},
					},
				}
				require.NoError(f.T(), f.Client.Create(context.TODO(), basedOnCAIssuer))

				// Backend client cert, can use for upstream validation as well.
				backendClientCert := &certmanagerv1.Certificate{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace: namespace,
						Name:      "backend-client-cert",
					},
					Spec: certmanagerv1.CertificateSpec{
						Usages: []certmanagerv1.KeyUsage{
							certmanagerv1.UsageClientAuth,
						},
						CommonName: "client",
						SecretName: "backend-client-cert",
						IssuerRef: certmanagermetav1.ObjectReference{
							Name: "ca-issuer",
						},
					},
				}
				require.NoError(f.T(), f.Client.Create(context.TODO(), backendClientCert))

				contourConfig.TLS = config.TLSParameters{
					ClientCertificate: config.NamespacedName{
						Namespace: namespace,
						Name:      "backend-client-cert",
					},
				}

				contourConfiguration.Spec.Envoy.ClientCertificate = &contour_v1alpha1.NamespacedName{
					Name:      "backend-client-cert",
					Namespace: namespace,
				}
			})

			testBackendTLS(namespace)
		})
	})

	f.NamespacedTest("httpproxy-backend-tls-version", func(namespace string) {
		BeforeEach(func() {
			// Top level issuer.
			selfSignedIssuer := &certmanagerv1.Issuer{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: namespace,
					Name:      "selfsigned",
				},
				Spec: certmanagerv1.IssuerSpec{
					IssuerConfig: certmanagerv1.IssuerConfig{
						SelfSigned: &certmanagerv1.SelfSignedIssuer{},
					},
				},
			}
			require.NoError(f.T(), f.Client.Create(context.TODO(), selfSignedIssuer))

			// CA to sign backend certs with.
			caCertificate := &certmanagerv1.Certificate{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: namespace,
					Name:      "ca-cert",
				},
				Spec: certmanagerv1.CertificateSpec{
					IsCA: true,
					Usages: []certmanagerv1.KeyUsage{
						certmanagerv1.UsageSigning,
						certmanagerv1.UsageCertSign,
					},
					CommonName: "ca-cert",
					SecretName: "ca-cert",
					IssuerRef: certmanagermetav1.ObjectReference{
						Name: "selfsigned",
					},
				},
			}
			require.NoError(f.T(), f.Client.Create(context.TODO(), caCertificate))

			// Issuer based on CA to generate new certs with.
			basedOnCAIssuer := &certmanagerv1.Issuer{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: namespace,
					Name:      "ca-issuer",
				},
				Spec: certmanagerv1.IssuerSpec{
					IssuerConfig: certmanagerv1.IssuerConfig{
						CA: &certmanagerv1.CAIssuer{
							SecretName: "ca-cert",
						},
					},
				},
			}
			require.NoError(f.T(), f.Client.Create(context.TODO(), basedOnCAIssuer))

			// Backend client cert, can use for upstream validation as well.
			backendClientCert := &certmanagerv1.Certificate{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: namespace,
					Name:      "backend-client-cert",
				},
				Spec: certmanagerv1.CertificateSpec{
					Usages: []certmanagerv1.KeyUsage{
						certmanagerv1.UsageClientAuth,
					},
					CommonName: "client",
					SecretName: "backend-client-cert",
					IssuerRef: certmanagermetav1.ObjectReference{
						Name: "ca-issuer",
					},
				},
			}
			require.NoError(f.T(), f.Client.Create(context.TODO(), backendClientCert))

			contourConfig.TLS = config.TLSParameters{
				ClientCertificate: config.NamespacedName{
					Namespace: namespace,
					Name:      "backend-client-cert",
				},
			}

			contourConfiguration.Spec.Envoy.ClientCertificate = &contour_v1alpha1.NamespacedName{
				Name:      "backend-client-cert",
				Namespace: namespace,
			}
		})
		Context("with backend tls version configured", func() {
			protocolVersion := "1.3"
			expectedProtocolVersion := "TLSv1.3"

			BeforeEach(func() {
				contourConfig.Cluster.UpstreamTLS.MinimumProtocolVersion = protocolVersion

				// need to set this because it isn't set in the default config
				contourConfig.Cluster.DNSLookupFamily = "auto"

				contourConfiguration.Spec.Envoy.Cluster.UpstreamTLS = &contour_v1alpha1.EnvoyTLS{
					MinimumProtocolVersion: protocolVersion,
				}
			})

			testBackendTLSProtocolVersion(namespace, expectedProtocolVersion)
		})
	})

	f.NamespacedTest("httpproxy-default-compression", func(namespace string) {
		testEnvoyDisableCompression(namespace, "gzip", "gzip", false)
	})

	f.NamespacedTest("httpproxy-disable-compression", func(namespace string) {
		Context("with compression disabled", func() {
			BeforeEach(func() {
				contourConfig.Compression = config.CompressionParameters{
					Algorithm: config.CompressionDisabled,
				}
				contourConfiguration.Spec.Envoy.Listener.Compression = &contour_v1alpha1.EnvoyCompression{
					Algorithm: contour_v1alpha1.DisabledCompression,
				}
			})
			testEnvoyDisableCompression(namespace, "gzip", "gzip", true)
		})
	})

	f.NamespacedTest("httpproxy-brotli-compression", func(namespace string) {
		Context("with brotli compression", func() {
			BeforeEach(func() {
				contourConfig.Compression = config.CompressionParameters{
					Algorithm: config.CompressionBrotli,
				}
				contourConfiguration.Spec.Envoy.Listener.Compression = &contour_v1alpha1.EnvoyCompression{
					Algorithm: contour_v1alpha1.BrotliCompression,
				}
			})
			testEnvoyDisableCompression(namespace, "br", "br", false)
		})
	})

	f.NamespacedTest("httpproxy-zstd-compression", func(namespace string) {
		Context("with zstd compression", func() {
			BeforeEach(func() {
				contourConfig.Compression = config.CompressionParameters{
					Algorithm: config.CompressionZstd,
				}
				contourConfiguration.Spec.Envoy.Listener.Compression = &contour_v1alpha1.EnvoyCompression{
					Algorithm: contour_v1alpha1.ZstdCompression,
				}
			})
			testEnvoyDisableCompression(namespace, "zstd", "zstd", false)
		})
	})

	f.NamespacedTest("httpproxy-external-auth", testExternalAuth)

	f.NamespacedTest("httpproxy-http-health-checks", testHTTPHealthChecks)

	f.NamespacedTest("httpproxy-dynamic-headers", testDynamicHeaders)

	f.NamespacedTest("httpproxy-host-header-rewrite-literal", testHostRewriteLiteral)

	f.NamespacedTest("httpproxy-host-header-rewrite-header", testHostRewriteHeaderHTTPService)

	f.NamespacedTest("httpproxy-host-header-rewrite-header-https", testHostRewriteHeaderHTTPSService)

	f.NamespacedTest("httpproxy-host-header-rewrite-header-externalname-service", func(namespace string) {
		Context("with ExternalName Services enabled", func() {
			BeforeEach(func() {
				contourConfig.EnableExternalNameService = true
				contourConfiguration.Spec.EnableExternalNameService = ptr.To(true)
			})
			testHostRewriteHeaderExternalNameService(namespace)
		})
	})

	f.NamespacedTest("httpproxy-ip-filters", func(namespace string) {
		// ip filter tests rely on the ability to forge x-forwarded-for
		Context("with trusted xff hops", func() {
			BeforeEach(func() {
				contourConfig.Network.XffNumTrustedHops = 1
				contourConfiguration.Spec.Envoy.Network.XffNumTrustedHops = ptr.To(uint32(1))
			})

			testIPFilterPolicy(namespace)
		})
	})

	f.NamespacedTest("httpproxy-multiple-ingress-classes-field", func(namespace string) {
		Context("with more than one ingress ClassName set", func() {
			BeforeEach(func() {
				additionalContourArgs = []string{
					"--ingress-class-name=contour,team1",
				}
				contourConfiguration.Spec.Ingress = &contour_v1alpha1.IngressConfig{
					ClassNames: []string{"contour", "team1"},
				}
			})
			testMultipleIngressClassesField(namespace)
		})
	})
	f.NamespacedTest("httpproxy-multiple-ingress-classes-annotation", func(namespace string) {
		Context("with more than one ingress ClassName set", func() {
			BeforeEach(func() {
				additionalContourArgs = []string{
					"--ingress-class-name=contour,team1",
				}
				contourConfiguration.Spec.Ingress = &contour_v1alpha1.IngressConfig{
					ClassNames: []string{"contour", "team1"},
				}
			})
			testMultipleIngressClassesAnnotation(namespace)
		})
	})

	f.NamespacedTest("httpproxy-external-name-service-insecure", func(namespace string) {
		Context("with ExternalName Services enabled", func() {
			BeforeEach(func() {
				contourConfig.EnableExternalNameService = true
				contourConfiguration.Spec.EnableExternalNameService = ptr.To(true)
			})
			testExternalNameServiceInsecure(namespace)
		})
	})

	f.NamespacedTest("httpproxy-external-name-service-tls", func(namespace string) {
		Context("with ExternalName Services enabled", func() {
			BeforeEach(func() {
				contourConfig.EnableExternalNameService = true
				contourConfiguration.Spec.EnableExternalNameService = ptr.To(true)
			})
			testExternalNameServiceTLS(namespace)
		})
	})

	f.NamespacedTest("httpproxy-external-name-service-localhost", func(namespace string) {
		Context("with ExternalName Services enabled", func() {
			BeforeEach(func() {
				contourConfig.EnableExternalNameService = true
				contourConfiguration.Spec.EnableExternalNameService = ptr.To(true)
			})
			testExternalNameServiceLocalhostInvalid(namespace)
		})
	})
	f.NamespacedTest("httpproxy-local-rate-limiting-vhost", testLocalRateLimitingVirtualHost)

	f.NamespacedTest("httpproxy-local-rate-limiting-route", testLocalRateLimitingRoute)

	Context("global rate limiting", func() {
		withRateLimitService := func(body e2e.NamespacedTestBody) e2e.NamespacedTestBody {
			return func(namespace string) {
				Context("with rate limit service", func() {
					BeforeEach(func() {
						contourConfig.RateLimitService = config.RateLimitService{
							ExtensionService: fmt.Sprintf("%s/%s", namespace, f.Deployment.RateLimitExtensionService.Name),
							Domain:           "contour",
							FailOpen:         false,
						}
						contourConfiguration.Spec.RateLimitService = &contour_v1alpha1.RateLimitServiceConfig{
							ExtensionService: contour_v1alpha1.NamespacedName{
								Name:      f.Deployment.RateLimitExtensionService.Name,
								Namespace: namespace,
							},
							Domain:                  "contour",
							FailOpen:                ptr.To(false),
							EnableXRateLimitHeaders: ptr.To(false),
						}
						require.NoError(f.T(),
							f.Deployment.EnsureRateLimitResources(
								namespace,
								`
domain: contour
descriptors:
  - key: generic_key
    value: vhostlimit
    rate_limit:
      unit: hour
      requests_per_unit: 1
  - key: route_limit_key
    value: routelimit
    rate_limit:
      unit: hour
      requests_per_unit: 1
  - key: generic_key
    value: tlsvhostlimit
    rate_limit:
      unit: hour
      requests_per_unit: 1
  - key: generic_key
    value: tlsroutelimit
    rate_limit:
      unit: hour
      requests_per_unit: 1
  - key: generic_key
    value: randomvalue
    rate_limit:
      unit: hour
      requests_per_unit: 1`))
					})

					body(namespace)
				})
			}
		}

		f.NamespacedTest("httpproxy-global-rate-limiting-vhost-non-tls", withRateLimitService(testGlobalRateLimitingVirtualHostNonTLS))

		f.NamespacedTest("httpproxy-global-rate-limiting-route-non-tls", withRateLimitService(testGlobalRateLimitingRouteNonTLS))

		f.NamespacedTest("httpproxy-global-rate-limiting-vhost-tls", withRateLimitService(testGlobalRateLimitingVirtualHostTLS))

		f.NamespacedTest("httpproxy-global-rate-limiting-route-tls", withRateLimitService(testGlobalRateLimitingRouteTLS))

		f.NamespacedTest("httpproxy-global-rate-limiting-vhost-disable-per-route", withRateLimitService(testDisableVirtualHostGlobalRateLimitingOnRoute))
	})

	Context("default global rate limiting", func() {
		withRateLimitService := func(body e2e.NamespacedTestBody) e2e.NamespacedTestBody {
			return func(namespace string) {
				Context("with rate limit service", func() {
					BeforeEach(func() {
						contourConfig.RateLimitService = config.RateLimitService{
							ExtensionService: fmt.Sprintf("%s/%s", namespace, f.Deployment.RateLimitExtensionService.Name),
							Domain:           "contour-default-global-rate-limit",
							FailOpen:         false,
							DefaultGlobalRateLimitPolicy: &contour_v1.GlobalRateLimitPolicy{
								Descriptors: []contour_v1.RateLimitDescriptor{
									{
										Entries: []contour_v1.RateLimitDescriptorEntry{
											{
												RequestHeader: &contour_v1.RequestHeaderDescriptor{
													HeaderName:    "X-Default-Header",
													DescriptorKey: "defaultHeader",
												},
											},
										},
									},
									{
										Entries: []contour_v1.RateLimitDescriptorEntry{
											{
												RequestHeader: &contour_v1.RequestHeaderDescriptor{
													HeaderName:    "X-Another-Header",
													DescriptorKey: "anotherHeader",
												},
											},
										},
									},
								},
							},
						}
						contourConfiguration.Spec.RateLimitService = &contour_v1alpha1.RateLimitServiceConfig{
							ExtensionService: contour_v1alpha1.NamespacedName{
								Name:      f.Deployment.RateLimitExtensionService.Name,
								Namespace: namespace,
							},
							Domain:                  "contour-default-global-rate-limit",
							FailOpen:                ptr.To(false),
							EnableXRateLimitHeaders: ptr.To(false),
							DefaultGlobalRateLimitPolicy: &contour_v1.GlobalRateLimitPolicy{
								Descriptors: []contour_v1.RateLimitDescriptor{
									{
										Entries: []contour_v1.RateLimitDescriptorEntry{
											{
												RequestHeader: &contour_v1.RequestHeaderDescriptor{
													HeaderName:    "X-Default-Header",
													DescriptorKey: "defaultHeader",
												},
											},
										},
									},
									{
										Entries: []contour_v1.RateLimitDescriptorEntry{
											{
												RequestHeader: &contour_v1.RequestHeaderDescriptor{
													HeaderName:    "X-Another-Header",
													DescriptorKey: "anotherHeader",
												},
											},
										},
									},
								},
							},
						}
						require.NoError(f.T(),
							f.Deployment.EnsureRateLimitResources(
								namespace,
								`
domain: contour-default-global-rate-limit
descriptors:
  - key: generic_key
    value: foo
    rate_limit:
      unit: hour
      requests_per_unit: 1
  - key: defaultHeader
    rate_limit:
      unit: hour
      requests_per_unit: 1
  - key: customHeader
    rate_limit:
      unit: hour
      requests_per_unit: 1
  - key: anotherHeader
    rate_limit:
      unit: hour
      requests_per_unit: 1`))
					})

					body(namespace)
				})
			}
		}

		f.NamespacedTest("httpproxy-default-global-rate-limiting-vhost-non-tls", withRateLimitService(testDefaultGlobalRateLimitingVirtualHostNonTLS))
		f.NamespacedTest("httpproxy-default-global-rate-limiting-vhost-tls", withRateLimitService(testDefaultGlobalRateLimitingVirtualHostTLS))
		f.NamespacedTest("httpproxy-default-global-rate-limiting-vhost-rate-limits-ignore", withRateLimitService(testDefaultGlobalRateLimitingWithVhRateLimitsIgnore))
	})

	Context("cookie-rewriting", func() {
		f.NamespacedTest("app-cookie-rewrite", testAppCookieRewrite)

		f.NamespacedTest("cookie-rewrite-tls", testCookieRewriteTLS)

		Context("rewriting cookies from globally rewritten headers", func() {
			BeforeEach(func() {
				contourConfig.Policy = config.PolicyParameters{
					ResponseHeadersPolicy: config.HeadersPolicy{
						Set: map[string]string{
							"Set-Cookie": "global=foo",
						},
					},
				}
				contourConfiguration.Spec.Policy = &contour_v1alpha1.PolicyConfig{
					ResponseHeadersPolicy: &contour_v1alpha1.HeadersPolicy{
						Set: map[string]string{
							"Set-Cookie": "global=foo",
						},
					},
				}
			})

			f.NamespacedTest("global-rewrite-headers-cookie-rewrite", testHeaderGlobalRewriteCookieRewrite)
		})

		f.NamespacedTest("rewrite-headers-cookie-rewrite", testHeaderRewriteCookieRewrite)
	})

	Context("using root namespaces", func() {
		Context("configured via config CRD", func() {
			rootNamespaces := []string{
				"root-ns-crd-1",
				"root-ns-crd-2",
			}
			nonRootNamespace := "nonroot-ns-crd"

			BeforeEach(func() {
				if !e2e.UsingContourConfigCRD() {
					// Test only applies to contour config CRD.
					Skip("")
				}
				for _, ns := range rootNamespaces {
					f.CreateNamespace(ns)
				}
				contourConfiguration.Spec.HTTPProxy.RootNamespaces = rootNamespaces
			})

			AfterEach(func() {
				for _, ns := range rootNamespaces {
					f.DeleteNamespace(ns, false)
				}
			})

			f.NamespacedTest(nonRootNamespace, testRootNamespaces(rootNamespaces))
		})

		Context("configured via CLI flag", func() {
			rootNamespaces := []string{
				"root-ns-cli-1",
				"root-ns-cli-2",
			}
			nonRootNamespace := "nonroot-ns-cli"

			BeforeEach(func() {
				if e2e.UsingContourConfigCRD() {
					// Test only applies to contour configmap.
					Skip("")
				}
				for _, ns := range rootNamespaces {
					f.CreateNamespace(ns)
				}
				additionalContourArgs = []string{
					"--root-namespaces=" + strings.Join(rootNamespaces, ","),
				}
			})

			AfterEach(func() {
				for _, ns := range rootNamespaces {
					f.DeleteNamespace(ns, false)
				}
			})

			f.NamespacedTest(nonRootNamespace, testRootNamespaces(rootNamespaces))
		})
	})

	Context("using watch namespaces", func() {
		Context("only", func() {
			watchNamespaces := []string{
				"watched-ns-1",
				"watched-ns-2",
			}
			nonWatchedNamespace := "nonwatched-ns"

			BeforeEach(func() {
				if e2e.UsingContourConfigCRD() {
					// Test only applies to contour configmap.
					Skip("")
				}
				for _, ns := range watchNamespaces {
					f.CreateNamespace(ns)
				}
				additionalContourArgs = []string{
					"--watch-namespaces=" + strings.Join(watchNamespaces, ","),
				}
			})

			AfterEach(func() {
				for _, ns := range watchNamespaces {
					f.DeleteNamespace(ns, false)
				}
			})

			f.NamespacedTest(nonWatchedNamespace, testWatchNamespaces(watchNamespaces))
		})

		Context("with root namespaces", func() {
			rootNamespaces := []string{
				"root-ns-1",
				"root-ns-2",
			}
			nonRootNamespace := "nonroot-ns"
			watchNamespaces := append(rootNamespaces, nonRootNamespace)
			nonWatchedNamespace := "nonwatched-nonroot-ns"

			BeforeEach(func() {
				if e2e.UsingContourConfigCRD() {
					// Test only applies to contour configmap.
					Skip("")
				}
				for _, ns := range watchNamespaces {
					f.CreateNamespace(ns)
				}
				additionalContourArgs = []string{
					"--watch-namespaces=" + strings.Join(watchNamespaces, ","),
					"--root-namespaces=" + strings.Join(rootNamespaces, ","),
				}
			})

			AfterEach(func() {
				for _, ns := range watchNamespaces {
					f.DeleteNamespace(ns, false)
				}
			})

			f.NamespacedTest(nonWatchedNamespace, testWatchAndRootNamespaces(rootNamespaces, nonRootNamespace))
		})
	})

	f.NamespacedTest("httpproxy-crl", testClientCertRevocation)

	Context("gRPC tests", func() {
		f.NamespacedTest("grpc-upstream-plaintext", testGRPCServicePlaintext)

		f.NamespacedTest("grpc-web", testGRPCWeb)
	})

	Context("global external auth", func() {
		withGlobalExtAuth := func(body e2e.NamespacedTestBody) e2e.NamespacedTestBody {
			return func(namespace string) {
				Context("with global external auth service", func() {
					BeforeEach(func() {
						contourConfig.GlobalExternalAuthorization = config.GlobalExternalAuthorization{
							ExtensionService: fmt.Sprintf("%s/%s", namespace, "testserver"),
							FailOpen:         false,
							AuthPolicy: &config.GlobalAuthorizationPolicy{
								Context: map[string]string{
									"location": "global_config",
									"header_2": "message_2",
								},
							},
							ResponseTimeout: "10s",
						}
						contourConfiguration.Spec.GlobalExternalAuthorization = &contour_v1.AuthorizationServer{
							ExtensionServiceRef: contour_v1.ExtensionServiceReference{
								Namespace: namespace,
								Name:      "testserver",
							},
							FailOpen: false,
							AuthPolicy: &contour_v1.AuthorizationPolicy{
								Disabled: false,
								Context: map[string]string{
									"location": "global_config",
									"header_2": "message_2",
								},
							},
							ResponseTimeout: "10s",
						}
						require.NoError(f.T(),
							f.Deployment.EnsureGlobalExternalAuthResources(namespace))
					})
					body(namespace)
				})
			}
		}

		f.NamespacedTest("httpproxy-global-ext-auth-non-tls", withGlobalExtAuth(testGlobalExternalAuthVirtualHostNonTLS))

		f.NamespacedTest("httpproxy-global-ext-auth-tls", withGlobalExtAuth(testGlobalExternalAuthTLS))

		f.NamespacedTest("httpproxy-global-ext-auth-non-tls-disabled", withGlobalExtAuth(testGlobalExternalAuthNonTLSAuthDisabled))

		f.NamespacedTest("httpproxy-global-ext-auth-tls-disabled", withGlobalExtAuth(testGlobalExternalAuthTLSAuthDisabled))
	})
})
