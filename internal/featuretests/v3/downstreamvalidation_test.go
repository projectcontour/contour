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

	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/dag"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/featuretests"
	"github.com/projectcontour/contour/internal/fixture"
)

func TestDownstreamTLSCertificateValidation(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	serverTLSSecret := featuretests.TLSSecret(t, "serverTLSSecret", &featuretests.ServerCertificate)
	rh.OnAdd(serverTLSSecret)

	clientCASecret := featuretests.CASecret(t, "clientCASecret", &featuretests.CACertificate)
	rh.OnAdd(clientCASecret)

	service := fixture.NewService("kuard").
		WithPorts(core_v1.ServicePort{Name: "http", Port: 8080, TargetPort: intstr.FromInt(8080)})
	rh.OnAdd(service)

	proxy1 := fixture.NewProxy("example.com").
		WithSpec(contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_v1.TLS{
					SecretName: serverTLSSecret.Name,
					ClientValidation: &contour_v1.DownstreamValidation{
						CACertificate: clientCASecret.Name,
					},
				},
			},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		})

	rh.OnAdd(proxy1)

	ingressHTTPS := &envoy_config_listener_v3.Listener{
		Name:    "ingress_https",
		Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
		ListenerFilters: envoy_v3.ListenerFilters(
			envoy_v3.TLSInspector(),
		),
		FilterChains: appendFilterChains(
			filterchaintls("example.com", serverTLSSecret,
				httpsFilterFor("example.com"),
				&dag.PeerValidationContext{
					CACertificates: []*dag.Secret{
						{
							Object: clientCASecret,
						},
					},
				},
				"h2", "http/1.1",
			),
		),
		SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
	}

	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			defaultHTTPListener(),
			ingressHTTPS,
			statsListener(),
		),
		TypeUrl: listenerType,
	}).Status(proxy1).IsValid()

	proxy2 := fixture.NewProxy("example.com").
		WithSpec(contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_v1.TLS{
					SecretName: serverTLSSecret.Name,
					ClientValidation: &contour_v1.DownstreamValidation{
						SkipClientCertValidation: true,
					},
				},
			},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		})

	rh.OnUpdate(proxy1, proxy2)

	ingressHTTPSSkipVerify := &envoy_config_listener_v3.Listener{
		Name:    "ingress_https",
		Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
		ListenerFilters: envoy_v3.ListenerFilters(
			envoy_v3.TLSInspector(),
		),
		FilterChains: appendFilterChains(
			filterchaintls("example.com", serverTLSSecret,
				httpsFilterFor("example.com"),
				&dag.PeerValidationContext{
					SkipClientCertValidation: true,
				},
				"h2", "http/1.1",
			),
		),
		SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
	}

	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			defaultHTTPListener(),
			ingressHTTPSSkipVerify,
			statsListener(),
		),
		TypeUrl: listenerType,
	}).Status(proxy2).IsValid()

	proxy3 := fixture.NewProxy("example.com").
		WithSpec(contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_v1.TLS{
					SecretName: serverTLSSecret.Name,
					ClientValidation: &contour_v1.DownstreamValidation{
						SkipClientCertValidation: true,
						CACertificate:            clientCASecret.Name,
					},
				},
			},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		})
	rh.OnUpdate(proxy2, proxy3)

	ingressHTTPSSkipVerifyWithCA := &envoy_config_listener_v3.Listener{
		Name:    "ingress_https",
		Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
		ListenerFilters: envoy_v3.ListenerFilters(
			envoy_v3.TLSInspector(),
		),
		FilterChains: appendFilterChains(
			filterchaintls("example.com", serverTLSSecret,
				httpsFilterFor("example.com"),
				&dag.PeerValidationContext{
					SkipClientCertValidation: true,
					CACertificates: []*dag.Secret{
						{
							Object: clientCASecret,
						},
					},
				},
				"h2", "http/1.1",
			),
		),
		SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
	}

	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			defaultHTTPListener(),
			ingressHTTPSSkipVerifyWithCA,
			statsListener(),
		),
		TypeUrl: listenerType,
	}).Status(proxy3).IsValid()

	crlSecret := featuretests.CRLSecret(t, "crl", &featuretests.CRL)
	rh.OnAdd(crlSecret)

	proxy4 := fixture.NewProxy("example.com").
		WithSpec(contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_v1.TLS{
					SecretName: serverTLSSecret.Name,
					ClientValidation: &contour_v1.DownstreamValidation{
						CACertificate:             clientCASecret.Name,
						CertificateRevocationList: crlSecret.Name,
					},
				},
			},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		})
	rh.OnUpdate(proxy3, proxy4)

	ingressHTTPSWithCRLandCA := &envoy_config_listener_v3.Listener{
		Name:    "ingress_https",
		Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
		ListenerFilters: envoy_v3.ListenerFilters(
			envoy_v3.TLSInspector(),
		),
		FilterChains: appendFilterChains(
			filterchaintls("example.com", serverTLSSecret,
				httpsFilterFor("example.com"),
				&dag.PeerValidationContext{
					CACertificates: []*dag.Secret{
						{
							Object: clientCASecret,
						},
					},
					CRL: &dag.Secret{
						Object: crlSecret,
					},
				},
				"h2", "http/1.1",
			),
		),
		SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
	}
	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			defaultHTTPListener(),
			ingressHTTPSWithCRLandCA,
			statsListener(),
		),
	}).Status(proxy4).IsValid()

	proxy5 := fixture.NewProxy("example.com").
		WithSpec(contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_v1.TLS{
					SecretName: serverTLSSecret.Name,
					ClientValidation: &contour_v1.DownstreamValidation{
						CACertificate:             clientCASecret.Name,
						CertificateRevocationList: crlSecret.Name,
						OnlyVerifyLeafCertCrl:     true,
					},
				},
			},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		})
	rh.OnUpdate(proxy4, proxy5)

	ingressHTTPSWithLeafCRLandCA := &envoy_config_listener_v3.Listener{
		Name:    "ingress_https",
		Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
		ListenerFilters: envoy_v3.ListenerFilters(
			envoy_v3.TLSInspector(),
		),
		FilterChains: appendFilterChains(
			filterchaintls("example.com", serverTLSSecret,
				httpsFilterFor("example.com"),
				&dag.PeerValidationContext{
					CACertificates: []*dag.Secret{
						{
							Object: clientCASecret,
						},
					},
					CRL: &dag.Secret{
						Object: crlSecret,
					},
					OnlyVerifyLeafCertCrl: true,
				},
				"h2", "http/1.1",
			),
		),
		SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
	}
	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			defaultHTTPListener(),
			ingressHTTPSWithLeafCRLandCA,
			statsListener(),
		),
	}).Status(proxy5).IsValid()

	proxy6 := fixture.NewProxy("example.com").
		WithSpec(contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_v1.TLS{
					SecretName: serverTLSSecret.Name,
					ClientValidation: &contour_v1.DownstreamValidation{
						CACertificate:             clientCASecret.Name,
						OptionalClientCertificate: true,
					},
				},
			},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		})
	rh.OnUpdate(proxy5, proxy6)

	ingressHTTPSOptionalVerify := &envoy_config_listener_v3.Listener{
		Name:    "ingress_https",
		Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
		ListenerFilters: envoy_v3.ListenerFilters(
			envoy_v3.TLSInspector(),
		),
		FilterChains: appendFilterChains(
			filterchaintls("example.com", serverTLSSecret,
				httpsFilterFor("example.com"),
				&dag.PeerValidationContext{
					CACertificates: []*dag.Secret{
						{
							Object: clientCASecret,
						},
					},
					OptionalClientCertificate: true,
				},
				"h2", "http/1.1",
			),
		),
		SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
	}
	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			defaultHTTPListener(),
			ingressHTTPSOptionalVerify,
			statsListener(),
		),
	}).Status(proxy6).IsValid()

	proxy7 := fixture.NewProxy("example.com").
		WithSpec(contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_v1.TLS{
					SecretName: serverTLSSecret.Name,
					ClientValidation: &contour_v1.DownstreamValidation{
						CACertificate: clientCASecret.Name,
						ForwardClientCertificate: &contour_v1.ClientCertificateDetails{
							Subject: true,
							Cert:    true,
							Chain:   true,
							DNS:     true,
							URI:     true,
						},
					},
				},
			},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		})
	rh.OnUpdate(proxy6, proxy7)

	ingressHTTPSForwardClientCert := &envoy_config_listener_v3.Listener{
		Name:    "ingress_https",
		Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
		ListenerFilters: envoy_v3.ListenerFilters(
			envoy_v3.TLSInspector(),
		),
		FilterChains: appendFilterChains(
			filterchaintls("example.com", serverTLSSecret,
				httpsFilterWithXfccFor("example.com", &dag.ClientCertificateDetails{
					Subject: true,
					Cert:    true,
					Chain:   true,
					DNS:     true,
					URI:     true,
				}),
				&dag.PeerValidationContext{
					CACertificates: []*dag.Secret{
						{
							Object: clientCASecret,
						},
					},
				},
				"h2", "http/1.1",
			),
		),
		SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
	}
	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			defaultHTTPListener(),
			ingressHTTPSForwardClientCert,
			statsListener(),
		),
	}).Status(proxy7).IsValid()

	proxy8 := fixture.NewProxy("example.com").
		WithSpec(contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_v1.TLS{
					SecretName: serverTLSSecret.Name,
					ClientValidation: &contour_v1.DownstreamValidation{
						SkipClientCertValidation: true,
						ForwardClientCertificate: &contour_v1.ClientCertificateDetails{
							Subject: true,
							DNS:     true,
							URI:     true,
						},
					},
				},
			},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		})
	rh.OnUpdate(proxy7, proxy8)

	ingressHTTPSForwardClientCertSkipValidation := &envoy_config_listener_v3.Listener{
		Name:    "ingress_https",
		Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
		ListenerFilters: envoy_v3.ListenerFilters(
			envoy_v3.TLSInspector(),
		),
		FilterChains: appendFilterChains(
			filterchaintls("example.com", serverTLSSecret,
				httpsFilterWithXfccFor("example.com", &dag.ClientCertificateDetails{
					Subject: true,
					DNS:     true,
					URI:     true,
				}),
				&dag.PeerValidationContext{
					SkipClientCertValidation: true,
				},
				"h2", "http/1.1",
			),
		),
		SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
	}
	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			defaultHTTPListener(),
			ingressHTTPSForwardClientCertSkipValidation,
			statsListener(),
		),
	}).Status(proxy8).IsValid()
}
