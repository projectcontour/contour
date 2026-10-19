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

	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	core_v1 "k8s.io/api/core/v1"
	networking_v1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/featuretests"
	"github.com/projectcontour/contour/internal/fixture"
)

func TestRetryPolicy(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	s1 := fixture.NewService("backend").
		WithPorts(core_v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)})
	rh.OnAdd(s1)

	i1 := &networking_v1.Ingress{
		ObjectMeta: fixture.ObjectMetaWithAnnotations("hello", map[string]string{
			"projectcontour.io/retry-on":        "5xx,gateway-error",
			"projectcontour.io/num-retries":     "7",
			"projectcontour.io/per-try-timeout": "120ms",
		}),
		Spec: networking_v1.IngressSpec{
			DefaultBackend: featuretests.IngressBackend(s1),
		},
	}
	rh.OnAdd(i1)

	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("*",
					&envoy_config_route_v3.Route{
						Match:  routePrefix("/"),
						Action: withRetryPolicy(routeCluster("default/backend/80/da39a3ee5e"), "5xx,gateway-error", 7, 120*time.Millisecond),
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	i2 := &networking_v1.Ingress{
		ObjectMeta: fixture.ObjectMetaWithAnnotations("hello", map[string]string{
			"projectcontour.io/retry-on":        "5xx,gateway-error",
			"projectcontour.io/num-retries":     "7",
			"projectcontour.io/per-try-timeout": "120ms",
		}),
		Spec: networking_v1.IngressSpec{
			DefaultBackend: featuretests.IngressBackend(s1),
		},
	}
	rh.OnUpdate(i1, i2)

	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("*",
					&envoy_config_route_v3.Route{
						Match:  routePrefix("/"),
						Action: withRetryPolicy(routeCluster("default/backend/80/da39a3ee5e"), "5xx,gateway-error", 7, 120*time.Millisecond),
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	i3 := &networking_v1.Ingress{
		ObjectMeta: fixture.ObjectMetaWithAnnotations("hello", map[string]string{
			"projectcontour.io/retry-on":        "5xx,gateway-error",
			"projectcontour.io/num-retries":     "7",
			"projectcontour.io/per-try-timeout": "120ms",
		}),
		Spec: networking_v1.IngressSpec{
			DefaultBackend: featuretests.IngressBackend(s1),
		},
	}
	rh.OnUpdate(i2, i3)

	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("*",
					&envoy_config_route_v3.Route{
						Match:  routePrefix("/"),
						Action: withRetryPolicy(routeCluster("default/backend/80/da39a3ee5e"), "5xx,gateway-error", 7, 120*time.Millisecond),
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	i4 := &networking_v1.Ingress{
		ObjectMeta: fixture.ObjectMetaWithAnnotations("hello", map[string]string{
			"projectcontour.io/retry-on":        "5xx,gateway-error",
			"projectcontour.io/num-retries":     "-1",
			"projectcontour.io/per-try-timeout": "120ms",
		}),
		Spec: networking_v1.IngressSpec{
			DefaultBackend: featuretests.IngressBackend(s1),
		},
	}
	rh.OnUpdate(i3, i4)

	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("*",
					&envoy_config_route_v3.Route{
						Match:  routePrefix("/"),
						Action: withRetryPolicy(routeCluster("default/backend/80/da39a3ee5e"), "5xx,gateway-error", 0, 120*time.Millisecond),
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	i5 := &networking_v1.Ingress{
		ObjectMeta: fixture.ObjectMetaWithAnnotations("hello", map[string]string{
			"projectcontour.io/retry-on":        "5xx,gateway-error",
			"projectcontour.io/num-retries":     "0",
			"projectcontour.io/per-try-timeout": "120ms",
		}),
		Spec: networking_v1.IngressSpec{
			DefaultBackend: featuretests.IngressBackend(s1),
		},
	}
	rh.OnUpdate(i4, i5)

	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("*",
					&envoy_config_route_v3.Route{
						Match:  routePrefix("/"),
						Action: withRetryPolicy(routeCluster("default/backend/80/da39a3ee5e"), "5xx,gateway-error", 1, 120*time.Millisecond),
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	rh.OnDelete(i5)

	hp1 := &contour_v1.HTTPProxy{
		ObjectMeta: fixture.ObjectMeta("simple"),
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{Fqdn: "test3.test.com"},
			Routes: []contour_v1.Route{{
				RetryPolicy: &contour_v1.RetryPolicy{
					NumRetries:    5,
					PerTryTimeout: "105s",
				},
				Services: []contour_v1.Service{{
					Name: s1.Name,
					Port: 80,
				}},
			}},
		},
	}
	rh.OnAdd(hp1)

	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost(hp1.Spec.VirtualHost.Fqdn,
					&envoy_config_route_v3.Route{
						Match:  routePrefix("/"),
						Action: withRetryPolicy(routeCluster("default/backend/80/da39a3ee5e"), "5xx", 5, 105*time.Second),
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	hp2 := &contour_v1.HTTPProxy{
		ObjectMeta: fixture.ObjectMeta("simple"),
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{Fqdn: "test3.test.com"},
			Routes: []contour_v1.Route{{
				RetryPolicy: &contour_v1.RetryPolicy{
					NumRetries:    -1,
					PerTryTimeout: "105s",
				},
				Services: []contour_v1.Service{{
					Name: s1.Name,
					Port: 80,
				}},
			}},
		},
	}
	rh.OnUpdate(hp1, hp2)

	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost(hp1.Spec.VirtualHost.Fqdn,
					&envoy_config_route_v3.Route{
						Match:  routePrefix("/"),
						Action: withRetryPolicy(routeCluster("default/backend/80/da39a3ee5e"), "5xx", 0, 105*time.Second),
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	hp3 := &contour_v1.HTTPProxy{
		ObjectMeta: fixture.ObjectMeta("simple"),
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{Fqdn: "test3.test.com"},
			Routes: []contour_v1.Route{{
				RetryPolicy: &contour_v1.RetryPolicy{
					NumRetries:    0,
					PerTryTimeout: "105s",
				},
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
						Action: withRetryPolicy(routeCluster("default/backend/80/da39a3ee5e"), "5xx", 1, 105*time.Second),
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	rh.OnDelete(hp3)
}
