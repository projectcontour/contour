// Copyright © 2018 Heptio
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

package contour

import (
	"reflect"
	"testing"
	"time"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/cluster"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/gogo/protobuf/types"
	ingressroutev1 "github.com/heptio/contour/apis/contour/v1beta1"
	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestClusterVisit(t *testing.T) {
	tests := map[string]struct {
		objs []interface{}
		want map[string]*v2.Cluster
	}{
		"nothing": {
			objs: nil,
			want: map[string]*v2.Cluster{},
		},
		"single unnamed service": {
			objs: []interface{}{
				&v1beta1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
					},
					Spec: v1beta1.IngressSpec{
						Backend: &v1beta1.IngressBackend{
							ServiceName: "kuard",
							ServicePort: intstr.FromInt(443),
						},
					},
				},
				service("default", "kuard",
					v1.ServicePort{
						Protocol:   "TCP",
						Port:       443,
						TargetPort: intstr.FromInt(8443),
					},
				),
			},
			want: clustermap(
				&v2.Cluster{
					Name: "default/kuard/443",
					Type: v2.Cluster_EDS,
					EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
						EdsConfig:   apiconfigsource("contour"), // hard coded by initconfig
						ServiceName: "default/kuard",
					},
					ConnectTimeout: 250 * time.Millisecond,
					LbPolicy:       v2.Cluster_ROUND_ROBIN,
				}),
		},
		"single named service": {
			objs: []interface{}{
				&v1beta1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
					},
					Spec: v1beta1.IngressSpec{
						Backend: &v1beta1.IngressBackend{
							ServiceName: "kuard",
							ServicePort: intstr.FromString("https"),
						},
					},
				},
				service("default", "kuard",
					v1.ServicePort{
						Name:       "https",
						Protocol:   "TCP",
						Port:       443,
						TargetPort: intstr.FromInt(8443),
					},
				),
			},
			want: clustermap(
				&v2.Cluster{
					Name: "default/kuard/443",
					Type: v2.Cluster_EDS,
					EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
						EdsConfig:   apiconfigsource("contour"), // hard coded by initconfig
						ServiceName: "default/kuard/https",
					},
					ConnectTimeout: 250 * time.Millisecond,
					LbPolicy:       v2.Cluster_ROUND_ROBIN,
				}),
		},
		"h2c upstream": {
			objs: []interface{}{
				&v1beta1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
					},
					Spec: v1beta1.IngressSpec{
						Backend: &v1beta1.IngressBackend{
							ServiceName: "kuard",
							ServicePort: intstr.FromString("http"),
						},
					},
				},
				serviceWithAnnotations(
					"default",
					"kuard",
					map[string]string{
						"contour.heptio.com/upstream-protocol.h2c": "80,http",
					},
					v1.ServicePort{
						Protocol: "TCP",
						Name:     "http",
						Port:     80,
					},
				),
			},
			want: clustermap(
				&v2.Cluster{
					Name: "default/kuard/80",
					Type: v2.Cluster_EDS,
					EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
						EdsConfig:   apiconfigsource("contour"), // hard coded by initconfig
						ServiceName: "default/kuard/http",
					},
					ConnectTimeout:       250 * time.Millisecond,
					LbPolicy:             v2.Cluster_ROUND_ROBIN,
					Http2ProtocolOptions: &core.Http2ProtocolOptions{},
				},
			),
		},
		"long namespace and service name": {
			objs: []interface{}{
				&v1beta1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "webserver-1-unimatrix-zero-one",
						Namespace: "beurocratic-company-test-domain-1",
					},
					Spec: v1beta1.IngressSpec{
						Backend: &v1beta1.IngressBackend{
							ServiceName: "tiny-cog-department-test-instance",
							ServicePort: intstr.FromInt(443),
						},
					},
				},
				service("beurocratic-company-test-domain-1", "tiny-cog-department-test-instance",
					v1.ServicePort{
						Name:       "svc-0",
						Protocol:   "TCP",
						Port:       443,
						TargetPort: intstr.FromInt(8443),
					},
				),
			},
			want: clustermap(
				&v2.Cluster{
					Name: "beurocratic-company-test-domain-1/tiny-cog-depa-81582b/443",
					Type: v2.Cluster_EDS,
					EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
						EdsConfig:   apiconfigsource("contour"), // hard coded by initconfig
						ServiceName: "beurocratic-company-test-domain-1/tiny-cog-department-test-instance/svc-0",
					},
					ConnectTimeout: 250 * time.Millisecond,
					LbPolicy:       v2.Cluster_ROUND_ROBIN,
				}),
		},
		"two service ports": {
			objs: []interface{}{
				&ingressroutev1.IngressRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: ingressroutev1.IngressRouteSpec{
						VirtualHost: &ingressroutev1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []ingressroutev1.Route{{
							Match: "/",
							Services: []ingressroutev1.Service{{
								Name: "backend",
								Port: 80,
							}, {
								Name: "backend",
								Port: 8080,
							}},
						}},
					},
				},
				service("default", "backend", v1.ServicePort{
					Name:       "http",
					Protocol:   "TCP",
					Port:       80,
					TargetPort: intstr.FromInt(6502),
				}, v1.ServicePort{
					Name:       "alt",
					Protocol:   "TCP",
					Port:       8080,
					TargetPort: intstr.FromString("9001"),
				}),
			},
			want: clustermap(
				&v2.Cluster{
					Name: "default/backend/80",
					Type: v2.Cluster_EDS,
					EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
						EdsConfig:   apiconfigsource("contour"), // hard coded by initconfig
						ServiceName: "default/backend/http",
					},
					ConnectTimeout: 250 * time.Millisecond,
					LbPolicy:       v2.Cluster_ROUND_ROBIN,
				},
				&v2.Cluster{
					Name: "default/backend/8080",
					Type: v2.Cluster_EDS,
					EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
						EdsConfig:   apiconfigsource("contour"), // hard coded by initconfig
						ServiceName: "default/backend/alt",
					},
					ConnectTimeout: 250 * time.Millisecond,
					LbPolicy:       v2.Cluster_ROUND_ROBIN,
				},
			),
		},
		"ingressroute with simple path healthcheck": {
			objs: []interface{}{
				&ingressroutev1.IngressRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: ingressroutev1.IngressRouteSpec{
						VirtualHost: &ingressroutev1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []ingressroutev1.Route{{
							Match: "/",
							Services: []ingressroutev1.Service{{
								Name: "backend",
								Port: 80,
								HealthCheck: &ingressroutev1.HealthCheck{
									Path: "/healthy",
								},
							}},
						}},
					},
				},
				service("default", "backend", v1.ServicePort{
					Name:       "http",
					Protocol:   "TCP",
					Port:       80,
					TargetPort: intstr.FromInt(6502),
				}),
			},
			want: clustermap(
				&v2.Cluster{
					Name: "default/backend/80",
					Type: v2.Cluster_EDS,
					EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
						EdsConfig:   apiconfigsource("contour"), // hard coded by initconfig
						ServiceName: "default/backend/http",
					},
					ConnectTimeout: 250 * time.Millisecond,
					LbPolicy:       v2.Cluster_ROUND_ROBIN,
					HealthChecks: []*core.HealthCheck{{
						Timeout:  duration(hcTimeout),
						Interval: duration(hcInterval),
						UnhealthyThreshold: &types.UInt32Value{
							Value: hcUnhealthyThreshold,
						},
						HealthyThreshold: &types.UInt32Value{
							Value: hcHealthyThreshold,
						},
						HealthChecker: &core.HealthCheck_HttpHealthCheck_{
							HttpHealthCheck: &core.HealthCheck_HttpHealthCheck{
								Path: "/healthy",
								Host: "contour-envoy-healthcheck",
							},
						},
					}},
				},
			),
		},
		"ingressroute with custom healthcheck": {
			objs: []interface{}{
				&ingressroutev1.IngressRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: ingressroutev1.IngressRouteSpec{
						VirtualHost: &ingressroutev1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []ingressroutev1.Route{{
							Match: "/",
							Services: []ingressroutev1.Service{{
								Name: "backend",
								Port: 80,
								HealthCheck: &ingressroutev1.HealthCheck{
									Path:                    "/healthy",
									TimeoutSeconds:          99,
									IntervalSeconds:         98,
									UnhealthyThresholdCount: 97,
									HealthyThresholdCount:   96,
									Host:                    "foo-bar-host",
								},
							}},
						}},
					},
				},
				service("default", "backend", v1.ServicePort{
					Name:       "http",
					Protocol:   "TCP",
					Port:       80,
					TargetPort: intstr.FromInt(6502),
				}),
			},
			want: clustermap(
				&v2.Cluster{
					Name: "default/backend/80",
					Type: v2.Cluster_EDS,
					EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
						EdsConfig:   apiconfigsource("contour"), // hard coded by initconfig
						ServiceName: "default/backend/http",
					},
					ConnectTimeout: 250 * time.Millisecond,
					LbPolicy:       v2.Cluster_ROUND_ROBIN,
					HealthChecks: []*core.HealthCheck{{
						Timeout:  duration(99 * time.Second),
						Interval: duration(98 * time.Second),
						UnhealthyThreshold: &types.UInt32Value{
							Value: 97,
						},
						HealthyThreshold: &types.UInt32Value{
							Value: 96,
						},
						HealthChecker: &core.HealthCheck_HttpHealthCheck_{
							HttpHealthCheck: &core.HealthCheck_HttpHealthCheck{
								Path: "/healthy",
								Host: "foo-bar-host",
							},
						},
					}},
				},
			),
		},
		"ingressroute with RoundRobin lb algorithm": {
			objs: []interface{}{
				&ingressroutev1.IngressRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: ingressroutev1.IngressRouteSpec{
						VirtualHost: &ingressroutev1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []ingressroutev1.Route{{
							Match: "/",
							Services: []ingressroutev1.Service{{
								Name:     "backend",
								Port:     80,
								Strategy: "RoundRobin",
							}},
						}},
					},
				},
				service("default", "backend", v1.ServicePort{
					Name:       "http",
					Protocol:   "TCP",
					Port:       80,
					TargetPort: intstr.FromInt(6502),
				}),
			},
			want: clustermap(
				&v2.Cluster{
					Name: "default/backend/80",
					Type: v2.Cluster_EDS,
					EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
						EdsConfig:   apiconfigsource("contour"), // hard coded by initconfig
						ServiceName: "default/backend/http",
					},
					ConnectTimeout: 250 * time.Millisecond,
					LbPolicy:       v2.Cluster_ROUND_ROBIN,
				},
			),
		},
		"ingressroute with WeightedLeastRequest lb algorithm": {
			objs: []interface{}{
				&ingressroutev1.IngressRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: ingressroutev1.IngressRouteSpec{
						VirtualHost: &ingressroutev1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []ingressroutev1.Route{{
							Match: "/",
							Services: []ingressroutev1.Service{{
								Name:     "backend",
								Port:     80,
								Strategy: "WeightedLeastRequest",
							}},
						}},
					},
				},
				service("default", "backend", v1.ServicePort{
					Name:       "http",
					Protocol:   "TCP",
					Port:       80,
					TargetPort: intstr.FromInt(6502),
				}),
			},
			want: clustermap(
				&v2.Cluster{
					Name: "default/backend/80",
					Type: v2.Cluster_EDS,
					EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
						EdsConfig:   apiconfigsource("contour"), // hard coded by initconfig
						ServiceName: "default/backend/http",
					},
					ConnectTimeout: 250 * time.Millisecond,
					LbPolicy:       v2.Cluster_LEAST_REQUEST,
				},
			),
		},
		"ingressroute with RingHash lb algorithm": {
			objs: []interface{}{
				&ingressroutev1.IngressRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: ingressroutev1.IngressRouteSpec{
						VirtualHost: &ingressroutev1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []ingressroutev1.Route{{
							Match: "/",
							Services: []ingressroutev1.Service{{
								Name:     "backend",
								Port:     80,
								Strategy: "RingHash",
							}},
						}},
					},
				},
				service("default", "backend", v1.ServicePort{
					Name:       "http",
					Protocol:   "TCP",
					Port:       80,
					TargetPort: intstr.FromInt(6502),
				}),
			},
			want: clustermap(
				&v2.Cluster{
					Name: "default/backend/80",
					Type: v2.Cluster_EDS,
					EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
						EdsConfig:   apiconfigsource("contour"), // hard coded by initconfig
						ServiceName: "default/backend/http",
					},
					ConnectTimeout: 250 * time.Millisecond,
					LbPolicy:       v2.Cluster_RING_HASH,
				},
			),
		},
		"ingressroute with Maglev lb algorithm": {
			objs: []interface{}{
				&ingressroutev1.IngressRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: ingressroutev1.IngressRouteSpec{
						VirtualHost: &ingressroutev1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []ingressroutev1.Route{{
							Match: "/",
							Services: []ingressroutev1.Service{{
								Name:     "backend",
								Port:     80,
								Strategy: "Maglev",
							}},
						}},
					},
				},
				service("default", "backend", v1.ServicePort{
					Name:       "http",
					Protocol:   "TCP",
					Port:       80,
					TargetPort: intstr.FromInt(6502),
				}),
			},
			want: clustermap(
				&v2.Cluster{
					Name: "default/backend/80",
					Type: v2.Cluster_EDS,
					EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
						EdsConfig:   apiconfigsource("contour"), // hard coded by initconfig
						ServiceName: "default/backend/http",
					},
					ConnectTimeout: 250 * time.Millisecond,
					LbPolicy:       v2.Cluster_MAGLEV,
				},
			),
		},
		"ingressroute with Random lb algorithm": {
			objs: []interface{}{
				&ingressroutev1.IngressRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: ingressroutev1.IngressRouteSpec{
						VirtualHost: &ingressroutev1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []ingressroutev1.Route{{
							Match: "/",
							Services: []ingressroutev1.Service{{
								Name:     "backend",
								Port:     80,
								Strategy: "Random",
							}},
						}},
					},
				},
				service("default", "backend", v1.ServicePort{
					Name:       "http",
					Protocol:   "TCP",
					Port:       80,
					TargetPort: intstr.FromInt(6502),
				}),
			},
			want: clustermap(
				&v2.Cluster{
					Name: "default/backend/80",
					Type: v2.Cluster_EDS,
					EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
						EdsConfig:   apiconfigsource("contour"), // hard coded by initconfig
						ServiceName: "default/backend/http",
					},
					ConnectTimeout: 250 * time.Millisecond,
					LbPolicy:       v2.Cluster_RANDOM,
				},
			),
		},
		"ingressroute with unknown lb algorithm": {
			objs: []interface{}{
				&ingressroutev1.IngressRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: ingressroutev1.IngressRouteSpec{
						VirtualHost: &ingressroutev1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []ingressroutev1.Route{{
							Match: "/",
							Services: []ingressroutev1.Service{{
								Name:     "backend",
								Port:     80,
								Strategy: "lulz",
							}},
						}},
					},
				},
				service("default", "backend", v1.ServicePort{
					Name:       "http",
					Protocol:   "TCP",
					Port:       80,
					TargetPort: intstr.FromInt(6502),
				}),
			},
			want: clustermap(
				&v2.Cluster{
					Name: "default/backend/80",
					Type: v2.Cluster_EDS,
					EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
						EdsConfig:   apiconfigsource("contour"), // hard coded by initconfig
						ServiceName: "default/backend/http",
					},
					ConnectTimeout: 250 * time.Millisecond,
					LbPolicy:       v2.Cluster_ROUND_ROBIN,
				},
			),
		},
		"circuitbreaker annotations": {
			objs: []interface{}{
				&v1beta1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
					},
					Spec: v1beta1.IngressSpec{
						Backend: &v1beta1.IngressBackend{
							ServiceName: "kuard",
							ServicePort: intstr.FromString("http"),
						},
					},
				},
				serviceWithAnnotations(
					"default",
					"kuard",
					map[string]string{
						"contour.heptio.com/max-connections":      "9000",
						"contour.heptio.com/max-pending-requests": "4096",
						"contour.heptio.com/max-requests":         "404",
						"contour.heptio.com/max-retries":          "7",
					},
					v1.ServicePort{
						Protocol: "TCP",
						Name:     "http",
						Port:     80,
					},
				),
			},
			want: clustermap(
				&v2.Cluster{
					Name: "default/kuard/80",
					Type: v2.Cluster_EDS,
					EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
						EdsConfig:   apiconfigsource("contour"), // hard coded by initconfig
						ServiceName: "default/kuard/http",
					},
					ConnectTimeout: 250 * time.Millisecond,
					LbPolicy:       v2.Cluster_ROUND_ROBIN,
					CircuitBreakers: &cluster.CircuitBreakers{
						Thresholds: []*cluster.CircuitBreakers_Thresholds{{
							MaxConnections:     uint32t(9000),
							MaxPendingRequests: uint32t(4096),
							MaxRequests:        uint32t(404),
							MaxRetries:         uint32t(7),
						}},
					},
				},
			),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var d DAGAdapter
			for _, o := range tc.objs {
				d.OnAdd(o)
			}
			v := clusterVisitor{
				ClusterCache: new(ClusterCache),
				DAG:          &d.DAG,
			}
			got := v.Visit()
			if !reflect.DeepEqual(tc.want, got) {
				t.Fatalf("expected:\n%+v\ngot:\n%+v", tc.want, got)
			}
		})
	}
}

