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
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tsaarni/certyaml"
	"golang.org/x/net/http2"
	"google.golang.org/grpc"
	"k8s.io/utils/ptr"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/fixture"
	"github.com/projectcontour/contour/pkg/config"
)

func TestServeContextProxyRootNamespaces(t *testing.T) {
	tests := map[string]struct {
		ctx  serveContext
		want []string
	}{
		"empty": {
			ctx: serveContext{
				rootNamespaces: "",
			},
			want: nil,
		},
		"blank-ish": {
			ctx: serveContext{
				rootNamespaces: " \t ",
			},
			want: nil,
		},
		"one value": {
			ctx: serveContext{
				rootNamespaces: "projectcontour",
			},
			want: []string{"projectcontour"},
		},
		"multiple, easy": {
			ctx: serveContext{
				rootNamespaces: "prod1,prod2,prod3",
			},
			want: []string{"prod1", "prod2", "prod3"},
		},
		"multiple, hard": {
			ctx: serveContext{
				rootNamespaces: "prod1, prod2, prod3 ",
			},
			want: []string{"prod1", "prod2", "prod3"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := tc.ctx.proxyRootNamespaces()
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("expected: %q, got: %q", tc.want, got)
			}
		})
	}
}

func TestServeContextTLSParams(t *testing.T) {
	tests := map[string]struct {
		tls         *contour_v1alpha1.TLS
		expectError bool
	}{
		"tls supplied correctly": {
			tls: &contour_v1alpha1.TLS{
				CAFile:   "cacert.pem",
				CertFile: "contourcert.pem",
				KeyFile:  "contourkey.pem",
				Insecure: ptr.To(false),
			},
			expectError: false,
		},
		"tls partially supplied": {
			tls: &contour_v1alpha1.TLS{
				CertFile: "contourcert.pem",
				KeyFile:  "contourkey.pem",
				Insecure: ptr.To(false),
			},
			expectError: true,
		},
		"tls not supplied": {
			tls:         &contour_v1alpha1.TLS{},
			expectError: true,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := verifyTLSFlags(tc.tls)
			goterror := err != nil
			if goterror != tc.expectError {
				t.Errorf("TLS Config: %s", err)
			}
		})
	}
}

