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
	"path"
	"sort"
	"sync"

	envoy_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	resource "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/projectcontour/contour/internal/contour"
	"github.com/projectcontour/contour/internal/dag"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/protobuf"
	"github.com/projectcontour/contour/internal/sorter"
)

// RouteCache manages the contents of the gRPC RDS cache.
type RouteCache struct {
	mu     sync.Mutex
	values map[string]*envoy_route_v3.RouteConfiguration
	contour.Cond
}

// Update replaces the contents of the cache with the supplied map.
func (c *RouteCache) Update(v map[string]*envoy_route_v3.RouteConfiguration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.values = v
	c.Cond.Notify()
}

// Contents returns a copy of the cache's contents.
func (c *RouteCache) Contents() []proto.Message {
	c.mu.Lock()
	defer c.mu.Unlock()

	var values []*envoy_route_v3.RouteConfiguration
	for _, v := range c.values {
		values = append(values, v)
	}

	sort.Stable(sorter.For(values))
	return protobuf.AsMessages(values)
}

// Query searches the RouteCache for the named RouteConfiguration entries.
func (c *RouteCache) Query(names []string) []proto.Message {
	c.mu.Lock()
	defer c.mu.Unlock()

	var values []*envoy_route_v3.RouteConfiguration
	for _, n := range names {
		v, ok := c.values[n]
		if !ok {
			// if there is no route registered with the cache
			// we return a blank route configuration. This is
			// not the same as returning nil, we're choosing to
			// say "the configuration you asked for _does exists_,
			// but it contains no useful information.
			v = &envoy_route_v3.RouteConfiguration{
				Name: n,
			}
		}
		values = append(values, v)
	}

	//sort.RouteConfigurations(values)
	sort.Stable(sorter.For(values))
	return protobuf.AsMessages(values)
}

// TypeURL returns the string type of RouteCache Resource.
func (*RouteCache) TypeURL() string { return resource.RouteType }

func (c *RouteCache) OnChange(root *dag.DAG) {
	routes := visitRoutes(root)
	c.Update(routes)
}

type routeVisitor struct {
	routes map[string]*envoy_route_v3.RouteConfiguration
}

func visitRoutes(root dag.Vertex) map[string]*envoy_route_v3.RouteConfiguration {
	// Collect the route configurations for all the routes we can
	// find. For HTTP hosts, the routes will all be collected on the
	// well-known ENVOY_HTTP_LISTENER, but for HTTPS hosts, we will
	// generate a per-vhost collection. This lets us keep different
	// SNI names disjoint when we later configure the listener.
	rv := routeVisitor{
		routes: map[string]*envoy_route_v3.RouteConfiguration{
			ENVOY_HTTP_LISTENER: envoy_v3.RouteConfiguration(ENVOY_HTTP_LISTENER),
		},
	}

	rv.visit(root)

	for _, v := range rv.routes {
		sort.Stable(sorter.For(v.VirtualHosts))
	}

	return rv.routes
}

func (v *routeVisitor) onVirtualHost(vh *dag.VirtualHost) {
	var routes []*envoy_route_v3.Route

	vh.Visit(func(v dag.Vertex) {
		route, ok := v.(*dag.Route)
		if !ok {
			return
		}

		if route.HTTPSUpgrade {
			// TODO(dfc) if we ensure the builder never returns a dag.Route connected
			// to a SecureVirtualHost that requires upgrade, this logic can move to
			// envoy.RouteRoute.
			routes = append(routes, &envoy_route_v3.Route{
				Match:  envoy_v3.RouteMatch(route),
				Action: envoy_v3.UpgradeHTTPS(),
			})
		} else {
			rt := &envoy_route_v3.Route{
				Match:  envoy_v3.RouteMatch(route),
				Action: envoy_v3.RouteRoute(route),
			}
			if route.RequestHeadersPolicy != nil {
				rt.RequestHeadersToAdd = envoy_v3.HeaderValueList(route.RequestHeadersPolicy.Set, false)
				rt.RequestHeadersToRemove = route.RequestHeadersPolicy.Remove
			}
			if route.ResponseHeadersPolicy != nil {
				rt.ResponseHeadersToAdd = envoy_v3.HeaderValueList(route.ResponseHeadersPolicy.Set, false)
				rt.ResponseHeadersToRemove = route.ResponseHeadersPolicy.Remove
			}
			if route.RateLimitPolicy != nil && route.RateLimitPolicy.Local != nil {
				if rt.TypedPerFilterConfig == nil {
					rt.TypedPerFilterConfig = map[string]*any.Any{}
				}
				rt.TypedPerFilterConfig["envoy.filters.http.local_ratelimit"] = envoy_v3.LocalRateLimitConfig(route.RateLimitPolicy.Local, "vhost."+vh.Name)
			}
			routes = append(routes, rt)
		}
	})

	if len(routes) > 0 {
		sortRoutes(routes)

		evh := envoy_v3.VirtualHost(vh.Name, routes...)
		if vh.CORSPolicy != nil {
			evh.Cors = envoy_v3.CORSPolicy(vh.CORSPolicy)
		}
		if vh.RateLimitPolicy != nil && vh.RateLimitPolicy.Local != nil {
			if evh.TypedPerFilterConfig == nil {
				evh.TypedPerFilterConfig = map[string]*any.Any{}
			}
			evh.TypedPerFilterConfig["envoy.filters.http.local_ratelimit"] = envoy_v3.LocalRateLimitConfig(vh.RateLimitPolicy.Local, "vhost."+vh.Name)
		}

		v.routes[ENVOY_HTTP_LISTENER].VirtualHosts = append(v.routes[ENVOY_HTTP_LISTENER].VirtualHosts, evh)
	}
}

