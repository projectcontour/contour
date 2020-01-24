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

package contour

import (
	"sort"
	"strings"
	"sync"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_api_v2_route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/envoyproxy/go-control-plane/pkg/cache"
	"github.com/golang/protobuf/proto"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/envoy"
)

// RouteCache manages the contents of the gRPC RDS cache.
type RouteCache struct {
	mu     sync.Mutex
	values map[string]*v2.RouteConfiguration
	Cond
}

// Update replaces the contents of the cache with the supplied map.
func (c *RouteCache) Update(v map[string]*v2.RouteConfiguration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.values = v
	c.Cond.Notify()
}

// Contents returns a copy of the cache's contents.
func (c *RouteCache) Contents() []proto.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	var values []proto.Message
	for _, v := range c.values {
		values = append(values, v)
	}
	sort.Stable(routeConfigurationsByName(values))
	return values
}

// Query searches the RouteCache for the named RouteConfiguration entries.
func (c *RouteCache) Query(names []string) []proto.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	var values []proto.Message
	for _, n := range names {
		v, ok := c.values[n]
		if !ok {
			// if there is no route registered with the cache
			// we return a blank route configuration. This is
			// not the same as returning nil, we're choosing to
			// say "the configuration you asked for _does exists_,
			// but it contains no useful information.
			v = &v2.RouteConfiguration{
				Name: n,
			}
		}
		values = append(values, v)
	}
	sort.Stable(routeConfigurationsByName(values))
	return values
}

type routeConfigurationsByName []proto.Message

func (r routeConfigurationsByName) Len() int      { return len(r) }
func (r routeConfigurationsByName) Swap(i, j int) { r[i], r[j] = r[j], r[i] }
func (r routeConfigurationsByName) Less(i, j int) bool {
	return r[i].(*v2.RouteConfiguration).Name < r[j].(*v2.RouteConfiguration).Name
}

// TypeURL returns the string type of RouteCache Resource.
func (*RouteCache) TypeURL() string { return cache.RouteType }

type routeVisitor struct {
	routes map[string]*v2.RouteConfiguration
}

func visitRoutes(root dag.Vertex) map[string]*v2.RouteConfiguration {
	rv := routeVisitor{
		routes: map[string]*v2.RouteConfiguration{
			"ingress_http":  envoy.RouteConfiguration("ingress_http"),
			"ingress_https": envoy.RouteConfiguration("ingress_https"),
		},
	}
	rv.visit(root)
	for _, v := range rv.routes {
		sort.Stable(virtualHostsByName(v.VirtualHosts))
	}
	return rv.routes
}

