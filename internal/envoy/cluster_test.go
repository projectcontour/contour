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
	envoy_cluster "github.com/envoyproxy/go-control-plane/envoy/api/v2/cluster"
	envoy_api_v2_core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoy_type "github.com/envoyproxy/go-control-plane/envoy/type"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/projectcontour/contour/internal/assert"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/protobuf"
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

	tests := map[string]struct {
		cluster *dag.Cluster
		want    *v2.Cluster
	}{
		"simple service": {
			cluster: &dag.Cluster{
				Upstream: service(s1),
			},
			want: &v2.Cluster{
				Name:                 "default/kuard/443/da39a3ee5e",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(v2.Cluster_EDS),
				EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
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
			want: &v2.Cluster{
				Name:                 "default/kuard/443/da39a3ee5e",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(v2.Cluster_EDS),
				EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
					EdsConfig:   ConfigSource("contour"),
					ServiceName: "default/kuard/http",
				},
				Http2ProtocolOptions: &envoy_api_v2_core.Http2ProtocolOptions{},
			},
		},
		"h2 upstream": {
			cluster: &dag.Cluster{
				Upstream: service(s1, "h2"),
				Protocol: "h2",
			},
			want: &v2.Cluster{
				Name:                 "default/kuard/443/da39a3ee5e",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(v2.Cluster_EDS),
				EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
					EdsConfig:   ConfigSource("contour"),
					ServiceName: "default/kuard/http",
				},
				TransportSocket: UpstreamTLSTransportSocket(
					UpstreamTLSContext(nil, "", "h2"),
				),
				Http2ProtocolOptions: &envoy_api_v2_core.Http2ProtocolOptions{},
			},
		},
		"externalName service": {
			cluster: &dag.Cluster{
				Upstream: service(s2),
			},
			want: &v2.Cluster{
				Name:                 "default/kuard/443/da39a3ee5e",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(v2.Cluster_STRICT_DNS),
				LoadAssignment:       StaticClusterLoadAssignment(service(s2)),
			},
		},
		"tls upstream": {
			cluster: &dag.Cluster{
				Upstream: service(s1, "tls"),
				Protocol: "tls",
			},
			want: &v2.Cluster{
				Name:                 "default/kuard/443/da39a3ee5e",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(v2.Cluster_EDS),
				EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
					EdsConfig:   ConfigSource("contour"),
					ServiceName: "default/kuard/http",
				},
				TransportSocket: UpstreamTLSTransportSocket(
					UpstreamTLSContext(nil, ""),
				),
			},
		},
		"tls upstream - external name": {
			cluster: &dag.Cluster{
				Upstream: service(svcExternal, "tls"),
				Protocol: "tls",
				SNI:      "projectcontour.local",
			},
			want: &v2.Cluster{
				Name:                 "default/kuard/443/da39a3ee5e",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(v2.Cluster_STRICT_DNS),
				LoadAssignment:       StaticClusterLoadAssignment(service(svcExternal, "tls")),
				TransportSocket: UpstreamTLSTransportSocket(
					UpstreamTLSContext(nil, "projectcontour.local"),
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
			want: &v2.Cluster{
				Name:                 "default/kuard/443/3ac4e90987",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(v2.Cluster_EDS),
				EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
					EdsConfig:   ConfigSource("contour"),
					ServiceName: "default/kuard/http",
				},
				TransportSocket: UpstreamTLSTransportSocket(
					UpstreamTLSContext(
						&dag.PeerValidationContext{
							CACertificate: secret,
							SubjectName:   "foo.bar.io",
						},
						""),
				),
			},
		},
		"projectcontour.io/max-connections": {
			cluster: &dag.Cluster{
				Upstream: &dag.Service{
					Name: s1.Name, Namespace: s1.Namespace,
					ServicePort:    &s1.Spec.Ports[0],
					MaxConnections: 9000,
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
				CircuitBreakers: &envoy_cluster.CircuitBreakers{
					Thresholds: []*envoy_cluster.CircuitBreakers_Thresholds{{
						MaxConnections: protobuf.UInt32(9000),
					}},
				},
			},
		},
		"projectcontour.io/max-pending-requests": {
			cluster: &dag.Cluster{
				Upstream: &dag.Service{
					Name: s1.Name, Namespace: s1.Namespace,
					ServicePort:        &s1.Spec.Ports[0],
					MaxPendingRequests: 4096,
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
				CircuitBreakers: &envoy_cluster.CircuitBreakers{
					Thresholds: []*envoy_cluster.CircuitBreakers_Thresholds{{
						MaxPendingRequests: protobuf.UInt32(4096),
					}},
				},
			},
		},
		"projectcontour.io/max-requests": {
			cluster: &dag.Cluster{
				Upstream: &dag.Service{
					Name: s1.Name, Namespace: s1.Namespace,
					ServicePort: &s1.Spec.Ports[0],
					MaxRequests: 404,
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
				CircuitBreakers: &envoy_cluster.CircuitBreakers{
					Thresholds: []*envoy_cluster.CircuitBreakers_Thresholds{{
						MaxRequests: protobuf.UInt32(404),
					}},
				},
			},
		},
		"projectcontour.io/max-retries": {
			cluster: &dag.Cluster{
				Upstream: &dag.Service{
					Name: s1.Name, Namespace: s1.Namespace,
					ServicePort: &s1.Spec.Ports[0],
					MaxRetries:  7,
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
				CircuitBreakers: &envoy_cluster.CircuitBreakers{
					Thresholds: []*envoy_cluster.CircuitBreakers_Thresholds{{
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
			want: &v2.Cluster{
				Name:                 "default/kuard/443/58d888c08a",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(v2.Cluster_EDS),
				EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
					EdsConfig:   ConfigSource("contour"),
					ServiceName: "default/kuard/http",
				},
				LbPolicy: v2.Cluster_RANDOM,
			},
		},
		"cluster with cookie policy": {
			cluster: &dag.Cluster{
				Upstream:           service(s1),
				LoadBalancerPolicy: "Cookie",
			},
			want: &v2.Cluster{
				Name:                 "default/kuard/443/e4f81994fe",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(v2.Cluster_EDS),
				EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
					EdsConfig:   ConfigSource("contour"),
					ServiceName: "default/kuard/http",
				},
				LbPolicy: v2.Cluster_RING_HASH,
			},
		},

		"tcp service": {
			cluster: &dag.Cluster{
				Upstream: service(s1),
			},
			want: &v2.Cluster{
				Name:                 "default/kuard/443/da39a3ee5e",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(v2.Cluster_EDS),
				EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
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
			want: &v2.Cluster{
				Name:                 "default/kuard/443/da39a3ee5e",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(v2.Cluster_EDS),
				EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
					EdsConfig:   ConfigSource("contour"),
					ServiceName: "default/kuard/http",
				},
				DrainConnectionsOnHostRemoval: true,
				HealthChecks: []*envoy_api_v2_core.HealthCheck{{
					Timeout:            durationOrDefault(2, hcTimeout),
					Interval:           durationOrDefault(10, hcInterval),
					UnhealthyThreshold: countOrDefault(3, hcUnhealthyThreshold),
					HealthyThreshold:   countOrDefault(2, hcHealthyThreshold),
					HealthChecker: &envoy_api_v2_core.HealthCheck_TcpHealthCheck_{
						TcpHealthCheck: &envoy_api_v2_core.HealthCheck_TcpHealthCheck{},
					},
				}},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := Cluster(tc.cluster)
			want := clusterDefaults()

			proto.Merge(want, tc.want)

			assert.Equal(t, want, got)
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
				Upstream: &dag.Service{
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
				Upstream: &dag.Service{
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
				Upstream: &dag.Service{
					Name:      "backend",
					Namespace: "default",
					ServicePort: &v1.ServicePort{
						Name:       "http",
						Protocol:   "TCP",
						Port:       80,
						TargetPort: intstr.FromInt(6502),
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
		},
		"upstream tls validation with subject alt name": {
			cluster: &dag.Cluster{
				Upstream: &dag.Service{
					Name:      "backend",
					Namespace: "default",
					ServicePort: &v1.ServicePort{
						Name:       "http",
						Protocol:   "TCP",
						Port:       80,
						TargetPort: intstr.FromInt(6502),
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
			},
			want: "default/backend/80/6bf46b7b3a",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := Clustername(tc.cluster)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestLBPolicy(t *testing.T) {
	tests := map[string]v2.Cluster_LbPolicy{
		"WeightedLeastRequest": v2.Cluster_LEAST_REQUEST,
		"Random":               v2.Cluster_RANDOM,
		"RoundRobin":           v2.Cluster_ROUND_ROBIN,
		"":                     v2.Cluster_ROUND_ROBIN,
		"unknown":              v2.Cluster_ROUND_ROBIN,
		"Cookie":               v2.Cluster_RING_HASH,

		// RingHash and Maglev were removed as options in 0.13.
		// See #1150
		"RingHash": v2.Cluster_ROUND_ROBIN,
		"Maglev":   v2.Cluster_ROUND_ROBIN,
	}

	for policy, want := range tests {
		t.Run(policy, func(t *testing.T) {
			got := lbPolicy(policy)
			assert.Equal(t, want, got)
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
	assert.Equal(t, false, anyPositive(0))
	assert.Equal(t, true, anyPositive(1))
	assert.Equal(t, false, anyPositive(0, 0))
	assert.Equal(t, true, anyPositive(1, 0))
	assert.Equal(t, true, anyPositive(0, 1))
}

func TestU32nil(t *testing.T) {
	assert.Equal(t, (*wrappers.UInt32Value)(nil), u32nil(0))
	assert.Equal(t, protobuf.UInt32(1), u32nil(1))
}

func TestClusterCommonLBConfig(t *testing.T) {
	got := ClusterCommonLBConfig()
	want := &v2.Cluster_CommonLbConfig{
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
		Name:         s.Name,
		Namespace:    s.Namespace,
		ServicePort:  &s.Spec.Ports[0],
		ExternalName: s.Spec.ExternalName,
		Protocol:     protocol,
	}
}
