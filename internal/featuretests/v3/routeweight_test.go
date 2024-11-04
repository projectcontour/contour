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

// End to ends tests for translator to grpc operations.
package v3

import (
	"testing"

	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"github.com/stretchr/testify/require"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	gatewayapi_v1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/fixture"
	"github.com/projectcontour/contour/internal/gatewayapi"
)

func TestHTTPProxy_RouteWithAServiceWeight(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	rh.OnAdd(fixture.NewService("kuard").
		WithPorts(core_v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)}))

	proxy1 := &contour_v1.HTTPProxy{
		ObjectMeta: fixture.ObjectMeta("simple"),
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{Fqdn: "test2.test.com"},
			Routes: []contour_v1.Route{{
				Conditions: conditions(prefixCondition("/a")),
				Services: []contour_v1.Service{{
					Name:   "kuard",
					Port:   80,
					Weight: 90, // ignored
				}},
			}},
		},
	}

	rh.OnAdd(proxy1)
	assertRDS(t, c, "1", virtualhosts(
		envoy_v3.VirtualHost("test2.test.com",
			&envoy_config_route_v3.Route{
				Match:  routePrefix("/a"),
				Action: routecluster("default/kuard/80/da39a3ee5e"),
			},
		),
	), nil)

	proxy2 := &contour_v1.HTTPProxy{
		ObjectMeta: fixture.ObjectMeta("simple"),
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{Fqdn: "test2.test.com"},
			Routes: []contour_v1.Route{{
				Conditions: conditions(prefixCondition("/a")),
				Services: []contour_v1.Service{{
					Name:   "kuard",
					Port:   80,
					Weight: 90,
				}, {
					Name:   "kuard",
					Port:   80,
					Weight: 60,
				}},
			}},
		},
	}

	rh.OnUpdate(proxy1, proxy2)
	assertRDS(t, c, "2", virtualhosts(
		envoy_v3.VirtualHost("test2.test.com",
			&envoy_config_route_v3.Route{
				Match: routePrefix("/a"),
				Action: routeWeightedCluster(
					weightedCluster{"default/kuard/80/da39a3ee5e", 60},
					weightedCluster{"default/kuard/80/da39a3ee5e", 90}),
			},
		),
	), nil)
}

