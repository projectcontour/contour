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
	"testing"

	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/dag"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/fixture"
)

func TestRootNamespaces(t *testing.T) {
	rh, c, done := setup(t, func(b *dag.Builder) {
		b.Source.RootNamespaces = []string{"roots"}
	})
	defer done()

	// Not in root namespace set.
	svc1 := fixture.NewService("kuard").
		WithPorts(core_v1.ServicePort{Port: 8080, TargetPort: intstr.FromInt(8080)})
	rh.OnAdd(svc1)

	// Inside root namespace set.
	svc2 := fixture.NewService("roots/kuard").
		WithPorts(core_v1.ServicePort{Port: 8080, TargetPort: intstr.FromInt(8080)})
	rh.OnAdd(svc2)

	// assert that there is only a static listener
	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			statsListener(),
		),
		TypeUrl: listenerType,
	})

	// assert that the route tables are empty
	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: nil,
		TypeUrl:   routeType,
	})

	// hp1 is not in the root namespace set.
	hp1 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "simple",
			Namespace: svc1.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "hp1.example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: matchconditions(prefixMatchCondition("/")),
				Services: []contour_v1.Service{{
					Name: svc1.Name,
					Port: 8080,
				}},
			}},
		},
	}
	rh.OnAdd(hp1)

	// assert that hp1 has no effect on the listener set.
	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			statsListener(),
		),
		TypeUrl: listenerType,
	})

	// assert that the route tables are present but empty.
	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http"),
		),
		TypeUrl: routeType,
	})

	// hp2 is in the root namespace set.
	hp2 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "simple",
			Namespace: svc2.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "hp2.example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: matchconditions(prefixMatchCondition("/")),
				Services: []contour_v1.Service{{
					Name: svc2.Name,
					Port: 8080,
				}},
			}},
		},
	}
	rh.OnAdd(hp2)

	// assert that hp2 creates port 80 listener.
	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			defaultHTTPListener(),
			statsListener(),
		),
		TypeUrl: listenerType,
	})

	// assert that hp2.example.com's routes are visible.
	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("hp2.example.com",
					&envoy_config_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routeCluster("roots/kuard/8080/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
	})
}
