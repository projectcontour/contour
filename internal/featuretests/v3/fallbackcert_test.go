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

	envoy_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_tls_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoy_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/dag"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/featuretests"
	"github.com/projectcontour/contour/internal/fixture"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestFallbackCertificate(t *testing.T) {
	rh, c, done := setup(t, func(b *dag.Builder) {
		for _, processor := range b.Processors {
			if httpProxyProcessor, ok := processor.(*dag.HTTPProxyProcessor); ok {
				httpProxyProcessor.FallbackCertificate = &types.NamespacedName{
					Name:      "fallbacksecret",
					Namespace: "admin",
				}
			}
		}

		b.Source.ConfiguredSecretRefs = []*types.NamespacedName{
			{Namespace: "admin", Name: "fallbacksecret"},
		}
	})
	defer done()

	sec1 := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret",
			Namespace: "default",
		},
		Type: "kubernetes.io/tls",
		Data: featuretests.Secretdata(featuretests.CERTIFICATE, featuretests.RSA_PRIVATE_KEY),
	}
	rh.OnAdd(sec1)

	fallbackSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "fallbacksecret",
			Namespace: "admin",
		},
		Type: "kubernetes.io/tls",
		Data: featuretests.Secretdata(featuretests.CERTIFICATE, featuretests.RSA_PRIVATE_KEY),
	}

	rh.OnAdd(fallbackSecret)

	s1 := fixture.NewService("backend").
		WithPorts(v1.ServicePort{Name: "http", Port: 80})
	rh.OnAdd(s1)

	// Valid HTTPProxy without FallbackCertificate enabled
	proxy1 := fixture.NewProxy("simple").WithSpec(
		contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "fallback.example.com",
				TLS: &contour_api_v1.TLS{
					SecretName:                "secret",
					EnableFallbackCertificate: false,
				},
			},
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: s1.Name,
					Port: 80,
				}},
			}},
		})

	rh.OnAdd(proxy1)

	// We should start with a single generic HTTPS service.
	c.Request(listenerType, "ingress_https").Equals(&envoy_discovery_v3.DiscoveryResponse{
		TypeUrl: listenerType,
		Resources: resources(t,
			&envoy_listener_v3.Listener{
				Name:    "ingress_https",
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				FilterChains: appendFilterChains(
					filterchaintls("fallback.example.com", sec1,
						httpsFilterFor("fallback.example.com"),
						nil, "h2", "http/1.1"),
				),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			},
		),
	})

	// Valid HTTPProxy with FallbackCertificate enabled
	proxy2 := fixture.NewProxy("simple").WithSpec(
		contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "fallback.example.com",
				TLS: &contour_api_v1.TLS{
					SecretName:                "secret",
					EnableFallbackCertificate: true,
				},
			},
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: s1.Name,
					Port: 80,
				}},
			}},
		})

	rh.OnUpdate(proxy1, proxy2)

	// Invalid since there's no TLSCertificateDelegation configured
	c.Request(listenerType, "ingress_https").Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: nil,
		TypeUrl:   listenerType,
	})

	certDelegationAll := &contour_api_v1.TLSCertificateDelegation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "fallbackcertdelegation",
			Namespace: "admin",
		},
		Spec: contour_api_v1.TLSCertificateDelegationSpec{
			Delegations: []contour_api_v1.CertificateDelegation{{
				SecretName:       "fallbacksecret",
				TargetNamespaces: []string{"*"},
			}},
		},
	}

	rh.OnAdd(certDelegationAll)

	// Now we should still have the generic HTTPS service filter,
	// but also the fallback certificate filter.
	c.Request(listenerType, "ingress_https").Equals(&envoy_discovery_v3.DiscoveryResponse{
		TypeUrl: listenerType,
		Resources: resources(t,
			&envoy_listener_v3.Listener{
				Name:    "ingress_https",
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				FilterChains: appendFilterChains(
					filterchaintls("fallback.example.com", sec1,
						httpsFilterFor("fallback.example.com"),
						nil, "h2", "http/1.1"),
					filterchaintlsfallback(fallbackSecret, nil, "h2", "http/1.1"),
				),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			},
		),
	})

	rh.OnDelete(certDelegationAll)

	c.Request(listenerType, "ingress_https").Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: nil,
		TypeUrl:   listenerType,
	})

	certDelegationSingle := &contour_api_v1.TLSCertificateDelegation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "fallbackcertdelegation",
			Namespace: "admin",
		},
		Spec: contour_api_v1.TLSCertificateDelegationSpec{
			Delegations: []contour_api_v1.CertificateDelegation{{
				SecretName:       "fallbacksecret",
				TargetNamespaces: []string{"default"},
			}},
		},
	}

	rh.OnAdd(certDelegationSingle)

	c.Request(listenerType, "ingress_https").Equals(&envoy_discovery_v3.DiscoveryResponse{
		TypeUrl: listenerType,
		Resources: resources(t,
			&envoy_listener_v3.Listener{
				Name:    "ingress_https",
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				FilterChains: appendFilterChains(
					filterchaintls("fallback.example.com", sec1,
						httpsFilterFor("fallback.example.com"),
						nil, "h2", "http/1.1"),
					filterchaintlsfallback(fallbackSecret, nil, "h2", "http/1.1"),
				),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			},
		),
	})

	// Invalid HTTPProxy with FallbackCertificate enabled along with ClientValidation
	proxy3 := fixture.NewProxy("simple").WithSpec(
		contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "fallback.example.com",
				TLS: &contour_api_v1.TLS{
					SecretName:                "secret",
					EnableFallbackCertificate: true,
					ClientValidation: &contour_api_v1.DownstreamValidation{
						CACertificate: "something",
					},
				},
			},
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: s1.Name,
					Port: 80,
				}},
			}},
		})

	rh.OnUpdate(proxy2, proxy3)

	c.Request(listenerType, "ingress_https").Equals(&envoy_discovery_v3.DiscoveryResponse{
		TypeUrl:   listenerType,
		Resources: nil,
	})

	// Valid HTTPProxy with FallbackCertificate enabled
	proxy4 := fixture.NewProxy("simple-two").WithSpec(
		contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "anotherfallback.example.com",
				TLS: &contour_api_v1.TLS{
					SecretName:                "secret",
					EnableFallbackCertificate: true,
				},
			},
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: s1.Name,
					Port: 80,
				}},
			}},
		})

	rh.OnUpdate(proxy3, proxy2) // proxy3 is invalid, resolve that to test two valid proxies
	rh.OnAdd(proxy4)

	c.Request(listenerType, "ingress_https").Equals(&envoy_discovery_v3.DiscoveryResponse{
		TypeUrl: listenerType,
		Resources: resources(t,
			&envoy_listener_v3.Listener{
				Name:    "ingress_https",
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				FilterChains: appendFilterChains(
					filterchaintls("anotherfallback.example.com", sec1,
						httpsFilterFor("anotherfallback.example.com"),
						nil, "h2", "http/1.1"),
					filterchaintls("fallback.example.com", sec1,
						httpsFilterFor("fallback.example.com"),
						nil, "h2", "http/1.1"),
					filterchaintlsfallback(fallbackSecret, nil, "h2", "http/1.1"),
				),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			},
		),
	})

	// We should have emitted TLS certificate secrets for both
	// the proxy certificate and for the fallback certificate.
	c.Request(secretType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		TypeUrl: secretType,
		Resources: resources(t,
			&envoy_tls_v3.Secret{
				Name: "admin/fallbacksecret/0567f551af",
				Type: &envoy_tls_v3.Secret_TlsCertificate{
					TlsCertificate: &envoy_tls_v3.TlsCertificate{
						CertificateChain: &envoy_core_v3.DataSource{
							Specifier: &envoy_core_v3.DataSource_InlineBytes{
								InlineBytes: fallbackSecret.Data[v1.TLSCertKey],
							},
						},
						PrivateKey: &envoy_core_v3.DataSource{
							Specifier: &envoy_core_v3.DataSource_InlineBytes{
								InlineBytes: fallbackSecret.Data[v1.TLSPrivateKeyKey],
							},
						},
					},
				},
			},
			&envoy_tls_v3.Secret{
				Name: "default/secret/0567f551af",
				Type: &envoy_tls_v3.Secret_TlsCertificate{
					TlsCertificate: &envoy_tls_v3.TlsCertificate{
						CertificateChain: &envoy_core_v3.DataSource{
							Specifier: &envoy_core_v3.DataSource_InlineBytes{
								InlineBytes: sec1.Data[v1.TLSCertKey],
							},
						},
						PrivateKey: &envoy_core_v3.DataSource{
							Specifier: &envoy_core_v3.DataSource_InlineBytes{
								InlineBytes: sec1.Data[v1.TLSPrivateKeyKey],
							},
						},
					},
				},
			},
		),
	})

	rh.OnDelete(fallbackSecret)

	c.Request(listenerType, "ingress_https").Equals(&envoy_discovery_v3.DiscoveryResponse{
		TypeUrl:   listenerType,
		Resources: nil,
	})

	rh.OnDelete(proxy4)
	rh.OnDelete(proxy2)

	c.Request(secretType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		TypeUrl:   secretType,
		Resources: nil,
	})
}