func TestServeContextCertificateHandling(t *testing.T) {
	// Create trusted CA, server and client certs.
	trustedCACert := certyaml.Certificate{
		Subject: "cn=trusted-ca",
	}
	contourCertBeforeRotation := certyaml.Certificate{
		Subject:         "cn=contour-before-rotation",
		SubjectAltNames: []string{"DNS:localhost"},
		Issuer:          &trustedCACert,
	}
	contourCertAfterRotation := certyaml.Certificate{
		Subject:         "cn=contour-after-rotation",
		SubjectAltNames: []string{"DNS:localhost"},
		Issuer:          &trustedCACert,
	}
	trustedEnvoyCert := certyaml.Certificate{
		Subject: "cn=trusted-envoy",
		Issuer:  &trustedCACert,
	}

	// Create another CA and a client cert to test that untrusted clients are denied.
	untrustedCACert := certyaml.Certificate{
		Subject: "cn=untrusted-ca",
	}
	untrustedClientCert := certyaml.Certificate{
		Subject: "cn=untrusted-client",
		Issuer:  &untrustedCACert,
	}

	caCertPool := x509.NewCertPool()
	ca, err := trustedCACert.X509Certificate()
	require.NoError(t, err)
	caCertPool.AddCert(&ca)

	tests := map[string]struct {
		serverCredentials *certyaml.Certificate
		clientCredentials *certyaml.Certificate
		expectError       bool
	}{
		"successful TLS connection established": {
			serverCredentials: &contourCertBeforeRotation,
			clientCredentials: &trustedEnvoyCert,
			expectError:       false,
		},
		"rotating server credentials returns new server cert": {
			serverCredentials: &contourCertAfterRotation,
			clientCredentials: &trustedEnvoyCert,
			expectError:       false,
		},
		"rotating server credentials again to ensure rotation can be repeated": {
			serverCredentials: &contourCertBeforeRotation,
			clientCredentials: &trustedEnvoyCert,
			expectError:       false,
		},
		"fail to connect with client certificate which is not signed by correct CA": {
			serverCredentials: &contourCertBeforeRotation,
			clientCredentials: &untrustedClientCert,
			expectError:       true,
		},
	}

	// Create temporary directory to store certificates and key for the server.
	configDir, err := os.MkdirTemp("", "contour-testdata-")
	require.NoError(t, err)
	defer os.RemoveAll(configDir)

	contourTLS := &contour_v1alpha1.TLS{
		CAFile:   filepath.Join(configDir, "CAcert.pem"),
		CertFile: filepath.Join(configDir, "contourcert.pem"),
		KeyFile:  filepath.Join(configDir, "contourkey.pem"),
		Insecure: ptr.To(false),
	}

	// Initial set of credentials must be written into temp directory before
	// starting the tests to avoid error at server startup.
	err = trustedCACert.WritePEM(contourTLS.CAFile, filepath.Join(configDir, "CAkey.pem"))
	require.NoError(t, err)
	err = contourCertBeforeRotation.WritePEM(contourTLS.CertFile, contourTLS.KeyFile)
	require.NoError(t, err)

	// Start a dummy server.
	log := fixture.NewTestLogger(t)
	opts := grpcOptions(log, contourTLS)
	g := grpc.NewServer(opts...)
	require.NotNil(t, g)

	l, err := net.Listen("tcp", "localhost:")
	require.NoError(t, err)
	address := l.Addr().String()

	go func() {
		// If server fails to start, connecting to it below will fail so
		// can ignore the error.
		_ = g.Serve(l)
	}()
	defer g.GracefulStop()

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Store certificate and key to temp dir used by serveContext.
			err = tc.serverCredentials.WritePEM(contourTLS.CertFile, contourTLS.KeyFile)
			require.NoError(t, err)
			clientCert, err := tc.clientCredentials.TLSCertificate()
			require.NoError(t, err)
			receivedCert, err := tryConnect(address, clientCert, caCertPool)
			if tc.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				expectedCert, err := tc.serverCredentials.X509Certificate()
				require.NoError(t, err)
				assert.Equal(t, &expectedCert, receivedCert)
			}
		})
	}
}

func TestTlsVersionDeprecation(t *testing.T) {
	// To get tls.Config for the gRPC XDS server, we need to arrange valid TLS certificates and keys.
	// Create temporary directory to store them for the server.
	configDir, err := os.MkdirTemp("", "contour-testdata-")
	require.NoError(t, err)
	defer os.RemoveAll(configDir)

	caCert := certyaml.Certificate{
		Subject: "cn=ca",
	}
	contourCert := certyaml.Certificate{
		Subject: "cn=contourBeforeRotation",
		Issuer:  &caCert,
	}

	contourTLS := &contour_v1alpha1.TLS{
		CAFile:   filepath.Join(configDir, "CAcert.pem"),
		CertFile: filepath.Join(configDir, "contourcert.pem"),
		KeyFile:  filepath.Join(configDir, "contourkey.pem"),
		Insecure: ptr.To(false),
	}

	err = caCert.WritePEM(contourTLS.CAFile, filepath.Join(configDir, "CAkey.pem"))
	require.NoError(t, err)
	err = contourCert.WritePEM(contourTLS.CertFile, contourTLS.KeyFile)
	require.NoError(t, err)

	// Get preliminary TLS config from the serveContext.
	log := fixture.NewTestLogger(t)
	preliminaryTLSConfig := tlsconfig(log, contourTLS)

	// Get actual TLS config that will be used during TLS handshake.
	tlsConfig, err := preliminaryTLSConfig.GetConfigForClient(nil)
	require.NoError(t, err)

	assert.Equal(t, tlsConfig.MinVersion, uint16(tls.VersionTLS13))
}

// tryConnect tries to establish TLS connection to the server.
// If successful, return the server certificate.
func tryConnect(address string, clientCert tls.Certificate, caCertPool *x509.CertPool) (*x509.Certificate, error) {
	rawConn, err := net.Dial("tcp", address)
	if err != nil {
		rawConn.Close()
		return nil, errors.Wrapf(err, "error dialing %s", address)
	}

	clientConfig := &tls.Config{
		ServerName:   "localhost",
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{clientCert},
		RootCAs:      caCertPool,
		NextProtos:   []string{http2.NextProtoTLS},
	}

	conn := tls.Client(rawConn, clientConfig)
	defer conn.Close()

	if err := peekError(conn); err != nil {
		return nil, errors.Wrap(err, "error peeking TLS alert")
	}

	return conn.ConnectionState().PeerCertificates[0], nil
}

