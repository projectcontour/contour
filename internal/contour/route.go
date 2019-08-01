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
	"sort"
	"sync"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/envoyproxy/go-control-plane/pkg/cache"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"github.com/heptio/contour/internal/dag"
	"github.com/heptio/contour/internal/envoy"
)

// RouteCache manages the contents of the gRPC RDS cache.
type RouteCache struct {
	mu      sync.Mutex
	values  map[string]*v2.RouteConfiguration
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
func (c *RouteCache) Register(ch chan int, last int) {
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
func (c *RouteCache) Update(v map[string]*v2.RouteConfiguration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.values = v
	c.notify()
}

// notify notifies all registered waiters that an event has occurred.
func (c *RouteCache) notify() {
	c.last++

	for _, ch := range c.waiters {
		ch <- c.last
	}
	c.waiters = c.waiters[:0]
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
	rv := routeVisitor{
		routes: map[string]*v2.RouteConfiguration{
			"ingress_http": {
				Name: "ingress_http",
			},
			"ingress_https": {
				Name: "ingress_https",
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
				vhost := envoy.VirtualHost(vh.Name)
				vh.Visit(func(v dag.Vertex) {
					switch r := v.(type) {
					case *dag.PrefixRoute:
						if len(r.Clusters) < 1 {
							// no services for this route, skip it.
							return
						}
						rr := route.Route{
							Match:               envoy.RoutePrefix(r.Prefix),
							Action:              envoy.RouteRoute(&r.Route),
							RequestHeadersToAdd: envoy.RouteHeaders(),
						}

						if r.HTTPSUpgrade {
							rr.Action = envoy.UpgradeHTTPS()
							rr.RequestHeadersToAdd = nil
						}
						vhost.Routes = append(vhost.Routes, rr)
					case *dag.RegexRoute:
						if len(r.Clusters) < 1 {
							// no services for this route, skip it.
							return
						}
						rr := route.Route{
							Match:               envoy.RouteRegex(r.Regex),
							Action:              envoy.RouteRoute(&r.Route),
							RequestHeadersToAdd: envoy.RouteHeaders(),
						}

						if r.HTTPSUpgrade {
							rr.Action = envoy.UpgradeHTTPS()
							rr.RequestHeadersToAdd = nil
						}
						vhost.Routes = append(vhost.Routes, rr)
					}
				})
				if len(vhost.Routes) < 1 {
					return
				}
				sort.Stable(longestRouteFirst(vhost.Routes))
				v.routes["ingress_http"].VirtualHosts = append(v.routes["ingress_http"].VirtualHosts, vhost)
			case *dag.SecureVirtualHost:
				vhost := envoy.VirtualHost(vh.VirtualHost.Name)
				vh.Visit(func(v dag.Vertex) {
					switch r := v.(type) {
					case *dag.PrefixRoute:
						if len(r.Clusters) < 1 {
							// no services for this route, skip it.
							return
						}
						vhost.Routes = append(vhost.Routes, route.Route{
							Match:               envoy.RoutePrefix(r.Prefix),
							Action:              envoy.RouteRoute(&r.Route),
							RequestHeadersToAdd: envoy.RouteHeaders(),
						})
					case *dag.RegexRoute:
						if len(r.Clusters) < 1 {
							// no services for this route, skip it.
							return
						}
						vhost.Routes = append(vhost.Routes, route.Route{
							Match:               envoy.RouteRegex(r.Regex),
							Action:              envoy.RouteRoute(&r.Route),
							RequestHeadersToAdd: envoy.RouteHeaders(),
						})

					}
				})
				if len(vhost.Routes) < 1 {
					return
				}
				sort.Stable(longestRouteFirst(vhost.Routes))
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

type virtualHostsByName []route.VirtualHost

func (v virtualHostsByName) Len() int           { return len(v) }
func (v virtualHostsByName) Swap(i, j int)      { v[i], v[j] = v[j], v[i] }
func (v virtualHostsByName) Less(i, j int) bool { return v[i].Name < v[j].Name }

type longestRouteFirst []route.Route

func (l longestRouteFirst) Len() int      { return len(l) }
func (l longestRouteFirst) Swap(i, j int) { l[i], l[j] = l[j], l[i] }
func (l longestRouteFirst) Less(i, j int) bool {
	switch a := l[i].Match.PathSpecifier.(type) {
	case *route.RouteMatch_Prefix:
		switch b := l[j].Match.PathSpecifier.(type) {
		case *route.RouteMatch_Prefix:
			return a.Prefix > b.Prefix
		}
	case *route.RouteMatch_Regex:
		switch b := l[j].Match.PathSpecifier.(type) {
		case *route.RouteMatch_Regex:
			return a.Regex > b.Regex
		case *route.RouteMatch_Prefix:
			return true
		}
	}
	return false
}

func u32(val int) *types.UInt32Value { return &types.UInt32Value{Value: uint32(val)} }
