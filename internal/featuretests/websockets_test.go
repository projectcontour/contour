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

package featuretests

import (
	"testing"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_api_v2_route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	projcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/envoy"
	v1 "k8s.io/api/core/v1"
	"k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestWebsocketsIngress(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	s1 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ws",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}
	rh.OnAdd(s1)

	i1 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ws",
			Namespace: s1.Namespace,
			Annotations: map[string]string{
				"contour.heptio.com/websocket-routes": "/",
			},
		},
		Spec: v1beta1.IngressSpec{
			Rules: []v1beta1.IngressRule{{
				Host: "websocket.hello.world",
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Path: "/",
							Backend: v1beta1.IngressBackend{
								ServiceName: s1.Name,
								ServicePort: intstr.FromInt(80),
							},
						}},
					},
				},
			}},
		},
	}
	rh.OnAdd(i1)

	// check legacy websocket annotation
	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("websocket.hello.world",
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/"),
						Action: withWebsocket(routeCluster("default/ws/80/da39a3ee5e")),
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	i2 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ws",
			Namespace: s1.Namespace,
			Annotations: map[string]string{
				"projectcontour.io/websocket-routes": "/ws2",
			},
		},
		Spec: v1beta1.IngressSpec{
			Rules: []v1beta1.IngressRule{{
				Host: "websocket.hello.world",
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Path: "/ws2",
							Backend: v1beta1.IngressBackend{
								ServiceName: s1.Name,
								ServicePort: intstr.FromInt(80),
							},
						}},
					},
				},
			}},
		},
	}
	rh.OnUpdate(i1, i2)

	// check websocket annotation
	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("websocket.hello.world",
					&envoy_api_v2_route.Route{
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

	s1 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ws",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}
	rh.OnAdd(s1)

	s2 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ws2",
			Namespace: s1.Namespace,
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}
	rh.OnAdd(s2)

	hp1 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: s1.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{Fqdn: "websocket.hello.world"},
			Routes: []projcontour.Route{{
				Conditions: matchconditions(prefixMatchCondition("/")),
				Services: []projcontour.Service{{
					Name: s1.Name,
					Port: 80,
				}},
			}, {
				Conditions:       matchconditions(prefixMatchCondition("/ws-1")),
				EnableWebsockets: true,
				Services: []projcontour.Service{{
					Name: s1.Name,
					Port: 80,
				}},
			}, {
				Conditions:       matchconditions(prefixMatchCondition("/ws-2")),
				EnableWebsockets: true,
				Services: []projcontour.Service{{
					Name: s1.Name,
					Port: 80,
				}},
			}},
		},
	}
	rh.OnAdd(hp1)

	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("websocket.hello.world",
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/ws-2"),
						Action: withWebsocket(routeCluster("default/ws/80/da39a3ee5e")),
					},
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/ws-1"),
						Action: withWebsocket(routeCluster("default/ws/80/da39a3ee5e")),
					},
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/"),
						Action: routeCluster("default/ws/80/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	hp2 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: s1.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{Fqdn: "websocket.hello.world"},
			Routes: []projcontour.Route{{
				Conditions: matchconditions(prefixMatchCondition("/")),
				Services: []projcontour.Service{{
					Name: s1.Name,
					Port: 80,
				}},
			}, {
				Conditions:       matchconditions(prefixMatchCondition("/ws-1")),
				EnableWebsockets: true,
				Services: []projcontour.Service{{
					Name: s1.Name,
					Port: 80,
				}, {
					Name: s2.Name,
					Port: 80,
				}},
			}, {
				Conditions:       matchconditions(prefixMatchCondition("/ws-2")),
				EnableWebsockets: true,
				Services: []projcontour.Service{{
					Name: s1.Name,
					Port: 80,
				}},
			}},
		},
	}
	rh.OnUpdate(hp1, hp2)

	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("websocket.hello.world",
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/ws-2"),
						Action: withWebsocket(routeCluster("default/ws/80/da39a3ee5e")),
					},
					&envoy_api_v2_route.Route{
						Match: routePrefix("/ws-1"),
						Action: withWebsocket(routeWeightedCluster(
							weightedCluster{"default/ws/80/da39a3ee5e", 1},
							weightedCluster{"default/ws2/80/da39a3ee5e", 1},
						)),
					},
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/"),
						Action: routeCluster("default/ws/80/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
	})

}
