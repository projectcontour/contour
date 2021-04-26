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

	envoy_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"github.com/projectcontour/contour/internal/dag"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/featuretests"
	"github.com/projectcontour/contour/internal/fixture"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
	gatewayapi_v1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

var gateway = &gatewayapi_v1alpha1.Gateway{
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
		}, {
			Port:     443,
			Protocol: "HTTPS",
			TLS: &gatewayapi_v1alpha1.GatewayTLSConfig{
				CertificateRef: &gatewayapi_v1alpha1.LocalObjectReference{
					Group: "core",
					Kind:  "Secret",
					Name:  "tlscert",
				},
			},
			Routes: gatewayapi_v1alpha1.RouteBindingSelector{
				Namespaces: &gatewayapi_v1alpha1.RouteNamespaces{
					From: routeSelectTypePtr(gatewayapi_v1alpha1.RouteSelectAll),
				},
				Kind: dag.KindHTTPRoute,
			},
		}},
	},
}

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

	rh.OnAdd(gateway)

	rh.OnAdd(&gatewayapi_v1alpha1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "basic",
			Namespace: "default",
			Labels: map[string]string{
				"app":  "contour",
				"type": "controller",
			},
		},
		Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
			Gateways: gatewayapi_v1alpha1.RouteGateways{
				Allow: gatewayapi_v1alpha1.GatewayAllowAll,
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
					ServiceName: pointer.StringPtr("svc2"),
					Port:        gatewayPort(80),
					Weight:      pointer.Int32Ptr(1),
				}},
			}, {
				Matches: []gatewayapi_v1alpha1.HTTPRouteMatch{{
					Path: &gatewayapi_v1alpha1.HTTPPathMatch{
						Type:  pathMatchTypePtr(gatewayapi_v1alpha1.PathMatchPrefix),
						Value: pointer.StringPtr("/"),
					},
				}},
				ForwardTo: []gatewayapi_v1alpha1.HTTPRouteForwardTo{{
					ServiceName: pointer.StringPtr("svc1"),
					Port:        gatewayPort(80),
					Weight:      pointer.Int32Ptr(10),
				}},
			}},
		},
	})

	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("https/test.projectcontour.io",
				envoy_v3.VirtualHost("test.projectcontour.io",
					&envoy_route_v3.Route{
						Match:  routePrefix("/blog"),
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
						Match:  routePrefix("/blog"),
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
				Name:    "ingress_https",
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
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

func gatewayPort(port int) *gatewayapi_v1alpha1.PortNumber {
	p := gatewayapi_v1alpha1.PortNumber(port)
	return &p
}

func pathMatchTypePtr(pmt gatewayapi_v1alpha1.PathMatchType) *gatewayapi_v1alpha1.PathMatchType {
	return &pmt
}

func routeSelectTypePtr(rst gatewayapi_v1alpha1.RouteSelectType) *gatewayapi_v1alpha1.RouteSelectType {
	return &rst
}
