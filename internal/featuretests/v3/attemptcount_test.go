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

	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/fixture"
)

func TestVirtualHostAttemptCount(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	s1 := fixture.NewService("backend").
		WithPorts(core_v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)})
	rh.OnAdd(s1)

	// x-envoy-attempt-count is emitted only in the request.
	hp1 := &contour_v1.HTTPProxy{
		ObjectMeta: fixture.ObjectMeta("simple"),
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn:                       "attemptcount.projectcontour.io",
				IncludeRequestAttemptCount: true,
			},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: s1.Name,
					Port: 80,
				}},
			}},
		},
	}
	rh.OnAdd(hp1)

	requestOnly := envoy_v3.VirtualHost(hp1.Spec.VirtualHost.Fqdn,
		&envoy_config_route_v3.Route{
			Match:  routePrefix("/"),
			Action: routeCluster("default/backend/80/da39a3ee5e"),
		},
	)
	requestOnly.IncludeRequestAttemptCount = true

	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http", requestOnly),
		),
		TypeUrl: routeType,
	})

	// x-envoy-attempt-count is emitted in both the request and response.
	hp2 := &contour_v1.HTTPProxy{
		ObjectMeta: fixture.ObjectMeta("simple"),
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn:                          "attemptcount.projectcontour.io",
				IncludeRequestAttemptCount:    true,
				IncludeAttemptCountInResponse: true,
			},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: s1.Name,
					Port: 80,
				}},
			}},
		},
	}
	rh.OnUpdate(hp1, hp2)

	requestAndResponse := envoy_v3.VirtualHost(hp1.Spec.VirtualHost.Fqdn,
		&envoy_config_route_v3.Route{
			Match:  routePrefix("/"),
			Action: routeCluster("default/backend/80/da39a3ee5e"),
		},
	)
	requestAndResponse.IncludeRequestAttemptCount = true
	requestAndResponse.IncludeAttemptCountInResponse = true

	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http", requestAndResponse),
		),
		TypeUrl: routeType,
	})

	// Fields default to false when unset.
	hp3 := &contour_v1.HTTPProxy{
		ObjectMeta: fixture.ObjectMeta("simple"),
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "attemptcount.projectcontour.io",
			},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: s1.Name,
					Port: 80,
				}},
			}},
		},
	}
	rh.OnUpdate(hp2, hp3)

	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost(hp1.Spec.VirtualHost.Fqdn,
					&envoy_config_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routeCluster("default/backend/80/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	rh.OnDelete(hp3)
}