func (v *routeVisitor) visit(vertex dag.Vertex) {
	switch l := vertex.(type) {
	case *dag.Listener:
		l.Visit(func(vertex dag.Vertex) {
			switch vh := vertex.(type) {
			case *dag.VirtualHost:
				var routes []*envoy_api_v2_route.Route

				vh.Visit(func(v dag.Vertex) {
					route, ok := v.(*dag.Route)
					if !ok {
						return
					}

					if route.HTTPSUpgrade {
						// TODO(dfc) if we ensure the builder never returns a dag.Route connected
						// to a SecureVirtualHost that requires upgrade, this logic can move to
						// envoy.RouteRoute.
						routes = append(routes, &envoy_api_v2_route.Route{
							Match:  envoy.RouteMatch(route),
							Action: envoy.UpgradeHTTPS(),
						})
					} else {
						rt := &envoy_api_v2_route.Route{
							Match:  envoy.RouteMatch(route),
							Action: envoy.RouteRoute(route),
						}
						if route.RequestHeadersPolicy != nil {
							rt.RequestHeadersToAdd = envoy.HeaderValueList(route.RequestHeadersPolicy.Set, false)
							rt.RequestHeadersToRemove = route.RequestHeadersPolicy.Remove
						}
						if route.ResponseHeadersPolicy != nil {
							rt.ResponseHeadersToAdd = envoy.HeaderValueList(route.ResponseHeadersPolicy.Set, false)
							rt.ResponseHeadersToRemove = route.ResponseHeadersPolicy.Remove
						}
						routes = append(routes, rt)
					}
				})

				if len(routes) < 1 {
					return
				}

				sortRoutes(routes)
				vhost := envoy.VirtualHost(vh.Name, routes...)
				v.routes["ingress_http"].VirtualHosts = append(v.routes["ingress_http"].VirtualHosts, vhost)
			case *dag.SecureVirtualHost:
				var routes []*envoy_api_v2_route.Route
				vh.Visit(func(v dag.Vertex) {
					route, ok := v.(*dag.Route)
					if !ok {
						return
					}

					rt := &envoy_api_v2_route.Route{
						Match:  envoy.RouteMatch(route),
						Action: envoy.RouteRoute(route),
					}
					if route.RequestHeadersPolicy != nil {
						rt.RequestHeadersToAdd = envoy.HeaderValueList(route.RequestHeadersPolicy.Set, false)
						rt.RequestHeadersToRemove = route.RequestHeadersPolicy.Remove
					}
					if route.ResponseHeadersPolicy != nil {
						rt.ResponseHeadersToAdd = envoy.HeaderValueList(route.ResponseHeadersPolicy.Set, false)
						rt.ResponseHeadersToRemove = route.ResponseHeadersPolicy.Remove
					}
					routes = append(routes, rt)
				})
				if len(routes) < 1 {
					return
				}
				sortRoutes(routes)
				vhost := envoy.VirtualHost(vh.VirtualHost.Name, routes...)
				v.routes["ingress_https"].VirtualHosts = append(v.routes["ingress_https"].VirtualHosts, vhost)
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

type headerMatcherByName []*envoy_api_v2_route.HeaderMatcher

func (h headerMatcherByName) Len() int      { return len(h) }
func (h headerMatcherByName) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

// Less compares HeaderMatcher objects, first by the header name,
// then by their matcher conditions (textually).
func (h headerMatcherByName) Less(i, j int) bool {
	val := strings.Compare(h[i].Name, h[j].Name)
	switch val {
	case -1:
		return true
	case 1:
		return false
	case 0:
		return proto.CompactTextString(h[i]) < proto.CompactTextString(h[j])
	}

	panic("bad compare")
}

type virtualHostsByName []*envoy_api_v2_route.VirtualHost

func (v virtualHostsByName) Len() int           { return len(v) }
func (v virtualHostsByName) Swap(i, j int)      { v[i], v[j] = v[j], v[i] }
func (v virtualHostsByName) Less(i, j int) bool { return v[i].Name < v[j].Name }

// sortRoutes sorts the given Route slice in place. Routes are ordered
// first by longest prefix (or regex), then by the length of the
// HeaderMatch slice (if any). The HeaderMatch slice is also ordered
// by the matching header name.
func sortRoutes(routes []*envoy_api_v2_route.Route) {
	for _, r := range routes {
		sort.Stable(headerMatcherByName(r.Match.Headers))
	}

	sort.Stable(longestRouteFirst(routes))
}

// longestRouteByHeaders compares the HeaderMatcher slices for lhs and rhs and
// returns true if lhs is longer.
func longestRouteByHeaders(lhs, rhs *envoy_api_v2_route.Route) bool {
	if len(lhs.Match.Headers) == len(rhs.Match.Headers) {
		pair := make([]*envoy_api_v2_route.HeaderMatcher, 2)

		for i := 0; i < len(lhs.Match.Headers); i++ {
			pair[0] = lhs.Match.Headers[i]
			pair[1] = rhs.Match.Headers[i]

			if headerMatcherByName(pair).Less(0, 1) {
				return true
			}
		}
	}

	return len(lhs.Match.Headers) > len(rhs.Match.Headers)
}

type longestRouteFirst []*envoy_api_v2_route.Route

func (l longestRouteFirst) Len() int      { return len(l) }
func (l longestRouteFirst) Swap(i, j int) { l[i], l[j] = l[j], l[i] }
func (l longestRouteFirst) Less(i, j int) bool {
	switch a := l[i].Match.PathSpecifier.(type) {
	case *envoy_api_v2_route.RouteMatch_Prefix:
		switch b := l[j].Match.PathSpecifier.(type) {
		case *envoy_api_v2_route.RouteMatch_Prefix:
			cmp := strings.Compare(a.Prefix, b.Prefix)
			switch cmp {
			case 1:
				// Sort longest prefix first.
				return true
			case -1:
				return false
			default:
				return longestRouteByHeaders(l[i], l[j])
			}
		}
	case *envoy_api_v2_route.RouteMatch_SafeRegex:
		switch b := l[j].Match.PathSpecifier.(type) {
		case *envoy_api_v2_route.RouteMatch_SafeRegex:
			cmp := strings.Compare(a.SafeRegex.Regex, b.SafeRegex.Regex)
			switch cmp {
			case 1:
				// Sort longest regex first.
				return true
			case -1:
				return false
			default:
				return longestRouteByHeaders(l[i], l[j])
			}
		case *envoy_api_v2_route.RouteMatch_Prefix:
			return true
		}
	}

	return false
}
