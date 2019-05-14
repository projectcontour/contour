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
	"time"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	http "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	"github.com/envoyproxy/go-control-plane/pkg/util"
	"github.com/gogo/protobuf/proto"
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
	duration := func(d time.Duration) *time.Duration {
		return &d
	}
	return listener.Filter{
		Name: util.HTTPConnectionManager,
		ConfigType: &listener.Filter_TypedConfig{
			TypedConfig: any(&http.HttpConnectionManager{
				StatPrefix: routename,
				RouteSpecifier: &http.HttpConnectionManager_Rds{
					Rds: &http.Rds{
						RouteConfigName: routename,
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
				AccessLog:        FileAccessLog(accessLogPath),
				UseRemoteAddress: &types.BoolValue{Value: true}, // TODO(jbeda) should this ever be false?
				NormalizePath:    &types.BoolValue{Value: true},
				IdleTimeout:      duration(60 * time.Second),
			}),
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
						"access_log": lv(
							st(map[string]*types.Value{
								"name": sv(util.FileAccessLog),
								"config": st(map[string]*types.Value{
									"path": sv(accessLogPath),
								}),
							}),
						),
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
						"access_log": lv(
							st(map[string]*types.Value{
								"name": sv(util.FileAccessLog),
								"config": st(map[string]*types.Value{
									"path": sv(accessLogPath),
								}),
							}),
						),
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

func any(pb proto.Message) *types.Any {
	any, err := types.MarshalAny(pb)
	if err != nil {
		panic(err.Error())
	}
	return any
}
