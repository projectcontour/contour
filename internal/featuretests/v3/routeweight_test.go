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
	"time"

	envoy_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_tcp_proxy_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	envoy_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/dag"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/fixture"
	"github.com/projectcontour/contour/internal/protobuf"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
	gatewayapi_v1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

type weightedcluster struct {
	name   string
	weight uint32
}

func TestHTTPProxy_RouteWithAServiceWeight(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	rh.OnAdd(fixture.NewService("kuard").
		WithPorts(v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)}))

	proxy1 := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{Fqdn: "test2.test.com"},
			Routes: []contour_api_v1.Route{{
				Conditions: conditions(prefixCondition("/a")),
				Services: []contour_api_v1.Service{{
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
			&envoy_route_v3.Route{
				Match:  routePrefix("/a"),
				Action: routecluster("default/kuard/80/da39a3ee5e"),
			},
		),
	), nil)

	proxy2 := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{Fqdn: "test2.test.com"},
			Routes: []contour_api_v1.Route{{
				Conditions: conditions(prefixCondition("/a")),
				Services: []contour_api_v1.Service{{
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
			&envoy_route_v3.Route{
				Match: routePrefix("/a"),
				Action: routeweightedcluster(
					weightedcluster{"default/kuard/80/da39a3ee5e", 60},
					weightedcluster{"default/kuard/80/da39a3ee5e", 90}),
			},
		),
	), nil)
}

func TestHTTPRoute_RouteWithAServiceWeight(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	rh.OnAdd(fixture.NewService("svc1").
		WithPorts(v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)}))

	rh.OnAdd(fixture.NewService("svc2").
		WithPorts(v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)}))

	rh.OnAdd(&gatewayapi_v1alpha1.GatewayClass{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-gc",
		},
		Spec: gatewayapi_v1alpha1.GatewayClassSpec{
			Controller: "projectcontour.io/contour",
		},
		Status: gatewayapi_v1alpha1.GatewayClassStatus{
			Conditions: []metav1.Condition{
				{
					Type:   string(gatewayapi_v1alpha1.GatewayClassConditionStatusAdmitted),
					Status: metav1.ConditionTrue,
				},
			},
		},
	})

	rh.OnAdd(&gatewayapi_v1alpha1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "contour",
			Namespace: "projectcontour",
		},
		Spec: gatewayapi_v1alpha1.GatewaySpec{
			Listeners: []gatewayapi_v1alpha1.Listener{{
				Port:     80,
				Protocol: "HTTP",
				Routes: gatewayapi_v1alpha1.RouteBindingSelector{
					Namespaces: &gatewayapi_v1alpha1.RouteNamespaces{
						From: routeSelectTypePtr(gatewayapi_v1alpha1.RouteSelectAll),
					},
					Kind: dag.KindHTTPRoute,
				},
			}},
		},
	})

	// HTTPRoute with a single weight.
	route1 := &gatewayapi_v1alpha1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "basic",
			Namespace: "default",
			Labels: map[string]string{
				"app":  "contour",
				"type": "controller",
			},
		},
		Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
			Gateways: &gatewayapi_v1alpha1.RouteGateways{
				Allow: gatewayAllowTypePtr(gatewayapi_v1alpha1.GatewayAllowAll),
			},
			Hostnames: []gatewayapi_v1alpha1.Hostname{
				"test.projectcontour.io",
			},
			Rules: []gatewayapi_v1alpha1.HTTPRouteRule{{
				Matches: []gatewayapi_v1alpha1.HTTPRouteMatch{{
					Path: &gatewayapi_v1alpha1.HTTPPathMatch{
						Type:  pathMatchTypePtr(gatewayapi_v1alpha1.PathMatchPrefix),
						Value: pointer.StringPtr("/blog"),
					},
				}},
				ForwardTo: []gatewayapi_v1alpha1.HTTPRouteForwardTo{{
					ServiceName: pointer.StringPtr("svc1"),
					Port:        gatewayPort(80),
					Weight:      pointer.Int32Ptr(1),
				}},
			}},
		},
	}

	rh.OnAdd(route1)

	assertRDS(t, c, "1", virtualhosts(
		envoy_v3.VirtualHost("test.projectcontour.io",
			&envoy_route_v3.Route{
				Match:  routePrefix("/blog"),
				Action: routecluster("default/svc1/80/da39a3ee5e"),
			},
		),
	), nil)

	// HTTPRoute with multiple weights.
	route2 := &gatewayapi_v1alpha1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "basic",
			Namespace: "default",
			Labels: map[string]string{
				"app":  "contour",
				"type": "controller",
			},
		},
		Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
			Gateways: &gatewayapi_v1alpha1.RouteGateways{
				Allow: gatewayAllowTypePtr(gatewayapi_v1alpha1.GatewayAllowAll),
			},
			Hostnames: []gatewayapi_v1alpha1.Hostname{
				"test.projectcontour.io",
			},
			Rules: []gatewayapi_v1alpha1.HTTPRouteRule{{
				Matches: []gatewayapi_v1alpha1.HTTPRouteMatch{{
					Path: &gatewayapi_v1alpha1.HTTPPathMatch{
						Type:  pathMatchTypePtr(gatewayapi_v1alpha1.PathMatchPrefix),
						Value: pointer.StringPtr("/blog"),
					},
				}},
				ForwardTo: []gatewayapi_v1alpha1.HTTPRouteForwardTo{{
					ServiceName: pointer.StringPtr("svc1"),
					Port:        gatewayPort(80),
					Weight:      pointer.Int32Ptr(60),
				}, {
					ServiceName: pointer.StringPtr("svc2"),
					Port:        gatewayPort(80),
					Weight:      pointer.Int32Ptr(90),
				}},
			}},
		},
	}

	rh.OnUpdate(route1, route2)
	assertRDS(t, c, "2", virtualhosts(
		envoy_v3.VirtualHost("test.projectcontour.io",
			&envoy_route_v3.Route{
				Match: routePrefix("/blog"),
				Action: routeweightedcluster(
					weightedcluster{"default/svc1/80/da39a3ee5e", 60},
					weightedcluster{"default/svc2/80/da39a3ee5e", 90}),
			},
		),
	), nil)
}

