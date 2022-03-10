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

	envoy_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/fixture"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// Session affinity is only available in httpproxy.
func TestLoadBalancerPolicySessionAffinity(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	s1 := fixture.NewService("app").WithPorts(
		v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)},
		v1.ServicePort{Port: 8080, TargetPort: intstr.FromInt(8080)})
	rh.OnAdd(s1)

	// simple single service
	proxy1 := fixture.NewProxy("simple").
		WithFQDN("www.example.com").
		WithSpec(contour_api_v1.HTTPProxySpec{
			Routes: []contour_api_v1.Route{{
				Conditions: matchconditions(prefixMatchCondition("/cart")),
				LoadBalancerPolicy: &contour_api_v1.LoadBalancerPolicy{
					Strategy: "Cookie",
				},
				Services: []contour_api_v1.Service{{
					Name: s1.Name,
					Port: 80,
				}},
			}},
		})
	rh.OnAdd(proxy1)

	c.Request(clusterType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			DefaultCluster(&envoy_cluster_v3.Cluster{
				Name:                 s1.Namespace + "/" + s1.Name + "/80/e4f81994fe",
				ClusterDiscoveryType: envoy_v3.ClusterDiscoveryType(envoy_cluster_v3.Cluster_EDS),
				AltStatName:          s1.Namespace + "_" + s1.Name + "_80",
				EdsClusterConfig: &envoy_cluster_v3.Cluster_EdsClusterConfig{
					EdsConfig:   envoy_v3.ConfigSource("contour"),
					ServiceName: s1.Namespace + "/" + s1.Name,
				},
				LbPolicy: envoy_cluster_v3.Cluster_RING_HASH,
			}),
		),
		TypeUrl: clusterType,
	})

	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("www.example.com",
					&envoy_route_v3.Route{
						Match:  routePrefix("/cart"),
						Action: withSessionAffinity(routeCluster("default/app/80/e4f81994fe")),
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	// two backends
	rh.OnUpdate(
		proxy1,
		fixture.NewProxy("simple").
			WithFQDN("www.example.com").
			WithSpec(contour_api_v1.HTTPProxySpec{
				Routes: []contour_api_v1.Route{{
					Conditions: matchconditions(prefixMatchCondition("/cart")),
					LoadBalancerPolicy: &contour_api_v1.LoadBalancerPolicy{
						Strategy: "Cookie",
					},
					Services: []contour_api_v1.Service{{
						Name: s1.Name,
						Port: 80,
					}, {
						Name: s1.Name,
						Port: 8080,
					}},
				}},
			}),
	)

	c.Request(clusterType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			DefaultCluster(&envoy_cluster_v3.Cluster{
				Name:                 s1.Namespace + "/" + s1.Name + "/80/e4f81994fe",
				ClusterDiscoveryType: envoy_v3.ClusterDiscoveryType(envoy_cluster_v3.Cluster_EDS),
				AltStatName:          s1.Namespace + "_" + s1.Name + "_80",
				EdsClusterConfig: &envoy_cluster_v3.Cluster_EdsClusterConfig{
					EdsConfig:   envoy_v3.ConfigSource("contour"),
					ServiceName: s1.Namespace + "/" + s1.Name,
				},
				LbPolicy: envoy_cluster_v3.Cluster_RING_HASH,
			}),
			DefaultCluster(&envoy_cluster_v3.Cluster{
				Name:                 s1.Namespace + "/" + s1.Name + "/8080/e4f81994fe",
				ClusterDiscoveryType: envoy_v3.ClusterDiscoveryType(envoy_cluster_v3.Cluster_EDS),
				AltStatName:          s1.Namespace + "_" + s1.Name + "_8080",
				EdsClusterConfig: &envoy_cluster_v3.Cluster_EdsClusterConfig{
					EdsConfig:   envoy_v3.ConfigSource("contour"),
					ServiceName: s1.Namespace + "/" + s1.Name,
				},
				LbPolicy: envoy_cluster_v3.Cluster_RING_HASH,
			}),
		),
		TypeUrl: clusterType,
	})

	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("www.example.com",
					&envoy_route_v3.Route{
						Match: routePrefix("/cart"),
						Action: withSessionAffinity(
							routeWeightedCluster(
								weightedCluster{"default/app/80/e4f81994fe", 1},
								weightedCluster{"default/app/8080/e4f81994fe", 1},
							),
						),
					},
				),
			),
		),
		TypeUrl: routeType,
	})
}

