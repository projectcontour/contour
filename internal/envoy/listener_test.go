// Copyright © 2018 Heptio
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
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	http "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	envoy_config_v2_tcpproxy "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/tcp_proxy/v2"
	"github.com/envoyproxy/go-control-plane/pkg/util"
	"github.com/gogo/protobuf/types"
	"github.com/google/go-cmp/cmp"
	"github.com/heptio/contour/internal/dag"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestListener(t *testing.T) {
	tests := map[string]struct {
		name, address string
		port          int
		lf            []listener.ListenerFilter
		f             []listener.Filter
		want          *v2.Listener
	}{
		"insecure listener": {
			name:    "http",
			address: "0.0.0.0",
			port:    9000,
			f:       []listener.Filter{HTTPConnectionManager("http", "/dev/null")},
			want: &v2.Listener{
				Name:    "http",
				Address: *SocketAddress("0.0.0.0", 9000),
				FilterChains: []listener.FilterChain{{
					Filters: []listener.Filter{
						HTTPConnectionManager("http", "/dev/null"),
					},
				}},
			},
		},
		"insecure listener w/ proxy": {
			name:    "http-proxy",
			address: "0.0.0.0",
			port:    9000,
			lf: []listener.ListenerFilter{
				ProxyProtocol(),
			},
			f: []listener.Filter{
				HTTPConnectionManager("http-proxy", "/dev/null"),
			},
			want: &v2.Listener{
				Name:    "http-proxy",
				Address: *SocketAddress("0.0.0.0", 9000),
				ListenerFilters: []listener.ListenerFilter{
					ProxyProtocol(),
				},
				FilterChains: []listener.FilterChain{{
					Filters: []listener.Filter{
						HTTPConnectionManager("http-proxy", "/dev/null"),
					},
				}},
			},
		},
		"secure listener": {
			name:    "https",
			address: "0.0.0.0",
			port:    9000,
			lf: []listener.ListenerFilter{
				TLSInspector(),
			},
			want: &v2.Listener{
				Name:    "https",
				Address: *SocketAddress("0.0.0.0", 9000),
				ListenerFilters: []listener.ListenerFilter{
					TLSInspector(),
				},
			},
		},
		"secure listener w/ proxy": {
			name:    "https-proxy",
			address: "0.0.0.0",
			port:    9000,
			lf: []listener.ListenerFilter{
				ProxyProtocol(),
				TLSInspector(),
			},
			want: &v2.Listener{
				Name:    "https-proxy",
				Address: *SocketAddress("0.0.0.0", 9000),
				ListenerFilters: []listener.ListenerFilter{
					ProxyProtocol(),
					TLSInspector(),
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := Listener(tc.name, tc.address, tc.port, tc.lf, tc.f...)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func TestSocketAddress(t *testing.T) {
	const (
		addr = "foo.example.com"
		port = 8123
	)

	got := SocketAddress(addr, port)
	want := &core.Address{
		Address: &core.Address_SocketAddress{
			SocketAddress: &core.SocketAddress{
				Protocol: core.TCP,
				Address:  addr,
				PortSpecifier: &core.SocketAddress_PortValue{
					PortValue: port,
				},
			},
		},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatal(diff)
	}

	got = SocketAddress("::", port)
	want = &core.Address{
		Address: &core.Address_SocketAddress{
			SocketAddress: &core.SocketAddress{
				Protocol:   core.TCP,
				Address:    "::",
				Ipv4Compat: true, // Set only for ipv6-any "::"
				PortSpecifier: &core.SocketAddress_PortValue{
					PortValue: port,
				},
			},
		},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatal(diff)
	}
}

func TestDownstreamTLSContext(t *testing.T) {
	const secretName = "default/tls-cert"

	got := DownstreamTLSContext(secretName, auth.TlsParameters_TLSv1_1, "h2", "http/1.1")
	want := &auth.DownstreamTlsContext{
		CommonTlsContext: &auth.CommonTlsContext{
			TlsParams: &auth.TlsParameters{
				TlsMinimumProtocolVersion: auth.TlsParameters_TLSv1_1,
				TlsMaximumProtocolVersion: auth.TlsParameters_TLSv1_3,
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
			TlsCertificateSdsSecretConfigs: []*auth.SdsSecretConfig{{
				Name: secretName,
				SdsConfig: &core.ConfigSource{
					ConfigSourceSpecifier: &core.ConfigSource_ApiConfigSource{
						ApiConfigSource: &core.ApiConfigSource{
							ApiType: core.ApiConfigSource_GRPC,
							GrpcServices: []*core.GrpcService{{
								TargetSpecifier: &core.GrpcService_EnvoyGrpc_{
									EnvoyGrpc: &core.GrpcService_EnvoyGrpc{
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
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatal(diff)
	}
}

func TestHTTPConnectionManager(t *testing.T) {
	duration := func(d time.Duration) *time.Duration {
		return &d
	}
	tests := map[string]struct {
		routename string
		accesslog string
		want      listener.Filter
	}{
		"default": {
			routename: "default/kuard",
			accesslog: "/dev/stdout",
			want: listener.Filter{
				Name: util.HTTPConnectionManager,
				ConfigType: &listener.Filter_TypedConfig{
					TypedConfig: any(&http.HttpConnectionManager{
						StatPrefix: "default/kuard",
						RouteSpecifier: &http.HttpConnectionManager_Rds{
							Rds: &http.Rds{
								RouteConfigName: "default/kuard",
								ConfigSource: core.ConfigSource{
									ConfigSourceSpecifier: &core.ConfigSource_ApiConfigSource{
										ApiConfigSource: &core.ApiConfigSource{
											ApiType: core.ApiConfigSource_GRPC,
											GrpcServices: []*core.GrpcService{{
												TargetSpecifier: &core.GrpcService_EnvoyGrpc_{
													EnvoyGrpc: &core.GrpcService_EnvoyGrpc{
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
							Name: util.Gzip,
						}, {
							Name: util.GRPCWeb,
						}, {
							Name: util.Router,
						}},
						HttpProtocolOptions: &core.Http1ProtocolOptions{
							// Enable support for HTTP/1.0 requests that carry
							// a Host: header. See #537.
							AcceptHttp_10: true,
						},
						AccessLog:                 FileAccessLog("/dev/stdout"),
						UseRemoteAddress:          &types.BoolValue{Value: true},
						NormalizePath:             &types.BoolValue{Value: true},
						IdleTimeout:               duration(HTTPDefaultIdleTimeout),
						PreserveExternalRequestId: true,
					}),
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := HTTPConnectionManager(tc.routename, tc.accesslog)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func TestTCPProxy(t *testing.T) {
	const (
		statPrefix    = "ingress_https"
		accessLogPath = "/dev/stdout"
	)

	c1 := &dag.Cluster{
		Upstream: &dag.TCPService{
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
		Upstream: &dag.TCPService{
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
		want  listener.Filter
	}{
		"single cluster": {
			proxy: &dag.TCPProxy{
				Clusters: []*dag.Cluster{c1},
			},
			want: listener.Filter{
				Name: util.TCPProxy,
				ConfigType: &listener.Filter_TypedConfig{
					TypedConfig: any(&envoy_config_v2_tcpproxy.TcpProxy{
						StatPrefix: statPrefix,
						ClusterSpecifier: &envoy_config_v2_tcpproxy.TcpProxy_Cluster{
							Cluster: Clustername(c1),
						},
						AccessLog:   FileAccessLog(accessLogPath),
						IdleTimeout: idleTimeout(TCPDefaultIdleTimeout),
					}),
				},
			},
		},
		"multiple cluster": {
			proxy: &dag.TCPProxy{
				Clusters: []*dag.Cluster{c2, c1},
			},
			want: listener.Filter{
				Name: util.TCPProxy,
				ConfigType: &listener.Filter_TypedConfig{
					TypedConfig: any(&envoy_config_v2_tcpproxy.TcpProxy{
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
						AccessLog:   FileAccessLog(accessLogPath),
						IdleTimeout: idleTimeout(TCPDefaultIdleTimeout),
					}),
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := TCPProxy(statPrefix, tc.proxy, accessLogPath)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}
