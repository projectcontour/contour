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

package envoy

import (
	"testing"
	"time"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_cluster "github.com/envoyproxy/go-control-plane/envoy/api/v2/cluster"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoy_type "github.com/envoyproxy/go-control-plane/envoy/type"
	"github.com/gogo/protobuf/types"
	"github.com/google/go-cmp/cmp"
	ingressroutev1 "github.com/heptio/contour/apis/contour/v1beta1"
	"github.com/heptio/contour/internal/dag"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	tests := map[string]struct {
		cluster *dag.Cluster
		want    *v2.Cluster
	}{
		"simple service": {
			cluster: &dag.Cluster{
				Upstream: &dag.HTTPService{
					TCPService: service(s1),
				},
			},
			want: &v2.Cluster{
				Name:                 "default/kuard/443/da39a3ee5e",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(v2.Cluster_EDS),
				EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
					EdsConfig:   ConfigSource("contour"),
					ServiceName: "default/kuard/http",
				},
				ConnectTimeout: 250 * time.Millisecond,
				LbPolicy:       v2.Cluster_ROUND_ROBIN,
				CommonLbConfig: ClusterCommonLBConfig(),
			},
		},
		"h2c upstream": {
			cluster: &dag.Cluster{
				Upstream: &dag.HTTPService{
					TCPService: service(s1),
					Protocol:   "h2c",
				},
			},
			want: &v2.Cluster{
				Name:                 "default/kuard/443/da39a3ee5e",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(v2.Cluster_EDS),
				EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
					EdsConfig:   ConfigSource("contour"),
					ServiceName: "default/kuard/http",
				},
				ConnectTimeout:       250 * time.Millisecond,
				LbPolicy:             v2.Cluster_ROUND_ROBIN,
				Http2ProtocolOptions: &core.Http2ProtocolOptions{},
				CommonLbConfig:       ClusterCommonLBConfig(),
			},
		},
		"h2 upstream": {
			cluster: &dag.Cluster{
				Upstream: &dag.HTTPService{
					TCPService: service(s1),
					Protocol:   "h2",
				},
			},
			want: &v2.Cluster{
				Name:                 "default/kuard/443/da39a3ee5e",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(v2.Cluster_EDS),
				EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
					EdsConfig:   ConfigSource("contour"),
					ServiceName: "default/kuard/http",
				},
				ConnectTimeout:       250 * time.Millisecond,
				LbPolicy:             v2.Cluster_ROUND_ROBIN,
				TlsContext:           UpstreamTLSContext(nil, "", "h2"),
				Http2ProtocolOptions: &core.Http2ProtocolOptions{},
				CommonLbConfig:       ClusterCommonLBConfig(),
			},
		},
		"externalName service": {
			cluster: &dag.Cluster{
				Upstream: &dag.HTTPService{
					TCPService: *externalnameservice(s2),
				},
			},
			want: &v2.Cluster{
				Name:                 "default/kuard/443/da39a3ee5e",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(v2.Cluster_STRICT_DNS),
				LoadAssignment:       StaticClusterLoadAssignment(externalnameservice(s2)),
				ConnectTimeout:       250 * time.Millisecond,
				LbPolicy:             v2.Cluster_ROUND_ROBIN,
				CommonLbConfig:       ClusterCommonLBConfig(),
			},
		},
		"tls upstream": {
			cluster: &dag.Cluster{
				Upstream: &dag.HTTPService{
					TCPService: service(s1),
					Protocol:   "tls",
				},
			},
			want: &v2.Cluster{
				Name:                 "default/kuard/443/da39a3ee5e",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(v2.Cluster_EDS),
				EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
					EdsConfig:   ConfigSource("contour"),
					ServiceName: "default/kuard/http",
				},
				ConnectTimeout: 250 * time.Millisecond,
				LbPolicy:       v2.Cluster_ROUND_ROBIN,
				TlsContext:     UpstreamTLSContext(nil, ""),
				CommonLbConfig: ClusterCommonLBConfig(),
			},
		},
		"verify tls upstream with san": {
			cluster: &dag.Cluster{
				Upstream: &dag.HTTPService{
					TCPService: tlsservice(s1, "cacert", "foo.bar.io"),
					Protocol:   "tls",
				},
				UpstreamValidation: &dag.UpstreamValidation{
					CACertificate: &dag.Secret{
						Object: &v1.Secret{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "secret",
								Namespace: "default",
							},
							Data: map[string][]byte{
								"ca.crt": []byte("cacert"),
							},
						},
					},
					SubjectName: "foo.bar.io",
				},
			},
			want: &v2.Cluster{
				Name:                 "default/kuard/443/3ac4e90987",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(v2.Cluster_EDS),
				EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
					EdsConfig:   ConfigSource("contour"),
					ServiceName: "default/kuard/http",
				},
				ConnectTimeout: 250 * time.Millisecond,
				LbPolicy:       v2.Cluster_ROUND_ROBIN,
				TlsContext:     UpstreamTLSContext([]byte("cacert"), "foo.bar.io"),
				CommonLbConfig: ClusterCommonLBConfig(),
			},
		},
		"contour.heptio.com/max-connections": {
			cluster: &dag.Cluster{
				Upstream: &dag.HTTPService{
					TCPService: dag.TCPService{
						Name: s1.Name, Namespace: s1.Namespace,
						ServicePort:    &s1.Spec.Ports[0],
						MaxConnections: 9000,
					},
				},
			},
			want: &v2.Cluster{
				Name:                 "default/kuard/443/da39a3ee5e",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(v2.Cluster_EDS),
				EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
					EdsConfig:   ConfigSource("contour"),
					ServiceName: "default/kuard/http",
				},
				ConnectTimeout: 250 * time.Millisecond,
				LbPolicy:       v2.Cluster_ROUND_ROBIN,
				CircuitBreakers: &envoy_cluster.CircuitBreakers{
					Thresholds: []*envoy_cluster.CircuitBreakers_Thresholds{{
						MaxConnections: u32(9000),
					}},
				},
				CommonLbConfig: ClusterCommonLBConfig(),
			},
		},
		"contour.heptio.com/max-pending-requests": {
			cluster: &dag.Cluster{
				Upstream: &dag.HTTPService{
					TCPService: dag.TCPService{
						Name: s1.Name, Namespace: s1.Namespace,
						ServicePort:        &s1.Spec.Ports[0],
						MaxPendingRequests: 4096,
					},
				},
			},
			want: &v2.Cluster{
				Name:                 "default/kuard/443/da39a3ee5e",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(v2.Cluster_EDS),
				EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
					EdsConfig:   ConfigSource("contour"),
					ServiceName: "default/kuard/http",
				},
				ConnectTimeout: 250 * time.Millisecond,
				LbPolicy:       v2.Cluster_ROUND_ROBIN,
				CircuitBreakers: &envoy_cluster.CircuitBreakers{
					Thresholds: []*envoy_cluster.CircuitBreakers_Thresholds{{
						MaxPendingRequests: u32(4096),
					}},
				},
				CommonLbConfig: ClusterCommonLBConfig(),
			},
		},
		"contour.heptio.com/max-requests": {
			cluster: &dag.Cluster{
				Upstream: &dag.HTTPService{
					TCPService: dag.TCPService{
						Name: s1.Name, Namespace: s1.Namespace,
						ServicePort: &s1.Spec.Ports[0],
						MaxRequests: 404,
					},
				},
			},
			want: &v2.Cluster{
				Name:                 "default/kuard/443/da39a3ee5e",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(v2.Cluster_EDS),
				EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
					EdsConfig:   ConfigSource("contour"),
					ServiceName: "default/kuard/http",
				},
				ConnectTimeout: 250 * time.Millisecond,
				LbPolicy:       v2.Cluster_ROUND_ROBIN,
				CircuitBreakers: &envoy_cluster.CircuitBreakers{
					Thresholds: []*envoy_cluster.CircuitBreakers_Thresholds{{
						MaxRequests: u32(404),
					}},
				},
				CommonLbConfig: ClusterCommonLBConfig(),
			},
		},
		"contour.heptio.com/max-retries": {
			cluster: &dag.Cluster{
				Upstream: &dag.HTTPService{
					TCPService: dag.TCPService{
						Name: s1.Name, Namespace: s1.Namespace,
						ServicePort: &s1.Spec.Ports[0],
						MaxRetries:  7,
					},
				},
			},
			want: &v2.Cluster{
				Name:                 "default/kuard/443/da39a3ee5e",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(v2.Cluster_EDS),
				EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
					EdsConfig:   ConfigSource("contour"),
					ServiceName: "default/kuard/http",
				},
				ConnectTimeout: 250 * time.Millisecond,
				LbPolicy:       v2.Cluster_ROUND_ROBIN,
				CircuitBreakers: &envoy_cluster.CircuitBreakers{
					Thresholds: []*envoy_cluster.CircuitBreakers_Thresholds{{
						MaxRetries: u32(7),
					}},
				},
				CommonLbConfig: ClusterCommonLBConfig(),
			},
		},
		"cluster with random load balancer policy": {
			cluster: &dag.Cluster{
				Upstream: &dag.HTTPService{
					TCPService: service(s1),
				},
				LoadBalancerStrategy: "Random",
			},
			want: &v2.Cluster{
				Name:                 "default/kuard/443/58d888c08a",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(v2.Cluster_EDS),
				EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
					EdsConfig:   ConfigSource("contour"),
					ServiceName: "default/kuard/http",
				},
				ConnectTimeout: 250 * time.Millisecond,
				LbPolicy:       v2.Cluster_RANDOM,
				CommonLbConfig: ClusterCommonLBConfig(),
			},
		},
		"cluster with cookie policy": {
			cluster: &dag.Cluster{
				Upstream: &dag.HTTPService{
					TCPService: service(s1),
				},
				LoadBalancerStrategy: "Cookie",
			},
			want: &v2.Cluster{
				Name:                 "default/kuard/443/e4f81994fe",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(v2.Cluster_EDS),
				EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
					EdsConfig:   ConfigSource("contour"),
					ServiceName: "default/kuard/http",
				},
				ConnectTimeout: 250 * time.Millisecond,
				LbPolicy:       v2.Cluster_RING_HASH,
				CommonLbConfig: ClusterCommonLBConfig(),
			},
		},

		"tcp service": {
			cluster: &dag.Cluster{
				Upstream: &dag.TCPService{
					Name: s1.Name, Namespace: s1.Namespace,
					ServicePort: &s1.Spec.Ports[0],
				},
			},
			want: &v2.Cluster{
				Name:                 "default/kuard/443/da39a3ee5e",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(v2.Cluster_EDS),
				EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
					EdsConfig:   ConfigSource("contour"),
					ServiceName: "default/kuard/http",
				},
				ConnectTimeout: 250 * time.Millisecond,
				LbPolicy:       v2.Cluster_ROUND_ROBIN,
				CommonLbConfig: ClusterCommonLBConfig(),
			},
		},
		"tcp service with healthcheck": {
			cluster: &dag.Cluster{
				Upstream: &dag.TCPService{
					Name: s1.Name, Namespace: s1.Namespace,
					ServicePort: &s1.Spec.Ports[0],
				},
				HealthCheck: &ingressroutev1.HealthCheck{
					Path: "/healthz",
				},
			},
			want: &v2.Cluster{
				Name:                 "default/kuard/443/bc862a33ca",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(v2.Cluster_EDS),
				EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
					EdsConfig:   ConfigSource("contour"),
					ServiceName: "default/kuard/http",
				},
				ConnectTimeout:                250 * time.Millisecond,
				LbPolicy:                      v2.Cluster_ROUND_ROBIN,
				CommonLbConfig:                ClusterCommonLBConfig(),
				DrainConnectionsOnHostRemoval: true,
				HealthChecks: []*core.HealthCheck{
					{
						Timeout:            secondsOrDefault(0, hcTimeout),
						Interval:           secondsOrDefault(0, hcInterval),
						UnhealthyThreshold: countOrDefault(0, hcUnhealthyThreshold),
						HealthyThreshold:   countOrDefault(0, hcHealthyThreshold),
						HealthChecker: &core.HealthCheck_HttpHealthCheck_{
							HttpHealthCheck: &core.HealthCheck_HttpHealthCheck{
								Host: hcHost,
								Path: "/healthz",
							},
						},
					},
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := Cluster(tc.cluster)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func TestClustername(t *testing.T) {
	tests := map[string]struct {
		cluster *dag.Cluster
		want    string
	}{
		"simple": {
			cluster: &dag.Cluster{
				Upstream: &dag.TCPService{
					Name:      "backend",
					Namespace: "default",
					ServicePort: &v1.ServicePort{
						Name:       "http",
						Protocol:   "TCP",
						Port:       80,
						TargetPort: intstr.FromInt(6502),
					},
				},
			},
			want: "default/backend/80/da39a3ee5e",
		},
		"far too long": {
			cluster: &dag.Cluster{
				Upstream: &dag.TCPService{
					Name:      "must-be-in-want-of-a-wife",
					Namespace: "it-is-a-truth-universally-acknowledged-that-a-single-man-in-possession-of-a-good-fortune",
					ServicePort: &v1.ServicePort{
						Name:       "http",
						Protocol:   "TCP",
						Port:       9999,
						TargetPort: intstr.FromString("http-alt"),
					},
				},
			},
			want: "it-is-a--dea8b0/must-be--dea8b0/9999/da39a3ee5e",
		},
		"various healthcheck params": {
			cluster: &dag.Cluster{
				Upstream: &dag.TCPService{
					Name:      "backend",
					Namespace: "default",
					ServicePort: &v1.ServicePort{
						Name:       "http",
						Protocol:   "TCP",
						Port:       80,
						TargetPort: intstr.FromInt(6502),
					},
				},
				LoadBalancerStrategy: "Random",
				HealthCheck: &ingressroutev1.HealthCheck{
					Path:                    "/healthz",
					IntervalSeconds:         5,
					TimeoutSeconds:          30,
					UnhealthyThresholdCount: 3,
					HealthyThresholdCount:   1,
				},
			},
			want: "default/backend/80/5c26077e1d",
		},
		"upstream tls validation with subject alt name": {
			cluster: &dag.Cluster{
				Upstream: &dag.TCPService{
					Name:      "backend",
					Namespace: "default",
					ServicePort: &v1.ServicePort{
						Name:       "http",
						Protocol:   "TCP",
						Port:       80,
						TargetPort: intstr.FromInt(6502),
					},
				},
				LoadBalancerStrategy: "Random",
				UpstreamValidation: &dag.UpstreamValidation{
					CACertificate: &dag.Secret{
						Object: &v1.Secret{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "secret",
								Namespace: "default",
							},
							Data: map[string][]byte{
								"ca.crt": []byte("somethingsecret"),
							},
						},
					},
					SubjectName: "foo.com",
				},
			},
			want: "default/backend/80/6bf46b7b3a",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := Clustername(tc.cluster)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func TestLBPolicy(t *testing.T) {
	tests := map[string]v2.Cluster_LbPolicy{
		"WeightedLeastRequest": v2.Cluster_LEAST_REQUEST,
		"Random":               v2.Cluster_RANDOM,
		"":                     v2.Cluster_ROUND_ROBIN,
		"unknown":              v2.Cluster_ROUND_ROBIN,
		"Cookie":               v2.Cluster_RING_HASH,

		// RingHash and Maglev were removed as options in 0.13.
		// See #1150
		"RingHash": v2.Cluster_ROUND_ROBIN,
		"Maglev":   v2.Cluster_ROUND_ROBIN,
	}

	for strategy, want := range tests {
		t.Run(strategy, func(t *testing.T) {
			got := lbPolicy(strategy)
			if diff := cmp.Diff(want, got); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func TestHashname(t *testing.T) {
	tests := []struct {
		name string
		l    int
		s    []string
		want string
	}{
		{name: "empty s", l: 99, s: nil, want: ""},
		{name: "single element", l: 99, s: []string{"alpha"}, want: "alpha"},
		{name: "long single element, hashed", l: 12, s: []string{"gammagammagamma"}, want: "0d350ea5c204"},
		{name: "single element, truncated", l: 4, s: []string{"alpha"}, want: "8ed3"},
		{name: "two elements, truncated", l: 19, s: []string{"gammagamma", "betabeta"}, want: "ga-edf159/betabeta"},
		{name: "three elements", l: 99, s: []string{"alpha", "beta", "gamma"}, want: "alpha/beta/gamma"},
		{name: "issue/25", l: 60, s: []string{"default", "my-service-name", "my-very-very-long-service-host-name.my.domainname"}, want: "default/my-service-name/my-very-very--c4d2d4"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := hashname(tc.l, append([]string{}, tc.s...)...)
			if got != tc.want {
				t.Fatalf("hashname(%d, %q): got %q, want %q", tc.l, tc.s, got, tc.want)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name   string
		l      int
		s      string
		suffix string
		want   string
	}{
		{name: "no truncate", l: 10, s: "quijibo", suffix: "a8c5e6", want: "quijibo"},
		{name: "limit", l: len("quijibo"), s: "quijibo", suffix: "a8c5e6", want: "quijibo"},
		{name: "truncate some", l: 6, s: "quijibo", suffix: "a8c5", want: "q-a8c5"},
		{name: "truncate suffix", l: 4, s: "quijibo", suffix: "a8c5", want: "a8c5"},
		{name: "truncate more", l: 3, s: "quijibo", suffix: "a8c5", want: "a8c"},
		{name: "long single element, truncated", l: 9, s: "gammagamma", suffix: "0d350e", want: "ga-0d350e"},
		{name: "long single element, truncated", l: 12, s: "gammagammagamma", suffix: "0d350e", want: "gamma-0d350e"},
		{name: "issue/25", l: 60 / 3, s: "my-very-very-long-service-host-name.my.domainname", suffix: "a8c5e6", want: "my-very-very--a8c5e6"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := truncate(tc.l, tc.s, tc.suffix)
			if got != tc.want {
				t.Fatalf("hashname(%d, %q, %q): got %q, want %q", tc.l, tc.s, tc.suffix, got, tc.want)
			}
		})
	}
}

func TestAnyPositive(t *testing.T) {
	assert := func(want, got bool) {
		t.Helper()
		if want != got {
			t.Fatal("expected", want, "got", got)
		}
	}

	assert(false, anyPositive(0))
	assert(true, anyPositive(1))
	assert(false, anyPositive(-1))
	assert(false, anyPositive(0, 0))
	assert(true, anyPositive(1, 0))
	assert(true, anyPositive(0, 1))
	assert(true, anyPositive(-1, 1))
	assert(true, anyPositive(1, -1))
}

func TestU32nil(t *testing.T) {
	assert := func(want, got *types.UInt32Value) {
		t.Helper()
		if diff := cmp.Diff(want, got); diff != "" {
			t.Fatal(diff)
		}
	}

	assert(nil, u32nil(0))
	assert(u32(1), u32nil(1))
}

func TestClusterCommonLBConfig(t *testing.T) {
	got := ClusterCommonLBConfig()
	want := &v2.Cluster_CommonLbConfig{
		HealthyPanicThreshold: &envoy_type.Percent{ // Disable HealthyPanicThreshold
			Value: 0,
		},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatal(diff)
	}
}

func service(s *v1.Service) dag.TCPService {
	return dag.TCPService{
		Name:        s.Name,
		Namespace:   s.Namespace,
		ServicePort: &s.Spec.Ports[0],
	}
}
func externalnameservice(s *v1.Service) *dag.TCPService {
	return &dag.TCPService{
		Name:         s.Name,
		Namespace:    s.Namespace,
		ServicePort:  &s.Spec.Ports[0],
		ExternalName: s.Spec.ExternalName,
	}
}

func tlsservice(s *v1.Service, cert, subjectaltname string) dag.TCPService {
	return dag.TCPService{
		Name:        s.Name,
		Namespace:   s.Namespace,
		ServicePort: &s.Spec.Ports[0],
	}
}