// Request hash load balancing is only available in httpproxy.
func TestLoadBalancerPolicyRequestHashHeader(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	s1 := fixture.NewService("app").WithPorts(
		v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)},
		v1.ServicePort{Port: 8080, TargetPort: intstr.FromInt(8080)})
	rh.OnAdd(s1)

	proxy1 := fixture.NewProxy("simple").
		WithFQDN("www.example.com").
		WithSpec(contour_api_v1.HTTPProxySpec{
			Routes: []contour_api_v1.Route{{
				Conditions: matchconditions(prefixMatchCondition("/cart")),
				LoadBalancerPolicy: &contour_api_v1.LoadBalancerPolicy{
					Strategy: "RequestHash",
					RequestHashPolicies: []contour_api_v1.RequestHashPolicy{
						{
							Terminal: true,
							HeaderHashOptions: &contour_api_v1.HeaderHashOptions{
								HeaderName: "X-Some-Header",
							},
						},
						{
							HeaderHashOptions: &contour_api_v1.HeaderHashOptions{
								HeaderName: "X-Some-Other-Header",
							},
						},
					},
				},
				Services: []contour_api_v1.Service{{
					Name: s1.Name,
					Port: 80,
				}},
			}},
		})
	rh.OnAdd(proxy1)

	c.Request(clusterType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			DefaultCluster(&envoy_cluster_v3.Cluster{
				Name:                 s1.Namespace + "/" + s1.Name + "/80/1a2ffc1fef",
				ClusterDiscoveryType: envoy_v3.ClusterDiscoveryType(envoy_cluster_v3.Cluster_EDS),
				AltStatName:          s1.Namespace + "_" + s1.Name + "_80",
				EdsClusterConfig: &envoy_cluster_v3.Cluster_EdsClusterConfig{
					EdsConfig:   envoy_v3.ConfigSource("contour"),
					ServiceName: s1.Namespace + "/" + s1.Name,
				},
				LbPolicy: envoy_cluster_v3.Cluster_RING_HASH,
			}),
		),
		TypeUrl: clusterType,
	})

	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("www.example.com",
					&envoy_route_v3.Route{
						Match: routePrefix("/cart"),
						Action: withRequestHashPolicySpecifiers(
							routeCluster("default/app/80/1a2ffc1fef"),
							hashPolicySpecifier{headerName: "X-Some-Header", terminal: true},
							hashPolicySpecifier{headerName: "X-Some-Other-Header"},
						),
					},
				),
			),
		),
		TypeUrl: routeType,
	})
}

