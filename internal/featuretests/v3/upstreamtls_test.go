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

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_transport_socket_tls_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	envoy_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	core_v1 "k8s.io/api/core/v1"
	networking_v1 "k8s.io/api/networking/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	gatewayapi_v1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayapi_v1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/dag"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/featuretests"
	"github.com/projectcontour/contour/internal/fixture"
	"github.com/projectcontour/contour/internal/gatewayapi"
)

func TestUpstreamTLSWithHTTPProxy(t *testing.T) {
	rh, c, done := setup(t, proxyClientCertificateOpt(t), func(b *dag.Builder) {
		for _, processor := range b.Processors {
			if httpProxyProcessor, ok := processor.(*dag.HTTPProxyProcessor); ok {
				httpProxyProcessor.UpstreamTLS = &dag.UpstreamTLS{
					MinimumProtocolVersion: "1.2",
					MaximumProtocolVersion: "1.2",
				}
			}
		}
	})
	defer done()

	clientSecret := featuretests.TLSSecret(t, "envoyclientsecret", &featuretests.ClientCertificate)
	caSecret := featuretests.CASecret(t, "backendcacert", &featuretests.CACertificate)
	rh.OnAdd(clientSecret)
	rh.OnAdd(caSecret)

	svc := fixture.NewService("backend").
		WithPorts(core_v1.ServicePort{Name: "http", Port: 443})
	rh.OnAdd(svc)

	proxy := fixture.NewProxy("authenticated").WithSpec(
		contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "www.example.com",
			},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name:     svc.Name,
					Port:     443,
					Protocol: ptr.To("tls"),
					UpstreamValidation: &contour_v1.UpstreamValidation{
						CACertificate: caSecret.Name,
						SubjectName:   "subjname",
					},
				}},
			}},
		})
	rh.OnAdd(proxy)

	c.Request(clusterType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			tlsCluster(
				cluster("default/backend/443/950c17581f", "default/backend/http", "default_backend_443"),
				caSecret,
				"subjname",
				"",
				clientSecret,
				&dag.UpstreamTLS{
					MinimumProtocolVersion: "1.2",
					MaximumProtocolVersion: "1.2",
				}),
		),
		TypeUrl: clusterType,
	})
}

func TestUpstreamTLSWithIngress(t *testing.T) {
	rh, c, done := setup(t, func(b *dag.Builder) {
		for _, processor := range b.Processors {
			if ingressProcessor, ok := processor.(*dag.IngressProcessor); ok {
				ingressProcessor.UpstreamTLS = &dag.UpstreamTLS{
					MinimumProtocolVersion: "1.2",
					MaximumProtocolVersion: "1.2",
				}
			}
		}
	})
	defer done()

	s1 := fixture.NewService("kuard").
		Annotate("projectcontour.io/upstream-protocol.tls", "securebackend").
		WithPorts(core_v1.ServicePort{Name: "securebackend", Port: 443, TargetPort: intstr.FromInt(8888)})
	rh.OnAdd(s1)

	i1 := &networking_v1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: networking_v1.IngressSpec{
			DefaultBackend: featuretests.IngressBackend(s1),
		},
	}
	rh.OnAdd(i1)

	c.Request(clusterType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			tlsCluster(
				cluster("default/kuard/443/4929fca9d4", "default/kuard/securebackend", "default_kuard_443"),
				nil,
				"",
				"",
				nil,
				&dag.UpstreamTLS{
					MinimumProtocolVersion: "1.2",
					MaximumProtocolVersion: "1.2",
				},
			),
		),
		TypeUrl: clusterType,
	})
}

func TestUpstreamTLSWithExtensionService(t *testing.T) {
	rh, c, done := setup(t, func(b *dag.Builder) {
		for _, processor := range b.Processors {
			if extensionServiceProcessor, ok := processor.(*dag.ExtensionServiceProcessor); ok {
				extensionServiceProcessor.UpstreamTLS = &dag.UpstreamTLS{
					MinimumProtocolVersion: "1.2",
					MaximumProtocolVersion: "1.2",
				}
			}
		}
	})
	defer done()

	// Add common test fixtures.

	rh.OnAdd(featuretests.CASecret(t, "ns/cacert", &featuretests.CACertificate))

	rh.OnAdd(fixture.NewService("ns/svc1").WithPorts(core_v1.ServicePort{Port: 8081}))

	rh.OnAdd(featuretests.Endpoints("ns", "svc1", core_v1.EndpointSubset{
		Addresses: featuretests.Addresses("192.168.183.20"),
		Ports:     featuretests.Ports(featuretests.Port("", 8081)),
	}))

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
				TlsParams: &envoy_transport_socket_tls_v3.TlsParameters{
					TlsMinimumProtocolVersion: envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2,
					TlsMaximumProtocolVersion: envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2,
				},
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
}

