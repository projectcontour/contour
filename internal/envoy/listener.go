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

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	http "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	tcp "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/tcp_proxy/v2"
	"github.com/envoyproxy/go-control-plane/pkg/util"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"github.com/heptio/contour/internal/dag"
)

// HTTPDefaultIdleTimeout sets the idle timeout for HTTP connections
// to 60 seconds. This is chosen as a rough default to stop idle connections
// wasting resources, without stopping slow connections from being terminated
// too quickly.
// Exported so the same value can be used here and in e2e tests.
const HTTPDefaultIdleTimeout = 60 * time.Second

// TCPDefaultIdleTimeout sets the idle timeout in seconds for
// connections through a TCP Proxy type filter.
// It's defaulted to two and a half hours for reasons documented at
// https://github.com/heptio/contour/issues/1074
// Set to 9001 because now it's OVER NINE THOUSAND.
// Exported so the same value can be used here and in e2e tests.
const TCPDefaultIdleTimeout = 9001 * time.Second

// TLSInspector returns a new TLS inspector listener filter.
func TLSInspector() listener.ListenerFilter {
	return listener.ListenerFilter{
		Name: util.TlsInspector,
	}
}

// ProxyProtocol returns a new Proxy Protocol listener filter.
func ProxyProtocol() listener.ListenerFilter {
	return listener.ListenerFilter{
		Name: util.ProxyProtocol,
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

func idleTimeout(d time.Duration) *time.Duration {
	return &d
}

// HTTPConnectionManager creates a new HTTP Connection Manager filter
// for the supplied route and access log.
func HTTPConnectionManager(routename, accessLogPath string) listener.Filter {
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
				IdleTimeout:      idleTimeout(HTTPDefaultIdleTimeout),
			}),
		},
	}
}

// TCPProxy creates a new TCPProxy filter.
func TCPProxy(statPrefix string, proxy *dag.TCPProxy, accessLogPath string) listener.Filter {
	tcpIdleTimeout := idleTimeout(TCPDefaultIdleTimeout)
	switch len(proxy.Clusters) {
	case 1:
		return listener.Filter{
			Name: util.TCPProxy,
			ConfigType: &listener.Filter_TypedConfig{
				TypedConfig: any(&tcp.TcpProxy{
					StatPrefix: statPrefix,
					ClusterSpecifier: &tcp.TcpProxy_Cluster{
						Cluster: Clustername(proxy.Clusters[0]),
					},
					AccessLog:   FileAccessLog(accessLogPath),
					IdleTimeout: tcpIdleTimeout,
				}),
			},
		}
	default:
		var clusters []*tcp.TcpProxy_WeightedCluster_ClusterWeight
		for _, c := range proxy.Clusters {
			weight := uint32(c.Weight)
			if weight == 0 {
				weight = 1
			}
			clusters = append(clusters, &tcp.TcpProxy_WeightedCluster_ClusterWeight{
				Name:   Clustername(c),
				Weight: weight,
			})
		}
		sort.Stable(clustersByNameAndWeight(clusters))
		return listener.Filter{
			Name: util.TCPProxy,
			ConfigType: &listener.Filter_TypedConfig{
				TypedConfig: any(&tcp.TcpProxy{
					StatPrefix: statPrefix,
					ClusterSpecifier: &tcp.TcpProxy_WeightedClusters{
						WeightedClusters: &tcp.TcpProxy_WeightedCluster{
							Clusters: clusters,
						},
					},
					AccessLog:   FileAccessLog(accessLogPath),
					IdleTimeout: tcpIdleTimeout,
				}),
			},
		}
	}
}

type clustersByNameAndWeight []*tcp.TcpProxy_WeightedCluster_ClusterWeight

func (c clustersByNameAndWeight) Len() int      { return len(c) }
func (c clustersByNameAndWeight) Swap(i, j int) { c[i], c[j] = c[j], c[i] }
func (c clustersByNameAndWeight) Less(i, j int) bool {
	if c[i].Name == c[j].Name {
		return c[i].Weight < c[j].Weight
	}
	return c[i].Name < c[j].Name
}

// SocketAddress creates a new TCP core.Address.
func SocketAddress(address string, port int) *core.Address {
	if address == "::" {
		return &core.Address{
			Address: &core.Address_SocketAddress{
				SocketAddress: &core.SocketAddress{
					Protocol:   core.TCP,
					Address:    address,
					Ipv4Compat: true,
					PortSpecifier: &core.SocketAddress_PortValue{
						PortValue: uint32(port),
					},
				},
			},
		}
	}
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

func any(pb proto.Message) *types.Any {
	any, err := types.MarshalAny(pb)
	if err != nil {
		panic(err.Error())
	}
	return any
}
