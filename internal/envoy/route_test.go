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

package envoy

import (
	"testing"
	"time"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_api_v2_core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoy_api_v2_route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/projectcontour/contour/internal/assert"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/protobuf"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestRouteRoute(t *testing.T) {
	s1 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}
	c1 := &dag.Cluster{
		Upstream: &dag.Service{
			Name:        s1.Name,
			Namespace:   s1.Namespace,
			ServicePort: &s1.Spec.Ports[0],
		},
	}
	c2 := &dag.Cluster{
		Upstream: &dag.Service{
			Name:        s1.Name,
			Namespace:   s1.Namespace,
			ServicePort: &s1.Spec.Ports[0],
		},
		LoadBalancerPolicy: "Cookie",
	}

	tests := map[string]struct {
		route *dag.Route
		want  *envoy_api_v2_route.Route_Route
	}{
		"single service": {
			route: &dag.Route{
				Clusters: []*dag.Cluster{c1},
			},
			want: &envoy_api_v2_route.Route_Route{
				Route: &envoy_api_v2_route.RouteAction{
					ClusterSpecifier: &envoy_api_v2_route.RouteAction_Cluster{
						Cluster: "default/kuard/8080/da39a3ee5e",
					},
				},
			},
		},
		"websocket": {
			route: &dag.Route{
				Websocket: true,
				Clusters:  []*dag.Cluster{c1},
			},
			want: &envoy_api_v2_route.Route_Route{
				Route: &envoy_api_v2_route.RouteAction{
					ClusterSpecifier: &envoy_api_v2_route.RouteAction_Cluster{
						Cluster: "default/kuard/8080/da39a3ee5e",
					},
					UpgradeConfigs: []*envoy_api_v2_route.RouteAction_UpgradeConfig{{
						UpgradeType: "websocket",
					}},
				},
			},
		},
		"multiple": {
			route: &dag.Route{
				Clusters: []*dag.Cluster{{
					Upstream: &dag.Service{
						Name:        s1.Name,
						Namespace:   s1.Namespace,
						ServicePort: &s1.Spec.Ports[0],
					},
					Weight: 90,
				}, {
					Upstream: &dag.Service{
						Name:        s1.Name,
						Namespace:   s1.Namespace, // it's valid to mention the same service several times per route.
						ServicePort: &s1.Spec.Ports[0],
					},
				}},
			},
			want: &envoy_api_v2_route.Route_Route{
				Route: &envoy_api_v2_route.RouteAction{
					ClusterSpecifier: &envoy_api_v2_route.RouteAction_WeightedClusters{
						WeightedClusters: &envoy_api_v2_route.WeightedCluster{
							Clusters: []*envoy_api_v2_route.WeightedCluster_ClusterWeight{{
								Name:   "default/kuard/8080/da39a3ee5e",
								Weight: protobuf.UInt32(0),
							}, {
								Name:   "default/kuard/8080/da39a3ee5e",
								Weight: protobuf.UInt32(90),
							}},
							TotalWeight: protobuf.UInt32(90),
						},
					},
				},
			},
		},
		"multiple websocket": {
			route: &dag.Route{
				Websocket: true,
				Clusters: []*dag.Cluster{{
					Upstream: &dag.Service{
						Name:        s1.Name,
						Namespace:   s1.Namespace,
						ServicePort: &s1.Spec.Ports[0],
					},
					Weight: 90,
				}, {
					Upstream: &dag.Service{
						Name:        s1.Name,
						Namespace:   s1.Namespace, // it's valid to mention the same service several times per route.
						ServicePort: &s1.Spec.Ports[0],
					},
				}},
			},
			want: &envoy_api_v2_route.Route_Route{
				Route: &envoy_api_v2_route.RouteAction{
					ClusterSpecifier: &envoy_api_v2_route.RouteAction_WeightedClusters{
						WeightedClusters: &envoy_api_v2_route.WeightedCluster{
							Clusters: []*envoy_api_v2_route.WeightedCluster_ClusterWeight{{
								Name:   "default/kuard/8080/da39a3ee5e",
								Weight: protobuf.UInt32(0),
							}, {
								Name:   "default/kuard/8080/da39a3ee5e",
								Weight: protobuf.UInt32(90),
							}},
							TotalWeight: protobuf.UInt32(90),
						},
					},
					UpgradeConfigs: []*envoy_api_v2_route.RouteAction_UpgradeConfig{{
						UpgradeType: "websocket",
					}},
				},
			},
		},
		"single with header manipulations": {
			route: &dag.Route{
				Websocket: true,
				Clusters: []*dag.Cluster{{
					Upstream: &dag.Service{
						Name:        s1.Name,
						Namespace:   s1.Namespace,
						ServicePort: &s1.Spec.Ports[0],
					},

					RequestHeadersPolicy: &dag.HeadersPolicy{
						Set: map[string]string{
							"K-Foo":   "bar",
							"K-Sauce": "spicy",
						},
						Remove: []string{"K-Bar"},
					},
					ResponseHeadersPolicy: &dag.HeadersPolicy{
						Set: map[string]string{
							"K-Blah": "boo",
						},
						Remove: []string{"K-Baz"},
					},
				}},
			},
			want: &envoy_api_v2_route.Route_Route{
				Route: &envoy_api_v2_route.RouteAction{
					ClusterSpecifier: &envoy_api_v2_route.RouteAction_WeightedClusters{
						WeightedClusters: &envoy_api_v2_route.WeightedCluster{
							Clusters: []*envoy_api_v2_route.WeightedCluster_ClusterWeight{{
								Name:   "default/kuard/8080/da39a3ee5e",
								Weight: protobuf.UInt32(1),
								RequestHeadersToAdd: []*envoy_api_v2_core.HeaderValueOption{{
									Header: &envoy_api_v2_core.HeaderValue{
										Key:   "K-Foo",
										Value: "bar",
									},
									Append: &wrappers.BoolValue{
										Value: false,
									},
								}, {
									Header: &envoy_api_v2_core.HeaderValue{
										Key:   "K-Sauce",
										Value: "spicy",
									},
									Append: &wrappers.BoolValue{
										Value: false,
									},
								}},
								RequestHeadersToRemove: []string{"K-Bar"},
								ResponseHeadersToAdd: []*envoy_api_v2_core.HeaderValueOption{{
									Header: &envoy_api_v2_core.HeaderValue{
										Key:   "K-Blah",
										Value: "boo",
									},
									Append: &wrappers.BoolValue{
										Value: false,
									},
								}},
								ResponseHeadersToRemove: []string{"K-Baz"},
							}},
							TotalWeight: protobuf.UInt32(1),
						},
					},
					UpgradeConfigs: []*envoy_api_v2_route.RouteAction_UpgradeConfig{{
						UpgradeType: "websocket",
					}},
				},
			},
		},
		"single service without retry-on": {
			route: &dag.Route{
				RetryPolicy: &dag.RetryPolicy{
					NumRetries:    7,                // ignored
					PerTryTimeout: 10 * time.Second, // ignored
				},
				Clusters: []*dag.Cluster{c1},
			},
			want: &envoy_api_v2_route.Route_Route{
				Route: &envoy_api_v2_route.RouteAction{
					ClusterSpecifier: &envoy_api_v2_route.RouteAction_Cluster{
						Cluster: "default/kuard/8080/da39a3ee5e",
					},
				},
			},
		},
		"retry-on: 503": {
			route: &dag.Route{
				RetryPolicy: &dag.RetryPolicy{
					RetryOn:       "503",
					NumRetries:    6,
					PerTryTimeout: 100 * time.Millisecond,
				},
				Clusters: []*dag.Cluster{c1},
			},
			want: &envoy_api_v2_route.Route_Route{
				Route: &envoy_api_v2_route.RouteAction{
					ClusterSpecifier: &envoy_api_v2_route.RouteAction_Cluster{
						Cluster: "default/kuard/8080/da39a3ee5e",
					},
					RetryPolicy: &envoy_api_v2_route.RetryPolicy{
						RetryOn:       "503",
						NumRetries:    protobuf.UInt32(6),
						PerTryTimeout: protobuf.Duration(100 * time.Millisecond),
					},
				},
			},
		},
		"retriable status codes: 502, 503, 504": {
			route: &dag.Route{
				RetryPolicy: &dag.RetryPolicy{
					RetryOn:              "retriable-status-codes",
					RetriableStatusCodes: []uint32{503, 503, 504},
					NumRetries:           6,
					PerTryTimeout:        100 * time.Millisecond,
				},
				Clusters: []*dag.Cluster{c1},
			},
			want: &envoy_api_v2_route.Route_Route{
				Route: &envoy_api_v2_route.RouteAction{
					ClusterSpecifier: &envoy_api_v2_route.RouteAction_Cluster{
						Cluster: "default/kuard/8080/da39a3ee5e",
					},
					RetryPolicy: &envoy_api_v2_route.RetryPolicy{
						RetryOn:              "retriable-status-codes",
						RetriableStatusCodes: []uint32{503, 503, 504},
						NumRetries:           protobuf.UInt32(6),
						PerTryTimeout:        protobuf.Duration(100 * time.Millisecond),
					},
				},
			},
		},
		"timeout 90s": {
			route: &dag.Route{
				TimeoutPolicy: &dag.TimeoutPolicy{
					ResponseTimeout: 90 * time.Second,
				},
				Clusters: []*dag.Cluster{c1},
			},
			want: &envoy_api_v2_route.Route_Route{
				Route: &envoy_api_v2_route.RouteAction{
					ClusterSpecifier: &envoy_api_v2_route.RouteAction_Cluster{
						Cluster: "default/kuard/8080/da39a3ee5e",
					},
					Timeout: protobuf.Duration(90 * time.Second),
				},
			},
		},
		"timeout infinity": {
			route: &dag.Route{
				TimeoutPolicy: &dag.TimeoutPolicy{
					ResponseTimeout: -1,
				},
				Clusters: []*dag.Cluster{c1},
			},
			want: &envoy_api_v2_route.Route_Route{
				Route: &envoy_api_v2_route.RouteAction{
					ClusterSpecifier: &envoy_api_v2_route.RouteAction_Cluster{
						Cluster: "default/kuard/8080/da39a3ee5e",
					},
					Timeout: protobuf.Duration(0),
				},
			},
		},
		"idle timeout 10m": {
			route: &dag.Route{
				TimeoutPolicy: &dag.TimeoutPolicy{
					IdleTimeout: 10 * time.Minute,
				},
				Clusters: []*dag.Cluster{c1},
			},
			want: &envoy_api_v2_route.Route_Route{
				Route: &envoy_api_v2_route.RouteAction{
					ClusterSpecifier: &envoy_api_v2_route.RouteAction_Cluster{
						Cluster: "default/kuard/8080/da39a3ee5e",
					},
					IdleTimeout: protobuf.Duration(600 * time.Second),
				},
			},
		},
		"idle timeout infinity": {
			route: &dag.Route{
				TimeoutPolicy: &dag.TimeoutPolicy{
					IdleTimeout: -1,
				},
				Clusters: []*dag.Cluster{c1},
			},
			want: &envoy_api_v2_route.Route_Route{
				Route: &envoy_api_v2_route.RouteAction{
					ClusterSpecifier: &envoy_api_v2_route.RouteAction_Cluster{
						Cluster: "default/kuard/8080/da39a3ee5e",
					},
					IdleTimeout: protobuf.Duration(0),
				},
			},
		},
		"single service w/ session affinity": {
			route: &dag.Route{
				Clusters: []*dag.Cluster{c2},
			},
			want: &envoy_api_v2_route.Route_Route{
				Route: &envoy_api_v2_route.RouteAction{
					ClusterSpecifier: &envoy_api_v2_route.RouteAction_Cluster{
						Cluster: "default/kuard/8080/e4f81994fe",
					},
					HashPolicy: []*envoy_api_v2_route.RouteAction_HashPolicy{{
						PolicySpecifier: &envoy_api_v2_route.RouteAction_HashPolicy_Cookie_{
							Cookie: &envoy_api_v2_route.RouteAction_HashPolicy_Cookie{
								Name: "X-Contour-Session-Affinity",
								Ttl:  protobuf.Duration(0),
								Path: "/",
							},
						},
					}},
				},
			},
		},
		"multiple service w/ session affinity": {
			route: &dag.Route{
				Clusters: []*dag.Cluster{c2, c2},
			},
			want: &envoy_api_v2_route.Route_Route{
				Route: &envoy_api_v2_route.RouteAction{
					ClusterSpecifier: &envoy_api_v2_route.RouteAction_WeightedClusters{
						WeightedClusters: &envoy_api_v2_route.WeightedCluster{
							Clusters: []*envoy_api_v2_route.WeightedCluster_ClusterWeight{{
								Name:   "default/kuard/8080/e4f81994fe",
								Weight: protobuf.UInt32(1),
							}, {
								Name:   "default/kuard/8080/e4f81994fe",
								Weight: protobuf.UInt32(1),
							}},
							TotalWeight: protobuf.UInt32(2),
						},
					},
					HashPolicy: []*envoy_api_v2_route.RouteAction_HashPolicy{{
						PolicySpecifier: &envoy_api_v2_route.RouteAction_HashPolicy_Cookie_{
							Cookie: &envoy_api_v2_route.RouteAction_HashPolicy_Cookie{
								Name: "X-Contour-Session-Affinity",
								Ttl:  protobuf.Duration(0),
								Path: "/",
							},
						},
					}},
				},
			},
		},
		"mixed service w/ session affinity": {
			route: &dag.Route{
				Clusters: []*dag.Cluster{c2, c1},
			},
			want: &envoy_api_v2_route.Route_Route{
				Route: &envoy_api_v2_route.RouteAction{
					ClusterSpecifier: &envoy_api_v2_route.RouteAction_WeightedClusters{
						WeightedClusters: &envoy_api_v2_route.WeightedCluster{
							Clusters: []*envoy_api_v2_route.WeightedCluster_ClusterWeight{{
								Name:   "default/kuard/8080/da39a3ee5e",
								Weight: protobuf.UInt32(1),
							}, {
								Name:   "default/kuard/8080/e4f81994fe",
								Weight: protobuf.UInt32(1),
							}},
							TotalWeight: protobuf.UInt32(2),
						},
					},
					HashPolicy: []*envoy_api_v2_route.RouteAction_HashPolicy{{
						PolicySpecifier: &envoy_api_v2_route.RouteAction_HashPolicy_Cookie_{
							Cookie: &envoy_api_v2_route.RouteAction_HashPolicy_Cookie{
								Name: "X-Contour-Session-Affinity",
								Ttl:  protobuf.Duration(0),
								Path: "/",
							},
						},
					}},
				},
			},
		},
		"host header rewrite": {
			route: &dag.Route{
				RequestHeadersPolicy: &dag.HeadersPolicy{
					HostRewrite: "bar.com",
				},
				Clusters: []*dag.Cluster{c1},
			},
			want: &envoy_api_v2_route.Route_Route{
				Route: &envoy_api_v2_route.RouteAction{
					ClusterSpecifier: &envoy_api_v2_route.RouteAction_Cluster{
						Cluster: "default/kuard/8080/da39a3ee5e",
					},
					HostRewriteSpecifier: &envoy_api_v2_route.RouteAction_HostRewrite{HostRewrite: "bar.com"},
				},
			},
		},
		"mirror": {
			route: &dag.Route{
				Clusters: []*dag.Cluster{{
					Upstream: &dag.Service{
						Name:        s1.Name,
						Namespace:   s1.Namespace,
						ServicePort: &s1.Spec.Ports[0],
					},
					Weight: 90,
				}},
				MirrorPolicy: &dag.MirrorPolicy{
					Cluster: &dag.Cluster{
						Upstream: &dag.Service{
							Name:        s1.Name,
							Namespace:   s1.Namespace,
							ServicePort: &s1.Spec.Ports[0],
						},
					},
				},
			},
			want: &envoy_api_v2_route.Route_Route{
				Route: &envoy_api_v2_route.RouteAction{
					ClusterSpecifier: &envoy_api_v2_route.RouteAction_Cluster{
						Cluster: "default/kuard/8080/da39a3ee5e",
					},
					RequestMirrorPolicies: []*envoy_api_v2_route.RouteAction_RequestMirrorPolicy{{
						Cluster: "default/kuard/8080/da39a3ee5e",
					}},
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := RouteRoute(tc.route)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestWeightedClusters(t *testing.T) {
	tests := map[string]struct {
		clusters []*dag.Cluster
		want     *envoy_api_v2_route.WeightedCluster
	}{
		"multiple services w/o weights": {
			clusters: []*dag.Cluster{{
				Upstream: &dag.Service{
					Name:      "kuard",
					Namespace: "default",
					ServicePort: &v1.ServicePort{
						Port: 8080,
					},
				},
			}, {
				Upstream: &dag.Service{
					Name:      "nginx",
					Namespace: "default",
					ServicePort: &v1.ServicePort{
						Port: 8080,
					},
				},
			}},
			want: &envoy_api_v2_route.WeightedCluster{
				Clusters: []*envoy_api_v2_route.WeightedCluster_ClusterWeight{{
					Name:   "default/kuard/8080/da39a3ee5e",
					Weight: protobuf.UInt32(1),
				}, {
					Name:   "default/nginx/8080/da39a3ee5e",
					Weight: protobuf.UInt32(1),
				}},
				TotalWeight: protobuf.UInt32(2),
			},
		},
		"multiple weighted services": {
			clusters: []*dag.Cluster{{
				Upstream: &dag.Service{
					Name:      "kuard",
					Namespace: "default",
					ServicePort: &v1.ServicePort{
						Port: 8080,
					},
				},
				Weight: 80,
			}, {
				Upstream: &dag.Service{
					Name:      "nginx",
					Namespace: "default",
					ServicePort: &v1.ServicePort{
						Port: 8080,
					},
				},
				Weight: 20,
			}},
			want: &envoy_api_v2_route.WeightedCluster{
				Clusters: []*envoy_api_v2_route.WeightedCluster_ClusterWeight{{
					Name:   "default/kuard/8080/da39a3ee5e",
					Weight: protobuf.UInt32(80),
				}, {
					Name:   "default/nginx/8080/da39a3ee5e",
					Weight: protobuf.UInt32(20),
				}},
				TotalWeight: protobuf.UInt32(100),
			},
		},
		"multiple weighted services and one with no weight specified": {
			clusters: []*dag.Cluster{{
				Upstream: &dag.Service{
					Name:      "kuard",
					Namespace: "default",
					ServicePort: &v1.ServicePort{
						Port: 8080,
					},
				},
				Weight: 80,
			}, {
				Upstream: &dag.Service{
					Name:      "nginx",
					Namespace: "default",
					ServicePort: &v1.ServicePort{
						Port: 8080,
					},
				},
				Weight: 20,
			}, {
				Upstream: &dag.Service{
					Name:      "notraffic",
					Namespace: "default",
					ServicePort: &v1.ServicePort{
						Port: 8080,
					},
				},
			}},
			want: &envoy_api_v2_route.WeightedCluster{
				Clusters: []*envoy_api_v2_route.WeightedCluster_ClusterWeight{{
					Name:   "default/kuard/8080/da39a3ee5e",
					Weight: protobuf.UInt32(80),
				}, {
					Name:   "default/nginx/8080/da39a3ee5e",
					Weight: protobuf.UInt32(20),
				}, {
					Name:   "default/notraffic/8080/da39a3ee5e",
					Weight: protobuf.UInt32(0),
				}},
				TotalWeight: protobuf.UInt32(100),
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := weightedClusters(tc.clusters)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestRouteConfiguration(t *testing.T) {
	tests := map[string]struct {
		name         string
		virtualhosts []*envoy_api_v2_route.VirtualHost
		want         *v2.RouteConfiguration
	}{

		"empty": {
			name: "ingress_http",
			want: &v2.RouteConfiguration{
				Name: "ingress_http",
				RequestHeadersToAdd: []*envoy_api_v2_core.HeaderValueOption{{
					Header: &envoy_api_v2_core.HeaderValue{
						Key:   "x-request-start",
						Value: "t=%START_TIME(%s.%3f)%",
					},
					Append: protobuf.Bool(true),
				}},
			},
		},
		"one virtualhost": {
			name: "ingress_https",
			virtualhosts: virtualhosts(
				VirtualHost("www.example.com"),
			),
			want: &v2.RouteConfiguration{
				Name: "ingress_https",
				VirtualHosts: virtualhosts(
					VirtualHost("www.example.com"),
				),
				RequestHeadersToAdd: []*envoy_api_v2_core.HeaderValueOption{{
					Header: &envoy_api_v2_core.HeaderValue{
						Key:   "x-request-start",
						Value: "t=%START_TIME(%s.%3f)%",
					},
					Append: protobuf.Bool(true),
				}},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := RouteConfiguration(tc.name, tc.virtualhosts...)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestVirtualHost(t *testing.T) {
	tests := map[string]struct {
		hostname string
		port     int
		want     *envoy_api_v2_route.VirtualHost
	}{
		"default hostname": {
			hostname: "*",
			port:     9999,
			want: &envoy_api_v2_route.VirtualHost{
				Name:    "*",
				Domains: []string{"*"},
			},
		},
		"www.example.com": {
			hostname: "www.example.com",
			port:     9999,
			want: &envoy_api_v2_route.VirtualHost{
				Name:    "www.example.com",
				Domains: []string{"www.example.com", "www.example.com:*"},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := VirtualHost(tc.hostname)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestUpgradeHTTPS(t *testing.T) {
	got := UpgradeHTTPS()
	want := &envoy_api_v2_route.Route_Redirect{
		Redirect: &envoy_api_v2_route.RedirectAction{
			SchemeRewriteSpecifier: &envoy_api_v2_route.RedirectAction_HttpsRedirect{
				HttpsRedirect: true,
			},
		},
	}

	assert.Equal(t, want, got)
}

func TestRouteMatch(t *testing.T) {
	tests := map[string]struct {
		route *dag.Route
		want  *envoy_api_v2_route.RouteMatch
	}{
		"contains match with dashes": {
			route: &dag.Route{
				HeaderMatchConditions: []dag.HeaderMatchCondition{{
					Name:      "x-header",
					Value:     "11-22-33-44",
					MatchType: "contains",
					Invert:    false,
				}},
			},
			want: &envoy_api_v2_route.RouteMatch{
				Headers: []*envoy_api_v2_route.HeaderMatcher{{
					Name:        "x-header",
					InvertMatch: false,
					HeaderMatchSpecifier: &envoy_api_v2_route.HeaderMatcher_SafeRegexMatch{
						SafeRegexMatch: SafeRegexMatch(".*11-22-33-44.*"),
					},
				}},
			},
		},
		"contains match with dots": {
			route: &dag.Route{
				HeaderMatchConditions: []dag.HeaderMatchCondition{{
					Name:      "x-header",
					Value:     "11.22.33.44",
					MatchType: "contains",
					Invert:    false,
				}},
			},
			want: &envoy_api_v2_route.RouteMatch{
				Headers: []*envoy_api_v2_route.HeaderMatcher{{
					Name:        "x-header",
					InvertMatch: false,
					HeaderMatchSpecifier: &envoy_api_v2_route.HeaderMatcher_SafeRegexMatch{
						SafeRegexMatch: SafeRegexMatch(".*11\\.22\\.33\\.44.*"),
					},
				}},
			},
		},
		"contains match with regex group": {
			route: &dag.Route{
				HeaderMatchConditions: []dag.HeaderMatchCondition{{
					Name:      "x-header",
					Value:     "11.[22].*33.44",
					MatchType: "contains",
					Invert:    false,
				}},
			},
			want: &envoy_api_v2_route.RouteMatch{
				Headers: []*envoy_api_v2_route.HeaderMatcher{{
					Name:        "x-header",
					InvertMatch: false,
					HeaderMatchSpecifier: &envoy_api_v2_route.HeaderMatcher_SafeRegexMatch{
						SafeRegexMatch: SafeRegexMatch(".*11\\.\\[22\\]\\.\\*33\\.44.*"),
					},
				}},
			},
		},
		"path prefix": {
			route: &dag.Route{
				PathMatchCondition: &dag.PrefixMatchCondition{
					Prefix: "/foo",
				},
			},
			want: &envoy_api_v2_route.RouteMatch{
				PathSpecifier: &envoy_api_v2_route.RouteMatch_Prefix{
					Prefix: "/foo",
				},
			},
		},
		"path regex": {
			route: &dag.Route{
				PathMatchCondition: &dag.RegexMatchCondition{
					Regex: "/v.1/*",
				},
			},
			want: &envoy_api_v2_route.RouteMatch{
				PathSpecifier: &envoy_api_v2_route.RouteMatch_SafeRegex{
					// note, unlike header conditions this is not a quoted regex because
					// the value comes directly from the Ingress.Paths.Path value which
					// is permitted to be a bare regex.
					SafeRegex: SafeRegexMatch("/v.1/*"),
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := RouteMatch(tc.route)
			assert.Equal(t, tc.want, got)
		})
	}
}

func virtualhosts(v ...*envoy_api_v2_route.VirtualHost) []*envoy_api_v2_route.VirtualHost { return v }
