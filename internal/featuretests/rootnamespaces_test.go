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
	"github.com/projectcontour/contour/internal/contour"
	"github.com/projectcontour/contour/internal/envoy"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestRootNamespaces(t *testing.T) {
	rh, c, done := setup(t, func(reh *contour.EventHandler) {
		reh.Builder.Source.RootNamespaces = []string{"roots"}
	})
	defer done()

	svc1 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default", // not in root namespace set
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}
	rh.OnAdd(svc1)

	svc2 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      svc1.Name,
			Namespace: "roots", // inside root namespace set
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}
	rh.OnAdd(svc2)

	// assert that there is only a static listener
	c.Request(listenerType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			staticListener(),
		),
		TypeUrl: listenerType,
	})

	// assert that the route tables are empty
	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: nil,
		TypeUrl:   routeType,
	})

	// hp1 is not in the root namespace set.
	hp1 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: svc1.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "hp1.example.com",
			},
			Routes: []projcontour.Route{{
				Conditions: matchconditions(prefixMatchCondition("/")),
				Services: []projcontour.Service{{
					Name: svc1.Name,
					Port: 8080,
				}},
			}},
		},
	}
	rh.OnAdd(hp1)

	// assert that hp1 has no effect on the listener set.
	c.Request(listenerType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			staticListener(),
		),
		TypeUrl: listenerType,
	})

	// assert that the route tables are present but empty.
	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http"),
		),
		TypeUrl: routeType,
	})

	// hp2 is in the root namespace set.
	hp2 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: svc2.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "hp2.example.com",
			},
			Routes: []projcontour.Route{{
				Conditions: matchconditions(prefixMatchCondition("/")),
				Services: []projcontour.Service{{
					Name: svc2.Name,
					Port: 8080,
				}},
			}},
		},
	}
	rh.OnAdd(hp2)

	// assert that hp2 creates port 80 listener.
	c.Request(listenerType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			&v2.Listener{
				Name:    "ingress_http",
				Address: envoy.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy.FilterChains(
					envoy.HTTPConnectionManager("ingress_http", envoy.FileAccessLogEnvoy("/dev/stdout"), 0),
				),
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			},
			staticListener(),
		),
		TypeUrl: listenerType,
	})

	// assert that hp2.example.com's routes are visible.
	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("hp2.example.com",
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/"),
						Action: routeCluster("roots/kuard/8080/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
	})
}
