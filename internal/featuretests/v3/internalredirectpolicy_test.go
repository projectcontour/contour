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

	"google.golang.org/protobuf/types/known/wrapperspb"

	envoy_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_internal_redirect_previous_routes_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/internal_redirect/previous_routes/v3"
	envoy_internal_redirect_safe_cross_scheme_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/internal_redirect/safe_cross_scheme/v3"
	envoy_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/fixture"
	"github.com/projectcontour/contour/internal/protobuf"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func withInternalRedirectPolicy(route *envoy_route_v3.Route_Route, policy *envoy_route_v3.InternalRedirectPolicy) *envoy_route_v3.Route_Route {
	route.Route.InternalRedirectPolicy = policy
	return route
}

func TestInternalRedirectPolicy_HTTProxy(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	rh.OnAdd(fixture.NewService("svc1").
		WithPorts(v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)}),
	)

	proxy := fixture.NewProxy("simple").WithSpec(
		contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{Fqdn: "hello.world"},
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: "svc1",
					Port: 80,
				}},
				InternalRedirectPolicy: &contour_api_v1.HTTPInternalRedirectPolicy{},
			}},
		})

	rh.OnAdd(proxy)

	conf := c.Request(routeType)
	// Verify default values
	conf.Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("hello.world",
					&envoy_route_v3.Route{
						Match: routePrefix("/"),
						Action: withInternalRedirectPolicy(routeCluster("default/svc1/80/da39a3ee5e"), &envoy_route_v3.InternalRedirectPolicy{
							MaxInternalRedirects:     nil,
							RedirectResponseCodes:    []uint32{},
							Predicates:               nil,
							AllowCrossSchemeRedirect: false,
						}),
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	proxyCrossAlways := fixture.NewProxy("always").WithSpec(
		contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{Fqdn: "hello.world"},
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: "svc1",
					Port: 80,
				}},
				InternalRedirectPolicy: &contour_api_v1.HTTPInternalRedirectPolicy{
					AllowCrossSchemeRedirect: "Always",
				},
			}},
		})

	rh.OnUpdate(proxy, proxyCrossAlways)

	// Always: No predicate and AllowCrossSchemeRedirect true
	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("hello.world",
					&envoy_route_v3.Route{
						Match: routePrefix("/"),
						Action: withInternalRedirectPolicy(routeCluster("default/svc1/80/da39a3ee5e"), &envoy_route_v3.InternalRedirectPolicy{
							AllowCrossSchemeRedirect: true,
						}),
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	proxyCrossNever := fixture.NewProxy("always").WithSpec(
		contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{Fqdn: "hello.world"},
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: "svc1",
					Port: 80,
				}},
				InternalRedirectPolicy: &contour_api_v1.HTTPInternalRedirectPolicy{
					AllowCrossSchemeRedirect: "Never",
				},
			}},
		})

	rh.OnUpdate(proxyCrossAlways, proxyCrossNever)

	// Never: No predicate and AllowCrossSchemeRedirect false
	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("hello.world",
					&envoy_route_v3.Route{
						Match: routePrefix("/"),
						Action: withInternalRedirectPolicy(routeCluster("default/svc1/80/da39a3ee5e"), &envoy_route_v3.InternalRedirectPolicy{
							AllowCrossSchemeRedirect: false,
						}),
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	proxySafeOnly := fixture.NewProxy("simple").WithSpec(
		contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{Fqdn: "hello.world"},
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: "svc1",
					Port: 80,
				}},
				InternalRedirectPolicy: &contour_api_v1.HTTPInternalRedirectPolicy{
					MaxInternalRedirects:      2,
					RedirectResponseCodes:     []contour_api_v1.RedirectResponseCode{302, 307},
					DenyRepeatedRouteRedirect: true,
					AllowCrossSchemeRedirect:  "SafeOnly",
				},
			}},
		})

	rh.OnUpdate(proxyCrossNever, proxySafeOnly)

	// Ensure predicates are properly generated for SafeOnly and DenyRepeatedRouteRedirect
	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("hello.world",
					&envoy_route_v3.Route{
						Match: routePrefix("/"),
						Action: withInternalRedirectPolicy(routeCluster("default/svc1/80/da39a3ee5e"), &envoy_route_v3.InternalRedirectPolicy{
							MaxInternalRedirects:  wrapperspb.UInt32(2),
							RedirectResponseCodes: []uint32{302, 307},
							Predicates: []*envoy_core_v3.TypedExtensionConfig{
								{
									Name:        "envoy.internal_redirect_predicates.safe_cross_scheme",
									TypedConfig: protobuf.MustMarshalAny(&envoy_internal_redirect_safe_cross_scheme_v3.SafeCrossSchemeConfig{}),
								},
								{
									Name:        "envoy.internal_redirect_predicates.previous_routes",
									TypedConfig: protobuf.MustMarshalAny(&envoy_internal_redirect_previous_routes_v3.PreviousRoutesConfig{}),
								},
							},
							AllowCrossSchemeRedirect: true,
						}),
					},
				),
			),
		),
		TypeUrl: routeType,
	})

}
