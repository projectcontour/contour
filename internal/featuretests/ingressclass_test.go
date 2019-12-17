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
	envoy_api_v2_route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	ingressroutev1 "github.com/projectcontour/contour/apis/contour/v1beta1"
	projcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/contour"
	"github.com/projectcontour/contour/internal/envoy"
	v1 "k8s.io/api/core/v1"
	"k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestIngressClassAnnotation(t *testing.T) {
	rh, c, done := setup(t, func(reh *contour.EventHandler) {
		reh.Builder.Source.IngressClass = "linkerd"
	})
	defer done()

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
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

	i1 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard-ing",
			Namespace: svc.Namespace,
		},
		Spec: v1beta1.IngressSpec{
			Backend: backend(svc),
		},
	}
	rh.OnAdd(i1)

	// ingress object without a class matches any ingress controller
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

	i2 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      i1.Name,
			Namespace: i1.Namespace,
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "contour",
			},
		},
		Spec: v1beta1.IngressSpec{
			Backend: backend(svc),
		},
	}
	rh.OnUpdate(i1, i2)

	// ingress class does not match ingress controller, ignored.
	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http"),
			envoy.RouteConfiguration("ingress_https"),
		),
		TypeUrl: routeType,
	})

	i3 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      i2.Name,
			Namespace: i2.Namespace,
			Annotations: map[string]string{
				"contour.heptio.com/ingress.class": "contour",
			},
		},
		Spec: v1beta1.IngressSpec{
			Backend: backend(svc),
		},
	}
	rh.OnUpdate(i2, i3)

	// ingress class does not match
	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http"),
			envoy.RouteConfiguration("ingress_https"),
		),
		TypeUrl: routeType,
	})

	i4 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      i3.Name,
			Namespace: i3.Namespace,
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "linkerd",
			},
		},
		Spec: v1beta1.IngressSpec{
			Backend: backend(svc),
		},
	}
	rh.OnUpdate(i3, i4)

	// ingress class matches explicitly.
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

	i5 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      i4.Name,
			Namespace: i4.Namespace,
			Annotations: map[string]string{
				"contour.heptio.com/ingress.class": "linkerd",
			},
		},
		Spec: v1beta1.IngressSpec{
			Backend: backend(svc),
		},
	}
	rh.OnUpdate(i4, i5)

	// ingress class matches explicitly.
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

	i6 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      i5.Name,
			Namespace: i5.Namespace,
			Annotations: map[string]string{
				"projectcontour.io/ingress.class": "contour",
			},
		},
		Spec: v1beta1.IngressSpec{
			Backend: backend(svc),
		},
	}
	rh.OnUpdate(i5, i6)

	// ingress class does not match ingress controller, ignored.
	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http"),
			envoy.RouteConfiguration("ingress_https"),
		),
		TypeUrl: routeType,
	})

	i7 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      i6.Name,
			Namespace: i6.Namespace,
			Annotations: map[string]string{
				"contour.heptio.com/ingress.class": "linkerd",
			},
		},
		Spec: v1beta1.IngressSpec{
			Backend: backend(svc),
		},
	}
	rh.OnUpdate(i6, i7)

	// ingress class matches explicitly.
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

	rh.OnDelete(i7)
	// gone
	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http"),
			envoy.RouteConfiguration("ingress_https"),
		),
		TypeUrl: routeType,
	})

	ir1 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard-ingressroute",
			Namespace: svc.Namespace,
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
	rh.OnAdd(ir1)

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

	ir2 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ir1.Name,
			Namespace: ir1.Namespace,
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
	rh.OnUpdate(ir1, ir2)

	// ingress class does not match ingress controller, ignored.
	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http"),
			envoy.RouteConfiguration("ingress_https"),
		),
		TypeUrl: routeType,
	})

	ir3 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ir2.Name,
			Namespace: ir2.Namespace,
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
	rh.OnUpdate(ir2, ir3)

	// ingress class does not match ingress controller, ignored.
	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http"),
			envoy.RouteConfiguration("ingress_https"),
		),
		TypeUrl: routeType,
	})

	ir4 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ir3.Name,
			Namespace: ir3.Namespace,
			Annotations: map[string]string{
				"contour.heptio.com/ingress.class": "linkerd",
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
	rh.OnUpdate(ir3, ir4)

	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost(ir4.Spec.VirtualHost.Fqdn,
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

	ir5 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ir4.Name,
			Namespace: ir4.Namespace,
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "linkerd",
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

	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost(ir5.Spec.VirtualHost.Fqdn,
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

	ir6 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ir5.Name,
			Namespace: ir5.Namespace,
			Annotations: map[string]string{
				"projectcontour.io/ingress.class": "contour",
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
	rh.OnUpdate(ir5, ir6)

	// ingress class does not match ingress controller, ignored.
	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http"),
			envoy.RouteConfiguration("ingress_https"),
		),
		TypeUrl: routeType,
	})

	ir7 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ir6.Name,
			Namespace: ir6.Namespace,
			Annotations: map[string]string{
				"contour.heptio.com/ingress.class": "linkerd",
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
	rh.OnUpdate(ir6, ir4)

	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost(ir5.Spec.VirtualHost.Fqdn,
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

	rh.OnDelete(ir7)

	proxy1 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard-httpproxy",
			Namespace: svc.Namespace,
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
	rh.OnAdd(proxy1)

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

	proxy2 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      proxy1.Name,
			Namespace: proxy1.Namespace,
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

	rh.OnUpdate(proxy1, proxy2)

	// ingress class does not match ingress controller, ignored.
	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http"),
			envoy.RouteConfiguration("ingress_https"),
		),
		TypeUrl: routeType,
	})

	proxy3 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      proxy2.Name,
			Namespace: proxy2.Namespace,
			Annotations: map[string]string{
				"contour.heptio.com/ingress.class": "contour",
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
	rh.OnUpdate(proxy2, proxy3)

	// ingress class does not match ingress controller, ignored.
	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http"),
			envoy.RouteConfiguration("ingress_https"),
		),
		TypeUrl: routeType,
	})

	proxy4 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      proxy3.Name,
			Namespace: proxy3.Namespace,
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
	rh.OnUpdate(proxy3, proxy4)

	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost(proxy4.Spec.VirtualHost.Fqdn,
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

	proxy5 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      proxy4.Name,
			Namespace: proxy4.Namespace,
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "linkerd",
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

	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost(proxy5.Spec.VirtualHost.Fqdn,
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

	proxy6 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      proxy5.Name,
			Namespace: proxy5.Namespace,
			Annotations: map[string]string{
				"projectcontour.io/ingress.class": "contour",
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
	rh.OnUpdate(proxy5, proxy6)

	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http"),
			envoy.RouteConfiguration("ingress_https"),
		),
		TypeUrl: routeType,
	})

	proxy7 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      proxy6.Name,
			Namespace: proxy6.Namespace,
			Annotations: map[string]string{
				"projectcontour.io/ingress.class": "linkerd",
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
	rh.OnUpdate(proxy6, proxy7)

	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost(proxy5.Spec.VirtualHost.Fqdn,
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

	rh.OnDelete(proxy7)
}
