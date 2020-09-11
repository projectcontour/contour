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

package featuretests

import (
	"testing"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_api_v2_auth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	envoy_api_v2_core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoy_api_v2_endpoint "github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
	matcher "github.com/envoyproxy/go-control-plane/envoy/type/matcher"
	projcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/envoy"
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

	c.Request(clusterType).Equals(&v2.DiscoveryResponse{
		TypeUrl: clusterType,
		Resources: resources(t,
			DefaultCluster(
				h2cCluster(cluster("extension/ns/ext", "extension/ns/ext", "extension_ns_ext")),
				&v2.Cluster{
					TransportSocket: envoy.UpstreamTLSTransportSocket(
						&envoy_api_v2_auth.UpstreamTlsContext{
							CommonTlsContext: &envoy_api_v2_auth.CommonTlsContext{
								AlpnProtocols: []string{"h2"},
							},
							// Note there's no SNI in this scenario.
						},
					),
				},
			),
		),
	})

	c.Request(endpointType).Equals(&v2.DiscoveryResponse{
		TypeUrl: endpointType,
		Resources: resources(t, &v2.ClusterLoadAssignment{
			ClusterName: "extension/ns/ext",
			Endpoints: []*envoy_api_v2_endpoint.LocalityLbEndpoints{
				envoy.WeightedEndpoints(1, envoy.SocketAddress("192.168.183.20", 8081))[0],
				envoy.WeightedEndpoints(1, envoy.SocketAddress("192.168.183.21", 8082))[0],
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

	c.Request(clusterType).Equals(&v2.DiscoveryResponse{
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
			UpstreamValidation: &projcontour.UpstreamValidation{
				CACertificate: "cacert",
				SubjectName:   "ext.projectcontour.io",
			},
		},
	}

	rh.OnAdd(ext)

	// Enabling validation add SNI as well as CA and server altname validation.
	tlsSocket := envoy.UpstreamTLSTransportSocket(
		&envoy_api_v2_auth.UpstreamTlsContext{
			Sni: "ext.projectcontour.io",
			CommonTlsContext: &envoy_api_v2_auth.CommonTlsContext{
				AlpnProtocols: []string{"h2"},
				ValidationContextType: &envoy_api_v2_auth.CommonTlsContext_ValidationContext{
					ValidationContext: &envoy_api_v2_auth.CertificateValidationContext{
						TrustedCa: &envoy_api_v2_core.DataSource{
							Specifier: &envoy_api_v2_core.DataSource_InlineBytes{
								InlineBytes: []byte(CERTIFICATE),
							},
						},
						MatchSubjectAltNames: []*matcher.StringMatcher{{
							MatchPattern: &matcher.StringMatcher_Exact{
								Exact: "ext.projectcontour.io",
							}},
						},
					},
				},
			},
		},
	)

	c.Request(clusterType).Equals(&v2.DiscoveryResponse{
		TypeUrl: clusterType,
		Resources: resources(t,
			DefaultCluster(
				h2cCluster(cluster("extension/ns/ext", "extension/ns/ext", "extension_ns_ext")),
				&v2.Cluster{TransportSocket: tlsSocket},
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
			UpstreamValidation: &projcontour.UpstreamValidation{
				CACertificate: "missing",
				SubjectName:   "ext.projectcontour.io",
			},
		},
	})

	// No Clusters are build because the CACertificate secret didn't resolve.
	c.Request(clusterType).Equals(&v2.DiscoveryResponse{
		TypeUrl: clusterType,
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
	c.Request(clusterType).Equals(&v2.DiscoveryResponse{
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

	c.Request(clusterType).Equals(&v2.DiscoveryResponse{
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
			TimeoutPolicy: &projcontour.TimeoutPolicy{
				Response: "invalid",
			},
		},
	})

	c.Request(clusterType).Equals(&v2.DiscoveryResponse{
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
			UpstreamValidation: &projcontour.UpstreamValidation{
				CACertificate: "cacert",
				SubjectName:   "ext.projectcontour.io",
			},
		},
	})

	// Should have no clusters because Protocol and UpstreamValidation is inconsistent.
	c.Request(clusterType).Equals(&v2.DiscoveryResponse{
		TypeUrl: clusterType,
	})
}

func TestExtensionService(t *testing.T) {
	subtests := map[string]func(*testing.T, cache.ResourceEventHandler, *Contour){
		"Basic":              extBasic,
		"Cleartext":          extCleartext,
		"UpstreamValidation": extUpstreamValidation,
		"ExternalName":       extExternalName,
		"MissingService":     extMissingService,
		"InconsistentProto":  extInconsistentProto,
		"InvalidTimeout":     extInvalidTimeout,
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
					dag.CACertificateKey: []byte(CERTIFICATE),
				},
			})

			rh.OnAdd(fixture.NewService("ns/svc1").WithPorts(corev1.ServicePort{Port: 8081}))
			rh.OnAdd(fixture.NewService("ns/svc2").WithPorts(corev1.ServicePort{Port: 8082}))

			rh.OnAdd(endpoints("ns", "svc1", corev1.EndpointSubset{
				Addresses: addresses("192.168.183.20"),
				Ports:     ports(port("", 8081)),
			}))

			rh.OnAdd(endpoints("ns", "svc2", corev1.EndpointSubset{
				Addresses: addresses("192.168.183.21"),
				Ports:     ports(port("", 8082)),
			}))

			f(t, rh, c)
		})
	}
}
