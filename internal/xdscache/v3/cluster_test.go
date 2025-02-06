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

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_upstream_http_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	core_v1 "k8s.io/api/core/v1"
	networking_v1 "k8s.io/api/networking/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/protobuf"
)

func TestClusterCacheContents(t *testing.T) {
	envoyConfigSource := envoy_v3.NewEnvoyGen(envoy_v3.EnvoyGenOpt{
		XDSClusterName: envoy_v3.DefaultXDSClusterName,
	}).GetConfigSource()
	tests := map[string]struct {
		contents map[string]*envoy_config_cluster_v3.Cluster
		want     []proto.Message
	}{
		"empty": {
			contents: nil,
			want:     nil,
		},
		"simple": {
			contents: clustermap(
				&envoy_config_cluster_v3.Cluster{
					Name:                 "default/kuard/443/da39a3ee5e",
					AltStatName:          "default_kuard_443",
					ClusterDiscoveryType: envoy_v3.ClusterDiscoveryType(envoy_config_cluster_v3.Cluster_EDS),
					EdsClusterConfig: &envoy_config_cluster_v3.Cluster_EdsClusterConfig{
						EdsConfig:   envoyConfigSource,
						ServiceName: "default/kuard",
					},
				}),
			want: []proto.Message{
				cluster(&envoy_config_cluster_v3.Cluster{
					Name:                 "default/kuard/443/da39a3ee5e",
					AltStatName:          "default_kuard_443",
					ClusterDiscoveryType: envoy_v3.ClusterDiscoveryType(envoy_config_cluster_v3.Cluster_EDS),
					EdsClusterConfig: &envoy_config_cluster_v3.Cluster_EdsClusterConfig{
						EdsConfig:   envoyConfigSource,
						ServiceName: "default/kuard",
					},
				}),
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var cc ClusterCache
			cc.Update(tc.contents)
			got := cc.Contents()
			protobuf.ExpectEqual(t, tc.want, got)
		})
	}
}

