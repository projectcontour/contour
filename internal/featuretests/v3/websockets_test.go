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
	networking_v1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/featuretests"
	"github.com/projectcontour/contour/internal/fixture"
)

func TestWebsocketsIngress(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	s1 := fixture.NewService("ws").
		WithPorts(core_v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)})
	rh.OnAdd(s1)

	i1 := &networking_v1.Ingress{
		ObjectMeta: fixture.ObjectMetaWithAnnotations("ws", map[string]string{
			"projectcontour.io/websocket-routes": "/ws2",
		}),
		Spec: networking_v1.IngressSpec{
			Rules: []networking_v1.IngressRule{{
				Host: "websocket.hello.world",
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Path:    "/ws2",
							Backend: *featuretests.IngressBackend(s1),
						}},
					},
				},
			}},
		},
	}
	rh.OnAdd(i1)

	// check websocket annotation
	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("websocket.hello.world",
					&envoy_config_route_v3.Route{
						Match:  routePrefix("/ws2"),
						Action: withWebsocket(routeCluster("default/ws/80/da39a3ee5e")),
					},
				),
			),
		),
		TypeUrl: routeType,
	})
}

func TestWebsocketHTTPProxy(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	s1 := fixture.NewService("ws").
		WithPorts(core_v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)})
	rh.OnAdd(s1)

	s2 := fixture.NewService("ws2").
		WithPorts(core_v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)})
	rh.OnAdd(s2)

	hp1 := &contour_v1.HTTPProxy{
		ObjectMeta: fixture.ObjectMeta("simple"),
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{Fqdn: "websocket.hello.world"},
			Routes: []contour_v1.Route{{
				Conditions: matchconditions(prefixMatchCondition("/")),
				Services: []contour_v1.Service{{
					Name: s1.Name,
					Port: 80,
				}},
			}, {
				Conditions:       matchconditions(prefixMatchCondition("/ws-1")),
				EnableWebsockets: true,
				Services: []contour_v1.Service{{
					Name: s1.Name,
					Port: 80,
				}},
			}, {
				Conditions:       matchconditions(prefixMatchCondition("/ws-2")),
				EnableWebsockets: true,
				Services: []contour_v1.Service{{
					Name: s1.Name,
					Port: 80,
				}},
			}},
		},
	}
	rh.OnAdd(hp1)

	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("websocket.hello.world",
					&envoy_config_route_v3.Route{
						Match:  routePrefix("/ws-2"),
						Action: withWebsocket(routeCluster("default/ws/80/da39a3ee5e")),
					},
					&envoy_config_route_v3.Route{
						Match:  routePrefix("/ws-1"),
						Action: withWebsocket(routeCluster("default/ws/80/da39a3ee5e")),
					},
					&envoy_config_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routeCluster("default/ws/80/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	hp2 := &contour_v1.HTTPProxy{
		ObjectMeta: fixture.ObjectMeta("simple"),
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{Fqdn: "websocket.hello.world"},
			Routes: []contour_v1.Route{{
				Conditions: matchconditions(prefixMatchCondition("/")),
				Services: []contour_v1.Service{{
					Name: s1.Name,
					Port: 80,
				}},
			}, {
				Conditions:       matchconditions(prefixMatchCondition("/ws-1")),
				EnableWebsockets: true,
				Services: []contour_v1.Service{{
					Name: s1.Name,
					Port: 80,
				}, {
					Name: s2.Name,
					Port: 80,
				}},
			}, {
				Conditions:       matchconditions(prefixMatchCondition("/ws-2")),
				EnableWebsockets: true,
				Services: []contour_v1.Service{{
					Name: s1.Name,
					Port: 80,
				}},
			}},
		},
	}
	rh.OnUpdate(hp1, hp2)

	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("websocket.hello.world",
					&envoy_config_route_v3.Route{
						Match:  routePrefix("/ws-2"),
						Action: withWebsocket(routeCluster("default/ws/80/da39a3ee5e")),
					},
					&envoy_config_route_v3.Route{
						Match: routePrefix("/ws-1"),
						Action: withWebsocket(routeWeightedCluster(
							weightedCluster{"default/ws/80/da39a3ee5e", 1},
							weightedCluster{"default/ws2/80/da39a3ee5e", 1},
						)),
					},
					&envoy_config_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routeCluster("default/ws/80/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
	})
}
