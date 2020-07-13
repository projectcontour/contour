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
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/projectcontour/contour/internal/envoy"
	"github.com/projectcontour/contour/internal/k8s"

	"github.com/projectcontour/contour/internal/contour"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
)

type serveContext struct {
	// Note about parameter behavior: if the parameter is going to be in the config file,
	// it has to be exported. If not, the YAML decoder will not see the field.

	// Enable debug logging
	Debug bool

	// contour's kubernetes client parameters
	InCluster  bool   `yaml:"incluster,omitempty"`
	Kubeconfig string `yaml:"kubeconfig,omitempty"`

	// contour's xds service parameters
	xdsAddr                         string
	xdsPort                         int
	caFile, contourCert, contourKey string

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
	ingressClass string

	// Address to be placed in status.loadbalancer field of Ingress objects.
	// May be either a literal IP address or a host name.
	// The value will be placed directly into the relevant field inside the status.loadBalancer struct.
	IngressStatusAddress string `yaml:"ingress-status-address,omitempty"`

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

	// Envoy's access logging format options

	// AccessLogFormat sets the global access log format.
	// Valid options are 'envoy' or 'json'
	AccessLogFormat string `yaml:"accesslog-format,omitempty"`

	// AccessLogFields sets the fields that JSON logging will
	// output when AccessLogFormat is json.
	AccessLogFields []string `yaml:"json-fields,omitempty"`

	// PermitInsecureGRPC disables TLS on Contour's gRPC listener.
	PermitInsecureGRPC bool `yaml:"-"`

	TLSConfig `yaml:"tls,omitempty"`

	// DisablePermitInsecure disables the use of the
	// permitInsecure field in HTTPProxy.
	DisablePermitInsecure bool `yaml:"disablePermitInsecure,omitempty"`

	// DisableLeaderElection can only be set by command line flag.
	DisableLeaderElection bool `yaml:"-"`

	// LeaderElectionConfig can be set in the config file.
	LeaderElectionConfig `yaml:"leaderelection,omitempty"`

	// TimeoutConfig holds various configurable timeouts that can
	// be set in the config file.
	TimeoutConfig `yaml:"timeouts,omitempty"`

	// RequestTimeout sets the client request timeout globally for Contour.
	RequestTimeout time.Duration `yaml:"request-timeout,omitempty"`

	// Should Contour register to watch the new service-apis types?
	// By default this value is false, meaning Contour will not do anything with any of the new
	// types.
	// If the value is true, Contour will register for all the service-apis types
	// (GatewayClass, Gateway, HTTPRoute, TCPRoute, and any more as they are added)
	UseExperimentalServiceAPITypes bool `yaml:"-"`

	// envoy service details

	// Namespace of the envoy service to inspect for Ingress status details.
	EnvoyServiceNamespace string `yaml:"envoy-service-namespace,omitempty"`

	// Name of the envoy service to inspect for Ingress status details.
	EnvoyServiceName string `yaml:"envoy-service-name,omitempty"`

	// DefaultHTTPVersions defines the default set of HTTPS
	// versions the proxy should accept. HTTP versions are
	// strings of the form "HTTP/xx". Supported versions are
	// "HTTP/1.1" and "HTTP/2".
	//
	// If this field not specified, all supported versions are accepted.
	DefaultHTTPVersions []string `yaml:"default-http-versions"`
}

