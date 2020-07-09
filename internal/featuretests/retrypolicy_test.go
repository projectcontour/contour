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
	"time"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_api_v2_route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	projcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/envoy"
	v1 "k8s.io/api/core/v1"
	"k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestRetryPolicy(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	s1 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "backend",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}
	rh.OnAdd(s1)

	i1 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "hello",
			Namespace: s1.Namespace,
			Annotations: map[string]string{
				"projectcontour.io/retry-on":         "5xx,gateway-error",
				"contour.heptio.com/num-retries":     "7",
				"contour.heptio.com/per-try-timeout": "120ms",
			},
		},
		Spec: v1beta1.IngressSpec{
			Backend: backend(s1),
		},
	}
	rh.OnAdd(i1)

	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("*",
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/"),
						Action: withRetryPolicy(routeCluster("default/backend/80/da39a3ee5e"), "5xx,gateway-error", 7, 120*time.Millisecond),
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	i2 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name: "hello", Namespace: "default",
			Annotations: map[string]string{
				"projectcontour.io/retry-on":         "5xx,gateway-error",
				"projectcontour.io/num-retries":      "7",
				"contour.heptio.com/per-try-timeout": "120ms",
			},
		},
		Spec: v1beta1.IngressSpec{
			Backend: backend(s1),
		},
	}
	rh.OnUpdate(i1, i2)

	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("*",
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/"),
						Action: withRetryPolicy(routeCluster("default/backend/80/da39a3ee5e"), "5xx,gateway-error", 7, 120*time.Millisecond),
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	i3 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name: "hello", Namespace: "default",
			Annotations: map[string]string{
				"projectcontour.io/retry-on":        "5xx,gateway-error",
				"projectcontour.io/num-retries":     "7",
				"projectcontour.io/per-try-timeout": "120ms",
			},
		},
		Spec: v1beta1.IngressSpec{
			Backend: backend(s1),
		},
	}
	rh.OnUpdate(i2, i3)

	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("*",
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/"),
						Action: withRetryPolicy(routeCluster("default/backend/80/da39a3ee5e"), "5xx,gateway-error", 7, 120*time.Millisecond),
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	rh.OnDelete(i3)

	hp1 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: s1.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{Fqdn: "test3.test.com"},
			Routes: []projcontour.Route{{
				RetryPolicy: &projcontour.RetryPolicy{
					NumRetries:    5,
					PerTryTimeout: "105s",
				},
				Services: []projcontour.Service{{
					Name: s1.Name,
					Port: 80,
				}},
			}},
		},
	}
	rh.OnAdd(hp1)

	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost(hp1.Spec.VirtualHost.Fqdn,
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/"),
						Action: withRetryPolicy(routeCluster("default/backend/80/da39a3ee5e"), "5xx", 5, 105*time.Second),
					},
				),
			),
		),
		TypeUrl: routeType,
	})
}
