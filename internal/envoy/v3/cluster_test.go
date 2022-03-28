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

	envoy_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_extensions_upstream_http_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	envoy_type "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/envoy"
	"github.com/projectcontour/contour/internal/protobuf"
	"github.com/projectcontour/contour/internal/timeout"
	"github.com/projectcontour/contour/internal/xds"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestCluster(t *testing.T) {
	s1 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       443,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}

	s2 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			ExternalName: "foo.io",
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       443,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}

	svcExternal := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			ExternalName: "projectcontour.local",
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       443,
				TargetPort: intstr.FromInt(8080),
			}},
			Type: v1.ServiceTypeExternalName,
		},
	}

	secret := &dag.Secret{
		Object: &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "secret",
				Namespace: "default",
			},
			Type: v1.SecretTypeTLS,
			Data: map[string][]byte{dag.CACertificateKey: []byte("cacert")},
		},
	}

	clientSecret := &dag.Secret{
		Object: &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "clientcertsecret",
				Namespace: "default",
			},
			Type: v1.SecretTypeTLS,
			Data: map[string][]byte{
				v1.TLSCertKey:       []byte("cert"),
				v1.TLSPrivateKeyKey: []byte("key"),
			},
		},
	}

	tests := map[string]struct {
		cluster *dag.Cluster
		want    *envoy_cluster_v3.Cluster
	}{
		"simple service": {
			cluster: &dag.Cluster{
				Upstream: service(s1),
			},
			want: &envoy_cluster_v3.Cluster{
				Name:                 "default/kuard/443/da39a3ee5e",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(envoy_cluster_v3.Cluster_EDS),
				EdsClusterConfig: &envoy_cluster_v3.Cluster_EdsClusterConfig{
					EdsConfig:   ConfigSource("contour"),
					ServiceName: "default/kuard/http",
				},
			},
		},
		"h2c upstream": {
			cluster: &dag.Cluster{
				Upstream: service(s1, "h2c"),
				Protocol: "h2c",
			},
			want: &envoy_cluster_v3.Cluster{
				Name:                 "default/kuard/443/f4f94965ec",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(envoy_cluster_v3.Cluster_EDS),
				EdsClusterConfig: &envoy_cluster_v3.Cluster_EdsClusterConfig{
					EdsConfig:   ConfigSource("contour"),
					ServiceName: "default/kuard/http",
				},
				TypedExtensionProtocolOptions: map[string]*any.Any{
					"envoy.extensions.upstreams.http.v3.HttpProtocolOptions": protobuf.MustMarshalAny(
						&envoy_extensions_upstream_http_v3.HttpProtocolOptions{
							UpstreamProtocolOptions: &envoy_extensions_upstream_http_v3.HttpProtocolOptions_ExplicitHttpConfig_{
								ExplicitHttpConfig: &envoy_extensions_upstream_http_v3.HttpProtocolOptions_ExplicitHttpConfig{
									ProtocolConfig: &envoy_extensions_upstream_http_v3.HttpProtocolOptions_ExplicitHttpConfig_Http2ProtocolOptions{},
								},
							},
						}),
				},
			},
		},
		"h2 upstream": {
			cluster: &dag.Cluster{
				Upstream: service(s1, "h2"),
				Protocol: "h2",
			},
			want: &envoy_cluster_v3.Cluster{
				Name:                 "default/kuard/443/bf1c365741",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(envoy_cluster_v3.Cluster_EDS),
				EdsClusterConfig: &envoy_cluster_v3.Cluster_EdsClusterConfig{
					EdsConfig:   ConfigSource("contour"),
					ServiceName: "default/kuard/http",
				},
				TransportSocket: UpstreamTLSTransportSocket(
					UpstreamTLSContext(nil, "", nil, "h2"),
				),
				TypedExtensionProtocolOptions: map[string]*any.Any{
					"envoy.extensions.upstreams.http.v3.HttpProtocolOptions": protobuf.MustMarshalAny(
						&envoy_extensions_upstream_http_v3.HttpProtocolOptions{
							UpstreamProtocolOptions: &envoy_extensions_upstream_http_v3.HttpProtocolOptions_ExplicitHttpConfig_{
								ExplicitHttpConfig: &envoy_extensions_upstream_http_v3.HttpProtocolOptions_ExplicitHttpConfig{
									ProtocolConfig: &envoy_extensions_upstream_http_v3.HttpProtocolOptions_ExplicitHttpConfig_Http2ProtocolOptions{},
								},
							},
						}),
				},
			},
		},
		"externalName service": {
			cluster: &dag.Cluster{
				Upstream: service(s2),
			},
			want: &envoy_cluster_v3.Cluster{
				Name:                 "default/kuard/443/da39a3ee5e",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(envoy_cluster_v3.Cluster_STRICT_DNS),
				LoadAssignment:       StaticClusterLoadAssignment(service(s2)),
			},
		},
		"externalName service - dns-lookup-family v4": {
			cluster: &dag.Cluster{
				Upstream:        service(s2),
				DNSLookupFamily: "v4",
			},
			want: &envoy_cluster_v3.Cluster{
				Name:                 "default/kuard/443/da39a3ee5e",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(envoy_cluster_v3.Cluster_STRICT_DNS),
				LoadAssignment:       StaticClusterLoadAssignment(service(s2)),
				DnsLookupFamily:      envoy_cluster_v3.Cluster_V4_ONLY,
			},
		},
		"externalName service - dns-lookup-family v6": {
			cluster: &dag.Cluster{
				Upstream:        service(s2),
				DNSLookupFamily: "v6",
			},
			want: &envoy_cluster_v3.Cluster{
				Name:                 "default/kuard/443/da39a3ee5e",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(envoy_cluster_v3.Cluster_STRICT_DNS),
				LoadAssignment:       StaticClusterLoadAssignment(service(s2)),
				DnsLookupFamily:      envoy_cluster_v3.Cluster_V6_ONLY,
			},
		},
		"externalName service - dns-lookup-family auto": {
			cluster: &dag.Cluster{
				Upstream:        service(s2),
				DNSLookupFamily: "auto",
			},
			want: &envoy_cluster_v3.Cluster{
				Name:                 "default/kuard/443/da39a3ee5e",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(envoy_cluster_v3.Cluster_STRICT_DNS),
				LoadAssignment:       StaticClusterLoadAssignment(service(s2)),
				DnsLookupFamily:      envoy_cluster_v3.Cluster_AUTO,
			},
		},
		"externalName service - dns-lookup-family not defined": {
			cluster: &dag.Cluster{
				Upstream: service(s2),
				//DNSLookupFamily: "auto",
			},
			want: &envoy_cluster_v3.Cluster{
				Name:                 "default/kuard/443/da39a3ee5e",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(envoy_cluster_v3.Cluster_STRICT_DNS),
				LoadAssignment:       StaticClusterLoadAssignment(service(s2)),
				DnsLookupFamily:      envoy_cluster_v3.Cluster_AUTO,
			},
		},
		"tls upstream": {
			cluster: &dag.Cluster{
				Upstream: service(s1, "tls"),
				Protocol: "tls",
			},
			want: &envoy_cluster_v3.Cluster{
				Name:                 "default/kuard/443/4929fca9d4",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(envoy_cluster_v3.Cluster_EDS),
				EdsClusterConfig: &envoy_cluster_v3.Cluster_EdsClusterConfig{
					EdsConfig:   ConfigSource("contour"),
					ServiceName: "default/kuard/http",
				},
				TransportSocket: UpstreamTLSTransportSocket(
					UpstreamTLSContext(nil, "", nil),
				),
			},
		},
		"tls upstream - external name": {
			cluster: &dag.Cluster{
				Upstream: service(svcExternal, "tls"),
				Protocol: "tls",
				SNI:      "projectcontour.local",
			},
			want: &envoy_cluster_v3.Cluster{
				Name:                 "default/kuard/443/a996a742af",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(envoy_cluster_v3.Cluster_STRICT_DNS),
				LoadAssignment:       StaticClusterLoadAssignment(service(svcExternal, "tls")),
				TransportSocket: UpstreamTLSTransportSocket(
					UpstreamTLSContext(nil, "projectcontour.local", nil),
				),
			},
		},
		"verify tls upstream with san": {
			cluster: &dag.Cluster{
				Upstream: service(s1, "tls"),
				Protocol: "tls",
				UpstreamValidation: &dag.PeerValidationContext{
					CACertificate: secret,
					SubjectName:   "foo.bar.io",
				},
			},
			want: &envoy_cluster_v3.Cluster{
				Name:                 "default/kuard/443/62d1f9ad02",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(envoy_cluster_v3.Cluster_EDS),
				EdsClusterConfig: &envoy_cluster_v3.Cluster_EdsClusterConfig{
					EdsConfig:   ConfigSource("contour"),
					ServiceName: "default/kuard/http",
				},
				TransportSocket: UpstreamTLSTransportSocket(
					UpstreamTLSContext(
						&dag.PeerValidationContext{
							CACertificate: secret,
							SubjectName:   "foo.bar.io",
						},
						"",
						nil),
				),
			},
		},
		"projectcontour.io/max-connections": {
			cluster: &dag.Cluster{
				Upstream: &dag.Service{
					MaxConnections: 9000,
					Weighted: dag.WeightedService{
						Weight:           1,
						ServiceName:      s1.Name,
						ServiceNamespace: s1.Namespace,
						ServicePort:      s1.Spec.Ports[0],
					},
				},
			},
			want: &envoy_cluster_v3.Cluster{
				Name:                 "default/kuard/443/da39a3ee5e",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(envoy_cluster_v3.Cluster_EDS),
				EdsClusterConfig: &envoy_cluster_v3.Cluster_EdsClusterConfig{
					EdsConfig:   ConfigSource("contour"),
					ServiceName: "default/kuard/http",
				},
				CircuitBreakers: &envoy_cluster_v3.CircuitBreakers{
					Thresholds: []*envoy_cluster_v3.CircuitBreakers_Thresholds{{
						MaxConnections: protobuf.UInt32(9000),
					}},
				},
			},
		},
		"projectcontour.io/max-pending-requests": {
			cluster: &dag.Cluster{
				Upstream: &dag.Service{
					MaxPendingRequests: 4096,
					Weighted: dag.WeightedService{
						Weight:           1,
						ServiceName:      s1.Name,
						ServiceNamespace: s1.Namespace,
						ServicePort:      s1.Spec.Ports[0],
					},
				},
			},
			want: &envoy_cluster_v3.Cluster{
				Name:                 "default/kuard/443/da39a3ee5e",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(envoy_cluster_v3.Cluster_EDS),
				EdsClusterConfig: &envoy_cluster_v3.Cluster_EdsClusterConfig{
					EdsConfig:   ConfigSource("contour"),
					ServiceName: "default/kuard/http",
				},
				CircuitBreakers: &envoy_cluster_v3.CircuitBreakers{
					Thresholds: []*envoy_cluster_v3.CircuitBreakers_Thresholds{{
						MaxPendingRequests: protobuf.UInt32(4096),
					}},
				},
			},
		},
		"projectcontour.io/max-requests": {
			cluster: &dag.Cluster{
				Upstream: &dag.Service{
					MaxRequests: 404,
					Weighted: dag.WeightedService{
						Weight:           1,
						ServiceName:      s1.Name,
						ServiceNamespace: s1.Namespace,
						ServicePort:      s1.Spec.Ports[0],
					},
				},
			},
			want: &envoy_cluster_v3.Cluster{
				Name:                 "default/kuard/443/da39a3ee5e",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(envoy_cluster_v3.Cluster_EDS),
				EdsClusterConfig: &envoy_cluster_v3.Cluster_EdsClusterConfig{
					EdsConfig:   ConfigSource("contour"),
					ServiceName: "default/kuard/http",
				},
				CircuitBreakers: &envoy_cluster_v3.CircuitBreakers{
					Thresholds: []*envoy_cluster_v3.CircuitBreakers_Thresholds{{
						MaxRequests: protobuf.UInt32(404),
					}},
				},
			},
		},
		"projectcontour.io/max-retries": {
			cluster: &dag.Cluster{
				Upstream: &dag.Service{
					MaxRetries: 7,
					Weighted: dag.WeightedService{
						Weight:           1,
						ServiceName:      s1.Name,
						ServiceNamespace: s1.Namespace,
						ServicePort:      s1.Spec.Ports[0],
					},
				},
			},
			want: &envoy_cluster_v3.Cluster{
				Name:                 "default/kuard/443/da39a3ee5e",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(envoy_cluster_v3.Cluster_EDS),
				EdsClusterConfig: &envoy_cluster_v3.Cluster_EdsClusterConfig{
					EdsConfig:   ConfigSource("contour"),
					ServiceName: "default/kuard/http",
				},
				CircuitBreakers: &envoy_cluster_v3.CircuitBreakers{
					Thresholds: []*envoy_cluster_v3.CircuitBreakers_Thresholds{{
						MaxRetries: protobuf.UInt32(7),
					}},
				},
			},
		},
		"cluster with random load balancer policy": {
			cluster: &dag.Cluster{
				Upstream:           service(s1),
				LoadBalancerPolicy: "Random",
			},
			want: &envoy_cluster_v3.Cluster{
				Name:                 "default/kuard/443/58d888c08a",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(envoy_cluster_v3.Cluster_EDS),
				EdsClusterConfig: &envoy_cluster_v3.Cluster_EdsClusterConfig{
					EdsConfig:   ConfigSource("contour"),
					ServiceName: "default/kuard/http",
				},
				LbPolicy: envoy_cluster_v3.Cluster_RANDOM,
			},
		},
		"cluster with cookie policy": {
			cluster: &dag.Cluster{
				Upstream:           service(s1),
				LoadBalancerPolicy: "Cookie",
			},
			want: &envoy_cluster_v3.Cluster{
				Name:                 "default/kuard/443/e4f81994fe",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(envoy_cluster_v3.Cluster_EDS),
				EdsClusterConfig: &envoy_cluster_v3.Cluster_EdsClusterConfig{
					EdsConfig:   ConfigSource("contour"),
					ServiceName: "default/kuard/http",
				},
				LbPolicy: envoy_cluster_v3.Cluster_RING_HASH,
			},
		},

		"tcp service": {
			cluster: &dag.Cluster{
				Upstream: service(s1),
			},
			want: &envoy_cluster_v3.Cluster{
				Name:                 "default/kuard/443/da39a3ee5e",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(envoy_cluster_v3.Cluster_EDS),
				EdsClusterConfig: &envoy_cluster_v3.Cluster_EdsClusterConfig{
					EdsConfig:   ConfigSource("contour"),
					ServiceName: "default/kuard/http",
				},
			},
		},
		"tcp service with healthcheck": {
			cluster: &dag.Cluster{
				Upstream: service(s1),
				TCPHealthCheckPolicy: &dag.TCPHealthCheckPolicy{
					Timeout:            2,
					Interval:           10,
					UnhealthyThreshold: 3,
					HealthyThreshold:   2,
				},
			},
			want: &envoy_cluster_v3.Cluster{
				Name:                 "default/kuard/443/da39a3ee5e",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(envoy_cluster_v3.Cluster_EDS),
				EdsClusterConfig: &envoy_cluster_v3.Cluster_EdsClusterConfig{
					EdsConfig:   ConfigSource("contour"),
					ServiceName: "default/kuard/http",
				},
				IgnoreHealthOnHostRemoval: true,
				HealthChecks: []*envoy_core_v3.HealthCheck{{
					Timeout:            durationOrDefault(2, envoy.HCTimeout),
					Interval:           durationOrDefault(10, envoy.HCInterval),
					UnhealthyThreshold: protobuf.UInt32OrDefault(3, envoy.HCUnhealthyThreshold),
					HealthyThreshold:   protobuf.UInt32OrDefault(2, envoy.HCHealthyThreshold),
					HealthChecker: &envoy_core_v3.HealthCheck_TcpHealthCheck_{
						TcpHealthCheck: &envoy_core_v3.HealthCheck_TcpHealthCheck{},
					},
				}},
			},
		},
		"use client certificate to authentication towards backend": {
			cluster: &dag.Cluster{
				Upstream:          service(s1, "tls"),
				Protocol:          "tls",
				ClientCertificate: clientSecret,
			},
			want: &envoy_cluster_v3.Cluster{
				Name:                 "default/kuard/443/4929fca9d4",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(envoy_cluster_v3.Cluster_EDS),
				EdsClusterConfig: &envoy_cluster_v3.Cluster_EdsClusterConfig{
					EdsConfig:   ConfigSource("contour"),
					ServiceName: "default/kuard/http",
				},
				TransportSocket: UpstreamTLSTransportSocket(
					UpstreamTLSContext(nil, "", clientSecret),
				),
			},
		},
		"cluster with connect timeout set": {
			cluster: &dag.Cluster{
				Upstream:      service(s1),
				TimeoutPolicy: dag.ClusterTimeoutPolicy{ConnectTimeout: 10 * time.Second},
			},
			want: &envoy_cluster_v3.Cluster{
				Name:                 "default/kuard/443/da39a3ee5e",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(envoy_cluster_v3.Cluster_EDS),
				EdsClusterConfig: &envoy_cluster_v3.Cluster_EdsClusterConfig{
					EdsConfig:   ConfigSource("contour"),
					ServiceName: "default/kuard/http",
				},
				ConnectTimeout: protobuf.Duration(10 * time.Second),
			},
		},
		"cluster with idle connection timeout set": {
			cluster: &dag.Cluster{
				Upstream:      service(s1),
				TimeoutPolicy: dag.ClusterTimeoutPolicy{IdleConnectionTimeout: timeout.DurationSetting(10 * time.Second)},
			},
			want: &envoy_cluster_v3.Cluster{
				Name:                 "default/kuard/443/357c84df09",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(envoy_cluster_v3.Cluster_EDS),
				EdsClusterConfig: &envoy_cluster_v3.Cluster_EdsClusterConfig{
					EdsConfig:   ConfigSource("contour"),
					ServiceName: "default/kuard/http",
				},
				TypedExtensionProtocolOptions: map[string]*any.Any{
					"envoy.extensions.upstreams.http.v3.HttpProtocolOptions": protobuf.MustMarshalAny(
						&envoy_extensions_upstream_http_v3.HttpProtocolOptions{
							CommonHttpProtocolOptions: &envoy_core_v3.HttpProtocolOptions{
								IdleTimeout: protobuf.Duration(10 * time.Second),
							},
							UpstreamProtocolOptions: &envoy_extensions_upstream_http_v3.HttpProtocolOptions_ExplicitHttpConfig_{
								ExplicitHttpConfig: &envoy_extensions_upstream_http_v3.HttpProtocolOptions_ExplicitHttpConfig{
									ProtocolConfig: &envoy_extensions_upstream_http_v3.HttpProtocolOptions_ExplicitHttpConfig_HttpProtocolOptions{},
								},
							},
						},
					),
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := Cluster(tc.cluster)
			want := clusterDefaults()

			proto.Merge(want, tc.want)

			protobuf.ExpectEqual(t, want, got)
		})
	}
}

