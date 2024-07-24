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
	envoy_transport_socket_tls_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	envoy_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"google.golang.org/protobuf/types/known/wrapperspb"
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/dag"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/featuretests"
	"github.com/projectcontour/contour/internal/fixture"
)

func extBasic(t *testing.T, rh ResourceEventHandlerWrapper, c *Contour) {
	rh.OnAdd(&contour_v1alpha1.ExtensionService{
		ObjectMeta: fixture.ObjectMeta("ns/ext"),
		Spec: contour_v1alpha1.ExtensionServiceSpec{
			Services: []contour_v1alpha1.ExtensionServiceTarget{
				{Name: "svc1", Port: 8081},
				{Name: "svc2", Port: 8082},
			},
		},
	})

	c.Request(clusterType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: clusterType,
		Resources: resources(t,
			DefaultCluster(
				h2cCluster(cluster("extension/ns/ext", "extension/ns/ext", "extension_ns_ext")),
				&envoy_config_cluster_v3.Cluster{
					TransportSocket: envoy_v3.UpstreamTLSTransportSocket(
						&envoy_transport_socket_tls_v3.UpstreamTlsContext{
							CommonTlsContext: &envoy_transport_socket_tls_v3.CommonTlsContext{
								AlpnProtocols: []string{"h2"},
							},
							// Note there's no SNI in this scenario.
						},
					),
				},
			),
		),
	})

	c.Request(endpointType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: endpointType,
		Resources: resources(t, &envoy_config_endpoint_v3.ClusterLoadAssignment{
			ClusterName: "extension/ns/ext",
			Endpoints: []*envoy_config_endpoint_v3.LocalityLbEndpoints{
				envoy_v3.WeightedEndpoints(1, envoy_v3.SocketAddress("192.168.183.20", 8081))[0],
				envoy_v3.WeightedEndpoints(1, envoy_v3.SocketAddress("192.168.183.21", 8082))[0],
			},
		}),
	})
}

func extCleartext(t *testing.T, rh ResourceEventHandlerWrapper, c *Contour) {
	rh.OnAdd(&contour_v1alpha1.ExtensionService{
		ObjectMeta: fixture.ObjectMeta("ns/ext"),
		Spec: contour_v1alpha1.ExtensionServiceSpec{
			Protocol: ptr.To("h2c"),
			Services: []contour_v1alpha1.ExtensionServiceTarget{
				{Name: "svc1", Port: 8081},
				{Name: "svc2", Port: 8082},
			},
		},
	})

	c.Request(clusterType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: clusterType,
		Resources: resources(t,
			DefaultCluster(
				h2cCluster(cluster("extension/ns/ext", "extension/ns/ext", "extension_ns_ext")),
			),
		),
	})
}

