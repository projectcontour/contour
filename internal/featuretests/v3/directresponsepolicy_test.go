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
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
	"testing"

	envoy_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/fixture"
)

func TestDirectResponsePolicy_HTTProxy(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	rh.OnAdd(fixture.NewService("svc1").
		WithPorts(v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)}),
	)

	proxy403 := fixture.NewProxy("simple-403").WithSpec(
		contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{Fqdn: "directresponse.projectcontour.io"},
			Routes: []contour_api_v1.Route{{
				DirectResponsePolicy: &contour_api_v1.HTTPDirectResponsePolicy{
					StatusCode: pointer.Int(403),
					Body:       pointer.StringPtr("forbidden"),
				},
			}},
		})

	rh.OnAdd(proxy403)

	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("directresponse.projectcontour.io",

					&envoy_route_v3.Route{
						Match: routePrefix("/"),
						Action: &envoy_route_v3.Route_DirectResponse{
							DirectResponse: &envoy_route_v3.DirectResponseAction{
								Status: 403,
								Body: &envoy_core_v3.DataSource{
									Specifier: &envoy_core_v3.DataSource_InlineString{
										InlineString: "forbidden",
									},
								},
							},
						},
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	proxyNobody := fixture.NewProxy("simple-nobody").WithSpec(
		contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{Fqdn: "directresponse.projectcontour.io"},
			Routes: []contour_api_v1.Route{{
				DirectResponsePolicy: &contour_api_v1.HTTPDirectResponsePolicy{
					StatusCode: pointer.Int(200),
				},
			}},
		})

	rh.OnUpdate(proxy403, proxyNobody)

	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("directresponse.projectcontour.io",

					&envoy_route_v3.Route{
						Match: routePrefix("/"),
						Action: &envoy_route_v3.Route_DirectResponse{
							DirectResponse: &envoy_route_v3.DirectResponseAction{
								Status: 200,
							},
						},
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	proxyInvalid := fixture.NewProxy("simple-multiple-match").WithSpec(
		contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{Fqdn: "directresponse.projectcontour.io"},
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: "svc1",
					Port: 80,
				}},
				DirectResponsePolicy: &contour_api_v1.HTTPDirectResponsePolicy{
					StatusCode: pointer.Int(200),
				},
			}},
		})

	rh.OnUpdate(proxyNobody, proxyInvalid)

	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http"),
		),
		TypeUrl: routeType,
	})
}