func TestClusterLoadAssignmentName(t *testing.T) {
	assert.Equal(t, xds.ClusterLoadAssignmentName(types.NamespacedName{Namespace: "ns", Name: "svc"}, "port"), "ns/svc/port")
	assert.Equal(t, xds.ClusterLoadAssignmentName(types.NamespacedName{Namespace: "ns", Name: "svc"}, ""), "ns/svc")
	assert.Equal(t, xds.ClusterLoadAssignmentName(types.NamespacedName{}, ""), "/")
}

func TestClustername(t *testing.T) {
	type testcase struct {
		cluster *dag.Cluster
		want    string
	}

	run := func(t *testing.T, name string, tc testcase) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			t.Helper()
			got := envoy.Clustername(tc.cluster)
			assert.Equal(t, tc.want, got)
		})
	}

	run(t, "simple", testcase{
		cluster: &dag.Cluster{
			Upstream: &dag.Service{
				Weighted: dag.WeightedService{
					Weight:           1,
					ServiceName:      "backend",
					ServiceNamespace: "default",
					ServicePort: v1.ServicePort{
						Name:       "http",
						Protocol:   "TCP",
						Port:       80,
						TargetPort: intstr.FromInt(6502),
					},
				},
			},
		},
		want: "default/backend/80/da39a3ee5e",
	})

	run(t, "far too long", testcase{
		cluster: &dag.Cluster{
			Upstream: &dag.Service{
				Weighted: dag.WeightedService{
					Weight:           1,
					ServiceName:      "must-be-in-want-of-a-wife",
					ServiceNamespace: "it-is-a-truth-universally-acknowledged-that-a-single-man-in-possession-of-a-good-fortune",
					ServicePort: v1.ServicePort{
						Name:       "http",
						Protocol:   "TCP",
						Port:       9999,
						TargetPort: intstr.FromString("http-alt"),
					},
				},
			},
		},
		want: "it-is-a--dea8b0/must-be--dea8b0/9999/da39a3ee5e",
	})

	run(t, "various healthcheck params", testcase{
		cluster: &dag.Cluster{
			Upstream: &dag.Service{
				Weighted: dag.WeightedService{
					Weight:           1,
					ServiceName:      "backend",
					ServiceNamespace: "default",
					ServicePort: v1.ServicePort{
						Name:       "http",
						Protocol:   "TCP",
						Port:       80,
						TargetPort: intstr.FromInt(6502),
					},
				},
			},
			LoadBalancerPolicy: "Random",
			HTTPHealthCheckPolicy: &dag.HTTPHealthCheckPolicy{
				Path:               "/healthz",
				Interval:           5 * time.Second,
				Timeout:            30 * time.Second,
				UnhealthyThreshold: 3,
				HealthyThreshold:   1,
			},
		},
		want: "default/backend/80/5c26077e1d",
	})

	cluster1 := &dag.Cluster{
		Upstream: &dag.Service{
			Weighted: dag.WeightedService{
				Weight:           1,
				ServiceName:      "backend",
				ServiceNamespace: "default",
				ServicePort: v1.ServicePort{
					Name:       "http",
					Protocol:   "TCP",
					Port:       80,
					TargetPort: intstr.FromInt(6502),
				},
			},
		},
		LoadBalancerPolicy: "Random",
		UpstreamValidation: &dag.PeerValidationContext{
			CACertificate: &dag.Secret{
				Object: &v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret",
						Namespace: "default",
					},
					Data: map[string][]byte{
						dag.CACertificateKey: []byte("somethingsecret"),
					},
				},
			},
			SubjectName: "foo.com",
		},
	}

	run(t, "upstream tls validation with subject alt name", testcase{
		cluster: cluster1,
		want:    "default/backend/80/6bf46b7b3a",
	})

	cluster1.SNI = "foo.bar"
	run(t, "upstream tls validation with subject alt name with SNI", testcase{
		cluster: cluster1,
		want:    "default/backend/80/b8a2ccb774",
	})

	cluster1.Protocol = "h2"
	run(t, "upstream tls validation with subject alt name with Protocol", testcase{
		cluster: cluster1,
		want:    "default/backend/80/50abc1400c",
	})

}

