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

package contour

import (
	"sync"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"github.com/heptio/contour/internal/dag"
	"github.com/heptio/contour/internal/envoy"
	"k8s.io/api/core/v1"
)

// ListenerCache manages the contents of the gRPC LDS cache.
type ListenerCache struct {
	// Envoy's HTTP (non TLS) listener address.
	// If not set, defaults to DEFAULT_HTTP_LISTENER_ADDRESS.
	HTTPAddress string

	// Envoy's HTTP (non TLS) listener port.
	// If not set, defaults to DEFAULT_HTTP_LISTENER_PORT.
	HTTPPort int

	// Envoy's HTTP (non TLS) access log path.
	// If not set, defaults to DEFAULT_HTTP_ACCESS_LOG.
	HTTPAccessLog string

	// Envoy's HTTPS (TLS) listener address.
	// If not set, defaults to DEFAULT_HTTPS_LISTENER_ADDRESS.
	HTTPSAddress string

	// Envoy's HTTPS (TLS) listener port.
	// If not set, defaults to DEFAULT_HTTPS_LISTENER_PORT.
	HTTPSPort int

	// Envoy's HTTPS (TLS) access log path.
	// If not set, defaults to DEFAULT_HTTPS_ACCESS_LOG.
	HTTPSAccessLog string

	// UseProxyProto configurs all listeners to expect a PROXY protocol
	// V1 header on new connections.
	// If not set, defaults to false.
	UseProxyProto bool

	listenerCache
}

// httpAddress returns the port for the HTTP (non TLS)
// listener or DEFAULT_HTTP_LISTENER_ADDRESS if not configured.
func (lc *ListenerCache) httpAddress() string {
	if lc.HTTPAddress != "" {
		return lc.HTTPAddress
	}
	return DEFAULT_HTTP_LISTENER_ADDRESS
}

// httpPort returns the port for the HTTP (non TLS)
// listener or DEFAULT_HTTP_LISTENER_PORT if not configured.
func (lc *ListenerCache) httpPort() uint32 {
	if lc.HTTPPort != 0 {
		return uint32(lc.HTTPPort)
	}
	return DEFAULT_HTTP_LISTENER_PORT
}

// httpAccessLog returns the access log for the HTTP (non TLS)
// listener or DEFAULT_HTTP_ACCESS_LOG if not configured.
func (lc *ListenerCache) httpAccessLog() string {
	if lc.HTTPAccessLog != "" {
		return lc.HTTPAccessLog
	}
	return DEFAULT_HTTP_ACCESS_LOG
}

// httpsAddress returns the port for the HTTPS (TLS)
// listener or DEFAULT_HTTPS_LISTENER_ADDRESS if not configured.
func (lc *ListenerCache) httpsAddress() string {
	if lc.HTTPSAddress != "" {
		return lc.HTTPSAddress
	}
	return DEFAULT_HTTPS_LISTENER_ADDRESS
}

// httpsPort returns the port for the HTTPS (TLS) listener
// or DEFAULT_HTTPS_LISTENER_PORT if not configured.
func (lc *ListenerCache) httpsPort() uint32 {
	if lc.HTTPSPort != 0 {
		return uint32(lc.HTTPSPort)
	}
	return DEFAULT_HTTPS_LISTENER_PORT
}

// httpsAccessLog returns the access log for the HTTPS (TLS)
// listener or DEFAULT_HTTPS_ACCESS_LOG if not configured.
func (lc *ListenerCache) httpsAccessLog() string {
	if lc.HTTPSAccessLog != "" {
		return lc.HTTPSAccessLog
	}
	return DEFAULT_HTTPS_ACCESS_LOG
}

type listenerCache struct {
	mu      sync.Mutex
	values  map[string]*v2.Listener
	waiters []chan int
	last    int
}

// Register registers ch to receive a value when Notify is called.
// The value of last is the count of the times Notify has been called on this Cache.
// It functions of a sequence counter, if the value of last supplied to Register
// is less than the Cache's internal counter, then the caller has missed at least
// one notification and will fire immediately.
//
// Sends by the broadcaster to ch must not block, therefor ch must have a capacity
// of at least 1.
func (c *listenerCache) Register(ch chan int, last int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if last < c.last {
		// notify this channel immediately
		ch <- c.last
		return
	}
	c.waiters = append(c.waiters, ch)
}

// Update replaces the contents of the cache with the supplied map.
func (c *listenerCache) Update(v map[string]*v2.Listener) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.values = v
	c.notify()
}

// notify notifies all registered waiters that an event has occurred.
func (c *listenerCache) notify() {
	c.last++

	for _, ch := range c.waiters {
		ch <- c.last
	}
	c.waiters = c.waiters[:0]
}

// Values returns a slice of the value stored in the cache.
func (c *listenerCache) Values(filter func(string) bool) []proto.Message {
	c.mu.Lock()
	values := make([]proto.Message, 0, len(c.values))
	for _, v := range c.values {
		if filter(v.Name) {
			values = append(values, v)
		}
	}
	c.mu.Unlock()
	return values
}

