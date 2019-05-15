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

	"github.com/gogo/protobuf/types"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	health_check "github.com/envoyproxy/go-control-plane/envoy/config/filter/http/health_check/v2"
	http "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	"github.com/envoyproxy/go-control-plane/pkg/util"
	"github.com/gogo/protobuf/proto"
	"github.com/heptio/contour/internal/dag"
	"github.com/heptio/contour/internal/envoy"
)

const (
	ENVOY_HTTP_LISTENER            = "ingress_http"
	ENVOY_HTTPS_LISTENER           = "ingress_https"
	DEFAULT_HTTP_ACCESS_LOG        = "/dev/stdout"
	DEFAULT_HTTP_LISTENER_ADDRESS  = "0.0.0.0"
	DEFAULT_HTTP_LISTENER_PORT     = 8080
	DEFAULT_HTTPS_ACCESS_LOG       = "/dev/stdout"
	DEFAULT_HTTPS_LISTENER_ADDRESS = DEFAULT_HTTP_LISTENER_ADDRESS
	DEFAULT_HTTPS_LISTENER_PORT    = 8443
)

// ListenerVisitorConfig holds configuration parameters for visitListeners.
type ListenerVisitorConfig struct {
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

	// UseProxyProto configurs all listeners to expect a PROXY
	// V1 or V2 preamble.
	// If not set, defaults to false.
	UseProxyProto bool
}

// httpAddress returns the port for the HTTP (non TLS)
// listener or DEFAULT_HTTP_LISTENER_ADDRESS if not configured.
func (lvc *ListenerVisitorConfig) httpAddress() string {
	if lvc.HTTPAddress != "" {
		return lvc.HTTPAddress
	}
	return DEFAULT_HTTP_LISTENER_ADDRESS
}

// httpPort returns the port for the HTTP (non TLS)
// listener or DEFAULT_HTTP_LISTENER_PORT if not configured.
func (lvc *ListenerVisitorConfig) httpPort() int {
	if lvc.HTTPPort != 0 {
		return lvc.HTTPPort
	}
	return DEFAULT_HTTP_LISTENER_PORT
}

// httpAccessLog returns the access log for the HTTP (non TLS)
// listener or DEFAULT_HTTP_ACCESS_LOG if not configured.
func (lvc *ListenerVisitorConfig) httpAccessLog() string {
	if lvc.HTTPAccessLog != "" {
		return lvc.HTTPAccessLog
	}
	return DEFAULT_HTTP_ACCESS_LOG
}

// httpsAddress returns the port for the HTTPS (TLS)
// listener or DEFAULT_HTTPS_LISTENER_ADDRESS if not configured.
func (lvc *ListenerVisitorConfig) httpsAddress() string {
	if lvc.HTTPSAddress != "" {
		return lvc.HTTPSAddress
	}
	return DEFAULT_HTTPS_LISTENER_ADDRESS
}

// httpsPort returns the port for the HTTPS (TLS) listener
// or DEFAULT_HTTPS_LISTENER_PORT if not configured.
func (lvc *ListenerVisitorConfig) httpsPort() int {
	if lvc.HTTPSPort != 0 {
		return lvc.HTTPSPort
	}
	return DEFAULT_HTTPS_LISTENER_PORT
}

// httpsAccessLog returns the access log for the HTTPS (TLS)
// listener or DEFAULT_HTTPS_ACCESS_LOG if not configured.
func (lvc *ListenerVisitorConfig) httpsAccessLog() string {
	if lvc.HTTPSAccessLog != "" {
		return lvc.HTTPSAccessLog
	}
	return DEFAULT_HTTPS_ACCESS_LOG
}

// ListenerCache manages the contents of the gRPC LDS cache.
type ListenerCache struct {
	mu           sync.Mutex
	values       map[string]*v2.Listener
	staticValues map[string]*v2.Listener
	waiters      []chan int
	last         int
}

