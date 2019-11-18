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
	const secretName = "default/tls-cert"

	got := DownstreamTLSContext(secretName, envoy_api_v2_auth.TlsParameters_TLSv1_1, "h2", "http/1.1")
	want := &envoy_api_v2_auth.DownstreamTlsContext{
		CommonTlsContext: &envoy_api_v2_auth.CommonTlsContext{
			TlsParams: &envoy_api_v2_auth.TlsParameters{
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
			},
			TlsCertificateSdsSecretConfigs: []*envoy_api_v2_auth.SdsSecretConfig{{
				Name: secretName,
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
			}},
			AlpnProtocols: []string{"h2", "http/1.1"},
		},
	}
	assert.Equal(t, want, got)
}

func TestHTTPConnectionManager(t *testing.T) {
	tests := map[string]struct {
		routename      string
		accesslogger   []*envoy_api_v2_accesslog.AccessLog
		requestTimeout time.Duration
		want           *envoy_api_v2_listener.Filter
	}{
		"default": {
			routename:      "default/kuard",
			accesslogger:   FileAccessLogEnvoy("/dev/stdout"),
			requestTimeout: 0,
			want: &envoy_api_v2_listener.Filter{
				Name: wellknown.HTTPConnectionManager,
				ConfigType: &envoy_api_v2_listener.Filter_TypedConfig{
					TypedConfig: toAny(&http.HttpConnectionManager{
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
							IdleTimeout: protobuf.Duration(60 * time.Second),
						},
						AccessLog:                 FileAccessLogEnvoy("/dev/stdout"),
						UseRemoteAddress:          protobuf.Bool(true),
						NormalizePath:             protobuf.Bool(true),
						RequestTimeout:            protobuf.Duration(0),
						PreserveExternalRequestId: true,
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
					TypedConfig: toAny(&http.HttpConnectionManager{
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
							IdleTimeout: protobuf.Duration(60 * time.Second),
						},
						AccessLog:                 FileAccessLogEnvoy("/dev/stdout"),
						UseRemoteAddress:          protobuf.Bool(true),
						NormalizePath:             protobuf.Bool(true),
						RequestTimeout:            protobuf.Duration(10 * time.Second),
						PreserveExternalRequestId: true,
					}),
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := HTTPConnectionManager(tc.routename, tc.accesslogger, tc.requestTimeout)
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
					TypedConfig: toAny(&envoy_config_v2_tcpproxy.TcpProxy{
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
					TypedConfig: toAny(&envoy_config_v2_tcpproxy.TcpProxy{
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