const (
	ENVOY_HTTP_LISTENER            = "ingress_http"
	ENVOY_HTTPS_LISTENER           = "ingress_https"
	DEFAULT_HTTP_ACCESS_LOG        = "/dev/stdout"
	DEFAULT_HTTP_LISTENER_ADDRESS  = "0.0.0.0"
	DEFAULT_HTTP_LISTENER_PORT     = 8080
	DEFAULT_HTTPS_ACCESS_LOG       = "/dev/stdout"
	DEFAULT_HTTPS_LISTENER_ADDRESS = DEFAULT_HTTP_LISTENER_ADDRESS
	DEFAULT_HTTPS_LISTENER_PORT    = 8443

	router     = "envoy.router"
	grpcWeb    = "envoy.grpc_web"
	gzip       = "envoy.gzip"
	httpFilter = "envoy.http_connection_manager"
	accessLog  = "envoy.file_access_log"
)

type listenerVisitor struct {
	*ListenerCache
	dag.Visitable
}

func (v *listenerVisitor) Visit() map[string]*v2.Listener {
	m := make(map[string]*v2.Listener)
	http := 0
	ingress_https := v2.Listener{
		Name:    ENVOY_HTTPS_LISTENER,
		Address: socketaddress(v.httpsAddress(), v.httpsPort()),
		ListenerFilters: []listener.ListenerFilter{
			envoy.TLSInspector(),
		},
	}
	filters := []listener.Filter{
		httpfilter(ENVOY_HTTPS_LISTENER, v.httpsAccessLog()),
	}
	v.Visitable.Visit(func(vh dag.Vertex) {
		switch vh := vh.(type) {
		case *dag.VirtualHost:
			// we only create on http listener so record the fact
			// that we need to then double back at the end and add
			// the listener properly.
			http++
		case *dag.SecureVirtualHost:
			data := vh.Data()
			if data == nil {
				// no secret for this vhost, skip it
				return
			}
			fc := listener.FilterChain{
				FilterChainMatch: &listener.FilterChainMatch{
					ServerNames: []string{vh.Host},
				},
				TlsContext: tlscontext(data, vh.MinProtoVersion, "h2", "http/1.1"),
				Filters:    filters,
			}
			if v.UseProxyProto {
				fc.UseProxyProto = bv(true)
			}
			ingress_https.FilterChains = append(ingress_https.FilterChains, fc)
		}
	})
	if http > 0 {
		m[ENVOY_HTTP_LISTENER] = &v2.Listener{
			Name:    ENVOY_HTTP_LISTENER,
			Address: socketaddress(v.httpAddress(), v.httpPort()),
			FilterChains: []listener.FilterChain{
				filterchain(v.UseProxyProto, httpfilter(ENVOY_HTTP_LISTENER, v.httpAccessLog())),
			},
		}
	}
	if len(ingress_https.FilterChains) > 0 {
		m[ENVOY_HTTPS_LISTENER] = &ingress_https
	}
	return m
}

func socketaddress(address string, port uint32) core.Address {
	return core.Address{
		Address: &core.Address_SocketAddress{
			SocketAddress: &core.SocketAddress{
				Protocol: core.TCP,
				Address:  address,
				PortSpecifier: &core.SocketAddress_PortValue{
					PortValue: port,
				},
			},
		},
	}
}

func filterchain(useproxy bool, filters ...listener.Filter) listener.FilterChain {
	fc := listener.FilterChain{
		Filters: filters,
	}
	if useproxy {
		fc.UseProxyProto = bv(true)
	}
	return fc
}

func httpfilter(routename, accessLogPath string) listener.Filter {
	return listener.Filter{
		Name: httpFilter,
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
						"name": sv(gzip),
					}),
					st(map[string]*types.Value{
						"name": sv(grpcWeb),
					}),
					st(map[string]*types.Value{
						"name": sv(router),
					}),
				),
				"use_remote_address": {Kind: &types.Value_BoolValue{BoolValue: true}}, // TODO(jbeda) should this ever be false?
				"access_log":         accesslog(accessLogPath),
			},
		},
	}
}

func tlscontext(data map[string][]byte, tlsMinProtoVersion auth.TlsParameters_TlsProtocol, alpnprotos ...string) *auth.DownstreamTlsContext {
	return &auth.DownstreamTlsContext{
		CommonTlsContext: &auth.CommonTlsContext{
			TlsParams: &auth.TlsParameters{
				TlsMinimumProtocolVersion: tlsMinProtoVersion,
				TlsMaximumProtocolVersion: auth.TlsParameters_TLSv1_3,
			},
			TlsCertificates: []*auth.TlsCertificate{{
				CertificateChain: &core.DataSource{
					Specifier: &core.DataSource_InlineBytes{
						InlineBytes: data[v1.TLSCertKey],
					},
				},
				PrivateKey: &core.DataSource{
					Specifier: &core.DataSource_InlineBytes{
						InlineBytes: data[v1.TLSPrivateKeyKey],
					},
				},
			}},
			AlpnProtocols: alpnprotos,
		},
	}
}

func accesslog(path string) *types.Value {
	return lv(
		st(map[string]*types.Value{
			"name": sv(accessLog),
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
