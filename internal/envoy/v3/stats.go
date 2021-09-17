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
	envoy_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	http "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/projectcontour/contour/internal/protobuf"
)

// StatsListener returns a *envoy_listener_v3.Listener configured to serve prometheus
// metrics on /stats.
func StatsListener(address string, port int) *envoy_listener_v3.Listener {
	return &envoy_listener_v3.Listener{
		Name:    "stats-health",
		Address: SocketAddress(address, port),
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
									Routes: []*envoy_route_v3.Route{
										serviceStatsRoute("/ready"),
										serviceStatsRoute("/stats"),
									},
								}},
							},
						},
						HttpFilters: []*http.HttpFilter{{
							Name: wellknown.Router,
						}},
						NormalizePath: protobuf.Bool(true),
					}),
				},
			},
		),
		SocketOptions: TCPKeepaliveSocketOptions(),
	}
}

// EnvoyAdminListener returns a *envoy_listener_v3.Listener configured to serve
// Envoy Admin endpoints.
func EnvoyAdminListener(port int) *envoy_listener_v3.Listener {
	return &envoy_listener_v3.Listener{
		Name:    "envoy-admin",
		Address: SocketAddress("127.0.0.1", port),
		FilterChains: FilterChains(
			&envoy_listener_v3.Filter{
				Name: wellknown.HTTPConnectionManager,
				ConfigType: &envoy_listener_v3.Filter_TypedConfig{
					TypedConfig: protobuf.MustMarshalAny(&http.HttpConnectionManager{
						StatPrefix: "envoy-admin",
						RouteSpecifier: &http.HttpConnectionManager_RouteConfig{
							RouteConfig: &envoy_route_v3.RouteConfiguration{
								VirtualHosts: []*envoy_route_v3.VirtualHost{{
									Name:    "backend",
									Domains: []string{"*"},
									Routes: []*envoy_route_v3.Route{
										serviceStatsRoute("/certs"),
										serviceStatsRoute("/clusters"),
										serviceStatsRoute("/listeners"),
										serviceStatsRoute("/config_dump"),
										serviceStatsRoute("/memory"),
										serviceStatsRoute("/ready"),
										serviceStatsRoute("/runtime"),
										serviceStatsRoute("/server_info"),
										serviceStatsRoute("/stats"),
										serviceStatsRoute("/stats/prometheus"),
										serviceStatsRoute("/stats/recentlookups"),
									},
								}},
							},
						},
						HttpFilters: []*http.HttpFilter{{
							Name: wellknown.Router,
						}},
						NormalizePath: protobuf.Bool(true),
					}),
				},
			},
		),
		SocketOptions: TCPKeepaliveSocketOptions(),
	}
}

// AdminListener returns a *envoy_listener_v3.Listener configured to serve Envoy
// debug routes from the admin webpage.
func AdminListener(address string, port int) *envoy_listener_v3.Listener {
	return &envoy_listener_v3.Listener{
		Name:    "envoy-admin",
		Address: SocketAddress(address, port),
		FilterChains: FilterChains(
			&envoy_listener_v3.Filter{
				Name: wellknown.HTTPConnectionManager,
				ConfigType: &envoy_listener_v3.Filter_TypedConfig{
					TypedConfig: protobuf.MustMarshalAny(&http.HttpConnectionManager{
						StatPrefix: "envoy-admin",
						RouteSpecifier: &http.HttpConnectionManager_RouteConfig{
							RouteConfig: &envoy_route_v3.RouteConfiguration{
								VirtualHosts: []*envoy_route_v3.VirtualHost{{
									Name:    "backend",
									Domains: []string{"*"},
									Routes: []*envoy_route_v3.Route{
										serviceStatsRoute("/certs"),
										serviceStatsRoute("/clusters"),
										serviceStatsRoute("/listeners"),
										serviceStatsRoute("/config_dump"),
										serviceStatsRoute("/memory"),
										serviceStatsRoute("/ready"),
										serviceStatsRoute("/runtime"),
										serviceStatsRoute("/server_info"),
										serviceStatsRoute("/stats"),
										serviceStatsRoute("/stats/prometheus"),
										serviceStatsRoute("/stats/recentlookups"),
									},
								}},
							},
						},
						HttpFilters: []*http.HttpFilter{{
							Name: wellknown.Router,
						}},
						NormalizePath: protobuf.Bool(true),
					}),
				},
			},
		),
		SocketOptions: TCPKeepaliveSocketOptions(),
	}
}

func serviceStatsRoute(prefix string) *envoy_route_v3.Route {
	return &envoy_route_v3.Route{
		Match: &envoy_route_v3.RouteMatch{
			PathSpecifier: &envoy_route_v3.RouteMatch_Prefix{
				Prefix: prefix,
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
}