// newServeContext returns a serveContext initialized to defaults.
func newServeContext() *serveContext {
	// Set defaults for parameters which are then overridden via flags, ENV, or ConfigFile
	return &serveContext{
		Kubeconfig:            filepath.Join(os.Getenv("HOME"), ".kube", "config"),
		xdsAddr:               "127.0.0.1",
		xdsPort:               8001,
		statsAddr:             "0.0.0.0",
		statsPort:             8002,
		debugAddr:             "127.0.0.1",
		debugPort:             6060,
		healthAddr:            "0.0.0.0",
		healthPort:            8000,
		metricsAddr:           "0.0.0.0",
		metricsPort:           8000,
		httpAccessLog:         contour.DEFAULT_HTTP_ACCESS_LOG,
		httpsAccessLog:        contour.DEFAULT_HTTPS_ACCESS_LOG,
		httpAddr:              "0.0.0.0",
		httpsAddr:             "0.0.0.0",
		httpPort:              8080,
		httpsPort:             8443,
		PermitInsecureGRPC:    false,
		DisablePermitInsecure: false,
		DisableLeaderElection: false,
		AccessLogFormat:       "envoy",
		LeaderElectionConfig: LeaderElectionConfig{
			LeaseDuration: time.Second * 15,
			RenewDeadline: time.Second * 10,
			RetryPeriod:   time.Second * 2,
			Namespace:     getEnv("CONTOUR_NAMESPACE", "projectcontour"),
			Name:          "leader-elect",
		},
		UseExperimentalServiceAPITypes: false,
		EnvoyServiceName:               "envoy",
		EnvoyServiceNamespace:          getEnv("CONTOUR_NAMESPACE", "projectcontour"),
		TimeoutConfig: TimeoutConfig{
			// This is chosen as a rough default to stop idle connections wasting resources,
			// without stopping slow connections from being terminated too quickly.
			ConnectionIdleTimeout: 60 * time.Second,

			// This is the Envoy default.
			StreamIdleTimeout: 5 * time.Minute,

			// This is the Envoy default.
			DrainTimeout: 5 * time.Second,
		},
	}
}

// TLSConfig holds configuration file TLS configuration details.
type TLSConfig struct {
	MinimumProtocolVersion string `yaml:"minimum-protocol-version"`

	// FallbackCertificate defines the namespace/name of the Kubernetes secret to
	// use as fallback when a non-SNI request is received.
	FallbackCertificate FallbackCertificate `yaml:"fallback-certificate,omitempty"`
}

// FallbackCertificate defines the namespace/name of the Kubernetes secret to
// use as fallback when a non-SNI request is received.
type FallbackCertificate struct {
	Name      string `yaml:"name"`
	Namespace string `yaml:"namespace"`
}

func (ctx *serveContext) fallbackCertificate() (*k8s.FullName, error) {
	if len(strings.TrimSpace(ctx.TLSConfig.FallbackCertificate.Name)) == 0 && len(strings.TrimSpace(ctx.TLSConfig.FallbackCertificate.Namespace)) == 0 {
		return nil, nil
	}

	// Validate namespace is defined
	if len(strings.TrimSpace(ctx.TLSConfig.FallbackCertificate.Namespace)) == 0 {
		return nil, errors.New("namespace must be defined")
	}

	// Validate name is defined
	if len(strings.TrimSpace(ctx.TLSConfig.FallbackCertificate.Name)) == 0 {
		return nil, errors.New("name must be defined")
	}

	return &k8s.FullName{
		Name:      ctx.TLSConfig.FallbackCertificate.Name,
		Namespace: ctx.TLSConfig.FallbackCertificate.Namespace,
	}, nil
}

// LeaderElectionConfig holds the config bits for leader election inside the
// configuration file.
type LeaderElectionConfig struct {
	LeaseDuration time.Duration `yaml:"lease-duration,omitempty"`
	RenewDeadline time.Duration `yaml:"renew-deadline,omitempty"`
	RetryPeriod   time.Duration `yaml:"retry-period,omitempty"`
	Namespace     string        `yaml:"configmap-namespace,omitempty"`
	Name          string        `yaml:"configmap-name,omitempty"`
}

