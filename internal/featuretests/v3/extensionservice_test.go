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
	envoy_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoy_v3_tls "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoy_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	matcher "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/dag"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/featuretests"
	"github.com/projectcontour/contour/internal/fixture"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/utils/pointer"
)

func extBasic(t *testing.T, rh cache.ResourceEventHandler, c *Contour) {
	rh.OnAdd(&v1alpha1.ExtensionService{
		ObjectMeta: fixture.ObjectMeta("ns/ext"),
		Spec: v1alpha1.ExtensionServiceSpec{
			Services: []v1alpha1.ExtensionServiceTarget{
				{Name: "svc1", Port: 8081},
				{Name: "svc2", Port: 8082},
			},
		},
	})

	c.Request(clusterType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		TypeUrl: clusterType,
		Resources: resources(t,
			DefaultCluster(
				h2cCluster(cluster("extension/ns/ext", "extension/ns/ext", "extension_ns_ext")),
				&envoy_cluster_v3.Cluster{
					TransportSocket: envoy_v3.UpstreamTLSTransportSocket(
						&envoy_v3_tls.UpstreamTlsContext{
							CommonTlsContext: &envoy_v3_tls.CommonTlsContext{
								AlpnProtocols: []string{"h2"},
							},
							// Note there's no SNI in this scenario.
						},
					),
				},
			),
		),
	})

	c.Request(endpointType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		TypeUrl: endpointType,
		Resources: resources(t, &envoy_endpoint_v3.ClusterLoadAssignment{
			ClusterName: "extension/ns/ext",
			Endpoints: []*envoy_endpoint_v3.LocalityLbEndpoints{
				envoy_v3.WeightedEndpoints(1, envoy_v3.SocketAddress("192.168.183.20", 8081))[0],
				envoy_v3.WeightedEndpoints(1, envoy_v3.SocketAddress("192.168.183.21", 8082))[0],
			},
		}),
	})
}

func extCleartext(t *testing.T, rh cache.ResourceEventHandler, c *Contour) {
	rh.OnAdd(&v1alpha1.ExtensionService{
		ObjectMeta: fixture.ObjectMeta("ns/ext"),
		Spec: v1alpha1.ExtensionServiceSpec{
			Protocol: pointer.StringPtr("h2c"),
			Services: []v1alpha1.ExtensionServiceTarget{
				{Name: "svc1", Port: 8081},
				{Name: "svc2", Port: 8082},
			},
		},
	})

	c.Request(clusterType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		TypeUrl: clusterType,
		Resources: resources(t,
			DefaultCluster(
				h2cCluster(cluster("extension/ns/ext", "extension/ns/ext", "extension_ns_ext")),
			),
		),
	})
}