func TestUpstreamTLSWithHTTPRoute(t *testing.T) {
	rh, c, done := setup(t, func(b *dag.Builder) {
		for _, processor := range b.Processors {
			if gatewayAPIProcessor, ok := processor.(*dag.GatewayAPIProcessor); ok {
				gatewayAPIProcessor.UpstreamTLS = &dag.UpstreamTLS{
					MinimumProtocolVersion: "1.2",
					MaximumProtocolVersion: "1.2",
				}
			}
		}
	})
	defer done()

	sec1 := featuretests.TLSSecret(t, "sec1", &featuretests.ClientCertificate)
	sec2 := featuretests.CASecret(t, "sec2", &featuretests.CACertificate)
	rh.OnAdd(sec1)
	rh.OnAdd(sec2)

	rh.OnAdd(&gatewayapi_v1.GatewayClass{
		TypeMeta:   meta_v1.TypeMeta{},
		ObjectMeta: fixture.ObjectMeta("test-gc"),
		Spec: gatewayapi_v1.GatewayClassSpec{
			ControllerName: "projectcontour.io/contour",
		},
		Status: gatewayapi_v1.GatewayClassStatus{
			Conditions: []meta_v1.Condition{
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusAccepted),
					Status: meta_v1.ConditionTrue,
				},
			},
		},
	})

	gateway := &gatewayapi_v1.Gateway{
		ObjectMeta: fixture.ObjectMeta("projectcontour/contour"),
		Spec: gatewayapi_v1.GatewaySpec{
			Listeners: []gatewayapi_v1.Listener{{
				Name:     "http",
				Port:     80,
				Protocol: gatewayapi_v1.HTTPProtocolType,
				AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
					Namespaces: &gatewayapi_v1.RouteNamespaces{
						From: ptr.To(gatewayapi_v1.NamespacesFromAll),
					},
				},
			}},
		},
	}
	rh.OnAdd(gateway)

	svc := fixture.NewService("backend").
		WithPorts(core_v1.ServicePort{Name: "http", Port: 443})
	rh.OnAdd(svc)

	rh.OnAdd(&gatewayapi_v1.HTTPRoute{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "authenticated",
			Namespace: "default",
		},
		Spec: gatewayapi_v1.HTTPRouteSpec{
			CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
				ParentRefs: []gatewayapi_v1.ParentReference{
					gatewayapi.GatewayParentRef("projectcontour", "contour"),
				},
			},
			Hostnames: []gatewayapi_v1.Hostname{
				"test.projectcontour.io",
			},
			Rules: []gatewayapi_v1.HTTPRouteRule{{
				Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
				BackendRefs: gatewayapi.HTTPBackendRef("backend", 443, 1),
			}},
		},
	})

	rh.OnAdd(&gatewayapi_v1alpha3.BackendTLSPolicy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "authenticated",
			Namespace: "default",
		},
		Spec: gatewayapi_v1alpha3.BackendTLSPolicySpec{
			TargetRefs: []gatewayapi_v1alpha2.LocalPolicyTargetReferenceWithSectionName{
				{
					LocalPolicyTargetReference: gatewayapi_v1alpha2.LocalPolicyTargetReference{
						Kind: "Service",
						Name: "backend",
					},
				},
			},
			Validation: gatewayapi_v1alpha3.BackendTLSPolicyValidation{
				CACertificateRefs: []gatewayapi_v1.LocalObjectReference{{
					Kind: "Secret",
					Name: gatewayapi_v1.ObjectName(sec2.Name),
				}},
				Hostname: "subjname",
			},
		},
	})

	c.Request(clusterType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			tlsCluster(
				cluster("default/backend/443/867941ed65", "default/backend/http", "default_backend_443"),
				sec2,
				"subjname",
				"",
				nil,
				&dag.UpstreamTLS{
					MinimumProtocolVersion: "1.2",
					MaximumProtocolVersion: "1.2",
				}),
		),
		TypeUrl: clusterType,
	})
}

