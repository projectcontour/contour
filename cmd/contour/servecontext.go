// Copyright © 2019 VMware
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
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/projectcontour/contour/internal/contour"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
)

type serveContext struct {
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

	// ingressroute root namespaces
	rootNamespaces string

	// ingress class
	ingressClass string

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
	// permitInsecure field in IngressRoute.
	DisablePermitInsecure bool `yaml:"disablePermitInsecure,omitempty"`

	// DisableLeaderElection can only be set by command line flag.
	DisableLeaderElection bool `yaml:"-"`

	// LeaderElectionConfig can be set in the config file.
	LeaderElectionConfig `yaml:"leaderelection,omitempty"`

	// RequestTimeout sets the client request timeout globally for Contour.
	RequestTimeout time.Duration `yaml:"request-timeout,omitempty"`

	// Should Contour fall back to registering an informer for the deprecated
	// extensions/v1beta1.Ingress type.
	// By default this value is false, meaning Contour will register an informer for
	// networking/v1beta1.Ingress and expect the API server to rewrite extensions/v1beta1.Ingress
	// objects transparently.
	// If the value is true, Contour will register for extensions/v1beta1.Ingress type and do
	// the rewrite itself.
	UseExtensionsV1beta1Ingress bool `yaml:"-"`
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
		AccessLogFields: []string{
			"@timestamp",
			"authority",
			"bytes_received",
			"bytes_sent",
			"downstream_local_address",
			"downstream_remote_address",
			"duration",
			"method",
			"path",
			"protocol",
			"request_id",
			"requested_server_name",
			"response_code",
			"response_flags",
			"uber_trace_id",
			"upstream_cluster",
			"upstream_host",
			"upstream_local_address",
			"upstream_service_time",
			"user_agent",
			"x_forwarded_for",
		},
		LeaderElectionConfig: LeaderElectionConfig{
			LeaseDuration: time.Second * 15,
			RenewDeadline: time.Second * 10,
			RetryPeriod:   time.Second * 2,
			Namespace:     "projectcontour",
			Name:          "leader-elect",
		},
		UseExtensionsV1beta1Ingress: false,
	}
}

// TLSConfig holds configuration file TLS configuration details.
type TLSConfig struct {
	MinimumProtocolVersion string `yaml:"minimum-protocol-version"`
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

	cert, err := tls.LoadX509KeyPair(ctx.contourCert, ctx.contourKey)
	check(err)

	ca, err := ioutil.ReadFile(ctx.caFile)
	check(err)

	certPool := x509.NewCertPool()
	if ok := certPool.AppendCertsFromPEM(ca); !ok {
		log.Fatalf("unable to append certificate in %s to CA pool", ctx.caFile)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    certPool,
		Rand:         rand.Reader,
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

// ingressRouteRootNamespaces returns a slice of namespaces restricting where
// contour should look for ingressroute roots.
func (ctx *serveContext) ingressRouteRootNamespaces() []string {
	if strings.TrimSpace(ctx.rootNamespaces) == "" {
		return nil
	}
	var ns []string
	for _, s := range strings.Split(ctx.rootNamespaces, ",") {
		ns = append(ns, strings.TrimSpace(s))
	}
	return ns
}