func TestLBPolicy(t *testing.T) {
	tests := map[string]envoy_cluster_v3.Cluster_LbPolicy{
		"WeightedLeastRequest": envoy_cluster_v3.Cluster_LEAST_REQUEST,
		"Random":               envoy_cluster_v3.Cluster_RANDOM,
		"RoundRobin":           envoy_cluster_v3.Cluster_ROUND_ROBIN,
		"":                     envoy_cluster_v3.Cluster_ROUND_ROBIN,
		"unknown":              envoy_cluster_v3.Cluster_ROUND_ROBIN,
		"Cookie":               envoy_cluster_v3.Cluster_RING_HASH,
		"RequestHash":          envoy_cluster_v3.Cluster_RING_HASH,

		// RingHash and Maglev were removed as options in 0.13.
		// See #1150
		"RingHash": envoy_cluster_v3.Cluster_ROUND_ROBIN,
		"Maglev":   envoy_cluster_v3.Cluster_ROUND_ROBIN,
	}

	for policy, want := range tests {
		t.Run(policy, func(t *testing.T) {
			got := lbPolicy(policy)
			assert.Equal(t, want, got)
		})
	}
}

func TestClusterCommonLBConfig(t *testing.T) {
	got := ClusterCommonLBConfig()
	want := &envoy_cluster_v3.Cluster_CommonLbConfig{
		HealthyPanicThreshold: &envoy_type.Percent{ // Disable HealthyPanicThreshold
			Value: 0,
		},
	}
	assert.Equal(t, want, got)
}

func service(s *v1.Service, protocols ...string) *dag.Service {
	protocol := ""
	if len(protocols) > 0 {
		protocol = protocols[0]
	}
	return &dag.Service{
		Weighted: dag.WeightedService{
			Weight:           1,
			ServiceName:      s.Name,
			ServiceNamespace: s.Namespace,
			ServicePort:      s.Spec.Ports[0],
		},
		ExternalName: s.Spec.ExternalName,
		Protocol:     protocol,
	}
}