func TestHTTPProxy_TCPProxyWithAServiceWeight(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	rh.OnAdd(fixture.NewService("kuard-1").WithPorts(core_v1.ServicePort{Port: 443, TargetPort: intstr.FromInt(8443)}))
	rh.OnAdd(fixture.NewService("kuard-2").WithPorts(core_v1.ServicePort{Port: 443, TargetPort: intstr.FromInt(8443)}))
	rh.OnAdd(fixture.NewService("kuard-3").WithPorts(core_v1.ServicePort{Port: 443, TargetPort: intstr.FromInt(8443)}))

	// proxy1 has a TCPProxy with a single service.
	proxy1 := &contour_v1.HTTPProxy{
		ObjectMeta: fixture.ObjectMeta("simple"),
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "tcpproxy.test.com",
				TLS: &contour_v1.TLS{
					Passthrough: true,
				},
			},
			TCPProxy: &contour_v1.TCPProxy{
				Services: []contour_v1.Service{
					{
						Name:   "kuard-1",
						Port:   443,
						Weight: 70, // ignored
					},
				},
			},
		},
	}

	rh.OnAdd(proxy1)

	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			&envoy_config_listener_v3.Listener{
				Name:    "ingress_https",
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_config_listener_v3.FilterChain{{
					Filters: envoy_v3.Filters(
						tcpproxy("ingress_https", "default/kuard-1/443/da39a3ee5e"),
					),
					FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
						ServerNames: []string{"tcpproxy.test.com"},
					},
				}},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			},
			statsListener(),
		),
		TypeUrl: listenerType,
	})

	// check that ingress_http is empty
	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http"),
		),
		TypeUrl: routeType,
	})

	// proxy2 has a TCPProxy with multiple services,
	// each with an explicit weight.
	proxy2 := &contour_v1.HTTPProxy{
		ObjectMeta: fixture.ObjectMeta("simple"),
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "tcpproxy.test.com",
				TLS: &contour_v1.TLS{
					Passthrough: true,
				},
			},
			TCPProxy: &contour_v1.TCPProxy{
				Services: []contour_v1.Service{
					{Name: "kuard-1", Port: 443, Weight: 7},
					{Name: "kuard-2", Port: 443, Weight: 77},
				},
			},
		},
	}
	rh.OnUpdate(proxy1, proxy2)

	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			&envoy_config_listener_v3.Listener{
				Name:    "ingress_https",
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_config_listener_v3.FilterChain{{
					Filters: envoy_v3.Filters(
						tcpproxyWeighted(
							"ingress_https",
							clusterWeight{name: "default/kuard-1/443/da39a3ee5e", weight: 7},
							clusterWeight{name: "default/kuard-2/443/da39a3ee5e", weight: 77},
						),
					),
					FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
						ServerNames: []string{"tcpproxy.test.com"},
					},
				}},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			},
			statsListener(),
		),
		TypeUrl: listenerType,
	})

	// check that ingress_http is empty
	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http"),
		),
		TypeUrl: routeType,
	})

	// proxy3 has a TCPProxy with multiple services,
	// each with no weight specified.
	proxy3 := &contour_v1.HTTPProxy{
		ObjectMeta: fixture.ObjectMeta("simple"),
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "tcpproxy.test.com",
				TLS: &contour_v1.TLS{
					Passthrough: true,
				},
			},
			TCPProxy: &contour_v1.TCPProxy{
				Services: []contour_v1.Service{
					{Name: "kuard-1", Port: 443},
					{Name: "kuard-2", Port: 443},
				},
			},
		},
	}
	rh.OnUpdate(proxy2, proxy3)

	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			&envoy_config_listener_v3.Listener{
				Name:    "ingress_https",
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_config_listener_v3.FilterChain{{
					Filters: envoy_v3.Filters(
						tcpproxyWeighted(
							"ingress_https",
							clusterWeight{name: "default/kuard-1/443/da39a3ee5e", weight: 1},
							clusterWeight{name: "default/kuard-2/443/da39a3ee5e", weight: 1},
						),
					),
					FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
						ServerNames: []string{"tcpproxy.test.com"},
					},
				}},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			},
			statsListener(),
		),
		TypeUrl: listenerType,
	})

	// check that ingress_http is empty
	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http"),
		),
		TypeUrl: routeType,
	})

	// proxy4 has a TCPProxy with multiple services,
	// some with weights specified and some without.
	proxy4 := &contour_v1.HTTPProxy{
		ObjectMeta: fixture.ObjectMeta("simple"),
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "tcpproxy.test.com",
				TLS: &contour_v1.TLS{
					Passthrough: true,
				},
			},
			TCPProxy: &contour_v1.TCPProxy{
				Services: []contour_v1.Service{
					{Name: "kuard-1", Port: 443, Weight: 77},
					{Name: "kuard-2", Port: 443},
					{Name: "kuard-3", Port: 443, Weight: 7},
				},
			},
		},
	}
	rh.OnUpdate(proxy3, proxy4)

	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			&envoy_config_listener_v3.Listener{
				Name:    "ingress_https",
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_config_listener_v3.FilterChain{{
					Filters: envoy_v3.Filters(
						tcpproxyWeighted(
							"ingress_https",
							clusterWeight{name: "default/kuard-1/443/da39a3ee5e", weight: 77},
							clusterWeight{name: "default/kuard-3/443/da39a3ee5e", weight: 7},
						),
					),
					FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
						ServerNames: []string{"tcpproxy.test.com"},
					},
				}},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			},
			statsListener(),
		),
		TypeUrl: listenerType,
	})

	// check that ingress_http is empty
	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http"),
		),
		TypeUrl: routeType,
	})
}

