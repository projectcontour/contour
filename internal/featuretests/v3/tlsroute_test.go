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

	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"github.com/stretchr/testify/require"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	gatewayapi_v1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/featuretests"
	"github.com/projectcontour/contour/internal/fixture"
	"github.com/projectcontour/contour/internal/gatewayapi"
)

func TestTLSRoute_TLSPassthrough(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	svc := fixture.NewService("correct-backend").
		WithPorts(core_v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)})

	svcAnother := fixture.NewService("another-backend").
		WithPorts(core_v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)})

	rh.OnAdd(svc)
	rh.OnAdd(svcAnother)

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

	gatewayPassthrough := &gatewayapi_v1.Gateway{
		ObjectMeta: fixture.ObjectMeta("projectcontour/contour"),
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
	}

	rh.OnAdd(gatewayPassthrough)

	route1 := &gatewayapi_v1alpha2.TLSRoute{
		ObjectMeta: fixture.ObjectMeta("basic"),
		Spec: gatewayapi_v1alpha2.TLSRouteSpec{
			CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
				ParentRefs: []gatewayapi_v1.ParentReference{
					gatewayapi.GatewayParentRef("projectcontour", "contour"),
				},
			},
			Hostnames: []gatewayapi_v1.Hostname{"tcp.projectcontour.io"},
			Rules: []gatewayapi_v1alpha2.TLSRouteRule{{
				BackendRefs: gatewayapi.TLSRouteBackendRef("correct-backend", 80, nil),
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
						tcpproxy("https-443", "default/correct-backend/80/da39a3ee5e"),
					),
					FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
						ServerNames: []string{"tcp.projectcontour.io"},
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

	// Route2 doesn't define any SNIs, so this should become the default backend.
	route2 := &gatewayapi_v1alpha2.TLSRoute{
		ObjectMeta: fixture.ObjectMeta("basic"),
		Spec: gatewayapi_v1alpha2.TLSRouteSpec{
			CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
				ParentRefs: []gatewayapi_v1.ParentReference{
					gatewayapi.GatewayParentRef("projectcontour", "contour"),
				},
			},
			Rules: []gatewayapi_v1alpha2.TLSRouteRule{{
				BackendRefs: gatewayapi.TLSRouteBackendRef("correct-backend", 80, nil),
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
						tcpproxy("https-443", "default/correct-backend/80/da39a3ee5e"),
					),
					FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
						TransportProtocol: "tls",
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

	route3 := &gatewayapi_v1alpha2.TLSRoute{
		ObjectMeta: fixture.ObjectMeta("basic"),
		Spec: gatewayapi_v1alpha2.TLSRouteSpec{
			CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
				ParentRefs: []gatewayapi_v1.ParentReference{
					gatewayapi.GatewayParentRef("projectcontour", "contour"),
				},
			},
			Hostnames: []gatewayapi_v1.Hostname{"tcp.projectcontour.io"},
			Rules: []gatewayapi_v1alpha2.TLSRouteRule{{
				BackendRefs: gatewayapi.TLSRouteBackendRef("correct-backend", 80, nil),
			}},
		},
	}

	route4 := &gatewayapi_v1alpha2.TLSRoute{
		ObjectMeta: fixture.ObjectMeta("basic-wildcard"),
		Spec: gatewayapi_v1alpha2.TLSRouteSpec{
			CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
				ParentRefs: []gatewayapi_v1.ParentReference{
					gatewayapi.GatewayParentRef("projectcontour", "contour"),
				},
			},
			Rules: []gatewayapi_v1alpha2.TLSRouteRule{{
				BackendRefs: gatewayapi.TLSRouteBackendRef("another-backend", 80, nil),
			}},
		},
	}

	rh.OnUpdate(route2, route3)
	rh.OnAdd(route4)

	// Validate that we have a TCP match against 'tcp.projectcontour.io' routing to 'correct-backend`
	// as well as a wildcard TCP match routing to 'another-service'.
	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			&envoy_config_listener_v3.Listener{
				Name:    "https-443",
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_config_listener_v3.FilterChain{{
					Filters: envoy_v3.Filters(
						tcpproxy("https-443", "default/correct-backend/80/da39a3ee5e"),
					),
					FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
						ServerNames: []string{"tcp.projectcontour.io"},
					},
				}, {
					Filters: envoy_v3.Filters(
						tcpproxy("https-443", "default/another-backend/80/da39a3ee5e"),
					),
					FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
						TransportProtocol: "tls",
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

	rh.OnDelete(route1)
	rh.OnDelete(route2)
	rh.OnDelete(route3)
	rh.OnDelete(route4)
}

func TestTLSRoute_TLSTermination(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	rh.OnAdd(fixture.NewService("svc1").
		WithPorts(core_v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)}),
	)

	rh.OnAdd(fixture.NewService("svc2").
		WithPorts(core_v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)}),
	)

	sec1 := featuretests.TLSSecret(t, "projectcontour/tlscert", &featuretests.ServerCertificate)
	rh.OnAdd(sec1)

	rh.OnAdd(gc)

	gateway := &gatewayapi_v1.Gateway{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "contour",
			Namespace: "projectcontour",
		},
		Spec: gatewayapi_v1.GatewaySpec{
			GatewayClassName: gatewayapi_v1.ObjectName(gc.Name),
			Listeners: []gatewayapi_v1.Listener{
				{
					Name:     "tls",
					Port:     5000,
					Protocol: gatewayapi_v1.TLSProtocolType,
					TLS: &gatewayapi_v1.GatewayTLSConfig{
						Mode: ptr.To(gatewayapi_v1.TLSModeTerminate),
						CertificateRefs: []gatewayapi_v1.SecretObjectReference{
							gatewayapi.CertificateRef("tlscert", ""),
						},
					},
					Hostname: ptr.To(gatewayapi_v1.Hostname("*.projectcontour.io")),
					AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
						Namespaces: &gatewayapi_v1.RouteNamespaces{
							From: ptr.To(gatewayapi_v1.NamespacesFromAll),
						},
					},
				},
			},
		},
	}

	rh.OnAdd(gateway)

	rh.OnAdd(&gatewayapi_v1alpha2.TLSRoute{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "basic",
			Namespace: "default",
		},
		Spec: gatewayapi_v1alpha2.TLSRouteSpec{
			CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
				ParentRefs: []gatewayapi_v1.ParentReference{
					gatewayapi.GatewayParentRef("projectcontour", "contour"),
				},
			},
			Hostnames: []gatewayapi_v1.Hostname{
				"test1.projectcontour.io",
			},
			Rules: []gatewayapi_v1alpha2.TLSRouteRule{{
				BackendRefs: gatewayapi.TLSRouteBackendRef("svc1", 80, ptr.To(int32(1))),
			}},
		},
	})

	c.Request(listenerType, "https-5000").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: listenerType,
		Resources: resources(t,
			&envoy_config_listener_v3.Listener{
				Name:    "https-5000",
				Address: envoy_v3.SocketAddress("0.0.0.0", 13000),
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				FilterChains: appendFilterChains(
					filterchaintls("test1.projectcontour.io", sec1, tcpproxy("https-5000", "default/svc1/80/da39a3ee5e"), nil),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			},
		),
	})

	rh.OnAdd(&gatewayapi_v1alpha2.TLSRoute{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "basic-2",
			Namespace: "default",
		},
		Spec: gatewayapi_v1alpha2.TLSRouteSpec{
			CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
				ParentRefs: []gatewayapi_v1.ParentReference{
					gatewayapi.GatewayParentRef("projectcontour", "contour"),
				},
			},
			Hostnames: []gatewayapi_v1.Hostname{
				"test2.projectcontour.io",
			},
			Rules: []gatewayapi_v1alpha2.TLSRouteRule{{
				BackendRefs: gatewayapi.TLSRouteBackendRef("svc2", 80, ptr.To(int32(1))),
			}},
		},
	})

	c.Request(listenerType, "https-5000").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: listenerType,
		Resources: resources(t,
			&envoy_config_listener_v3.Listener{
				Name:    "https-5000",
				Address: envoy_v3.SocketAddress("0.0.0.0", 13000),
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				FilterChains: appendFilterChains(
					filterchaintls("test1.projectcontour.io", sec1, tcpproxy("https-5000", "default/svc1/80/da39a3ee5e"), nil),
					filterchaintls("test2.projectcontour.io", sec1, tcpproxy("https-5000", "default/svc2/80/da39a3ee5e"), nil),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			},
		),
	})
}