func TestBackendTLSPolicyPrecedenceOverUpstreamProtocolAnnotationWithHTTPRoute(t *testing.T) {
	rh, c, done := setup(t, func(b *dag.Builder) {
		for _, processor := range b.Processors {
			if gatewayAPIProcessor, ok := processor.(*dag.GatewayAPIProcessor); ok {
				gatewayAPIProcessor.UpstreamTLS = &dag.UpstreamTLS{
					MinimumProtocolVersion: "1.2",
					MaximumProtocolVersion: "1.2",
				}
			}
		}
	})
	defer done()

	sec1 := featuretests.CASecret(t, "sec1", &featuretests.CACertificate)
	rh.OnAdd(sec1)

	rh.OnAdd(&gatewayapi_v1.GatewayClass{
		TypeMeta:   meta_v1.TypeMeta{},
		ObjectMeta: fixture.ObjectMeta("test-gc"),
		Spec: gatewayapi_v1.GatewayClassSpec{
			ControllerName: "projectcontour.io/contour",
		},
		Status: gatewayapi_v1.GatewayClassStatus{
			Conditions: []meta_v1.Condition{
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusAccepted),
					Status: meta_v1.ConditionTrue,
				},
			},
		},
	})

	gateway := &gatewayapi_v1.Gateway{
		ObjectMeta: fixture.ObjectMeta("projectcontour/contour"),
		Spec: gatewayapi_v1.GatewaySpec{
			Listeners: []gatewayapi_v1.Listener{{
				Name:     "http",
				Port:     80,
				Protocol: gatewayapi_v1.HTTPProtocolType,
				AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
					Namespaces: &gatewayapi_v1.RouteNamespaces{
						From: ptr.To(gatewayapi_v1.NamespacesFromAll),
					},
				},
			}},
		},
	}
	rh.OnAdd(gateway)

	svc := fixture.NewService("backend").
		Annotate("projectcontour.io/upstream-protocol.h2", "443").
		WithPorts(core_v1.ServicePort{Name: "http", Port: 443})
	rh.OnAdd(svc)

	rh.OnAdd(&gatewayapi_v1.HTTPRoute{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "authenticated",
			Namespace: "default",
		},
		Spec: gatewayapi_v1.HTTPRouteSpec{
			CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
				ParentRefs: []gatewayapi_v1.ParentReference{
					gatewayapi.GatewayParentRef("projectcontour", "contour"),
				},
			},
			Hostnames: []gatewayapi_v1.Hostname{
				"test.projectcontour.io",
			},
			Rules: []gatewayapi_v1.HTTPRouteRule{{
				Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
				BackendRefs: gatewayapi.HTTPBackendRef("backend", 443, 1),
			}},
		},
	})

	rh.OnAdd(&gatewayapi_v1alpha3.BackendTLSPolicy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "authenticated",
			Namespace: "default",
		},
		Spec: gatewayapi_v1alpha3.BackendTLSPolicySpec{
			TargetRefs: []gatewayapi_v1alpha2.LocalPolicyTargetReferenceWithSectionName{
				{
					LocalPolicyTargetReference: gatewayapi_v1alpha2.LocalPolicyTargetReference{
						Kind: "Service",
						Name: "backend",
					},
				},
			},
			Validation: gatewayapi_v1alpha3.BackendTLSPolicyValidation{
				CACertificateRefs: []gatewayapi_v1.LocalObjectReference{{
					Kind: "Secret",
					Name: gatewayapi_v1.ObjectName(sec1.Name),
				}},
				Hostname: "subjname",
			},
		},
	})

	c.Request(clusterType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			tlsCluster(
				cluster("default/backend/443/242c9163af", "default/backend/http", "default_backend_443"),
				sec1,
				"subjname",
				"",
				nil,
				&dag.UpstreamTLS{
					MinimumProtocolVersion: "1.2",
					MaximumProtocolVersion: "1.2",
				}),
		),
		TypeUrl: clusterType,
	})
}