func TestHTTPRoute_RouteWithAServiceWeight(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	rh.OnAdd(fixture.NewService("svc1").
		WithPorts(core_v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)}))

	rh.OnAdd(fixture.NewService("svc2").
		WithPorts(core_v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)}))

	rh.OnAdd(&gatewayapi_v1.GatewayClass{
		TypeMeta:   meta_v1.TypeMeta{},
		ObjectMeta: fixture.ObjectMeta("test-gc"),
		Spec: gatewayapi_v1.GatewayClassSpec{
			ControllerName: "projectcontour.io/contour",
		},
		Status: gatewayapi_v1.GatewayClassStatus{
			Conditions: []meta_v1.Condition{
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusAccepted),
					Status: meta_v1.ConditionTrue,
				},
			},
		},
	})

	rh.OnAdd(&gatewayapi_v1.Gateway{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "contour",
			Namespace: "projectcontour",
		},
		Spec: gatewayapi_v1.GatewaySpec{
			Listeners: []gatewayapi_v1.Listener{{
				Port:     80,
				Protocol: gatewayapi_v1.HTTPProtocolType,
				AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
					Namespaces: &gatewayapi_v1.RouteNamespaces{
						From: ptr.To(gatewayapi_v1.NamespacesFromAll),
					},
				},
			}},
		},
	})

	// HTTPRoute with a single weight.
	route1 := &gatewayapi_v1.HTTPRoute{
		ObjectMeta: fixture.ObjectMetaWithAnnotations("basic", map[string]string{
			"app":  "contour",
			"type": "controller",
		}),
		Spec: gatewayapi_v1.HTTPRouteSpec{
			CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
				ParentRefs: []gatewayapi_v1.ParentReference{
					gatewayapi.GatewayParentRef("projectcontour", "contour"),
				},
			},
			Hostnames: []gatewayapi_v1.Hostname{
				"test.projectcontour.io",
			},
			Rules: []gatewayapi_v1.HTTPRouteRule{{
				Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/blog"),
				BackendRefs: gatewayapi.HTTPBackendRef("svc1", 80, 1),
			}},
		},
	}

	rh.OnAdd(route1)

	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t, envoy_v3.RouteConfiguration("http-80", envoy_v3.VirtualHost("test.projectcontour.io",
			&envoy_config_route_v3.Route{
				Match:  routeSegmentPrefix("/blog"),
				Action: routecluster("default/svc1/80/da39a3ee5e"),
			},
		))),
	})

	// HTTPRoute with multiple weights.
	route2 := &gatewayapi_v1.HTTPRoute{
		ObjectMeta: fixture.ObjectMetaWithAnnotations("basic", map[string]string{
			"app":  "contour",
			"type": "controller",
		}),
		Spec: gatewayapi_v1.HTTPRouteSpec{
			CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
				ParentRefs: []gatewayapi_v1.ParentReference{
					gatewayapi.GatewayParentRef("projectcontour", "contour"),
				},
			},
			Hostnames: []gatewayapi_v1.Hostname{
				"test.projectcontour.io",
			},
			Rules: []gatewayapi_v1.HTTPRouteRule{{
				Matches: gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/blog"),
				BackendRefs: gatewayapi.HTTPBackendRefs(
					gatewayapi.HTTPBackendRef("svc1", 80, 60),
					gatewayapi.HTTPBackendRef("svc2", 80, 90),
				),
			}},
		},
	}

	rh.OnUpdate(route1, route2)

	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t, envoy_v3.RouteConfiguration("http-80", envoy_v3.VirtualHost("test.projectcontour.io",
			&envoy_config_route_v3.Route{
				Match: routeSegmentPrefix("/blog"),
				Action: routeWeightedCluster(
					weightedCluster{"default/svc1/80/da39a3ee5e", 60},
					weightedCluster{"default/svc2/80/da39a3ee5e", 90},
				),
			},
		))),
	})
}

