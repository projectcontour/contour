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
	projcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/envoy"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestConditions_ContainsHeader_HTTProxy(t *testing.T) {
	rh, c, done := setup(t)
	defer done()
	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "svc1",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	})

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "svc2",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	})

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "svc3",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	})

	proxy1 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{Fqdn: "hello.world"},
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: "svc1",
					Port: 80,
				}},
			}, {
				Conditions: conditions(
					prefixCondition("/"),
					headerContainsCondition("x-header", "abc"),
				),
				Services: []projcontour.Service{{
					Name: "svc2",
					Port: 80,
				}},
			}, {
				Conditions: conditions(
					prefixCondition("/blog"),
					headerContainsCondition("x-header", "abc"),
				),
				Services: []projcontour.Service{{
					Name: "svc3",
					Port: 80,
				}},
			}},
		},
	}
	rh.OnAdd(proxy1)

	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("hello.world",
					&envoy_api_v2_route.Route{
						Match: routePrefix("/blog", dag.HeaderCondition{
							Name:      "x-header",
							Value:     "abc",
							MatchType: "contains",
							Invert:    false,
						}),
						Action: routeCluster("default/svc3/80/da39a3ee5e"),
					},
					&envoy_api_v2_route.Route{
						Match: routePrefix("/", dag.HeaderCondition{
							Name:      "x-header",
							Value:     "abc",
							MatchType: "contains",
							Invert:    false,
						}),
						Action: routeCluster("default/svc2/80/da39a3ee5e"),
					},
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/"),
						Action: routeCluster("default/svc1/80/da39a3ee5e"),
					},
				),
			),
			envoy.RouteConfiguration("ingress_https"),
		),
		TypeUrl: routeType,
	})

	proxy2 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{Fqdn: "hello.world"},
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: "svc1",
					Port: 80,
				}},
			}, {
				Conditions: conditions(
					prefixCondition("/"),
					headerNotContainsCondition("x-header", "123"),
				),
				Services: []projcontour.Service{{
					Name: "svc2",
					Port: 80,
				}},
			}, {
				Conditions: conditions(
					prefixCondition("/blog"),
					headerNotContainsCondition("x-header", "abc"),
				),
				Services: []projcontour.Service{{
					Name: "svc3",
					Port: 80,
				}},
			}},
		},
	}

	rh.OnUpdate(proxy1, proxy2)

	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("hello.world",
					&envoy_api_v2_route.Route{
						Match: routePrefix("/blog", dag.HeaderCondition{
							Name:      "x-header",
							Value:     "abc",
							MatchType: "contains",
							Invert:    true,
						}),
						Action: routeCluster("default/svc3/80/da39a3ee5e"),
					},
					&envoy_api_v2_route.Route{
						Match: routePrefix("/", dag.HeaderCondition{
							Name:      "x-header",
							Value:     "123",
							MatchType: "contains",
							Invert:    true,
						}),
						Action: routeCluster("default/svc2/80/da39a3ee5e"),
					},
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/"),
						Action: routeCluster("default/svc1/80/da39a3ee5e"),
					},
				),
			),
			envoy.RouteConfiguration("ingress_https"),
		),
		TypeUrl: routeType,
	})

	proxy3 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{Fqdn: "hello.world"},
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: "svc1",
					Port: 80,
				}},
			}, {
				Conditions: conditions(
					prefixCondition("/"),
					headerExactCondition("x-header", "abc"),
				),
				Services: []projcontour.Service{{
					Name: "svc2",
					Port: 80,
				}},
			}, {
				Conditions: conditions(
					prefixCondition("/blog"),
					headerExactCondition("x-header", "123"),
				),
				Services: []projcontour.Service{{
					Name: "svc3",
					Port: 80,
				}},
			}},
		},
	}

	rh.OnUpdate(proxy2, proxy3)

	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("hello.world",
					&envoy_api_v2_route.Route{
						Match: routePrefix("/blog", dag.HeaderCondition{
							Name:      "x-header",
							Value:     "123",
							MatchType: "exact",
							Invert:    false,
						}),
						Action: routeCluster("default/svc3/80/da39a3ee5e"),
					},
					&envoy_api_v2_route.Route{
						Match: routePrefix("/", dag.HeaderCondition{
							Name:      "x-header",
							Value:     "abc",
							MatchType: "exact",
							Invert:    false,
						}),
						Action: routeCluster("default/svc2/80/da39a3ee5e"),
					},
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/"),
						Action: routeCluster("default/svc1/80/da39a3ee5e"),
					},
				),
			),
			envoy.RouteConfiguration("ingress_https"),
		),
		TypeUrl: routeType,
	})

	proxy4 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{Fqdn: "hello.world"},
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: "svc1",
					Port: 80,
				}},
			}, {
				Conditions: conditions(
					prefixCondition("/"),
					headerNotExactCondition("x-header", "abc"),
				),
				Services: []projcontour.Service{{
					Name: "svc2",
					Port: 80,
				}},
			}, {
				Conditions: conditions(
					prefixCondition("/blog"),
					headerNotExactCondition("x-header", "123"),
				),
				Services: []projcontour.Service{{
					Name: "svc3",
					Port: 80,
				}},
			}},
		},
	}

	rh.OnUpdate(proxy3, proxy4)

	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("hello.world",
					&envoy_api_v2_route.Route{
						Match: routePrefix("/blog", dag.HeaderCondition{
							Name:      "x-header",
							Value:     "123",
							MatchType: "exact",
							Invert:    true,
						}),
						Action: routeCluster("default/svc3/80/da39a3ee5e"),
					},
					&envoy_api_v2_route.Route{
						Match: routePrefix("/", dag.HeaderCondition{
							Name:      "x-header",
							Value:     "abc",
							MatchType: "exact",
							Invert:    true,
						}),
						Action: routeCluster("default/svc2/80/da39a3ee5e"),
					},
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/"),
						Action: routeCluster("default/svc1/80/da39a3ee5e"),
					},
				),
			),
			envoy.RouteConfiguration("ingress_https"),
		),
		TypeUrl: routeType,
	})

	proxy5 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{Fqdn: "hello.world"},
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: "svc1",
					Port: 80,
				}},
			}, {
				Conditions: conditions(
					prefixCondition("/"),
					headerPresentCondition("x-header"),
				),
				Services: []projcontour.Service{{
					Name: "svc2",
					Port: 80,
				}},
			}, {
				Conditions: conditions(
					prefixCondition("/blog"),
					headerPresentCondition("x-header"),
				),
				Services: []projcontour.Service{{
					Name: "svc3",
					Port: 80,
				}},
			}},
		},
	}

	rh.OnUpdate(proxy4, proxy5)

	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("hello.world",
					&envoy_api_v2_route.Route{
						Match: routePrefix("/blog", dag.HeaderCondition{
							Name:      "x-header",
							MatchType: "present",
							Invert:    false,
						}),
						Action: routeCluster("default/svc3/80/da39a3ee5e"),
					},
					&envoy_api_v2_route.Route{
						Match: routePrefix("/", dag.HeaderCondition{
							Name:      "x-header",
							MatchType: "present",
							Invert:    false,
						}),
						Action: routeCluster("default/svc2/80/da39a3ee5e"),
					},
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/"),
						Action: routeCluster("default/svc1/80/da39a3ee5e"),
					},
				),
			),
			envoy.RouteConfiguration("ingress_https"),
		),
		TypeUrl: routeType,
	})
}
