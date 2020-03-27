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

	ingressroutev1 "github.com/projectcontour/contour/apis/contour/v1beta1"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_api_v2_route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	projcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/contour"
	"github.com/projectcontour/contour/internal/envoy"
	"github.com/projectcontour/contour/internal/k8s"
	v1 "k8s.io/api/core/v1"
	"k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	IngressName      = "kuard-ing"
	IngressRouteName = "kuard-ingressroute"
	HTTPProxyName    = "kuard-httpproxy"
	Namespace        = "default"
)

func TestIngressClassAnnotation_Configured(t *testing.T) {
	rh, c, done := setup(t, func(reh *contour.EventHandler) {
		reh.Builder.Source.IngressClass = "linkerd"
	})
	defer done()

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: Namespace,
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}
	rh.OnAdd(svc)

	// Ingress
	{
		// --- ingress class matches explicitly
		ingressValid := &v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      IngressName,
				Namespace: Namespace,
				Annotations: map[string]string{
					"kubernetes.io/ingress.class": "linkerd",
				},
			},
			Spec: v1beta1.IngressSpec{
				Backend: backend(svc),
			},
		}

		rh.OnAdd(ingressValid)

		c.Request(routeType).Equals(&v2.DiscoveryResponse{
			Resources: resources(t,
				envoy.RouteConfiguration("ingress_http",
					envoy.VirtualHost("*",
						&envoy_api_v2_route.Route{
							Match:  routePrefix("/"),
							Action: routeCluster("default/kuard/8080/da39a3ee5e"),
						},
					),
				),
				envoy.RouteConfiguration("ingress_https"),
			),
			TypeUrl: routeType,
		})

		// --- wrong ingress class specified
		ingressWrongClass := &v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      IngressName,
				Namespace: Namespace,
				Annotations: map[string]string{
					"kubernetes.io/ingress.class": "invalid",
				},
			},
			Spec: v1beta1.IngressSpec{
				Backend: backend(svc),
			},
		}

		rh.OnUpdate(ingressValid, ingressWrongClass)

		c.Request(routeType).Equals(&v2.DiscoveryResponse{
			Resources: resources(t,
				envoy.RouteConfiguration("ingress_http"),
				envoy.RouteConfiguration("ingress_https"),
			),
			TypeUrl: routeType,
		})

		// --- no ingress class specified
		ingressNoClass := &v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      IngressName,
				Namespace: Namespace,
			},
			Spec: v1beta1.IngressSpec{
				Backend: backend(svc),
			},
		}
		rh.OnUpdate(ingressWrongClass, ingressNoClass)

		c.Request(routeType).Equals(&v2.DiscoveryResponse{
			Resources: resources(t,
				envoy.RouteConfiguration("ingress_http"),
				envoy.RouteConfiguration("ingress_https"),
			),
			TypeUrl: routeType,
		})

		// --- insert valid ingress object
		rh.OnAdd(ingressValid)

		c.Request(routeType).Equals(&v2.DiscoveryResponse{
			Resources: resources(t,
				envoy.RouteConfiguration("ingress_http",
					envoy.VirtualHost("*",
						&envoy_api_v2_route.Route{
							Match:  routePrefix("/"),
							Action: routeCluster("default/kuard/8080/da39a3ee5e"),
						},
					),
				),
				envoy.RouteConfiguration("ingress_https"),
			),
			TypeUrl: routeType,
		})

		rh.OnDelete(ingressValid)

		// verify ingress is gone
		c.Request(routeType).Equals(&v2.DiscoveryResponse{
			Resources: resources(t,
				envoy.RouteConfiguration("ingress_http"),
				envoy.RouteConfiguration("ingress_https"),
			),
			TypeUrl: routeType,
		})
	}

	// IngressRoute
	{
		// --- ingress class matches explicitly
		ingressrouteValid := &ingressroutev1.IngressRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      IngressRouteName,
				Namespace: Namespace,
				Annotations: map[string]string{
					"kubernetes.io/ingress.class": "linkerd",
				},
			},
			Spec: ingressroutev1.IngressRouteSpec{
				VirtualHost: &projcontour.VirtualHost{
					Fqdn: "www.example.com",
				},
				Routes: []ingressroutev1.Route{{
					Match: "/",
					Services: []ingressroutev1.Service{{
						Name: svc.Name,
						Port: int(svc.Spec.Ports[0].Port),
					}},
				}},
			},
		}

		rh.OnAdd(ingressrouteValid)

		c.Request(routeType).Equals(&v2.DiscoveryResponse{
			Resources: resources(t,
				envoy.RouteConfiguration("ingress_http",
					envoy.VirtualHost("www.example.com",
						&envoy_api_v2_route.Route{
							Match:  routePrefix("/"),
							Action: routeCluster("default/kuard/8080/da39a3ee5e"),
						},
					),
				),
				envoy.RouteConfiguration("ingress_https"),
			),
			TypeUrl: routeType,
		})

		// --- wrong ingress class specified
		ingressrouteWrongClass := &ingressroutev1.IngressRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      IngressRouteName,
				Namespace: Namespace,
				Annotations: map[string]string{
					"kubernetes.io/ingress.class": "contour",
				},
			},
			Spec: ingressroutev1.IngressRouteSpec{
				VirtualHost: &projcontour.VirtualHost{
					Fqdn: "www.example.com",
				},
				Routes: []ingressroutev1.Route{{
					Services: []ingressroutev1.Service{{
						Name: svc.Name,
						Port: int(svc.Spec.Ports[0].Port),
					}},
				}},
			},
		}

		rh.OnUpdate(ingressrouteValid, ingressrouteWrongClass)

		c.Request(routeType).Equals(&v2.DiscoveryResponse{
			Resources: resources(t,
				envoy.RouteConfiguration("ingress_http"),
				envoy.RouteConfiguration("ingress_https"),
			),
			TypeUrl: routeType,
		})

		// --- no ingress class specified
		ingressrouteNoClass := &ingressroutev1.IngressRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      IngressRouteName,
				Namespace: Namespace,
				Annotations: map[string]string{
					"contour.heptio.com/ingress.class": "contour",
				},
			},
			Spec: ingressroutev1.IngressRouteSpec{
				VirtualHost: &projcontour.VirtualHost{
					Fqdn: "www.example.com",
				},
				Routes: []ingressroutev1.Route{{
					Services: []ingressroutev1.Service{{
						Name: svc.Name,
						Port: int(svc.Spec.Ports[0].Port),
					}},
				}},
			},
		}

		rh.OnUpdate(ingressrouteWrongClass, ingressrouteNoClass)

		c.Request(routeType).Equals(&v2.DiscoveryResponse{
			Resources: resources(t,
				envoy.RouteConfiguration("ingress_http"),
				envoy.RouteConfiguration("ingress_https"),
			),
			TypeUrl: routeType,
		})

		// --- insert valid ingressroute object
		rh.OnAdd(ingressrouteValid)

		c.Request(routeType).Equals(&v2.DiscoveryResponse{
			Resources: resources(t,
				envoy.RouteConfiguration("ingress_http",
					envoy.VirtualHost("www.example.com",
						&envoy_api_v2_route.Route{
							Match:  routePrefix("/"),
							Action: routeCluster("default/kuard/8080/da39a3ee5e"),
						},
					),
				),
				envoy.RouteConfiguration("ingress_https"),
			),
			TypeUrl: routeType,
		})

		rh.OnDelete(ingressrouteValid)

		// verify ingress is gone
		c.Request(routeType).Equals(&v2.DiscoveryResponse{
			Resources: resources(t,
				envoy.RouteConfiguration("ingress_http"),
				envoy.RouteConfiguration("ingress_https"),
			),
			TypeUrl: routeType,
		})
	}

	// HTTPProxy
	{
		// --- ingress class matches explicitly
		proxyValid := &projcontour.HTTPProxy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      HTTPProxyName,
				Namespace: Namespace,
				Annotations: map[string]string{
					"contour.heptio.com/ingress.class": "linkerd",
				},
			},
			Spec: projcontour.HTTPProxySpec{
				VirtualHost: &projcontour.VirtualHost{
					Fqdn: "www.example.com",
				},
				Routes: []projcontour.Route{{
					Services: []projcontour.Service{{
						Name: svc.Name,
						Port: int(svc.Spec.Ports[0].Port),
					}},
				}},
			},
		}

		rh.OnAdd(proxyValid)

		c.Request(routeType).Equals(&v2.DiscoveryResponse{
			Resources: resources(t,
				envoy.RouteConfiguration("ingress_http",
					envoy.VirtualHost("www.example.com",
						&envoy_api_v2_route.Route{
							Match:  routePrefix("/"),
							Action: routeCluster("default/kuard/8080/da39a3ee5e"),
						},
					),
				),
				envoy.RouteConfiguration("ingress_https"),
			),
			TypeUrl: routeType,
		})

		// --- wrong ingress class specified
		proxyWrongClass := &projcontour.HTTPProxy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      HTTPProxyName,
				Namespace: Namespace,
				Annotations: map[string]string{
					"kubernetes.io/ingress.class": "contour",
				},
			},
			Spec: projcontour.HTTPProxySpec{
				VirtualHost: &projcontour.VirtualHost{
					Fqdn: "www.example.com",
				},
				Routes: []projcontour.Route{{
					Services: []projcontour.Service{{
						Name: svc.Name,
						Port: int(svc.Spec.Ports[0].Port),
					}},
				}},
			},
		}

		rh.OnUpdate(proxyValid, proxyWrongClass)

		// ingress class does not match ingress controller, ignored.
		c.Request(routeType).Equals(&v2.DiscoveryResponse{
			Resources: resources(t,
				envoy.RouteConfiguration("ingress_http"),
				envoy.RouteConfiguration("ingress_https"),
			),
			TypeUrl: routeType,
		})

		// --- no ingress class specified
		proxyNoClass := &projcontour.HTTPProxy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      HTTPProxyName,
				Namespace: Namespace,
			},
			Spec: projcontour.HTTPProxySpec{
				VirtualHost: &projcontour.VirtualHost{
					Fqdn: "www.example.com",
				},
				Routes: []projcontour.Route{{
					Services: []projcontour.Service{{
						Name: svc.Name,
						Port: int(svc.Spec.Ports[0].Port),
					}},
				}},
			},
		}

		rh.OnUpdate(proxyWrongClass, proxyNoClass)

		// ingress class does not match ingress controller, ignored.
		c.Request(routeType).Equals(&v2.DiscoveryResponse{
			Resources: resources(t,
				envoy.RouteConfiguration("ingress_http"),
				envoy.RouteConfiguration("ingress_https"),
			),
			TypeUrl: routeType,
		})

		// --- insert valid httpproxy object
		rh.OnAdd(proxyValid)

		c.Request(routeType).Equals(&v2.DiscoveryResponse{
			Resources: resources(t,
				envoy.RouteConfiguration("ingress_http",
					envoy.VirtualHost("www.example.com",
						&envoy_api_v2_route.Route{
							Match:  routePrefix("/"),
							Action: routeCluster("default/kuard/8080/da39a3ee5e"),
						},
					),
				),
				envoy.RouteConfiguration("ingress_https"),
			),
			TypeUrl: routeType,
		})

		rh.OnDelete(proxyValid)

		// verify ingress is gone
		c.Request(routeType).Equals(&v2.DiscoveryResponse{
			Resources: resources(t,
				envoy.RouteConfiguration("ingress_http"),
				envoy.RouteConfiguration("ingress_https"),
			),
			TypeUrl: routeType,
		})
	}
}

