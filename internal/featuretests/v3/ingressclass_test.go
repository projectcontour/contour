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
	"k8s.io/utils/ptr"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/dag"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/featuretests"
	"github.com/projectcontour/contour/internal/fixture"
)

const (
	IngressName   = "kuard-ing"
	HTTPProxyName = "kuard-httpproxy"
	Namespace     = "default"
)

func TestIngressClassAnnotation_Configured(t *testing.T) {
	rh, c, done := setup(t, func(b *dag.Builder) {
		b.Source.IngressClassNames = []string{"linkerd"}
	})
	defer done()

	svc := fixture.NewService("kuard").
		WithPorts(core_v1.ServicePort{Port: 8080, TargetPort: intstr.FromInt(8080)})
	rh.OnAdd(svc)

	// Ingress
	{
		// --- ingress class matches explicitly
		ingressValid := &networking_v1.Ingress{
			ObjectMeta: fixture.ObjectMetaWithAnnotations(IngressName, map[string]string{
				"kubernetes.io/ingress.class": "linkerd",
			}),
			Spec: networking_v1.IngressSpec{
				DefaultBackend: featuretests.IngressBackend(svc),
			},
		}

		rh.OnAdd(ingressValid)

		c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
			Resources: resources(t,
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("*",
						&envoy_config_route_v3.Route{
							Match:  routePrefix("/"),
							Action: routeCluster("default/kuard/8080/da39a3ee5e"),
						},
					),
				),
			),
			TypeUrl: routeType,
		})

		// --- wrong ingress class specified
		ingressWrongClass := &networking_v1.Ingress{
			ObjectMeta: fixture.ObjectMetaWithAnnotations(IngressName, map[string]string{
				"kubernetes.io/ingress.class": "invalid",
			}),
			Spec: networking_v1.IngressSpec{
				DefaultBackend: featuretests.IngressBackend(svc),
			},
		}

		rh.OnUpdate(ingressValid, ingressWrongClass)

		c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
			Resources: resources(t,
				envoy_v3.RouteConfiguration("ingress_http"),
			),
			TypeUrl: routeType,
		})

		// --- no ingress class specified
		ingressNoClass := &networking_v1.Ingress{
			ObjectMeta: fixture.ObjectMeta(IngressName),
			Spec: networking_v1.IngressSpec{
				DefaultBackend: featuretests.IngressBackend(svc),
			},
		}
		rh.OnUpdate(ingressWrongClass, ingressNoClass)

		c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
			Resources: resources(t,
				envoy_v3.RouteConfiguration("ingress_http"),
			),
			TypeUrl: routeType,
		})

		// --- insert valid ingress object
		rh.OnAdd(ingressValid)

		c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
			Resources: resources(t,
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("*",
						&envoy_config_route_v3.Route{
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
		c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
			Resources: resources(t,
				envoy_v3.RouteConfiguration("ingress_http"),
			),
			TypeUrl: routeType,
		})
	}

	// HTTPProxy
	{
		// --- ingress class matches explicitly
		proxyValid := fixture.NewProxy(HTTPProxyName).
			Annotate("projectcontour.io/ingress.class", "linkerd").
			WithSpec(contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "www.example.com",
				},
				Routes: []contour_v1.Route{{
					Services: []contour_v1.Service{{
						Name: svc.Name,
						Port: int(svc.Spec.Ports[0].Port),
					}},
				}},
			})

		rh.OnAdd(proxyValid)

		c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
			Resources: resources(t,
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("www.example.com",
						&envoy_config_route_v3.Route{
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
			WithSpec(contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "www.example.com",
				},
				Routes: []contour_v1.Route{{
					Services: []contour_v1.Service{{
						Name: svc.Name,
						Port: int(svc.Spec.Ports[0].Port),
					}},
				}},
			})

		rh.OnUpdate(proxyValid, proxyWrongClass)

		// ingress class does not match ingress controller, ignored.
		c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
			Resources: resources(t,
				envoy_v3.RouteConfiguration("ingress_http"),
			),
			TypeUrl: routeType,
		})

		// --- no ingress class specified
		proxyNoClass := fixture.NewProxy(HTTPProxyName).
			WithSpec(contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "www.example.com",
				},
				Routes: []contour_v1.Route{{
					Services: []contour_v1.Service{{
						Name: svc.Name,
						Port: int(svc.Spec.Ports[0].Port),
					}},
				}},
			})

		rh.OnUpdate(proxyWrongClass, proxyNoClass)

		// ingress class does not match ingress controller, ignored.
		c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
			Resources: resources(t,
				envoy_v3.RouteConfiguration("ingress_http"),
			),
			TypeUrl: routeType,
		})

		// --- insert valid httpproxy object
		rh.OnAdd(proxyValid)

		c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
			Resources: resources(t,
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("www.example.com",
						&envoy_config_route_v3.Route{
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
		c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
			Resources: resources(t,
				envoy_v3.RouteConfiguration("ingress_http"),
			),
			TypeUrl: routeType,
		})
	}
}

