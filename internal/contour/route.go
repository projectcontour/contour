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

func (*RouteCache) TypeURL() string { return cache.RouteType }

type routeVisitor struct {
	routes map[string]*v2.RouteConfiguration
}

func visitRoutes(root dag.Vertex) map[string]*v2.RouteConfiguration {
	headers := envoy.Headers(
		envoy.AppendHeader("x-request-start", "t=%START_TIME(%s.%3f)%"),
	)

	rv := routeVisitor{
		routes: map[string]*v2.RouteConfiguration{
			"ingress_http": {
				Name:                "ingress_http",
				RequestHeadersToAdd: headers,
			},
			"ingress_https": {
				Name:                "ingress_https",
				RequestHeadersToAdd: headers,
			},
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

					match := envoy.RouteMatch(route)
					if route.HTTPSUpgrade {
						// TODO(dfc) if we ensure the builder never returns a dag.Route connected
						// to a SecureVirtualHost that requires upgrade, this logic can move to
						// envoy.RouteRoute.
						routes = append(routes, &envoy_api_v2_route.Route{
							Match:  match,
							Action: envoy.UpgradeHTTPS(),
						})
						return
					}
					routes = append(routes, &envoy_api_v2_route.Route{
						Match:  match,
						Action: envoy.RouteRoute(route),
					})
				})
				if len(routes) < 1 {
					return
				}
				sort.Stable(longestRouteFirst(routes))
				vhost := envoy.VirtualHost(vh.Name, routes...)
				v.routes["ingress_http"].VirtualHosts = append(v.routes["ingress_http"].VirtualHosts, vhost)
			case *dag.SecureVirtualHost:
				var routes []*envoy_api_v2_route.Route
				vh.Visit(func(v dag.Vertex) {
					route, ok := v.(*dag.Route)
					if !ok {
						return
					}

					routes = append(routes, &envoy_api_v2_route.Route{
						Match:  envoy.RouteMatch(route),
						Action: envoy.RouteRoute(route),
					})
				})
				if len(routes) < 1 {
					return
				}
				sort.Stable(longestRouteFirst(routes))
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

type virtualHostsByName []*envoy_api_v2_route.VirtualHost

func (v virtualHostsByName) Len() int           { return len(v) }
func (v virtualHostsByName) Swap(i, j int)      { v[i], v[j] = v[j], v[i] }
func (v virtualHostsByName) Less(i, j int) bool { return v[i].Name < v[j].Name }

type longestRouteFirst []*envoy_api_v2_route.Route

func (l longestRouteFirst) Len() int      { return len(l) }
func (l longestRouteFirst) Swap(i, j int) { l[i], l[j] = l[j], l[i] }
func (l longestRouteFirst) Less(i, j int) bool {
	switch a := l[i].Match.PathSpecifier.(type) {
	case *envoy_api_v2_route.RouteMatch_Prefix:
		switch b := l[j].Match.PathSpecifier.(type) {
		case *envoy_api_v2_route.RouteMatch_Prefix:
			if len(l[i].Match.Headers) == len(l[j].Match.Headers) {
				return a.Prefix > b.Prefix
			}
			return len(l[i].Match.Headers) > len(l[j].Match.Headers)
		}
	case *envoy_api_v2_route.RouteMatch_Regex:
		switch b := l[j].Match.PathSpecifier.(type) {
		case *envoy_api_v2_route.RouteMatch_Regex:
			return a.Regex > b.Regex
		case *envoy_api_v2_route.RouteMatch_Prefix:
			return true
		}
	}
	return false
}