func TestClusterVisit(t *testing.T) {
	envoyGen := envoy_v3.NewEnvoyGen(envoy_v3.EnvoyGenOpt{
		XDSClusterName: envoy_v3.DefaultXDSClusterName,
	})
	envoyConfigSource := envoyGen.GetConfigSource()
	tests := map[string]struct {
		objs []any
		want map[string]*envoy_config_cluster_v3.Cluster
	}{
		"nothing": {
			objs: nil,
			want: map[string]*envoy_config_cluster_v3.Cluster{},
		},
		"single unnamed service": {
			objs: []any{
				&networking_v1.Ingress{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
					},
					Spec: networking_v1.IngressSpec{
						DefaultBackend: backend("kuard", 443),
					},
				},
				service("default", "kuard",
					core_v1.ServicePort{
						Protocol:   "TCP",
						Port:       443,
						TargetPort: intstr.FromInt(8443),
					},
				),
			},
			want: clustermap(
				&envoy_config_cluster_v3.Cluster{
					Name:                 "default/kuard/443/da39a3ee5e",
					AltStatName:          "default_kuard_443",
					ClusterDiscoveryType: envoy_v3.ClusterDiscoveryType(envoy_config_cluster_v3.Cluster_EDS),
					EdsClusterConfig: &envoy_config_cluster_v3.Cluster_EdsClusterConfig{
						EdsConfig:   envoyConfigSource,
						ServiceName: "default/kuard",
					},
				}),
		},
		"single named service": {
			objs: []any{
				&networking_v1.Ingress{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
					},
					Spec: networking_v1.IngressSpec{
						DefaultBackend: &networking_v1.IngressBackend{
							Service: &networking_v1.IngressServiceBackend{
								Name: "kuard",
								Port: networking_v1.ServiceBackendPort{Name: "https"},
							},
						},
					},
				},
				service("default", "kuard",
					core_v1.ServicePort{
						Name:       "https",
						Protocol:   "TCP",
						Port:       443,
						TargetPort: intstr.FromInt(8443),
					},
				),
			},
			want: clustermap(
				&envoy_config_cluster_v3.Cluster{
					Name:                 "default/kuard/443/da39a3ee5e",
					AltStatName:          "default_kuard_443",
					ClusterDiscoveryType: envoy_v3.ClusterDiscoveryType(envoy_config_cluster_v3.Cluster_EDS),
					EdsClusterConfig: &envoy_config_cluster_v3.Cluster_EdsClusterConfig{
						EdsConfig:   envoyConfigSource,
						ServiceName: "default/kuard/https",
					},
				}),
		},
		"h2c upstream": {
			objs: []any{
				&networking_v1.Ingress{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
					},
					Spec: networking_v1.IngressSpec{
						DefaultBackend: &networking_v1.IngressBackend{
							Service: &networking_v1.IngressServiceBackend{
								Name: "kuard",
								Port: networking_v1.ServiceBackendPort{Name: "http"},
							},
						},
					},
				},
				serviceWithAnnotations(
					"default",
					"kuard",
					map[string]string{
						"projectcontour.io/upstream-protocol.h2c": "80,http",
					},
					core_v1.ServicePort{
						Protocol: "TCP",
						Name:     "http",
						Port:     80,
					},
				),
			},
			want: clustermap(
				&envoy_config_cluster_v3.Cluster{
					Name:                 "default/kuard/80/f4f94965ec",
					AltStatName:          "default_kuard_80",
					ClusterDiscoveryType: envoy_v3.ClusterDiscoveryType(envoy_config_cluster_v3.Cluster_EDS),
					EdsClusterConfig: &envoy_config_cluster_v3.Cluster_EdsClusterConfig{
						EdsConfig:   envoyConfigSource,
						ServiceName: "default/kuard/http",
					},
					TypedExtensionProtocolOptions: map[string]*anypb.Any{
						"envoy.extensions.upstreams.http.v3.HttpProtocolOptions": protobuf.MustMarshalAny(
							&envoy_upstream_http_v3.HttpProtocolOptions{
								UpstreamProtocolOptions: &envoy_upstream_http_v3.HttpProtocolOptions_ExplicitHttpConfig_{
									ExplicitHttpConfig: &envoy_upstream_http_v3.HttpProtocolOptions_ExplicitHttpConfig{
										ProtocolConfig: &envoy_upstream_http_v3.HttpProtocolOptions_ExplicitHttpConfig_Http2ProtocolOptions{},
									},
								},
							}),
					},
				},
			),
		},
		"long namespace and service name": {
			objs: []any{
				&networking_v1.Ingress{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "webserver-1-unimatrix-zero-one",
						Namespace: "beurocratic-company-test-domain-1",
					},
					Spec: networking_v1.IngressSpec{
						DefaultBackend: backend("tiny-cog-department-test-instance", 443),
					},
				},
				service("beurocratic-company-test-domain-1", "tiny-cog-department-test-instance",
					core_v1.ServicePort{
						Name:       "svc-0",
						Protocol:   "TCP",
						Port:       443,
						TargetPort: intstr.FromInt(8443),
					},
				),
			},
			want: clustermap(
				&envoy_config_cluster_v3.Cluster{
					Name:                 "beurocra-7fe4b4/tiny-cog-7fe4b4/443/da39a3ee5e",
					AltStatName:          "beurocratic-company-test-domain-1_tiny-cog-department-test-instance_443",
					ClusterDiscoveryType: envoy_v3.ClusterDiscoveryType(envoy_config_cluster_v3.Cluster_EDS),
					EdsClusterConfig: &envoy_config_cluster_v3.Cluster_EdsClusterConfig{
						EdsConfig:   envoyConfigSource,
						ServiceName: "beurocratic-company-test-domain-1/tiny-cog-department-test-instance/svc-0",
					},
				}),
		},
		"two service ports": {
			objs: []any{
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_v1.Route{{
							Conditions: []contour_v1.MatchCondition{{
								Prefix: "/",
							}},
							Services: []contour_v1.Service{{
								Name: "backend",
								Port: 80,
							}, {
								Name: "backend",
								Port: 8080,
							}},
						}},
					},
				},
				service("default", "backend", core_v1.ServicePort{
					Name:       "http",
					Protocol:   "TCP",
					Port:       80,
					TargetPort: intstr.FromInt(6502),
				}, core_v1.ServicePort{
					Name:       "alt",
					Protocol:   "TCP",
					Port:       8080,
					TargetPort: intstr.FromString("9001"),
				}),
			},
			want: clustermap(
				&envoy_config_cluster_v3.Cluster{
					Name:                 "default/backend/80/da39a3ee5e",
					AltStatName:          "default_backend_80",
					ClusterDiscoveryType: envoy_v3.ClusterDiscoveryType(envoy_config_cluster_v3.Cluster_EDS),
					EdsClusterConfig: &envoy_config_cluster_v3.Cluster_EdsClusterConfig{
						EdsConfig:   envoyConfigSource,
						ServiceName: "default/backend/http",
					},
				},
				&envoy_config_cluster_v3.Cluster{
					Name:                 "default/backend/8080/da39a3ee5e",
					AltStatName:          "default_backend_8080",
					ClusterDiscoveryType: envoy_v3.ClusterDiscoveryType(envoy_config_cluster_v3.Cluster_EDS),
					EdsClusterConfig: &envoy_config_cluster_v3.Cluster_EdsClusterConfig{
						EdsConfig:   envoyConfigSource,
						ServiceName: "default/backend/alt",
					},
				},
			),
		},
		"httpproxy with simple path healthcheck": {
			objs: []any{
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_v1.Route{{
							HealthCheckPolicy: &contour_v1.HTTPHealthCheckPolicy{
								Path: "/healthy",
							},
							Conditions: []contour_v1.MatchCondition{{
								Prefix: "/",
							}},
							Services: []contour_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				service("default", "backend", core_v1.ServicePort{
					Name:       "http",
					Protocol:   "TCP",
					Port:       80,
					TargetPort: intstr.FromInt(6502),
				}),
			},
			want: clustermap(
				&envoy_config_cluster_v3.Cluster{
					Name:                 "default/backend/80/c184349821",
					AltStatName:          "default_backend_80",
					ClusterDiscoveryType: envoy_v3.ClusterDiscoveryType(envoy_config_cluster_v3.Cluster_EDS),
					EdsClusterConfig: &envoy_config_cluster_v3.Cluster_EdsClusterConfig{
						EdsConfig:   envoyConfigSource,
						ServiceName: "default/backend/http",
					},
					HealthChecks: []*envoy_config_core_v3.HealthCheck{{
						Timeout:            &durationpb.Duration{Seconds: 2},
						Interval:           &durationpb.Duration{Seconds: 10},
						UnhealthyThreshold: wrapperspb.UInt32(3),
						HealthyThreshold:   wrapperspb.UInt32(2),
						HealthChecker: &envoy_config_core_v3.HealthCheck_HttpHealthCheck_{
							HttpHealthCheck: &envoy_config_core_v3.HealthCheck_HttpHealthCheck{
								Path: "/healthy",
								Host: "contour-envoy-healthcheck",
							},
						},
					}},
					IgnoreHealthOnHostRemoval: true,
				},
			),
		},
		"httpproxy with custom healthcheck": {
			objs: []any{
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_v1.Route{{
							HealthCheckPolicy: &contour_v1.HTTPHealthCheckPolicy{
								Host:                    "foo-bar-host",
								Path:                    "/healthy",
								TimeoutSeconds:          99,
								IntervalSeconds:         98,
								UnhealthyThresholdCount: 97,
								HealthyThresholdCount:   96,
							},
							Conditions: []contour_v1.MatchCondition{{
								Prefix: "/",
							}},
							Services: []contour_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				service("default", "backend", core_v1.ServicePort{
					Name:       "http",
					Protocol:   "TCP",
					Port:       80,
					TargetPort: intstr.FromInt(6502),
				}),
			},
			want: clustermap(
				&envoy_config_cluster_v3.Cluster{
					Name:                 "default/backend/80/7f8051653a",
					AltStatName:          "default_backend_80",
					ClusterDiscoveryType: envoy_v3.ClusterDiscoveryType(envoy_config_cluster_v3.Cluster_EDS),
					EdsClusterConfig: &envoy_config_cluster_v3.Cluster_EdsClusterConfig{
						EdsConfig:   envoyConfigSource,
						ServiceName: "default/backend/http",
					},
					HealthChecks: []*envoy_config_core_v3.HealthCheck{{
						Timeout:            &durationpb.Duration{Seconds: 99},
						Interval:           &durationpb.Duration{Seconds: 98},
						UnhealthyThreshold: wrapperspb.UInt32(97),
						HealthyThreshold:   wrapperspb.UInt32(96),
						HealthChecker: &envoy_config_core_v3.HealthCheck_HttpHealthCheck_{
							HttpHealthCheck: &envoy_config_core_v3.HealthCheck_HttpHealthCheck{
								Path: "/healthy",
								Host: "foo-bar-host",
							},
						},
					}},
					IgnoreHealthOnHostRemoval: true,
				},
			),
		},
		"httpproxy with RoundRobin lb algorithm": {
			objs: []any{
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_v1.Route{{
							LoadBalancerPolicy: &contour_v1.LoadBalancerPolicy{
								Strategy: "RoundRobin",
							},
							Conditions: []contour_v1.MatchCondition{{
								Prefix: "/",
							}},
							Services: []contour_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				service("default", "backend", core_v1.ServicePort{
					Name:       "http",
					Protocol:   "TCP",
					Port:       80,
					TargetPort: intstr.FromInt(6502),
				}),
			},
			want: clustermap(
				&envoy_config_cluster_v3.Cluster{
					Name:                 "default/backend/80/da39a3ee5e",
					AltStatName:          "default_backend_80",
					ClusterDiscoveryType: envoy_v3.ClusterDiscoveryType(envoy_config_cluster_v3.Cluster_EDS),
					EdsClusterConfig: &envoy_config_cluster_v3.Cluster_EdsClusterConfig{
						EdsConfig:   envoyConfigSource,
						ServiceName: "default/backend/http",
					},
				},
			),
		},
		"httpproxy with WeightedLeastRequest lb algorithm": {
			objs: []any{
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_v1.Route{{
							LoadBalancerPolicy: &contour_v1.LoadBalancerPolicy{
								Strategy: "WeightedLeastRequest",
							},
							Conditions: []contour_v1.MatchCondition{{
								Prefix: "/",
							}},
							Services: []contour_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				service("default", "backend", core_v1.ServicePort{
					Name:       "http",
					Protocol:   "TCP",
					Port:       80,
					TargetPort: intstr.FromInt(6502),
				}),
			},
			want: clustermap(
				&envoy_config_cluster_v3.Cluster{
					Name:                 "default/backend/80/8bf87fefba",
					AltStatName:          "default_backend_80",
					ClusterDiscoveryType: envoy_v3.ClusterDiscoveryType(envoy_config_cluster_v3.Cluster_EDS),
					EdsClusterConfig: &envoy_config_cluster_v3.Cluster_EdsClusterConfig{
						EdsConfig:   envoyConfigSource,
						ServiceName: "default/backend/http",
					},
					LbPolicy: envoy_config_cluster_v3.Cluster_LEAST_REQUEST,
				},
			),
		},
		"httpproxy with Random lb algorithm": {
			objs: []any{
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_v1.Route{{
							LoadBalancerPolicy: &contour_v1.LoadBalancerPolicy{
								Strategy: "Random",
							},
							Conditions: []contour_v1.MatchCondition{{
								Prefix: "/",
							}},
							Services: []contour_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				service("default", "backend", core_v1.ServicePort{
					Name:       "http",
					Protocol:   "TCP",
					Port:       80,
					TargetPort: intstr.FromInt(6502),
				}),
			},
			want: clustermap(
				&envoy_config_cluster_v3.Cluster{
					Name:                 "default/backend/80/58d888c08a",
					AltStatName:          "default_backend_80",
					ClusterDiscoveryType: envoy_v3.ClusterDiscoveryType(envoy_config_cluster_v3.Cluster_EDS),
					EdsClusterConfig: &envoy_config_cluster_v3.Cluster_EdsClusterConfig{
						EdsConfig:   envoyConfigSource,
						ServiceName: "default/backend/http",
					},
					LbPolicy: envoy_config_cluster_v3.Cluster_RANDOM,
				},
			),
		},
		"httpproxy with RequestHash lb algorithm and valid header hash option": {
			objs: []any{
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_v1.Route{{
							LoadBalancerPolicy: &contour_v1.LoadBalancerPolicy{
								Strategy: "RequestHash",
								RequestHashPolicies: []contour_v1.RequestHashPolicy{
									{
										HeaderHashOptions: &contour_v1.HeaderHashOptions{
											HeaderName: "X-Custom-Header",
										},
									},
								},
							},
							Conditions: []contour_v1.MatchCondition{{
								Prefix: "/",
							}},
							Services: []contour_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				service("default", "backend", core_v1.ServicePort{
					Name:       "http",
					Protocol:   "TCP",
					Port:       80,
					TargetPort: intstr.FromInt(6502),
				}),
			},
			want: clustermap(
				&envoy_config_cluster_v3.Cluster{
					Name:                 "default/backend/80/1a2ffc1fef",
					AltStatName:          "default_backend_80",
					ClusterDiscoveryType: envoy_v3.ClusterDiscoveryType(envoy_config_cluster_v3.Cluster_EDS),
					EdsClusterConfig: &envoy_config_cluster_v3.Cluster_EdsClusterConfig{
						EdsConfig:   envoyConfigSource,
						ServiceName: "default/backend/http",
					},
					LbPolicy: envoy_config_cluster_v3.Cluster_RING_HASH,
				},
			),
		},
		// Removed testcase - "httpproxy with differing lb algorithms"
		// HTTPProxy has LB algorithm as a route-level construct, so it's not possible.
		"httpproxy with unknown lb algorithm": {
			objs: []any{
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_v1.Route{{
							LoadBalancerPolicy: &contour_v1.LoadBalancerPolicy{
								Strategy: "lulz",
							},
							Conditions: []contour_v1.MatchCondition{{
								Prefix: "/",
							}},
							Services: []contour_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				service("default", "backend", core_v1.ServicePort{
					Name:       "http",
					Protocol:   "TCP",
					Port:       80,
					TargetPort: intstr.FromInt(6502),
				}),
			},
			want: clustermap(
				&envoy_config_cluster_v3.Cluster{
					Name:                 "default/backend/80/da39a3ee5e",
					AltStatName:          "default_backend_80",
					ClusterDiscoveryType: envoy_v3.ClusterDiscoveryType(envoy_config_cluster_v3.Cluster_EDS),
					EdsClusterConfig: &envoy_config_cluster_v3.Cluster_EdsClusterConfig{
						EdsConfig:   envoyConfigSource,
						ServiceName: "default/backend/http",
					},
				},
			),
		},
		"CircuitBreakers annotations": {
			objs: []any{
				&networking_v1.Ingress{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
					},
					Spec: networking_v1.IngressSpec{
						DefaultBackend: &networking_v1.IngressBackend{
							Service: &networking_v1.IngressServiceBackend{
								Name: "kuard",
								Port: networking_v1.ServiceBackendPort{Name: "http"},
							},
						},
					},
				},
				serviceWithAnnotations(
					"default",
					"kuard",
					map[string]string{
						"projectcontour.io/max-connections":          "9000",
						"projectcontour.io/max-pending-requests":     "4096",
						"projectcontour.io/max-requests":             "404",
						"projectcontour.io/max-retries":              "7",
						"projectcontour.io/per-host-max-connections": "45",
					},
					core_v1.ServicePort{
						Protocol: "TCP",
						Name:     "http",
						Port:     80,
					},
				),
			},
			want: clustermap(
				&envoy_config_cluster_v3.Cluster{
					Name:                 "default/kuard/80/da39a3ee5e",
					AltStatName:          "default_kuard_80",
					ClusterDiscoveryType: envoy_v3.ClusterDiscoveryType(envoy_config_cluster_v3.Cluster_EDS),
					EdsClusterConfig: &envoy_config_cluster_v3.Cluster_EdsClusterConfig{
						EdsConfig:   envoyConfigSource,
						ServiceName: "default/kuard/http",
					},
					CircuitBreakers: &envoy_config_cluster_v3.CircuitBreakers{
						Thresholds: []*envoy_config_cluster_v3.CircuitBreakers_Thresholds{{
							MaxConnections:     wrapperspb.UInt32(9000),
							MaxPendingRequests: wrapperspb.UInt32(4096),
							MaxRequests:        wrapperspb.UInt32(404),
							MaxRetries:         wrapperspb.UInt32(7),
							TrackRemaining:     true,
						}},
						PerHostThresholds: []*envoy_config_cluster_v3.CircuitBreakers_Thresholds{{
							MaxConnections: wrapperspb.UInt32(45),
							TrackRemaining: true,
						}},
					},
				},
			),
		},
		"projectcontour.io/num-retries annotation": {
			objs: []any{
				&networking_v1.Ingress{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
						Annotations: map[string]string{
							"projectcontour.io/num-retries": "7",
							"projectcontour.io/retry-on":    "gateway-error",
						},
					},
					Spec: networking_v1.IngressSpec{
						DefaultBackend: &networking_v1.IngressBackend{
							Service: &networking_v1.IngressServiceBackend{
								Name: "kuard",
								Port: networking_v1.ServiceBackendPort{Name: "https"},
							},
						},
					},
				},
				service("default", "kuard",
					core_v1.ServicePort{
						Name:       "https",
						Protocol:   "TCP",
						Port:       443,
						TargetPort: intstr.FromInt(8443),
					},
				),
			},
			want: clustermap(
				&envoy_config_cluster_v3.Cluster{
					Name:                 "default/kuard/443/da39a3ee5e",
					AltStatName:          "default_kuard_443",
					ClusterDiscoveryType: envoy_v3.ClusterDiscoveryType(envoy_config_cluster_v3.Cluster_EDS),
					EdsClusterConfig: &envoy_config_cluster_v3.Cluster_EdsClusterConfig{
						EdsConfig:   envoyConfigSource,
						ServiceName: "default/kuard/https",
					},
				}),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cc := ClusterCache{
				envoyGen: envoyGen,
			}
			cc.OnChange(buildDAG(t, tc.objs...))
			protobuf.ExpectEqual(t, tc.want, cc.values)
		})
	}
}

func service(ns, name string, ports ...core_v1.ServicePort) *core_v1.Service {
	return serviceWithAnnotations(ns, name, nil, ports...)
}

func serviceWithAnnotations(ns, name string, annotations map[string]string, ports ...core_v1.ServicePort) *core_v1.Service {
	return &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:        name,
			Namespace:   ns,
			Annotations: annotations,
		},
		Spec: core_v1.ServiceSpec{
			Ports: ports,
		},
	}
}

func cluster(c *envoy_config_cluster_v3.Cluster) *envoy_config_cluster_v3.Cluster {
	// NOTE: Keep this in sync with envoy.defaultCluster().
	defaults := &envoy_config_cluster_v3.Cluster{
		ConnectTimeout: durationpb.New(2 * time.Second),
		CommonLbConfig: envoy_v3.ClusterCommonLBConfig(),
		LbPolicy:       envoy_config_cluster_v3.Cluster_ROUND_ROBIN,
	}

	proto.Merge(defaults, c)
	return defaults
}

func clustermap(clusters ...*envoy_config_cluster_v3.Cluster) map[string]*envoy_config_cluster_v3.Cluster {
	m := make(map[string]*envoy_config_cluster_v3.Cluster)
	for _, c := range clusters {
		m[c.Name] = cluster(c)
	}
	return m
}