// NewListenerCache returns an instance of a ListenerCache
func NewListenerCache(address string, port int) ListenerCache {
	return ListenerCache{
		staticValues: map[string]*v2.Listener{
			"stats-health": {
				Name:    "stats-health",
				Address: *envoy.SocketAddress(address, port),
				FilterChains: []listener.FilterChain{{
					Filters: []listener.Filter{{
						Name: util.HTTPConnectionManager,
						ConfigType: &listener.Filter_TypedConfig{
							TypedConfig: any(&http.HttpConnectionManager{
								// TODO(dfc) should the stats listener expose stats? is that likely to collapse the multiverse?
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
			},
		},
	}
}

// Register registers ch to receive a value when Notify is called.
// The value of last is the count of the times Notify has been called on this Cache.
// It functions of a sequence counter, if the value of last supplied to Register
// is less than the Cache's internal counter, then the caller has missed at least
// one notification and will fire immediately.
//
// Sends by the broadcaster to ch must not block, therefor ch must have a capacity
// of at least 1.
func (c *ListenerCache) Register(ch chan int, last int) {
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
func (c *ListenerCache) Update(v map[string]*v2.Listener) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.values = v
	c.notify()
}

// notify notifies all registered waiters that an event has occurred.
func (c *ListenerCache) notify() {
	c.last++

	for _, ch := range c.waiters {
		ch <- c.last
	}
	c.waiters = c.waiters[:0]
}

// Values returns a slice of the value stored in the cache.
func (c *ListenerCache) Values(filter func(string) bool) []proto.Message {
	c.mu.Lock()
	values := make([]proto.Message, 0, len(c.values))
	for _, v := range c.values {
		if filter(v.Name) {
			values = append(values, v)
		}
	}
	for _, v := range c.staticValues {
		values = append(values, v)
	}

	c.mu.Unlock()
	return values
}

type listenerVisitor struct {
	*ListenerVisitorConfig

	listeners map[string]*v2.Listener
	http      bool // at least one dag.VirtualHost encountered
}

func visitListeners(root dag.Vertex, lvc *ListenerVisitorConfig) map[string]*v2.Listener {
	lv := listenerVisitor{
		ListenerVisitorConfig: lvc,
		listeners: map[string]*v2.Listener{
			ENVOY_HTTPS_LISTENER: envoy.Listener(
				ENVOY_HTTPS_LISTENER,
				lvc.httpsAddress(), lvc.httpsPort(),
				secureProxyProtocol(lvc.UseProxyProto),
			),
		},
	}
	lv.visit(root)

	// add a listener if there are vhosts bound to http.
	if lv.http {
		lv.listeners[ENVOY_HTTP_LISTENER] = envoy.Listener(
			ENVOY_HTTP_LISTENER,
			lvc.httpAddress(), lvc.httpPort(),
			proxyProtocol(lvc.UseProxyProto),
			envoy.HTTPConnectionManager(ENVOY_HTTP_LISTENER, lvc.httpAccessLog()),
		)

	}

	// remove the https listener if there are no vhosts bound to it.
	if len(lv.listeners[ENVOY_HTTPS_LISTENER].FilterChains) == 0 {
		delete(lv.listeners, ENVOY_HTTPS_LISTENER)
	}

	return lv.listeners
}

func proxyProtocol(useProxy bool) []listener.ListenerFilter {
	if useProxy {
		return []listener.ListenerFilter{
			envoy.ProxyProtocol(),
		}
	}
	return nil
}

func secureProxyProtocol(useProxy bool) []listener.ListenerFilter {
	return append(proxyProtocol(useProxy), envoy.TLSInspector())
}

func (v *listenerVisitor) visit(vertex dag.Vertex) {
	switch vh := vertex.(type) {
	case *dag.VirtualHost:
		// we only create on http listener so record the fact
		// that we need to then double back at the end and add
		// the listener properly.
		v.http = true
	case *dag.SecureVirtualHost:
		filters := []listener.Filter{
			envoy.HTTPConnectionManager(ENVOY_HTTPS_LISTENER, v.httpsAccessLog()),
		}
		alpnProtos := []string{"h2", "http/1.1"}
		if vh.VirtualHost.TCPProxy != nil {
			filters = []listener.Filter{
				envoy.TCPProxy(ENVOY_HTTPS_LISTENER, vh.VirtualHost.TCPProxy, v.httpsAccessLog()),
			}
			alpnProtos = nil // do not offer ALPN
		}

		fc := listener.FilterChain{
			FilterChainMatch: &listener.FilterChainMatch{
				ServerNames: []string{vh.VirtualHost.Name},
			},
			Filters: filters,
		}

		// attach certificate data to this listener if provided.
		if vh.Secret != nil {
			fc.TlsContext = envoy.DownstreamTLSContext(envoy.Secretname(vh.Secret), vh.MinProtoVersion, alpnProtos...)
		}

		v.listeners[ENVOY_HTTPS_LISTENER].FilterChains = append(v.listeners[ENVOY_HTTPS_LISTENER].FilterChains, fc)
	default:
		// recurse
		vertex.Visit(v.visit)
	}
}

func any(pb proto.Message) *types.Any {
	any, err := types.MarshalAny(pb)
	if err != nil {
		panic(err.Error())
	}
	return any
}
