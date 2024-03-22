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
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	gatewayapi_v1 "sigs.k8s.io/gateway-api/apis/v1"

	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/featuretests"
	"github.com/projectcontour/contour/internal/fixture"
	"github.com/projectcontour/contour/internal/gatewayapi"
)

var (
	gc = &gatewayapi_v1.GatewayClass{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "contour",
		},
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
	}

	gateway = &gatewayapi_v1.Gateway{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "contour",
			Namespace: "projectcontour",
		},
		Spec: gatewayapi_v1.GatewaySpec{
			GatewayClassName: gatewayapi_v1.ObjectName(gc.Name),
			Listeners: []gatewayapi_v1.Listener{
				{
					Name:     "http",
					Port:     80,
					Protocol: gatewayapi_v1.HTTPProtocolType,
					AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
						Namespaces: &gatewayapi_v1.RouteNamespaces{
							From: ptr.To(gatewayapi_v1.NamespacesFromAll),
						},
					},
				},
				{
					Name:     "https",
					Port:     443,
					Protocol: gatewayapi_v1.HTTPSProtocolType,
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
)

func TestGateway_TLS(t *testing.T) {
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

	rh.OnAdd(gateway)

	rh.OnAdd(&gatewayapi_v1.HTTPRoute{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "basic",
			Namespace: "default",
		},
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
				BackendRefs: gatewayapi.HTTPBackendRef("svc2", 80, 1),
			}, {
				Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
				BackendRefs: gatewayapi.HTTPBackendRef("svc1", 80, 10),
			}},
		},
	})

	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("http-80",
				envoy_v3.VirtualHost("test.projectcontour.io",
					&envoy_config_route_v3.Route{
						Match:  routeSegmentPrefix("/blog"),
						Action: routeCluster("default/svc2/80/da39a3ee5e"),
					}, &envoy_config_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routeCluster("default/svc1/80/da39a3ee5e"),
					},
				),
			),
			envoy_v3.RouteConfiguration("https-443/test.projectcontour.io",
				envoy_v3.VirtualHost("test.projectcontour.io",
					&envoy_config_route_v3.Route{
						Match:  routeSegmentPrefix("/blog"),
						Action: routeCluster("default/svc2/80/da39a3ee5e"),
					}, &envoy_config_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routeCluster("default/svc1/80/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	c.Request(listenerType, "https-443").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: listenerType,
		Resources: resources(t,
			&envoy_config_listener_v3.Listener{
				Name:    "https-443",
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				FilterChains: appendFilterChains(
					filterchaintls("test.projectcontour.io", sec1,
						httpsFilterForGateway("https-443", "test.projectcontour.io"),
						nil, "h2", "http/1.1"),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			},
		),
	})
}