// peekError is a workaround for TLS 1.3: due to shortened handshake, TLS alert
// from server is received at first read from the socket.
// To receive alert for bad certificate, this function tries to read one byte.
// Adapted from https://golang.org/src/crypto/tls/handshake_client_test.go
func peekError(conn net.Conn) error {
	if err := conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond)); err != nil {
		return err
	}
	_, err := conn.Read(make([]byte, 1))
	if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return nil
}

func TestParseHTTPVersions(t *testing.T) {
	cases := map[string]struct {
		versions      []contour_v1alpha1.HTTPVersionType
		parseVersions []envoy_v3.HTTPVersionType
	}{
		"empty": {
			versions:      []contour_v1alpha1.HTTPVersionType{},
			parseVersions: nil,
		},
		"http/1.1": {
			versions:      []contour_v1alpha1.HTTPVersionType{contour_v1alpha1.HTTPVersion1},
			parseVersions: []envoy_v3.HTTPVersionType{envoy_v3.HTTPVersion1},
		},
		"http/1.1+http/2": {
			versions:      []contour_v1alpha1.HTTPVersionType{contour_v1alpha1.HTTPVersion1, contour_v1alpha1.HTTPVersion2},
			parseVersions: []envoy_v3.HTTPVersionType{envoy_v3.HTTPVersion1, envoy_v3.HTTPVersion2},
		},
		"http/1.1+http/2 duplicated": {
			versions: []contour_v1alpha1.HTTPVersionType{
				contour_v1alpha1.HTTPVersion1, contour_v1alpha1.HTTPVersion2,
				contour_v1alpha1.HTTPVersion1, contour_v1alpha1.HTTPVersion2,
			},
			parseVersions: []envoy_v3.HTTPVersionType{envoy_v3.HTTPVersion1, envoy_v3.HTTPVersion2},
		},
	}

	for name, testcase := range cases {
		testcase := testcase
		t.Run(name, func(t *testing.T) {
			vers := parseDefaultHTTPVersions(testcase.versions)

			// parseDefaultHTTPVersions doesn't guarantee a stable result, but the order doesn't matter.
			sort.Slice(vers,
				func(i, j int) bool { return vers[i] < vers[j] })
			sort.Slice(testcase.parseVersions,
				func(i, j int) bool { return testcase.parseVersions[i] < testcase.parseVersions[j] })

			assert.Equal(t, testcase.parseVersions, vers)
		})
	}
}

func defaultContext() *serveContext {
	ctx := newServeContext()
	ctx.ServerConfig = ServerConfig{
		xdsAddr:     "127.0.0.1",
		xdsPort:     8001,
		caFile:      "/certs/ca.crt",
		contourCert: "/certs/cert.crt",
		contourKey:  "/certs/cert.key",
	}
	return ctx
}