func extUpstreamValidation(t *testing.T, rh ResourceEventHandlerWrapper, c *Contour) {
	ext := &contour_v1alpha1.ExtensionService{
		ObjectMeta: fixture.ObjectMeta("ns/ext"),
		Spec: contour_v1alpha1.ExtensionServiceSpec{
			Services: []contour_v1alpha1.ExtensionServiceTarget{
				{Name: "svc1", Port: 8081},
			},
			UpstreamValidation: &contour_v1.UpstreamValidation{
				CACertificate: "cacert",
				SubjectName:   "ext.projectcontour.io",
			},
		},
	}

	rh.OnAdd(ext)

	// Enabling validation add SNI as well as CA and server altname validation.
	tlsSocket := envoy_v3.UpstreamTLSTransportSocket(
		&envoy_transport_socket_tls_v3.UpstreamTlsContext{
			Sni: "ext.projectcontour.io",
			CommonTlsContext: &envoy_transport_socket_tls_v3.CommonTlsContext{
				AlpnProtocols: []string{"h2"},
				ValidationContextType: &envoy_transport_socket_tls_v3.CommonTlsContext_ValidationContext{
					ValidationContext: &envoy_transport_socket_tls_v3.CertificateValidationContext{
						TrustedCa: &envoy_config_core_v3.DataSource{
							Specifier: &envoy_config_core_v3.DataSource_InlineBytes{
								InlineBytes: featuretests.PEMBytes(t, &featuretests.CACertificate),
							},
						},
						MatchTypedSubjectAltNames: []*envoy_transport_socket_tls_v3.SubjectAltNameMatcher{
							{
								SanType: envoy_transport_socket_tls_v3.SubjectAltNameMatcher_DNS,
								Matcher: &envoy_matcher_v3.StringMatcher{
									MatchPattern: &envoy_matcher_v3.StringMatcher_Exact{
										Exact: "ext.projectcontour.io",
									},
								},
							},
						},
					},
				},
			},
		},
	)

	c.Request(clusterType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: clusterType,
		Resources: resources(t,
			DefaultCluster(
				h2cCluster(cluster("extension/ns/ext", "extension/ns/ext", "extension_ns_ext")),
				&envoy_config_cluster_v3.Cluster{TransportSocket: tlsSocket},
			),
		),
	})

	// Update the validation spec to reference a missing secret.
	rh.OnUpdate(ext, &contour_v1alpha1.ExtensionService{
		ObjectMeta: fixture.ObjectMeta("ns/ext"),
		Spec: contour_v1alpha1.ExtensionServiceSpec{
			Services: []contour_v1alpha1.ExtensionServiceTarget{
				{Name: "svc1", Port: 8081},
			},
			UpstreamValidation: &contour_v1.UpstreamValidation{
				CACertificate: "missing",
				SubjectName:   "ext.projectcontour.io",
			},
		},
	})

	// No Clusters are build because the CACertificate secret didn't resolve.
	c.Request(clusterType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: clusterType,
	})

	// Create a secret for the CA certificate that can be delegated
	rh.OnAdd(featuretests.CASecret(t, "otherNs/cacert", &featuretests.CACertificate))

	// Update the validation spec to reference a secret that is not delegated.
	rh.OnUpdate(ext, &contour_v1alpha1.ExtensionService{
		ObjectMeta: fixture.ObjectMeta("ns/ext"),
		Spec: contour_v1alpha1.ExtensionServiceSpec{
			Services: []contour_v1alpha1.ExtensionServiceTarget{
				{Name: "svc1", Port: 8081},
			},
			UpstreamValidation: &contour_v1.UpstreamValidation{
				CACertificate: "otherNs/cacert",
				SubjectName:   "ext.projectcontour.io",
			},
		},
	})

	// No Clusters are build because the CACertificate secret is not delegated.
	c.Request(clusterType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: clusterType,
	})

	// Delegate the CACertificate secret to be used in the ExtensionService's namespace
	rh.OnAdd(&contour_v1.TLSCertificateDelegation{
		ObjectMeta: fixture.ObjectMeta("otherNs/delegate-cacert"),
		Spec: contour_v1.TLSCertificateDelegationSpec{
			Delegations: []contour_v1.CertificateDelegation{{
				SecretName:       "cacert",
				TargetNamespaces: []string{"*"},
			}},
		},
	})

	// Expect cluster corresponding to the ExtensionService.
	c.Request(clusterType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: clusterType,
		Resources: resources(t,
			DefaultCluster(
				h2cCluster(cluster("extension/ns/ext", "extension/ns/ext", "extension_ns_ext")),
				&envoy_config_cluster_v3.Cluster{TransportSocket: tlsSocket},
			),
		),
	})
}

func extExternalName(_ *testing.T, rh ResourceEventHandlerWrapper, c *Contour) {
	rh.OnAdd(fixture.NewService("ns/external").
		WithSpec(core_v1.ServiceSpec{
			Type:         core_v1.ServiceTypeExternalName,
			ExternalName: "external.projectcontour.io",
			Ports: []core_v1.ServicePort{{
				Port:     443,
				Protocol: core_v1.ProtocolTCP,
			}},
		}),
	)

	rh.OnAdd(&contour_v1alpha1.ExtensionService{
		ObjectMeta: fixture.ObjectMeta("ns/ext"),
		Spec: contour_v1alpha1.ExtensionServiceSpec{
			Services: []contour_v1alpha1.ExtensionServiceTarget{
				{Name: "external", Port: 443},
			},
		},
	})

	// Using externalname services isn't implemented, so doesn't build a cluster.
	c.Request(clusterType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: clusterType,
	})
}