//no configured ingress.class, none on object - pass
//no configured ingress.class, "contour" on object - pass
//no configured ingress.class, anything else on object - fail

func TestIngressClassAnnotation_NotConfigured(t *testing.T) {
	rh, c, done := setup(t, func(reh *contour.EventHandler) {})
	defer done()

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: Namespace,
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}
	rh.OnAdd(svc)

	// Ingress
	{
		// --- no ingress class specified
		ingressNoClass := &v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      IngressName,
				Namespace: Namespace,
			},
			Spec: v1beta1.IngressSpec{
				Backend: backend(svc),
			},
		}

		rh.OnAdd(ingressNoClass)

		c.Request(routeType).Equals(&v2.DiscoveryResponse{
			Resources: resources(t,
				envoy.RouteConfiguration("ingress_http",
					envoy.VirtualHost("*",
						&envoy_api_v2_route.Route{
							Match:  routePrefix("/"),
							Action: routeCluster("default/kuard/8080/da39a3ee5e"),
						},
					),
				),
				envoy.RouteConfiguration("ingress_https"),
			),
			TypeUrl: routeType,
		})

		// --- matching ingress class specified
		ingressMatchingClass := &v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      IngressName,
				Namespace: Namespace,
				Annotations: map[string]string{
					"kubernetes.io/ingress.class": "contour",
				},
			},
			Spec: v1beta1.IngressSpec{
				Backend: backend(svc),
			},
		}

		rh.OnUpdate(ingressNoClass, ingressMatchingClass)

		c.Request(routeType).Equals(&v2.DiscoveryResponse{
			Resources: resources(t,
				envoy.RouteConfiguration("ingress_http",
					envoy.VirtualHost("*",
						&envoy_api_v2_route.Route{
							Match:  routePrefix("/"),
							Action: routeCluster("default/kuard/8080/da39a3ee5e"),
						},
					),
				),
				envoy.RouteConfiguration("ingress_https"),
			),
			TypeUrl: routeType,
		})

		// --- non-matching ingress class specified
		ingressNonMatchingClass := &v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      IngressName,
				Namespace: Namespace,
				Annotations: map[string]string{
					"kubernetes.io/ingress.class": "invalid",
				},
			},
			Spec: v1beta1.IngressSpec{
				Backend: backend(svc),
			},
		}
		rh.OnUpdate(ingressMatchingClass, ingressNonMatchingClass)

		c.Request(routeType).Equals(&v2.DiscoveryResponse{
			Resources: resources(t,
				envoy.RouteConfiguration("ingress_http"),
				envoy.RouteConfiguration("ingress_https"),
			),
			TypeUrl: routeType,
		})

		// --- insert valid ingress object
		rh.OnAdd(ingressNoClass)

		c.Request(routeType).Equals(&v2.DiscoveryResponse{
			Resources: resources(t,
				envoy.RouteConfiguration("ingress_http",
					envoy.VirtualHost("*",
						&envoy_api_v2_route.Route{
							Match:  routePrefix("/"),
							Action: routeCluster("default/kuard/8080/da39a3ee5e"),
						},
					),
				),
				envoy.RouteConfiguration("ingress_https"),
			),
			TypeUrl: routeType,
		})

		rh.OnDelete(ingressNoClass)

		// verify ingress is gone
		c.Request(routeType).Equals(&v2.DiscoveryResponse{
			Resources: resources(t,
				envoy.RouteConfiguration("ingress_http"),
				envoy.RouteConfiguration("ingress_https"),
			),
			TypeUrl: routeType,
		})
	}

	// IngressRoute
	{
		// --- no ingress class specified
		ingressRouteNoClass := &ingressroutev1.IngressRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      IngressRouteName,
				Namespace: Namespace,
			},
			Spec: ingressroutev1.IngressRouteSpec{
				VirtualHost: &projcontour.VirtualHost{
					Fqdn: "www.example.com",
				},
				Routes: []ingressroutev1.Route{{
					Match: "/",
					Services: []ingressroutev1.Service{{
						Name: svc.Name,
						Port: int(svc.Spec.Ports[0].Port),
					}},
				}},
			},
		}

		rh.OnAdd(ingressRouteNoClass)

		c.Request(routeType).Equals(&v2.DiscoveryResponse{
			Resources: resources(t,
				envoy.RouteConfiguration("ingress_http",
					envoy.VirtualHost("www.example.com",
						&envoy_api_v2_route.Route{
							Match:  routePrefix("/"),
							Action: routeCluster("default/kuard/8080/da39a3ee5e"),
						},
					),
				),
				envoy.RouteConfiguration("ingress_https"),
			),
			TypeUrl: routeType,
		})

		// --- matching ingress class specified
		ingressrouteMatchingClass := &ingressroutev1.IngressRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      IngressRouteName,
				Namespace: Namespace,
				Annotations: map[string]string{
					"kubernetes.io/ingress.class": "contour",
				},
			},
			Spec: ingressroutev1.IngressRouteSpec{
				VirtualHost: &projcontour.VirtualHost{
					Fqdn: "www.example.com",
				},
				Routes: []ingressroutev1.Route{{
					Match: "/",
					Services: []ingressroutev1.Service{{
						Name: svc.Name,
						Port: int(svc.Spec.Ports[0].Port),
					}},
				}},
			},
		}

		rh.OnUpdate(ingressRouteNoClass, ingressrouteMatchingClass)

		c.Request(routeType).Equals(&v2.DiscoveryResponse{
			Resources: resources(t,
				envoy.RouteConfiguration("ingress_http",
					envoy.VirtualHost("www.example.com",
						&envoy_api_v2_route.Route{
							Match:  routePrefix("/"),
							Action: routeCluster("default/kuard/8080/da39a3ee5e"),
						},
					),
				),
				envoy.RouteConfiguration("ingress_https"),
			),
			TypeUrl: routeType,
		})

		// --- non-matching ingress class specified
		ingressrouteNonMatchingClass := &ingressroutev1.IngressRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      IngressRouteName,
				Namespace: Namespace,
				Annotations: map[string]string{
					"contour.heptio.com/ingress.class": "invalid",
				},
			},
			Spec: ingressroutev1.IngressRouteSpec{
				VirtualHost: &projcontour.VirtualHost{
					Fqdn: "www.example.com",
				},
				Routes: []ingressroutev1.Route{{
					Services: []ingressroutev1.Service{{
						Name: svc.Name,
						Port: int(svc.Spec.Ports[0].Port),
					}},
				}},
			},
		}

		rh.OnUpdate(ingressrouteMatchingClass, ingressrouteNonMatchingClass)

		c.Request(routeType).Equals(&v2.DiscoveryResponse{
			Resources: resources(t,
				envoy.RouteConfiguration("ingress_http"),
				envoy.RouteConfiguration("ingress_https"),
			),
			TypeUrl: routeType,
		})

		// --- insert valid ingressroute object
		rh.OnAdd(ingressRouteNoClass)

		c.Request(routeType).Equals(&v2.DiscoveryResponse{
			Resources: resources(t,
				envoy.RouteConfiguration("ingress_http",
					envoy.VirtualHost("www.example.com",
						&envoy_api_v2_route.Route{
							Match:  routePrefix("/"),
							Action: routeCluster("default/kuard/8080/da39a3ee5e"),
						},
					),
				),
				envoy.RouteConfiguration("ingress_https"),
			),
			TypeUrl: routeType,
		})

		rh.OnDelete(ingressRouteNoClass)

		// verify ingress is gone
		c.Request(routeType).Equals(&v2.DiscoveryResponse{
			Resources: resources(t,
				envoy.RouteConfiguration("ingress_http"),
				envoy.RouteConfiguration("ingress_https"),
			),
			TypeUrl: routeType,
		})
	}

	// HTTPProxy
	{
		// --- no ingress class specified
		proxyNoClass := &projcontour.HTTPProxy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      HTTPProxyName,
				Namespace: Namespace,
			},
			Spec: projcontour.HTTPProxySpec{
				VirtualHost: &projcontour.VirtualHost{
					Fqdn: "www.example.com",
				},
				Routes: []projcontour.Route{{
					Services: []projcontour.Service{{
						Name: svc.Name,
						Port: int(svc.Spec.Ports[0].Port),
					}},
				}},
			},
		}

		rh.OnAdd(proxyNoClass)

		c.Request(routeType).Equals(&v2.DiscoveryResponse{
			Resources: resources(t,
				envoy.RouteConfiguration("ingress_http",
					envoy.VirtualHost("www.example.com",
						&envoy_api_v2_route.Route{
							Match:  routePrefix("/"),
							Action: routeCluster("default/kuard/8080/da39a3ee5e"),
						},
					),
				),
				envoy.RouteConfiguration("ingress_https"),
			),
			TypeUrl: routeType,
		})

		// --- matching ingress class specified
		proxyMatchingClass := &projcontour.HTTPProxy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      HTTPProxyName,
				Namespace: Namespace,
				Annotations: map[string]string{
					"kubernetes.io/ingress.class": "contour",
				},
			},
			Spec: projcontour.HTTPProxySpec{
				VirtualHost: &projcontour.VirtualHost{
					Fqdn: "www.example.com",
				},
				Routes: []projcontour.Route{{
					Services: []projcontour.Service{{
						Name: svc.Name,
						Port: int(svc.Spec.Ports[0].Port),
					}},
				}},
			},
		}

		rh.OnUpdate(proxyNoClass, proxyMatchingClass)

		c.Request(routeType).Equals(&v2.DiscoveryResponse{
			Resources: resources(t,
				envoy.RouteConfiguration("ingress_http",
					envoy.VirtualHost("www.example.com",
						&envoy_api_v2_route.Route{
							Match:  routePrefix("/"),
							Action: routeCluster("default/kuard/8080/da39a3ee5e"),
						},
					),
				),
				envoy.RouteConfiguration("ingress_https"),
			),
			TypeUrl: routeType,
		})

		// --- non-matching ingress class specified
		proxyNonMatchingClass := &projcontour.HTTPProxy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      HTTPProxyName,
				Namespace: Namespace,
				Annotations: map[string]string{
					"kubernetes.io/ingress.class": "invalid",
				},
			},
			Spec: projcontour.HTTPProxySpec{
				VirtualHost: &projcontour.VirtualHost{
					Fqdn: "www.example.com",
				},
				Routes: []projcontour.Route{{
					Services: []projcontour.Service{{
						Name: svc.Name,
						Port: int(svc.Spec.Ports[0].Port),
					}},
				}},
			},
		}

		rh.OnUpdate(proxyMatchingClass, proxyNonMatchingClass)

		// ingress class does not match ingress controller, ignored.
		c.Request(routeType).Equals(&v2.DiscoveryResponse{
			Resources: resources(t,
				envoy.RouteConfiguration("ingress_http"),
				envoy.RouteConfiguration("ingress_https"),
			),
			TypeUrl: routeType,
		})

		// --- insert valid httpproxy object
		rh.OnAdd(proxyNoClass)

		c.Request(routeType).Equals(&v2.DiscoveryResponse{
			Resources: resources(t,
				envoy.RouteConfiguration("ingress_http",
					envoy.VirtualHost("www.example.com",
						&envoy_api_v2_route.Route{
							Match:  routePrefix("/"),
							Action: routeCluster("default/kuard/8080/da39a3ee5e"),
						},
					),
				),
				envoy.RouteConfiguration("ingress_https"),
			),
			TypeUrl: routeType,
		})

		rh.OnDelete(proxyNoClass)

		// verify ingress is gone
		c.Request(routeType).Equals(&v2.DiscoveryResponse{
			Resources: resources(t,
				envoy.RouteConfiguration("ingress_http"),
				envoy.RouteConfiguration("ingress_https"),
			),
			TypeUrl: routeType,
		})
	}
}

