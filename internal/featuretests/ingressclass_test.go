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

	"github.com/projectcontour/contour/internal/fixture"

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
	IngressName   = "kuard-ing"
	HTTPProxyName = "kuard-httpproxy"
	Namespace     = "default"
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
			),
			TypeUrl: routeType,
		})

		rh.OnDelete(ingressValid)

		// verify ingress is gone
		c.Request(routeType).Equals(&v2.DiscoveryResponse{
			Resources: resources(t,
				envoy.RouteConfiguration("ingress_http"),
			),
			TypeUrl: routeType,
		})
	}

	// HTTPProxy
	{
		// --- ingress class matches explicitly
		proxyValid := fixture.NewProxy(HTTPProxyName).
			Annotate("contour.heptio.com/ingress.class", "linkerd").
			WithSpec(projcontour.HTTPProxySpec{
				VirtualHost: &projcontour.VirtualHost{
					Fqdn: "www.example.com",
				},
				Routes: []projcontour.Route{{
					Services: []projcontour.Service{{
						Name: svc.Name,
						Port: int(svc.Spec.Ports[0].Port),
					}},
				}},
			})

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
			),
			TypeUrl: routeType,
		})

		// --- wrong ingress class specified
		proxyWrongClass := fixture.NewProxy(HTTPProxyName).
			Annotate("kubernetes.io/ingress.class", "contour").
			WithSpec(projcontour.HTTPProxySpec{
				VirtualHost: &projcontour.VirtualHost{
					Fqdn: "www.example.com",
				},
				Routes: []projcontour.Route{{
					Services: []projcontour.Service{{
						Name: svc.Name,
						Port: int(svc.Spec.Ports[0].Port),
					}},
				}},
			})

		rh.OnUpdate(proxyValid, proxyWrongClass)

		// ingress class does not match ingress controller, ignored.
		c.Request(routeType).Equals(&v2.DiscoveryResponse{
			Resources: resources(t,
				envoy.RouteConfiguration("ingress_http"),
			),
			TypeUrl: routeType,
		})

		// --- no ingress class specified
		proxyNoClass := fixture.NewProxy(HTTPProxyName).
			WithSpec(projcontour.HTTPProxySpec{
				VirtualHost: &projcontour.VirtualHost{
					Fqdn: "www.example.com",
				},
				Routes: []projcontour.Route{{
					Services: []projcontour.Service{{
						Name: svc.Name,
						Port: int(svc.Spec.Ports[0].Port),
					}},
				}},
			})

		rh.OnUpdate(proxyWrongClass, proxyNoClass)

		// ingress class does not match ingress controller, ignored.
		c.Request(routeType).Equals(&v2.DiscoveryResponse{
			Resources: resources(t,
				envoy.RouteConfiguration("ingress_http"),
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
			),
			TypeUrl: routeType,
		})

		rh.OnDelete(proxyValid)

		// verify ingress is gone
		c.Request(routeType).Equals(&v2.DiscoveryResponse{
			Resources: resources(t,
				envoy.RouteConfiguration("ingress_http"),
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
			),
			TypeUrl: routeType,
		})

		rh.OnDelete(ingressNoClass)

		// verify ingress is gone
		c.Request(routeType).Equals(&v2.DiscoveryResponse{
			Resources: resources(t,
				envoy.RouteConfiguration("ingress_http"),
			),
			TypeUrl: routeType,
		})
	}

	// HTTPProxy
	{
		// --- no ingress class specified
		proxyNoClass := fixture.NewProxy(HTTPProxyName).
			WithSpec(projcontour.HTTPProxySpec{
				VirtualHost: &projcontour.VirtualHost{
					Fqdn: "www.example.com",
				},
				Routes: []projcontour.Route{{
					Services: []projcontour.Service{{
						Name: svc.Name,
						Port: int(svc.Spec.Ports[0].Port),
					}},
				}},
			})

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
			),
			TypeUrl: routeType,
		})

		// --- matching ingress class specified
		proxyMatchingClass := fixture.NewProxy(HTTPProxyName).
			Annotate("kubernetes.io/ingress.class", "contour").
			WithSpec(projcontour.HTTPProxySpec{
				VirtualHost: &projcontour.VirtualHost{
					Fqdn: "www.example.com",
				},
				Routes: []projcontour.Route{{
					Services: []projcontour.Service{{
						Name: svc.Name,
						Port: int(svc.Spec.Ports[0].Port),
					}},
				}},
			})

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
			),
			TypeUrl: routeType,
		})

		// --- non-matching ingress class specified
		proxyNonMatchingClass := fixture.NewProxy(HTTPProxyName).
			Annotate("kubernetes.io/ingress.class", "invalid").
			WithSpec(projcontour.HTTPProxySpec{
				VirtualHost: &projcontour.VirtualHost{
					Fqdn: "www.example.com",
				},
				Routes: []projcontour.Route{{
					Services: []projcontour.Service{{
						Name: svc.Name,
						Port: int(svc.Spec.Ports[0].Port),
					}},
				}},
			})

		rh.OnUpdate(proxyMatchingClass, proxyNonMatchingClass)

		// ingress class does not match ingress controller, ignored.
		c.Request(routeType).Equals(&v2.DiscoveryResponse{
			Resources: resources(t,
				envoy.RouteConfiguration("ingress_http"),
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
			),
			TypeUrl: routeType,
		})

		rh.OnDelete(proxyNoClass)

		// verify ingress is gone
		c.Request(routeType).Equals(&v2.DiscoveryResponse{
			Resources: resources(t,
				envoy.RouteConfiguration("ingress_http"),
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
		ObjectMeta: fixture.ObjectMeta("default/kuard"),
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
		ObjectMeta: fixture.ObjectMeta("default/kuard"),
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
		),
		TypeUrl: routeType,
	}).Status(vhost).Like(
		projcontour.HTTPProxyStatus{CurrentStatus: k8s.StatusValid},
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
		),
		TypeUrl: routeType,
	}).NoStatus(vhost)
}