// no configured ingress.class, none on object - pass
// no configured ingress.class, "contour" on object - pass
// no configured ingress.class, anything else on object - fail

func TestIngressClassAnnotation_NotConfigured(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	svc := fixture.NewService("kuard").
		WithPorts(core_v1.ServicePort{Port: 8080, TargetPort: intstr.FromInt(8080)})
	rh.OnAdd(svc)

	// Ingress
	{
		// --- no ingress class specified
		ingressNoClass := &networking_v1.Ingress{
			ObjectMeta: fixture.ObjectMeta(IngressName),
			Spec: networking_v1.IngressSpec{
				DefaultBackend: featuretests.IngressBackend(svc),
			},
		}

		rh.OnAdd(ingressNoClass)

		c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
			Resources: resources(t,
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("*",
						&envoy_config_route_v3.Route{
							Match:  routePrefix("/"),
							Action: routeCluster("default/kuard/8080/da39a3ee5e"),
						},
					),
				),
			),
			TypeUrl: routeType,
		})

		// --- matching ingress class specified
		ingressMatchingClass := &networking_v1.Ingress{
			ObjectMeta: fixture.ObjectMetaWithAnnotations(IngressName, map[string]string{
				"kubernetes.io/ingress.class": "contour",
			}),
			Spec: networking_v1.IngressSpec{
				DefaultBackend: featuretests.IngressBackend(svc),
			},
		}

		rh.OnUpdate(ingressNoClass, ingressMatchingClass)

		c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
			Resources: resources(t,
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("*",
						&envoy_config_route_v3.Route{
							Match:  routePrefix("/"),
							Action: routeCluster("default/kuard/8080/da39a3ee5e"),
						},
					),
				),
			),
			TypeUrl: routeType,
		})

		// --- non-matching ingress class specified
		ingressNonMatchingClass := &networking_v1.Ingress{
			ObjectMeta: fixture.ObjectMetaWithAnnotations(IngressName, map[string]string{
				"kubernetes.io/ingress.class": "invalid",
			}),
			Spec: networking_v1.IngressSpec{
				DefaultBackend: featuretests.IngressBackend(svc),
			},
		}
		rh.OnUpdate(ingressMatchingClass, ingressNonMatchingClass)

		c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
			Resources: resources(t,
				envoy_v3.RouteConfiguration("ingress_http"),
			),
			TypeUrl: routeType,
		})

		// --- insert valid ingress object
		rh.OnAdd(ingressNoClass)

		c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
			Resources: resources(t,
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("*",
						&envoy_config_route_v3.Route{
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
		c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
			Resources: resources(t,
				envoy_v3.RouteConfiguration("ingress_http"),
			),
			TypeUrl: routeType,
		})
	}

	// HTTPProxy
	{
		// --- no ingress class specified
		proxyNoClass := fixture.NewProxy(HTTPProxyName).
			WithSpec(contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "www.example.com",
				},
				Routes: []contour_v1.Route{{
					Services: []contour_v1.Service{{
						Name: svc.Name,
						Port: int(svc.Spec.Ports[0].Port),
					}},
				}},
			})

		rh.OnAdd(proxyNoClass)

		c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
			Resources: resources(t,
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("www.example.com",
						&envoy_config_route_v3.Route{
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
			WithSpec(contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "www.example.com",
				},
				Routes: []contour_v1.Route{{
					Services: []contour_v1.Service{{
						Name: svc.Name,
						Port: int(svc.Spec.Ports[0].Port),
					}},
				}},
			})

		rh.OnUpdate(proxyNoClass, proxyMatchingClass)

		c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
			Resources: resources(t,
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("www.example.com",
						&envoy_config_route_v3.Route{
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
			WithSpec(contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "www.example.com",
				},
				Routes: []contour_v1.Route{{
					Services: []contour_v1.Service{{
						Name: svc.Name,
						Port: int(svc.Spec.Ports[0].Port),
					}},
				}},
			})

		rh.OnUpdate(proxyMatchingClass, proxyNonMatchingClass)

		// ingress class does not match ingress controller, ignored.
		c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
			Resources: resources(t,
				envoy_v3.RouteConfiguration("ingress_http"),
			),
			TypeUrl: routeType,
		})

		// --- insert valid httpproxy object
		rh.OnAdd(proxyNoClass)

		c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
			Resources: resources(t,
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("www.example.com",
						&envoy_config_route_v3.Route{
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
		c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
			Resources: resources(t,
				envoy_v3.RouteConfiguration("ingress_http"),
			),
			TypeUrl: routeType,
		})
	}
}

