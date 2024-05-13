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

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"google.golang.org/protobuf/types/known/structpb"
	core_v1 "k8s.io/api/core/v1"
	networking_v1 "k8s.io/api/networking/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	gatewayapi_v1 "sigs.k8s.io/gateway-api/apis/v1"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/dag"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/featuretests"
	"github.com/projectcontour/contour/internal/fixture"
	"github.com/projectcontour/contour/internal/gatewayapi"
)

func TestRouteSourceMetadataIsSet(t *testing.T) {
	setRouteSourceMetadata := func(builder *dag.Builder) {
		for _, processor := range builder.Processors {
			switch processor := processor.(type) {
			case *dag.IngressProcessor:
				processor.SetSourceMetadataOnRoutes = true
			case *dag.HTTPProxyProcessor:
				processor.SetSourceMetadataOnRoutes = true
			case *dag.GatewayAPIProcessor:
				processor.SetSourceMetadataOnRoutes = true
			}
		}
	}

	rh, c, done := setup(t, setRouteSourceMetadata)
	defer done()

	s1 := fixture.NewService("kuard").WithPorts(core_v1.ServicePort{Name: "http", Port: 80, TargetPort: intstr.FromInt(8080)})
	rh.OnAdd(s1)

	// Test an Ingress route gets it source metadata set correctly.
	ing := &networking_v1.Ingress{
		ObjectMeta: fixture.ObjectMeta("default/ingress-kuard"),
		Spec: networking_v1.IngressSpec{
			Rules: []networking_v1.IngressRule{
				{
					Host: "ingress.projectcontour.io",
					IngressRuleValue: networking_v1.IngressRuleValue{
						HTTP: &networking_v1.HTTPIngressRuleValue{
							Paths: []networking_v1.HTTPIngressPath{
								{
									Path:    "/",
									Backend: *featuretests.IngressBackend(s1),
								},
							},
						},
					},
				},
			},
		},
	}
	rh.OnAdd(ing)

	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		VersionInfo: "1",
		Resources: routeResources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("ingress.projectcontour.io", &envoy_config_route_v3.Route{
					Match:  routePrefix("/"),
					Action: routecluster("default/kuard/80/da39a3ee5e"),
					Metadata: &envoy_config_core_v3.Metadata{
						FilterMetadata: map[string]*structpb.Struct{
							"envoy.access_loggers.file": {
								Fields: map[string]*structpb.Value{
									"io.projectcontour.kind":      structpb.NewStringValue("Ingress"),
									"io.projectcontour.namespace": structpb.NewStringValue(ing.Namespace),
									"io.projectcontour.name":      structpb.NewStringValue(ing.Name),
								},
							},
						},
					},
					Decorator: &envoy_config_route_v3.Decorator{
						Operation: "ingress-kuard",
					},
				}),
			),
		),
		TypeUrl: routeType,
		Nonce:   "1",
	})

	rh.OnDelete(ing)

	// Test an HTTPProxy route gets it source metadata set correctly.
	httpProxy := &contour_v1.HTTPProxy{
		ObjectMeta: fixture.ObjectMeta("default/httpproxy-kuard"),
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "httpproxy.projectcontour.io",
			},
			Routes: []contour_v1.Route{
				{
					Conditions: []contour_v1.MatchCondition{{Prefix: "/"}},
					Services:   []contour_v1.Service{{Name: "kuard", Port: 80}},
				},
			},
		},
	}

	rh.OnAdd(httpProxy)

	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		VersionInfo: "1",
		Resources: routeResources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("httpproxy.projectcontour.io", &envoy_config_route_v3.Route{
					Match:  routePrefix("/"),
					Action: routecluster("default/kuard/80/da39a3ee5e"),
					Metadata: &envoy_config_core_v3.Metadata{
						FilterMetadata: map[string]*structpb.Struct{
							"envoy.access_loggers.file": {
								Fields: map[string]*structpb.Value{
									"io.projectcontour.kind":      structpb.NewStringValue("HTTPProxy"),
									"io.projectcontour.namespace": structpb.NewStringValue(httpProxy.Namespace),
									"io.projectcontour.name":      structpb.NewStringValue(httpProxy.Name),
								},
							},
						},
					},
					Decorator: &envoy_config_route_v3.Decorator{
						Operation: "httpproxy-kuard",
					},
				}),
			),
		),
		TypeUrl: routeType,
		Nonce:   "1",
	})

	rh.OnDelete(httpProxy)

	// Test a Gateway API HTTPRoute route gets it source metadata set correctly.
	rh.OnAdd(gc)
	rh.OnAdd(gateway)
	httpRoute := &gatewayapi_v1.HTTPRoute{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "httproute-kuard",
			Namespace: "default",
		},
		Spec: gatewayapi_v1.HTTPRouteSpec{
			CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
				ParentRefs: []gatewayapi_v1.ParentReference{
					gatewayapi.GatewayListenerParentRef("projectcontour", "contour", "http", 80),
				},
			},
			Hostnames: []gatewayapi_v1.Hostname{
				"gatewayapi.projectcontour.io",
			},
			Rules: []gatewayapi_v1.HTTPRouteRule{{
				Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
				BackendRefs: gatewayapi.HTTPBackendRef("kuard", 80, 1),
			}},
		},
	}
	rh.OnAdd(httpRoute)

	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		VersionInfo: "1",
		Resources: routeResources(t,
			envoy_v3.RouteConfiguration("http-80",
				envoy_v3.VirtualHost("gatewayapi.projectcontour.io", &envoy_config_route_v3.Route{
					Match:  routePrefix("/"),
					Action: routecluster("default/kuard/80/da39a3ee5e"),
					Metadata: &envoy_config_core_v3.Metadata{
						FilterMetadata: map[string]*structpb.Struct{
							"envoy.access_loggers.file": {
								Fields: map[string]*structpb.Value{
									"io.projectcontour.kind":      structpb.NewStringValue("HTTPRoute"),
									"io.projectcontour.namespace": structpb.NewStringValue(httpRoute.Namespace),
									"io.projectcontour.name":      structpb.NewStringValue(httpRoute.Name),
								},
							},
						},
					},
					Decorator: &envoy_config_route_v3.Decorator{
						Operation: "httproute-kuard",
					},
				}),
			),
		),
		TypeUrl: routeType,
		Nonce:   "1",
	})
}
