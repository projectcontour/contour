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

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_api_v2_listener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	envoy_api_v2_route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	http "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/google/go-cmp/cmp"
	"github.com/projectcontour/contour/internal/protobuf"
)

func TestStatsListener(t *testing.T) {
	tests := map[string]struct {
		address string
		port    int
		want    *v2.Listener
	}{
		"stats-health": {
			address: "127.0.0.127",
			port:    8123,
			want: &v2.Listener{
				Name:    "stats-health",
				Address: SocketAddress("127.0.0.127", 8123),
				FilterChains: FilterChains(
					&envoy_api_v2_listener.Filter{
						Name: wellknown.HTTPConnectionManager,
						ConfigType: &envoy_api_v2_listener.Filter_TypedConfig{
							TypedConfig: protobuf.MustMarshalAny(&http.HttpConnectionManager{
								StatPrefix: "stats",
								RouteSpecifier: &http.HttpConnectionManager_RouteConfig{
									RouteConfig: &v2.RouteConfiguration{
										VirtualHosts: []*envoy_api_v2_route.VirtualHost{{
											Name:    "backend",
											Domains: []string{"*"},
											Routes: []*envoy_api_v2_route.Route{{
												Match: &envoy_api_v2_route.RouteMatch{
													PathSpecifier: &envoy_api_v2_route.RouteMatch_Prefix{
														Prefix: "/ready",
													},
												},
												Action: &envoy_api_v2_route.Route_Route{
													Route: &envoy_api_v2_route.RouteAction{
														ClusterSpecifier: &envoy_api_v2_route.RouteAction_Cluster{
															Cluster: "service-stats",
														},
													},
												},
											}, {
												Match: &envoy_api_v2_route.RouteMatch{
													PathSpecifier: &envoy_api_v2_route.RouteMatch_Prefix{
														Prefix: "/stats",
													},
												},
												Action: &envoy_api_v2_route.Route_Route{
													Route: &envoy_api_v2_route.RouteAction{
														ClusterSpecifier: &envoy_api_v2_route.RouteAction_Cluster{
															Cluster: "service-stats",
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
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}