// extIdleConnectionTimeout sets timeout on ExtensionService which will be set in cluster.
func extIdleConnectionTimeout(t *testing.T, rh ResourceEventHandlerWrapper, c *Contour) {
	rh.OnAdd(&contour_v1alpha1.ExtensionService{
		ObjectMeta: fixture.ObjectMeta("ns/ext"),
		Spec: contour_v1alpha1.ExtensionServiceSpec{
			Services: []contour_v1alpha1.ExtensionServiceTarget{
				{Name: "svc1", Port: 8081},
			},
			TimeoutPolicy: &contour_v1.TimeoutPolicy{
				IdleConnection: "60s",
			},
		},
	})

	c.Request(clusterType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: clusterType,
		Resources: resources(t,
			DefaultCluster(
				withConnectionTimeout(cluster("extension/ns/ext", "extension/ns/ext", "extension_ns_ext"), 60*time.Second, envoy_v3.HTTPVersion2),
				&envoy_config_cluster_v3.Cluster{
					TransportSocket: envoy_v3.UpstreamTLSTransportSocket(
						&envoy_transport_socket_tls_v3.UpstreamTlsContext{
							CommonTlsContext: &envoy_transport_socket_tls_v3.CommonTlsContext{
								AlpnProtocols: []string{"h2"},
							},
						},
					),
				},
			),
		),
	})
}

func extMissingService(_ *testing.T, rh ResourceEventHandlerWrapper, c *Contour) {
	rh.OnAdd(&contour_v1alpha1.ExtensionService{
		ObjectMeta: fixture.ObjectMeta("ns/ext"),
		Spec: contour_v1alpha1.ExtensionServiceSpec{
			Services: []contour_v1alpha1.ExtensionServiceTarget{
				{Name: "missing", Port: 443},
			},
		},
	})

	c.Request(clusterType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: clusterType,
	})
}

func extInvalidTimeout(_ *testing.T, rh ResourceEventHandlerWrapper, c *Contour) {
	rh.OnAdd(&contour_v1alpha1.ExtensionService{
		ObjectMeta: fixture.ObjectMeta("ns/ext"),
		Spec: contour_v1alpha1.ExtensionServiceSpec{
			Services: []contour_v1alpha1.ExtensionServiceTarget{
				{Name: "svc1", Port: 8081},
				{Name: "svc2", Port: 8082},
			},
			TimeoutPolicy: &contour_v1.TimeoutPolicy{
				Response: "invalid",
			},
		},
	})

	c.Request(clusterType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: clusterType,
	})
}

func extInconsistentProto(_ *testing.T, rh ResourceEventHandlerWrapper, c *Contour) {
	rh.OnAdd(&contour_v1alpha1.ExtensionService{
		ObjectMeta: fixture.ObjectMeta("ns/ext"),
		Spec: contour_v1alpha1.ExtensionServiceSpec{
			Services: []contour_v1alpha1.ExtensionServiceTarget{
				{Name: "svc1", Port: 8081},
			},
			Protocol: ptr.To("h2c"),
			UpstreamValidation: &contour_v1.UpstreamValidation{
				CACertificate: "cacert",
				SubjectName:   "ext.projectcontour.io",
			},
		},
	})

	// Should have no clusters because Protocol and UpstreamValidation is inconsistent.
	c.Request(clusterType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: clusterType,
	})
}

