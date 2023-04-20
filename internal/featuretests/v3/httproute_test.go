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

	"github.com/golang/protobuf/ptypes/wrappers"

	envoy_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/featuretests"
	"github.com/projectcontour/contour/internal/fixture"
	"github.com/projectcontour/contour/internal/gatewayapi"
	"github.com/projectcontour/contour/internal/ref"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	gatewayapi_v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

var (
	gc = &gatewayapi_v1beta1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "contour",
		},
		Spec: gatewayapi_v1beta1.GatewayClassSpec{
			ControllerName: "projectcontour.io/contour",
		},
		Status: gatewayapi_v1beta1.GatewayClassStatus{
			Conditions: []metav1.Condition{
				{
					Type:   string(gatewayapi_v1beta1.GatewayClassConditionStatusAccepted),
					Status: metav1.ConditionTrue,
				},
			},
		},
	}

	gateway = &gatewayapi_v1beta1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "contour",
			Namespace: "projectcontour",
		},
		Spec: gatewayapi_v1beta1.GatewaySpec{
			GatewayClassName: gatewayapi_v1beta1.ObjectName(gc.Name),
			Listeners: []gatewayapi_v1beta1.Listener{
				{
					Name:     "http",
					Port:     80,
					Protocol: gatewayapi_v1beta1.HTTPProtocolType,
					AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
						Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
							From: ref.To(gatewayapi_v1beta1.NamespacesFromAll),
						},
					},
				},
				{
					Name:     "https",
					Port:     443,
					Protocol: gatewayapi_v1beta1.HTTPSProtocolType,
					TLS: &gatewayapi_v1beta1.GatewayTLSConfig{
						CertificateRefs: []gatewayapi_v1beta1.SecretObjectReference{
							gatewayapi.CertificateRef("tlscert", ""),
						},
					},
					AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
						Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
							From: ref.To(gatewayapi_v1beta1.NamespacesFromAll),
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
		WithPorts(v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)}),
	)

	rh.OnAdd(fixture.NewService("svc2").
		WithPorts(v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)}),
	)

	sec1 := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tlscert",
			Namespace: "projectcontour",
		},
		Type: v1.SecretTypeTLS,
		Data: featuretests.Secretdata(featuretests.CERTIFICATE, featuretests.RSA_PRIVATE_KEY),
	}

	rh.OnAdd(sec1)

	rh.OnAdd(gc)

	rh.OnAdd(gateway)

	rh.OnAdd(&gatewayapi_v1beta1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "basic",
			Namespace: "default",
			Labels: map[string]string{
				"app":  "contour",
				"type": "controller",
			},
		},
		Spec: gatewayapi_v1beta1.HTTPRouteSpec{
			CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
				ParentRefs: []gatewayapi_v1beta1.ParentReference{
					gatewayapi.GatewayParentRef("projectcontour", "contour"),
				},
			},
			Hostnames: []gatewayapi_v1beta1.Hostname{
				"test.projectcontour.io",
			},
			Rules: []gatewayapi_v1beta1.HTTPRouteRule{{
				Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1beta1.PathMatchPathPrefix, "/blog"),
				BackendRefs: gatewayapi.HTTPBackendRef("svc2", 80, 1),
			}, {
				Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1beta1.PathMatchPathPrefix, "/"),
				BackendRefs: gatewayapi.HTTPBackendRef("svc1", 80, 10),
			}},
		},
	})

	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("https/test.projectcontour.io",
				envoy_v3.VirtualHost("test.projectcontour.io",
					&envoy_route_v3.Route{
						Match:  routeSegmentPrefix("/blog"),
						Action: routeCluster("default/svc2/80/da39a3ee5e"),
					}, &envoy_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routeCluster("default/svc1/80/da39a3ee5e"),
					},
				),
			),
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("test.projectcontour.io",
					&envoy_route_v3.Route{
						Match:  routeSegmentPrefix("/blog"),
						Action: routeCluster("default/svc2/80/da39a3ee5e"),
					}, &envoy_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routeCluster("default/svc1/80/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	c.Request(listenerType, "ingress_https").Equals(&envoy_discovery_v3.DiscoveryResponse{
		TypeUrl: listenerType,
		Resources: resources(t,
			&envoy_listener_v3.Listener{
				Name:                          "ingress_https",
				Address:                       envoy_v3.SocketAddress("0.0.0.0", 8443),
				PerConnectionBufferLimitBytes: &wrappers.UInt32Value{Value: 32768},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				FilterChains: appendFilterChains(
					filterchaintls("test.projectcontour.io", sec1,
						httpsFilterFor("test.projectcontour.io"),
						nil, "h2", "http/1.1"),
				),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			},
		),
	})
}
