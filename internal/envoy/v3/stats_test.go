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

package v3

import (
	"testing"

	envoy_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_router_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/router/v3"
	http "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoy_tls_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	contour_api_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/protobuf"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestStatsListeners(t *testing.T) {
	readyRoute := &envoy_route_v3.Route{
		Match: &envoy_route_v3.RouteMatch{
			PathSpecifier: &envoy_route_v3.RouteMatch_Prefix{
				Prefix: "/ready",
			},
		},
		Action: &envoy_route_v3.Route_Route{
			Route: &envoy_route_v3.RouteAction{
				ClusterSpecifier: &envoy_route_v3.RouteAction_Cluster{
					Cluster: "envoy-admin",
				},
			},
		},
	}

	statsRoute := &envoy_route_v3.Route{
		Match: &envoy_route_v3.RouteMatch{
			PathSpecifier: &envoy_route_v3.RouteMatch_Prefix{
				Prefix: "/stats",
			},
		},
		Action: &envoy_route_v3.Route_Route{
			Route: &envoy_route_v3.RouteAction{
				ClusterSpecifier: &envoy_route_v3.RouteAction_Cluster{
					Cluster: "envoy-admin",
				},
			},
		},
	}

	type testcase struct {
		metrics contour_api_v1alpha1.MetricsConfig
		health  contour_api_v1alpha1.HealthConfig
		want    []*envoy_listener_v3.Listener
	}

	run := func(t *testing.T, name string, tc testcase) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			t.Helper()
			got := StatsListeners(tc.metrics, tc.health)
			protobuf.ExpectEqual(t, tc.want, got)
		})
	}

	run(t, "stats-and-health-over-http-single-listener", testcase{
		metrics: contour_api_v1alpha1.MetricsConfig{Address: "127.0.0.127", Port: 8123},
		health:  contour_api_v1alpha1.HealthConfig{Address: "127.0.0.127", Port: 8123},
		want: []*envoy_listener_v3.Listener{{
			Name:    "stats-health",
			Address: SocketAddress("127.0.0.127", 8123),
			FilterChains: FilterChains(
				&envoy_listener_v3.Filter{
					Name: wellknown.HTTPConnectionManager,
					ConfigType: &envoy_listener_v3.Filter_TypedConfig{
						TypedConfig: protobuf.MustMarshalAny(&http.HttpConnectionManager{
							StatPrefix: "stats",
							RouteSpecifier: &http.HttpConnectionManager_RouteConfig{
								RouteConfig: &envoy_route_v3.RouteConfiguration{
									VirtualHosts: []*envoy_route_v3.VirtualHost{{
										Name:    "backend",
										Domains: []string{"*"},
										Routes:  []*envoy_route_v3.Route{readyRoute, statsRoute},
									}},
								},
							},
							HttpFilters: []*http.HttpFilter{{
								Name: wellknown.Router,
								ConfigType: &http.HttpFilter_TypedConfig{
									TypedConfig: protobuf.MustMarshalAny(&envoy_router_v3.Router{}),
								},
							}},
							NormalizePath: wrapperspb.Bool(true),
						}),
					},
				},
			),
			SocketOptions: TCPKeepaliveSocketOptions(),
		}}})

	run(t, "stats-over-https-and-health-over-http", testcase{
		metrics: contour_api_v1alpha1.MetricsConfig{
			Address: "127.0.0.127",
			Port:    8123,
			TLS: &contour_api_v1alpha1.MetricsTLS{
				CertFile: "certfile",
				KeyFile:  "keyfile",
			},
		},
		health: contour_api_v1alpha1.HealthConfig{
			Address: "127.0.0.127",
			Port:    8124},
		want: []*envoy_listener_v3.Listener{{
			Name:    "stats",
			Address: SocketAddress("127.0.0.127", 8123),
			FilterChains: []*envoy_listener_v3.FilterChain{{
				Filters: []*envoy_listener_v3.Filter{{
					Name: wellknown.HTTPConnectionManager,
					ConfigType: &envoy_listener_v3.Filter_TypedConfig{
						TypedConfig: protobuf.MustMarshalAny(&http.HttpConnectionManager{
							StatPrefix: "stats",
							RouteSpecifier: &http.HttpConnectionManager_RouteConfig{
								RouteConfig: &envoy_route_v3.RouteConfiguration{
									VirtualHosts: []*envoy_route_v3.VirtualHost{{
										Name:    "backend",
										Domains: []string{"*"},
										Routes:  []*envoy_route_v3.Route{statsRoute},
									}},
								},
							},
							HttpFilters: []*http.HttpFilter{{
								Name: wellknown.Router,
								ConfigType: &http.HttpFilter_TypedConfig{
									TypedConfig: protobuf.MustMarshalAny(&envoy_router_v3.Router{}),
								},
							}},
							NormalizePath: wrapperspb.Bool(true),
						}),
					},
				}},
				TransportSocket: DownstreamTLSTransportSocket(
					&envoy_tls_v3.DownstreamTlsContext{
						CommonTlsContext: &envoy_tls_v3.CommonTlsContext{
							TlsParams: &envoy_tls_v3.TlsParameters{
								TlsMinimumProtocolVersion: envoy_tls_v3.TlsParameters_TLSv1_3,
								TlsMaximumProtocolVersion: envoy_tls_v3.TlsParameters_TLSv1_3,
							},
							TlsCertificateSdsSecretConfigs: []*envoy_tls_v3.SdsSecretConfig{{
								Name:      "metrics-tls-certificate",
								SdsConfig: ConfigSource("contour"),
							}},
						},
					},
				),
			}},
			SocketOptions: TCPKeepaliveSocketOptions(),
		}, {
			Name:    "health",
			Address: SocketAddress("127.0.0.127", 8124),
			FilterChains: FilterChains(
				&envoy_listener_v3.Filter{
					Name: wellknown.HTTPConnectionManager,
					ConfigType: &envoy_listener_v3.Filter_TypedConfig{
						TypedConfig: protobuf.MustMarshalAny(&http.HttpConnectionManager{
							StatPrefix: "stats",
							RouteSpecifier: &http.HttpConnectionManager_RouteConfig{
								RouteConfig: &envoy_route_v3.RouteConfiguration{
									VirtualHosts: []*envoy_route_v3.VirtualHost{{
										Name:    "backend",
										Domains: []string{"*"},
										Routes:  []*envoy_route_v3.Route{readyRoute},
									}},
								},
							},
							HttpFilters: []*http.HttpFilter{{
								Name: wellknown.Router,
								ConfigType: &http.HttpFilter_TypedConfig{
									TypedConfig: protobuf.MustMarshalAny(&envoy_router_v3.Router{}),
								},
							}},
							NormalizePath: wrapperspb.Bool(true),
						}),
					},
				},
			),
			SocketOptions: TCPKeepaliveSocketOptions(),
		}}})

	run(t, "stats-over-https-with-client-auth-and-health-over-http", testcase{
		metrics: contour_api_v1alpha1.MetricsConfig{
			Address: "127.0.0.127",
			Port:    8123,
			TLS: &contour_api_v1alpha1.MetricsTLS{
				CertFile: "certfile",
				KeyFile:  "keyfile",
				CAFile:   "cabundle",
			},
		},
		health: contour_api_v1alpha1.HealthConfig{
			Address: "127.0.0.127",
			Port:    8124},
		want: []*envoy_listener_v3.Listener{{
			Name:    "stats",
			Address: SocketAddress("127.0.0.127", 8123),
			FilterChains: []*envoy_listener_v3.FilterChain{{
				Filters: []*envoy_listener_v3.Filter{{
					Name: wellknown.HTTPConnectionManager,
					ConfigType: &envoy_listener_v3.Filter_TypedConfig{
						TypedConfig: protobuf.MustMarshalAny(&http.HttpConnectionManager{
							StatPrefix: "stats",
							RouteSpecifier: &http.HttpConnectionManager_RouteConfig{
								RouteConfig: &envoy_route_v3.RouteConfiguration{
									VirtualHosts: []*envoy_route_v3.VirtualHost{{
										Name:    "backend",
										Domains: []string{"*"},
										Routes:  []*envoy_route_v3.Route{statsRoute},
									}},
								},
							},
							HttpFilters: []*http.HttpFilter{{
								Name: wellknown.Router,
								ConfigType: &http.HttpFilter_TypedConfig{
									TypedConfig: protobuf.MustMarshalAny(&envoy_router_v3.Router{}),
								},
							}},
							NormalizePath: wrapperspb.Bool(true),
						}),
					},
				}},
				TransportSocket: DownstreamTLSTransportSocket(
					&envoy_tls_v3.DownstreamTlsContext{
						CommonTlsContext: &envoy_tls_v3.CommonTlsContext{
							TlsParams: &envoy_tls_v3.TlsParameters{
								TlsMinimumProtocolVersion: envoy_tls_v3.TlsParameters_TLSv1_3,
								TlsMaximumProtocolVersion: envoy_tls_v3.TlsParameters_TLSv1_3,
							},
							TlsCertificateSdsSecretConfigs: []*envoy_tls_v3.SdsSecretConfig{{
								Name:      "metrics-tls-certificate",
								SdsConfig: ConfigSource("contour"),
							}},
							ValidationContextType: &envoy_tls_v3.CommonTlsContext_ValidationContextSdsSecretConfig{
								ValidationContextSdsSecretConfig: &envoy_tls_v3.SdsSecretConfig{
									Name:      "metrics-ca-certificate",
									SdsConfig: ConfigSource("contour"),
								},
							},
						},
						RequireClientCertificate: wrapperspb.Bool(true),
					},
				),
			}},
			SocketOptions: TCPKeepaliveSocketOptions(),
		}, {
			Name:    "health",
			Address: SocketAddress("127.0.0.127", 8124),
			FilterChains: FilterChains(
				&envoy_listener_v3.Filter{
					Name: wellknown.HTTPConnectionManager,
					ConfigType: &envoy_listener_v3.Filter_TypedConfig{
						TypedConfig: protobuf.MustMarshalAny(&http.HttpConnectionManager{
							StatPrefix: "stats",
							RouteSpecifier: &http.HttpConnectionManager_RouteConfig{
								RouteConfig: &envoy_route_v3.RouteConfiguration{
									VirtualHosts: []*envoy_route_v3.VirtualHost{{
										Name:    "backend",
										Domains: []string{"*"},
										Routes:  []*envoy_route_v3.Route{readyRoute},
									}},
								},
							},
							HttpFilters: []*http.HttpFilter{{
								Name: wellknown.Router,
								ConfigType: &http.HttpFilter_TypedConfig{
									TypedConfig: protobuf.MustMarshalAny(&envoy_router_v3.Router{}),
								},
							}},
							NormalizePath: wrapperspb.Bool(true),
						}),
					},
				},
			),
			SocketOptions: TCPKeepaliveSocketOptions(),
		}}})

	run(t, "stats-and-health-over-http-but-different-listeners", testcase{
		metrics: contour_api_v1alpha1.MetricsConfig{
			Address: "127.0.0.127",
			Port:    8123,
		},
		health: contour_api_v1alpha1.HealthConfig{
			Address: "127.0.0.128",
			Port:    8124},
		want: []*envoy_listener_v3.Listener{{
			Name:    "stats",
			Address: SocketAddress("127.0.0.127", 8123),
			FilterChains: FilterChains(
				&envoy_listener_v3.Filter{
					Name: wellknown.HTTPConnectionManager,
					ConfigType: &envoy_listener_v3.Filter_TypedConfig{
						TypedConfig: protobuf.MustMarshalAny(&http.HttpConnectionManager{
							StatPrefix: "stats",
							RouteSpecifier: &http.HttpConnectionManager_RouteConfig{
								RouteConfig: &envoy_route_v3.RouteConfiguration{
									VirtualHosts: []*envoy_route_v3.VirtualHost{{
										Name:    "backend",
										Domains: []string{"*"},
										Routes:  []*envoy_route_v3.Route{statsRoute},
									}},
								},
							},
							HttpFilters: []*http.HttpFilter{{
								Name: wellknown.Router,
								ConfigType: &http.HttpFilter_TypedConfig{
									TypedConfig: protobuf.MustMarshalAny(&envoy_router_v3.Router{}),
								},
							}},
							NormalizePath: wrapperspb.Bool(true),
						}),
					},
				},
			),
			SocketOptions: TCPKeepaliveSocketOptions(),
		}, {
			Name:    "health",
			Address: SocketAddress("127.0.0.128", 8124),
			FilterChains: FilterChains(
				&envoy_listener_v3.Filter{
					Name: wellknown.HTTPConnectionManager,
					ConfigType: &envoy_listener_v3.Filter_TypedConfig{
						TypedConfig: protobuf.MustMarshalAny(&http.HttpConnectionManager{
							StatPrefix: "stats",
							RouteSpecifier: &http.HttpConnectionManager_RouteConfig{
								RouteConfig: &envoy_route_v3.RouteConfiguration{
									VirtualHosts: []*envoy_route_v3.VirtualHost{{
										Name:    "backend",
										Domains: []string{"*"},
										Routes:  []*envoy_route_v3.Route{readyRoute},
									}},
								},
							},
							HttpFilters: []*http.HttpFilter{{
								Name: wellknown.Router,
								ConfigType: &http.HttpFilter_TypedConfig{
									TypedConfig: protobuf.MustMarshalAny(&envoy_router_v3.Router{}),
								},
							}},
							NormalizePath: wrapperspb.Bool(true),
						}),
					},
				},
			),
			SocketOptions: TCPKeepaliveSocketOptions(),
		}}})

}