// "Cookie" and "RequestHash" policies are not valid on ExtensionService.
func extInvalidLoadBalancerPolicy(t *testing.T, rh ResourceEventHandlerWrapper, c *Contour) {
	ext := &contour_v1alpha1.ExtensionService{
		ObjectMeta: fixture.ObjectMeta("ns/ext"),
		Spec: contour_v1alpha1.ExtensionServiceSpec{
			Services: []contour_v1alpha1.ExtensionServiceTarget{
				{Name: "svc1", Port: 8081},
				{Name: "svc2", Port: 8082},
			},
			LoadBalancerPolicy: &contour_v1.LoadBalancerPolicy{
				Strategy: "Cookie",
			},
		},
	}

	rh.OnAdd(ext)

	c.Request(clusterType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: clusterType,
		Resources: resources(t,
			DefaultCluster(
				// Default load balancer policy should be set as we were passed
				// an invalid value, we can assert we get a basic cluster.
				h2cCluster(cluster("extension/ns/ext", "extension/ns/ext", "extension_ns_ext")),
				&envoy_config_cluster_v3.Cluster{
					TransportSocket: envoy_v3.UpstreamTLSTransportSocket(
						&envoy_transport_socket_tls_v3.UpstreamTlsContext{
							CommonTlsContext: &envoy_transport_socket_tls_v3.CommonTlsContext{
								AlpnProtocols: []string{"h2"},
							},
						},
					),
				},
			),
		),
	})

	rh.OnUpdate(ext, &contour_v1alpha1.ExtensionService{
		ObjectMeta: fixture.ObjectMeta("ns/ext"),
		Spec: contour_v1alpha1.ExtensionServiceSpec{
			Services: []contour_v1alpha1.ExtensionServiceTarget{
				{Name: "svc1", Port: 8081},
				{Name: "svc2", Port: 8082},
			},
			LoadBalancerPolicy: &contour_v1.LoadBalancerPolicy{
				Strategy: "RequestHash",
			},
		},
	})

	c.Request(clusterType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: clusterType,
		Resources: resources(t,
			DefaultCluster(
				// Default load balancer policy should be set as we were passed
				// an invalid value, we can assert we get a basic cluster.
				h2cCluster(cluster("extension/ns/ext", "extension/ns/ext", "extension_ns_ext")),
				&envoy_config_cluster_v3.Cluster{
					TransportSocket: envoy_v3.UpstreamTLSTransportSocket(
						&envoy_transport_socket_tls_v3.UpstreamTlsContext{
							CommonTlsContext: &envoy_transport_socket_tls_v3.CommonTlsContext{
								AlpnProtocols: []string{"h2"},
							},
						},
					),
				},
			),
		),
	})
}

func extCircuitBreakers(t *testing.T, rh ResourceEventHandlerWrapper, c *Contour) {
	ext := &contour_v1alpha1.ExtensionService{
		ObjectMeta: fixture.ObjectMeta("ns/ext"),
		Spec: contour_v1alpha1.ExtensionServiceSpec{
			Services: []contour_v1alpha1.ExtensionServiceTarget{
				{Name: "svc1", Port: 8081},
				{Name: "svc2", Port: 8082},
			},
			LoadBalancerPolicy: &contour_v1.LoadBalancerPolicy{
				Strategy: "Cookie",
			},
			CircuitBreakerPolicy: &contour_v1alpha1.CircuitBreakers{
				MaxConnections:        10000,
				MaxPendingRequests:    1048,
				MaxRequests:           494,
				MaxRetries:            10,
				PerHostMaxConnections: 1,
			},
		},
	}

	rh.OnAdd(ext)

	c.Request(clusterType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: clusterType,
		Resources: resources(t,
			DefaultCluster(
				// Default load balancer policy should be set as we were passed
				// an invalid value, we can assert we get a basic cluster.
				h2cCluster(cluster("extension/ns/ext", "extension/ns/ext", "extension_ns_ext")),
				&envoy_config_cluster_v3.Cluster{
					TransportSocket: envoy_v3.UpstreamTLSTransportSocket(
						&envoy_transport_socket_tls_v3.UpstreamTlsContext{
							CommonTlsContext: &envoy_transport_socket_tls_v3.CommonTlsContext{
								AlpnProtocols: []string{"h2"},
							},
						},
					),
					CircuitBreakers: &envoy_config_cluster_v3.CircuitBreakers{
						Thresholds: []*envoy_config_cluster_v3.CircuitBreakers_Thresholds{{
							MaxConnections:     wrapperspb.UInt32(10000),
							MaxPendingRequests: wrapperspb.UInt32(1048),
							MaxRequests:        wrapperspb.UInt32(494),
							MaxRetries:         wrapperspb.UInt32(10),
							TrackRemaining:     true,
						}},
						PerHostThresholds: []*envoy_config_cluster_v3.CircuitBreakers_Thresholds{{
							MaxConnections: wrapperspb.UInt32(1),
							TrackRemaining: true,
						}},
					},
				},
			),
		),
	})

	rh.OnUpdate(ext, &contour_v1alpha1.ExtensionService{
		ObjectMeta: fixture.ObjectMeta("ns/ext"),
		Spec: contour_v1alpha1.ExtensionServiceSpec{
			Services: []contour_v1alpha1.ExtensionServiceTarget{
				{Name: "svc1", Port: 8081},
				{Name: "svc2", Port: 8082},
			},
			LoadBalancerPolicy: &contour_v1.LoadBalancerPolicy{
				Strategy: "RequestHash",
			},
		},
	})

	c.Request(clusterType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: clusterType,
		Resources: resources(t,
			DefaultCluster(
				// Default load balancer policy should be set as we were passed
				// an invalid value, we can assert we get a basic cluster.
				h2cCluster(cluster("extension/ns/ext", "extension/ns/ext", "extension_ns_ext")),
				&envoy_config_cluster_v3.Cluster{
					TransportSocket: envoy_v3.UpstreamTLSTransportSocket(
						&envoy_transport_socket_tls_v3.UpstreamTlsContext{
							CommonTlsContext: &envoy_transport_socket_tls_v3.CommonTlsContext{
								AlpnProtocols: []string{"h2"},
							},
						},
					),
				},
			),
		),
	})
}

