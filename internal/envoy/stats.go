// Copyright © 2019 Heptio
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
	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	health_check "github.com/envoyproxy/go-control-plane/envoy/config/filter/http/health_check/v2"
	http "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	"github.com/envoyproxy/go-control-plane/pkg/util"
	"github.com/gogo/protobuf/types"
)

// StatsListener returns a *v2.Listener configured to serve prometheus
// metrics on /stats.
func StatsListener(address string, port int) *v2.Listener {
	return &v2.Listener{
		Name:    "stats-health",
		Address: *SocketAddress(address, port),
		FilterChains: []listener.FilterChain{{
			Filters: []listener.Filter{{
				Name: util.HTTPConnectionManager,
				ConfigType: &listener.Filter_TypedConfig{
					TypedConfig: any(&http.HttpConnectionManager{
						StatPrefix: "stats",
						RouteSpecifier: &http.HttpConnectionManager_RouteConfig{
							RouteConfig: &v2.RouteConfiguration{
								VirtualHosts: []route.VirtualHost{{
									Name:    "backend",
									Domains: []string{"*"},
									Routes: []route.Route{{
										Match: route.RouteMatch{
											PathSpecifier: &route.RouteMatch_Prefix{
												Prefix: "/stats",
											},
										},
										Action: &route.Route_Route{
											Route: &route.RouteAction{
												ClusterSpecifier: &route.RouteAction_Cluster{
													Cluster: "service-stats",
												},
											},
										},
									}},
								}},
							},
						},
						HttpFilters: []*http.HttpFilter{{
							Name: util.HealthCheck,
							ConfigType: &http.HttpFilter_TypedConfig{
								TypedConfig: any(&health_check.HealthCheck{
									PassThroughMode: &types.BoolValue{Value: false},
									Headers: []*route.HeaderMatcher{{
										Name: ":path",
										HeaderMatchSpecifier: &route.HeaderMatcher_ExactMatch{
											ExactMatch: "/healthz",
										},
									}},
								}),
							},
						}, {
							Name: util.Router,
						}},
						NormalizePath: &types.BoolValue{Value: true},
					}),
				},
			}},
		}},
	}
}