func TestStatsTLSSecrets(t *testing.T) {
	type testcase struct {
		metricsTLS contour_api_v1alpha1.MetricsTLS
		want       []*envoy_tls_v3.Secret
	}
	run := func(t *testing.T, name string, tc testcase) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			t.Helper()
			protobuf.ExpectEqual(t, tc.want, StatsSecrets(&tc.metricsTLS))
		})
	}

	run(t, "only-server-credentials", testcase{
		metricsTLS: contour_api_v1alpha1.MetricsTLS{
			CertFile: "certfile",
			KeyFile:  "keyfile",
		},
		want: []*envoy_tls_v3.Secret{{
			Name: "metrics-tls-certificate",
			Type: &envoy_tls_v3.Secret_TlsCertificate{
				TlsCertificate: &envoy_tls_v3.TlsCertificate{
					CertificateChain: &envoy_core_v3.DataSource{
						Specifier: &envoy_core_v3.DataSource_Filename{
							Filename: "certfile",
						},
					},
					PrivateKey: &envoy_core_v3.DataSource{
						Specifier: &envoy_core_v3.DataSource_Filename{
							Filename: "keyfile",
						},
					},
				},
			},
		}},
	})

	run(t, "with-client-authentication", testcase{
		metricsTLS: contour_api_v1alpha1.MetricsTLS{
			CertFile: "certfile",
			KeyFile:  "keyfile",
			CAFile:   "cabundle",
		},
		want: []*envoy_tls_v3.Secret{{
			Name: "metrics-tls-certificate",
			Type: &envoy_tls_v3.Secret_TlsCertificate{
				TlsCertificate: &envoy_tls_v3.TlsCertificate{
					CertificateChain: &envoy_core_v3.DataSource{
						Specifier: &envoy_core_v3.DataSource_Filename{
							Filename: "certfile",
						},
					},
					PrivateKey: &envoy_core_v3.DataSource{
						Specifier: &envoy_core_v3.DataSource_Filename{
							Filename: "keyfile",
						},
					},
				},
			},
		}, {
			Name: "metrics-ca-certificate",
			Type: &envoy_tls_v3.Secret_ValidationContext{
				ValidationContext: &envoy_tls_v3.CertificateValidationContext{
					TrustedCa: &envoy_core_v3.DataSource{
						Specifier: &envoy_core_v3.DataSource_Filename{
							Filename: "cabundle",
						},
					},
				},
			},
		}},
	})
}
