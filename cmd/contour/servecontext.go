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

package main

import (
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io/ioutil"
	"strings"
	"time"

	"github.com/projectcontour/contour/internal/k8s"
	"k8s.io/utils/pointer"

	contour_api_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	xdscache_v3 "github.com/projectcontour/contour/internal/xdscache/v3"
	"github.com/projectcontour/contour/pkg/config"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
)

type serveContext struct {
	// Name of the ContourConfiguration CRD to use for configuration.
	contourConfigurationName string

	Config config.Parameters

	ServerConfig

	// Enable Kubernetes client-go debugging.
	KubernetesDebug uint

	// contour's debug handler parameters
	debugAddr string
	debugPort int

	// contour's metrics handler parameters
	metricsAddr string
	metricsPort int

	// Contour's health handler parameters.
	healthAddr string
	healthPort int

	// httpproxy root namespaces
	rootNamespaces string

	// ingress class
	ingressClassName string

	// envoy's stats listener parameters
	statsAddr string
	statsPort int

	// envoy's listener parameters
	useProxyProto bool

	// envoy's http listener parameters
	httpAddr      string
	httpPort      int
	httpAccessLog string

	// envoy's https listener parameters
	httpsAddr      string
	httpsPort      int
	httpsAccessLog string

	// PermitInsecureGRPC disables TLS on Contour's gRPC listener.
	PermitInsecureGRPC bool

	// DisableLeaderElection can only be set by command line flag.
	DisableLeaderElection bool
}

type ServerConfig struct {
	// contour's xds service parameters
	xdsAddr                         string
	xdsPort                         int
	caFile, contourCert, contourKey string
}

// newServeContext returns a serveContext initialized to defaults.
func newServeContext() *serveContext {
	// Set defaults for parameters which are then overridden via flags, ENV, or ConfigFile
	return &serveContext{
		Config:                config.Defaults(),
		statsAddr:             "0.0.0.0",
		statsPort:             8002,
		debugAddr:             "127.0.0.1",
		debugPort:             6060,
		healthAddr:            "0.0.0.0",
		healthPort:            8000,
		metricsAddr:           "0.0.0.0",
		metricsPort:           8000,
		httpAccessLog:         xdscache_v3.DEFAULT_HTTP_ACCESS_LOG,
		httpsAccessLog:        xdscache_v3.DEFAULT_HTTPS_ACCESS_LOG,
		httpAddr:              "0.0.0.0",
		httpsAddr:             "0.0.0.0",
		httpPort:              8080,
		httpsPort:             8443,
		PermitInsecureGRPC:    false,
		DisableLeaderElection: false,
		ServerConfig: ServerConfig{
			xdsAddr:     "127.0.0.1",
			xdsPort:     8001,
			caFile:      "",
			contourCert: "",
			contourKey:  "",
		},
	}
}

// grpcOptions returns a slice of grpc.ServerOptions.
// if ctx.PermitInsecureGRPC is false, the option set will
// include TLS configuration.
func grpcOptions(log logrus.FieldLogger, contourXDSConfig *contour_api_v1alpha1.TLS) []grpc.ServerOption {
	opts := []grpc.ServerOption{
		// By default the Go grpc library defaults to a value of ~100 streams per
		// connection. This number is likely derived from the HTTP/2 spec:
		// https://http2.github.io/http2-spec/#SettingValues
		// We need to raise this value because Envoy will open one EDS stream per
		// CDS entry. There doesn't seem to be a penalty for increasing this value,
		// so set it the limit similar to envoyproxy/go-control-plane#70.
		//
		// Somewhat arbitrary limit to handle many, many, EDS streams.
		grpc.MaxConcurrentStreams(1 << 20),
		// Set gRPC keepalive params.
		// See https://github.com/projectcontour/contour/issues/1756 for background.
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			PermitWithoutStream: true,
		}),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Time:    60 * time.Second,
			Timeout: 20 * time.Second,
		}),
	}
	if contourXDSConfig != nil && !contourXDSConfig.Insecure {
		tlsconfig := tlsconfig(log, contourXDSConfig)
		creds := credentials.NewTLS(tlsconfig)
		opts = append(opts, grpc.Creds(creds))
	}
	return opts
}