func TestTLSRoute_RouteWithAServiceWeight(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	rh.OnAdd(fixture.NewService("svc1").
		WithPorts(v1.ServicePort{Port: 443, TargetPort: intstr.FromInt(8443)}))

	rh.OnAdd(fixture.NewService("svc2").
		WithPorts(v1.ServicePort{Port: 443, TargetPort: intstr.FromInt(8443)}))

	rh.OnAdd(&gatewayapi_v1alpha1.GatewayClass{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-gc",
		},
		Spec: gatewayapi_v1alpha1.GatewayClassSpec{
			Controller: "projectcontour.io/contour",
		},
		Status: gatewayapi_v1alpha1.GatewayClassStatus{
			Conditions: []metav1.Condition{
				{
					Type:   string(gatewayapi_v1alpha1.GatewayClassConditionStatusAdmitted),
					Status: metav1.ConditionTrue,
				},
			},
		},
	})

	tlsMode := gatewayapi_v1alpha1.TLSModePassthrough

	rh.OnAdd(&gatewayapi_v1alpha1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "contour",
			Namespace: "projectcontour",
		},
		Spec: gatewayapi_v1alpha1.GatewaySpec{
			Listeners: []gatewayapi_v1alpha1.Listener{{
				Port:     443,
				Protocol: "TLS",
				TLS: &gatewayapi_v1alpha1.GatewayTLSConfig{
					Mode: &tlsMode,
				},
				Routes: gatewayapi_v1alpha1.RouteBindingSelector{
					Namespaces: &gatewayapi_v1alpha1.RouteNamespaces{
						From: routeSelectTypePtr(gatewayapi_v1alpha1.RouteSelectAll),
					},
					Kind: dag.KindTLSRoute,
				},
			}},
		},
	})

	// TLSRoute with a single service/weight.
	route1 := &gatewayapi_v1alpha1.TLSRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "basic",
			Namespace: "default",
			Labels: map[string]string{
				"app":  "contour",
				"type": "controller",
			},
		},
		Spec: gatewayapi_v1alpha1.TLSRouteSpec{
			Gateways: &gatewayapi_v1alpha1.RouteGateways{
				Allow: gatewayAllowTypePtr(gatewayapi_v1alpha1.GatewayAllowAll),
			},
			Rules: []gatewayapi_v1alpha1.TLSRouteRule{{
				Matches: []gatewayapi_v1alpha1.TLSRouteMatch{
					{
						SNIs: []gatewayapi_v1alpha1.Hostname{"test.projectcontour.io"},
					},
				},
				ForwardTo: []gatewayapi_v1alpha1.RouteForwardTo{{
					ServiceName: pointer.StringPtr("svc1"),
					Port:        gatewayPort(443),
					Weight:      pointer.Int32Ptr(1),
				}},
			}},
		},
	}

	rh.OnAdd(route1)

	c.Request(listenerType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			&envoy_listener_v3.Listener{
				Name:    "ingress_https",
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_listener_v3.FilterChain{{
					Filters: envoy_v3.Filters(
						tcpproxy("ingress_https", "default/svc1/443/da39a3ee5e"),
					),
					FilterChainMatch: &envoy_listener_v3.FilterChainMatch{
						ServerNames: []string{"test.projectcontour.io"},
					},
				}},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			},
			statsListener(),
		),
		TypeUrl: listenerType,
	})

	// check that ingress_http is empty
	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http"),
		),
		TypeUrl: routeType,
	})

	// TLSRoute with multiple weighted services.
	route2 := &gatewayapi_v1alpha1.TLSRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "basic",
			Namespace: "default",
			Labels: map[string]string{
				"app":  "contour",
				"type": "controller",
			},
		},
		Spec: gatewayapi_v1alpha1.TLSRouteSpec{
			Gateways: &gatewayapi_v1alpha1.RouteGateways{
				Allow: gatewayAllowTypePtr(gatewayapi_v1alpha1.GatewayAllowAll),
			},
			Rules: []gatewayapi_v1alpha1.TLSRouteRule{{
				Matches: []gatewayapi_v1alpha1.TLSRouteMatch{
					{
						SNIs: []gatewayapi_v1alpha1.Hostname{"test.projectcontour.io"},
					},
				},
				ForwardTo: []gatewayapi_v1alpha1.RouteForwardTo{
					{
						ServiceName: pointer.StringPtr("svc1"),
						Port:        gatewayPort(443),
						Weight:      pointer.Int32Ptr(1),
					},
					{
						ServiceName: pointer.StringPtr("svc2"),
						Port:        gatewayPort(443),
						Weight:      pointer.Int32Ptr(7),
					},
				},
			}},
		},
	}

	rh.OnUpdate(route1, route2)

	c.Request(listenerType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			&envoy_listener_v3.Listener{
				Name:    "ingress_https",
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_listener_v3.FilterChain{{
					Filters: envoy_v3.Filters(
						&envoy_listener_v3.Filter{
							Name: wellknown.TCPProxy,
							ConfigType: &envoy_listener_v3.Filter_TypedConfig{
								TypedConfig: protobuf.MustMarshalAny(&envoy_tcp_proxy_v3.TcpProxy{
									StatPrefix: "ingress_https",
									ClusterSpecifier: &envoy_tcp_proxy_v3.TcpProxy_WeightedClusters{
										WeightedClusters: &envoy_tcp_proxy_v3.TcpProxy_WeightedCluster{
											Clusters: []*envoy_tcp_proxy_v3.TcpProxy_WeightedCluster_ClusterWeight{
												{Name: "default/svc1/443/da39a3ee5e", Weight: 1},
												{Name: "default/svc2/443/da39a3ee5e", Weight: 7},
											},
										},
									},
									AccessLog:   envoy_v3.FileAccessLogEnvoy("/dev/stdout", "", nil),
									IdleTimeout: protobuf.Duration(9001 * time.Second),
								}),
							},
						},
					),
					FilterChainMatch: &envoy_listener_v3.FilterChainMatch{
						ServerNames: []string{"test.projectcontour.io"},
					},
				}},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			},
			statsListener(),
		),
		TypeUrl: listenerType,
	})

	// check that ingress_http is empty
	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http"),
		),
		TypeUrl: routeType,
	})
}

func routeweightedcluster(clusters ...weightedcluster) *envoy_route_v3.Route_Route {
	return &envoy_route_v3.Route_Route{
		Route: &envoy_route_v3.RouteAction{
			ClusterSpecifier: &envoy_route_v3.RouteAction_WeightedClusters{
				WeightedClusters: weightedclusters(clusters),
			},
		},
	}
}

func weightedclusters(clusters []weightedcluster) *envoy_route_v3.WeightedCluster {
	var wc envoy_route_v3.WeightedCluster
	var total uint32
	for _, c := range clusters {
		total += c.weight
		wc.Clusters = append(wc.Clusters, &envoy_route_v3.WeightedCluster_ClusterWeight{
			Name:   c.name,
			Weight: protobuf.UInt32(c.weight),
		})
	}
	wc.TotalWeight = protobuf.UInt32(total)
	return &wc
}
