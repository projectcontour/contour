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

	"github.com/projectcontour/contour/internal/featuretests"
	"github.com/projectcontour/contour/internal/gatewayapi"

	envoy_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/fixture"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func TestTLSRoute(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	svc := fixture.NewService("correct-backend").
		WithPorts(v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)})

	svcAnother := fixture.NewService("another-backend").
		WithPorts(v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)})

	rh.OnAdd(svc)
	rh.OnAdd(svcAnother)

	rh.OnAdd(&gatewayapi_v1alpha2.GatewayClass{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-gc",
		},
		Spec: gatewayapi_v1alpha2.GatewayClassSpec{
			Controller: "projectcontour.io/contour",
		},
		Status: gatewayapi_v1alpha2.GatewayClassStatus{
			Conditions: []metav1.Condition{
				{
					Type:   string(gatewayapi_v1alpha2.GatewayClassConditionStatusAdmitted),
					Status: metav1.ConditionTrue,
				},
			},
		},
	})

	gatewayPassthrough := &gatewayapi_v1alpha2.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "contour",
			Namespace: "projectcontour",
		},
		Spec: gatewayapi_v1alpha2.GatewaySpec{
			Listeners: []gatewayapi_v1alpha2.Listener{{
				Port:     443,
				Protocol: gatewayapi_v1alpha2.TLSProtocolType,
				TLS: &gatewayapi_v1alpha2.GatewayTLSConfig{
					Mode: gatewayapi.TLSModeTypePtr(gatewayapi_v1alpha2.TLSModePassthrough),
				},
				AllowedRoutes: &gatewayapi_v1alpha2.AllowedRoutes{
					Namespaces: &gatewayapi_v1alpha2.RouteNamespaces{
						From: gatewayapi.FromNamespacesPtr(gatewayapi_v1alpha2.NamespacesFromAll),
					},
				},
			}},
		},
	}

	rh.OnAdd(gatewayPassthrough)

	route1 := &gatewayapi_v1alpha2.TLSRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "basic",
			Namespace: "default",
		},
		Spec: gatewayapi_v1alpha2.TLSRouteSpec{
			CommonRouteSpec: gatewayapi_v1alpha2.CommonRouteSpec{
				ParentRefs: []gatewayapi_v1alpha2.ParentRef{
					gatewayapi.GatewayParentRef("projectcontour", "contour"),
				},
			},
			Hostnames: []gatewayapi_v1alpha2.Hostname{"tcp.projectcontour.io"},
			Rules: []gatewayapi_v1alpha2.TLSRouteRule{{
				BackendRefs: gatewayapi.TLSRouteBackendRef("correct-backend", 80, nil),
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
						tcpproxy("ingress_https", "default/correct-backend/80/da39a3ee5e"),
					),
					FilterChainMatch: &envoy_listener_v3.FilterChainMatch{
						ServerNames: []string{"tcp.projectcontour.io"},
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

	// Route2 doesn't define any SNIs, so this should become the default backend.
	route2 := &gatewayapi_v1alpha2.TLSRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "basic",
			Namespace: "default",
		},
		Spec: gatewayapi_v1alpha2.TLSRouteSpec{
			CommonRouteSpec: gatewayapi_v1alpha2.CommonRouteSpec{
				ParentRefs: []gatewayapi_v1alpha2.ParentRef{
					gatewayapi.GatewayParentRef("projectcontour", "contour"),
				},
			},
			Rules: []gatewayapi_v1alpha2.TLSRouteRule{{
				BackendRefs: gatewayapi.TLSRouteBackendRef("correct-backend", 80, nil),
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
						tcpproxy("ingress_https", "default/correct-backend/80/da39a3ee5e"),
					),
					FilterChainMatch: &envoy_listener_v3.FilterChainMatch{
						TransportProtocol: "tls",
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

	route3 := &gatewayapi_v1alpha2.TLSRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "basic",
			Namespace: "default",
		},
		Spec: gatewayapi_v1alpha2.TLSRouteSpec{
			CommonRouteSpec: gatewayapi_v1alpha2.CommonRouteSpec{
				ParentRefs: []gatewayapi_v1alpha2.ParentRef{
					gatewayapi.GatewayParentRef("projectcontour", "contour"),
				},
			},
			Hostnames: []gatewayapi_v1alpha2.Hostname{"tcp.projectcontour.io"},
			Rules: []gatewayapi_v1alpha2.TLSRouteRule{{
				BackendRefs: gatewayapi.TLSRouteBackendRef("correct-backend", 80, nil),
			}},
		},
	}

	route4 := &gatewayapi_v1alpha2.TLSRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "basic-wildcard",
			Namespace: "default",
		},
		Spec: gatewayapi_v1alpha2.TLSRouteSpec{
			CommonRouteSpec: gatewayapi_v1alpha2.CommonRouteSpec{
				ParentRefs: []gatewayapi_v1alpha2.ParentRef{
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
	c.Request(listenerType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			&envoy_listener_v3.Listener{
				Name:    "ingress_https",
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_listener_v3.FilterChain{{
					Filters: envoy_v3.Filters(
						tcpproxy("ingress_https", "default/correct-backend/80/da39a3ee5e"),
					),
					FilterChainMatch: &envoy_listener_v3.FilterChainMatch{
						ServerNames: []string{"tcp.projectcontour.io"},
					},
				}, {
					Filters: envoy_v3.Filters(
						tcpproxy("ingress_https", "default/another-backend/80/da39a3ee5e"),
					),
					FilterChainMatch: &envoy_listener_v3.FilterChainMatch{
						TransportProtocol: "tls",
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

	rh.OnDelete(route1)
	rh.OnDelete(route2)
	rh.OnDelete(route3)
	rh.OnDelete(route4)

	sec1 := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tlscert",
			Namespace: "projectcontour",
		},
		Type: "kubernetes.io/tls",
		Data: featuretests.Secretdata(featuretests.CERTIFICATE, featuretests.RSA_PRIVATE_KEY),
	}

	// Validate TLSTerminate.
	gatewayTerminate := &gatewayapi_v1alpha2.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "contour",
			Namespace: "projectcontour",
		},
		Spec: gatewayapi_v1alpha2.GatewaySpec{
			Listeners: []gatewayapi_v1alpha2.Listener{{
				Port:     443,
				Protocol: "TLS",
				TLS: &gatewayapi_v1alpha2.GatewayTLSConfig{
					Mode:           gatewayapi.TLSModeTypePtr(gatewayapi_v1alpha2.TLSModeTerminate),
					CertificateRef: gatewayapi.CertificateRef("tlscert", ""),
				},
				AllowedRoutes: &gatewayapi_v1alpha2.AllowedRoutes{
					Namespaces: &gatewayapi_v1alpha2.RouteNamespaces{
						From: gatewayapi.FromNamespacesPtr(gatewayapi_v1alpha2.NamespacesFromAll),
					},
				},
			}},
		},
	}

	rh.OnAdd(sec1)
	rh.OnAdd(gatewayTerminate)
	rh.OnAdd(route1)

	c.Request(listenerType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			&envoy_listener_v3.Listener{
				Name:    "ingress_https",
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: appendFilterChains(
					filterchaintls("tcp.projectcontour.io", sec1, tcpproxy("ingress_https", "default/correct-backend/80/da39a3ee5e"), nil),
				),
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
