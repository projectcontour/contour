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

func TestTCPRoute(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	svc1 := fixture.NewService("backend-1").
		WithPorts(core_v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)})

	svc2 := fixture.NewService("backend-2").
		WithPorts(core_v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)})

	rh.OnAdd(svc1)
	rh.OnAdd(svc2)

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

	gateway := &gatewayapi_v1.Gateway{
		ObjectMeta: fixture.ObjectMeta("projectcontour/contour"),
		Spec: gatewayapi_v1.GatewaySpec{
			Listeners: []gatewayapi_v1.Listener{{
				Name:     "tcp-1",
				Port:     10000,
				Protocol: gatewayapi_v1.TCPProtocolType,
				AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
					Namespaces: &gatewayapi_v1.RouteNamespaces{
						From: ptr.To(gatewayapi_v1.NamespacesFromAll),
					},
				},
			}},
		},
	}
	rh.OnAdd(gateway)

	route1 := &gatewayapi_v1alpha2.TCPRoute{
		ObjectMeta: fixture.ObjectMeta("tcproute-1"),
		Spec: gatewayapi_v1alpha2.TCPRouteSpec{
			CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
				ParentRefs: []gatewayapi_v1.ParentReference{
					{
						Namespace:   ptr.To(gatewayapi_v1.Namespace("projectcontour")),
						Name:        gatewayapi_v1.ObjectName("contour"),
						SectionName: ptr.To(gatewayapi_v1.SectionName("tcp-1")),
					},
				},
			},
			Rules: []gatewayapi_v1alpha2.TCPRouteRule{{
				BackendRefs: gatewayapi.TLSRouteBackendRef("backend-1", 80, nil),
			}},
		},
	}
	rh.OnAdd(route1)

	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			statsListener(),
			&envoy_config_listener_v3.Listener{
				Name:    "tcp-10000",
				Address: envoy_v3.SocketAddress("0.0.0.0", 18000),
				FilterChains: []*envoy_config_listener_v3.FilterChain{{
					Filters: envoy_v3.Filters(
						tcpproxy("tcp-10000", "default/backend-1/80/da39a3ee5e"),
					),
				}},
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			},
		),
		TypeUrl: listenerType,
	})

	// check that there is no route config
	require.Empty(t, c.Request(routeType).Resources)

	gateway.Spec.Listeners = append(gateway.Spec.Listeners, gatewayapi_v1.Listener{
		Name:     "tcp-2",
		Port:     10001,
		Protocol: gatewayapi_v1.TCPProtocolType,
		AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
			Namespaces: &gatewayapi_v1.RouteNamespaces{
				From: ptr.To(gatewayapi_v1.NamespacesFromAll),
			},
		},
	})
	rh.OnUpdate(gateway, gateway)

	route2 := &gatewayapi_v1alpha2.TCPRoute{
		ObjectMeta: fixture.ObjectMeta("tcproute-2"),
		Spec: gatewayapi_v1alpha2.TCPRouteSpec{
			CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
				ParentRefs: []gatewayapi_v1.ParentReference{
					{
						Namespace:   ptr.To(gatewayapi_v1.Namespace("projectcontour")),
						Name:        gatewayapi_v1.ObjectName("contour"),
						SectionName: ptr.To(gatewayapi_v1.SectionName("tcp-2")),
					},
				},
			},
			Rules: []gatewayapi_v1alpha2.TCPRouteRule{{
				BackendRefs: gatewayapi.TLSRouteBackendRef("backend-2", 80, nil),
			}},
		},
	}
	rh.OnAdd(route2)

	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			statsListener(),
			&envoy_config_listener_v3.Listener{
				Name:    "tcp-10000",
				Address: envoy_v3.SocketAddress("0.0.0.0", 18000),
				FilterChains: []*envoy_config_listener_v3.FilterChain{{
					Filters: envoy_v3.Filters(
						tcpproxy("tcp-10000", "default/backend-1/80/da39a3ee5e"),
					),
				}},
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			},
			&envoy_config_listener_v3.Listener{
				Name:    "tcp-10001",
				Address: envoy_v3.SocketAddress("0.0.0.0", 18001),
				FilterChains: []*envoy_config_listener_v3.FilterChain{{
					Filters: envoy_v3.Filters(
						tcpproxy("tcp-10001", "default/backend-2/80/da39a3ee5e"),
					),
				}},
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			},
		),
		TypeUrl: listenerType,
	})

	// check that there is no route config
	require.Empty(t, c.Request(routeType).Resources)
}

func TestTCPRoute_TLSTermination(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	svc1 := fixture.NewService("backend-1").
		WithPorts(core_v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)})

	rh.OnAdd(svc1)

	sec1 := featuretests.TLSSecret(t, "projectcontour/tlscert", &featuretests.ServerCertificate)
	rh.OnAdd(sec1)

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

	gateway := &gatewayapi_v1.Gateway{
		ObjectMeta: fixture.ObjectMeta("projectcontour/contour"),
		Spec: gatewayapi_v1.GatewaySpec{
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

	route1 := &gatewayapi_v1alpha2.TCPRoute{
		ObjectMeta: fixture.ObjectMeta("tcproute-1"),
		Spec: gatewayapi_v1alpha2.TCPRouteSpec{
			CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
				ParentRefs: []gatewayapi_v1.ParentReference{
					{
						Namespace:   ptr.To(gatewayapi_v1.Namespace("projectcontour")),
						Name:        gatewayapi_v1.ObjectName("contour"),
						SectionName: ptr.To(gatewayapi_v1.SectionName("tls")),
					},
				},
			},
			Rules: []gatewayapi_v1alpha2.TCPRouteRule{{
				BackendRefs: gatewayapi.TLSRouteBackendRef("backend-1", 80, nil),
			}},
		},
	}
	rh.OnAdd(route1)

	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			&envoy_config_listener_v3.Listener{
				Name:    "https-5000",
				Address: envoy_v3.SocketAddress("0.0.0.0", 13000),
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				FilterChains: appendFilterChains(
					filterchaintls("*", sec1, tcpproxy("https-5000", "default/backend-1/80/da39a3ee5e"), nil),
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