func uint32t(v int) *types.UInt32Value {
	return &types.UInt32Value{Value: uint32(v)}
}

func service(ns, name string, ports ...v1.ServicePort) *v1.Service {
	return serviceWithAnnotations(ns, name, nil, ports...)
}

func serviceWithAnnotations(ns, name string, annotations map[string]string, ports ...v1.ServicePort) *v1.Service {
	return &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   ns,
			Annotations: annotations,
		},
		Spec: v1.ServiceSpec{
			Ports: ports,
		},
	}
}

func clustermap(clusters ...*v2.Cluster) map[string]*v2.Cluster {
	m := make(map[string]*v2.Cluster)
	for _, c := range clusters {
		m[c.Name] = c
	}
	return m
}

func duration(d time.Duration) *types.Duration {
	return &types.Duration{
		Seconds: int64(d / time.Second),
	}
}

func TestServiceName(t *testing.T) {
	tests := map[string]struct {
		name, namespace string
		portname        string
		want            string
	}{
		"named service": {
			namespace: "default",
			name:      "kuard",
			portname:  "http",
			want:      "default/kuard/http",
		},
		"unnamed service": {
			namespace: "default",
			name:      "kuard",
			want:      "default/kuard",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := servicename(tc.namespace, tc.name, tc.portname)
			if got != tc.want {
				t.Fatalf("servicename(%s/%s, %q): want %q, got %q", tc.namespace, tc.name, tc.portname, tc.want, got)
			}
		})
	}
}