func TestLoadBalancerPolicyRequestHashSourceIP(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	s1 := fixture.NewService("app").WithPorts(
		v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)},
		v1.ServicePort{Port: 8080, TargetPort: intstr.FromInt(8080)})
	rh.OnAdd(s1)

	proxy1 := fixture.NewProxy("simple").
		WithFQDN("www.example.com").
		WithSpec(contour_api_v1.HTTPProxySpec{
			Routes: []contour_api_v1.Route{{
				Conditions: matchconditions(prefixMatchCondition("/cart")),
				LoadBalancerPolicy: &contour_api_v1.LoadBalancerPolicy{
					Strategy: "RequestHash",
					RequestHashPolicies: []contour_api_v1.RequestHashPolicy{
						{
							HeaderHashOptions: &contour_api_v1.HeaderHashOptions{
								HeaderName: "X-Some-Header",
							},
						},
						{
							HashSourceIP: true,
						},
					},
				},
				Services: []contour_api_v1.Service{{
					Name: s1.Name,
					Port: 80,
				}},
			}},
		})
	rh.OnAdd(proxy1)

	c.Request(clusterType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			DefaultCluster(&envoy_cluster_v3.Cluster{
				Name:                 s1.Namespace + "/" + s1.Name + "/80/1a2ffc1fef",
				ClusterDiscoveryType: envoy_v3.ClusterDiscoveryType(envoy_cluster_v3.Cluster_EDS),
				AltStatName:          s1.Namespace + "_" + s1.Name + "_80",
				EdsClusterConfig: &envoy_cluster_v3.Cluster_EdsClusterConfig{
					EdsConfig:   envoy_v3.ConfigSource("contour"),
					ServiceName: s1.Namespace + "/" + s1.Name,
				},
				LbPolicy: envoy_cluster_v3.Cluster_RING_HASH,
			}),
		),
		TypeUrl: clusterType,
	})

	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("www.example.com",
					&envoy_route_v3.Route{
						Match: routePrefix("/cart"),
						Action: withRequestHashPolicySpecifiers(
							routeCluster("default/app/80/1a2ffc1fef"),
							hashPolicySpecifier{headerName: "X-Some-Header"},
							hashPolicySpecifier{hashSourceIP: true},
						),
					},
				),
			),
		),
		TypeUrl: routeType,
	})
}

func TestLoadBalancerPolicyRequestHashQueryParameter(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	s1 := fixture.NewService("app").WithPorts(
		v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)},
		v1.ServicePort{Port: 8080, TargetPort: intstr.FromInt(8080)})
	rh.OnAdd(s1)

	proxy1 := fixture.NewProxy("simple").
		WithFQDN("www.example.com").
		WithSpec(contour_api_v1.HTTPProxySpec{
			Routes: []contour_api_v1.Route{{
				Conditions: matchconditions(prefixMatchCondition("/cart")),
				LoadBalancerPolicy: &contour_api_v1.LoadBalancerPolicy{
					Strategy: "RequestHash",
					RequestHashPolicies: []contour_api_v1.RequestHashPolicy{
						{
							Terminal: true,
							QueryParameterHashOptions: &contour_api_v1.QueryParameterHashOptions{
								ParameterName: "something",
							},
						},
						{
							QueryParameterHashOptions: &contour_api_v1.QueryParameterHashOptions{
								ParameterName: "other",
							},
						},
					},
				},
				Services: []contour_api_v1.Service{{
					Name: s1.Name,
					Port: 80,
				}},
			}},
		})
	rh.OnAdd(proxy1)

	c.Request(clusterType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			DefaultCluster(&envoy_cluster_v3.Cluster{
				Name:                 s1.Namespace + "/" + s1.Name + "/80/1a2ffc1fef",
				ClusterDiscoveryType: envoy_v3.ClusterDiscoveryType(envoy_cluster_v3.Cluster_EDS),
				AltStatName:          s1.Namespace + "_" + s1.Name + "_80",
				EdsClusterConfig: &envoy_cluster_v3.Cluster_EdsClusterConfig{
					EdsConfig:   envoy_v3.ConfigSource("contour"),
					ServiceName: s1.Namespace + "/" + s1.Name,
				},
				LbPolicy: envoy_cluster_v3.Cluster_RING_HASH,
			}),
		),
		TypeUrl: clusterType,
	})

	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("www.example.com",
					&envoy_route_v3.Route{
						Match: routePrefix("/cart"),
						Action: withRequestHashPolicySpecifiers(
							routeCluster("default/app/80/1a2ffc1fef"),
							hashPolicySpecifier{parameterName: "something", terminal: true},
							hashPolicySpecifier{parameterName: "other"},
						),
					},
				),
			),
		),
		TypeUrl: routeType,
	})
}