// TimeoutConfig holds various configurable proxy timeout values.
type TimeoutConfig struct {
	// ConnectionIdleTimeout defines how long the proxy should wait while there
	// are no active requests before terminating an HTTP connection. Set
	// to 0 to disable the timeout.
	ConnectionIdleTimeout time.Duration `yaml:"connection-idle-timeout,omitempty"`

	// StreamIdleTimeout defines how long the proxy should wait while there
	// is no stream activity before terminating a stream. Set to 0 to
	// disable the timeout.
	StreamIdleTimeout time.Duration `yaml:"stream-idle-timeout,omitempty"`

	// MaxConnectionDuration defines the maximum period of time after an HTTP connection
	// has been established from the client to the proxy before it is closed by the proxy,
	// regardless of whether there has been activity or not. Set to 0 for no max duration.
	MaxConnectionDuration time.Duration `yaml:"max-connection-duration,omitempty"`

	// DrainTimeout defines how long the proxy will wait between sending an initial GOAWAY
	// frame and a second, final GOAWAY frame when terminating an HTTP/2 connection. During
	// this grace period, the proxy will continue to respond to new streams. After the final
	// GOAWAY frame has been sent, the proxy will refuse new streams. Set to 0 for no grace
	// period.
	DrainTimeout time.Duration `yaml:"drain-timeout,omitempty"`
}

// grpcOptions returns a slice of grpc.ServerOptions.
// if ctx.PermitInsecureGRPC is false, the option set will
// include TLS configuration.
func (ctx *serveContext) grpcOptions() []grpc.ServerOption {
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
	if !ctx.PermitInsecureGRPC {
		tlsconfig := ctx.tlsconfig()
		creds := credentials.NewTLS(tlsconfig)
		opts = append(opts, grpc.Creds(creds))
	}
	return opts
}

// tlsconfig returns a new *tls.Config. If the context is not properly configured
// for tls communication, tlsconfig returns nil.
func (ctx *serveContext) tlsconfig() *tls.Config {
	err := ctx.verifyTLSFlags()
	check(err)

	// Define a closure that lazily loads certificates and key at TLS handshake
	// to ensure that latest certificates are used in case they have been rotated.
	loadConfig := func() (*tls.Config, error) {
		cert, err := tls.LoadX509KeyPair(ctx.contourCert, ctx.contourKey)
		if err != nil {
			return nil, err
		}

		ca, err := ioutil.ReadFile(ctx.caFile)
		if err != nil {
			return nil, err
		}

		certPool := x509.NewCertPool()
		if ok := certPool.AppendCertsFromPEM(ca); !ok {
			return nil, fmt.Errorf("unable to append certificate in %s to CA pool", ctx.caFile)
		}

		return &tls.Config{
			Certificates: []tls.Certificate{cert},
			ClientAuth:   tls.RequireAndVerifyClientCert,
			ClientCAs:    certPool,
			Rand:         rand.Reader,
		}, nil
	}

	// Attempt to load certificates and key to catch configuration errors early.
	_, err = loadConfig()
	check(err)

	return &tls.Config{
		ClientAuth: tls.RequireAndVerifyClientCert,
		Rand:       rand.Reader,
		GetConfigForClient: func(*tls.ClientHelloInfo) (*tls.Config, error) {
			config, err := loadConfig()
			check(err)
			return config, err
		},
	}
}

// verifyTLSFlags indicates if the TLS flags are set up correctly.
func (ctx *serveContext) verifyTLSFlags() error {
	if ctx.caFile == "" && ctx.contourCert == "" && ctx.contourKey == "" {
		return errors.New("no TLS parameters and --insecure not supplied. You must supply one or the other")
	}
	// If one of the three TLS commands is not empty, they all must be not empty
	if !(ctx.caFile != "" && ctx.contourCert != "" && ctx.contourKey != "") {
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
func parseDefaultHTTPVersions(versions []string) ([]envoy.HTTPVersionType, error) {
	wanted := map[envoy.HTTPVersionType]struct{}{}

	for _, v := range versions {
		switch strings.ToLower(v) {
		case "http/1.1":
			wanted[envoy.HTTPVersion1] = struct{}{}
		case "http/2":
			wanted[envoy.HTTPVersion2] = struct{}{}
		default:
			return nil, fmt.Errorf("invalid HTTP protocol version %q", v)
		}
	}

	var parsed []envoy.HTTPVersionType
	for k := range wanted {
		parsed = append(parsed, k)

	}

	return parsed, nil
}

// Simple helper function to read an environment or return a default value
func getEnv(key string, defaultVal string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultVal
}
