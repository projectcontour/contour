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
	"time"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_api_v2_route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	projcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/contour"
	"github.com/projectcontour/contour/internal/envoy"
	v1 "k8s.io/api/core/v1"
	"k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestTimeoutPolicyRequestTimeout(t *testing.T) {
	rh, c, done := setup(t, func(reh *contour.EventHandler) {})
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
			Annotations: map[string]string{
				"projectcontour.io/response-timeout": "1m20s",
			},
		},
		Spec: v1beta1.IngressSpec{
			Backend: backend(svc),
		},
	}
	rh.OnAdd(i1)

	// check annotation with explicit timeout is propagated
	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("*",
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/"),
						Action: withResponseTimeout(routeCluster("default/kuard/8080/da39a3ee5e"), 80*time.Second),
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	i2 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard-ing",
			Namespace: svc.Namespace,
			Annotations: map[string]string{
				"projectcontour.io/response-timeout": "infinity",
			},
		},
		Spec: i1.Spec,
	}
	rh.OnUpdate(i1, i2)

	// check annotation with infinite timeout is propogated
	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("*",
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/"),
						Action: withResponseTimeout(routeCluster("default/kuard/8080/da39a3ee5e"), 0), // zero means infinity
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	i3 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard-ing",
			Namespace: svc.Namespace,
			Annotations: map[string]string{
				"projectcontour.io/response-timeout": "monday",
			},
		},
		Spec: i2.Spec,
	}
	rh.OnUpdate(i2, i3)

	// check annotation with malformed timeout is propogated as infinity
	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("*",
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/"),
						Action: withResponseTimeout(routeCluster("default/kuard/8080/da39a3ee5e"), 0), // zero means infinity
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	i4 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard-ing",
			Namespace: svc.Namespace,
			Annotations: map[string]string{
				"contour.heptio.com/request-timeout": "90s",
				"projectcontour.io/response-timeout": "99s",
			},
		},
		Spec: i2.Spec,
	}
	rh.OnUpdate(i3, i4)

	// assert that projectcontour.io/response-timeout takes priority.
	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("*",
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/"),
						Action: withResponseTimeout(routeCluster("default/kuard/8080/da39a3ee5e"), 99*time.Second),
					},
				),
			),
		),
		TypeUrl: routeType,
	})
	rh.OnDelete(i4)

	p1 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: svc.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{Fqdn: "test2.test.com"},
			Routes: []projcontour.Route{{
				Conditions: matchconditions(prefixMatchCondition("/")),
				TimeoutPolicy: &projcontour.TimeoutPolicy{
					Response: "600", // not 600s
				},
				Services: []projcontour.Service{{
					Name: svc.Name,
					Port: 8080,
				}},
			}},
		},
	}
	rh.OnAdd(p1)

	// check timeout policy with malformed response timeout is propogated as infinity
	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("test2.test.com",
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/"),
						Action: withResponseTimeout(routeCluster("default/kuard/8080/da39a3ee5e"), 0), // zero means infinity
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	p2 := &projcontour.HTTPProxy{
		ObjectMeta: p1.ObjectMeta,
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{Fqdn: "test2.test.com"},
			Routes: []projcontour.Route{{
				Conditions: matchconditions(prefixMatchCondition("/")),
				TimeoutPolicy: &projcontour.TimeoutPolicy{
					Response: "3m",
				},
				Services: []projcontour.Service{{
					Name: svc.Name,
					Port: 8080,
				}},
			}},
		},
	}
	rh.OnUpdate(p1, p2)

	// check timeout policy with response timeout is propogated correctly
	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("test2.test.com",
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/"),
						Action: withResponseTimeout(routeCluster("default/kuard/8080/da39a3ee5e"), 180*time.Second),
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	p3 := &projcontour.HTTPProxy{
		ObjectMeta: p2.ObjectMeta,
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{Fqdn: "test2.test.com"},
			Routes: []projcontour.Route{{
				Conditions: matchconditions(prefixMatchCondition("/")),
				TimeoutPolicy: &projcontour.TimeoutPolicy{
					Response: "infinty",
				},
				Services: []projcontour.Service{{
					Name: svc.Name,
					Port: 8080,
				}},
			}},
		},
	}
	rh.OnUpdate(p2, p3)

	// check timeout policy with explicit infine response timeout is propogated as infinity
	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("test2.test.com",
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/"),
						Action: withResponseTimeout(routeCluster("default/kuard/8080/da39a3ee5e"), 0), // zero means infinity
					},
				),
			),
		),
		TypeUrl: routeType,
	})
}

func TestTimeoutPolicyIdleTimeout(t *testing.T) {
	rh, c, done := setup(t, func(reh *contour.EventHandler) {})
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

	p1 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: svc.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{Fqdn: "test2.test.com"},
			Routes: []projcontour.Route{{
				Conditions: matchconditions(prefixMatchCondition("/")),
				TimeoutPolicy: &projcontour.TimeoutPolicy{
					Idle: "600", // not 600s
				},
				Services: []projcontour.Service{{
					Name: svc.Name,
					Port: 8080,
				}},
			}},
		},
	}
	rh.OnAdd(p1)

	// check timeout policy with malformed response timeout is propogated as infinity
	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("test2.test.com",
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/"),
						Action: withIdleTimeout(routeCluster("default/kuard/8080/da39a3ee5e"), 0), // zero means infinity
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	p2 := &projcontour.HTTPProxy{
		ObjectMeta: p1.ObjectMeta,
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{Fqdn: "test2.test.com"},
			Routes: []projcontour.Route{{
				Conditions: matchconditions(prefixMatchCondition("/")),
				TimeoutPolicy: &projcontour.TimeoutPolicy{
					Idle: "3m",
				},
				Services: []projcontour.Service{{
					Name: svc.Name,
					Port: 8080,
				}},
			}},
		},
	}
	rh.OnUpdate(p1, p2)

	// check timeout policy with response timeout is propogated correctly
	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("test2.test.com",
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/"),
						Action: withIdleTimeout(routeCluster("default/kuard/8080/da39a3ee5e"), 180*time.Second),
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	p3 := &projcontour.HTTPProxy{
		ObjectMeta: p2.ObjectMeta,
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{Fqdn: "test2.test.com"},
			Routes: []projcontour.Route{{
				Conditions: matchconditions(prefixMatchCondition("/")),
				TimeoutPolicy: &projcontour.TimeoutPolicy{
					Idle: "infinty",
				},
				Services: []projcontour.Service{{
					Name: svc.Name,
					Port: 8080,
				}},
			}},
		},
	}
	rh.OnUpdate(p2, p3)

	// check timeout policy with explicit infine response timeout is propogated as infinity
	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("test2.test.com",
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/"),
						Action: withIdleTimeout(routeCluster("default/kuard/8080/da39a3ee5e"), 0), // zero means infinity
					},
				),
			),
		),
		TypeUrl: routeType,
	})

}