// TestIngressClassAnnotationUpdate verifies that if an object changes its
// ingress class annotation, we stop paying attention to it.
// TODO(youngnick)#2964: Disabled as part of #2495 work.
func TestIngressClassAnnotationUpdate(t *testing.T) {
	t.Skip("Test disabled, see issue #2964")
	rh, c, done := setup(t, func(b *dag.Builder) {
		b.Source.IngressClassNames = []string{"contour"}
	})
	defer done()

	svc := fixture.NewService("kuard").
		WithPorts(core_v1.ServicePort{Port: 8080, TargetPort: intstr.FromInt(8080)})
	rh.OnAdd(svc)

	vhost := &contour_v1.HTTPProxy{
		ObjectMeta: fixture.ObjectMeta("default/kuard"),
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "kuard.projectcontour.io",
			},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
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

	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("kuard.projectcontour.io",
					&envoy_config_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routeCluster("default/kuard/8080/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
	}).Status(vhost).IsValid()

	// Updating to the non-configured ingress class should remove the
	// vhost.
	orig := vhost.DeepCopy()
	vhost.ObjectMeta.Annotations = map[string]string{
		"kubernetes.io/ingress.class": "not-contour",
	}

	rh.OnUpdate(orig, vhost)

	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http"),
		),
		TypeUrl: routeType,
	}).NoStatus(vhost)
}

