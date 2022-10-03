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

	envoy_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	matcher "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	wrappers "github.com/golang/protobuf/ptypes/wrappers"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/fixture"
	"github.com/projectcontour/contour/internal/protobuf"
	"github.com/projectcontour/contour/internal/timeout"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestRouteRoute(t *testing.T) {
	s1 := fixture.NewService("kuard").
		WithPorts(v1.ServicePort{Name: "http", Port: 8080, TargetPort: intstr.FromInt(8080)})
	c1 := &dag.Cluster{
		Upstream: &dag.Service{
			Weighted: dag.WeightedService{
				Weight:           1,
				ServiceName:      s1.Name,
				ServiceNamespace: s1.Namespace,
				ServicePort:      s1.Spec.Ports[0],
			},
		},
	}
	c2 := &dag.Cluster{
		Upstream: &dag.Service{
			Weighted: dag.WeightedService{
				Weight:           1,
				ServiceName:      s1.Name,
				ServiceNamespace: s1.Namespace,
				ServicePort:      s1.Spec.Ports[0],
			},
		},
		LoadBalancerPolicy: "Cookie",
	}

	c3 := &dag.Cluster{
		Upstream: &dag.Service{
			Weighted: dag.WeightedService{
				Weight:           1,
				ServiceName:      s1.Name,
				ServiceNamespace: s1.Namespace,
				ServicePort:      s1.Spec.Ports[0],
			},
		},
		LoadBalancerPolicy: "RequestHash",
	}

	tests := map[string]struct {
		route *dag.Route
		want  *envoy_route_v3.Route_Route
	}{
		"single service": {
			route: &dag.Route{
				Clusters: []*dag.Cluster{c1},
			},
			want: &envoy_route_v3.Route_Route{
				Route: &envoy_route_v3.RouteAction{
					ClusterSpecifier: &envoy_route_v3.RouteAction_Cluster{
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
			want: &envoy_route_v3.Route_Route{
				Route: &envoy_route_v3.RouteAction{
					ClusterSpecifier: &envoy_route_v3.RouteAction_Cluster{
						Cluster: "default/kuard/8080/da39a3ee5e",
					},
					UpgradeConfigs: []*envoy_route_v3.RouteAction_UpgradeConfig{{
						UpgradeType: "websocket",
					}},
				},
			},
		},
		"multiple": {
			route: &dag.Route{
				Clusters: []*dag.Cluster{{
					Upstream: &dag.Service{
						Weighted: dag.WeightedService{
							Weight:           1,
							ServiceName:      s1.Name,
							ServiceNamespace: s1.Namespace,
							ServicePort:      s1.Spec.Ports[0],
						},
					},
					Weight: 90,
				}, {
					Upstream: &dag.Service{
						Weighted: dag.WeightedService{
							Weight:           1,
							ServiceName:      s1.Name,
							ServiceNamespace: s1.Namespace, // it's valid to mention the same service several times per route.
							ServicePort:      s1.Spec.Ports[0],
						},
					},
				}},
			},
			want: &envoy_route_v3.Route_Route{
				Route: &envoy_route_v3.RouteAction{
					ClusterSpecifier: &envoy_route_v3.RouteAction_WeightedClusters{
						WeightedClusters: &envoy_route_v3.WeightedCluster{
							Clusters: []*envoy_route_v3.WeightedCluster_ClusterWeight{{
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
						Weighted: dag.WeightedService{
							Weight:           1,
							ServiceName:      s1.Name,
							ServiceNamespace: s1.Namespace,
							ServicePort:      s1.Spec.Ports[0],
						},
					},

					Weight: 90,
				}, {
					Upstream: &dag.Service{
						Weighted: dag.WeightedService{
							Weight:           1,
							ServiceName:      s1.Name,
							ServiceNamespace: s1.Namespace,
							ServicePort:      s1.Spec.Ports[0],
						},
					},
				}},
			},
			want: &envoy_route_v3.Route_Route{
				Route: &envoy_route_v3.RouteAction{
					ClusterSpecifier: &envoy_route_v3.RouteAction_WeightedClusters{
						WeightedClusters: &envoy_route_v3.WeightedCluster{
							Clusters: []*envoy_route_v3.WeightedCluster_ClusterWeight{{
								Name:   "default/kuard/8080/da39a3ee5e",
								Weight: protobuf.UInt32(0),
							}, {
								Name:   "default/kuard/8080/da39a3ee5e",
								Weight: protobuf.UInt32(90),
							}},
							TotalWeight: protobuf.UInt32(90),
						},
					},
					UpgradeConfigs: []*envoy_route_v3.RouteAction_UpgradeConfig{{
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
						Weighted: dag.WeightedService{
							Weight:           1,
							ServiceName:      s1.Name,
							ServiceNamespace: s1.Namespace,
							ServicePort:      s1.Spec.Ports[0],
						},
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
			want: &envoy_route_v3.Route_Route{
				Route: &envoy_route_v3.RouteAction{
					ClusterSpecifier: &envoy_route_v3.RouteAction_WeightedClusters{
						WeightedClusters: &envoy_route_v3.WeightedCluster{
							Clusters: []*envoy_route_v3.WeightedCluster_ClusterWeight{{
								Name:   "default/kuard/8080/da39a3ee5e",
								Weight: protobuf.UInt32(1),
								RequestHeadersToAdd: []*envoy_core_v3.HeaderValueOption{{
									Header: &envoy_core_v3.HeaderValue{
										Key:   "K-Foo",
										Value: "bar",
									},
									Append: &wrappers.BoolValue{
										Value: false,
									},
								}, {
									Header: &envoy_core_v3.HeaderValue{
										Key:   "K-Sauce",
										Value: "spicy",
									},
									Append: &wrappers.BoolValue{
										Value: false,
									},
								}},
								RequestHeadersToRemove: []string{"K-Bar"},
								ResponseHeadersToAdd: []*envoy_core_v3.HeaderValueOption{{
									Header: &envoy_core_v3.HeaderValue{
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
					UpgradeConfigs: []*envoy_route_v3.RouteAction_UpgradeConfig{{
						UpgradeType: "websocket",
					}},
				},
			},
		},
		"single service without retry-on": {
			route: &dag.Route{
				RetryPolicy: &dag.RetryPolicy{
					NumRetries:    7,                                         // ignored
					PerTryTimeout: timeout.DurationSetting(10 * time.Second), // ignored
				},
				Clusters: []*dag.Cluster{c1},
			},
			want: &envoy_route_v3.Route_Route{
				Route: &envoy_route_v3.RouteAction{
					ClusterSpecifier: &envoy_route_v3.RouteAction_Cluster{
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
					PerTryTimeout: timeout.DurationSetting(100 * time.Millisecond),
				},
				Clusters: []*dag.Cluster{c1},
			},
			want: &envoy_route_v3.Route_Route{
				Route: &envoy_route_v3.RouteAction{
					ClusterSpecifier: &envoy_route_v3.RouteAction_Cluster{
						Cluster: "default/kuard/8080/da39a3ee5e",
					},
					RetryPolicy: &envoy_route_v3.RetryPolicy{
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
					PerTryTimeout:        timeout.DurationSetting(100 * time.Millisecond),
				},
				Clusters: []*dag.Cluster{c1},
			},
			want: &envoy_route_v3.Route_Route{
				Route: &envoy_route_v3.RouteAction{
					ClusterSpecifier: &envoy_route_v3.RouteAction_Cluster{
						Cluster: "default/kuard/8080/da39a3ee5e",
					},
					RetryPolicy: &envoy_route_v3.RetryPolicy{
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
				TimeoutPolicy: dag.RouteTimeoutPolicy{
					ResponseTimeout: timeout.DurationSetting(90 * time.Second),
				},
				Clusters: []*dag.Cluster{c1},
			},
			want: &envoy_route_v3.Route_Route{
				Route: &envoy_route_v3.RouteAction{
					ClusterSpecifier: &envoy_route_v3.RouteAction_Cluster{
						Cluster: "default/kuard/8080/da39a3ee5e",
					},
					Timeout: protobuf.Duration(90 * time.Second),
				},
			},
		},
		"timeout infinity": {
			route: &dag.Route{
				TimeoutPolicy: dag.RouteTimeoutPolicy{
					ResponseTimeout: timeout.DisabledSetting(),
				},
				Clusters: []*dag.Cluster{c1},
			},
			want: &envoy_route_v3.Route_Route{
				Route: &envoy_route_v3.RouteAction{
					ClusterSpecifier: &envoy_route_v3.RouteAction_Cluster{
						Cluster: "default/kuard/8080/da39a3ee5e",
					},
					Timeout: protobuf.Duration(0),
				},
			},
		},
		"idle timeout 10m": {
			route: &dag.Route{
				TimeoutPolicy: dag.RouteTimeoutPolicy{
					IdleStreamTimeout: timeout.DurationSetting(10 * time.Minute),
				},
				Clusters: []*dag.Cluster{c1},
			},
			want: &envoy_route_v3.Route_Route{
				Route: &envoy_route_v3.RouteAction{
					ClusterSpecifier: &envoy_route_v3.RouteAction_Cluster{
						Cluster: "default/kuard/8080/da39a3ee5e",
					},
					IdleTimeout: protobuf.Duration(600 * time.Second),
				},
			},
		},
		"idle timeout infinity": {
			route: &dag.Route{
				TimeoutPolicy: dag.RouteTimeoutPolicy{
					IdleStreamTimeout: timeout.DisabledSetting(),
				},
				Clusters: []*dag.Cluster{c1},
			},
			want: &envoy_route_v3.Route_Route{
				Route: &envoy_route_v3.RouteAction{
					ClusterSpecifier: &envoy_route_v3.RouteAction_Cluster{
						Cluster: "default/kuard/8080/da39a3ee5e",
					},
					IdleTimeout: protobuf.Duration(0),
				},
			},
		},
		"single service w/ a cookie hash policy (session affinity)": {
			route: &dag.Route{
				Clusters: []*dag.Cluster{c2},
				RequestHashPolicies: []dag.RequestHashPolicy{
					{CookieHashOptions: &dag.CookieHashOptions{
						CookieName: "X-Contour-Session-Affinity",
						TTL:        time.Duration(0),
						Path:       "/",
					}},
				},
			},
			want: &envoy_route_v3.Route_Route{
				Route: &envoy_route_v3.RouteAction{
					ClusterSpecifier: &envoy_route_v3.RouteAction_Cluster{
						Cluster: "default/kuard/8080/e4f81994fe",
					},
					HashPolicy: []*envoy_route_v3.RouteAction_HashPolicy{{
						PolicySpecifier: &envoy_route_v3.RouteAction_HashPolicy_Cookie_{
							Cookie: &envoy_route_v3.RouteAction_HashPolicy_Cookie{
								Name: "X-Contour-Session-Affinity",
								Ttl:  protobuf.Duration(0),
								Path: "/",
							},
						},
					}},
				},
			},
		},
		"multiple services w/ a cookie hash policy (session affinity)": {
			route: &dag.Route{
				Clusters: []*dag.Cluster{c2, c2},
				RequestHashPolicies: []dag.RequestHashPolicy{
					{CookieHashOptions: &dag.CookieHashOptions{
						CookieName: "X-Contour-Session-Affinity",
						TTL:        time.Duration(0),
						Path:       "/",
					}},
				},
			},
			want: &envoy_route_v3.Route_Route{
				Route: &envoy_route_v3.RouteAction{
					ClusterSpecifier: &envoy_route_v3.RouteAction_WeightedClusters{
						WeightedClusters: &envoy_route_v3.WeightedCluster{
							Clusters: []*envoy_route_v3.WeightedCluster_ClusterWeight{{
								Name:   "default/kuard/8080/e4f81994fe",
								Weight: protobuf.UInt32(1),
							}, {
								Name:   "default/kuard/8080/e4f81994fe",
								Weight: protobuf.UInt32(1),
							}},
							TotalWeight: protobuf.UInt32(2),
						},
					},
					HashPolicy: []*envoy_route_v3.RouteAction_HashPolicy{{
						PolicySpecifier: &envoy_route_v3.RouteAction_HashPolicy_Cookie_{
							Cookie: &envoy_route_v3.RouteAction_HashPolicy_Cookie{
								Name: "X-Contour-Session-Affinity",
								Ttl:  protobuf.Duration(0),
								Path: "/",
							},
						},
					}},
				},
			},
		},
		"single service w/ request header hashing": {
			route: &dag.Route{
				Clusters: []*dag.Cluster{c3},
				RequestHashPolicies: []dag.RequestHashPolicy{
					{
						Terminal: true,
						HeaderHashOptions: &dag.HeaderHashOptions{
							HeaderName: "X-Some-Header",
						},
					},
					{
						HeaderHashOptions: &dag.HeaderHashOptions{
							HeaderName: "User-Agent",
						},
					},
				},
			},
			want: &envoy_route_v3.Route_Route{
				Route: &envoy_route_v3.RouteAction{
					ClusterSpecifier: &envoy_route_v3.RouteAction_Cluster{
						Cluster: "default/kuard/8080/1a2ffc1fef",
					},
					HashPolicy: []*envoy_route_v3.RouteAction_HashPolicy{
						{
							Terminal: true,
							PolicySpecifier: &envoy_route_v3.RouteAction_HashPolicy_Header_{
								Header: &envoy_route_v3.RouteAction_HashPolicy_Header{
									HeaderName: "X-Some-Header",
								},
							},
						},
						{
							PolicySpecifier: &envoy_route_v3.RouteAction_HashPolicy_Header_{
								Header: &envoy_route_v3.RouteAction_HashPolicy_Header{
									HeaderName: "User-Agent",
								},
							},
						},
					},
				},
			},
		},
		"single service w/ request source ip hashing": {
			route: &dag.Route{
				Clusters: []*dag.Cluster{c3},
				RequestHashPolicies: []dag.RequestHashPolicy{
					{
						HashSourceIP: true,
					},
					{
						HeaderHashOptions: &dag.HeaderHashOptions{
							HeaderName: "User-Agent",
						},
					},
				},
			},
			want: &envoy_route_v3.Route_Route{
				Route: &envoy_route_v3.RouteAction{
					ClusterSpecifier: &envoy_route_v3.RouteAction_Cluster{
						Cluster: "default/kuard/8080/1a2ffc1fef",
					},
					HashPolicy: []*envoy_route_v3.RouteAction_HashPolicy{
						{
							PolicySpecifier: &envoy_route_v3.RouteAction_HashPolicy_ConnectionProperties_{
								ConnectionProperties: &envoy_route_v3.RouteAction_HashPolicy_ConnectionProperties{
									SourceIp: true,
								},
							},
						},
						{
							PolicySpecifier: &envoy_route_v3.RouteAction_HashPolicy_Header_{
								Header: &envoy_route_v3.RouteAction_HashPolicy_Header{
									HeaderName: "User-Agent",
								},
							},
						},
					},
				},
			},
		},
		"single service w/ request query parameter hashing": {
			route: &dag.Route{
				Clusters: []*dag.Cluster{c3},
				RequestHashPolicies: []dag.RequestHashPolicy{
					{
						Terminal: true,
						QueryParameterHashOptions: &dag.QueryParameterHashOptions{
							ParameterName: "something",
						},
					},
					{
						QueryParameterHashOptions: &dag.QueryParameterHashOptions{
							ParameterName: "other",
						},
					},
				},
			},
			want: &envoy_route_v3.Route_Route{
				Route: &envoy_route_v3.RouteAction{
					ClusterSpecifier: &envoy_route_v3.RouteAction_Cluster{
						Cluster: "default/kuard/8080/1a2ffc1fef",
					},
					HashPolicy: []*envoy_route_v3.RouteAction_HashPolicy{
						{
							Terminal: true,
							PolicySpecifier: &envoy_route_v3.RouteAction_HashPolicy_QueryParameter_{
								QueryParameter: &envoy_route_v3.RouteAction_HashPolicy_QueryParameter{
									Name: "something",
								},
							},
						},
						{
							PolicySpecifier: &envoy_route_v3.RouteAction_HashPolicy_QueryParameter_{
								QueryParameter: &envoy_route_v3.RouteAction_HashPolicy_QueryParameter{
									Name: "other",
								},
							},
						},
					},
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
			want: &envoy_route_v3.Route_Route{
				Route: &envoy_route_v3.RouteAction{
					ClusterSpecifier: &envoy_route_v3.RouteAction_Cluster{
						Cluster: "default/kuard/8080/da39a3ee5e",
					},
					HostRewriteSpecifier: &envoy_route_v3.RouteAction_HostRewriteLiteral{HostRewriteLiteral: "bar.com"},
				},
			},
		},
		"mirror": {
			route: &dag.Route{
				Clusters: []*dag.Cluster{{
					Upstream: &dag.Service{
						Weighted: dag.WeightedService{
							Weight:           1,
							ServiceName:      s1.Name,
							ServiceNamespace: s1.Namespace,
							ServicePort:      s1.Spec.Ports[0],
						},
					},
					Weight: 90,
				}},
				MirrorPolicy: &dag.MirrorPolicy{
					Cluster: &dag.Cluster{
						Upstream: &dag.Service{
							Weighted: dag.WeightedService{
								Weight:           1,
								ServiceName:      s1.Name,
								ServiceNamespace: s1.Namespace,
								ServicePort:      s1.Spec.Ports[0],
							},
						},
					},
				},
			},
			want: &envoy_route_v3.Route_Route{
				Route: &envoy_route_v3.RouteAction{
					ClusterSpecifier: &envoy_route_v3.RouteAction_Cluster{
						Cluster: "default/kuard/8080/da39a3ee5e",
					},
					RequestMirrorPolicies: []*envoy_route_v3.RouteAction_RequestMirrorPolicy{{
						Cluster: "default/kuard/8080/da39a3ee5e",
					}},
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := routeRoute(tc.route)
			protobuf.ExpectEqual(t, tc.want, got)
		})
	}
}

func TestRouteDirectResponse(t *testing.T) {
	tests := map[string]struct {
		directResponse *dag.DirectResponse
		want           *envoy_route_v3.Route_DirectResponse
	}{
		"503-nobody": {
			directResponse: &dag.DirectResponse{StatusCode: 503},
			want: &envoy_route_v3.Route_DirectResponse{
				DirectResponse: &envoy_route_v3.DirectResponseAction{
					Status: 503,
				},
			},
		},
		"503": {
			directResponse: &dag.DirectResponse{StatusCode: 503, Body: "Service Unavailable"},
			want: &envoy_route_v3.Route_DirectResponse{
				DirectResponse: &envoy_route_v3.DirectResponseAction{
					Status: 503,
					Body: &envoy_core_v3.DataSource{
						Specifier: &envoy_core_v3.DataSource_InlineString{
							InlineString: "Service Unavailable",
						},
					},
				},
			},
		},
		"402-nobody": {
			directResponse: &dag.DirectResponse{StatusCode: 402},
			want: &envoy_route_v3.Route_DirectResponse{
				DirectResponse: &envoy_route_v3.DirectResponseAction{
					Status: 402,
				},
			},
		},
		"402": {
			directResponse: &dag.DirectResponse{StatusCode: 402, Body: "Payment Required"},
			want: &envoy_route_v3.Route_DirectResponse{
				DirectResponse: &envoy_route_v3.DirectResponseAction{
					Status: 402,
					Body: &envoy_core_v3.DataSource{
						Specifier: &envoy_core_v3.DataSource_InlineString{
							InlineString: "Payment Required",
						},
					},
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := routeDirectResponse(tc.directResponse)
			protobuf.ExpectEqual(t, tc.want, got)
		})
	}
}

func TestWeightedClusters(t *testing.T) {
	tests := map[string]struct {
		route *dag.Route
		want  *envoy_route_v3.WeightedCluster
	}{
		"multiple services w/o weights": {
			route: &dag.Route{
				Clusters: []*dag.Cluster{{
					Upstream: &dag.Service{
						Weighted: dag.WeightedService{
							Weight:           1,
							ServiceName:      "kuard",
							ServiceNamespace: "default",
							ServicePort: v1.ServicePort{
								Port: 8080,
							},
						},
					},
				}, {
					Upstream: &dag.Service{
						Weighted: dag.WeightedService{
							Weight:           1,
							ServiceName:      "nginx",
							ServiceNamespace: "default",
							ServicePort: v1.ServicePort{
								Port: 8080,
							},
						},
					},
				}},
			},
			want: &envoy_route_v3.WeightedCluster{
				Clusters: []*envoy_route_v3.WeightedCluster_ClusterWeight{{
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
			route: &dag.Route{
				Clusters: []*dag.Cluster{{
					Upstream: &dag.Service{
						Weighted: dag.WeightedService{
							Weight:           1,
							ServiceName:      "kuard",
							ServiceNamespace: "default",
							ServicePort: v1.ServicePort{
								Port: 8080,
							},
						},
					},
					Weight: 80,
				}, {
					Upstream: &dag.Service{
						Weighted: dag.WeightedService{
							Weight:           1,
							ServiceName:      "nginx",
							ServiceNamespace: "default",
							ServicePort: v1.ServicePort{
								Port: 8080,
							},
						},
					},
					Weight: 20,
				}},
			},
			want: &envoy_route_v3.WeightedCluster{
				Clusters: []*envoy_route_v3.WeightedCluster_ClusterWeight{{
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
			route: &dag.Route{
				Clusters: []*dag.Cluster{{
					Upstream: &dag.Service{
						Weighted: dag.WeightedService{
							Weight:           1,
							ServiceName:      "kuard",
							ServiceNamespace: "default",
							ServicePort: v1.ServicePort{
								Port: 8080,
							},
						},
					},
					Weight: 80,
				}, {
					Upstream: &dag.Service{
						Weighted: dag.WeightedService{
							Weight:           1,
							ServiceName:      "nginx",
							ServiceNamespace: "default",
							ServicePort: v1.ServicePort{
								Port: 8080,
							},
						},
					},
					Weight: 20,
				}, {
					Upstream: &dag.Service{
						Weighted: dag.WeightedService{
							Weight:           1,
							ServiceName:      "notraffic",
							ServiceNamespace: "default",
							ServicePort: v1.ServicePort{
								Port: 8080,
							},
						},
					},
				}},
			},
			want: &envoy_route_v3.WeightedCluster{
				Clusters: []*envoy_route_v3.WeightedCluster_ClusterWeight{{
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
			got := weightedClusters(tc.route)
			protobuf.ExpectEqual(t, tc.want, got)
		})
	}
}

func TestRouteConfiguration(t *testing.T) {
	tests := map[string]struct {
		name         string
		virtualhosts []*envoy_route_v3.VirtualHost
		want         *envoy_route_v3.RouteConfiguration
	}{

		"empty": {
			name: "ingress_http",
			want: &envoy_route_v3.RouteConfiguration{
				Name: "ingress_http",
				RequestHeadersToAdd: []*envoy_core_v3.HeaderValueOption{{
					Header: &envoy_core_v3.HeaderValue{
						Key:   "x-request-start",
						Value: "t=%START_TIME(%s.%3f)%",
					},
					Append: protobuf.Bool(true),
				}},
				RequestHeadersToRemove: []string{"x-forwarded-host"},
			},
		},
		"one virtualhost": {
			name: "ingress_https",
			virtualhosts: virtualhosts(
				VirtualHost("www.example.com"),
			),
			want: &envoy_route_v3.RouteConfiguration{
				Name: "ingress_https",
				VirtualHosts: virtualhosts(
					VirtualHost("www.example.com"),
				),
				RequestHeadersToAdd: []*envoy_core_v3.HeaderValueOption{{
					Header: &envoy_core_v3.HeaderValue{
						Key:   "x-request-start",
						Value: "t=%START_TIME(%s.%3f)%",
					},
					Append: protobuf.Bool(true),
				}},
				RequestHeadersToRemove: []string{"x-forwarded-host"},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := RouteConfiguration(tc.name, tc.virtualhosts...)
			protobuf.ExpectEqual(t, tc.want, got)
		})
	}
}

func TestVirtualHost(t *testing.T) {
	tests := map[string]struct {
		hostname string
		port     int
		want     *envoy_route_v3.VirtualHost
	}{
		"default hostname": {
			hostname: "*",
			port:     9999,
			want: &envoy_route_v3.VirtualHost{
				Name:    "*",
				Domains: []string{"*"},
			},
		},
		"wildcard hostname": {
			hostname: "*.bar.com",
			port:     9999,
			want: &envoy_route_v3.VirtualHost{
				Name:    "*.bar.com",
				Domains: []string{"*.bar.com"},
			},
		},
		"www.example.com": {
			hostname: "www.example.com",
			port:     9999,
			want: &envoy_route_v3.VirtualHost{
				Name:    "www.example.com",
				Domains: []string{"www.example.com"},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := VirtualHost(tc.hostname)
			protobuf.ExpectEqual(t, tc.want, got)
		})
	}
}

func TestCORSVirtualHost(t *testing.T) {
	tests := map[string]struct {
		hostname string
		cp       *envoy_route_v3.CorsPolicy
		want     *envoy_route_v3.VirtualHost
	}{
		"nil cors policy": {
			hostname: "www.example.com",
			cp:       nil,
			want: &envoy_route_v3.VirtualHost{
				Name:    "www.example.com",
				Domains: []string{"www.example.com"},
			},
		},
		"cors policy": {
			hostname: "www.example.com",
			cp: &envoy_route_v3.CorsPolicy{
				AllowOriginStringMatch: []*matcher.StringMatcher{
					{
						MatchPattern: &matcher.StringMatcher_Exact{
							Exact: "*",
						},
						IgnoreCase: true,
					}},
				AllowMethods: "GET,POST,PUT",
			},
			want: &envoy_route_v3.VirtualHost{
				Name:    "www.example.com",
				Domains: []string{"www.example.com"},
				Cors: &envoy_route_v3.CorsPolicy{
					AllowOriginStringMatch: []*matcher.StringMatcher{
						{
							MatchPattern: &matcher.StringMatcher_Exact{
								Exact: "*",
							},
							IgnoreCase: true,
						}},
					AllowMethods: "GET,POST,PUT",
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := CORSVirtualHost(tc.hostname, tc.cp)
			protobuf.ExpectEqual(t, tc.want, got)
		})
	}
}

func TestCORSPolicy(t *testing.T) {
	tests := map[string]struct {
		cp   *dag.CORSPolicy
		want *envoy_route_v3.CorsPolicy
	}{
		"only required properties set": {
			cp: &dag.CORSPolicy{
				AllowOrigin:  []dag.CORSAllowOriginMatch{{Type: dag.CORSAllowOriginMatchExact, Value: "*"}},
				AllowMethods: []string{"GET", "POST", "PUT"},
			},
			want: &envoy_route_v3.CorsPolicy{
				AllowOriginStringMatch: []*matcher.StringMatcher{
					{
						MatchPattern: &matcher.StringMatcher_Exact{
							Exact: "*",
						},
						IgnoreCase: true,
					}},
				AllowCredentials: protobuf.Bool(false),
				AllowMethods:     "GET,POST,PUT",
			},
		},
		"allow origin regex and specific": {
			cp: &dag.CORSPolicy{
				AllowOrigin: []dag.CORSAllowOriginMatch{
					{Type: dag.CORSAllowOriginMatchRegex, Value: `.*\.foo\.com`},
					{Type: dag.CORSAllowOriginMatchExact, Value: "https://bar.com"},
				},
				AllowMethods: []string{"GET"},
			},
			want: &envoy_route_v3.CorsPolicy{
				AllowOriginStringMatch: []*matcher.StringMatcher{
					{
						MatchPattern: &matcher.StringMatcher_SafeRegex{
							SafeRegex: &matcher.RegexMatcher{
								Regex: `.*\.foo\.com`,
							},
						},
					},
					{
						MatchPattern: &matcher.StringMatcher_Exact{
							Exact: "https://bar.com",
						},
						IgnoreCase: true,
					},
				},
				AllowCredentials: protobuf.Bool(false),
				AllowMethods:     "GET",
			},
		},
		"allow credentials": {
			cp: &dag.CORSPolicy{
				AllowOrigin:      []dag.CORSAllowOriginMatch{{Type: dag.CORSAllowOriginMatchExact, Value: "*"}},
				AllowMethods:     []string{"GET", "POST", "PUT"},
				AllowCredentials: true,
			},
			want: &envoy_route_v3.CorsPolicy{
				AllowOriginStringMatch: []*matcher.StringMatcher{
					{
						MatchPattern: &matcher.StringMatcher_Exact{
							Exact: "*",
						},
						IgnoreCase: true,
					}},
				AllowCredentials: protobuf.Bool(true),
				AllowMethods:     "GET,POST,PUT",
			},
		},
		"allow headers": {
			cp: &dag.CORSPolicy{
				AllowOrigin:  []dag.CORSAllowOriginMatch{{Type: dag.CORSAllowOriginMatchExact, Value: "*"}},
				AllowMethods: []string{"GET", "POST", "PUT"},
				AllowHeaders: []string{"header-1", "header-2"},
			},
			want: &envoy_route_v3.CorsPolicy{
				AllowOriginStringMatch: []*matcher.StringMatcher{
					{
						MatchPattern: &matcher.StringMatcher_Exact{
							Exact: "*",
						},
						IgnoreCase: true,
					}},
				AllowCredentials: protobuf.Bool(false),
				AllowMethods:     "GET,POST,PUT",
				AllowHeaders:     "header-1,header-2",
			},
		},
		"expose headers": {
			cp: &dag.CORSPolicy{
				AllowOrigin:   []dag.CORSAllowOriginMatch{{Type: dag.CORSAllowOriginMatchExact, Value: "*"}},
				AllowMethods:  []string{"GET", "POST", "PUT"},
				ExposeHeaders: []string{"header-1", "header-2"},
			},
			want: &envoy_route_v3.CorsPolicy{
				AllowOriginStringMatch: []*matcher.StringMatcher{
					{
						MatchPattern: &matcher.StringMatcher_Exact{
							Exact: "*",
						},
						IgnoreCase: true,
					}},
				AllowCredentials: protobuf.Bool(false),
				AllowMethods:     "GET,POST,PUT",
				ExposeHeaders:    "header-1,header-2",
			},
		},
		"max age": {
			cp: &dag.CORSPolicy{
				AllowOrigin:  []dag.CORSAllowOriginMatch{{Type: dag.CORSAllowOriginMatchExact, Value: "*"}},
				AllowMethods: []string{"GET", "POST", "PUT"},
				MaxAge:       timeout.DurationSetting(10 * time.Minute),
			},
			want: &envoy_route_v3.CorsPolicy{
				AllowOriginStringMatch: []*matcher.StringMatcher{
					{
						MatchPattern: &matcher.StringMatcher_Exact{
							Exact: "*",
						},
						IgnoreCase: true,
					}},
				AllowCredentials: protobuf.Bool(false),
				AllowMethods:     "GET,POST,PUT",
				MaxAge:           "600",
			},
		},
		"default max age": {
			cp: &dag.CORSPolicy{
				AllowOrigin:  []dag.CORSAllowOriginMatch{{Type: dag.CORSAllowOriginMatchExact, Value: "*"}},
				AllowMethods: []string{"GET", "POST", "PUT"},
				MaxAge:       timeout.DefaultSetting(),
			},
			want: &envoy_route_v3.CorsPolicy{
				AllowOriginStringMatch: []*matcher.StringMatcher{
					{
						MatchPattern: &matcher.StringMatcher_Exact{
							Exact: "*",
						},
						IgnoreCase: true,
					}},
				AllowCredentials: protobuf.Bool(false),
				AllowMethods:     "GET,POST,PUT",
			},
		},
		"max age disabled": {
			cp: &dag.CORSPolicy{
				AllowOrigin:  []dag.CORSAllowOriginMatch{{Type: dag.CORSAllowOriginMatchExact, Value: "*"}},
				AllowMethods: []string{"GET", "POST", "PUT"},
				MaxAge:       timeout.DisabledSetting(),
			},
			want: &envoy_route_v3.CorsPolicy{
				AllowOriginStringMatch: []*matcher.StringMatcher{
					{
						MatchPattern: &matcher.StringMatcher_Exact{
							Exact: "*",
						},
						IgnoreCase: true,
					}},
				AllowCredentials: protobuf.Bool(false),
				AllowMethods:     "GET,POST,PUT",
				MaxAge:           "0",
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := corsPolicy(tc.cp)
			protobuf.ExpectEqual(t, tc.want, got)
		})
	}
}

func TestUpgradeHTTPS(t *testing.T) {
	got := UpgradeHTTPS()
	want := &envoy_route_v3.Route_Redirect{
		Redirect: &envoy_route_v3.RedirectAction{
			SchemeRewriteSpecifier: &envoy_route_v3.RedirectAction_HttpsRedirect{
				HttpsRedirect: true,
			},
		},
	}

	assert.Equal(t, want, got)
}

func TestRouteMatch(t *testing.T) {
	tests := map[string]struct {
		route *dag.Route
		want  *envoy_route_v3.RouteMatch
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
			want: &envoy_route_v3.RouteMatch{
				Headers: []*envoy_route_v3.HeaderMatcher{{
					Name:        "x-header",
					InvertMatch: false,
					HeaderMatchSpecifier: &envoy_route_v3.HeaderMatcher_StringMatch{
						StringMatch: &matcher.StringMatcher{
							MatchPattern: &matcher.StringMatcher_SafeRegex{
								SafeRegex: SafeRegexMatch(".*11-22-33-44.*"),
							},
						},
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
			want: &envoy_route_v3.RouteMatch{
				Headers: []*envoy_route_v3.HeaderMatcher{{
					Name:        "x-header",
					InvertMatch: false,
					HeaderMatchSpecifier: &envoy_route_v3.HeaderMatcher_StringMatch{
						StringMatch: &matcher.StringMatcher{
							MatchPattern: &matcher.StringMatcher_SafeRegex{
								SafeRegex: SafeRegexMatch(".*11\\.22\\.33\\.44.*"),
							},
						},
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
			want: &envoy_route_v3.RouteMatch{
				Headers: []*envoy_route_v3.HeaderMatcher{{
					Name:        "x-header",
					InvertMatch: false,
					HeaderMatchSpecifier: &envoy_route_v3.HeaderMatcher_StringMatch{
						StringMatch: &matcher.StringMatcher{
							MatchPattern: &matcher.StringMatcher_SafeRegex{
								SafeRegex: SafeRegexMatch(".*11\\.\\[22\\]\\.\\*33\\.44.*"),
							},
						},
					},
				}},
			},
		},
		"path prefix string prefix": {
			route: &dag.Route{
				PathMatchCondition: &dag.PrefixMatchCondition{
					Prefix:          "/foo",
					PrefixMatchType: dag.PrefixMatchString,
				},
			},
			want: &envoy_route_v3.RouteMatch{
				PathSpecifier: &envoy_route_v3.RouteMatch_Prefix{
					Prefix: "/foo",
				},
			},
		},
		"path prefix match segment": {
			route: &dag.Route{
				PathMatchCondition: &dag.PrefixMatchCondition{
					Prefix:          "/foo",
					PrefixMatchType: dag.PrefixMatchSegment,
				},
			},
			want: &envoy_route_v3.RouteMatch{
				PathSpecifier: &envoy_route_v3.RouteMatch_PathSeparatedPrefix{
					PathSeparatedPrefix: "/foo",
				},
			},
		},
		"path exact": {
			route: &dag.Route{
				PathMatchCondition: &dag.ExactMatchCondition{
					Path: "/foo",
				},
			},
			want: &envoy_route_v3.RouteMatch{
				PathSpecifier: &envoy_route_v3.RouteMatch_Path{
					Path: "/foo",
				},
			},
		},
		"path regex": {
			route: &dag.Route{
				PathMatchCondition: &dag.RegexMatchCondition{
					Regex: "/v.1/*",
				},
			},
			want: &envoy_route_v3.RouteMatch{
				PathSpecifier: &envoy_route_v3.RouteMatch_SafeRegex{
					// note, unlike header conditions this is not a quoted regex because
					// the value comes directly from the Ingress.Paths.Path value which
					// is permitted to be a bare regex.
					// We add an anchor since we should always have a / prefix to reduce
					// complexity.
					SafeRegex: SafeRegexMatch("^/v.1/*"),
				},
			},
		},
		"header regex match": {
			route: &dag.Route{
				HeaderMatchConditions: []dag.HeaderMatchCondition{{
					Name:      "x-regex-header",
					Value:     "[a-z0-9][a-z0-9-]+someniceregex",
					MatchType: dag.HeaderMatchTypeRegex,
					Invert:    false,
				}},
			},
			want: &envoy_route_v3.RouteMatch{
				Headers: []*envoy_route_v3.HeaderMatcher{{
					Name:        "x-regex-header",
					InvertMatch: false,
					HeaderMatchSpecifier: &envoy_route_v3.HeaderMatcher_StringMatch{
						StringMatch: &matcher.StringMatcher{
							MatchPattern: &matcher.StringMatcher_SafeRegex{
								SafeRegex: &matcher.RegexMatcher{
									Regex: "[a-z0-9][a-z0-9-]+someniceregex",
								},
							},
						},
					},
				}},
			},
		},
		"query param exact match": {
			route: &dag.Route{
				QueryParamMatchConditions: []dag.QueryParamMatchCondition{
					{
						Name:      "query-param-1",
						Value:     "query-value-1",
						MatchType: "exact",
					},
				},
			},
			want: &envoy_route_v3.RouteMatch{
				QueryParameters: []*envoy_route_v3.QueryParameterMatcher{
					{
						Name: "query-param-1",
						QueryParameterMatchSpecifier: &envoy_route_v3.QueryParameterMatcher_StringMatch{
							StringMatch: &matcher.StringMatcher{
								MatchPattern: &matcher.StringMatcher_Exact{
									Exact: "query-value-1",
								},
							},
						},
					},
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := RouteMatch(tc.route)
			protobuf.ExpectEqual(t, tc.want, got)
		})
	}
}

func TestRouteRedirect(t *testing.T) {
	tests := map[string]struct {
		redirect *dag.Redirect
		want     *envoy_route_v3.Route_Redirect
	}{
		"hostname specified": {
			redirect: &dag.Redirect{
				Hostname: "foo.bar",
			},
			want: &envoy_route_v3.Route_Redirect{
				Redirect: &envoy_route_v3.RedirectAction{
					HostRedirect: "foo.bar",
				},
			},
		},
		"scheme specified": {
			redirect: &dag.Redirect{
				Scheme: "https",
			},
			want: &envoy_route_v3.Route_Redirect{
				Redirect: &envoy_route_v3.RedirectAction{
					SchemeRewriteSpecifier: &envoy_route_v3.RedirectAction_SchemeRedirect{
						SchemeRedirect: "https",
					},
				},
			},
		},
		"port number specified": {
			redirect: &dag.Redirect{
				PortNumber: 8080,
			},
			want: &envoy_route_v3.Route_Redirect{
				Redirect: &envoy_route_v3.RedirectAction{
					PortRedirect: 8080,
				},
			},
		},
		"status code specified": {
			redirect: &dag.Redirect{
				StatusCode: 302,
			},
			want: &envoy_route_v3.Route_Redirect{
				Redirect: &envoy_route_v3.RedirectAction{
					ResponseCode: envoy_route_v3.RedirectAction_FOUND,
				},
			},
		},
		"path specified": {
			redirect: &dag.Redirect{
				Path: "/blog",
			},
			want: &envoy_route_v3.Route_Redirect{
				Redirect: &envoy_route_v3.RedirectAction{
					PathRewriteSpecifier: &envoy_route_v3.RedirectAction_PathRedirect{
						PathRedirect: "/blog",
					},
				},
			},
		},
		"prefix specified": {
			redirect: &dag.Redirect{
				Prefix: "/blog",
			},
			want: &envoy_route_v3.Route_Redirect{
				Redirect: &envoy_route_v3.RedirectAction{
					PathRewriteSpecifier: &envoy_route_v3.RedirectAction_PrefixRewrite{
						PrefixRewrite: "/blog",
					},
				},
			},
		},
		"unsupported status code specified": {
			redirect: &dag.Redirect{
				StatusCode: 303,
			},
			want: &envoy_route_v3.Route_Redirect{
				Redirect: &envoy_route_v3.RedirectAction{},
			},
		},
		"all options specified": {
			redirect: &dag.Redirect{
				Hostname:   "foo.bar",
				Scheme:     "https",
				PortNumber: 8443,
				StatusCode: 302,
				Path:       "/blog",
			},
			want: &envoy_route_v3.Route_Redirect{
				Redirect: &envoy_route_v3.RedirectAction{
					HostRedirect: "foo.bar",
					SchemeRewriteSpecifier: &envoy_route_v3.RedirectAction_SchemeRedirect{
						SchemeRedirect: "https",
					},
					PortRedirect: 8443,
					ResponseCode: envoy_route_v3.RedirectAction_FOUND,
					PathRewriteSpecifier: &envoy_route_v3.RedirectAction_PathRedirect{
						PathRedirect: "/blog",
					},
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := routeRedirect(tc.redirect)
			protobuf.ExpectEqual(t, tc.want, got)
		})
	}
}

func virtualhosts(v ...*envoy_route_v3.VirtualHost) []*envoy_route_v3.VirtualHost { return v }
