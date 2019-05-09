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
	"sort"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	"github.com/envoyproxy/go-control-plane/pkg/util"
	"github.com/gogo/protobuf/types"
	"github.com/heptio/contour/internal/dag"
)

// TLSInspector returns a new TLS inspector listener filter.
func TLSInspector() listener.ListenerFilter {
	return listener.ListenerFilter{
		Name: util.TlsInspector,
		ConfigType: &listener.ListenerFilter_Config{
			Config: new(types.Struct),
		},
	}
}

// ProxyProtocol returns a new Proxy Protocol listener filter.
func ProxyProtocol() listener.ListenerFilter {
	return listener.ListenerFilter{
		Name: util.ProxyProtocol,
		ConfigType: &listener.ListenerFilter_Config{
			Config: new(types.Struct),
		},
	}
}

// Listener returns a new v2.Listener for the supplied address, port, and filters.
func Listener(name, address string, port int, lf []listener.ListenerFilter, filters ...listener.Filter) *v2.Listener {
	l := &v2.Listener{
		Name:            name,
		Address:         *SocketAddress(address, port),
		ListenerFilters: lf,
	}
	if len(filters) > 0 {
		l.FilterChains = append(
			l.FilterChains,
			listener.FilterChain{
				Filters: filters,
			},
		)
	}
	return l
}

// HTTPConnectionManager creates a new HTTP Connection Manager filter
// for the supplied route and access log.
func HTTPConnectionManager(routename, accessLogPath string) listener.Filter {
	return listener.Filter{
		Name: util.HTTPConnectionManager,
		ConfigType: &listener.Filter_Config{
			Config: &types.Struct{
				Fields: map[string]*types.Value{
					"stat_prefix": sv(routename),
					"rds": st(map[string]*types.Value{
						"route_config_name": sv(routename),
						"config_source": st(map[string]*types.Value{
							"api_config_source": st(map[string]*types.Value{
								"api_type": sv("GRPC"),
								"grpc_services": lv(
									st(map[string]*types.Value{
										"envoy_grpc": st(map[string]*types.Value{
											"cluster_name": sv("contour"),
										}),
									}),
								),
							}),
						}),
					}),
					"http_filters": lv(
						st(map[string]*types.Value{
							"name": sv(util.Gzip),
						}),
						st(map[string]*types.Value{
							"name": sv(util.GRPCWeb),
						}),
						st(map[string]*types.Value{
							"name": sv(util.Router),
						}),
					),
					"http_protocol_options": st(map[string]*types.Value{
						"accept_http_10": {Kind: &types.Value_BoolValue{BoolValue: true}},
					}),
					"access_log":         accesslog(accessLogPath),
					"use_remote_address": {Kind: &types.Value_BoolValue{BoolValue: true}}, // TODO(jbeda) should this ever be false?
					"normalize_path":     {Kind: &types.Value_BoolValue{BoolValue: true}},
					"idle_timeout":       sv("60s"),
				},
			},
		},
	}
}

// TCPProxy creates a new TCPProxy filter.
func TCPProxy(statPrefix string, proxy *dag.TCPProxy, accessLogPath string) listener.Filter {
	switch len(proxy.Clusters) {
	case 1:
		return listener.Filter{
			Name: util.TCPProxy,
			ConfigType: &listener.Filter_Config{
				Config: &types.Struct{
					Fields: map[string]*types.Value{
						"stat_prefix": sv(statPrefix),
						"cluster":     sv(Clustername(proxy.Clusters[0])),
						"access_log":  accesslog(accessLogPath),
					},
				},
			},
		}
	default:
		// its easier to sort the input of the cluster list rather than the
		// grpc type output. We have to make a copy to avoid mutating the dag.
		clusters := make([]*dag.Cluster, len(proxy.Clusters))
		copy(clusters, proxy.Clusters)
		sort.Stable(tcpServiceByName(clusters))
		var l []*types.Value
		for _, cluster := range clusters {
			weight := cluster.Weight
			if weight == 0 {
				weight = 1
			}
			l = append(l, st(map[string]*types.Value{
				"name":   sv(Clustername(cluster)),
				"weight": nv(float64(weight)),
			}))
		}
		return listener.Filter{
			Name: util.TCPProxy,
			ConfigType: &listener.Filter_Config{
				Config: &types.Struct{
					Fields: map[string]*types.Value{
						"stat_prefix": sv(statPrefix),
						"weighted_clusters": st(map[string]*types.Value{
							"clusters": lv(l...),
						}),
						"access_log": accesslog(accessLogPath),
					},
				},
			},
		}
	}
}

type tcpServiceByName []*dag.Cluster

func (t tcpServiceByName) Len() int      { return len(t) }
func (t tcpServiceByName) Swap(i, j int) { t[i], t[j] = t[j], t[i] }
func (t tcpServiceByName) Less(i, j int) bool {
	a, b := t[i].Upstream.(*dag.TCPService), t[j].Upstream.(*dag.TCPService)
	if a.Name == b.Name {
		return t[i].Weight < t[j].Weight
	}
	return a.Name < b.Name
}

// SocketAddress creates a new TCP core.Address.
func SocketAddress(address string, port int) *core.Address {
	return &core.Address{
		Address: &core.Address_SocketAddress{
			SocketAddress: &core.SocketAddress{
				Protocol: core.TCP,
				Address:  address,
				PortSpecifier: &core.SocketAddress_PortValue{
					PortValue: uint32(port),
				},
			},
		},
	}
}

func accesslog(path string) *types.Value {
	return lv(
		st(map[string]*types.Value{
			"name": sv(util.FileAccessLog),
			"config": st(map[string]*types.Value{
				"path": sv(path),
			}),
		}),
	)
}

func sv(s string) *types.Value {
	return &types.Value{Kind: &types.Value_StringValue{StringValue: s}}
}

func st(m map[string]*types.Value) *types.Value {
	return &types.Value{Kind: &types.Value_StructValue{StructValue: &types.Struct{Fields: m}}}
}

func lv(v ...*types.Value) *types.Value {
	return &types.Value{Kind: &types.Value_ListValue{ListValue: &types.ListValue{Values: v}}}
}

func nv(n float64) *types.Value {
	return &types.Value{Kind: &types.Value_NumberValue{NumberValue: n}}
}