func TestIngressClassResource_Configured(t *testing.T) {
	rh, c, done := setup(t, func(b *dag.Builder) {
		b.Source.IngressClassNames = []string{"testingressclass"}
	})
	defer done()

	svc := fixture.NewService("kuard").
		WithPorts(core_v1.ServicePort{Port: 8080, TargetPort: intstr.FromInt(8080)})
	rh.OnAdd(svc)

	ingressClass := networking_v1.IngressClass{
		ObjectMeta: fixture.ObjectMeta("testingressclass"),
		Spec: networking_v1.IngressClassSpec{
			Controller: "something",
		},
	}

	rh.OnAdd(ingressClass)

	// Ingress
	{
		// Spec.IngressClassName matches.
		ingressValid := &networking_v1.Ingress{
			ObjectMeta: fixture.ObjectMeta(IngressName),
			Spec: networking_v1.IngressSpec{
				IngressClassName: ptr.To("testingressclass"),
				DefaultBackend:   featuretests.IngressBackend(svc),
			},
		}

		rh.OnAdd(ingressValid)

		c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
			Resources: resources(t,
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("*",
						&envoy_config_route_v3.Route{
							Match:  routePrefix("/"),
							Action: routeCluster("default/kuard/8080/da39a3ee5e"),
						},
					),
				),
			),
			TypeUrl: routeType,
		})

		// Spec.IngressClassName does not match.
		ingressWrongClass := &networking_v1.Ingress{
			ObjectMeta: fixture.ObjectMeta(IngressName),
			Spec: networking_v1.IngressSpec{
				IngressClassName: ptr.To("wrongingressclass"),
				DefaultBackend:   featuretests.IngressBackend(svc),
			},
		}

		rh.OnUpdate(ingressValid, ingressWrongClass)

		c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
			Resources: resources(t,
				envoy_v3.RouteConfiguration("ingress_http"),
			),
			TypeUrl: routeType,
		})

		// No ingress class specified.
		ingressNoClass := &networking_v1.Ingress{
			ObjectMeta: fixture.ObjectMeta(IngressName),
			Spec: networking_v1.IngressSpec{
				DefaultBackend: featuretests.IngressBackend(svc),
			},
		}
		rh.OnUpdate(ingressWrongClass, ingressNoClass)

		c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
			Resources: resources(t,
				envoy_v3.RouteConfiguration("ingress_http"),
			),
			TypeUrl: routeType,
		})

		// Remove Ingress class.
		rh.OnDelete(ingressClass)

		// Insert valid ingress object
		rh.OnAdd(ingressValid)

		c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
			Resources: resources(t,
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("*",
						&envoy_config_route_v3.Route{
							Match:  routePrefix("/"),
							Action: routeCluster("default/kuard/8080/da39a3ee5e"),
						},
					),
				),
			),
			TypeUrl: routeType,
		})

		rh.OnDelete(ingressValid)

		// Verify ingress is gone.
		c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
			Resources: resources(t,
				envoy_v3.RouteConfiguration("ingress_http"),
			),
			TypeUrl: routeType,
		})
	}

	// HTTPProxy
	{
		// --- ingress class matches explicitly
		proxyValid := fixture.NewProxy(HTTPProxyName).
			WithSpec(contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "www.example.com",
				},
				Routes: []contour_v1.Route{{
					Services: []contour_v1.Service{{
						Name: svc.Name,
						Port: int(svc.Spec.Ports[0].Port),
					}},
				}},
				IngressClassName: "testingressclass",
			})

		rh.OnAdd(proxyValid)

		c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
			Resources: resources(t,
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("www.example.com",
						&envoy_config_route_v3.Route{
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
			WithSpec(contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "www.example.com",
				},
				Routes: []contour_v1.Route{{
					Services: []contour_v1.Service{{
						Name: svc.Name,
						Port: int(svc.Spec.Ports[0].Port),
					}},
				}},
				IngressClassName: "wrongingressclass",
			})

		rh.OnUpdate(proxyValid, proxyWrongClass)

		// ingress class does not match ingress controller, ignored.
		c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
			Resources: resources(t,
				envoy_v3.RouteConfiguration("ingress_http"),
			),
			TypeUrl: routeType,
		})

		// --- no ingress class specified
		proxyNoClass := fixture.NewProxy(HTTPProxyName).
			WithSpec(contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "www.example.com",
				},
				Routes: []contour_v1.Route{{
					Services: []contour_v1.Service{{
						Name: svc.Name,
						Port: int(svc.Spec.Ports[0].Port),
					}},
				}},
			})

		rh.OnUpdate(proxyWrongClass, proxyNoClass)

		// ingress class does not match ingress controller, ignored.
		c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
			Resources: resources(t,
				envoy_v3.RouteConfiguration("ingress_http"),
			),
			TypeUrl: routeType,
		})

		// --- insert valid httpproxy object
		rh.OnAdd(proxyValid)

		c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
			Resources: resources(t,
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("www.example.com",
						&envoy_config_route_v3.Route{
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
		c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
			Resources: resources(t,
				envoy_v3.RouteConfiguration("ingress_http"),
			),
			TypeUrl: routeType,
		})
	}
}