// tlsconfig returns a new *tls.Config. If the TLS parameters passed are not properly configured
// for tls communication, tlsconfig returns nil.
func tlsconfig(log logrus.FieldLogger, contourXDSTLS *contour_api_v1alpha1.TLS) *tls.Config {
	err := verifyTLSFlags(contourXDSTLS)
	if err != nil {
		log.WithError(err).Fatal("failed to verify TLS flags")
	}

	// Define a closure that lazily loads certificates and key at TLS handshake
	// to ensure that latest certificates are used in case they have been rotated.
	loadConfig := func() (*tls.Config, error) {
		if contourXDSTLS == nil {
			return nil, nil
		}
		cert, err := tls.LoadX509KeyPair(contourXDSTLS.CertFile, contourXDSTLS.KeyFile)
		if err != nil {
			return nil, err
		}

		ca, err := ioutil.ReadFile(contourXDSTLS.CAFile)
		if err != nil {
			return nil, err
		}

		certPool := x509.NewCertPool()
		if ok := certPool.AppendCertsFromPEM(ca); !ok {
			return nil, fmt.Errorf("unable to append certificate in %s to CA pool", contourXDSTLS.CAFile)
		}

		return &tls.Config{
			Certificates: []tls.Certificate{cert},
			ClientAuth:   tls.RequireAndVerifyClientCert,
			ClientCAs:    certPool,
			MinVersion:   tls.VersionTLS12,
		}, nil
	}

	// Attempt to load certificates and key to catch configuration errors early.
	if _, lerr := loadConfig(); lerr != nil {
		log.WithError(lerr).Fatal("failed to load certificate and key")
	}

	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		ClientAuth: tls.RequireAndVerifyClientCert,
		Rand:       rand.Reader,
		GetConfigForClient: func(*tls.ClientHelloInfo) (*tls.Config, error) {
			return loadConfig()
		},
	}
}

// verifyTLSFlags indicates if the TLS flags are set up correctly.
func verifyTLSFlags(contourXDSTLS *contour_api_v1alpha1.TLS) error {
	if contourXDSTLS.CAFile == "" && contourXDSTLS.CertFile == "" && contourXDSTLS.KeyFile == "" {
		return errors.New("no TLS parameters and --insecure not supplied. You must supply one or the other")
	}
	// If one of the three TLS commands is not empty, they all must be not empty
	if !(contourXDSTLS.CAFile != "" && contourXDSTLS.CertFile != "" && contourXDSTLS.KeyFile != "") {
		return errors.New("you must supply all three TLS parameters - --contour-cafile, --contour-cert-file, --contour-key-file, or none of them")
	}

	return nil
}

// proxyRootNamespaces returns a slice of namespaces restricting where
// contour should look for httpproxy roots.
func (ctx *serveContext) proxyRootNamespaces() []string {
	if strings.TrimSpace(ctx.rootNamespaces) == "" {
		return nil
	}
	var ns []string
	for _, s := range strings.Split(ctx.rootNamespaces, ",") {
		ns = append(ns, strings.TrimSpace(s))
	}
	return ns
}

// parseDefaultHTTPVersions parses a list of supported HTTP versions
//  (of the form "HTTP/xx") into a slice of unique version constants.
func parseDefaultHTTPVersions(versions []contour_api_v1alpha1.HTTPVersionType) []envoy_v3.HTTPVersionType {
	wanted := map[envoy_v3.HTTPVersionType]struct{}{}

	for _, v := range versions {
		switch v {
		case contour_api_v1alpha1.HTTPVersion1:
			wanted[envoy_v3.HTTPVersion1] = struct{}{}
		case contour_api_v1alpha1.HTTPVersion2:
			wanted[envoy_v3.HTTPVersion2] = struct{}{}
		}
	}

	var parsed []envoy_v3.HTTPVersionType
	for k := range wanted {
		parsed = append(parsed, k)
	}

	return parsed
}

