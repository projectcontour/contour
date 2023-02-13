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
	"github.com/projectcontour/contour/internal/contour"
	"github.com/projectcontour/contour/internal/dag"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/protobuf"
	"github.com/projectcontour/contour/internal/sorter"
	"google.golang.org/protobuf/proto"
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

	sort.Stable(sorter.For(values))
	return protobuf.AsMessages(values)
}

// TypeURL returns the string type of RouteCache Resource.
func (*RouteCache) TypeURL() string { return resource.RouteType }

func (c *RouteCache) OnChange(root *dag.DAG) {
	// RouteConfigs keyed by RouteConfig name:
	// 	- one for all the HTTP vhost routes -- "ingress_http"
	//	- one per svhost -- "https/<vhost fqdn>"
	//	- one for fallback cert (if configured) -- "ingress_fallbackcert"
	routeConfigs := map[string]*envoy_route_v3.RouteConfiguration{}

	for vhost, routes := range root.GetVirtualHostRoutes() {
		if routeConfigs[ENVOY_HTTP_LISTENER] == nil {
			routeConfigs[ENVOY_HTTP_LISTENER] = envoy_v3.RouteConfiguration(ENVOY_HTTP_LISTENER)
		}

		sortRoutes(routes)
		routeConfigs[ENVOY_HTTP_LISTENER].VirtualHosts = append(routeConfigs[ENVOY_HTTP_LISTENER].VirtualHosts,
			envoy_v3.VirtualHostAndRoutes(vhost, routes, false, nil))
	}

	for vhost, routes := range root.GetSecureVirtualHostRoutes() {
		// Add secure vhost route config if not already present.
		name := path.Join("https", vhost.VirtualHost.Name)
		if _, ok := routeConfigs[name]; !ok {
			routeConfigs[name] = envoy_v3.RouteConfiguration(name)
		}

		sortRoutes(routes)
		routeConfigs[name].VirtualHosts = append(routeConfigs[name].VirtualHosts,
			envoy_v3.VirtualHostAndRoutes(&vhost.VirtualHost, routes, true, vhost.AuthorizationService))

		// A fallback route configuration contains routes for all the vhosts that have the fallback certificate enabled.
		// When a request is received, the default TLS filterchain will accept the connection,
		// and this routing table in RDS defines where the request proxies next.
		if vhost.FallbackCertificate != nil {
			// Add fallback route config if not already present.
			if _, ok := routeConfigs[ENVOY_FALLBACK_ROUTECONFIG]; !ok {
				routeConfigs[ENVOY_FALLBACK_ROUTECONFIG] = envoy_v3.RouteConfiguration(ENVOY_FALLBACK_ROUTECONFIG)
			}

			routeConfigs[ENVOY_FALLBACK_ROUTECONFIG].VirtualHosts = append(routeConfigs[ENVOY_FALLBACK_ROUTECONFIG].VirtualHosts,
				envoy_v3.VirtualHostAndRoutes(&vhost.VirtualHost, routes, true, vhost.AuthorizationService))
		}
	}

	for _, routeConfig := range routeConfigs {
		sort.Stable(sorter.For(routeConfig.VirtualHosts))
	}

	c.Update(routeConfigs)
}

// sortRoutes sorts the given Route slice in place. Routes are ordered
// first by path match type, path match value via string comparison and
// then by the header and query param match conditions.
// We sort dag.Route objects before converting to Envoy types to ensure
// more accurate ordering of route matches. Contour route match types may
// be implemented by Envoy route match types that change over time, or by
// types that do not exactly match to the type in Contour (e.g. using a
// regex matcher to implement a different type of match). Sorting based on
// Contour types instead ensures we can sort from most to least specific
// route match regardless of the underlying Envoy type that is used to
// implement the match.
func sortRoutes(routes []*dag.Route) {
	for _, r := range routes {
		sort.Stable(sorter.For(r.HeaderMatchConditions))
		sort.Stable(sorter.For(r.QueryParamMatchConditions))
	}

	sort.Stable(sorter.For(routes))
}