func TestIngressClassResource_NotConfigured(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	svc := fixture.NewService("kuard").
		WithPorts(core_v1.ServicePort{Port: 8080, TargetPort: intstr.FromInt(8080)})
	rh.OnAdd(svc)

	ingressClass := networking_v1.IngressClass{
		ObjectMeta: fixture.ObjectMeta("contour"),
		Spec: networking_v1.IngressClassSpec{
			Controller: "something",
		},
	}

	rh.OnAdd(ingressClass)

	// Ingress
	{
		// No class specified.
		ingressNoClass := &networking_v1.Ingress{
			ObjectMeta: fixture.ObjectMeta(IngressName),
			Spec: networking_v1.IngressSpec{
				DefaultBackend: featuretests.IngressBackend(svc),
			},
		}

		rh.OnAdd(ingressNoClass)

		c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
			Resources: resources(t,
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("*",
						&envoy_config_route_v3.Route{
							Match:  routePrefix("/"),
							Action: routeCluster("default/kuard/8080/da39a3ee5e"),
						},
					),
				),
			),
			TypeUrl: routeType,
		})

		// Spec.IngressClassName matches.
		ingressMatchingClass := &networking_v1.Ingress{
			ObjectMeta: fixture.ObjectMeta(IngressName),
			Spec: networking_v1.IngressSpec{
				IngressClassName: ptr.To("contour"),
				DefaultBackend:   featuretests.IngressBackend(svc),
			},
		}

		rh.OnUpdate(ingressNoClass, ingressMatchingClass)

		c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
			Resources: resources(t,
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("*",
						&envoy_config_route_v3.Route{
							Match:  routePrefix("/"),
							Action: routeCluster("default/kuard/8080/da39a3ee5e"),
						},
					),
				),
			),
			TypeUrl: routeType,
		})

		// Spec.IngressClassName does not match.
		ingressNonMatchingClass := &networking_v1.Ingress{
			ObjectMeta: fixture.ObjectMeta(IngressName),
			Spec: networking_v1.IngressSpec{
				IngressClassName: ptr.To("notcontour"),
				DefaultBackend:   featuretests.IngressBackend(svc),
			},
		}
		rh.OnUpdate(ingressMatchingClass, ingressNonMatchingClass)

		c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
			Resources: resources(t,
				envoy_v3.RouteConfiguration("ingress_http"),
			),
			TypeUrl: routeType,
		})

		// Remove Ingress class.
		rh.OnDelete(ingressClass)

		// Insert valid ingress object
		rh.OnAdd(ingressMatchingClass)

		c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
			Resources: resources(t,
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("*",
						&envoy_config_route_v3.Route{
							Match:  routePrefix("/"),
							Action: routeCluster("default/kuard/8080/da39a3ee5e"),
						},
					),
				),
			),
			TypeUrl: routeType,
		})

		rh.OnDelete(ingressMatchingClass)

		// Verify ingress is gone.
		c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
			Resources: resources(t,
				envoy_v3.RouteConfiguration("ingress_http"),
			),
			TypeUrl: routeType,
		})
	}

	// HTTPProxy
	{
		// --- no ingress class specified
		proxyNoClass := fixture.NewProxy(HTTPProxyName).
			WithSpec(contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "www.example.com",
				},
				Routes: []contour_v1.Route{{
					Services: []contour_v1.Service{{
						Name: svc.Name,
						Port: int(svc.Spec.Ports[0].Port),
					}},
				}},
			})

		rh.OnAdd(proxyNoClass)

		c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
			Resources: resources(t,
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("www.example.com",
						&envoy_config_route_v3.Route{
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
			WithSpec(contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "www.example.com",
				},
				Routes: []contour_v1.Route{{
					Services: []contour_v1.Service{{
						Name: svc.Name,
						Port: int(svc.Spec.Ports[0].Port),
					}},
				}},
				IngressClassName: "contour",
			})

		rh.OnUpdate(proxyNoClass, proxyMatchingClass)

		c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
			Resources: resources(t,
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("www.example.com",
						&envoy_config_route_v3.Route{
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
			WithSpec(contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "www.example.com",
				},
				Routes: []contour_v1.Route{{
					Services: []contour_v1.Service{{
						Name: svc.Name,
						Port: int(svc.Spec.Ports[0].Port),
					}},
				}},
				IngressClassName: "notcontour",
			})

		rh.OnUpdate(proxyMatchingClass, proxyNonMatchingClass)

		// ingress class does not match ingress controller, ignored.
		c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
			Resources: resources(t,
				envoy_v3.RouteConfiguration("ingress_http"),
			),
			TypeUrl: routeType,
		})

		// --- insert valid httpproxy object
		rh.OnAdd(proxyNoClass)

		c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
			Resources: resources(t,
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("www.example.com",
						&envoy_config_route_v3.Route{
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
		c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
			Resources: resources(t,
				envoy_v3.RouteConfiguration("ingress_http"),
			),
			TypeUrl: routeType,
		})
	}
}