func (ctx *serveContext) convertToContourConfigurationSpec() contour_api_v1alpha1.ContourConfigurationSpec {
	ingress := &contour_api_v1alpha1.IngressConfig{}
	if len(ctx.ingressClassName) > 0 {
		ingress.ClassName = pointer.StringPtr(ctx.ingressClassName)
	}
	if len(ctx.Config.IngressStatusAddress) > 0 {
		ingress.StatusAddress = pointer.StringPtr(ctx.Config.IngressStatusAddress)
	}

	debugLogLevel := contour_api_v1alpha1.InfoLog
	switch ctx.Config.Debug {
	case true:
		debugLogLevel = contour_api_v1alpha1.DebugLog
	case false:
		debugLogLevel = contour_api_v1alpha1.InfoLog
	}

	var gatewayConfig *contour_api_v1alpha1.GatewayConfig
	if ctx.Config.GatewayConfig != nil {
		gatewayConfig = &contour_api_v1alpha1.GatewayConfig{
			ControllerName: ctx.Config.GatewayConfig.ControllerName,
		}
	}

	var cipherSuites []contour_api_v1alpha1.TLSCipherType
	for _, suite := range ctx.Config.TLS.CipherSuites {
		cipherSuites = append(cipherSuites, contour_api_v1alpha1.TLSCipherType(suite))
	}

	var accessLogFormat contour_api_v1alpha1.AccessLogType
	switch ctx.Config.AccessLogFormat {
	case config.EnvoyAccessLog:
		accessLogFormat = contour_api_v1alpha1.EnvoyAccessLog
	case config.JSONAccessLog:
		accessLogFormat = contour_api_v1alpha1.JSONAccessLog
	}

	var accessLogFields contour_api_v1alpha1.AccessLogFields
	for _, alf := range ctx.Config.AccessLogFields {
		accessLogFields = append(accessLogFields, alf)
	}

	var defaultHTTPVersions []contour_api_v1alpha1.HTTPVersionType
	for _, version := range ctx.Config.DefaultHTTPVersions {
		switch version {
		case config.HTTPVersion1:
			defaultHTTPVersions = append(defaultHTTPVersions, contour_api_v1alpha1.HTTPVersion1)
		case config.HTTPVersion2:
			defaultHTTPVersions = append(defaultHTTPVersions, contour_api_v1alpha1.HTTPVersion2)
		}
	}

	timeoutParams := &contour_api_v1alpha1.TimeoutParameters{}
	if len(ctx.Config.Timeouts.RequestTimeout) > 0 {
		timeoutParams.RequestTimeout = pointer.StringPtr(ctx.Config.Timeouts.RequestTimeout)
	}
	if len(ctx.Config.Timeouts.ConnectionIdleTimeout) > 0 {
		timeoutParams.ConnectionIdleTimeout = pointer.StringPtr(ctx.Config.Timeouts.ConnectionIdleTimeout)
	}
	if len(ctx.Config.Timeouts.StreamIdleTimeout) > 0 {
		timeoutParams.StreamIdleTimeout = pointer.StringPtr(ctx.Config.Timeouts.StreamIdleTimeout)
	}
	if len(ctx.Config.Timeouts.MaxConnectionDuration) > 0 {
		timeoutParams.MaxConnectionDuration = pointer.StringPtr(ctx.Config.Timeouts.MaxConnectionDuration)
	}
	if len(ctx.Config.Timeouts.DelayedCloseTimeout) > 0 {
		timeoutParams.DelayedCloseTimeout = pointer.StringPtr(ctx.Config.Timeouts.DelayedCloseTimeout)
	}
	if len(ctx.Config.Timeouts.ConnectionShutdownGracePeriod) > 0 {
		timeoutParams.ConnectionShutdownGracePeriod = pointer.StringPtr(ctx.Config.Timeouts.ConnectionShutdownGracePeriod)
	}

	var dnsLookupFamily contour_api_v1alpha1.ClusterDNSFamilyType
	switch ctx.Config.Cluster.DNSLookupFamily {
	case config.AutoClusterDNSFamily:
		dnsLookupFamily = contour_api_v1alpha1.AutoClusterDNSFamily
	case config.IPv6ClusterDNSFamily:
		dnsLookupFamily = contour_api_v1alpha1.IPv6ClusterDNSFamily
	case config.IPv4ClusterDNSFamily:
		dnsLookupFamily = contour_api_v1alpha1.IPv4ClusterDNSFamily
	}

	var rateLimitService *contour_api_v1alpha1.RateLimitServiceConfig
	if ctx.Config.RateLimitService.ExtensionService != "" {
		rateLimitService = &contour_api_v1alpha1.RateLimitServiceConfig{
			ExtensionService: contour_api_v1alpha1.NamespacedName{
				Name:      k8s.NamespacedNameFrom(ctx.Config.RateLimitService.ExtensionService).Name,
				Namespace: k8s.NamespacedNameFrom(ctx.Config.RateLimitService.ExtensionService).Namespace,
			},
			Domain:                  ctx.Config.RateLimitService.Domain,
			FailOpen:                ctx.Config.RateLimitService.FailOpen,
			EnableXRateLimitHeaders: ctx.Config.RateLimitService.EnableXRateLimitHeaders,
		}
	}

	policy := &contour_api_v1alpha1.PolicyConfig{
		RequestHeadersPolicy: &contour_api_v1alpha1.HeadersPolicy{
			Set:    ctx.Config.Policy.RequestHeadersPolicy.Set,
			Remove: ctx.Config.Policy.RequestHeadersPolicy.Remove,
		},
		ResponseHeadersPolicy: &contour_api_v1alpha1.HeadersPolicy{
			Set:    ctx.Config.Policy.ResponseHeadersPolicy.Set,
			Remove: ctx.Config.Policy.ResponseHeadersPolicy.Remove,
		},
		ApplyToIngress: ctx.Config.Policy.ApplyToIngress,
	}

	var clientCertificate *contour_api_v1alpha1.NamespacedName
	if len(ctx.Config.TLS.ClientCertificate.Name) > 0 {
		clientCertificate = &contour_api_v1alpha1.NamespacedName{
			Name:      ctx.Config.TLS.ClientCertificate.Name,
			Namespace: ctx.Config.TLS.ClientCertificate.Namespace,
		}
	}

	var accessLogFormatString *string
	if len(ctx.Config.AccessLogFormatString) > 0 {
		accessLogFormatString = pointer.StringPtr(ctx.Config.AccessLogFormatString)
	}

	var fallbackCertificate *contour_api_v1alpha1.NamespacedName
	if len(ctx.Config.TLS.FallbackCertificate.Name) > 0 {
		fallbackCertificate = &contour_api_v1alpha1.NamespacedName{
			Name:      ctx.Config.TLS.FallbackCertificate.Name,
			Namespace: ctx.Config.TLS.FallbackCertificate.Namespace,
		}
	}

	// Convert serveContext to a ContourConfiguration
	contourConfiguration := contour_api_v1alpha1.ContourConfigurationSpec{
		Ingress: ingress,
		Debug: contour_api_v1alpha1.DebugConfig{
			Address:                 ctx.debugAddr,
			Port:                    ctx.debugPort,
			DebugLogLevel:           debugLogLevel,
			KubernetesDebugLogLevel: ctx.KubernetesDebug,
		},
		Health: contour_api_v1alpha1.HealthConfig{
			Address: ctx.healthAddr,
			Port:    ctx.healthPort,
		},
		Envoy: contour_api_v1alpha1.EnvoyConfig{
			Listener: contour_api_v1alpha1.EnvoyListenerConfig{
				UseProxyProto:             ctx.useProxyProto,
				DisableAllowChunkedLength: ctx.Config.DisableAllowChunkedLength,
				ConnectionBalancer:        ctx.Config.Listener.ConnectionBalancer,
				TLS: contour_api_v1alpha1.EnvoyTLS{
					MinimumProtocolVersion: ctx.Config.TLS.MinimumProtocolVersion,
					CipherSuites:           cipherSuites,
				},
			},
			Service: contour_api_v1alpha1.NamespacedName{
				Name:      ctx.Config.EnvoyServiceName,
				Namespace: ctx.Config.EnvoyServiceNamespace,
			},
			HTTPListener: contour_api_v1alpha1.EnvoyListener{
				Address:   ctx.httpAddr,
				Port:      ctx.httpPort,
				AccessLog: ctx.httpAccessLog,
			},
			HTTPSListener: contour_api_v1alpha1.EnvoyListener{
				Address:   ctx.httpsAddr,
				Port:      ctx.httpsPort,
				AccessLog: ctx.httpsAccessLog,
			},
			Metrics: contour_api_v1alpha1.MetricsConfig{
				Address: ctx.statsAddr,
				Port:    ctx.statsPort,
			},
			ClientCertificate: clientCertificate,
			Logging: contour_api_v1alpha1.EnvoyLogging{
				AccessLogFormat:       accessLogFormat,
				AccessLogFormatString: accessLogFormatString,
				AccessLogFields:       accessLogFields,
			},
			DefaultHTTPVersions: defaultHTTPVersions,
			Timeouts:            timeoutParams,
			Cluster: contour_api_v1alpha1.ClusterParameters{
				DNSLookupFamily: dnsLookupFamily,
			},
			Network: contour_api_v1alpha1.NetworkParameters{
				XffNumTrustedHops: ctx.Config.Network.XffNumTrustedHops,
				EnvoyAdminPort:    ctx.Config.Network.EnvoyAdminPort,
			},
		},
		Gateway: gatewayConfig,
		HTTPProxy: contour_api_v1alpha1.HTTPProxyConfig{
			DisablePermitInsecure: ctx.Config.DisablePermitInsecure,
			RootNamespaces:        ctx.proxyRootNamespaces(),
			FallbackCertificate:   fallbackCertificate,
		},
		LeaderElection: contour_api_v1alpha1.LeaderElectionConfig{
			LeaseDuration: ctx.Config.LeaderElection.LeaseDuration.String(),
			RenewDeadline: ctx.Config.LeaderElection.RenewDeadline.String(),
			RetryPeriod:   ctx.Config.LeaderElection.RetryPeriod.String(),
			Configmap: contour_api_v1alpha1.NamespacedName{
				Name:      ctx.Config.LeaderElection.Name,
				Namespace: ctx.Config.LeaderElection.Namespace,
			},
			DisableLeaderElection: ctx.DisableLeaderElection,
		},
		EnableExternalNameService: ctx.Config.EnableExternalNameService,
		RateLimitService:          rateLimitService,
		Policy:                    policy,
		Metrics: contour_api_v1alpha1.MetricsConfig{
			Address: ctx.metricsAddr,
			Port:    ctx.metricsPort,
		},
	}

	xdsServerType := contour_api_v1alpha1.ContourServerType
	if ctx.Config.Server.XDSServerType == config.EnvoyServerType {
		xdsServerType = contour_api_v1alpha1.EnvoyServerType
	}

	contourConfiguration.XDSServer = contour_api_v1alpha1.XDSServerConfig{
		Type:    xdsServerType,
		Address: ctx.xdsAddr,
		Port:    ctx.xdsPort,
		TLS: &contour_api_v1alpha1.TLS{
			CAFile:   ctx.caFile,
			CertFile: ctx.contourCert,
			KeyFile:  ctx.contourKey,
			Insecure: ctx.PermitInsecureGRPC,
		},
	}

	return contourConfiguration
}