func TestTLSRoute_RouteWithAServiceWeight(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	rh.OnAdd(fixture.NewService("svc1").
		WithPorts(core_v1.ServicePort{Port: 443, TargetPort: intstr.FromInt(8443)}))

	rh.OnAdd(fixture.NewService("svc2").
		WithPorts(core_v1.ServicePort{Port: 443, TargetPort: intstr.FromInt(8443)}))

	rh.OnAdd(&gatewayapi_v1.GatewayClass{
		TypeMeta:   meta_v1.TypeMeta{},
		ObjectMeta: fixture.ObjectMeta("test-gc"),
		Spec: gatewayapi_v1.GatewayClassSpec{
			ControllerName: "projectcontour.io/contour",
		},
		Status: gatewayapi_v1.GatewayClassStatus{
			Conditions: []meta_v1.Condition{
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusAccepted),
					Status: meta_v1.ConditionTrue,
				},
			},
		},
	})

	rh.OnAdd(&gatewayapi_v1.Gateway{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "contour",
			Namespace: "projectcontour",
		},
		Spec: gatewayapi_v1.GatewaySpec{
			Listeners: []gatewayapi_v1.Listener{{
				Port:     443,
				Protocol: gatewayapi_v1.TLSProtocolType,
				TLS: &gatewayapi_v1.GatewayTLSConfig{
					Mode: ptr.To(gatewayapi_v1.TLSModePassthrough),
				},
				AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
					Namespaces: &gatewayapi_v1.RouteNamespaces{
						From: ptr.To(gatewayapi_v1.NamespacesFromAll),
					},
				},
			}},
		},
	})

	// TLSRoute with a single service/weight.
	route1 := &gatewayapi_v1alpha2.TLSRoute{
		ObjectMeta: fixture.ObjectMetaWithAnnotations("basic", map[string]string{
			"app":  "contour",
			"type": "controller",
		}),
		Spec: gatewayapi_v1alpha2.TLSRouteSpec{
			CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
				ParentRefs: []gatewayapi_v1.ParentReference{
					gatewayapi.GatewayParentRef("projectcontour", "contour"),
				},
			},
			Hostnames: []gatewayapi_v1.Hostname{"test.projectcontour.io"},
			Rules: []gatewayapi_v1alpha2.TLSRouteRule{{
				BackendRefs: gatewayapi.TLSRouteBackendRef("svc1", 443, ptr.To(int32(1))),
			}},
		},
	}

	rh.OnAdd(route1)

	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			&envoy_config_listener_v3.Listener{
				Name:    "https-443",
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_config_listener_v3.FilterChain{{
					Filters: envoy_v3.Filters(
						tcpproxy("https-443", "default/svc1/443/da39a3ee5e"),
					),
					FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
						ServerNames: []string{"test.projectcontour.io"},
					},
				}},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			},
			statsListener(),
		),
		TypeUrl: listenerType,
	})

	// check that there is no route config
	require.Empty(t, c.Request(routeType).Resources)

	// TLSRoute with multiple weighted services.
	route2 := &gatewayapi_v1alpha2.TLSRoute{
		ObjectMeta: fixture.ObjectMetaWithAnnotations("basic", map[string]string{
			"app":  "contour",
			"type": "controller",
		}),
		Spec: gatewayapi_v1alpha2.TLSRouteSpec{
			CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
				ParentRefs: []gatewayapi_v1.ParentReference{
					gatewayapi.GatewayParentRef("projectcontour", "contour"),
				},
			},
			Hostnames: []gatewayapi_v1.Hostname{"test.projectcontour.io"},
			Rules: []gatewayapi_v1alpha2.TLSRouteRule{{
				BackendRefs: gatewayapi.TLSRouteBackendRefs(
					gatewayapi.TLSRouteBackendRef("svc1", 443, ptr.To(int32(1))),
					gatewayapi.TLSRouteBackendRef("svc2", 443, ptr.To(int32(7))),
				),
			}},
		},
	}

	rh.OnUpdate(route1, route2)

	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			&envoy_config_listener_v3.Listener{
				Name:    "https-443",
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_config_listener_v3.FilterChain{{
					Filters: envoy_v3.Filters(
						tcpproxyWeighted(
							"https-443",
							clusterWeight{name: "default/svc1/443/da39a3ee5e", weight: 1},
							clusterWeight{name: "default/svc2/443/da39a3ee5e", weight: 7},
						),
					),
					FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
						ServerNames: []string{"test.projectcontour.io"},
					},
				}},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			},
			statsListener(),
		),
		TypeUrl: listenerType,
	})

	// check that there is no route config
	require.Empty(t, c.Request(routeType).Resources)
}
