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
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"github.com/heptio/contour/internal/dag"
)

// RouteCache manages the contents of the gRPC RDS cache.
type RouteCache struct {
	routeCache
}

type routeCache struct {
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
func (c *routeCache) Register(ch chan int, last int) {
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
func (c *routeCache) Update(v map[string]*v2.RouteConfiguration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.values = v
	c.notify()
}

// notify notifies all registered waiters that an event has occurred.
func (c *routeCache) notify() {
	c.last++

	for _, ch := range c.waiters {
		ch <- c.last
	}
	c.waiters = c.waiters[:0]
}

// Values returns a slice of the value stored in the cache.
func (c *routeCache) Values(filter func(string) bool) []proto.Message {
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

type routeVisitor struct {
	*RouteCache
	dag.Visitable
}

func (v *routeVisitor) Visit() map[string]*v2.RouteConfiguration {
	ingress_http := &v2.RouteConfiguration{
		Name: "ingress_http",
	}
	ingress_https := &v2.RouteConfiguration{
		Name: "ingress_https",
	}
	m := map[string]*v2.RouteConfiguration{
		ingress_http.Name:  ingress_http,
		ingress_https.Name: ingress_https,
	}
	v.Visitable.Visit(func(vh dag.Vertex) {
		switch vh := vh.(type) {
		case *dag.VirtualHost:
			hostname := vh.Host
			domains := []string{hostname}
			if hostname != "*" {
				domains = append(domains, hostname+":80")
			}
			vhost := route.VirtualHost{
				Name:    hashname(60, hostname),
				Domains: domains,
			}
			vh.Visit(func(r dag.Vertex) {
				switch r := r.(type) {
				case *dag.Route:
					var svcs []*dag.Service
					r.Visit(func(s dag.Vertex) {
						if s, ok := s.(*dag.Service); ok {
							svcs = append(svcs, s)
						}
					})
					if len(svcs) < 1 {
						// no services for this route, skip it.
						return
					}
					rr := route.Route{
						Match:  prefixmatch(r.Prefix),
						Action: actionroute(r, svcs),
					}

					if r.HTTPSUpgrade {
						rr.Action = &route.Route_Redirect{
							Redirect: &route.RedirectAction{
								HttpsRedirect: true,
							},
						}
					}
					vhost.Routes = append(vhost.Routes, rr)
				}
			})
			if len(vhost.Routes) < 1 {
				return
			}
			sort.Stable(sort.Reverse(longestRouteFirst(vhost.Routes)))
			ingress_http.VirtualHosts = append(ingress_http.VirtualHosts, vhost)
		case *dag.SecureVirtualHost:
			hostname := vh.Host
			domains := []string{hostname}
			if hostname != "*" {
				domains = append(domains, hostname+":443")
			}
			vhost := route.VirtualHost{
				Name:    hashname(60, hostname),
				Domains: domains,
			}
			vh.Visit(func(r dag.Vertex) {
				switch r := r.(type) {
				case *dag.Route:
					var svcs []*dag.Service
					r.Visit(func(s dag.Vertex) {
						if s, ok := s.(*dag.Service); ok {
							svcs = append(svcs, s)
						}
					})
					if len(svcs) < 1 {
						// no services for this route, skip it.
						return
					}
					vhost.Routes = append(vhost.Routes, route.Route{
						Match:  prefixmatch(r.Prefix),
						Action: actionroute(r, svcs),
					})
				}
			})
			if len(vhost.Routes) < 1 {
				return
			}
			sort.Stable(sort.Reverse(longestRouteFirst(vhost.Routes)))
			ingress_https.VirtualHosts = append(ingress_https.VirtualHosts, vhost)
		}
	})

	for _, v := range m {
		sort.Stable(virtualHostsByName(v.VirtualHosts))
	}
	return m
}

type virtualHostsByName []route.VirtualHost

func (v virtualHostsByName) Len() int           { return len(v) }
func (v virtualHostsByName) Swap(i, j int)      { v[i], v[j] = v[j], v[i] }
func (v virtualHostsByName) Less(i, j int) bool { return v[i].Name < v[j].Name }

type longestRouteFirst []route.Route

func (l longestRouteFirst) Len() int      { return len(l) }
func (l longestRouteFirst) Swap(i, j int) { l[i], l[j] = l[j], l[i] }
func (l longestRouteFirst) Less(i, j int) bool {
	a, ok := l[i].Match.PathSpecifier.(*route.RouteMatch_Prefix)
	if !ok {
		// ignore non prefix matches
		return false
	}

	b, ok := l[j].Match.PathSpecifier.(*route.RouteMatch_Prefix)
	if !ok {
		// ignore non prefix matches
		return false
	}

	return a.Prefix < b.Prefix
}

// prefixmatch returns a RouteMatch for the supplied prefix.
func prefixmatch(prefix string) route.RouteMatch {
	return route.RouteMatch{
		PathSpecifier: &route.RouteMatch_Prefix{
			Prefix: prefix,
		},
	}
}

// action computes the cluster route action, a *route.Route_route for the
// supplied ingress and backend.
func actionroute(r *dag.Route, services []*dag.Service) *route.Route_Route {
	rr := route.Route_Route{
		Route: &route.RouteAction{
			ClusterSpecifier: &route.RouteAction_WeightedClusters{
				WeightedClusters: weightedclusters(services),
			},
		},
	}

	// Check if no weights were defined, if not default to even distribution
	clusters := rr.Route.ClusterSpecifier.(*route.RouteAction_WeightedClusters).WeightedClusters
	if clusters.TotalWeight.Value == 0 {
		for _, c := range clusters.Clusters {
			c.Weight.Value = 1
		}
		clusters.TotalWeight.Value = uint32(len(clusters.Clusters))
	}

	if r.Websocket {
		rr.Route.UseWebsocket = &types.BoolValue{Value: true}
	}

	if r.RetryOn != "" {
		rr.Route.RetryPolicy = &route.RouteAction_RetryPolicy{
			RetryOn: r.RetryOn,
		}
		if r.NumRetries > 0 {
			rr.Route.RetryPolicy.NumRetries = &types.UInt32Value{Value: uint32(r.NumRetries)}
		}
		if r.PerTryTimeout > 0 {
			timeout := r.PerTryTimeout
			rr.Route.RetryPolicy.PerTryTimeout = &timeout
		}
	}

	switch timeout := r.Timeout; timeout {
	case 0:
		// no timeout specified, do nothing
	case -1:
		// infinite timeout, set timeout value to a pointer to zero which tells
		// envoy "infinite timeout"
		infinity := time.Duration(0)
		rr.Route.Timeout = &infinity
	default:
		rr.Route.Timeout = &timeout
	}

	return &rr
}

func weightedclusters(services []*dag.Service) *route.WeightedCluster {
	var wc route.WeightedCluster
	var total int
	for _, svc := range services {
		total += svc.Weight
		wc.Clusters = append(wc.Clusters, &route.WeightedCluster_ClusterWeight{
			Name:   clustername(svc),
			Weight: &types.UInt32Value{Value: uint32(svc.Weight)},
		})
	}
	wc.TotalWeight = &types.UInt32Value{
		Value: uint32(total),
	}
	sort.Stable(clusterWeightByName(wc.Clusters))
	return &wc
}

type clusterWeightByName []*route.WeightedCluster_ClusterWeight

func (c clusterWeightByName) Len() int      { return len(c) }
func (c clusterWeightByName) Swap(i, j int) { c[i], c[j] = c[j], c[i] }
func (c clusterWeightByName) Less(i, j int) bool {
	if c[i].Name == c[j].Name {
		return c[i].Weight.Value < c[j].Weight.Value
	}
	return c[i].Name < c[j].Name

}

// hashname takes a lenth l and a varargs of strings s and returns a string whose length
// which does not exceed l. Internally s is joined with strings.Join(s, "/"). If the
// combined length exceeds l then hashname truncates each element in s, starting from the
// end using a hash derived from the contents of s (not the current element). This process
// continues until the length of s does not exceed l, or all elements have been truncated.
// In which case, the entire string is replaced with a hash not exceeding the length of l.
func hashname(l int, s ...string) string {
	const shorthash = 6 // the length of the shorthash

	r := strings.Join(s, "/")
	if l > len(r) {
		// we're under the limit, nothing to do
		return r
	}
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(r)))
	for n := len(s) - 1; n >= 0; n-- {
		s[n] = truncate(l/len(s), s[n], hash[:shorthash])
		r = strings.Join(s, "/")
		if l > len(r) {
			return r
		}
	}
	// truncated everything, but we're still too long
	// just return the hash truncated to l.
	return hash[:min(len(hash), l)]
}

// truncate truncates s to l length by replacing the
// end of s with -suffix.
func truncate(l int, s, suffix string) string {
	if l >= len(s) {
		// under the limit, nothing to do
		return s
	}
	if l <= len(suffix) {
		// easy case, just return the start of the suffix
		return suffix[:min(l, len(suffix))]
	}
	return s[:l-len(suffix)-1] + "-" + suffix
}

func min(a, b int) int {
	if a > b {
		return b
	}
	return a
}
