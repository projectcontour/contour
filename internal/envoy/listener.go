// Copyright © 2019 VMware
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
	envoy_api_v2_auth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	envoy_api_v2_core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoy_api_v2_listener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	accesslog "github.com/envoyproxy/go-control-plane/envoy/config/filter/accesslog/v2"
	http "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	tcp "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/tcp_proxy/v2"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/protobuf"
	"github.com/projectcontour/contour/internal/sorter"
)

// TLSInspector returns a new TLS inspector listener filter.
func TLSInspector() *envoy_api_v2_listener.ListenerFilter {
	return &envoy_api_v2_listener.ListenerFilter{
		Name: wellknown.TlsInspector,
	}
}

// ProxyProtocol returns a new Proxy Protocol listener filter.
func ProxyProtocol() *envoy_api_v2_listener.ListenerFilter {
	return &envoy_api_v2_listener.ListenerFilter{
		Name: wellknown.ProxyProtocol,
	}
}

// Listener returns a new v2.Listener for the supplied address, port, and filters.
func Listener(name, address string, port int, lf []*envoy_api_v2_listener.ListenerFilter, filters ...*envoy_api_v2_listener.Filter) *v2.Listener {
	l := &v2.Listener{
		Name:            name,
		Address:         SocketAddress(address, port),
		ListenerFilters: lf,
	}
	if len(filters) > 0 {
		l.FilterChains = append(
			l.FilterChains,
			&envoy_api_v2_listener.FilterChain{
				Filters: filters,
			},
		)
	}
	return l
}

type httpConnectionManagerBuilder struct {
	routeConfigName string
	metricsPrefix   string
	accessLoggers   []*accesslog.AccessLog
	requestTimeout  time.Duration
}

// RouteConfigName sets the name of the RDS element that contains
// the routing table for this manager.
func (b *httpConnectionManagerBuilder) RouteConfigName(name string) *httpConnectionManagerBuilder {
	b.routeConfigName = name
	return b
}

// MetricsPrefix sets the prefix used for emitting metrics from the
// connection manager. Note that this prefix is externally visible in
// monitoring tools, so it is subject to compatibility concerns.
//
// See https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_conn_man/stats#config-http-conn-man-stats
func (b *httpConnectionManagerBuilder) MetricsPrefix(prefix string) *httpConnectionManagerBuilder {
	b.metricsPrefix = prefix
	return b
}

// AccessLoggers sets the access logging configuration.
func (b *httpConnectionManagerBuilder) AccessLoggers(loggers []*accesslog.AccessLog) *httpConnectionManagerBuilder {
	b.accessLoggers = loggers
	return b
}

// RequestTimeout sets the active request timeout on the connection
// manager. If not specified or set to 0, this timeout is disabled.
func (b *httpConnectionManagerBuilder) RequestTimeout(timeout time.Duration) *httpConnectionManagerBuilder {
	b.requestTimeout = timeout
	return b
}

