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
	"time"

	envoy_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/featuretests"
	"github.com/projectcontour/contour/internal/fixture"
	v1 "k8s.io/api/core/v1"
	networking_v1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestRetryPolicy(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	s1 := fixture.NewService("backend").
		WithPorts(v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)})
	rh.OnAdd(s1)

	i1 := &networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "hello",
			Namespace: s1.Namespace,
			Annotations: map[string]string{
				"projectcontour.io/retry-on":        "5xx,gateway-error",
				"projectcontour.io/num-retries":     "7",
				"projectcontour.io/per-try-timeout": "120ms",
			},
		},
		Spec: networking_v1.IngressSpec{
			DefaultBackend: featuretests.IngressBackend(s1),
		},
	}
	rh.OnAdd(i1)

	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("*",
					&envoy_route_v3.Route{
						Match:  routePrefix("/"),
						Action: withRetryPolicy(routeCluster("default/backend/80/da39a3ee5e"), "5xx,gateway-error", 7, 120*time.Millisecond),
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	i2 := &networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name: "hello", Namespace: "default",
			Annotations: map[string]string{
				"projectcontour.io/retry-on":        "5xx,gateway-error",
				"projectcontour.io/num-retries":     "7",
				"projectcontour.io/per-try-timeout": "120ms",
			},
		},
		Spec: networking_v1.IngressSpec{
			DefaultBackend: featuretests.IngressBackend(s1),
		},
	}
	rh.OnUpdate(i1, i2)

	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("*",
					&envoy_route_v3.Route{
						Match:  routePrefix("/"),
						Action: withRetryPolicy(routeCluster("default/backend/80/da39a3ee5e"), "5xx,gateway-error", 7, 120*time.Millisecond),
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	i3 := &networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name: "hello", Namespace: "default",
			Annotations: map[string]string{
				"projectcontour.io/retry-on":        "5xx,gateway-error",
				"projectcontour.io/num-retries":     "7",
				"projectcontour.io/per-try-timeout": "120ms",
			},
		},
		Spec: networking_v1.IngressSpec{
			DefaultBackend: featuretests.IngressBackend(s1),
		},
	}
	rh.OnUpdate(i2, i3)

	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("*",
					&envoy_route_v3.Route{
						Match:  routePrefix("/"),
						Action: withRetryPolicy(routeCluster("default/backend/80/da39a3ee5e"), "5xx,gateway-error", 7, 120*time.Millisecond),
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	rh.OnDelete(i3)

	hp1 := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: s1.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{Fqdn: "test3.test.com"},
			Routes: []contour_api_v1.Route{{
				RetryPolicy: &contour_api_v1.RetryPolicy{
					NumRetries:    5,
					PerTryTimeout: "105s",
				},
				Services: []contour_api_v1.Service{{
					Name: s1.Name,
					Port: 80,
				}},
			}},
		},
	}
	rh.OnAdd(hp1)

	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost(hp1.Spec.VirtualHost.Fqdn,
					&envoy_route_v3.Route{
						Match:  routePrefix("/"),
						Action: withRetryPolicy(routeCluster("default/backend/80/da39a3ee5e"), "5xx", 5, 105*time.Second),
					},
				),
			),
		),
		TypeUrl: routeType,
	})
}
