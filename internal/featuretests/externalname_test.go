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

package featuretests

import (
	"testing"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_api_v2_route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	projcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/envoy"
	"github.com/projectcontour/contour/internal/fixture"
	v1 "k8s.io/api/core/v1"
	"k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// Assert that services of type v1.ServiceTypeExternalName can be
// referenced by an Ingress, or HTTPProxy document.
func TestExternalNameService(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	s1 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
			ExternalName: "foo.io",
			Type:         v1.ServiceTypeExternalName,
		},
	}

	i1 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: s1.Namespace,
		},
		Spec: v1beta1.IngressSpec{
			Backend: &v1beta1.IngressBackend{
				ServiceName: s1.Name,
				ServicePort: intstr.FromInt(80),
			},
		},
	}
	rh.OnAdd(s1)
	rh.OnAdd(i1)

	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("*",
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/"),
						Action: routeCluster("default/kuard/80/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	c.Request(clusterType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			externalNameCluster("default/kuard/80/da39a3ee5e", "default/kuard/", "default_kuard_80", "foo.io", 80),
		),
		TypeUrl: clusterType,
	})

	rh.OnDelete(i1)

	rh.OnAdd(fixture.NewProxy("kuard").
		WithFQDN("kuard.projectcontour.io").
		WithSpec(projcontour.HTTPProxySpec{
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: s1.Name,
					Port: 80,
				}},
			}},
		}),
	)

	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("kuard.projectcontour.io",
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/"),
						Action: routeCluster("default/kuard/80/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	c.Request(clusterType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			externalNameCluster("default/kuard/80/da39a3ee5e", "default/kuard/", "default_kuard_80", "foo.io", 80),
		),
		TypeUrl: clusterType,
	})

	rh.OnAdd(fixture.NewProxy("kuard").
		WithFQDN("kuard.projectcontour.io").
		WithSpec(projcontour.HTTPProxySpec{
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: s1.Name,
					Port: 80,
				}},
				RequestHeadersPolicy: &projcontour.HeadersPolicy{
					Set: []projcontour.HeaderValue{{
						Name:  "Host",
						Value: "external.address",
					}},
				},
			}},
		}),
	)

	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("kuard.projectcontour.io",
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/"),
						Action: routeHostRewrite("default/kuard/80/da39a3ee5e", "external.address"),
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	c.Request(clusterType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			externalNameCluster("default/kuard/80/da39a3ee5e", "default/kuard/", "default_kuard_80", "foo.io", 80),
		),
		TypeUrl: clusterType,
	})
}