// Get returns a new http.HttpConnectionManager filter, constructed
// from the builder settings.
//
// See https://www.envoyproxy.io/docs/envoy/latest/api-v2/config/filter/network/http_connection_manager/v2/http_connection_manager.proto.html
func (b *httpConnectionManagerBuilder) Get() *envoy_api_v2_listener.Filter {
	cm := &http.HttpConnectionManager{
		RouteSpecifier: &http.HttpConnectionManager_Rds{
			Rds: &http.Rds{
				RouteConfigName: b.routeConfigName,
				ConfigSource: &envoy_api_v2_core.ConfigSource{
					ConfigSourceSpecifier: &envoy_api_v2_core.ConfigSource_ApiConfigSource{
						ApiConfigSource: &envoy_api_v2_core.ApiConfigSource{
							ApiType: envoy_api_v2_core.ApiConfigSource_GRPC,
							GrpcServices: []*envoy_api_v2_core.GrpcService{{
								TargetSpecifier: &envoy_api_v2_core.GrpcService_EnvoyGrpc_{
									EnvoyGrpc: &envoy_api_v2_core.GrpcService_EnvoyGrpc{
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
			Name: wellknown.Gzip,
		}, {
			Name: wellknown.GRPCWeb,
		}, {
			Name: wellknown.Router,
		}},
		CommonHttpProtocolOptions: &envoy_api_v2_core.HttpProtocolOptions{
			// Sets the idle timeout for HTTP connections to 60 seconds.
			// This is chosen as a rough default to stop idle connections wasting resources,
			// without stopping slow connections from being terminated too quickly.
			IdleTimeout: protobuf.Duration(60 * time.Second),
		},
		HttpProtocolOptions: &envoy_api_v2_core.Http1ProtocolOptions{
			// Enable support for HTTP/1.0 requests that carry
			// a Host: header. See #537.
			AcceptHttp_10: true,
		},
		UseRemoteAddress: protobuf.Bool(true),
		NormalizePath:    protobuf.Bool(true),
		RequestTimeout:   protobuf.Duration(b.requestTimeout),

		// issue #1487 pass through X-Request-Id if provided.
		PreserveExternalRequestId: true,
		MergeSlashes:              true,
	}

	if len(b.accessLoggers) > 0 {
		cm.AccessLog = b.accessLoggers
	}

	// If there's no explicit metrics prefix, default it to the
	// route config name.
	if b.metricsPrefix != "" {
		cm.StatPrefix = b.metricsPrefix
	} else {
		cm.StatPrefix = b.routeConfigName
	}

	return &envoy_api_v2_listener.Filter{
		Name: wellknown.HTTPConnectionManager,
		ConfigType: &envoy_api_v2_listener.Filter_TypedConfig{
			TypedConfig: toAny(cm),
		},
	}
}

// HTTPConnectionManager creates a new HTTP Connection Manager filter
// for the supplied route, access log, and client request timeout.
func HTTPConnectionManager(routename string, accesslogger []*accesslog.AccessLog, requestTimeout time.Duration) *envoy_api_v2_listener.Filter {
	return HTTPConnectionManagerBuilder().
		RouteConfigName(routename).
		MetricsPrefix(routename).
		AccessLoggers(accesslogger).
		RequestTimeout(requestTimeout).
		Get()
}

func HTTPConnectionManagerBuilder() *httpConnectionManagerBuilder {
	return &httpConnectionManagerBuilder{}
}

// TCPProxy creates a new TCPProxy filter.
func TCPProxy(statPrefix string, proxy *dag.TCPProxy, accesslogger []*accesslog.AccessLog) *envoy_api_v2_listener.Filter {
	// Set the idle timeout in seconds for connections through a TCP Proxy type filter.
	// The value of two and a half hours for reasons documented at
	// https://github.com/projectcontour/contour/issues/1074
	// Set to 9001 because now it's OVER NINE THOUSAND.
	idleTimeout := protobuf.Duration(9001 * time.Second)

	switch len(proxy.Clusters) {
	case 1:
		return &envoy_api_v2_listener.Filter{
			Name: wellknown.TCPProxy,
			ConfigType: &envoy_api_v2_listener.Filter_TypedConfig{
				TypedConfig: toAny(&tcp.TcpProxy{
					StatPrefix: statPrefix,
					ClusterSpecifier: &tcp.TcpProxy_Cluster{
						Cluster: Clustername(proxy.Clusters[0]),
					},
					AccessLog:   accesslogger,
					IdleTimeout: idleTimeout,
				}),
			},
		}
	default:
		var clusters []*tcp.TcpProxy_WeightedCluster_ClusterWeight
		for _, c := range proxy.Clusters {
			weight := c.Weight
			if weight == 0 {
				weight = 1
			}
			clusters = append(clusters, &tcp.TcpProxy_WeightedCluster_ClusterWeight{
				Name:   Clustername(c),
				Weight: weight,
			})
		}
		sort.Stable(sorter.For(clusters))
		return &envoy_api_v2_listener.Filter{
			Name: wellknown.TCPProxy,
			ConfigType: &envoy_api_v2_listener.Filter_TypedConfig{
				TypedConfig: toAny(&tcp.TcpProxy{
					StatPrefix: statPrefix,
					ClusterSpecifier: &tcp.TcpProxy_WeightedClusters{
						WeightedClusters: &tcp.TcpProxy_WeightedCluster{
							Clusters: clusters,
						},
					},
					AccessLog:   accesslogger,
					IdleTimeout: idleTimeout,
				}),
			},
		}
	}
}

// SocketAddress creates a new TCP envoy_api_v2_core.Address.
func SocketAddress(address string, port int) *envoy_api_v2_core.Address {
	if address == "::" {
		return &envoy_api_v2_core.Address{
			Address: &envoy_api_v2_core.Address_SocketAddress{
				SocketAddress: &envoy_api_v2_core.SocketAddress{
					Protocol:   envoy_api_v2_core.SocketAddress_TCP,
					Address:    address,
					Ipv4Compat: true,
					PortSpecifier: &envoy_api_v2_core.SocketAddress_PortValue{
						PortValue: uint32(port),
					},
				},
			},
		}
	}
	return &envoy_api_v2_core.Address{
		Address: &envoy_api_v2_core.Address_SocketAddress{
			SocketAddress: &envoy_api_v2_core.SocketAddress{
				Protocol: envoy_api_v2_core.SocketAddress_TCP,
				Address:  address,
				PortSpecifier: &envoy_api_v2_core.SocketAddress_PortValue{
					PortValue: uint32(port),
				},
			},
		},
	}
}

// Filters returns a []*envoy_api_v2_listener.Filter for the supplied filters.
func Filters(filters ...*envoy_api_v2_listener.Filter) []*envoy_api_v2_listener.Filter {
	if len(filters) == 0 {
		return nil
	}
	return filters
}

// FilterChain retruns a *envoy_api_v2_listener.FilterChain for the supplied filters.
func FilterChain(filters ...*envoy_api_v2_listener.Filter) *envoy_api_v2_listener.FilterChain {
	return &envoy_api_v2_listener.FilterChain{
		Filters: filters,
	}
}

// FilterChains returns a []*envoy_api_v2_listener.FilterChain for the supplied filters.
func FilterChains(filters ...*envoy_api_v2_listener.Filter) []*envoy_api_v2_listener.FilterChain {
	if len(filters) == 0 {
		return nil
	}
	return []*envoy_api_v2_listener.FilterChain{
		FilterChain(filters...),
	}
}

// FilterChainTLS returns a TLS enabled envoy_api_v2_listener.FilterChain,
func FilterChainTLS(domain string, downstream *envoy_api_v2_auth.DownstreamTlsContext, filters []*envoy_api_v2_listener.Filter) *envoy_api_v2_listener.FilterChain {
	fc := &envoy_api_v2_listener.FilterChain{
		Filters: filters,
		FilterChainMatch: &envoy_api_v2_listener.FilterChainMatch{
			ServerNames: []string{domain},
		},
	}
	// Attach TLS data to this listener if provided.
	if downstream != nil {
		fc.TransportSocket = DownstreamTLSTransportSocket(downstream)

	}
	return fc
}

// ListenerFilters returns a []*envoy_api_v2_listener.ListenerFilter for the supplied listener filters.
func ListenerFilters(filters ...*envoy_api_v2_listener.ListenerFilter) []*envoy_api_v2_listener.ListenerFilter {
	return filters
}

func toAny(pb proto.Message) *any.Any {
	a, err := ptypes.MarshalAny(pb)
	if err != nil {
		panic(err.Error())
	}
	return a
}