func extGlobalCircuitBreakers(t *testing.T, rh ResourceEventHandlerWrapper, c *Contour) {
	ext := &contour_v1alpha1.ExtensionService{
		ObjectMeta: fixture.ObjectMeta("ns/ext"),
		Spec: contour_v1alpha1.ExtensionServiceSpec{
			Services: []contour_v1alpha1.ExtensionServiceTarget{
				{Name: "svc1", Port: 8081},
				{Name: "svc2", Port: 8082},
			},
			LoadBalancerPolicy: &contour_v1.LoadBalancerPolicy{
				Strategy: "Cookie",
			},
		},
	}

	rh.OnAdd(ext)

	c.Request(clusterType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: clusterType,
		Resources: resources(t,
			DefaultCluster(
				// Default load balancer policy should be set as we were passed
				// an invalid value, we can assert we get a basic cluster.
				h2cCluster(cluster("extension/ns/ext", "extension/ns/ext", "extension_ns_ext")),
				&envoy_config_cluster_v3.Cluster{
					TransportSocket: envoy_v3.UpstreamTLSTransportSocket(
						&envoy_transport_socket_tls_v3.UpstreamTlsContext{
							CommonTlsContext: &envoy_transport_socket_tls_v3.CommonTlsContext{
								AlpnProtocols: []string{"h2"},
							},
						},
					),
					CircuitBreakers: &envoy_config_cluster_v3.CircuitBreakers{
						Thresholds: []*envoy_config_cluster_v3.CircuitBreakers_Thresholds{{
							MaxConnections:     wrapperspb.UInt32(20000),
							MaxPendingRequests: wrapperspb.UInt32(2048),
							MaxRequests:        wrapperspb.UInt32(294),
							MaxRetries:         wrapperspb.UInt32(20),
							TrackRemaining:     true,
						}},
						PerHostThresholds: []*envoy_config_cluster_v3.CircuitBreakers_Thresholds{{
							MaxConnections: wrapperspb.UInt32(10),
							TrackRemaining: true,
						}},
					},
				},
			),
		),
	})

	rh.OnUpdate(ext, &contour_v1alpha1.ExtensionService{
		ObjectMeta: fixture.ObjectMeta("ns/ext"),
		Spec: contour_v1alpha1.ExtensionServiceSpec{
			Services: []contour_v1alpha1.ExtensionServiceTarget{
				{Name: "svc1", Port: 8081},
				{Name: "svc2", Port: 8082},
			},
			LoadBalancerPolicy: &contour_v1.LoadBalancerPolicy{
				Strategy: "RequestHash",
			},
		},
	})

	c.Request(clusterType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: clusterType,
		Resources: resources(t,
			DefaultCluster(
				// Default load balancer policy should be set as we were passed
				// an invalid value, we can assert we get a basic cluster.
				h2cCluster(cluster("extension/ns/ext", "extension/ns/ext", "extension_ns_ext")),
				&envoy_config_cluster_v3.Cluster{
					TransportSocket: envoy_v3.UpstreamTLSTransportSocket(
						&envoy_transport_socket_tls_v3.UpstreamTlsContext{
							CommonTlsContext: &envoy_transport_socket_tls_v3.CommonTlsContext{
								AlpnProtocols: []string{"h2"},
							},
						},
					),
					CircuitBreakers: &envoy_config_cluster_v3.CircuitBreakers{
						Thresholds: []*envoy_config_cluster_v3.CircuitBreakers_Thresholds{{
							MaxConnections:     wrapperspb.UInt32(20000),
							MaxPendingRequests: wrapperspb.UInt32(2048),
							MaxRequests:        wrapperspb.UInt32(294),
							MaxRetries:         wrapperspb.UInt32(20),
							TrackRemaining:     true,
						}},
						PerHostThresholds: []*envoy_config_cluster_v3.CircuitBreakers_Thresholds{{
							MaxConnections: wrapperspb.UInt32(10),
							TrackRemaining: true,
						}},
					},
				},
			),
		),
	})
}

