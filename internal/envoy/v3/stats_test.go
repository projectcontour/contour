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

	envoy_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	http "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/projectcontour/contour/internal/protobuf"
)

func TestStatsListener(t *testing.T) {
	tests := map[string]struct {
		address string
		port    int
		want    *envoy_listener_v3.Listener
	}{
		"stats-health": {
			address: "127.0.0.127",
			port:    8123,
			want: &envoy_listener_v3.Listener{
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
											Routes: []*envoy_route_v3.Route{{
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
											}, {
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
											},
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
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := StatsListener(tc.address, tc.port)
			protobuf.ExpectEqual(t, tc.want, got)
		})
	}
}
