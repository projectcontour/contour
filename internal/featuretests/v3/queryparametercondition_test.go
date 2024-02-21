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
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/dag"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/fixture"
)

func TestConditions_ContainsQueryParameter_HTTProxy(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	rh.OnAdd(fixture.NewService("svc1").
		WithPorts(core_v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)}),
	)

	rh.OnAdd(fixture.NewService("svc2").
		WithPorts(core_v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)}),
	)

	rh.OnAdd(fixture.NewService("svc3").
		WithPorts(core_v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)}),
	)

	proxy1 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{Fqdn: "hello.world"},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: "svc1",
					Port: 80,
				}},
			}, {
				Conditions: matchconditions(
					prefixMatchCondition("/"),
					queryParameterContainsMatchCondition("query-param", "abc", false),
				),
				Services: []contour_v1.Service{{
					Name: "svc2",
					Port: 80,
				}},
			}, {
				Conditions: matchconditions(
					prefixMatchCondition("/blog"),
					queryParameterContainsMatchCondition("query-param", "abc", true),
				),
				Services: []contour_v1.Service{{
					Name: "svc3",
					Port: 80,
				}},
			}},
		},
	}
	rh.OnAdd(proxy1)

	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("hello.world",
					&envoy_config_route_v3.Route{
						Match: routePrefixWithQueryParameterConditions("/blog", dag.QueryParamMatchCondition{
							Name:       "query-param",
							Value:      "abc",
							MatchType:  "contains",
							IgnoreCase: true,
						}),
						Action: routeCluster("default/svc3/80/da39a3ee5e"),
					},
					&envoy_config_route_v3.Route{
						Match: routePrefixWithQueryParameterConditions("/", dag.QueryParamMatchCondition{
							Name:       "query-param",
							Value:      "abc",
							MatchType:  "contains",
							IgnoreCase: false,
						}),
						Action: routeCluster("default/svc2/80/da39a3ee5e"),
					},
					&envoy_config_route_v3.Route{
						Match:  routePrefixWithQueryParameterConditions("/"),
						Action: routeCluster("default/svc1/80/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	proxy2 := fixture.NewProxy("simple").WithSpec(
		contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{Fqdn: "hello.world"},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: "svc1",
					Port: 80,
				}},
			}, {
				Conditions: matchconditions(
					prefixMatchCondition("/"),
					queryParameterExactMatchCondition("query-param", "123", false),
				),
				Services: []contour_v1.Service{{
					Name: "svc2",
					Port: 80,
				}},
			}, {
				Conditions: matchconditions(
					prefixMatchCondition("/blog"),
					queryParameterExactMatchCondition("query-param", "abc", true),
				),
				Services: []contour_v1.Service{{
					Name: "svc3",
					Port: 80,
				}},
			}},
		})

	rh.OnUpdate(proxy1, proxy2)

	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("hello.world",
					&envoy_config_route_v3.Route{
						Match: routePrefixWithQueryParameterConditions("/blog", dag.QueryParamMatchCondition{
							Name:       "query-param",
							Value:      "abc",
							MatchType:  "exact",
							IgnoreCase: true,
						}),
						Action: routeCluster("default/svc3/80/da39a3ee5e"),
					},
					&envoy_config_route_v3.Route{
						Match: routePrefixWithQueryParameterConditions("/", dag.QueryParamMatchCondition{
							Name:       "query-param",
							Value:      "123",
							MatchType:  "exact",
							IgnoreCase: false,
						}),
						Action: routeCluster("default/svc2/80/da39a3ee5e"),
					},
					&envoy_config_route_v3.Route{
						Match:  routePrefixWithQueryParameterConditions("/"),
						Action: routeCluster("default/svc1/80/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	proxy3 := fixture.NewProxy("simple").WithSpec(
		contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{Fqdn: "hello.world"},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: "svc1",
					Port: 80,
				}},
			}, {
				Conditions: matchconditions(
					prefixMatchCondition("/"),
					queryParameterPrefixMatchCondition("query-param", "abc", false),
				),
				Services: []contour_v1.Service{{
					Name: "svc2",
					Port: 80,
				}},
			}, {
				Conditions: matchconditions(
					prefixMatchCondition("/blog"),
					queryParameterPrefixMatchCondition("query-param", "123", true),
				),
				Services: []contour_v1.Service{{
					Name: "svc3",
					Port: 80,
				}},
			}},
		})

	rh.OnUpdate(proxy2, proxy3)

	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("hello.world",
					&envoy_config_route_v3.Route{
						Match: routePrefixWithQueryParameterConditions("/blog", dag.QueryParamMatchCondition{
							Name:       "query-param",
							Value:      "123",
							MatchType:  "prefix",
							IgnoreCase: true,
						}),
						Action: routeCluster("default/svc3/80/da39a3ee5e"),
					},
					&envoy_config_route_v3.Route{
						Match: routePrefixWithQueryParameterConditions("/", dag.QueryParamMatchCondition{
							Name:       "query-param",
							Value:      "abc",
							MatchType:  "prefix",
							IgnoreCase: false,
						}),
						Action: routeCluster("default/svc2/80/da39a3ee5e"),
					},
					&envoy_config_route_v3.Route{
						Match:  routePrefixWithQueryParameterConditions("/"),
						Action: routeCluster("default/svc1/80/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	proxy4 := fixture.NewProxy("simple").WithSpec(
		contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{Fqdn: "hello.world"},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: "svc1",
					Port: 80,
				}},
			}, {
				Conditions: matchconditions(
					prefixMatchCondition("/"),
					queryParameterSuffixMatchCondition("query-param", "abc", false),
				),
				Services: []contour_v1.Service{{
					Name: "svc2",
					Port: 80,
				}},
			}, {
				Conditions: matchconditions(
					prefixMatchCondition("/blog"),
					queryParameterSuffixMatchCondition("query-param", "123", true),
				),
				Services: []contour_v1.Service{{
					Name: "svc3",
					Port: 80,
				}},
			}},
		})

	rh.OnUpdate(proxy3, proxy4)

	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("hello.world",
					&envoy_config_route_v3.Route{
						Match: routePrefixWithQueryParameterConditions("/blog", dag.QueryParamMatchCondition{
							Name:       "query-param",
							Value:      "123",
							MatchType:  "suffix",
							IgnoreCase: true,
						}),
						Action: routeCluster("default/svc3/80/da39a3ee5e"),
					},
					&envoy_config_route_v3.Route{
						Match: routePrefixWithQueryParameterConditions("/", dag.QueryParamMatchCondition{
							Name:       "query-param",
							Value:      "abc",
							MatchType:  "suffix",
							IgnoreCase: false,
						}),
						Action: routeCluster("default/svc2/80/da39a3ee5e"),
					},
					&envoy_config_route_v3.Route{
						Match:  routePrefixWithQueryParameterConditions("/"),
						Action: routeCluster("default/svc1/80/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	proxy5 := fixture.NewProxy("simple").WithSpec(
		contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{Fqdn: "hello.world"},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: "svc1",
					Port: 80,
				}},
			}, {
				Conditions: matchconditions(
					prefixMatchCondition("/"),
					queryParameterRegexMatchCondition("query-param", "^123.*"),
				),
				Services: []contour_v1.Service{{
					Name: "svc2",
					Port: 80,
				}},
			}, {
				Conditions: matchconditions(
					prefixMatchCondition("/blog"),
					queryParameterRegexMatchCondition("query-param", "^123.*"),
				),
				Services: []contour_v1.Service{{
					Name: "svc3",
					Port: 80,
				}},
			}},
		})

	rh.OnUpdate(proxy4, proxy5)

	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("hello.world",
					&envoy_config_route_v3.Route{
						Match: routePrefixWithQueryParameterConditions("/blog", dag.QueryParamMatchCondition{
							Name:      "query-param",
							Value:     "^123.*",
							MatchType: "regex",
						}),
						Action: routeCluster("default/svc3/80/da39a3ee5e"),
					},
					&envoy_config_route_v3.Route{
						Match: routePrefixWithQueryParameterConditions("/", dag.QueryParamMatchCondition{
							Name:      "query-param",
							Value:     "^123.*",
							MatchType: "regex",
						}),
						Action: routeCluster("default/svc2/80/da39a3ee5e"),
					},
					&envoy_config_route_v3.Route{
						Match:  routePrefixWithQueryParameterConditions("/"),
						Action: routeCluster("default/svc1/80/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	proxy6 := fixture.NewProxy("simple").WithSpec(
		contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{Fqdn: "hello.world"},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: "svc1",
					Port: 80,
				}},
			}, {
				Conditions: matchconditions(
					prefixMatchCondition("/"),
					queryParameterPresentMatchCondition("query-param"),
				),
				Services: []contour_v1.Service{{
					Name: "svc2",
					Port: 80,
				}},
			}, {
				Conditions: matchconditions(
					prefixMatchCondition("/blog"),
					queryParameterPresentMatchCondition("query-param"),
				),
				Services: []contour_v1.Service{{
					Name: "svc3",
					Port: 80,
				}},
			}},
		})

	rh.OnUpdate(proxy5, proxy6)

	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("hello.world",
					&envoy_config_route_v3.Route{
						Match: routePrefixWithQueryParameterConditions("/blog", dag.QueryParamMatchCondition{
							Name:      "query-param",
							MatchType: "present",
						}),
						Action: routeCluster("default/svc3/80/da39a3ee5e"),
					},
					&envoy_config_route_v3.Route{
						Match: routePrefixWithQueryParameterConditions("/", dag.QueryParamMatchCondition{
							Name:      "query-param",
							MatchType: "present",
						}),
						Action: routeCluster("default/svc2/80/da39a3ee5e"),
					},
					&envoy_config_route_v3.Route{
						Match:  routePrefixWithQueryParameterConditions("/"),
						Action: routeCluster("default/svc1/80/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	// proxy with two routes that have the same prefix and a Contains query parameter
	// condition on the same parameter name, differing only in the value of the condition.
	proxy7 := fixture.NewProxy("simple").WithSpec(
		contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{Fqdn: "hello.world"},
			Routes: []contour_v1.Route{
				{
					Conditions: matchconditions(
						prefixMatchCondition("/"),
						queryParameterContainsMatchCondition("query-param", "abc", false),
					),
					Services: []contour_v1.Service{{
						Name: "svc1",
						Port: 80,
					}},
				},
				{
					Conditions: matchconditions(
						prefixMatchCondition("/"),
						queryParameterContainsMatchCondition("query-param", "def", false),
					),
					Services: []contour_v1.Service{{
						Name: "svc2",
						Port: 80,
					}},
				},
			},
		},
	)

	rh.OnUpdate(proxy6, proxy7)

	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("hello.world",
					&envoy_config_route_v3.Route{
						Match: routePrefixWithQueryParameterConditions("/", dag.QueryParamMatchCondition{
							Name:      "query-param",
							Value:     "abc",
							MatchType: "contains",
						}),
						Action: routeCluster("default/svc1/80/da39a3ee5e"),
					},
					&envoy_config_route_v3.Route{
						Match: routePrefixWithQueryParameterConditions("/", dag.QueryParamMatchCondition{
							Name:      "query-param",
							Value:     "def",
							MatchType: "contains",
						}),
						Action: routeCluster("default/svc2/80/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
	})
}
