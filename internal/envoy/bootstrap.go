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
	"fmt"
	"strconv"
	"strings"
	"time"

	api "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	clusterv2 "github.com/envoyproxy/go-control-plane/envoy/api/v2/cluster"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	bootstrap "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v2"
	metrics "github.com/envoyproxy/go-control-plane/envoy/config/metrics/v2"
	"github.com/envoyproxy/go-control-plane/pkg/util"
	"github.com/gogo/protobuf/types"
)

// Bootstrap creates a new v2 Bootstrap configuration.
func Bootstrap(c *BootstrapConfig) *bootstrap.Bootstrap {
	b := &bootstrap.Bootstrap{
		DynamicResources: &bootstrap.Bootstrap_DynamicResources{
			LdsConfig: ConfigSource("contour"),
			CdsConfig: ConfigSource("contour"),
		},
		StaticResources: &bootstrap.Bootstrap_StaticResources{
			Listeners: []api.Listener{{
				Address: *SocketAddress(
					stringOrDefault(c.StatsAddress, "0.0.0.0"),
					intOrDefault(c.StatsPort, 8002),
				),
				FilterChains: []listener.FilterChain{{
					Filters: []listener.Filter{{
						Name: util.HTTPConnectionManager,
						ConfigType: &listener.Filter_Config{
							Config: &types.Struct{
								Fields: map[string]*types.Value{
									"access_log":  accesslog("/dev/stdout"),
									"codec_type":  sv("AUTO"),
									"stat_prefix": sv("stats"),
									"route_config": st(map[string]*types.Value{
										"virtual_hosts": st(map[string]*types.Value{
											"name":    sv("backend"),
											"domains": lv(sv("*")),
											"routes": lv(
												st(map[string]*types.Value{
													"match": st(map[string]*types.Value{
														"prefix": sv("/stats"),
													}),
													"route": st(map[string]*types.Value{
														"cluster": sv("service-stats"),
													}),
												}),
											),
										}),
									}),
									"http_filters": lv(
										st(map[string]*types.Value{
											"name": sv(util.HealthCheck),
											"config": st(map[string]*types.Value{
												"pass_through_mode": sv("false"), // not sure about this
												"headers": lv(
													st(map[string]*types.Value{
														"name":        sv(":path"),
														"exact_match": sv("/healthz"),
													}),
												),
											}),
										}),
										st(map[string]*types.Value{
											"name": sv(util.Router),
										}),
									),
									"normalize_path": {Kind: &types.Value_BoolValue{BoolValue: true}},
								},
							},
						},
					}},
				}},
			}},
			Clusters: []api.Cluster{{
				Name:                 "contour",
				AltStatName:          strings.Join([]string{c.Namespace, "contour", strconv.Itoa(intOrDefault(c.XDSGRPCPort, 8001))}, "_"),
				ConnectTimeout:       5 * time.Second,
				ClusterDiscoveryType: ClusterDiscoveryType(api.Cluster_STRICT_DNS),
				LbPolicy:             api.Cluster_ROUND_ROBIN,
				LoadAssignment: &api.ClusterLoadAssignment{
					ClusterName: "contour",
					Endpoints: []endpoint.LocalityLbEndpoints{{
						LbEndpoints: []endpoint.LbEndpoint{
							LBEndpoint(stringOrDefault(c.XDSAddress, "127.0.0.1"), intOrDefault(c.XDSGRPCPort, 8001)),
						},
					}},
				},
				Http2ProtocolOptions: new(core.Http2ProtocolOptions), // enables http2
				CircuitBreakers: &clusterv2.CircuitBreakers{
					Thresholds: []*clusterv2.CircuitBreakers_Thresholds{{
						Priority:           core.RoutingPriority_HIGH,
						MaxConnections:     u32(100000),
						MaxPendingRequests: u32(100000),
						MaxRequests:        u32(60000000),
						MaxRetries:         u32(50),
					}, {
						Priority:           core.RoutingPriority_DEFAULT,
						MaxConnections:     u32(100000),
						MaxPendingRequests: u32(100000),
						MaxRequests:        u32(60000000),
						MaxRetries:         u32(50),
					}},
				},
			}, {
				Name:                 "service-stats",
				AltStatName:          strings.Join([]string{c.Namespace, "service-stats", strconv.Itoa(intOrDefault(c.AdminPort, 9001))}, "_"),
				ConnectTimeout:       250 * time.Millisecond,
				ClusterDiscoveryType: ClusterDiscoveryType(api.Cluster_LOGICAL_DNS),
				LbPolicy:             api.Cluster_ROUND_ROBIN,
				LoadAssignment: &api.ClusterLoadAssignment{
					ClusterName: "service-stats",
					Endpoints: []endpoint.LocalityLbEndpoints{{
						LbEndpoints: []endpoint.LbEndpoint{
							LBEndpoint(stringOrDefault(c.AdminAddress, "127.0.0.1"), intOrDefault(c.AdminPort, 9001)),
						},
					}},
				},
			}},
		},
		Admin: &bootstrap.Admin{
			AccessLogPath: stringOrDefault(c.AdminAccessLogPath, "/dev/null"),
			Address:       SocketAddress(stringOrDefault(c.AdminAddress, "127.0.0.1"), intOrDefault(c.AdminPort, 9001)),
		},
	}

	if c.StatsdEnabled {
		b.StatsSinks = []*metrics.StatsSink{{
			Name: util.Statsd,
			ConfigType: &metrics.StatsSink_Config{
				Config: &types.Struct{
					Fields: map[string]*types.Value{
						"address": st(map[string]*types.Value{
							"socket_address": st(map[string]*types.Value{
								"protocol":   sv("UDP"),
								"address":    sv(stringOrDefault(c.StatsdAddress, "127.0.0.1")),
								"port_value": sv(fmt.Sprintf("%d", intOrDefault(c.StatsdPort, 9125))),
							}),
						}),
					},
				},
			},
		}}
	}
	return b
}

func stringOrDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func intOrDefault(i, def int) int {
	if i == 0 {
		return def
	}
	return i
}

// BootstrapConfig holds configuration values for a v2.Bootstrap.
type BootstrapConfig struct {
	// AdminAccessLogPath is the path to write the access log for the administration server.
	// Defaults to /dev/null.
	AdminAccessLogPath string

	// AdminAddress is the TCP address that the administration server will listen on.
	// Defaults to 127.0.0.1.
	AdminAddress string

	// AdminPort is the port that the administration server will listen on.
	// Defaults to 9001.
	AdminPort int

	// StatsAddress is the address that the /stats path will listen on.
	// Defaults to 0.0.0.0 and is only enabled if StatsdEnabled is true.
	StatsAddress string

	// StatsPort is the port that the /stats path will listen on.
	// Defaults to 8002 and is only enabled if StatsdEnabled is true.
	StatsPort int

	// XDSAddress is the TCP address of the gRPC XDS management server.
	// Defaults to 127.0.0.1.
	XDSAddress string

	// XDSGRPCPort is the management server port that provides the v2 gRPC API.
	// Defaults to 8001.
	XDSGRPCPort int

	// StatsdEnabled enables metrics output via statsd
	// Defaults to false.
	StatsdEnabled bool

	// StatsdAddress is the UDP address of the statsd endpoint
	// Defaults to 127.0.0.1.
	StatsdAddress string

	// StatsdPort is port of the statsd endpoint
	// Defaults to 9125.
	StatsdPort int

	// Namespace is the namespace where Contour is running
	Namespace string
}
