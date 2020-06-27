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

package envoy

import (
	"testing"
	"time"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_api_v2_auth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	envoy_api_v2_core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoy_api_v2_listener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	envoy_api_v2_accesslog "github.com/envoyproxy/go-control-plane/envoy/config/filter/accesslog/v2"
	http "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	envoy_config_v2_tcpproxy "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/tcp_proxy/v2"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/google/go-cmp/cmp"
	"github.com/projectcontour/contour/internal/assert"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/protobuf"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestListener(t *testing.T) {
	tests := map[string]struct {
		name, address string
		port          int
		lf            []*envoy_api_v2_listener.ListenerFilter
		f             []*envoy_api_v2_listener.Filter
		want          *v2.Listener
	}{
		"insecure listener": {
			name:    "http",
			address: "0.0.0.0",
			port:    9000,
			f: []*envoy_api_v2_listener.Filter{
				HTTPConnectionManager("http", FileAccessLogEnvoy("/dev/null"), 0),
			},
			want: &v2.Listener{
				Name:    "http",
				Address: SocketAddress("0.0.0.0", 9000),
				FilterChains: FilterChains(
					HTTPConnectionManager("http", FileAccessLogEnvoy("/dev/null"), 0),
				),
				SocketOptions: TCPKeepaliveSocketOptions(),
			},
		},
		"insecure listener w/ proxy": {
			name:    "http-proxy",
			address: "0.0.0.0",
			port:    9000,
			lf: []*envoy_api_v2_listener.ListenerFilter{
				ProxyProtocol(),
			},
			f: []*envoy_api_v2_listener.Filter{
				HTTPConnectionManager("http-proxy", FileAccessLogEnvoy("/dev/null"), 0),
			},
			want: &v2.Listener{
				Name:    "http-proxy",
				Address: SocketAddress("0.0.0.0", 9000),
				ListenerFilters: ListenerFilters(
					ProxyProtocol(),
				),
				FilterChains: FilterChains(
					HTTPConnectionManager("http-proxy", FileAccessLogEnvoy("/dev/null"), 0),
				),
				SocketOptions: TCPKeepaliveSocketOptions(),
			},
		},
		"secure listener": {
			name:    "https",
			address: "0.0.0.0",
			port:    9000,
			lf: ListenerFilters(
				TLSInspector(),
			),
			want: &v2.Listener{
				Name:    "https",
				Address: SocketAddress("0.0.0.0", 9000),
				ListenerFilters: ListenerFilters(
					TLSInspector(),
				),
				SocketOptions: TCPKeepaliveSocketOptions(),
			},
		},
		"secure listener w/ proxy": {
			name:    "https-proxy",
			address: "0.0.0.0",
			port:    9000,
			lf: ListenerFilters(
				ProxyProtocol(),
				TLSInspector(),
			),
			want: &v2.Listener{
				Name:    "https-proxy",
				Address: SocketAddress("0.0.0.0", 9000),
				ListenerFilters: ListenerFilters(
					ProxyProtocol(),
					TLSInspector(),
				),
				SocketOptions: TCPKeepaliveSocketOptions(),
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := Listener(tc.name, tc.address, tc.port, tc.lf, tc.f...)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestSocketAddress(t *testing.T) {
	const (
		addr = "foo.example.com"
		port = 8123
	)

	got := SocketAddress(addr, port)
	want := &envoy_api_v2_core.Address{
		Address: &envoy_api_v2_core.Address_SocketAddress{
			SocketAddress: &envoy_api_v2_core.SocketAddress{
				Protocol: envoy_api_v2_core.SocketAddress_TCP,
				Address:  addr,
				PortSpecifier: &envoy_api_v2_core.SocketAddress_PortValue{
					PortValue: port,
				},
			},
		},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatal(diff)
	}

	got = SocketAddress("::", port)
	want = &envoy_api_v2_core.Address{
		Address: &envoy_api_v2_core.Address_SocketAddress{
			SocketAddress: &envoy_api_v2_core.SocketAddress{
				Protocol:   envoy_api_v2_core.SocketAddress_TCP,
				Address:    "::",
				Ipv4Compat: true, // Set only for ipv6-any "::"
				PortSpecifier: &envoy_api_v2_core.SocketAddress_PortValue{
					PortValue: port,
				},
			},
		},
	}
	assert.Equal(t, want, got)
}

func TestDownstreamTLSContext(t *testing.T) {
	const subjectName = "client-subject-name"
	ca := []byte("client-ca-cert")

	serverSecret := &dag.Secret{
		Object: &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "tls-cert",
				Namespace: "default",
			},
			Data: map[string][]byte{
				v1.TLSCertKey:       []byte("cert"),
				v1.TLSPrivateKeyKey: []byte("key"),
			},
		},
	}

	tlsParams := &envoy_api_v2_auth.TlsParameters{
		TlsMinimumProtocolVersion: envoy_api_v2_auth.TlsParameters_TLSv1_1,
		TlsMaximumProtocolVersion: envoy_api_v2_auth.TlsParameters_TLSv1_3,
		CipherSuites: []string{
			"[ECDHE-ECDSA-AES128-GCM-SHA256|ECDHE-ECDSA-CHACHA20-POLY1305]",
			"[ECDHE-RSA-AES128-GCM-SHA256|ECDHE-RSA-CHACHA20-POLY1305]",
			"ECDHE-ECDSA-AES128-SHA",
			"ECDHE-RSA-AES128-SHA",
			"ECDHE-ECDSA-AES256-GCM-SHA384",
			"ECDHE-RSA-AES256-GCM-SHA384",
			"ECDHE-ECDSA-AES256-SHA",
			"ECDHE-RSA-AES256-SHA",
		},
	}

	tlsCertificateSdsSecretConfigs := []*envoy_api_v2_auth.SdsSecretConfig{{
		Name: Secretname(serverSecret),
		SdsConfig: &envoy_api_v2_core.ConfigSource{
			ConfigSourceSpecifier: &envoy_api_v2_core.ConfigSource_ApiConfigSource{
				ApiConfigSource: &envoy_api_v2_core.ApiConfigSource{
					ApiType: envoy_api_v2_core.ApiConfigSource_GRPC,
					GrpcServices: []*envoy_api_v2_core.GrpcService{{
						TargetSpecifier: &envoy_api_v2_core.GrpcService_EnvoyGrpc_{
							EnvoyGrpc: &envoy_api_v2_core.GrpcService_EnvoyGrpc{
								ClusterName: "contour",
							},
						},
					}},
				},
			},
		},
	}}

	alpnProtocols := []string{"h2", "http/1.1"}
	validationContext := &envoy_api_v2_auth.CommonTlsContext_ValidationContext{
		ValidationContext: &envoy_api_v2_auth.CertificateValidationContext{
			TrustedCa: &envoy_api_v2_core.DataSource{
				Specifier: &envoy_api_v2_core.DataSource_InlineBytes{
					InlineBytes: ca,
				},
			},
		},
	}

	peerValidationContext := &dag.PeerValidationContext{
		CACertificate: &dag.Secret{
			Object: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					dag.CACertificateKey: ca,
				},
			},
		},
	}

	// Negative test case: downstream validation should not contain subjectname.
	peerValidationContextWithSubjectName := &dag.PeerValidationContext{
		CACertificate: &dag.Secret{
			Object: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					dag.CACertificateKey: ca,
				},
			},
		},
		SubjectName: subjectName,
	}

	tests := map[string]struct {
		got  *envoy_api_v2_auth.DownstreamTlsContext
		want *envoy_api_v2_auth.DownstreamTlsContext
	}{
		"TLS context without client authentication": {
			DownstreamTLSContext(serverSecret, envoy_api_v2_auth.TlsParameters_TLSv1_1, nil, "h2", "http/1.1"),
			&envoy_api_v2_auth.DownstreamTlsContext{
				CommonTlsContext: &envoy_api_v2_auth.CommonTlsContext{
					TlsParams:                      tlsParams,
					TlsCertificateSdsSecretConfigs: tlsCertificateSdsSecretConfigs,
					AlpnProtocols:                  alpnProtocols,
				},
			},
		},
		"TLS context with client authentication": {
			DownstreamTLSContext(serverSecret, envoy_api_v2_auth.TlsParameters_TLSv1_1, peerValidationContext, "h2", "http/1.1"),
			&envoy_api_v2_auth.DownstreamTlsContext{
				CommonTlsContext: &envoy_api_v2_auth.CommonTlsContext{
					TlsParams:                      tlsParams,
					TlsCertificateSdsSecretConfigs: tlsCertificateSdsSecretConfigs,
					AlpnProtocols:                  alpnProtocols,
					ValidationContextType:          validationContext,
				},
				RequireClientCertificate: protobuf.Bool(true),
			},
		},
		"Downstream validation shall not support subjectName validation": {
			DownstreamTLSContext(serverSecret, envoy_api_v2_auth.TlsParameters_TLSv1_1, peerValidationContextWithSubjectName, "h2", "http/1.1"),
			&envoy_api_v2_auth.DownstreamTlsContext{
				CommonTlsContext: &envoy_api_v2_auth.CommonTlsContext{
					TlsParams:                      tlsParams,
					TlsCertificateSdsSecretConfigs: tlsCertificateSdsSecretConfigs,
					AlpnProtocols:                  alpnProtocols,
					ValidationContextType:          validationContext,
				},
				RequireClientCertificate: protobuf.Bool(true),
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.got)
		})
	}
}

