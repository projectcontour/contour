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

// End to ends tests for translator to grpc operations.
package v3

import (
	"testing"

	envoy_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/fixture"
	"github.com/projectcontour/contour/internal/protobuf"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type weightedcluster struct {
	name   string
	weight uint32
}

func TestHTTPProxyRouteWithAServiceWeight(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	rh.OnAdd(fixture.NewService("kuard").
		WithPorts(v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)}))

	proxy1 := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{Fqdn: "test2.test.com"},
			Routes: []contour_api_v1.Route{{
				Conditions: conditions(prefixCondition("/a")),
				Services: []contour_api_v1.Service{{
					Name:   "kuard",
					Port:   80,
					Weight: 90, // ignored
				}},
			}},
		},
	}

	rh.OnAdd(proxy1)
	assertRDS(t, c, "1", virtualhosts(
		envoy_v3.VirtualHost("test2.test.com",
			&envoy_route_v3.Route{
				Match:  routePrefix("/a"),
				Action: routecluster("default/kuard/80/da39a3ee5e"),
			},
		),
	), nil)

	proxy2 := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{Fqdn: "test2.test.com"},
			Routes: []contour_api_v1.Route{{
				Conditions: conditions(prefixCondition("/a")),
				Services: []contour_api_v1.Service{{
					Name:   "kuard",
					Port:   80,
					Weight: 90,
				}, {
					Name:   "kuard",
					Port:   80,
					Weight: 60,
				}},
			}},
		},
	}

	rh.OnUpdate(proxy1, proxy2)
	assertRDS(t, c, "2", virtualhosts(
		envoy_v3.VirtualHost("test2.test.com",
			&envoy_route_v3.Route{
				Match: routePrefix("/a"),
				Action: routeweightedcluster(
					weightedcluster{"default/kuard/80/da39a3ee5e", 60},
					weightedcluster{"default/kuard/80/da39a3ee5e", 90}),
			},
		),
	), nil)
}

func routeweightedcluster(clusters ...weightedcluster) *envoy_route_v3.Route_Route {
	return &envoy_route_v3.Route_Route{
		Route: &envoy_route_v3.RouteAction{
			ClusterSpecifier: &envoy_route_v3.RouteAction_WeightedClusters{
				WeightedClusters: weightedclusters(clusters),
			},
		},
	}
}

func weightedclusters(clusters []weightedcluster) *envoy_route_v3.WeightedCluster {
	var wc envoy_route_v3.WeightedCluster
	var total uint32
	for _, c := range clusters {
		total += c.weight
		wc.Clusters = append(wc.Clusters, &envoy_route_v3.WeightedCluster_ClusterWeight{
			Name:   c.name,
			Weight: protobuf.UInt32(c.weight),
		})
	}
	wc.TotalWeight = protobuf.UInt32(total)
	return &wc
}