func overrideExtGlobalCircuitBreakers(t *testing.T, rh ResourceEventHandlerWrapper, c *Contour) {
	ext := &contour_v1alpha1.ExtensionService{
		ObjectMeta: fixture.ObjectMeta("ns/ext"),
		Spec: contour_v1alpha1.ExtensionServiceSpec{
			Services: []contour_v1alpha1.ExtensionServiceTarget{
				{Name: "svc1", Port: 8081},
				{Name: "svc2", Port: 8082},
			},
			LoadBalancerPolicy: &contour_v1.LoadBalancerPolicy{
				Strategy: "Cookie",
			},
			CircuitBreakerPolicy: &contour_v1alpha1.CircuitBreakers{
				MaxConnections:        30000,
				MaxPendingRequests:    3048,
				MaxRequests:           394,
				MaxRetries:            30,
				PerHostMaxConnections: 30,
			},
		},
	}

	rh.OnAdd(ext)

	c.Request(clusterType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: clusterType,
		Resources: resources(t,
			DefaultCluster(
				// Default load balancer policy should be set as we were passed
				// an invalid value, we can assert we get a basic cluster.
				h2cCluster(cluster("extension/ns/ext", "extension/ns/ext", "extension_ns_ext")),
				&envoy_config_cluster_v3.Cluster{
					TransportSocket: envoy_v3.UpstreamTLSTransportSocket(
						&envoy_transport_socket_tls_v3.UpstreamTlsContext{
							CommonTlsContext: &envoy_transport_socket_tls_v3.CommonTlsContext{
								AlpnProtocols: []string{"h2"},
							},
						},
					),
					CircuitBreakers: &envoy_config_cluster_v3.CircuitBreakers{
						Thresholds: []*envoy_config_cluster_v3.CircuitBreakers_Thresholds{{
							MaxConnections:     wrapperspb.UInt32(30000),
							MaxPendingRequests: wrapperspb.UInt32(3048),
							MaxRequests:        wrapperspb.UInt32(394),
							MaxRetries:         wrapperspb.UInt32(30),
							TrackRemaining:     true,
						}},
						PerHostThresholds: []*envoy_config_cluster_v3.CircuitBreakers_Thresholds{{
							MaxConnections: wrapperspb.UInt32(30),
							TrackRemaining: true,
						}},
					},
				},
			),
		),
	})

	rh.OnUpdate(ext, &contour_v1alpha1.ExtensionService{
		ObjectMeta: fixture.ObjectMeta("ns/ext"),
		Spec: contour_v1alpha1.ExtensionServiceSpec{
			Services: []contour_v1alpha1.ExtensionServiceTarget{
				{Name: "svc1", Port: 8081},
				{Name: "svc2", Port: 8082},
			},
			LoadBalancerPolicy: &contour_v1.LoadBalancerPolicy{
				Strategy: "RequestHash",
			},
		},
	})

	c.Request(clusterType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: clusterType,
		Resources: resources(t,
			DefaultCluster(
				// Default load balancer policy should be set as we were passed
				// an invalid value, we can assert we get a basic cluster.
				h2cCluster(cluster("extension/ns/ext", "extension/ns/ext", "extension_ns_ext")),
				&envoy_config_cluster_v3.Cluster{
					TransportSocket: envoy_v3.UpstreamTLSTransportSocket(
						&envoy_transport_socket_tls_v3.UpstreamTlsContext{
							CommonTlsContext: &envoy_transport_socket_tls_v3.CommonTlsContext{
								AlpnProtocols: []string{"h2"},
							},
						},
					),
					CircuitBreakers: &envoy_config_cluster_v3.CircuitBreakers{
						Thresholds: []*envoy_config_cluster_v3.CircuitBreakers_Thresholds{{
							MaxConnections:     wrapperspb.UInt32(20000),
							MaxPendingRequests: wrapperspb.UInt32(2048),
							MaxRequests:        wrapperspb.UInt32(294),
							MaxRetries:         wrapperspb.UInt32(20),
							TrackRemaining:     true,
						}},
						PerHostThresholds: []*envoy_config_cluster_v3.CircuitBreakers_Thresholds{{
							MaxConnections: wrapperspb.UInt32(10),
							TrackRemaining: true,
						}},
					},
				},
			),
		),
	})
}