func TestHTTPConnectionManager(t *testing.T) {
	tests := map[string]struct {
		routename             string
		accesslogger          []*envoy_api_v2_accesslog.AccessLog
		requestTimeout        time.Duration
		connectionIdleTimeout time.Duration
		streamIdleTimeout     time.Duration
		maxConnectionDuration time.Duration
		drainTimeout          time.Duration
		want                  *envoy_api_v2_listener.Filter
	}{
		"default": {
			routename:      "default/kuard",
			accesslogger:   FileAccessLogEnvoy("/dev/stdout"),
			requestTimeout: 0,
			want: &envoy_api_v2_listener.Filter{
				Name: wellknown.HTTPConnectionManager,
				ConfigType: &envoy_api_v2_listener.Filter_TypedConfig{
					TypedConfig: protobuf.MustMarshalAny(&http.HttpConnectionManager{
						StatPrefix: "default/kuard",
						RouteSpecifier: &http.HttpConnectionManager_Rds{
							Rds: &http.Rds{
								RouteConfigName: "default/kuard",
								ConfigSource: &envoy_api_v2_core.ConfigSource{
									ConfigSourceSpecifier: &envoy_api_v2_core.ConfigSource_ApiConfigSource{
										ApiConfigSource: &envoy_api_v2_core.ApiConfigSource{
											ApiType: envoy_api_v2_core.ApiConfigSource_GRPC,
											GrpcServices: []*envoy_api_v2_core.GrpcService{{
												TargetSpecifier: &envoy_api_v2_core.GrpcService_EnvoyGrpc_{
													EnvoyGrpc: &envoy_api_v2_core.GrpcService_EnvoyGrpc{
														ClusterName: "contour",
													},
												},
											}},
										},
									},
								},
							},
						},
						HttpFilters: []*http.HttpFilter{{
							Name: wellknown.Gzip,
						}, {
							Name: wellknown.GRPCWeb,
						}, {
							Name: wellknown.Router,
						}},
						HttpProtocolOptions: &envoy_api_v2_core.Http1ProtocolOptions{
							// Enable support for HTTP/1.0 requests that carry
							// a Host: header. See #537.
							AcceptHttp_10: true,
						},
						CommonHttpProtocolOptions: &envoy_api_v2_core.HttpProtocolOptions{
							IdleTimeout: protobuf.Duration(0),
						},
						AccessLog:                 FileAccessLogEnvoy("/dev/stdout"),
						UseRemoteAddress:          protobuf.Bool(true),
						NormalizePath:             protobuf.Bool(true),
						RequestTimeout:            protobuf.Duration(0),
						PreserveExternalRequestId: true,
						MergeSlashes:              true,
						StreamIdleTimeout:         protobuf.Duration(0),
						DrainTimeout:              protobuf.Duration(0),
					}),
				},
			},
		},
		"request timeout of 10s": {
			routename:      "default/kuard",
			accesslogger:   FileAccessLogEnvoy("/dev/stdout"),
			requestTimeout: 10 * time.Second,
			want: &envoy_api_v2_listener.Filter{
				Name: wellknown.HTTPConnectionManager,
				ConfigType: &envoy_api_v2_listener.Filter_TypedConfig{
					TypedConfig: protobuf.MustMarshalAny(&http.HttpConnectionManager{
						StatPrefix: "default/kuard",
						RouteSpecifier: &http.HttpConnectionManager_Rds{
							Rds: &http.Rds{
								RouteConfigName: "default/kuard",
								ConfigSource: &envoy_api_v2_core.ConfigSource{
									ConfigSourceSpecifier: &envoy_api_v2_core.ConfigSource_ApiConfigSource{
										ApiConfigSource: &envoy_api_v2_core.ApiConfigSource{
											ApiType: envoy_api_v2_core.ApiConfigSource_GRPC,
											GrpcServices: []*envoy_api_v2_core.GrpcService{{
												TargetSpecifier: &envoy_api_v2_core.GrpcService_EnvoyGrpc_{
													EnvoyGrpc: &envoy_api_v2_core.GrpcService_EnvoyGrpc{
														ClusterName: "contour",
													},
												},
											}},
										},
									},
								},
							},
						},
						HttpFilters: []*http.HttpFilter{{
							Name: wellknown.Gzip,
						}, {
							Name: wellknown.GRPCWeb,
						}, {
							Name: wellknown.Router,
						}},
						HttpProtocolOptions: &envoy_api_v2_core.Http1ProtocolOptions{
							// Enable support for HTTP/1.0 requests that carry
							// a Host: header. See #537.
							AcceptHttp_10: true,
						},
						CommonHttpProtocolOptions: &envoy_api_v2_core.HttpProtocolOptions{
							IdleTimeout: protobuf.Duration(0),
						},
						AccessLog:                 FileAccessLogEnvoy("/dev/stdout"),
						UseRemoteAddress:          protobuf.Bool(true),
						NormalizePath:             protobuf.Bool(true),
						RequestTimeout:            protobuf.Duration(10 * time.Second),
						PreserveExternalRequestId: true,
						MergeSlashes:              true,
						StreamIdleTimeout:         protobuf.Duration(0),
						DrainTimeout:              protobuf.Duration(0),
					}),
				},
			},
		},
		"connection idle timeout of 90s": {
			routename:             "default/kuard",
			accesslogger:          FileAccessLogEnvoy("/dev/stdout"),
			requestTimeout:        0,
			connectionIdleTimeout: 90 * time.Second,
			want: &envoy_api_v2_listener.Filter{
				Name: wellknown.HTTPConnectionManager,
				ConfigType: &envoy_api_v2_listener.Filter_TypedConfig{
					TypedConfig: protobuf.MustMarshalAny(&http.HttpConnectionManager{
						StatPrefix: "default/kuard",
						RouteSpecifier: &http.HttpConnectionManager_Rds{
							Rds: &http.Rds{
								RouteConfigName: "default/kuard",
								ConfigSource: &envoy_api_v2_core.ConfigSource{
									ConfigSourceSpecifier: &envoy_api_v2_core.ConfigSource_ApiConfigSource{
										ApiConfigSource: &envoy_api_v2_core.ApiConfigSource{
											ApiType: envoy_api_v2_core.ApiConfigSource_GRPC,
											GrpcServices: []*envoy_api_v2_core.GrpcService{{
												TargetSpecifier: &envoy_api_v2_core.GrpcService_EnvoyGrpc_{
													EnvoyGrpc: &envoy_api_v2_core.GrpcService_EnvoyGrpc{
														ClusterName: "contour",
													},
												},
											}},
										},
									},
								},
							},
						},
						HttpFilters: []*http.HttpFilter{{
							Name: wellknown.Gzip,
						}, {
							Name: wellknown.GRPCWeb,
						}, {
							Name: wellknown.Router,
						}},
						HttpProtocolOptions: &envoy_api_v2_core.Http1ProtocolOptions{
							// Enable support for HTTP/1.0 requests that carry
							// a Host: header. See #537.
							AcceptHttp_10: true,
						},
						CommonHttpProtocolOptions: &envoy_api_v2_core.HttpProtocolOptions{
							IdleTimeout: protobuf.Duration(90 * time.Second),
						},
						AccessLog:                 FileAccessLogEnvoy("/dev/stdout"),
						UseRemoteAddress:          protobuf.Bool(true),
						NormalizePath:             protobuf.Bool(true),
						RequestTimeout:            protobuf.Duration(0),
						PreserveExternalRequestId: true,
						MergeSlashes:              true,
						StreamIdleTimeout:         protobuf.Duration(0),
						DrainTimeout:              protobuf.Duration(0),
					}),
				},
			},
		},
		"stream idle timeout of 90s": {
			routename:         "default/kuard",
			accesslogger:      FileAccessLogEnvoy("/dev/stdout"),
			requestTimeout:    0,
			streamIdleTimeout: 90 * time.Second,
			want: &envoy_api_v2_listener.Filter{
				Name: wellknown.HTTPConnectionManager,
				ConfigType: &envoy_api_v2_listener.Filter_TypedConfig{
					TypedConfig: protobuf.MustMarshalAny(&http.HttpConnectionManager{
						StatPrefix: "default/kuard",
						RouteSpecifier: &http.HttpConnectionManager_Rds{
							Rds: &http.Rds{
								RouteConfigName: "default/kuard",
								ConfigSource: &envoy_api_v2_core.ConfigSource{
									ConfigSourceSpecifier: &envoy_api_v2_core.ConfigSource_ApiConfigSource{
										ApiConfigSource: &envoy_api_v2_core.ApiConfigSource{
											ApiType: envoy_api_v2_core.ApiConfigSource_GRPC,
											GrpcServices: []*envoy_api_v2_core.GrpcService{{
												TargetSpecifier: &envoy_api_v2_core.GrpcService_EnvoyGrpc_{
													EnvoyGrpc: &envoy_api_v2_core.GrpcService_EnvoyGrpc{
														ClusterName: "contour",
													},
												},
											}},
										},
									},
								},
							},
						},
						HttpFilters: []*http.HttpFilter{{
							Name: wellknown.Gzip,
						}, {
							Name: wellknown.GRPCWeb,
						}, {
							Name: wellknown.Router,
						}},
						HttpProtocolOptions: &envoy_api_v2_core.Http1ProtocolOptions{
							// Enable support for HTTP/1.0 requests that carry
							// a Host: header. See #537.
							AcceptHttp_10: true,
						},
						CommonHttpProtocolOptions: &envoy_api_v2_core.HttpProtocolOptions{
							IdleTimeout: protobuf.Duration(0),
						},
						AccessLog:                 FileAccessLogEnvoy("/dev/stdout"),
						UseRemoteAddress:          protobuf.Bool(true),
						NormalizePath:             protobuf.Bool(true),
						RequestTimeout:            protobuf.Duration(0),
						PreserveExternalRequestId: true,
						MergeSlashes:              true,
						StreamIdleTimeout:         protobuf.Duration(90 * time.Second),
						DrainTimeout:              protobuf.Duration(0),
					}),
				},
			},
		},
		"max connection duration of 90s": {
			routename:             "default/kuard",
			accesslogger:          FileAccessLogEnvoy("/dev/stdout"),
			requestTimeout:        0,
			maxConnectionDuration: 90 * time.Second,
			want: &envoy_api_v2_listener.Filter{
				Name: wellknown.HTTPConnectionManager,
				ConfigType: &envoy_api_v2_listener.Filter_TypedConfig{
					TypedConfig: protobuf.MustMarshalAny(&http.HttpConnectionManager{
						StatPrefix: "default/kuard",
						RouteSpecifier: &http.HttpConnectionManager_Rds{
							Rds: &http.Rds{
								RouteConfigName: "default/kuard",
								ConfigSource: &envoy_api_v2_core.ConfigSource{
									ConfigSourceSpecifier: &envoy_api_v2_core.ConfigSource_ApiConfigSource{
										ApiConfigSource: &envoy_api_v2_core.ApiConfigSource{
											ApiType: envoy_api_v2_core.ApiConfigSource_GRPC,
											GrpcServices: []*envoy_api_v2_core.GrpcService{{
												TargetSpecifier: &envoy_api_v2_core.GrpcService_EnvoyGrpc_{
													EnvoyGrpc: &envoy_api_v2_core.GrpcService_EnvoyGrpc{
														ClusterName: "contour",
													},
												},
											}},
										},
									},
								},
							},
						},
						HttpFilters: []*http.HttpFilter{{
							Name: wellknown.Gzip,
						}, {
							Name: wellknown.GRPCWeb,
						}, {
							Name: wellknown.Router,
						}},
						HttpProtocolOptions: &envoy_api_v2_core.Http1ProtocolOptions{
							// Enable support for HTTP/1.0 requests that carry
							// a Host: header. See #537.
							AcceptHttp_10: true,
						},
						CommonHttpProtocolOptions: &envoy_api_v2_core.HttpProtocolOptions{
							IdleTimeout:           protobuf.Duration(0),
							MaxConnectionDuration: protobuf.Duration(90 * time.Second),
						},
						AccessLog:                 FileAccessLogEnvoy("/dev/stdout"),
						UseRemoteAddress:          protobuf.Bool(true),
						NormalizePath:             protobuf.Bool(true),
						RequestTimeout:            protobuf.Duration(0),
						PreserveExternalRequestId: true,
						MergeSlashes:              true,
						StreamIdleTimeout:         protobuf.Duration(0),
						DrainTimeout:              protobuf.Duration(0),
					}),
				},
			},
		},
		"max connection duration of 0s is omitted": {
			routename:             "default/kuard",
			accesslogger:          FileAccessLogEnvoy("/dev/stdout"),
			requestTimeout:        0,
			maxConnectionDuration: 0,
			want: &envoy_api_v2_listener.Filter{
				Name: wellknown.HTTPConnectionManager,
				ConfigType: &envoy_api_v2_listener.Filter_TypedConfig{
					TypedConfig: protobuf.MustMarshalAny(&http.HttpConnectionManager{
						StatPrefix: "default/kuard",
						RouteSpecifier: &http.HttpConnectionManager_Rds{
							Rds: &http.Rds{
								RouteConfigName: "default/kuard",
								ConfigSource: &envoy_api_v2_core.ConfigSource{
									ConfigSourceSpecifier: &envoy_api_v2_core.ConfigSource_ApiConfigSource{
										ApiConfigSource: &envoy_api_v2_core.ApiConfigSource{
											ApiType: envoy_api_v2_core.ApiConfigSource_GRPC,
											GrpcServices: []*envoy_api_v2_core.GrpcService{{
												TargetSpecifier: &envoy_api_v2_core.GrpcService_EnvoyGrpc_{
													EnvoyGrpc: &envoy_api_v2_core.GrpcService_EnvoyGrpc{
														ClusterName: "contour",
													},
												},
											}},
										},
									},
								},
							},
						},
						HttpFilters: []*http.HttpFilter{{
							Name: wellknown.Gzip,
						}, {
							Name: wellknown.GRPCWeb,
						}, {
							Name: wellknown.Router,
						}},
						HttpProtocolOptions: &envoy_api_v2_core.Http1ProtocolOptions{
							// Enable support for HTTP/1.0 requests that carry
							// a Host: header. See #537.
							AcceptHttp_10: true,
						},
						CommonHttpProtocolOptions: &envoy_api_v2_core.HttpProtocolOptions{
							IdleTimeout: protobuf.Duration(0),
						},
						AccessLog:                 FileAccessLogEnvoy("/dev/stdout"),
						UseRemoteAddress:          protobuf.Bool(true),
						NormalizePath:             protobuf.Bool(true),
						RequestTimeout:            protobuf.Duration(0),
						PreserveExternalRequestId: true,
						MergeSlashes:              true,
						StreamIdleTimeout:         protobuf.Duration(0),
						DrainTimeout:              protobuf.Duration(0),
					}),
				},
			},
		},
		"drain timeout of 90s": {
			routename:      "default/kuard",
			accesslogger:   FileAccessLogEnvoy("/dev/stdout"),
			requestTimeout: 0,
			drainTimeout:   90 * time.Second,
			want: &envoy_api_v2_listener.Filter{
				Name: wellknown.HTTPConnectionManager,
				ConfigType: &envoy_api_v2_listener.Filter_TypedConfig{
					TypedConfig: protobuf.MustMarshalAny(&http.HttpConnectionManager{
						StatPrefix: "default/kuard",
						RouteSpecifier: &http.HttpConnectionManager_Rds{
							Rds: &http.Rds{
								RouteConfigName: "default/kuard",
								ConfigSource: &envoy_api_v2_core.ConfigSource{
									ConfigSourceSpecifier: &envoy_api_v2_core.ConfigSource_ApiConfigSource{
										ApiConfigSource: &envoy_api_v2_core.ApiConfigSource{
											ApiType: envoy_api_v2_core.ApiConfigSource_GRPC,
											GrpcServices: []*envoy_api_v2_core.GrpcService{{
												TargetSpecifier: &envoy_api_v2_core.GrpcService_EnvoyGrpc_{
													EnvoyGrpc: &envoy_api_v2_core.GrpcService_EnvoyGrpc{
														ClusterName: "contour",
													},
												},
											}},
										},
									},
								},
							},
						},
						HttpFilters: []*http.HttpFilter{{
							Name: wellknown.Gzip,
						}, {
							Name: wellknown.GRPCWeb,
						}, {
							Name: wellknown.Router,
						}},
						HttpProtocolOptions: &envoy_api_v2_core.Http1ProtocolOptions{
							// Enable support for HTTP/1.0 requests that carry
							// a Host: header. See #537.
							AcceptHttp_10: true,
						},
						CommonHttpProtocolOptions: &envoy_api_v2_core.HttpProtocolOptions{
							IdleTimeout: protobuf.Duration(0),
						},
						AccessLog:                 FileAccessLogEnvoy("/dev/stdout"),
						UseRemoteAddress:          protobuf.Bool(true),
						NormalizePath:             protobuf.Bool(true),
						RequestTimeout:            protobuf.Duration(0),
						PreserveExternalRequestId: true,
						MergeSlashes:              true,
						StreamIdleTimeout:         protobuf.Duration(0),
						DrainTimeout:              protobuf.Duration(90 * time.Second),
					}),
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := HTTPConnectionManagerBuilder().
				RouteConfigName(tc.routename).
				MetricsPrefix(tc.routename).
				AccessLoggers(tc.accesslogger).
				RequestTimeout(tc.requestTimeout).
				ConnectionIdleTimeout(tc.connectionIdleTimeout).
				StreamIdleTimeout(tc.streamIdleTimeout).
				MaxConnectionDuration(tc.maxConnectionDuration).
				DrainTimeout(tc.drainTimeout).
				DefaultFilters().
				Get()

			assert.Equal(t, tc.want, got)
		})
	}
}

func TestTCPProxy(t *testing.T) {
	const (
		statPrefix    = "ingress_https"
		accessLogPath = "/dev/stdout"
	)

	c1 := &dag.Cluster{
		Upstream: &dag.Service{
			Name:      "example",
			Namespace: "default",
			ServicePort: &v1.ServicePort{
				Protocol:   "TCP",
				Port:       443,
				TargetPort: intstr.FromInt(8443),
			},
		},
	}
	c2 := &dag.Cluster{
		Upstream: &dag.Service{
			Name:      "example2",
			Namespace: "default",
			ServicePort: &v1.ServicePort{
				Protocol:   "TCP",
				Port:       443,
				TargetPort: intstr.FromInt(8443),
			},
		},
		Weight: 20,
	}

	tests := map[string]struct {
		proxy *dag.TCPProxy
		want  *envoy_api_v2_listener.Filter
	}{
		"single cluster": {
			proxy: &dag.TCPProxy{
				Clusters: []*dag.Cluster{c1},
			},
			want: &envoy_api_v2_listener.Filter{
				Name: wellknown.TCPProxy,
				ConfigType: &envoy_api_v2_listener.Filter_TypedConfig{
					TypedConfig: protobuf.MustMarshalAny(&envoy_config_v2_tcpproxy.TcpProxy{
						StatPrefix: statPrefix,
						ClusterSpecifier: &envoy_config_v2_tcpproxy.TcpProxy_Cluster{
							Cluster: Clustername(c1),
						},
						AccessLog:   FileAccessLogEnvoy(accessLogPath),
						IdleTimeout: protobuf.Duration(9001 * time.Second),
					}),
				},
			},
		},
		"multiple cluster": {
			proxy: &dag.TCPProxy{
				Clusters: []*dag.Cluster{c2, c1},
			},
			want: &envoy_api_v2_listener.Filter{
				Name: wellknown.TCPProxy,
				ConfigType: &envoy_api_v2_listener.Filter_TypedConfig{
					TypedConfig: protobuf.MustMarshalAny(&envoy_config_v2_tcpproxy.TcpProxy{
						StatPrefix: statPrefix,
						ClusterSpecifier: &envoy_config_v2_tcpproxy.TcpProxy_WeightedClusters{
							WeightedClusters: &envoy_config_v2_tcpproxy.TcpProxy_WeightedCluster{
								Clusters: []*envoy_config_v2_tcpproxy.TcpProxy_WeightedCluster_ClusterWeight{{
									Name:   Clustername(c1),
									Weight: 1,
								}, {
									Name:   Clustername(c2),
									Weight: 20,
								}},
							},
						},
						AccessLog:   FileAccessLogEnvoy(accessLogPath),
						IdleTimeout: protobuf.Duration(9001 * time.Second),
					}),
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := TCPProxy(statPrefix, tc.proxy, FileAccessLogEnvoy(accessLogPath))
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestCodecForVersions(t *testing.T) {
	assert.Equal(t, CodecForVersions(HTTPVersionAuto), HTTPVersionAuto)
	assert.Equal(t, CodecForVersions(HTTPVersion1, HTTPVersion2), HTTPVersionAuto)
	assert.Equal(t, CodecForVersions(HTTPVersion1), HTTPVersion1)
	assert.Equal(t, CodecForVersions(HTTPVersion2), HTTPVersion2)
}

func TestProtoNamesForVersions(t *testing.T) {
	assert.Equal(t, ProtoNamesForVersions(), []string{"h2", "http/1.1"})
	assert.Equal(t, ProtoNamesForVersions(HTTPVersionAuto), []string{"h2", "http/1.1"})
	assert.Equal(t, ProtoNamesForVersions(HTTPVersion1), []string{"http/1.1"})
	assert.Equal(t, ProtoNamesForVersions(HTTPVersion2), []string{"h2"})
	assert.Equal(t, ProtoNamesForVersions(HTTPVersion3), []string(nil))
	assert.Equal(t, ProtoNamesForVersions(HTTPVersion1, HTTPVersion2), []string{"h2", "http/1.1"})
}
