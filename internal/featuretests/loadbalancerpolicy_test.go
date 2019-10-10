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

package featuretests

import (
	"testing"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	ingressroutev1 "github.com/projectcontour/contour/apis/contour/v1beta1"
	projcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/envoy"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// session affinity is only available in ingressroute and httpproxy
func TestLoadBalancerPolicySessionAffinity(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	s1 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}, {
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}
	rh.OnAdd(s1)

	// simple single service
	ir1 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: s1.Namespace,
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &projcontour.VirtualHost{Fqdn: "www.example.com"},
			Routes: []ingressroutev1.Route{{
				Match: "/cart",
				Services: []ingressroutev1.Service{{
					Name:     s1.Name,
					Port:     80,
					Strategy: "Cookie",
				}},
			}},
		},
	}
	rh.OnAdd(ir1)

	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("www.example.com",
					envoy.Route(envoy.RoutePrefix("/cart"), withSessionAffinity(routeCluster("default/app/80/e4f81994fe"))),
				),
			),
			envoy.RouteConfiguration("ingress_https"),
		),
		TypeUrl: routeType,
	})

	// two backends
	ir2 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: s1.Namespace,
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &projcontour.VirtualHost{Fqdn: "www.example.com"},
			Routes: []ingressroutev1.Route{{
				Match: "/cart",
				Services: []ingressroutev1.Service{{
					Name:     s1.Name,
					Port:     80,
					Strategy: "Cookie",
				}, {
					Name:     s1.Name,
					Port:     8080,
					Strategy: "Cookie",
				}},
			}},
		},
	}
	rh.OnUpdate(ir1, ir2)

	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("www.example.com",
					envoy.Route(envoy.RoutePrefix("/cart"), withSessionAffinity(
						routeWeightedCluster(
							weightedCluster{"default/app/80/e4f81994fe", 1},
							weightedCluster{"default/app/8080/e4f81994fe", 1},
						),
					)),
				),
			),
			envoy.RouteConfiguration("ingress_https"),
		),
		TypeUrl: routeType,
	})

	// two mixed backends
	ir3 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: s1.Namespace,
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &projcontour.VirtualHost{Fqdn: "www.example.com"},
			Routes: []ingressroutev1.Route{{
				Match: "/cart",
				Services: []ingressroutev1.Service{{
					Name:     s1.Name,
					Port:     80,
					Strategy: "Cookie",
				}, {
					Name: s1.Name,
					Port: 8080,
				}},
			}, {
				Match: "/",
				Services: []ingressroutev1.Service{{
					Name: s1.Name,
					Port: 80,
				}},
			}},
		},
	}
	rh.OnUpdate(ir2, ir3)

	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("www.example.com",
					envoy.Route(envoy.RoutePrefix("/cart"), withSessionAffinity(
						routeWeightedCluster(
							weightedCluster{"default/app/80/e4f81994fe", 1},
							weightedCluster{"default/app/8080/da39a3ee5e", 1},
						),
					)),
					envoy.Route(envoy.RoutePrefix("/"), routeCluster("default/app/80/da39a3ee5e")),
				),
			),
			envoy.RouteConfiguration("ingress_https"),
		),
		TypeUrl: routeType,
	})

	rh.OnDelete(ir3)

	// simple single service
	proxy1 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: s1.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{Fqdn: "www.example.com"},
			Routes: []projcontour.Route{{
				Conditions: prefixCondition("/cart"),
				LoadBalancerPolicy: &projcontour.LoadBalancerPolicy{
					Strategy: "Cookie",
				},
				Services: []projcontour.Service{{
					Name: s1.Name,
					Port: 80,
				}},
			}},
		},
	}
	rh.OnAdd(proxy1)

	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("www.example.com",
					envoy.Route(envoy.RoutePrefix("/cart"), withSessionAffinity(routeCluster("default/app/80/e4f81994fe"))),
				),
			),
			envoy.RouteConfiguration("ingress_https"),
		),
		TypeUrl: routeType,
	})

	// two backends
	proxy2 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: s1.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{Fqdn: "www.example.com"},
			Routes: []projcontour.Route{{
				Conditions: prefixCondition("/cart"),
				LoadBalancerPolicy: &projcontour.LoadBalancerPolicy{
					Strategy: "Cookie",
				},
				Services: []projcontour.Service{{
					Name: s1.Name,
					Port: 80,
				}, {
					Name: s1.Name,
					Port: 8080,
				}},
			}},
		},
	}
	rh.OnUpdate(proxy1, proxy2)

	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("www.example.com",
					envoy.Route(envoy.RoutePrefix("/cart"), withSessionAffinity(
						routeWeightedCluster(
							weightedCluster{"default/app/80/e4f81994fe", 1},
							weightedCluster{"default/app/8080/e4f81994fe", 1},
						),
					)),
				),
			),
			envoy.RouteConfiguration("ingress_https"),
		),
		TypeUrl: routeType,
	})
}