func (v *routeVisitor) onSecureVirtualHost(svh *dag.SecureVirtualHost) {
	var routes []*envoy_route_v3.Route

	svh.Visit(func(v dag.Vertex) {
		route, ok := v.(*dag.Route)
		if !ok {
			return
		}

		rt := &envoy_route_v3.Route{
			Match:  envoy_v3.RouteMatch(route),
			Action: envoy_v3.RouteRoute(route),
		}
		if route.RequestHeadersPolicy != nil {
			rt.RequestHeadersToAdd = envoy_v3.HeaderValueList(route.RequestHeadersPolicy.Set, false)
			rt.RequestHeadersToRemove = route.RequestHeadersPolicy.Remove
		}
		if route.ResponseHeadersPolicy != nil {
			rt.ResponseHeadersToAdd = envoy_v3.HeaderValueList(route.ResponseHeadersPolicy.Set, false)
			rt.ResponseHeadersToRemove = route.ResponseHeadersPolicy.Remove
		}
		if route.RateLimitPolicy != nil && route.RateLimitPolicy.Local != nil {
			if rt.TypedPerFilterConfig == nil {
				rt.TypedPerFilterConfig = map[string]*any.Any{}
			}
			rt.TypedPerFilterConfig["envoy.filters.http.local_ratelimit"] = envoy_v3.LocalRateLimitConfig(route.RateLimitPolicy.Local, "vhost."+svh.Name)
		}

		// If authorization is enabled on this host, we may need to set per-route filter overrides.
		if svh.AuthorizationService != nil {
			// Apply per-route authorization policy modifications.
			if route.AuthDisabled {
				if rt.TypedPerFilterConfig == nil {
					rt.TypedPerFilterConfig = map[string]*any.Any{}
				}
				rt.TypedPerFilterConfig["envoy.filters.http.ext_authz"] = envoy_v3.RouteAuthzDisabled()
			} else {
				if len(route.AuthContext) > 0 {
					if rt.TypedPerFilterConfig == nil {
						rt.TypedPerFilterConfig = map[string]*any.Any{}
					}
					rt.TypedPerFilterConfig["envoy.filters.http.ext_authz"] = envoy_v3.RouteAuthzContext(route.AuthContext)
				}
			}
		}

		routes = append(routes, rt)
	})

	if len(routes) > 0 {
		sortRoutes(routes)

		name := path.Join("https", svh.VirtualHost.Name)

		if _, ok := v.routes[name]; !ok {
			v.routes[name] = envoy_v3.RouteConfiguration(name)
		}

		evh := envoy_v3.VirtualHost(svh.VirtualHost.Name, routes...)
		if svh.CORSPolicy != nil {
			evh.Cors = envoy_v3.CORSPolicy(svh.CORSPolicy)
		}
		if svh.RateLimitPolicy != nil && svh.RateLimitPolicy.Local != nil {
			if evh.TypedPerFilterConfig == nil {
				evh.TypedPerFilterConfig = map[string]*any.Any{}
			}
			evh.TypedPerFilterConfig["envoy.filters.http.local_ratelimit"] = envoy_v3.LocalRateLimitConfig(svh.RateLimitPolicy.Local, "vhost."+svh.Name)
		}

		v.routes[name].VirtualHosts = append(v.routes[name].VirtualHosts, evh)

		// A fallback route configuration contains routes for all the vhosts that have the fallback certificate enabled.
		// When a request is received, the default TLS filterchain will accept the connection,
		// and this routing table in RDS defines where the request proxies next.
		if svh.FallbackCertificate != nil {
			// Add fallback route if not already
			if _, ok := v.routes[ENVOY_FALLBACK_ROUTECONFIG]; !ok {
				v.routes[ENVOY_FALLBACK_ROUTECONFIG] = envoy_v3.RouteConfiguration(ENVOY_FALLBACK_ROUTECONFIG)
			}

			fvh := envoy_v3.VirtualHost(svh.Name, routes...)
			if svh.CORSPolicy != nil {
				fvh.Cors = envoy_v3.CORSPolicy(svh.CORSPolicy)
			}
			if svh.RateLimitPolicy != nil && svh.RateLimitPolicy.Local != nil {
				if fvh.TypedPerFilterConfig == nil {
					fvh.TypedPerFilterConfig = map[string]*any.Any{}
				}
				fvh.TypedPerFilterConfig["envoy.filters.http.local_ratelimit"] = envoy_v3.LocalRateLimitConfig(svh.RateLimitPolicy.Local, "vhost."+svh.Name)
			}

			v.routes[ENVOY_FALLBACK_ROUTECONFIG].VirtualHosts = append(v.routes[ENVOY_FALLBACK_ROUTECONFIG].VirtualHosts, fvh)
		}
	}
}

func (v *routeVisitor) visit(vertex dag.Vertex) {
	switch l := vertex.(type) {
	case *dag.Listener:
		l.Visit(func(vertex dag.Vertex) {
			switch vh := vertex.(type) {
			case *dag.VirtualHost:
				v.onVirtualHost(vh)
			case *dag.SecureVirtualHost:
				v.onSecureVirtualHost(vh)
			default:
				// recurse
				vertex.Visit(v.visit)
			}
		})
	default:
		// recurse
		vertex.Visit(v.visit)
	}
}

// sortRoutes sorts the given Route slice in place. Routes are ordered
// first by longest prefix (or regex), then by the length of the
// HeaderMatch slice (if any). The HeaderMatch slice is also ordered
// by the matching header name.
func sortRoutes(routes []*envoy_route_v3.Route) {
	for _, r := range routes {
		sort.Stable(sorter.For(r.Match.Headers))
	}

	sort.Stable(sorter.For(routes))
}
