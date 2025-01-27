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
	envoy_config_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoy_upstream_http_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	envoy_type_v3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/envoy"
	"github.com/projectcontour/contour/internal/protobuf"
	"github.com/projectcontour/contour/internal/timeout"
	"github.com/projectcontour/contour/internal/xds"
)

func TestCluster(t *testing.T) {
	s1 := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: core_v1.ServiceSpec{
			Ports: []core_v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       443,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}

	s2 := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: core_v1.ServiceSpec{
			ExternalName: "foo.io",
			Ports: []core_v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       443,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}

	s3 := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: core_v1.ServiceSpec{
			ExternalName: "foo.io",
			Ports: []core_v1.ServicePort{
				{
					Name:       "http",
					Protocol:   "TCP",
					Port:       443,
					TargetPort: intstr.FromInt(8080),
				}, {
					Name:       "health-check",
					Protocol:   "TCP",
					Port:       8998,
					TargetPort: intstr.FromInt(8998),
				},
			},
		},
	}

	svcExternal := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: core_v1.ServiceSpec{
			ExternalName: "projectcontour.local",
			Ports: []core_v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       443,
				TargetPort: intstr.FromInt(8080),
			}},
			Type: core_v1.ServiceTypeExternalName,
		},
	}

	secret := &dag.Secret{
		Object: &core_v1.Secret{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      "secret",
				Namespace: "default",
			},
			Type: core_v1.SecretTypeTLS,
			Data: map[string][]byte{dag.CACertificateKey: []byte("cacert")},
		},
	}

	clientSecret := &dag.Secret{
		Object: &core_v1.Secret{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      "clientcertsecret",
				Namespace: "default",
			},
			Type: core_v1.SecretTypeTLS,
			Data: map[string][]byte{
				core_v1.TLSCertKey:       []byte("cert"),
				core_v1.TLSPrivateKeyKey: []byte("key"),
			},
		},
	}

	envoyGen := NewEnvoyGen(EnvoyGenOpt{
		XDSClusterName: DefaultXDSClusterName,
	})
	edsConfig := envoyGen.GetConfigSource()
	tests := map[string]struct {
		cluster *dag.Cluster
		want    *envoy_config_cluster_v3.Cluster
	}{
		"simple service": {
			cluster: &dag.Cluster{
				Upstream: service(s1),
			},
			want: &envoy_config_cluster_v3.Cluster{
				Name:                 "default/kuard/443/da39a3ee5e",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(envoy_config_cluster_v3.Cluster_EDS),
				EdsClusterConfig: &envoy_config_cluster_v3.Cluster_EdsClusterConfig{
					EdsConfig:   edsConfig,
					ServiceName: "default/kuard/http",
				},
			},
		},
		"h2c upstream": {
			cluster: &dag.Cluster{
				Upstream: service(s1, "h2c"),
				Protocol: "h2c",
			},
			want: &envoy_config_cluster_v3.Cluster{
				Name:                 "default/kuard/443/f4f94965ec",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(envoy_config_cluster_v3.Cluster_EDS),
				EdsClusterConfig: &envoy_config_cluster_v3.Cluster_EdsClusterConfig{
					EdsConfig:   edsConfig,
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
		},
		"h2 upstream": {
			cluster: &dag.Cluster{
				Upstream: service(s1, "h2"),
				Protocol: "h2",
			},
			want: &envoy_config_cluster_v3.Cluster{
				Name:                 "default/kuard/443/bf1c365741",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(envoy_config_cluster_v3.Cluster_EDS),
				EdsClusterConfig: &envoy_config_cluster_v3.Cluster_EdsClusterConfig{
					EdsConfig:   edsConfig,
					ServiceName: "default/kuard/http",
				},
				TransportSocket: UpstreamTLSTransportSocket(
					envoyGen.UpstreamTLSContext(nil, "", nil, nil, "h2"),
				),
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
		},
		"externalName service": {
			cluster: &dag.Cluster{
				Upstream: service(s2),
			},
			want: &envoy_config_cluster_v3.Cluster{
				Name:                 "default/kuard/443/da39a3ee5e",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(envoy_config_cluster_v3.Cluster_STRICT_DNS),
				LoadAssignment:       externalNameClusterLoadAssignment(service(s2)),
			},
		},
		"externalName service healthcheckport": {
			cluster: &dag.Cluster{
				Upstream: healthcheckService(s3),
			},
			want: &envoy_config_cluster_v3.Cluster{
				Name:                 "default/kuard/443/da39a3ee5e",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(envoy_config_cluster_v3.Cluster_STRICT_DNS),
				LoadAssignment:       externalNameClusterLoadAssignment(healthcheckService(s3)),
			},
		},
		"externalName service - dns-lookup-family v4": {
			cluster: &dag.Cluster{
				Upstream:        service(s2),
				DNSLookupFamily: "v4",
			},
			want: &envoy_config_cluster_v3.Cluster{
				Name:                 "default/kuard/443/da39a3ee5e",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(envoy_config_cluster_v3.Cluster_STRICT_DNS),
				LoadAssignment:       externalNameClusterLoadAssignment(service(s2)),
				DnsLookupFamily:      envoy_config_cluster_v3.Cluster_V4_ONLY,
			},
		},
		"externalName service - dns-lookup-family v6": {
			cluster: &dag.Cluster{
				Upstream:        service(s2),
				DNSLookupFamily: "v6",
			},
			want: &envoy_config_cluster_v3.Cluster{
				Name:                 "default/kuard/443/da39a3ee5e",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(envoy_config_cluster_v3.Cluster_STRICT_DNS),
				LoadAssignment:       externalNameClusterLoadAssignment(service(s2)),
				DnsLookupFamily:      envoy_config_cluster_v3.Cluster_V6_ONLY,
			},
		},
		"externalName service - dns-lookup-family auto": {
			cluster: &dag.Cluster{
				Upstream:        service(s2),
				DNSLookupFamily: "auto",
			},
			want: &envoy_config_cluster_v3.Cluster{
				Name:                 "default/kuard/443/da39a3ee5e",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(envoy_config_cluster_v3.Cluster_STRICT_DNS),
				LoadAssignment:       externalNameClusterLoadAssignment(service(s2)),
				DnsLookupFamily:      envoy_config_cluster_v3.Cluster_AUTO,
			},
		},
		"externalName service - dns-lookup-family all": {
			cluster: &dag.Cluster{
				Upstream:        service(s2),
				DNSLookupFamily: "all",
			},
			want: &envoy_config_cluster_v3.Cluster{
				Name:                 "default/kuard/443/da39a3ee5e",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(envoy_config_cluster_v3.Cluster_LOGICAL_DNS),
				LoadAssignment:       externalNameClusterLoadAssignment(service(s2)),
				DnsLookupFamily:      envoy_config_cluster_v3.Cluster_ALL,
			},
		},
		"externalName service - dns-lookup-family not defined": {
			cluster: &dag.Cluster{
				Upstream: service(s2),
				// DNSLookupFamily: "auto",
			},
			want: &envoy_config_cluster_v3.Cluster{
				Name:                 "default/kuard/443/da39a3ee5e",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(envoy_config_cluster_v3.Cluster_STRICT_DNS),
				LoadAssignment:       externalNameClusterLoadAssignment(service(s2)),
				DnsLookupFamily:      envoy_config_cluster_v3.Cluster_AUTO,
			},
		},
		"tls upstream": {
			cluster: &dag.Cluster{
				Upstream: service(s1, "tls"),
				Protocol: "tls",
			},
			want: &envoy_config_cluster_v3.Cluster{
				Name:                 "default/kuard/443/4929fca9d4",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(envoy_config_cluster_v3.Cluster_EDS),
				EdsClusterConfig: &envoy_config_cluster_v3.Cluster_EdsClusterConfig{
					EdsConfig:   edsConfig,
					ServiceName: "default/kuard/http",
				},
				TransportSocket: UpstreamTLSTransportSocket(
					envoyGen.UpstreamTLSContext(nil, "", nil, nil),
				),
			},
		},
		"tls upstream - external name": {
			cluster: &dag.Cluster{
				Upstream: service(svcExternal, "tls"),
				Protocol: "tls",
				SNI:      "projectcontour.local",
			},
			want: &envoy_config_cluster_v3.Cluster{
				Name:                 "default/kuard/443/a996a742af",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(envoy_config_cluster_v3.Cluster_STRICT_DNS),
				LoadAssignment:       externalNameClusterLoadAssignment(service(svcExternal, "tls")),
				TransportSocket: UpstreamTLSTransportSocket(
					envoyGen.UpstreamTLSContext(nil, "projectcontour.local", nil, nil),
				),
			},
		},
		"verify tls upstream with san": {
			cluster: &dag.Cluster{
				Upstream: service(s1, "tls"),
				Protocol: "tls",
				UpstreamValidation: &dag.PeerValidationContext{
					CACertificates: []*dag.Secret{
						secret,
					},
					SubjectNames: []string{"foo.bar.io"},
				},
			},
			want: &envoy_config_cluster_v3.Cluster{
				Name:                 "default/kuard/443/62d1f9ad02",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(envoy_config_cluster_v3.Cluster_EDS),
				EdsClusterConfig: &envoy_config_cluster_v3.Cluster_EdsClusterConfig{
					EdsConfig:   edsConfig,
					ServiceName: "default/kuard/http",
				},
				TransportSocket: UpstreamTLSTransportSocket(
					envoyGen.UpstreamTLSContext(
						&dag.PeerValidationContext{
							CACertificates: []*dag.Secret{
								secret,
							},
							SubjectNames: []string{"foo.bar.io"},
						},
						"",
						nil,
						nil),
				),
			},
		},
		"UpstreamTLS protocol version set": {
			cluster: &dag.Cluster{
				Upstream: service(s1, "tls"),
				Protocol: "tls",
				UpstreamValidation: &dag.PeerValidationContext{
					CACertificates: []*dag.Secret{
						secret,
					},
					SubjectNames: []string{"foo.bar.io"},
				},
				UpstreamTLS: &dag.UpstreamTLS{
					MinimumProtocolVersion: "1.3",
					MaximumProtocolVersion: "1.3",
				},
			},
			want: &envoy_config_cluster_v3.Cluster{
				Name:                 "default/kuard/443/62d1f9ad02",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(envoy_config_cluster_v3.Cluster_EDS),
				EdsClusterConfig: &envoy_config_cluster_v3.Cluster_EdsClusterConfig{
					EdsConfig:   edsConfig,
					ServiceName: "default/kuard/http",
				},
				TransportSocket: UpstreamTLSTransportSocket(
					envoyGen.UpstreamTLSContext(
						&dag.PeerValidationContext{
							CACertificates: []*dag.Secret{
								secret,
							},
							SubjectNames: []string{"foo.bar.io"},
						},
						"",
						nil,
						&dag.UpstreamTLS{
							MinimumProtocolVersion: "1.3",
							MaximumProtocolVersion: "1.3",
						},
					),
				),
			},
		},
		"projectcontour.io/max-connections": {
			cluster: &dag.Cluster{
				Upstream: &dag.Service{
					CircuitBreakers: dag.CircuitBreakers{
						MaxConnections: 9000,
					},
					Weighted: dag.WeightedService{
						Weight:           1,
						ServiceName:      s1.Name,
						ServiceNamespace: s1.Namespace,
						ServicePort:      s1.Spec.Ports[0],
						HealthPort:       s1.Spec.Ports[0],
					},
				},
			},
			want: &envoy_config_cluster_v3.Cluster{
				Name:                 "default/kuard/443/da39a3ee5e",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(envoy_config_cluster_v3.Cluster_EDS),
				EdsClusterConfig: &envoy_config_cluster_v3.Cluster_EdsClusterConfig{
					EdsConfig:   edsConfig,
					ServiceName: "default/kuard/http",
				},
				CircuitBreakers: &envoy_config_cluster_v3.CircuitBreakers{
					Thresholds: []*envoy_config_cluster_v3.CircuitBreakers_Thresholds{{
						MaxConnections: wrapperspb.UInt32(9000),
						TrackRemaining: true,
					}},
					PerHostThresholds: []*envoy_config_cluster_v3.CircuitBreakers_Thresholds{{
						TrackRemaining: true,
					}},
				},
			},
		},
		"projectcontour.io/max-pending-requests": {
			cluster: &dag.Cluster{
				Upstream: &dag.Service{
					CircuitBreakers: dag.CircuitBreakers{
						MaxPendingRequests: 4096,
					},
					Weighted: dag.WeightedService{
						Weight:           1,
						ServiceName:      s1.Name,
						ServiceNamespace: s1.Namespace,
						ServicePort:      s1.Spec.Ports[0],
						HealthPort:       s1.Spec.Ports[0],
					},
				},
			},
			want: &envoy_config_cluster_v3.Cluster{
				Name:                 "default/kuard/443/da39a3ee5e",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(envoy_config_cluster_v3.Cluster_EDS),
				EdsClusterConfig: &envoy_config_cluster_v3.Cluster_EdsClusterConfig{
					EdsConfig:   edsConfig,
					ServiceName: "default/kuard/http",
				},
				CircuitBreakers: &envoy_config_cluster_v3.CircuitBreakers{
					Thresholds: []*envoy_config_cluster_v3.CircuitBreakers_Thresholds{{
						MaxPendingRequests: wrapperspb.UInt32(4096),
						TrackRemaining:     true,
					}},
					PerHostThresholds: []*envoy_config_cluster_v3.CircuitBreakers_Thresholds{{
						TrackRemaining: true,
					}},
				},
			},
		},
		"projectcontour.io/max-requests": {
			cluster: &dag.Cluster{
				Upstream: &dag.Service{
					CircuitBreakers: dag.CircuitBreakers{
						MaxRequests: 404,
					},
					Weighted: dag.WeightedService{
						Weight:           1,
						ServiceName:      s1.Name,
						ServiceNamespace: s1.Namespace,
						ServicePort:      s1.Spec.Ports[0],
						HealthPort:       s1.Spec.Ports[0],
					},
				},
			},
			want: &envoy_config_cluster_v3.Cluster{
				Name:                 "default/kuard/443/da39a3ee5e",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(envoy_config_cluster_v3.Cluster_EDS),
				EdsClusterConfig: &envoy_config_cluster_v3.Cluster_EdsClusterConfig{
					EdsConfig:   edsConfig,
					ServiceName: "default/kuard/http",
				},
				CircuitBreakers: &envoy_config_cluster_v3.CircuitBreakers{
					Thresholds: []*envoy_config_cluster_v3.CircuitBreakers_Thresholds{{
						MaxRequests:    wrapperspb.UInt32(404),
						TrackRemaining: true,
					}},
					PerHostThresholds: []*envoy_config_cluster_v3.CircuitBreakers_Thresholds{{
						TrackRemaining: true,
					}},
				},
			},
		},
		"projectcontour.io/max-retries": {
			cluster: &dag.Cluster{
				Upstream: &dag.Service{
					CircuitBreakers: dag.CircuitBreakers{
						MaxRetries: 7,
					},
					Weighted: dag.WeightedService{
						Weight:           1,
						ServiceName:      s1.Name,
						ServiceNamespace: s1.Namespace,
						ServicePort:      s1.Spec.Ports[0],
						HealthPort:       s1.Spec.Ports[0],
					},
				},
			},
			want: &envoy_config_cluster_v3.Cluster{
				Name:                 "default/kuard/443/da39a3ee5e",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(envoy_config_cluster_v3.Cluster_EDS),
				EdsClusterConfig: &envoy_config_cluster_v3.Cluster_EdsClusterConfig{
					EdsConfig:   edsConfig,
					ServiceName: "default/kuard/http",
				},
				CircuitBreakers: &envoy_config_cluster_v3.CircuitBreakers{
					Thresholds: []*envoy_config_cluster_v3.CircuitBreakers_Thresholds{{
						MaxRetries:     wrapperspb.UInt32(7),
						TrackRemaining: true,
					}},
					PerHostThresholds: []*envoy_config_cluster_v3.CircuitBreakers_Thresholds{{
						TrackRemaining: true,
					}},
				},
			},
		},
		"projectcontour.io/per-host-max-connections": {
			cluster: &dag.Cluster{
				Upstream: &dag.Service{
					CircuitBreakers: dag.CircuitBreakers{
						PerHostMaxConnections: 45,
					},
					Weighted: dag.WeightedService{
						Weight:           1,
						ServiceName:      s1.Name,
						ServiceNamespace: s1.Namespace,
						ServicePort:      s1.Spec.Ports[0],
						HealthPort:       s1.Spec.Ports[0],
					},
				},
			},
			want: &envoy_config_cluster_v3.Cluster{
				Name:                 "default/kuard/443/da39a3ee5e",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(envoy_config_cluster_v3.Cluster_EDS),
				EdsClusterConfig: &envoy_config_cluster_v3.Cluster_EdsClusterConfig{
					EdsConfig:   edsConfig,
					ServiceName: "default/kuard/http",
				},
				CircuitBreakers: &envoy_config_cluster_v3.CircuitBreakers{
					Thresholds: []*envoy_config_cluster_v3.CircuitBreakers_Thresholds{{
						TrackRemaining: true,
					}},
					PerHostThresholds: []*envoy_config_cluster_v3.CircuitBreakers_Thresholds{{
						MaxConnections: wrapperspb.UInt32(45),
						TrackRemaining: true,
					}},
				},
			},
		},
		"cluster with random load balancer policy": {
			cluster: &dag.Cluster{
				Upstream:           service(s1),
				LoadBalancerPolicy: "Random",
			},
			want: &envoy_config_cluster_v3.Cluster{
				Name:                 "default/kuard/443/58d888c08a",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(envoy_config_cluster_v3.Cluster_EDS),
				EdsClusterConfig: &envoy_config_cluster_v3.Cluster_EdsClusterConfig{
					EdsConfig:   edsConfig,
					ServiceName: "default/kuard/http",
				},
				LbPolicy: envoy_config_cluster_v3.Cluster_RANDOM,
			},
		},
		"cluster with cookie policy": {
			cluster: &dag.Cluster{
				Upstream:           service(s1),
				LoadBalancerPolicy: "Cookie",
			},
			want: &envoy_config_cluster_v3.Cluster{
				Name:                 "default/kuard/443/e4f81994fe",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(envoy_config_cluster_v3.Cluster_EDS),
				EdsClusterConfig: &envoy_config_cluster_v3.Cluster_EdsClusterConfig{
					EdsConfig:   edsConfig,
					ServiceName: "default/kuard/http",
				},
				LbPolicy: envoy_config_cluster_v3.Cluster_RING_HASH,
			},
		},

		"tcp service": {
			cluster: &dag.Cluster{
				Upstream: service(s1),
			},
			want: &envoy_config_cluster_v3.Cluster{
				Name:                 "default/kuard/443/da39a3ee5e",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(envoy_config_cluster_v3.Cluster_EDS),
				EdsClusterConfig: &envoy_config_cluster_v3.Cluster_EdsClusterConfig{
					EdsConfig:   edsConfig,
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
			want: &envoy_config_cluster_v3.Cluster{
				Name:                 "default/kuard/443/da39a3ee5e",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(envoy_config_cluster_v3.Cluster_EDS),
				EdsClusterConfig: &envoy_config_cluster_v3.Cluster_EdsClusterConfig{
					EdsConfig:   edsConfig,
					ServiceName: "default/kuard/http",
				},
				IgnoreHealthOnHostRemoval: true,
				HealthChecks: []*envoy_config_core_v3.HealthCheck{{
					Timeout:            durationOrDefault(2, envoy.HCTimeout),
					Interval:           durationOrDefault(10, envoy.HCInterval),
					UnhealthyThreshold: protobuf.UInt32OrDefault(3, envoy.HCUnhealthyThreshold),
					HealthyThreshold:   protobuf.UInt32OrDefault(2, envoy.HCHealthyThreshold),
					HealthChecker: &envoy_config_core_v3.HealthCheck_TcpHealthCheck_{
						TcpHealthCheck: &envoy_config_core_v3.HealthCheck_TcpHealthCheck{},
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
			want: &envoy_config_cluster_v3.Cluster{
				Name:                 "default/kuard/443/4929fca9d4",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(envoy_config_cluster_v3.Cluster_EDS),
				EdsClusterConfig: &envoy_config_cluster_v3.Cluster_EdsClusterConfig{
					EdsConfig:   edsConfig,
					ServiceName: "default/kuard/http",
				},
				TransportSocket: UpstreamTLSTransportSocket(
					envoyGen.UpstreamTLSContext(nil, "", clientSecret, nil),
				),
			},
		},
		"cluster with connect timeout set": {
			cluster: &dag.Cluster{
				Upstream:      service(s1),
				TimeoutPolicy: dag.ClusterTimeoutPolicy{ConnectTimeout: 10 * time.Second},
			},
			want: &envoy_config_cluster_v3.Cluster{
				Name:                 "default/kuard/443/da39a3ee5e",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(envoy_config_cluster_v3.Cluster_EDS),
				EdsClusterConfig: &envoy_config_cluster_v3.Cluster_EdsClusterConfig{
					EdsConfig:   edsConfig,
					ServiceName: "default/kuard/http",
				},
				ConnectTimeout: durationpb.New(10 * time.Second),
			},
		},
		"cluster with idle connection timeout set": {
			cluster: &dag.Cluster{
				Upstream:      service(s1),
				TimeoutPolicy: dag.ClusterTimeoutPolicy{IdleConnectionTimeout: timeout.DurationSetting(10 * time.Second)},
			},
			want: &envoy_config_cluster_v3.Cluster{
				Name:                 "default/kuard/443/357c84df09",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(envoy_config_cluster_v3.Cluster_EDS),
				EdsClusterConfig: &envoy_config_cluster_v3.Cluster_EdsClusterConfig{
					EdsConfig:   edsConfig,
					ServiceName: "default/kuard/http",
				},
				TypedExtensionProtocolOptions: map[string]*anypb.Any{
					"envoy.extensions.upstreams.http.v3.HttpProtocolOptions": protobuf.MustMarshalAny(
						&envoy_upstream_http_v3.HttpProtocolOptions{
							CommonHttpProtocolOptions: &envoy_config_core_v3.HttpProtocolOptions{
								IdleTimeout: durationpb.New(10 * time.Second),
							},
							UpstreamProtocolOptions: &envoy_upstream_http_v3.HttpProtocolOptions_ExplicitHttpConfig_{
								ExplicitHttpConfig: &envoy_upstream_http_v3.HttpProtocolOptions_ExplicitHttpConfig{
									ProtocolConfig: &envoy_upstream_http_v3.HttpProtocolOptions_ExplicitHttpConfig_HttpProtocolOptions{},
								},
							},
						},
					),
				},
			},
		},
		"slow start mode": {
			cluster: &dag.Cluster{
				Upstream: service(s1),
				SlowStartConfig: &dag.SlowStartConfig{
					Window:           10 * time.Second,
					Aggression:       1.0,
					MinWeightPercent: 10,
				},
			},
			want: &envoy_config_cluster_v3.Cluster{
				Name:                 "default/kuard/443/2c8f64025b",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(envoy_config_cluster_v3.Cluster_EDS),
				EdsClusterConfig: &envoy_config_cluster_v3.Cluster_EdsClusterConfig{
					EdsConfig:   edsConfig,
					ServiceName: "default/kuard/http",
				},
				LbConfig: &envoy_config_cluster_v3.Cluster_RoundRobinLbConfig_{
					RoundRobinLbConfig: &envoy_config_cluster_v3.Cluster_RoundRobinLbConfig{
						SlowStartConfig: &envoy_config_cluster_v3.Cluster_SlowStartConfig{
							SlowStartWindow: durationpb.New(10 * time.Second),
							Aggression: &envoy_config_core_v3.RuntimeDouble{
								DefaultValue: 1.0,
								RuntimeKey:   "contour.slowstart.aggression",
							},
							MinWeightPercent: &envoy_type_v3.Percent{
								Value: 10.0,
							},
						},
					},
				},
			},
		},
		"slow start mode: LB policy LEAST_REQUEST": {
			cluster: &dag.Cluster{
				Upstream: service(s1),
				SlowStartConfig: &dag.SlowStartConfig{
					Window:           10 * time.Second,
					Aggression:       1.0,
					MinWeightPercent: 10,
				},
				LoadBalancerPolicy: dag.LoadBalancerPolicyWeightedLeastRequest,
			},
			want: &envoy_config_cluster_v3.Cluster{
				Name:                 "default/kuard/443/0b01a6912a",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(envoy_config_cluster_v3.Cluster_EDS),
				EdsClusterConfig: &envoy_config_cluster_v3.Cluster_EdsClusterConfig{
					EdsConfig:   edsConfig,
					ServiceName: "default/kuard/http",
				},
				LbPolicy: envoy_config_cluster_v3.Cluster_LEAST_REQUEST,
				LbConfig: &envoy_config_cluster_v3.Cluster_LeastRequestLbConfig_{
					LeastRequestLbConfig: &envoy_config_cluster_v3.Cluster_LeastRequestLbConfig{
						SlowStartConfig: &envoy_config_cluster_v3.Cluster_SlowStartConfig{
							SlowStartWindow: durationpb.New(10 * time.Second),
							Aggression: &envoy_config_core_v3.RuntimeDouble{
								DefaultValue: 1.0,
								RuntimeKey:   "contour.slowstart.aggression",
							},
							MinWeightPercent: &envoy_type_v3.Percent{
								Value: 10.0,
							},
						},
					},
				},
			},
		},
		"cluster with per connection buffer limit bytes set": {
			cluster: &dag.Cluster{
				Upstream:                      service(s1),
				PerConnectionBufferLimitBytes: ptr.To(uint32(32768)),
			},
			want: &envoy_config_cluster_v3.Cluster{
				Name:                 "default/kuard/443/da39a3ee5e",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(envoy_config_cluster_v3.Cluster_EDS),
				EdsClusterConfig: &envoy_config_cluster_v3.Cluster_EdsClusterConfig{
					EdsConfig:   edsConfig,
					ServiceName: "default/kuard/http",
				},
				PerConnectionBufferLimitBytes: wrapperspb.UInt32(32768),
			},
		},
		"cluster with max requests per connection set": {
			cluster: &dag.Cluster{
				Upstream:                 service(s1),
				MaxRequestsPerConnection: ptr.To(uint32(1)),
			},
			want: &envoy_config_cluster_v3.Cluster{
				Name:                 "default/kuard/443/da39a3ee5e",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(envoy_config_cluster_v3.Cluster_EDS),
				EdsClusterConfig: &envoy_config_cluster_v3.Cluster_EdsClusterConfig{
					EdsConfig:   edsConfig,
					ServiceName: "default/kuard/http",
				},
				TypedExtensionProtocolOptions: map[string]*anypb.Any{
					"envoy.extensions.upstreams.http.v3.HttpProtocolOptions": protobuf.MustMarshalAny(
						&envoy_upstream_http_v3.HttpProtocolOptions{
							CommonHttpProtocolOptions: &envoy_config_core_v3.HttpProtocolOptions{
								MaxRequestsPerConnection: wrapperspb.UInt32(1),
							},
							UpstreamProtocolOptions: &envoy_upstream_http_v3.HttpProtocolOptions_ExplicitHttpConfig_{
								ExplicitHttpConfig: &envoy_upstream_http_v3.HttpProtocolOptions_ExplicitHttpConfig{
									ProtocolConfig: &envoy_upstream_http_v3.HttpProtocolOptions_ExplicitHttpConfig_HttpProtocolOptions{},
								},
							},
						},
					),
				},
			},
		},
		"cluster with max requests per connection and idle timeout set": {
			cluster: &dag.Cluster{
				Upstream:                 service(s1),
				MaxRequestsPerConnection: ptr.To(uint32(1)),
				TimeoutPolicy: dag.ClusterTimeoutPolicy{
					IdleConnectionTimeout: timeout.DurationSetting(time.Second * 60),
				},
			},
			want: &envoy_config_cluster_v3.Cluster{
				Name:                 "default/kuard/443/47b66db27a",
				AltStatName:          "default_kuard_443",
				ClusterDiscoveryType: ClusterDiscoveryType(envoy_config_cluster_v3.Cluster_EDS),
				EdsClusterConfig: &envoy_config_cluster_v3.Cluster_EdsClusterConfig{
					EdsConfig:   edsConfig,
					ServiceName: "default/kuard/http",
				},
				TypedExtensionProtocolOptions: map[string]*anypb.Any{
					"envoy.extensions.upstreams.http.v3.HttpProtocolOptions": protobuf.MustMarshalAny(
						&envoy_upstream_http_v3.HttpProtocolOptions{
							CommonHttpProtocolOptions: &envoy_config_core_v3.HttpProtocolOptions{
								MaxRequestsPerConnection: wrapperspb.UInt32(1),
								IdleTimeout:              durationpb.New(60 * time.Second),
							},
							UpstreamProtocolOptions: &envoy_upstream_http_v3.HttpProtocolOptions_ExplicitHttpConfig_{
								ExplicitHttpConfig: &envoy_upstream_http_v3.HttpProtocolOptions_ExplicitHttpConfig{
									ProtocolConfig: &envoy_upstream_http_v3.HttpProtocolOptions_ExplicitHttpConfig_HttpProtocolOptions{},
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
			got := envoyGen.Cluster(tc.cluster)
			want := clusterDefaults()

			proto.Merge(want, tc.want)

			protobuf.ExpectEqual(t, want, got)
		})
	}
}

func TestDNSNameCluster(t *testing.T) {
	envoyGen := NewEnvoyGen(EnvoyGenOpt{
		XDSClusterName: "notcontour",
	})
	tests := map[string]struct {
		cluster *dag.DNSNameCluster
		want    *envoy_config_cluster_v3.Cluster
	}{
		"plain HTTP cluster": {
			cluster: &dag.DNSNameCluster{
				Address:         "foo.projectcontour.io",
				Scheme:          "http",
				Port:            80,
				DNSLookupFamily: "auto",
			},
			want: &envoy_config_cluster_v3.Cluster{
				Name:                 "dnsname/http/foo.projectcontour.io",
				DnsLookupFamily:      envoy_config_cluster_v3.Cluster_AUTO,
				ClusterDiscoveryType: ClusterDiscoveryType(envoy_config_cluster_v3.Cluster_STRICT_DNS),
				LoadAssignment: &envoy_config_endpoint_v3.ClusterLoadAssignment{
					ClusterName: "dnsname/http/foo.projectcontour.io",
					Endpoints: []*envoy_config_endpoint_v3.LocalityLbEndpoints{
						{
							LbEndpoints: []*envoy_config_endpoint_v3.LbEndpoint{
								{
									HostIdentifier: &envoy_config_endpoint_v3.LbEndpoint_Endpoint{
										Endpoint: &envoy_config_endpoint_v3.Endpoint{
											Address: SocketAddress("foo.projectcontour.io", 80),
										},
									},
								},
							},
						},
					},
				},
			},
		},
		"plain HTTP cluster with DNS lookup family of v4": {
			cluster: &dag.DNSNameCluster{
				Address:         "foo.projectcontour.io",
				Scheme:          "http",
				Port:            80,
				DNSLookupFamily: "v4",
			},
			want: &envoy_config_cluster_v3.Cluster{
				Name:                 "dnsname/http/foo.projectcontour.io",
				DnsLookupFamily:      envoy_config_cluster_v3.Cluster_V4_ONLY,
				ClusterDiscoveryType: ClusterDiscoveryType(envoy_config_cluster_v3.Cluster_STRICT_DNS),
				LoadAssignment: &envoy_config_endpoint_v3.ClusterLoadAssignment{
					ClusterName: "dnsname/http/foo.projectcontour.io",
					Endpoints: []*envoy_config_endpoint_v3.LocalityLbEndpoints{
						{
							LbEndpoints: []*envoy_config_endpoint_v3.LbEndpoint{
								{
									HostIdentifier: &envoy_config_endpoint_v3.LbEndpoint_Endpoint{
										Endpoint: &envoy_config_endpoint_v3.Endpoint{
											Address: SocketAddress("foo.projectcontour.io", 80),
										},
									},
								},
							},
						},
					},
				},
			},
		},
		"plain HTTP cluster with DNS lookup family of all": {
			cluster: &dag.DNSNameCluster{
				Address:         "foo.projectcontour.io",
				Scheme:          "http",
				Port:            80,
				DNSLookupFamily: "all",
			},
			want: &envoy_config_cluster_v3.Cluster{
				Name:                 "dnsname/http/foo.projectcontour.io",
				DnsLookupFamily:      envoy_config_cluster_v3.Cluster_ALL,
				ClusterDiscoveryType: ClusterDiscoveryType(envoy_config_cluster_v3.Cluster_LOGICAL_DNS),
				LoadAssignment: &envoy_config_endpoint_v3.ClusterLoadAssignment{
					ClusterName: "dnsname/http/foo.projectcontour.io",
					Endpoints: []*envoy_config_endpoint_v3.LocalityLbEndpoints{
						{
							LbEndpoints: []*envoy_config_endpoint_v3.LbEndpoint{
								{
									HostIdentifier: &envoy_config_endpoint_v3.LbEndpoint_Endpoint{
										Endpoint: &envoy_config_endpoint_v3.Endpoint{
											Address: SocketAddress("foo.projectcontour.io", 80),
										},
									},
								},
							},
						},
					},
				},
			},
		},
		"HTTPS cluster": {
			cluster: &dag.DNSNameCluster{
				Address:         "foo.projectcontour.io",
				Scheme:          "https",
				Port:            443,
				DNSLookupFamily: "auto",
			},
			want: &envoy_config_cluster_v3.Cluster{
				Name:                 "dnsname/https/foo.projectcontour.io",
				DnsLookupFamily:      envoy_config_cluster_v3.Cluster_AUTO,
				ClusterDiscoveryType: ClusterDiscoveryType(envoy_config_cluster_v3.Cluster_STRICT_DNS),
				LoadAssignment: &envoy_config_endpoint_v3.ClusterLoadAssignment{
					ClusterName: "dnsname/https/foo.projectcontour.io",
					Endpoints: []*envoy_config_endpoint_v3.LocalityLbEndpoints{
						{
							LbEndpoints: []*envoy_config_endpoint_v3.LbEndpoint{
								{
									HostIdentifier: &envoy_config_endpoint_v3.LbEndpoint_Endpoint{
										Endpoint: &envoy_config_endpoint_v3.Endpoint{
											Address: SocketAddress("foo.projectcontour.io", 443),
										},
									},
								},
							},
						},
					},
				},
				TransportSocket: UpstreamTLSTransportSocket(envoyGen.UpstreamTLSContext(nil, "foo.projectcontour.io", nil, nil)),
			},
		},
		"HTTPS cluster with upstream validation": {
			cluster: &dag.DNSNameCluster{
				Address:         "foo.projectcontour.io",
				Scheme:          "https",
				Port:            443,
				DNSLookupFamily: "auto",
				UpstreamValidation: &dag.PeerValidationContext{
					CACertificates: []*dag.Secret{
						{
							Object: &core_v1.Secret{
								Data: map[string][]byte{
									"ca.crt": []byte("ca-cert"),
								},
							},
						},
					},
					SubjectNames: []string{"foo.projectcontour.io"},
				},
			},
			want: &envoy_config_cluster_v3.Cluster{
				Name:                 "dnsname/https/foo.projectcontour.io",
				DnsLookupFamily:      envoy_config_cluster_v3.Cluster_AUTO,
				ClusterDiscoveryType: ClusterDiscoveryType(envoy_config_cluster_v3.Cluster_STRICT_DNS),
				LoadAssignment: &envoy_config_endpoint_v3.ClusterLoadAssignment{
					ClusterName: "dnsname/https/foo.projectcontour.io",
					Endpoints: []*envoy_config_endpoint_v3.LocalityLbEndpoints{
						{
							LbEndpoints: []*envoy_config_endpoint_v3.LbEndpoint{
								{
									HostIdentifier: &envoy_config_endpoint_v3.LbEndpoint_Endpoint{
										Endpoint: &envoy_config_endpoint_v3.Endpoint{
											Address: SocketAddress("foo.projectcontour.io", 443),
										},
									},
								},
							},
						},
					},
				},
				TransportSocket: UpstreamTLSTransportSocket(envoyGen.UpstreamTLSContext(&dag.PeerValidationContext{
					CACertificates: []*dag.Secret{
						{
							Object: &core_v1.Secret{
								Data: map[string][]byte{
									"ca.crt": []byte("ca-cert"),
								},
							},
						},
					},
					SubjectNames: []string{"foo.projectcontour.io"},
				}, "foo.projectcontour.io", nil, nil)),
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := envoyGen.DNSNameCluster(tc.cluster)
			want := clusterDefaults()

			proto.Merge(want, tc.want)

			protobuf.ExpectEqual(t, want, got)
		})
	}
}

func TestClusterLoadAssignmentName(t *testing.T) {
	assert.Equal(t, "ns/svc/port", xds.ClusterLoadAssignmentName(types.NamespacedName{Namespace: "ns", Name: "svc"}, "port"))
	assert.Equal(t, "ns/svc", xds.ClusterLoadAssignmentName(types.NamespacedName{Namespace: "ns", Name: "svc"}, ""))
	assert.Equal(t, "/", xds.ClusterLoadAssignmentName(types.NamespacedName{}, ""))
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
					ServicePort: core_v1.ServicePort{
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
					ServicePort: core_v1.ServicePort{
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
					ServicePort: core_v1.ServicePort{
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
				ServicePort: core_v1.ServicePort{
					Name:       "http",
					Protocol:   "TCP",
					Port:       80,
					TargetPort: intstr.FromInt(6502),
				},
			},
		},
		LoadBalancerPolicy: "Random",
		UpstreamValidation: &dag.PeerValidationContext{
			CACertificates: []*dag.Secret{
				{
					Object: &core_v1.Secret{
						ObjectMeta: meta_v1.ObjectMeta{
							Name:      "secret",
							Namespace: "default",
						},
						Data: map[string][]byte{
							dag.CACertificateKey: []byte("somethingsecret"),
						},
					},
				},
			},
			SubjectNames: []string{"foo.com"},
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
	tests := map[string]envoy_config_cluster_v3.Cluster_LbPolicy{
		"WeightedLeastRequest": envoy_config_cluster_v3.Cluster_LEAST_REQUEST,
		"Random":               envoy_config_cluster_v3.Cluster_RANDOM,
		"RoundRobin":           envoy_config_cluster_v3.Cluster_ROUND_ROBIN,
		"":                     envoy_config_cluster_v3.Cluster_ROUND_ROBIN,
		"unknown":              envoy_config_cluster_v3.Cluster_ROUND_ROBIN,
		"Cookie":               envoy_config_cluster_v3.Cluster_RING_HASH,
		"RequestHash":          envoy_config_cluster_v3.Cluster_RING_HASH,

		// RingHash and Maglev were removed as options in 0.13.
		// See #1150
		"RingHash": envoy_config_cluster_v3.Cluster_ROUND_ROBIN,
		"Maglev":   envoy_config_cluster_v3.Cluster_ROUND_ROBIN,
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
	want := &envoy_config_cluster_v3.Cluster_CommonLbConfig{
		HealthyPanicThreshold: &envoy_type_v3.Percent{ // Disable HealthyPanicThreshold
			Value: 0,
		},
	}
	assert.Equal(t, want, got)
}

func service(s *core_v1.Service, protocols ...string) *dag.Service {
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
			HealthPort:       s.Spec.Ports[0],
		},
		ExternalName: s.Spec.ExternalName,
		Protocol:     protocol,
	}
}

func healthcheckService(s *core_v1.Service) *dag.Service {
	return &dag.Service{
		Weighted: dag.WeightedService{
			Weight:           1,
			ServiceName:      s.Name,
			ServiceNamespace: s.Namespace,
			ServicePort:      s.Spec.Ports[0],
			HealthPort:       s.Spec.Ports[1],
		},
		ExternalName: s.Spec.ExternalName,
	}
}