func extUpstreamValidation(t *testing.T, rh cache.ResourceEventHandler, c *Contour) {
	ext := &v1alpha1.ExtensionService{
		ObjectMeta: fixture.ObjectMeta("ns/ext"),
		Spec: v1alpha1.ExtensionServiceSpec{
			Services: []v1alpha1.ExtensionServiceTarget{
				{Name: "svc1", Port: 8081},
			},
			UpstreamValidation: &contour_api_v1.UpstreamValidation{
				CACertificate: "cacert",
				SubjectName:   "ext.projectcontour.io",
			},
		},
	}

	rh.OnAdd(ext)

	// Enabling validation add SNI as well as CA and server altname validation.
	tlsSocket := envoy_v3.UpstreamTLSTransportSocket(
		&envoy_v3_tls.UpstreamTlsContext{
			Sni: "ext.projectcontour.io",
			CommonTlsContext: &envoy_v3_tls.CommonTlsContext{
				AlpnProtocols: []string{"h2"},
				ValidationContextType: &envoy_v3_tls.CommonTlsContext_ValidationContext{
					ValidationContext: &envoy_v3_tls.CertificateValidationContext{
						TrustedCa: &envoy_core_v3.DataSource{
							Specifier: &envoy_core_v3.DataSource_InlineBytes{
								InlineBytes: []byte(featuretests.CERTIFICATE),
							},
						},
						MatchTypedSubjectAltNames: []*envoy_v3_tls.SubjectAltNameMatcher{
							{
								SanType: envoy_v3_tls.SubjectAltNameMatcher_DNS,
								Matcher: &matcher.StringMatcher{
									MatchPattern: &matcher.StringMatcher_Exact{
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

	c.Request(clusterType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		TypeUrl: clusterType,
		Resources: resources(t,
			DefaultCluster(
				h2cCluster(cluster("extension/ns/ext", "extension/ns/ext", "extension_ns_ext")),
				&envoy_cluster_v3.Cluster{TransportSocket: tlsSocket},
			),
		),
	})

	// Update the validation spec to reference a missing secret.
	rh.OnUpdate(ext, &v1alpha1.ExtensionService{
		ObjectMeta: fixture.ObjectMeta("ns/ext"),
		Spec: v1alpha1.ExtensionServiceSpec{
			Services: []v1alpha1.ExtensionServiceTarget{
				{Name: "svc1", Port: 8081},
			},
			UpstreamValidation: &contour_api_v1.UpstreamValidation{
				CACertificate: "missing",
				SubjectName:   "ext.projectcontour.io",
			},
		},
	})

	// No Clusters are build because the CACertificate secret didn't resolve.
	c.Request(clusterType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		TypeUrl: clusterType,
	})

	// Create a secret for the CA certificate that can be delegated
	rh.OnAdd(&corev1.Secret{
		ObjectMeta: fixture.ObjectMeta("otherNs/cacert"),
		Data: map[string][]byte{
			dag.CACertificateKey: []byte(featuretests.CERTIFICATE),
		},
	})

	// Update the validation spec to reference a secret that is not delegated.
	rh.OnUpdate(ext, &v1alpha1.ExtensionService{
		ObjectMeta: fixture.ObjectMeta("ns/ext"),
		Spec: v1alpha1.ExtensionServiceSpec{
			Services: []v1alpha1.ExtensionServiceTarget{
				{Name: "svc1", Port: 8081},
			},
			UpstreamValidation: &contour_api_v1.UpstreamValidation{
				CACertificate: "otherNs/cacert",
				SubjectName:   "ext.projectcontour.io",
			},
		},
	})

	// No Clusters are build because the CACertificate secret is not delegated.
	c.Request(clusterType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		TypeUrl: clusterType,
	})

	// Delegate the CACertificate secret to be used in the ExtensionService's namespace
	rh.OnAdd(&contour_api_v1.TLSCertificateDelegation{
		ObjectMeta: fixture.ObjectMeta("otherNs/delegate-cacert"),
		Spec: contour_api_v1.TLSCertificateDelegationSpec{
			Delegations: []contour_api_v1.CertificateDelegation{{
				SecretName:       "cacert",
				TargetNamespaces: []string{"*"},
			}},
		},
	})

	// Expect cluster corresponding to the ExtensionService.
	c.Request(clusterType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		TypeUrl: clusterType,
		Resources: resources(t,
			DefaultCluster(
				h2cCluster(cluster("extension/ns/ext", "extension/ns/ext", "extension_ns_ext")),
				&envoy_cluster_v3.Cluster{TransportSocket: tlsSocket},
			),
		),
	})
}

func extExternalName(_ *testing.T, rh cache.ResourceEventHandler, c *Contour) {
	rh.OnAdd(fixture.NewService("ns/external").
		WithSpec(corev1.ServiceSpec{
			Type:         corev1.ServiceTypeExternalName,
			ExternalName: "external.projectcontour.io",
			Ports: []corev1.ServicePort{{
				Port:     443,
				Protocol: corev1.ProtocolTCP,
			}},
		}),
	)

	rh.OnAdd(&v1alpha1.ExtensionService{
		ObjectMeta: fixture.ObjectMeta("ns/ext"),
		Spec: v1alpha1.ExtensionServiceSpec{
			Services: []v1alpha1.ExtensionServiceTarget{
				{Name: "external", Port: 443},
			},
		},
	})

	// Using externalname services isn't implemented, so doesn't build a cluster.
	c.Request(clusterType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		TypeUrl: clusterType,
	})
}

func extMissingService(_ *testing.T, rh cache.ResourceEventHandler, c *Contour) {
	rh.OnAdd(&v1alpha1.ExtensionService{
		ObjectMeta: fixture.ObjectMeta("ns/ext"),
		Spec: v1alpha1.ExtensionServiceSpec{
			Services: []v1alpha1.ExtensionServiceTarget{
				{Name: "missing", Port: 443},
			},
		},
	})

	c.Request(clusterType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		TypeUrl: clusterType,
	})
}

func extInvalidTimeout(_ *testing.T, rh cache.ResourceEventHandler, c *Contour) {
	rh.OnAdd(&v1alpha1.ExtensionService{
		ObjectMeta: fixture.ObjectMeta("ns/ext"),
		Spec: v1alpha1.ExtensionServiceSpec{
			Services: []v1alpha1.ExtensionServiceTarget{
				{Name: "svc1", Port: 8081},
				{Name: "svc2", Port: 8082},
			},
			TimeoutPolicy: &contour_api_v1.TimeoutPolicy{
				Response: "invalid",
			},
		},
	})

	c.Request(clusterType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		TypeUrl: clusterType,
	})
}

func extInconsistentProto(_ *testing.T, rh cache.ResourceEventHandler, c *Contour) {
	rh.OnAdd(&v1alpha1.ExtensionService{
		ObjectMeta: fixture.ObjectMeta("ns/ext"),
		Spec: v1alpha1.ExtensionServiceSpec{
			Services: []v1alpha1.ExtensionServiceTarget{
				{Name: "svc1", Port: 8081},
			},
			Protocol: pointer.StringPtr("h2c"),
			UpstreamValidation: &contour_api_v1.UpstreamValidation{
				CACertificate: "cacert",
				SubjectName:   "ext.projectcontour.io",
			},
		},
	})

	// Should have no clusters because Protocol and UpstreamValidation is inconsistent.
	c.Request(clusterType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		TypeUrl: clusterType,
	})
}

// "Cookie" and "RequestHash" policies are not valid on ExtensionService.
func extInvalidLoadBalancerPolicy(t *testing.T, rh cache.ResourceEventHandler, c *Contour) {
	ext := &v1alpha1.ExtensionService{
		ObjectMeta: fixture.ObjectMeta("ns/ext"),
		Spec: v1alpha1.ExtensionServiceSpec{
			Services: []v1alpha1.ExtensionServiceTarget{
				{Name: "svc1", Port: 8081},
				{Name: "svc2", Port: 8082},
			},
			LoadBalancerPolicy: &contour_api_v1.LoadBalancerPolicy{
				Strategy: "Cookie",
			},
		},
	}

	rh.OnAdd(ext)

	c.Request(clusterType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		TypeUrl: clusterType,
		Resources: resources(t,
			DefaultCluster(
				// Default load balancer policy should be set as we were passed
				// an invalid value, we can assert we get a basic cluster.
				h2cCluster(cluster("extension/ns/ext", "extension/ns/ext", "extension_ns_ext")),
				&envoy_cluster_v3.Cluster{
					TransportSocket: envoy_v3.UpstreamTLSTransportSocket(
						&envoy_v3_tls.UpstreamTlsContext{
							CommonTlsContext: &envoy_v3_tls.CommonTlsContext{
								AlpnProtocols: []string{"h2"},
							},
						},
					),
				},
			),
		),
	})

	rh.OnUpdate(ext, &v1alpha1.ExtensionService{
		ObjectMeta: fixture.ObjectMeta("ns/ext"),
		Spec: v1alpha1.ExtensionServiceSpec{
			Services: []v1alpha1.ExtensionServiceTarget{
				{Name: "svc1", Port: 8081},
				{Name: "svc2", Port: 8082},
			},
			LoadBalancerPolicy: &contour_api_v1.LoadBalancerPolicy{
				Strategy: "RequestHash",
			},
		},
	})

	c.Request(clusterType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		TypeUrl: clusterType,
		Resources: resources(t,
			DefaultCluster(
				// Default load balancer policy should be set as we were passed
				// an invalid value, we can assert we get a basic cluster.
				h2cCluster(cluster("extension/ns/ext", "extension/ns/ext", "extension_ns_ext")),
				&envoy_cluster_v3.Cluster{
					TransportSocket: envoy_v3.UpstreamTLSTransportSocket(
						&envoy_v3_tls.UpstreamTlsContext{
							CommonTlsContext: &envoy_v3_tls.CommonTlsContext{
								AlpnProtocols: []string{"h2"},
							},
						},
					),
				},
			),
		),
	})
}