func defaultContourConfiguration() contour_v1alpha1.ContourConfigurationSpec {
	return contour_v1alpha1.ContourConfigurationSpec{
		XDSServer: &contour_v1alpha1.XDSServerConfig{
			Address: "127.0.0.1",
			Port:    8001,
			TLS: &contour_v1alpha1.TLS{
				CAFile:   "/certs/ca.crt",
				CertFile: "/certs/cert.crt",
				KeyFile:  "/certs/cert.key",
				Insecure: ptr.To(false),
			},
		},
		Ingress: &contour_v1alpha1.IngressConfig{
			ClassNames:    nil,
			StatusAddress: "",
		},
		Debug: &contour_v1alpha1.DebugConfig{
			Address: "127.0.0.1",
			Port:    6060,
		},
		Health: &contour_v1alpha1.HealthConfig{
			Address: "0.0.0.0",
			Port:    8000,
		},
		Envoy: &contour_v1alpha1.EnvoyConfig{
			Service: &contour_v1alpha1.NamespacedName{
				Name:      "envoy",
				Namespace: "projectcontour",
			},
			Listener: &contour_v1alpha1.EnvoyListenerConfig{
				UseProxyProto:              ptr.To(false),
				DisableAllowChunkedLength:  ptr.To(false),
				DisableMergeSlashes:        ptr.To(false),
				ServerHeaderTransformation: contour_v1alpha1.OverwriteServerHeader,
				TLS: &contour_v1alpha1.EnvoyTLS{
					MinimumProtocolVersion: "",
					MaximumProtocolVersion: "",
				},
				SocketOptions: &contour_v1alpha1.SocketOptions{
					TOS:          0,
					TrafficClass: 0,
				},
			},
			HTTPListener: &contour_v1alpha1.EnvoyListener{
				Address:   "0.0.0.0",
				Port:      8080,
				AccessLog: "/dev/stdout",
			},
			HTTPSListener: &contour_v1alpha1.EnvoyListener{
				Address:   "0.0.0.0",
				Port:      8443,
				AccessLog: "/dev/stdout",
			},
			Health: &contour_v1alpha1.HealthConfig{
				Address: "0.0.0.0",
				Port:    8002,
			},
			Metrics: &contour_v1alpha1.MetricsConfig{
				Address: "0.0.0.0",
				Port:    8002,
			},
			ClientCertificate: nil,
			Logging: &contour_v1alpha1.EnvoyLogging{
				AccessLogFormat:       contour_v1alpha1.EnvoyAccessLog,
				AccessLogFormatString: "",
				AccessLogLevel:        contour_v1alpha1.LogLevelInfo,
				AccessLogJSONFields: contour_v1alpha1.AccessLogJSONFields([]string{
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
					"grpc_status",
					"grpc_status_number",
				}),
			},
			DefaultHTTPVersions: nil,
			Timeouts: &contour_v1alpha1.TimeoutParameters{
				ConnectionIdleTimeout: ptr.To("60s"),
				ConnectTimeout:        ptr.To("2s"),
			},
			Cluster: &contour_v1alpha1.ClusterParameters{
				DNSLookupFamily:              contour_v1alpha1.AutoClusterDNSFamily,
				GlobalCircuitBreakerDefaults: nil,
				UpstreamTLS: &contour_v1alpha1.EnvoyTLS{
					MinimumProtocolVersion: "",
					MaximumProtocolVersion: "",
				},
			},
			Network: &contour_v1alpha1.NetworkParameters{
				EnvoyAdminPort:            ptr.To(9001),
				XffNumTrustedHops:         ptr.To(uint32(0)),
				EnvoyStripTrailingHostDot: ptr.To(false),
			},
		},
		Gateway: nil,
		HTTPProxy: &contour_v1alpha1.HTTPProxyConfig{
			DisablePermitInsecure: ptr.To(false),
			FallbackCertificate:   nil,
		},
		EnableExternalNameService:   ptr.To(false),
		RateLimitService:            nil,
		GlobalExternalAuthorization: nil,
		Policy: &contour_v1alpha1.PolicyConfig{
			RequestHeadersPolicy:  &contour_v1alpha1.HeadersPolicy{},
			ResponseHeadersPolicy: &contour_v1alpha1.HeadersPolicy{},
			ApplyToIngress:        ptr.To(false),
		},
		Metrics: &contour_v1alpha1.MetricsConfig{
			Address: "0.0.0.0",
			Port:    8000,
		},
	}
}