func TestExtensionService(t *testing.T) {
	subtests := map[string]func(*testing.T, ResourceEventHandlerWrapper, *Contour){
		"Basic":                         extBasic,
		"Cleartext":                     extCleartext,
		"UpstreamValidation":            extUpstreamValidation,
		"ExternalName":                  extExternalName,
		"IdleConnectionTimeout":         extIdleConnectionTimeout,
		"MissingService":                extMissingService,
		"InconsistentProto":             extInconsistentProto,
		"InvalidTimeout":                extInvalidTimeout,
		"InvalidLoadBalancerPolicy":     extInvalidLoadBalancerPolicy,
		"CircuitBreakers":               extCircuitBreakers,
		"GlobalCircuitBreakers":         extGlobalCircuitBreakers,
		"OverrideGlobalCircuitBreakers": overrideExtGlobalCircuitBreakers,
	}

	for n, f := range subtests {
		f := f
		t.Run(n, func(t *testing.T) {
			var (
				rh   ResourceEventHandlerWrapper
				c    *Contour
				done func()
			)

			switch n {
			case "GlobalCircuitBreakers", "OverrideGlobalCircuitBreakers":
				rh, c, done = setup(t,
					func(b *dag.Builder) {
						for _, processor := range b.Processors {
							if extensionProcessor, ok := processor.(*dag.ExtensionServiceProcessor); ok {
								extensionProcessor.GlobalCircuitBreakerDefaults = &contour_v1alpha1.CircuitBreakers{
									MaxConnections:        20000,
									MaxPendingRequests:    2048,
									MaxRequests:           294,
									MaxRetries:            20,
									PerHostMaxConnections: 10,
								}
							}
						}
					})

			default:
				rh, c, done = setup(t)
			}

			defer done()

			// Add common test fixtures.

			rh.OnAdd(featuretests.CASecret(t, "ns/cacert", &featuretests.CACertificate))
			rh.OnAdd(fixture.NewService("ns/svc1").WithPorts(core_v1.ServicePort{Port: 8081}))
			rh.OnAdd(fixture.NewService("ns/svc2").WithPorts(core_v1.ServicePort{Port: 8082}))

			rh.OnAdd(featuretests.Endpoints("ns", "svc1", core_v1.EndpointSubset{
				Addresses: featuretests.Addresses("192.168.183.20"),
				Ports:     featuretests.Ports(featuretests.Port("", 8081)),
			}))

			rh.OnAdd(featuretests.Endpoints("ns", "svc2", core_v1.EndpointSubset{
				Addresses: featuretests.Addresses("192.168.183.21"),
				Ports:     featuretests.Ports(featuretests.Port("", 8082)),
			}))

			f(t, rh, c)
		})
	}
}