func TestExtensionService(t *testing.T) {
	subtests := map[string]func(*testing.T, cache.ResourceEventHandler, *Contour){
		"Basic":                     extBasic,
		"Cleartext":                 extCleartext,
		"UpstreamValidation":        extUpstreamValidation,
		"ExternalName":              extExternalName,
		"MissingService":            extMissingService,
		"InconsistentProto":         extInconsistentProto,
		"InvalidTimeout":            extInvalidTimeout,
		"InvalidLoadBalancerPolicy": extInvalidLoadBalancerPolicy,
	}

	for n, f := range subtests {
		f := f
		t.Run(n, func(t *testing.T) {
			rh, c, done := setup(t)
			defer done()

			// Add common test fixtures.

			rh.OnAdd(&corev1.Secret{
				ObjectMeta: fixture.ObjectMeta("ns/cacert"),
				Data: map[string][]byte{
					dag.CACertificateKey: []byte(featuretests.CERTIFICATE),
				},
			})

			rh.OnAdd(fixture.NewService("ns/svc1").WithPorts(corev1.ServicePort{Port: 8081}))
			rh.OnAdd(fixture.NewService("ns/svc2").WithPorts(corev1.ServicePort{Port: 8082}))

			rh.OnAdd(featuretests.Endpoints("ns", "svc1", corev1.EndpointSubset{
				Addresses: featuretests.Addresses("192.168.183.20"),
				Ports:     featuretests.Ports(featuretests.Port("", 8081)),
			}))

			rh.OnAdd(featuretests.Endpoints("ns", "svc2", corev1.EndpointSubset{
				Addresses: featuretests.Addresses("192.168.183.21"),
				Ports:     featuretests.Ports(featuretests.Port("", 8082)),
			}))

			f(t, rh, c)
		})
	}
}