func TestConvertServeContext(t *testing.T) {
	cases := map[string]struct {
		getServeContext         func(ctx *serveContext) *serveContext
		getContourConfiguration func(cfg contour_v1alpha1.ContourConfigurationSpec) contour_v1alpha1.ContourConfigurationSpec
	}{
		"default ServeContext": {
			getServeContext: func(ctx *serveContext) *serveContext {
				return ctx
			},
			getContourConfiguration: func(cfg contour_v1alpha1.ContourConfigurationSpec) contour_v1alpha1.ContourConfigurationSpec {
				return cfg
			},
		},
		"headers policy": {
			getServeContext: func(ctx *serveContext) *serveContext {
				ctx.Config.Policy = config.PolicyParameters{
					RequestHeadersPolicy: config.HeadersPolicy{
						Set:    map[string]string{"custom-request-header-set": "foo-bar", "Host": "request-bar.com"},
						Remove: []string{"custom-request-header-remove"},
					},
					ResponseHeadersPolicy: config.HeadersPolicy{
						Set:    map[string]string{"custom-response-header-set": "foo-bar", "Host": "response-bar.com"},
						Remove: []string{"custom-response-header-remove"},
					},
					ApplyToIngress: true,
				}
				return ctx
			},
			getContourConfiguration: func(cfg contour_v1alpha1.ContourConfigurationSpec) contour_v1alpha1.ContourConfigurationSpec {
				cfg.Policy = &contour_v1alpha1.PolicyConfig{
					RequestHeadersPolicy: &contour_v1alpha1.HeadersPolicy{
						Set:    map[string]string{"custom-request-header-set": "foo-bar", "Host": "request-bar.com"},
						Remove: []string{"custom-request-header-remove"},
					},
					ResponseHeadersPolicy: &contour_v1alpha1.HeadersPolicy{
						Set:    map[string]string{"custom-response-header-set": "foo-bar", "Host": "response-bar.com"},
						Remove: []string{"custom-response-header-remove"},
					},
					ApplyToIngress: ptr.To(true),
				}
				return cfg
			},
		},
		"ingress": {
			getServeContext: func(ctx *serveContext) *serveContext {
				ctx.ingressClassName = "coolclass"
				ctx.Config.IngressStatusAddress = "1.2.3.4"
				return ctx
			},
			getContourConfiguration: func(cfg contour_v1alpha1.ContourConfigurationSpec) contour_v1alpha1.ContourConfigurationSpec {
				cfg.Ingress = &contour_v1alpha1.IngressConfig{
					ClassNames:    []string{"coolclass"},
					StatusAddress: "1.2.3.4",
				}
				return cfg
			},
		},
		"gatewayapi": {
			getServeContext: func(ctx *serveContext) *serveContext {
				ctx.Config.GatewayConfig = &config.GatewayParameters{
					GatewayRef: config.NamespacedName{
						Namespace: "gateway-namespace",
						Name:      "gateway-name",
					},
				}
				return ctx
			},
			getContourConfiguration: func(cfg contour_v1alpha1.ContourConfigurationSpec) contour_v1alpha1.ContourConfigurationSpec {
				cfg.Gateway = &contour_v1alpha1.GatewayConfig{
					GatewayRef: contour_v1alpha1.NamespacedName{
						Namespace: "gateway-namespace",
						Name:      "gateway-name",
					},
				}
				return cfg
			},
		},
		"client certificate": {
			getServeContext: func(ctx *serveContext) *serveContext {
				ctx.Config.TLS.ClientCertificate = config.NamespacedName{
					Name:      "cert",
					Namespace: "secretplace",
				}
				return ctx
			},
			getContourConfiguration: func(cfg contour_v1alpha1.ContourConfigurationSpec) contour_v1alpha1.ContourConfigurationSpec {
				cfg.Envoy.ClientCertificate = &contour_v1alpha1.NamespacedName{
					Name:      "cert",
					Namespace: "secretplace",
				}
				return cfg
			},
		},
		"httpproxy": {
			getServeContext: func(ctx *serveContext) *serveContext {
				ctx.Config.DisablePermitInsecure = true
				ctx.Config.TLS.FallbackCertificate = config.NamespacedName{
					Name:      "fallbackname",
					Namespace: "fallbacknamespace",
				}
				return ctx
			},
			getContourConfiguration: func(cfg contour_v1alpha1.ContourConfigurationSpec) contour_v1alpha1.ContourConfigurationSpec {
				cfg.HTTPProxy = &contour_v1alpha1.HTTPProxyConfig{
					DisablePermitInsecure: ptr.To(true),
					FallbackCertificate: &contour_v1alpha1.NamespacedName{
						Name:      "fallbackname",
						Namespace: "fallbacknamespace",
					},
				}
				return cfg
			},
		},
		"ratelimit": {
			getServeContext: func(ctx *serveContext) *serveContext {
				ctx.Config.RateLimitService = config.RateLimitService{
					ExtensionService:            "ratens/ratelimitext",
					Domain:                      "contour",
					FailOpen:                    true,
					EnableXRateLimitHeaders:     true,
					EnableResourceExhaustedCode: true,
					DefaultGlobalRateLimitPolicy: &contour_v1.GlobalRateLimitPolicy{
						Descriptors: []contour_v1.RateLimitDescriptor{
							{
								Entries: []contour_v1.RateLimitDescriptorEntry{
									{
										GenericKey: &contour_v1.GenericKeyDescriptor{
											Key:   "foo",
											Value: "bar",
										},
									},
								},
							},
						},
					},
				}
				return ctx
			},
			getContourConfiguration: func(cfg contour_v1alpha1.ContourConfigurationSpec) contour_v1alpha1.ContourConfigurationSpec {
				cfg.RateLimitService = &contour_v1alpha1.RateLimitServiceConfig{
					ExtensionService: contour_v1alpha1.NamespacedName{
						Name:      "ratelimitext",
						Namespace: "ratens",
					},
					Domain:                      "contour",
					FailOpen:                    ptr.To(true),
					EnableXRateLimitHeaders:     ptr.To(true),
					EnableResourceExhaustedCode: ptr.To(true),
					DefaultGlobalRateLimitPolicy: &contour_v1.GlobalRateLimitPolicy{
						Descriptors: []contour_v1.RateLimitDescriptor{
							{
								Entries: []contour_v1.RateLimitDescriptorEntry{
									{
										GenericKey: &contour_v1.GenericKeyDescriptor{
											Key:   "foo",
											Value: "bar",
										},
									},
								},
							},
						},
					},
				}
				return cfg
			},
		},
		"default http versions": {
			getServeContext: func(ctx *serveContext) *serveContext {
				ctx.Config.DefaultHTTPVersions = []config.HTTPVersionType{
					config.HTTPVersion1,
				}
				return ctx
			},
			getContourConfiguration: func(cfg contour_v1alpha1.ContourConfigurationSpec) contour_v1alpha1.ContourConfigurationSpec {
				cfg.Envoy.DefaultHTTPVersions = []contour_v1alpha1.HTTPVersionType{
					contour_v1alpha1.HTTPVersion1,
				}
				return cfg
			},
		},
		"access log": {
			getServeContext: func(ctx *serveContext) *serveContext {
				ctx.Config.AccessLogFormat = config.JSONAccessLog
				ctx.Config.AccessLogFormatString = "foo-bar-baz"
				ctx.Config.AccessLogFields = []string{"custom_field"}
				return ctx
			},
			getContourConfiguration: func(cfg contour_v1alpha1.ContourConfigurationSpec) contour_v1alpha1.ContourConfigurationSpec {
				cfg.Envoy.Logging = &contour_v1alpha1.EnvoyLogging{
					AccessLogFormat:       contour_v1alpha1.JSONAccessLog,
					AccessLogFormatString: "foo-bar-baz",
					AccessLogLevel:        contour_v1alpha1.LogLevelInfo,
					AccessLogJSONFields: contour_v1alpha1.AccessLogJSONFields([]string{
						"custom_field",
					}),
				}
				return cfg
			},
		},
		"access log -- error": {
			getServeContext: func(ctx *serveContext) *serveContext {
				ctx.Config.AccessLogFormat = config.JSONAccessLog
				ctx.Config.AccessLogFormatString = "foo-bar-baz"
				ctx.Config.AccessLogFields = []string{"custom_field"}
				ctx.Config.AccessLogLevel = config.LogLevelError
				return ctx
			},
			getContourConfiguration: func(cfg contour_v1alpha1.ContourConfigurationSpec) contour_v1alpha1.ContourConfigurationSpec {
				cfg.Envoy.Logging = &contour_v1alpha1.EnvoyLogging{
					AccessLogFormat:       contour_v1alpha1.JSONAccessLog,
					AccessLogFormatString: "foo-bar-baz",
					AccessLogLevel:        contour_v1alpha1.LogLevelError,
					AccessLogJSONFields: contour_v1alpha1.AccessLogJSONFields([]string{
						"custom_field",
					}),
				}
				return cfg
			},
		},
		"access log -- critical": {
			getServeContext: func(ctx *serveContext) *serveContext {
				ctx.Config.AccessLogFormat = config.JSONAccessLog
				ctx.Config.AccessLogFormatString = "foo-bar-baz"
				ctx.Config.AccessLogFields = []string{"custom_field"}
				ctx.Config.AccessLogLevel = config.LogLevelCritical
				return ctx
			},
			getContourConfiguration: func(cfg contour_v1alpha1.ContourConfigurationSpec) contour_v1alpha1.ContourConfigurationSpec {
				cfg.Envoy.Logging = &contour_v1alpha1.EnvoyLogging{
					AccessLogFormat:       contour_v1alpha1.JSONAccessLog,
					AccessLogFormatString: "foo-bar-baz",
					AccessLogLevel:        contour_v1alpha1.LogLevelCritical,
					AccessLogJSONFields: contour_v1alpha1.AccessLogJSONFields([]string{
						"custom_field",
					}),
				}
				return cfg
			},
		},
		"disable merge slashes": {
			getServeContext: func(ctx *serveContext) *serveContext {
				ctx.Config.DisableMergeSlashes = true
				return ctx
			},
			getContourConfiguration: func(cfg contour_v1alpha1.ContourConfigurationSpec) contour_v1alpha1.ContourConfigurationSpec {
				cfg.Envoy.Listener.DisableMergeSlashes = ptr.To(true)
				return cfg
			},
		},
		"server header transformation": {
			getServeContext: func(ctx *serveContext) *serveContext {
				ctx.Config.ServerHeaderTransformation = config.AppendIfAbsentServerHeader
				return ctx
			},
			getContourConfiguration: func(cfg contour_v1alpha1.ContourConfigurationSpec) contour_v1alpha1.ContourConfigurationSpec {
				cfg.Envoy.Listener.ServerHeaderTransformation = contour_v1alpha1.AppendIfAbsentServerHeader
				return cfg
			},
		},
		"global circuit breaker defaults": {
			getServeContext: func(ctx *serveContext) *serveContext {
				ctx.Config.Cluster.GlobalCircuitBreakerDefaults = &contour_v1alpha1.CircuitBreakers{
					MaxConnections:     4,
					MaxPendingRequests: 5,
					MaxRequests:        6,
					MaxRetries:         7,
				}
				return ctx
			},
			getContourConfiguration: func(cfg contour_v1alpha1.ContourConfigurationSpec) contour_v1alpha1.ContourConfigurationSpec {
				cfg.Envoy.Cluster.GlobalCircuitBreakerDefaults = &contour_v1alpha1.CircuitBreakers{
					MaxConnections:     4,
					MaxPendingRequests: 5,
					MaxRequests:        6,
					MaxRetries:         7,
				}
				return cfg
			},
		},
		"global external authorization": {
			getServeContext: func(ctx *serveContext) *serveContext {
				ctx.Config.GlobalExternalAuthorization = config.GlobalExternalAuthorization{
					ExtensionService: "extauthns/extauthtext",
					FailOpen:         true,
					AuthPolicy: &config.GlobalAuthorizationPolicy{
						Context: map[string]string{
							"foo": "bar",
						},
					},
					WithRequestBody: &config.GlobalAuthorizationServerBufferSettings{
						MaxRequestBytes:     512,
						PackAsBytes:         true,
						AllowPartialMessage: true,
					},
				}
				return ctx
			},
			getContourConfiguration: func(cfg contour_v1alpha1.ContourConfigurationSpec) contour_v1alpha1.ContourConfigurationSpec {
				cfg.GlobalExternalAuthorization = &contour_v1.AuthorizationServer{
					ExtensionServiceRef: contour_v1.ExtensionServiceReference{
						Name:      "extauthtext",
						Namespace: "extauthns",
					},
					FailOpen: true,
					AuthPolicy: &contour_v1.AuthorizationPolicy{
						Context: map[string]string{
							"foo": "bar",
						},
						Disabled: false,
					},
					WithRequestBody: &contour_v1.AuthorizationServerBufferSettings{
						MaxRequestBytes:     512,
						PackAsBytes:         true,
						AllowPartialMessage: true,
					},
				}
				return cfg
			},
		},
		"tracing config normal": {
			getServeContext: func(ctx *serveContext) *serveContext {
				ctx.Config.Tracing = &config.Tracing{
					IncludePodDetail: ptr.To(false),
					ServiceName:      ptr.To("contour"),
					OverallSampling:  ptr.To("100"),
					MaxPathTagLength: ptr.To(uint32(256)),
					CustomTags: []config.CustomTag{
						{
							TagName: "literal",
							Literal: "this is literal",
						},
						{
							TagName:           "header",
							RequestHeaderName: ":method",
						},
					},
					ExtensionService: "otel/otel-collector",
				}
				return ctx
			},
			getContourConfiguration: func(cfg contour_v1alpha1.ContourConfigurationSpec) contour_v1alpha1.ContourConfigurationSpec {
				cfg.Tracing = &contour_v1alpha1.TracingConfig{
					IncludePodDetail: ptr.To(false),
					ServiceName:      ptr.To("contour"),
					OverallSampling:  ptr.To("100"),
					MaxPathTagLength: ptr.To(uint32(256)),
					CustomTags: []*contour_v1alpha1.CustomTag{
						{
							TagName: "literal",
							Literal: "this is literal",
						},
						{
							TagName:           "header",
							RequestHeaderName: ":method",
						},
					},
					ExtensionService: &contour_v1alpha1.NamespacedName{
						Name:      "otel-collector",
						Namespace: "otel",
					},
				}
				return cfg
			},
		},
		"tracing config only extensionService": {
			getServeContext: func(ctx *serveContext) *serveContext {
				ctx.Config.Tracing = &config.Tracing{
					ExtensionService: "otel/otel-collector",
				}
				return ctx
			},
			getContourConfiguration: func(cfg contour_v1alpha1.ContourConfigurationSpec) contour_v1alpha1.ContourConfigurationSpec {
				cfg.Tracing = &contour_v1alpha1.TracingConfig{
					ExtensionService: &contour_v1alpha1.NamespacedName{
						Name:      "otel-collector",
						Namespace: "otel",
					},
				}
				return cfg
			},
		},
		"envoy listener settings": {
			getServeContext: func(ctx *serveContext) *serveContext {
				ctx.Config.Listener.MaxRequestsPerIOCycle = ptr.To(uint32(10))
				ctx.Config.Listener.HTTP2MaxConcurrentStreams = ptr.To(uint32(30))
				ctx.Config.Listener.MaxConnectionsPerListener = ptr.To(uint32(50))
				return ctx
			},
			getContourConfiguration: func(cfg contour_v1alpha1.ContourConfigurationSpec) contour_v1alpha1.ContourConfigurationSpec {
				cfg.Envoy.Listener.MaxRequestsPerIOCycle = ptr.To(uint32(10))
				cfg.Envoy.Listener.HTTP2MaxConcurrentStreams = ptr.To(uint32(30))
				cfg.Envoy.Listener.MaxConnectionsPerListener = ptr.To(uint32(50))
				return cfg
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			serveContext := tc.getServeContext(defaultContext())
			want := tc.getContourConfiguration(defaultContourConfiguration())

			assert.Equal(t, want, serveContext.convertToContourConfigurationSpec())
		})
	}
}

func TestServeContextCompressionOptions(t *testing.T) {
	cases := map[string]struct {
		serveCompression  config.CompressionAlgorithm
		configCompression contour_v1alpha1.CompressionAlgorithm
	}{
		"Brotli":   {config.CompressionBrotli, contour_v1alpha1.BrotliCompression},
		"Disabled": {config.CompressionDisabled, contour_v1alpha1.DisabledCompression},
		"Gzip":     {config.CompressionGzip, contour_v1alpha1.GzipCompression},
		"Zstd":     {config.CompressionZstd, contour_v1alpha1.ZstdCompression},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			testServeContext := defaultContext()
			testServeContext.Config.Compression.Algorithm = tc.serveCompression

			want := defaultContourConfiguration()
			want.Envoy.Listener.Compression = &contour_v1alpha1.EnvoyCompression{
				Algorithm: tc.configCompression,
			}

			assert.Equal(t, want, testServeContext.convertToContourConfigurationSpec())
		})
	}
}
