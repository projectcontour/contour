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

func TestTimeoutPolicyRequestTimeout(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	svc := fixture.NewService("kuard").
		WithPorts(core_v1.ServicePort{Port: 8080, TargetPort: intstr.FromInt(8080)})
	rh.OnAdd(svc)

	i1 := &networking_v1.Ingress{
		ObjectMeta: fixture.ObjectMetaWithAnnotations("kuard-ing", map[string]string{
			"projectcontour.io/response-timeout": "1m20s",
		}),
		Spec: networking_v1.IngressSpec{
			DefaultBackend: featuretests.IngressBackend(svc),
		},
	}
	rh.OnAdd(i1)

	// check annotation with explicit timeout is propagated
	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("*",
					&envoy_config_route_v3.Route{
						Match:  routePrefix("/"),
						Action: withResponseTimeout(routeCluster("default/kuard/8080/da39a3ee5e"), 80*time.Second),
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	i2 := &networking_v1.Ingress{
		ObjectMeta: fixture.ObjectMetaWithAnnotations("kuard-ing", map[string]string{
			"projectcontour.io/response-timeout": "infinity",
		}),
		Spec: i1.Spec,
	}
	rh.OnUpdate(i1, i2)

	// check annotation with infinite timeout is propagated
	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("*",
					&envoy_config_route_v3.Route{
						Match:  routePrefix("/"),
						Action: withResponseTimeout(routeCluster("default/kuard/8080/da39a3ee5e"), 0), // zero means infinity
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	i3 := &networking_v1.Ingress{
		ObjectMeta: fixture.ObjectMetaWithAnnotations("kuard-ing", map[string]string{
			"projectcontour.io/response-timeout": "monday",
		}),
		Spec: i2.Spec,
	}
	rh.OnUpdate(i2, i3)

	// check annotation with malformed timeout is not propagated
	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("*",
					&envoy_config_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routeCluster("default/kuard/8080/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	i4 := &networking_v1.Ingress{
		ObjectMeta: fixture.ObjectMetaWithAnnotations("kuard-ing", map[string]string{
			"projectcontour.io/request-timeout":  "90s",
			"projectcontour.io/response-timeout": "99s",
		}),
		Spec: i2.Spec,
	}
	rh.OnUpdate(i3, i4)

	// assert that projectcontour.io/response-timeout takes priority.
	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("*",
					&envoy_config_route_v3.Route{
						Match:  routePrefix("/"),
						Action: withResponseTimeout(routeCluster("default/kuard/8080/da39a3ee5e"), 99*time.Second),
					},
				),
			),
		),
		TypeUrl: routeType,
	})
	rh.OnDelete(i4)

	p1 := httpProxyWithTimoutPolicy(svc, &contour_v1.TimeoutPolicy{Response: "600"}) // not 600s
	rh.OnAdd(p1)

	// check timeout policy with malformed response timeout is not propagated
	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http"),
		),
		TypeUrl: routeType,
	})

	p2 := httpProxyWithTimoutPolicy(svc, &contour_v1.TimeoutPolicy{Response: "3m"})
	rh.OnUpdate(p1, p2)

	// check timeout policy with response timeout is propagated correctly
	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("test2.test.com",
					&envoy_config_route_v3.Route{
						Match:  routePrefix("/"),
						Action: withResponseTimeout(routeCluster("default/kuard/8080/da39a3ee5e"), 180*time.Second),
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	p3 := httpProxyWithTimoutPolicy(svc, &contour_v1.TimeoutPolicy{Response: "infinity"})
	rh.OnUpdate(p2, p3)

	// check timeout policy with explicit infine response timeout is propagated as infinity
	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("test2.test.com",
					&envoy_config_route_v3.Route{
						Match:  routePrefix("/"),
						Action: withResponseTimeout(routeCluster("default/kuard/8080/da39a3ee5e"), 0), // zero means infinity
					},
				),
			),
		),
		TypeUrl: routeType,
	})
}

func TestTimeoutPolicyIdleStreamTimeout(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	svc := fixture.NewService("kuard").
		WithPorts(core_v1.ServicePort{Port: 8080, TargetPort: intstr.FromInt(8080)})
	rh.OnAdd(svc)
	p1 := httpProxyWithTimoutPolicy(svc, &contour_v1.TimeoutPolicy{Idle: "600"}) // not 600s
	rh.OnAdd(p1)

	// check timeout policy with malformed response timeout is not propagated
	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http"),
		),
		TypeUrl: routeType,
	})

	p2 := httpProxyWithTimoutPolicy(svc, &contour_v1.TimeoutPolicy{Idle: "3m"})
	rh.OnUpdate(p1, p2)

	// check timeout policy with response timeout is propagated correctly
	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("test2.test.com",
					&envoy_config_route_v3.Route{
						Match:  routePrefix("/"),
						Action: withIdleTimeout(routeCluster("default/kuard/8080/da39a3ee5e"), 180*time.Second),
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	p3 := httpProxyWithTimoutPolicy(svc, &contour_v1.TimeoutPolicy{Idle: "infinity"})
	rh.OnUpdate(p2, p3)

	// check timeout policy with explicit infine response timeout is propagated as infinity
	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("test2.test.com",
					&envoy_config_route_v3.Route{
						Match:  routePrefix("/"),
						Action: withIdleTimeout(routeCluster("default/kuard/8080/da39a3ee5e"), 0), // zero means infinity
					},
				),
			),
		),
		TypeUrl: routeType,
	})
}

func TestTimeoutPolicyIdleConnectionTimeout(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	svc := fixture.NewService("kuard").WithPorts(core_v1.ServicePort{Port: 8080, TargetPort: intstr.FromInt(8080)})
	rh.OnAdd(svc)

	p1 := httpProxyWithTimoutPolicy(svc, &contour_v1.TimeoutPolicy{IdleConnection: "invalid"})
	rh.OnAdd(p1)

	// Check that cluster was not created with invalid input.
	c.Request(clusterType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: nil,
		TypeUrl:   clusterType,
	})

	p2 := httpProxyWithTimoutPolicy(svc, &contour_v1.TimeoutPolicy{IdleConnection: "3m"})
	rh.OnUpdate(p1, p2)

	// Check that cluster has connection timeout set.
	c.Request(clusterType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t, withConnectionTimeout(cluster("default/kuard/8080/b7427dbbf9", "default/kuard", "default_kuard_8080"), 3*time.Minute, envoy_v3.HTTPVersion1)),
		TypeUrl:   clusterType,
	})

	p3 := httpProxyWithTimoutPolicy(svc, &contour_v1.TimeoutPolicy{IdleConnection: "infinite"})
	rh.OnUpdate(p2, p3)

	// Check that cluster has connection timeout set to zero (infinite).
	c.Request(clusterType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t, withConnectionTimeout(cluster("default/kuard/8080/97705cb30a", "default/kuard", "default_kuard_8080"), 0, envoy_v3.HTTPVersion1)),
		TypeUrl:   clusterType,
	})
}

func httpProxyWithTimoutPolicy(svc *core_v1.Service, tp *contour_v1.TimeoutPolicy) *contour_v1.HTTPProxy {
	return &contour_v1.HTTPProxy{
		ObjectMeta: fixture.ObjectMeta("simple"),
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{Fqdn: "test2.test.com"},
			Routes: []contour_v1.Route{{
				Conditions:    matchconditions(prefixMatchCondition("/")),
				TimeoutPolicy: tp,
				Services: []contour_v1.Service{{
					Name: svc.Name,
					Port: 8080,
				}},
			}},
		},
	}
}