// Test that a unique cluster name is generated when there is an HTTPProxy with upstream TLS settings
// and an HTTPRoute with a BackendTLSPolicy, configured with unique TLS settings, targeting the same service.
func TestUpstreamTLSWithHTTPRouteANDHTTPProxy(t *testing.T) {
	rh, c, done := setup(t, func(b *dag.Builder) {
		for _, processor := range b.Processors {
			if httpProxyProcessor, ok := processor.(*dag.HTTPProxyProcessor); ok {
				httpProxyProcessor.UpstreamTLS = &dag.UpstreamTLS{
					MinimumProtocolVersion: "1.2",
					MaximumProtocolVersion: "1.2",
				}
			}
			if gatewayAPIProcessor, ok := processor.(*dag.GatewayAPIProcessor); ok {
				gatewayAPIProcessor.UpstreamTLS = &dag.UpstreamTLS{
					MinimumProtocolVersion: "1.2",
					MaximumProtocolVersion: "1.2",
				}
			}
		}
	})
	defer done()

	caSecret := featuretests.CASecret(t, "backendcacert", &featuretests.CACertificate)
	rh.OnAdd(caSecret)

	sec1 := featuretests.CASecret(t, "sec1", &featuretests.CACertificate)
	rh.OnAdd(sec1)

	svc := fixture.NewService("backend").
		WithPorts(core_v1.ServicePort{Name: "http", Port: 443})
	rh.OnAdd(svc)

	proxy := fixture.NewProxy("authenticated").WithSpec(
		contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "www.example.com",
			},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name:     svc.Name,
					Port:     443,
					Protocol: ptr.To("tls"),
					UpstreamValidation: &contour_v1.UpstreamValidation{
						CACertificate: caSecret.Name,
						SubjectName:   "subjname",
					},
				}},
			}},
		})
	rh.OnAdd(proxy)

	rh.OnAdd(&gatewayapi_v1.GatewayClass{
		TypeMeta:   meta_v1.TypeMeta{},
		ObjectMeta: fixture.ObjectMeta("test-gc"),
		Spec: gatewayapi_v1.GatewayClassSpec{
			ControllerName: "projectcontour.io/contour",
		},
		Status: gatewayapi_v1.GatewayClassStatus{
			Conditions: []meta_v1.Condition{
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusAccepted),
					Status: meta_v1.ConditionTrue,
				},
			},
		},
	})

	gateway := &gatewayapi_v1.Gateway{
		ObjectMeta: fixture.ObjectMeta("projectcontour/contour"),
		Spec: gatewayapi_v1.GatewaySpec{
			Listeners: []gatewayapi_v1.Listener{{
				Name:     "http",
				Port:     80,
				Protocol: gatewayapi_v1.HTTPProtocolType,
				AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
					Namespaces: &gatewayapi_v1.RouteNamespaces{
						From: ptr.To(gatewayapi_v1.NamespacesFromAll),
					},
				},
			}},
		},
	}
	rh.OnAdd(gateway)

	rh.OnAdd(&gatewayapi_v1.HTTPRoute{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "authenticated",
			Namespace: "default",
		},
		Spec: gatewayapi_v1.HTTPRouteSpec{
			CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
				ParentRefs: []gatewayapi_v1.ParentReference{
					gatewayapi.GatewayParentRef("projectcontour", "contour"),
				},
			},
			Hostnames: []gatewayapi_v1.Hostname{
				"test.projectcontour.io",
			},
			Rules: []gatewayapi_v1.HTTPRouteRule{{
				Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
				BackendRefs: gatewayapi.HTTPBackendRef("backend", 443, 1),
			}},
		},
	})

	rh.OnAdd(&gatewayapi_v1alpha3.BackendTLSPolicy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "authenticated",
			Namespace: "default",
		},
		Spec: gatewayapi_v1alpha3.BackendTLSPolicySpec{
			TargetRefs: []gatewayapi_v1alpha2.LocalPolicyTargetReferenceWithSectionName{
				{
					LocalPolicyTargetReference: gatewayapi_v1alpha2.LocalPolicyTargetReference{
						Kind: "Service",
						Name: "backend",
					},
				},
			},
			Validation: gatewayapi_v1alpha3.BackendTLSPolicyValidation{
				CACertificateRefs: []gatewayapi_v1.LocalObjectReference{{
					Kind: "Secret",
					Name: gatewayapi_v1.ObjectName(sec1.Name),
				}},
				Hostname: "subjname",
			},
		},
	})

	c.Request(clusterType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			tlsCluster(
				cluster("default/backend/443/242c9163af", "default/backend/http", "default_backend_443"),
				sec1,
				"subjname",
				"",
				nil,
				&dag.UpstreamTLS{
					MinimumProtocolVersion: "1.2",
					MaximumProtocolVersion: "1.2",
				}),
			tlsCluster(
				cluster("default/backend/443/950c17581f", "default/backend/http", "default_backend_443"),
				caSecret,
				"subjname",
				"",
				nil,
				&dag.UpstreamTLS{
					MinimumProtocolVersion: "1.2",
					MaximumProtocolVersion: "1.2",
				}),
		),
		TypeUrl: clusterType,
	})
}