// TestIngressClassUpdate verifies that if an object changes its ingress
// class, we stop paying attention to it.
func TestIngressClassUpdate(t *testing.T) {
	rh, c, done := setup(t, func(reh *contour.EventHandler) {
		reh.Builder.Source.IngressClass = "contour"
	})
	defer done()

	svc := &v1.Service{
		ObjectMeta: meta("default/kuard"),
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}
	rh.OnAdd(svc)

	vhost := &projcontour.HTTPProxy{
		ObjectMeta: meta("default/kuard"),
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "kuard.projectcontour.io",
			},
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	// With the configured ingress class, a virtual show should be added.
	vhost.ObjectMeta.Annotations = map[string]string{
		"kubernetes.io/ingress.class": "contour",
	}

	rh.OnAdd(vhost)

	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("kuard.projectcontour.io",
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/"),
						Action: routeCluster("default/kuard/8080/da39a3ee5e"),
					},
				),
			),
			envoy.RouteConfiguration("ingress_https"),
		),
		TypeUrl: routeType,
	}).Status(vhost).Like(
		projcontour.Status{CurrentStatus: k8s.StatusValid},
	)

	// Updating to the non-configured ingress class should remove the
	// vhost.
	orig := vhost.DeepCopy()
	vhost.ObjectMeta.Annotations = map[string]string{
		"kubernetes.io/ingress.class": "not-contour",
	}

	rh.OnUpdate(orig, vhost)

	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http"),
			envoy.RouteConfiguration("ingress_https"),
		),
		TypeUrl: routeType,
	}).NoStatus(vhost)
}
