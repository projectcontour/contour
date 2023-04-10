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
	"crypto/tls"
	"crypto/x509"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/projectcontour/contour/pkg/config"
	"github.com/tsaarni/certyaml"

	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	contour_api_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/contourconfig"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/fixture"
	"github.com/projectcontour/contour/internal/ref"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
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
		tls         *contour_api_v1alpha1.TLS
		expectError bool
	}{
		"tls supplied correctly": {
			tls: &contour_api_v1alpha1.TLS{
				CAFile:   "cacert.pem",
				CertFile: "contourcert.pem",
				KeyFile:  "contourkey.pem",
				Insecure: ref.To(false),
			},
			expectError: false,
		},
		"tls partially supplied": {
			tls: &contour_api_v1alpha1.TLS{
				CertFile: "contourcert.pem",
				KeyFile:  "contourkey.pem",
				Insecure: ref.To(false),
			},
			expectError: true,
		},
		"tls not supplied": {
			tls:         &contour_api_v1alpha1.TLS{},
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
	checkFatalErr(t, err)
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
	checkFatalErr(t, err)
	defer os.RemoveAll(configDir)

	contourTLS := &contour_api_v1alpha1.TLS{
		CAFile:   filepath.Join(configDir, "CAcert.pem"),
		CertFile: filepath.Join(configDir, "contourcert.pem"),
		KeyFile:  filepath.Join(configDir, "contourkey.pem"),
		Insecure: ref.To(false),
	}

	// Initial set of credentials must be written into temp directory before
	// starting the tests to avoid error at server startup.
	err = trustedCACert.WritePEM(contourTLS.CAFile, filepath.Join(configDir, "CAkey.pem"))
	checkFatalErr(t, err)
	err = contourCertBeforeRotation.WritePEM(contourTLS.CertFile, contourTLS.KeyFile)
	checkFatalErr(t, err)

	// Start a dummy server.
	log := fixture.NewTestLogger(t)
	opts := grpcOptions(log, contourTLS)
	g := grpc.NewServer(opts...)
	if g == nil {
		t.Error("failed to create server")
	}

	address := "localhost:8001"
	l, err := net.Listen("tcp", address)
	checkFatalErr(t, err)

	go func() {
		err := g.Serve(l)
		checkFatalErr(t, err)
	}()
	defer g.GracefulStop()

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Store certificate and key to temp dir used by serveContext.
			err = tc.serverCredentials.WritePEM(contourTLS.CertFile, contourTLS.KeyFile)
			checkFatalErr(t, err)
			clientCert, _ := tc.clientCredentials.TLSCertificate()
			receivedCert, err := tryConnect(address, clientCert, caCertPool)
			gotError := err != nil
			if gotError != tc.expectError {
				t.Errorf("Unexpected result when connecting to the server: %s", err)
			}
			if err == nil {
				expectedCert, _ := tc.serverCredentials.X509Certificate()
				assert.Equal(t, receivedCert, &expectedCert)
			}
		})
	}
}

func TestTlsVersionDeprecation(t *testing.T) {
	// To get tls.Config for the gRPC XDS server, we need to arrange valid TLS certificates and keys.
	// Create temporary directory to store them for the server.
	configDir, err := os.MkdirTemp("", "contour-testdata-")
	checkFatalErr(t, err)
	defer os.RemoveAll(configDir)

	caCert := certyaml.Certificate{
		Subject: "cn=ca",
	}
	contourCert := certyaml.Certificate{
		Subject: "cn=contourBeforeRotation",
		Issuer:  &caCert,
	}

	contourTLS := &contour_api_v1alpha1.TLS{
		CAFile:   filepath.Join(configDir, "CAcert.pem"),
		CertFile: filepath.Join(configDir, "contourcert.pem"),
		KeyFile:  filepath.Join(configDir, "contourkey.pem"),
		Insecure: ref.To(false),
	}

	err = caCert.WritePEM(contourTLS.CAFile, filepath.Join(configDir, "CAkey.pem"))
	checkFatalErr(t, err)
	err = contourCert.WritePEM(contourTLS.CertFile, contourTLS.KeyFile)
	checkFatalErr(t, err)

	// Get preliminary TLS config from the serveContext.
	log := fixture.NewTestLogger(t)
	preliminaryTLSConfig := tlsconfig(log, contourTLS)

	// Get actual TLS config that will be used during TLS handshake.
	tlsConfig, err := preliminaryTLSConfig.GetConfigForClient(nil)
	checkFatalErr(t, err)

	assert.Equal(t, tlsConfig.MinVersion, uint16(tls.VersionTLS13))
}

func checkFatalErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// tryConnect tries to establish TLS connection to the server.
// If successful, return the server certificate.
func tryConnect(address string, clientCert tls.Certificate, caCertPool *x509.CertPool) (*x509.Certificate, error) {
	clientConfig := &tls.Config{
		ServerName:   "localhost",
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{clientCert},
		RootCAs:      caCertPool,
	}
	conn, err := tls.Dial("tcp", address, clientConfig)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	err = peekError(conn)
	if err != nil {
		return nil, err
	}

	return conn.ConnectionState().PeerCertificates[0], nil
}

// peekError is a workaround for TLS 1.3: due to shortened handshake, TLS alert
// from server is received at first read from the socket.
// To receive alert for bad certificate, this function tries to read one byte.
// Adapted from https://golang.org/src/crypto/tls/handshake_client_test.go
func peekError(conn net.Conn) error {
	_ = conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	_, err := conn.Read(make([]byte, 1))
	if err != nil {
		if netErr, ok := err.(net.Error); !ok || !netErr.Timeout() {
			return err
		}
	}
	return nil
}

func TestParseHTTPVersions(t *testing.T) {
	cases := map[string]struct {
		versions      []contour_api_v1alpha1.HTTPVersionType
		parseVersions []envoy_v3.HTTPVersionType
	}{
		"empty": {
			versions:      []contour_api_v1alpha1.HTTPVersionType{},
			parseVersions: nil,
		},
		"http/1.1": {
			versions:      []contour_api_v1alpha1.HTTPVersionType{contour_api_v1alpha1.HTTPVersion1},
			parseVersions: []envoy_v3.HTTPVersionType{envoy_v3.HTTPVersion1},
		},
		"http/1.1+http/2": {
			versions:      []contour_api_v1alpha1.HTTPVersionType{contour_api_v1alpha1.HTTPVersion1, contour_api_v1alpha1.HTTPVersion2},
			parseVersions: []envoy_v3.HTTPVersionType{envoy_v3.HTTPVersion1, envoy_v3.HTTPVersion2},
		},
		"http/1.1+http/2 duplicated": {
			versions: []contour_api_v1alpha1.HTTPVersionType{
				contour_api_v1alpha1.HTTPVersion1, contour_api_v1alpha1.HTTPVersion2,
				contour_api_v1alpha1.HTTPVersion1, contour_api_v1alpha1.HTTPVersion2},
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

func TestConvertServeContext(t *testing.T) {
	defaultContext := func() *serveContext {
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

	defaultContourConfiguration := func() contour_api_v1alpha1.ContourConfigurationSpec {
		return contour_api_v1alpha1.ContourConfigurationSpec{
			XDSServer: &contour_api_v1alpha1.XDSServerConfig{
				Type:    contour_api_v1alpha1.ContourServerType,
				Address: "127.0.0.1",
				Port:    8001,
				TLS: &contour_api_v1alpha1.TLS{
					CAFile:   "/certs/ca.crt",
					CertFile: "/certs/cert.crt",
					KeyFile:  "/certs/cert.key",
					Insecure: ref.To(false),
				},
			},
			Ingress: &contour_api_v1alpha1.IngressConfig{
				ClassNames:    nil,
				StatusAddress: "",
			},
			Debug: &contour_api_v1alpha1.DebugConfig{
				Address: "127.0.0.1",
				Port:    6060,
			},
			Health: &contour_api_v1alpha1.HealthConfig{
				Address: "0.0.0.0",
				Port:    8000,
			},
			Envoy: &contour_api_v1alpha1.EnvoyConfig{
				Service: &contour_api_v1alpha1.NamespacedName{
					Name:      "envoy",
					Namespace: "projectcontour",
				},
				Listener: &contour_api_v1alpha1.EnvoyListenerConfig{
					UseProxyProto:              ref.To(false),
					DisableAllowChunkedLength:  ref.To(false),
					DisableMergeSlashes:        ref.To(false),
					ServerHeaderTransformation: contour_api_v1alpha1.OverwriteServerHeader,
					TLS: &contour_api_v1alpha1.EnvoyTLS{
						MinimumProtocolVersion: "",
					},
				},
				HTTPListener: &contour_api_v1alpha1.EnvoyListener{
					Address:   "0.0.0.0",
					Port:      8080,
					AccessLog: "/dev/stdout",
				},
				HTTPSListener: &contour_api_v1alpha1.EnvoyListener{
					Address:   "0.0.0.0",
					Port:      8443,
					AccessLog: "/dev/stdout",
				},
				Health: &contour_api_v1alpha1.HealthConfig{
					Address: "0.0.0.0",
					Port:    8002,
				},
				Metrics: &contour_api_v1alpha1.MetricsConfig{
					Address: "0.0.0.0",
					Port:    8002,
				},
				ClientCertificate: nil,
				Logging: &contour_api_v1alpha1.EnvoyLogging{
					AccessLogFormat:       contour_api_v1alpha1.EnvoyAccessLog,
					AccessLogFormatString: "",
					AccessLogLevel:        contour_api_v1alpha1.LogLevelInfo,
					AccessLogJSONFields: contour_api_v1alpha1.AccessLogJSONFields([]string{
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
				Timeouts: &contour_api_v1alpha1.TimeoutParameters{
					ConnectionIdleTimeout: ref.To("60s"),
					ConnectTimeout:        ref.To("2s"),
				},
				Cluster: &contour_api_v1alpha1.ClusterParameters{
					DNSLookupFamily: contour_api_v1alpha1.AutoClusterDNSFamily,
				},
				Network: &contour_api_v1alpha1.NetworkParameters{
					EnvoyAdminPort:    ref.To(9001),
					XffNumTrustedHops: contourconfig.UInt32Ptr(0),
				},
			},
			Gateway: nil,
			HTTPProxy: &contour_api_v1alpha1.HTTPProxyConfig{
				DisablePermitInsecure: ref.To(false),
				FallbackCertificate:   nil,
			},
			EnableExternalNameService:   ref.To(false),
			RateLimitService:            nil,
			GlobalExternalAuthorization: nil,
			Policy: &contour_api_v1alpha1.PolicyConfig{
				RequestHeadersPolicy:  &contour_api_v1alpha1.HeadersPolicy{},
				ResponseHeadersPolicy: &contour_api_v1alpha1.HeadersPolicy{},
				ApplyToIngress:        ref.To(false),
			},
			Metrics: &contour_api_v1alpha1.MetricsConfig{
				Address: "0.0.0.0",
				Port:    8000,
			},
		}
	}

	cases := map[string]struct {
		getServeContext         func(ctx *serveContext) *serveContext
		getContourConfiguration func(cfg contour_api_v1alpha1.ContourConfigurationSpec) contour_api_v1alpha1.ContourConfigurationSpec
	}{
		"default ServeContext": {
			getServeContext: func(ctx *serveContext) *serveContext {
				return ctx
			},
			getContourConfiguration: func(cfg contour_api_v1alpha1.ContourConfigurationSpec) contour_api_v1alpha1.ContourConfigurationSpec {
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
			getContourConfiguration: func(cfg contour_api_v1alpha1.ContourConfigurationSpec) contour_api_v1alpha1.ContourConfigurationSpec {
				cfg.Policy = &contour_api_v1alpha1.PolicyConfig{
					RequestHeadersPolicy: &contour_api_v1alpha1.HeadersPolicy{
						Set:    map[string]string{"custom-request-header-set": "foo-bar", "Host": "request-bar.com"},
						Remove: []string{"custom-request-header-remove"},
					},
					ResponseHeadersPolicy: &contour_api_v1alpha1.HeadersPolicy{
						Set:    map[string]string{"custom-response-header-set": "foo-bar", "Host": "response-bar.com"},
						Remove: []string{"custom-response-header-remove"},
					},
					ApplyToIngress: ref.To(true),
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
			getContourConfiguration: func(cfg contour_api_v1alpha1.ContourConfigurationSpec) contour_api_v1alpha1.ContourConfigurationSpec {
				cfg.Ingress = &contour_api_v1alpha1.IngressConfig{
					ClassNames:    []string{"coolclass"},
					StatusAddress: "1.2.3.4",
				}
				return cfg
			},
		},
		"gatewayapi - controller": {
			getServeContext: func(ctx *serveContext) *serveContext {
				ctx.Config.GatewayConfig = &config.GatewayParameters{
					ControllerName: "projectcontour.io/gateway-controller",
				}
				return ctx
			},
			getContourConfiguration: func(cfg contour_api_v1alpha1.ContourConfigurationSpec) contour_api_v1alpha1.ContourConfigurationSpec {
				cfg.Gateway = &contour_api_v1alpha1.GatewayConfig{
					ControllerName: "projectcontour.io/gateway-controller",
				}
				return cfg
			},
		},
		"gatewayapi - specific gateway": {
			getServeContext: func(ctx *serveContext) *serveContext {
				ctx.Config.GatewayConfig = &config.GatewayParameters{
					GatewayRef: &config.NamespacedName{
						Namespace: "gateway-namespace",
						Name:      "gateway-name",
					},
				}
				return ctx
			},
			getContourConfiguration: func(cfg contour_api_v1alpha1.ContourConfigurationSpec) contour_api_v1alpha1.ContourConfigurationSpec {
				cfg.Gateway = &contour_api_v1alpha1.GatewayConfig{
					GatewayRef: &contour_api_v1alpha1.NamespacedName{
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
			getContourConfiguration: func(cfg contour_api_v1alpha1.ContourConfigurationSpec) contour_api_v1alpha1.ContourConfigurationSpec {
				cfg.Envoy.ClientCertificate = &contour_api_v1alpha1.NamespacedName{
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
			getContourConfiguration: func(cfg contour_api_v1alpha1.ContourConfigurationSpec) contour_api_v1alpha1.ContourConfigurationSpec {
				cfg.HTTPProxy = &contour_api_v1alpha1.HTTPProxyConfig{
					DisablePermitInsecure: ref.To(true),
					FallbackCertificate: &contour_api_v1alpha1.NamespacedName{
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
				}
				return ctx
			},
			getContourConfiguration: func(cfg contour_api_v1alpha1.ContourConfigurationSpec) contour_api_v1alpha1.ContourConfigurationSpec {
				cfg.RateLimitService = &contour_api_v1alpha1.RateLimitServiceConfig{
					ExtensionService: contour_api_v1alpha1.NamespacedName{
						Name:      "ratelimitext",
						Namespace: "ratens",
					},
					Domain:                      "contour",
					FailOpen:                    ref.To(true),
					EnableXRateLimitHeaders:     ref.To(true),
					EnableResourceExhaustedCode: ref.To(true),
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
			getContourConfiguration: func(cfg contour_api_v1alpha1.ContourConfigurationSpec) contour_api_v1alpha1.ContourConfigurationSpec {
				cfg.Envoy.DefaultHTTPVersions = []contour_api_v1alpha1.HTTPVersionType{
					contour_api_v1alpha1.HTTPVersion1,
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
			getContourConfiguration: func(cfg contour_api_v1alpha1.ContourConfigurationSpec) contour_api_v1alpha1.ContourConfigurationSpec {
				cfg.Envoy.Logging = &contour_api_v1alpha1.EnvoyLogging{
					AccessLogFormat:       contour_api_v1alpha1.JSONAccessLog,
					AccessLogFormatString: "foo-bar-baz",
					AccessLogLevel:        contour_api_v1alpha1.LogLevelInfo,
					AccessLogJSONFields: contour_api_v1alpha1.AccessLogJSONFields([]string{
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
			getContourConfiguration: func(cfg contour_api_v1alpha1.ContourConfigurationSpec) contour_api_v1alpha1.ContourConfigurationSpec {
				cfg.Envoy.Listener.DisableMergeSlashes = ref.To(true)
				return cfg
			},
		},
		"server header transformation": {
			getServeContext: func(ctx *serveContext) *serveContext {
				ctx.Config.ServerHeaderTransformation = config.AppendIfAbsentServerHeader
				return ctx
			},
			getContourConfiguration: func(cfg contour_api_v1alpha1.ContourConfigurationSpec) contour_api_v1alpha1.ContourConfigurationSpec {
				cfg.Envoy.Listener.ServerHeaderTransformation = contour_api_v1alpha1.AppendIfAbsentServerHeader
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
			getContourConfiguration: func(cfg contour_api_v1alpha1.ContourConfigurationSpec) contour_api_v1alpha1.ContourConfigurationSpec {
				cfg.GlobalExternalAuthorization = &contour_api_v1.AuthorizationServer{
					ExtensionServiceRef: contour_api_v1.ExtensionServiceReference{
						Name:      "extauthtext",
						Namespace: "extauthns",
					},
					FailOpen: true,
					AuthPolicy: &contour_api_v1.AuthorizationPolicy{
						Context: map[string]string{
							"foo": "bar",
						},
						Disabled: false,
					},
					WithRequestBody: &contour_api_v1.AuthorizationServerBufferSettings{
						MaxRequestBytes:     512,
						PackAsBytes:         true,
						AllowPartialMessage: true,
					},
				}
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
