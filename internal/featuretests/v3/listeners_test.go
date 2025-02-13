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

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_transport_socket_tls_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	core_v1 "k8s.io/api/core/v1"
	networking_v1 "k8s.io/api/networking/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	gatewayapi_v1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/envoy"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/featuretests"
	"github.com/projectcontour/contour/internal/fixture"
	"github.com/projectcontour/contour/internal/gatewayapi"
	"github.com/projectcontour/contour/internal/timeout"
	"github.com/projectcontour/contour/internal/xdscache"
	xdscache_v3 "github.com/projectcontour/contour/internal/xdscache/v3"
)

func customAdminPort(t *testing.T, port int) []xdscache.ResourceCache {
	log := fixture.NewTestLogger(t)
	et := xdscache_v3.NewEndpointsTranslator(log)
	conf := xdscache_v3.ListenerConfig{}
	return []xdscache.ResourceCache{
		xdscache_v3.NewListenerCache(
			conf,
			contour_v1alpha1.MetricsConfig{Address: "0.0.0.0", Port: 8002},
			contour_v1alpha1.HealthConfig{Address: "0.0.0.0", Port: 8002},
			port,
			envoy_v3.NewEnvoyGen(envoy_v3.EnvoyGenOpt{
				XDSClusterName: envoy_v3.DefaultXDSClusterName,
			}),
		),
		&xdscache_v3.SecretCache{},
		&xdscache_v3.RouteCache{},
		&xdscache_v3.ClusterCache{},
		et,
	}
}

func TestNonTLSListener(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	// assert that without any ingress objects registered
	// there are no active listeners
	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		VersionInfo: "0",
		Resources: resources(t,
			statsListener(),
		),
		TypeUrl: listenerType,
		Nonce:   "0",
	})

	svc1 := fixture.NewService("backend").
		WithPorts(core_v1.ServicePort{Name: "http", Port: 80})
	rh.OnAdd(svc1)

	// i1 is a simple ingress, no hostname, no tls.
	i1 := &networking_v1.Ingress{
		ObjectMeta: fixture.ObjectMeta("default/simple"),
		Spec: networking_v1.IngressSpec{
			DefaultBackend: featuretests.IngressBackend(svc1),
		},
	}

	// add it and assert that we now have a ingress_http listener
	rh.OnAdd(i1)
	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			defaultHTTPListener(),
			statsListener(),
		),
		TypeUrl: listenerType,
	})

	// i2 is the same as i1 but has the kubernetes.io/ingress.allow-http: "false" annotation
	i2 := &networking_v1.Ingress{
		ObjectMeta: fixture.ObjectMetaWithAnnotations("default/simple", map[string]string{
			"kubernetes.io/ingress.allow-http": "false",
		}),
		Spec: networking_v1.IngressSpec{
			DefaultBackend: featuretests.IngressBackend(svc1),
		},
	}

	// update i1 to i2 and verify that ingress_http has gone.
	rh.OnUpdate(i1, i2)
	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			statsListener(),
		),
		TypeUrl: listenerType,
	})

	// i3 is similar to i2, but uses the ingress.kubernetes.io/force-ssl-redirect: "true" annotation
	// to force 80 -> 443 upgrade
	i3 := &networking_v1.Ingress{
		ObjectMeta: fixture.ObjectMetaWithAnnotations("default/simple",
			map[string]string{"ingress.kubernetes.io/force-ssl-redirect": "true"}),
		Spec: networking_v1.IngressSpec{
			DefaultBackend: featuretests.IngressBackend(svc1),
		},
	}

	// update i2 to i3 and check that ingress_http has returned
	rh.OnUpdate(i2, i3)
	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			defaultHTTPListener(),
			statsListener(),
		),
		TypeUrl: listenerType,
	})
}

func TestAdminPortListener(t *testing.T) {
	_, c, done := setup(t, customAdminPort(t, 9001))
	defer done()

	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoyAdminListener(9001),
			statsListener(),
		),
		TypeUrl: listenerType,
	})
}

func TestTLSListener(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	s1 := featuretests.TLSSecret(t, "secret", &featuretests.ServerCertificate)

	svc1 := fixture.NewService("backend").
		WithPorts(core_v1.ServicePort{Name: "http", Port: 80})

	// i1 is a tls ingress
	i1 := &networking_v1.Ingress{
		ObjectMeta: fixture.ObjectMeta("simple"),
		Spec: networking_v1.IngressSpec{
			TLS: []networking_v1.IngressTLS{{
				Hosts:      []string{"kuard.example.com"},
				SecretName: "secret",
			}},
			Rules: []networking_v1.IngressRule{{
				Host: "kuard.example.com",
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Backend: *featuretests.IngressBackend(svc1),
						}},
					},
				},
			}},
		},
	}

	rh.OnAdd(svc1)

	// add secret
	rh.OnAdd(s1)

	// assert that there is only a static listener
	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			statsListener(),
		),
		TypeUrl: listenerType,
	})

	// add ingress and assert the existence of ingress_http and ingres_https
	rh.OnAdd(i1)
	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			defaultHTTPListener(),
			&envoy_config_listener_v3.Listener{
				Name:    "ingress_https",
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				FilterChains: []*envoy_config_listener_v3.FilterChain{
					filterchaintls("kuard.example.com", s1,
						httpsFilterFor("kuard.example.com"),
						nil, "h2", "http/1.1"),
				},
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			},
			statsListener(),
		),
		TypeUrl: listenerType,
	})

	// i2 is the same as i1 but has the kubernetes.io/ingress.allow-http: "false" annotation
	i2 := &networking_v1.Ingress{
		ObjectMeta: fixture.ObjectMetaWithAnnotations("simple", map[string]string{
			"kubernetes.io/ingress.allow-http": "false",
		}),
		Spec: networking_v1.IngressSpec{
			TLS: []networking_v1.IngressTLS{{
				Hosts:      []string{"kuard.example.com"},
				SecretName: "secret",
			}},
			Rules: []networking_v1.IngressRule{{
				Host: "kuard.example.com",
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Backend: *featuretests.IngressBackend(svc1),
						}},
					},
				},
			}},
		},
	}

	// update i1 to i2 and verify that ingress_http has gone.
	rh.OnUpdate(i1, i2)
	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			&envoy_config_listener_v3.Listener{
				Name:    "ingress_https",
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				FilterChains: []*envoy_config_listener_v3.FilterChain{
					filterchaintls("kuard.example.com", s1,
						httpsFilterFor("kuard.example.com"),
						nil, "h2", "http/1.1"),
				},
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			},
			statsListener(),
		),
		TypeUrl: listenerType,
	})

	// delete secret and assert that ingress_https is removed
	rh.OnDelete(s1)
	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			statsListener(),
		),
		TypeUrl: listenerType,
	})
}

func TestHTTPProxyTLSListener(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	envoyGen := envoy_v3.NewEnvoyGen(envoy_v3.EnvoyGenOpt{
		XDSClusterName: envoy_v3.DefaultXDSClusterName,
	})
	secret1 := featuretests.TLSSecret(t, "secret", &featuretests.ServerCertificate)

	svc1 := fixture.NewService("backend").
		WithPorts(core_v1.ServicePort{Name: "http", Port: 80})

	// p1 is a tls httpproxy
	p1 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "simple",
			Namespace: secret1.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "kuard.example.com",
				TLS: &contour_v1.TLS{
					SecretName:             secret1.Name,
					MinimumProtocolVersion: "1.2",
					MaximumProtocolVersion: "1.3",
				},
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: svc1.Name,
					Port: int(svc1.Spec.Ports[0].Port),
				}},
			}},
		},
	}

	// p2 is a tls httpproxy
	p2 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "simple",
			Namespace: secret1.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "kuard.example.com",
				TLS: &contour_v1.TLS{
					SecretName:             secret1.Name,
					MinimumProtocolVersion: "1.3",
					MaximumProtocolVersion: "1.3",
				},
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: svc1.Name,
					Port: int(svc1.Spec.Ports[0].Port),
				}},
			}},
		},
	}

	// add secret
	rh.OnAdd(secret1)

	// assert that there is only a static listener
	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			statsListener(),
		),
		TypeUrl: listenerType,
	})

	l1 := &envoy_config_listener_v3.Listener{
		Name:    "ingress_https",
		Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
		ListenerFilters: envoy_v3.ListenerFilters(
			envoy_v3.TLSInspector(),
		),
		FilterChains: []*envoy_config_listener_v3.FilterChain{
			filterchaintls("kuard.example.com", secret1,
				httpsFilterFor("kuard.example.com"),
				nil, "h2", "http/1.1"),
		},
		SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
	}

	// add service
	rh.OnAdd(svc1)

	// add ingress and assert the existence of ingress_http and ingres_https
	rh.OnAdd(p1)

	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			defaultHTTPListener(),
			l1,
			statsListener(),
		),
		TypeUrl: listenerType,
	})

	// delete secret and assert both listeners are removed because the
	// httpproxy is no longer valid.
	rh.OnDelete(secret1)
	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			statsListener(),
		),
		TypeUrl: listenerType,
	})

	rh.OnDelete(p1)
	// add secret
	rh.OnAdd(secret1)
	l2 := &envoy_config_listener_v3.Listener{
		Name:    "ingress_https",
		Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
		ListenerFilters: envoy_v3.ListenerFilters(
			envoy_v3.TLSInspector(),
		),
		FilterChains: []*envoy_config_listener_v3.FilterChain{
			envoy_v3.FilterChainTLS(
				"kuard.example.com",
				envoyGen.DownstreamTLSContext(
					&dag.Secret{Object: secret1},
					envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3,
					envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3,
					nil,
					nil,
					"h2", "http/1.1"),
				envoy_v3.Filters(httpsFilterFor("kuard.example.com")),
			),
		},
		SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
	}

	// add ingress and assert the existence of ingress_http and ingres_https
	rh.OnAdd(p2)
	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			defaultHTTPListener(),
			l2,
			statsListener(),
		),
		TypeUrl: listenerType,
	})
}

func TestTLSListenerCipherSuites(t *testing.T) {
	rh, c, done := setup(t, func(conf *xdscache_v3.ListenerConfig) {
		conf.CipherSuites = []string{"ECDHE-ECDSA-AES256-GCM-SHA384"}
	})
	defer done()

	envoyGen := envoy_v3.NewEnvoyGen(envoy_v3.EnvoyGenOpt{
		XDSClusterName: envoy_v3.DefaultXDSClusterName,
	})
	secret1 := featuretests.TLSSecret(t, "secret", &featuretests.ServerCertificate)

	svc1 := fixture.NewService("backend").
		WithPorts(core_v1.ServicePort{Name: "http", Port: 80})

	// p1 is a tls httpproxy
	p1 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "simple",
			Namespace: secret1.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "kuard.example.com",
				TLS: &contour_v1.TLS{
					SecretName:             secret1.Name,
					MinimumProtocolVersion: "1.2",
					MaximumProtocolVersion: "1.2",
				},
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: svc1.Name,
					Port: int(svc1.Spec.Ports[0].Port),
				}},
			}},
		},
	}

	// add secret
	rh.OnAdd(secret1)

	l1 := &envoy_config_listener_v3.Listener{
		Name:    "ingress_https",
		Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
		ListenerFilters: envoy_v3.ListenerFilters(
			envoy_v3.TLSInspector(),
		),
		FilterChains: []*envoy_config_listener_v3.FilterChain{
			envoy_v3.FilterChainTLS(
				"kuard.example.com",
				envoyGen.DownstreamTLSContext(
					&dag.Secret{Object: secret1},
					envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2,
					envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2,
					[]string{"ECDHE-ECDSA-AES256-GCM-SHA384"},
					nil,
					"h2", "http/1.1"),
				envoy_v3.Filters(httpsFilterFor("kuard.example.com")),
			),
		},
		SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
	}

	// add service
	rh.OnAdd(svc1)

	rh.OnAdd(p1)

	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			defaultHTTPListener(),
			l1,
			statsListener(),
		),
		TypeUrl: listenerType,
	})
}

func TestLDSFilter(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	s1 := featuretests.TLSSecret(t, "secret", &featuretests.ServerCertificate)

	svc1 := fixture.NewService("backend").
		WithPorts(core_v1.ServicePort{Name: "http", Port: 80})

	// i1 is a tls ingress
	i1 := &networking_v1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: networking_v1.IngressSpec{
			TLS: []networking_v1.IngressTLS{{
				Hosts:      []string{"kuard.example.com"},
				SecretName: "secret",
			}},
			Rules: []networking_v1.IngressRule{{
				Host: "kuard.example.com",
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Backend: *featuretests.IngressBackend(svc1),
						}},
					},
				},
			}},
		},
	}

	rh.OnAdd(svc1)

	// add secret
	rh.OnAdd(s1)

	// add ingress and fetch ingress_https
	rh.OnAdd(i1)
	c.Request(listenerType, "ingress_https").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			&envoy_config_listener_v3.Listener{
				Name:    "ingress_https",
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				FilterChains: []*envoy_config_listener_v3.FilterChain{
					filterchaintls("kuard.example.com", s1,
						httpsFilterFor("kuard.example.com"),
						nil, "h2", "http/1.1"),
				},
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			},
		),
		TypeUrl: listenerType,
	})

	// fetch ingress_http
	c.Request(listenerType, "ingress_http").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			defaultHTTPListener(),
		),
		TypeUrl: listenerType,
	})

	// fetch something non existent.
	c.Request(listenerType, "HTTP").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: listenerType,
	})
}

func TestLDSStreamEmpty(t *testing.T) {
	_, c, done := setup(t)
	defer done()

	// assert that streaming LDS with no ingresses does not stall.
	c.Request(listenerType, "ingress_http").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		VersionInfo: "0",
		TypeUrl:     listenerType,
		Nonce:       "0",
	})
}

func TestLDSIngressHTTPUseProxyProtocol(t *testing.T) {
	rh, c, done := setup(t, func(conf *xdscache_v3.ListenerConfig) {
		conf.UseProxyProto = true
	})
	defer done()

	// assert that without any ingress objects registered
	// there is only a static listener
	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		VersionInfo: "0",
		Resources: resources(t,
			statsListener(),
		),
		TypeUrl: listenerType,
		Nonce:   "0",
	})

	s1 := fixture.NewService("backend").
		WithPorts(core_v1.ServicePort{Name: "http", Port: 80})

	// i1 is a simple ingress, no hostname, no tls.
	i1 := &networking_v1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: networking_v1.IngressSpec{
			DefaultBackend: featuretests.IngressBackend(s1),
		},
	}

	rh.OnAdd(s1)

	rh.OnAdd(i1)

	// assert that we now have a ingress_http listener using
	// the proxy protocol
	httpListener := defaultHTTPListener()
	httpListener.ListenerFilters = envoy_v3.ListenerFilters(envoy_v3.ProxyProtocol())

	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		VersionInfo: "1",
		Resources: resources(t,
			httpListener,
			statsListener(),
		),
		TypeUrl: listenerType,
		Nonce:   "1",
	})
}

func TestLDSIngressHTTPSUseProxyProtocol(t *testing.T) {
	rh, c, done := setup(t, func(conf *xdscache_v3.ListenerConfig) {
		conf.UseProxyProto = true
	})
	defer done()

	s1 := featuretests.TLSSecret(t, "secret", &featuretests.ServerCertificate)

	svc1 := fixture.NewService("backend").
		WithPorts(core_v1.ServicePort{Name: "http", Port: 80})

	// i1 is a tls ingress
	i1 := &networking_v1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: networking_v1.IngressSpec{
			TLS: []networking_v1.IngressTLS{{
				Hosts:      []string{"kuard.example.com"},
				SecretName: "secret",
			}},
			Rules: []networking_v1.IngressRule{{
				Host: "kuard.example.com",
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Backend: *featuretests.IngressBackend(svc1),
						}},
					},
				},
			}},
		},
	}

	// add secret
	rh.OnAdd(s1)

	// assert that there is only a static listener
	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			statsListener(),
		),
		TypeUrl: listenerType,
	})

	rh.OnAdd(svc1)

	rh.OnAdd(i1)

	// assert the existence of ingress_http and ingres_https and both
	// are using proxy protocol
	httpListener := defaultHTTPListener()
	httpListener.ListenerFilters = envoy_v3.ListenerFilters(envoy_v3.ProxyProtocol())

	httpsListener := &envoy_config_listener_v3.Listener{
		Name:    "ingress_https",
		Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
		ListenerFilters: envoy_v3.ListenerFilters(
			envoy_v3.ProxyProtocol(),
			envoy_v3.TLSInspector(),
		),
		FilterChains: []*envoy_config_listener_v3.FilterChain{
			filterchaintls("kuard.example.com", s1,
				httpsFilterFor("kuard.example.com"),
				nil, "h2", "http/1.1"),
		},
		SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
	}
	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			httpListener,
			httpsListener,
			statsListener(),
		),
		TypeUrl: listenerType,
	})
}

func TestLDSCustomAddressAndPort(t *testing.T) {
	rh, c, done := setup(t, func(builder *dag.Builder) {
		for _, processor := range builder.Processors {
			if listenerProcessor, ok := processor.(*dag.ListenerProcessor); ok {
				listenerProcessor.HTTPAddress = "127.0.0.100"
				listenerProcessor.HTTPPort = 9100

				listenerProcessor.HTTPSAddress = "127.0.0.200"
				listenerProcessor.HTTPSPort = 9200
			}
		}
	})
	defer done()

	s1 := featuretests.TLSSecret(t, "secret", &featuretests.ServerCertificate)

	svc1 := fixture.NewService("backend").
		WithPorts(core_v1.ServicePort{Name: "http", Port: 80})

	// i1 is a tls ingress
	i1 := &networking_v1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: networking_v1.IngressSpec{
			TLS: []networking_v1.IngressTLS{{
				Hosts:      []string{"kuard.example.com"},
				SecretName: "secret",
			}},
			Rules: []networking_v1.IngressRule{{
				Host: "kuard.example.com",
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Backend: *featuretests.IngressBackend(svc1),
						}},
					},
				},
			}},
		},
	}

	// add secret
	rh.OnAdd(s1)

	// assert that there is only a static listener
	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		VersionInfo: "0",
		Resources: resources(t,
			statsListener(),
		),
		TypeUrl: listenerType,
		Nonce:   "0",
	})

	rh.OnAdd(svc1)

	// add ingress and assert the existence of ingress_http and ingres_https
	// using custom address and port
	rh.OnAdd(i1)

	httpListener := defaultHTTPListener()
	httpListener.Address = envoy_v3.SocketAddress("127.0.0.100", 9100)

	httpsListener := &envoy_config_listener_v3.Listener{
		Name:    "ingress_https",
		Address: envoy_v3.SocketAddress("127.0.0.200", 9200),
		ListenerFilters: envoy_v3.ListenerFilters(
			envoy_v3.TLSInspector(),
		),
		FilterChains: []*envoy_config_listener_v3.FilterChain{
			filterchaintls("kuard.example.com", s1,
				httpsFilterFor("kuard.example.com"),
				nil, "h2", "http/1.1"),
		},
		SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
	}
	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			httpListener,
			httpsListener,
			statsListener(),
		),
		TypeUrl: listenerType,
	})
}

func TestLDSCustomAccessLogPaths(t *testing.T) {
	rh, c, done := setup(t, func(conf *xdscache_v3.ListenerConfig) {
		conf.HTTPAccessLog = "/tmp/http_access.log"
		conf.HTTPSAccessLog = "/tmp/https_access.log"
	})
	defer done()

	envoyGen := envoy_v3.NewEnvoyGen(envoy_v3.EnvoyGenOpt{
		XDSClusterName: envoy_v3.DefaultXDSClusterName,
	})
	s1 := featuretests.TLSSecret(t, "secret", &featuretests.ServerCertificate)

	svc1 := fixture.NewService("backend").
		WithPorts(core_v1.ServicePort{Name: "http", Port: 80})
	rh.OnAdd(svc1)

	// i1 is a tls ingress
	i1 := &networking_v1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: networking_v1.IngressSpec{
			TLS: []networking_v1.IngressTLS{{
				Hosts:      []string{"kuard.example.com"},
				SecretName: "secret",
			}},
			Rules: []networking_v1.IngressRule{{
				Host: "kuard.example.com",
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Backend: *featuretests.IngressBackend(svc1),
						}},
					},
				},
			}},
		},
	}

	// add secret
	rh.OnAdd(s1)

	// assert that there is only a static listener
	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		VersionInfo: "0",
		Resources: resources(t,
			statsListener(),
		),
		TypeUrl: listenerType,
		Nonce:   "0",
	})

	rh.OnAdd(i1)

	httpListener := defaultHTTPListener()
	httpListener.FilterChains = envoy_v3.FilterChains(
		envoyGen.HTTPConnectionManager("ingress_http", envoy_v3.FileAccessLogEnvoy("/tmp/http_access.log", "", nil, contour_v1alpha1.LogLevelInfo), 0),
	)

	httpsListener := &envoy_config_listener_v3.Listener{
		Name:    "ingress_https",
		Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
		ListenerFilters: envoy_v3.ListenerFilters(
			envoy_v3.TLSInspector(),
		),
		FilterChains: []*envoy_config_listener_v3.FilterChain{
			filterchaintls("kuard.example.com", s1,
				envoyGen.HTTPConnectionManagerBuilder().
					AddFilter(envoy_v3.FilterMisdirectedRequests("kuard.example.com")).
					DefaultFilters().
					RouteConfigName("https/kuard.example.com").
					MetricsPrefix(xdscache_v3.ENVOY_HTTPS_LISTENER).
					AccessLoggers(envoy_v3.FileAccessLogEnvoy("/tmp/https_access.log", "", nil, contour_v1alpha1.LogLevelInfo)).
					Get(),
				nil, "h2", "http/1.1"),
		},
		SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
	}
	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		VersionInfo: "1",
		Resources: resources(t,
			httpListener,
			httpsListener,
			statsListener(),
		),
		TypeUrl: listenerType,
		Nonce:   "1",
	})
}

func TestHTTPProxyHTTPS(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	// assert that there is only a static listener
	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		VersionInfo: "0",
		Resources: resources(t,
			statsListener(),
		),
		TypeUrl: listenerType,
		Nonce:   "0",
	})

	s1 := featuretests.TLSSecret(t, "secret", &featuretests.ServerCertificate)

	// p1 is a httpproxy that has TLS
	p1 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_v1.TLS{
					SecretName: "secret",
				},
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	svc1 := fixture.NewService("kuard").
		WithPorts(core_v1.ServicePort{Name: "http", Port: 8080})

	// add secret
	rh.OnAdd(s1)

	// add service
	rh.OnAdd(svc1)

	// add httpproxy
	rh.OnAdd(p1)

	ingressHTTPS := &envoy_config_listener_v3.Listener{
		Name:    "ingress_https",
		Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
		ListenerFilters: envoy_v3.ListenerFilters(
			envoy_v3.TLSInspector(),
		),
		FilterChains: []*envoy_config_listener_v3.FilterChain{
			filterchaintls("example.com", s1,
				httpsFilterFor("example.com"),
				nil, "h2", "http/1.1"),
		},
		SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
	}
	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		VersionInfo: "1",
		Resources: resources(t,
			defaultHTTPListener(),
			ingressHTTPS,
			statsListener(),
		),
		TypeUrl: listenerType,
		Nonce:   "1",
	})
}

func TestHTTPProxyTLSVersion(t *testing.T) {
	rh, c, done := setup(t, func(conf *xdscache_v3.ListenerConfig) {
		conf.MinimumTLSVersion = "1.2"
		conf.MaximumTLSVersion = "1.3"
	})

	defer done()
	envoyGen := envoy_v3.NewEnvoyGen(envoy_v3.EnvoyGenOpt{
		XDSClusterName: envoy_v3.DefaultXDSClusterName,
	})
	secret1 := featuretests.TLSSecret(t, "secret", &featuretests.ServerCertificate)
	rh.OnAdd(secret1)

	rh.OnAdd(fixture.NewService("backend").
		WithPorts(core_v1.ServicePort{Name: "http", Port: 80}))

	// p1 is a tls httpproxy
	p1 := &contour_v1.HTTPProxy{
		ObjectMeta: fixture.ObjectMeta("simple"),
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "kuard.example.com",
				TLS: &contour_v1.TLS{
					SecretName:             "secret",
					MinimumProtocolVersion: "1.2",
					MaximumProtocolVersion: "1.3",
				},
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: "backend",
					Port: 80,
				}},
			}},
		},
	}
	rh.OnAdd(p1)

	l1 := &envoy_config_listener_v3.Listener{
		Name:    "ingress_https",
		Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
		ListenerFilters: envoy_v3.ListenerFilters(
			envoy_v3.TLSInspector(),
		),
		FilterChains: []*envoy_config_listener_v3.FilterChain{
			envoy_v3.FilterChainTLS(
				"kuard.example.com",
				envoyGen.DownstreamTLSContext(
					&dag.Secret{Object: secret1},
					envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2,
					envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3,
					nil,
					nil,
					"h2", "http/1.1"),
				envoy_v3.Filters(httpsFilterFor("kuard.example.com")),
			),
		},
		SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
	}

	// verify that p1's TLS 1.1 minimum has been upgraded to 1.2
	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			defaultHTTPListener(),
			l1,
			statsListener(),
		),
		TypeUrl: listenerType,
	})

	// p2 is a tls httpproxy
	p2 := &contour_v1.HTTPProxy{
		ObjectMeta: fixture.ObjectMeta("simple"),
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "kuard.example.com",
				TLS: &contour_v1.TLS{
					SecretName:             "secret",
					MinimumProtocolVersion: "1.3",
					MaximumProtocolVersion: "1.3",
				},
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},

				Services: []contour_v1.Service{{
					Name: "backend",
					Port: 80,
				}},
			}},
		},
	}
	rh.OnUpdate(p1, p2)

	l2 := &envoy_config_listener_v3.Listener{
		Name:    "ingress_https",
		Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
		ListenerFilters: envoy_v3.ListenerFilters(
			envoy_v3.TLSInspector(),
		),
		FilterChains: []*envoy_config_listener_v3.FilterChain{
			envoy_v3.FilterChainTLS(
				"kuard.example.com",
				envoyGen.DownstreamTLSContext(
					&dag.Secret{Object: secret1},
					envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3,
					envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3,
					nil,
					nil,
					"h2", "http/1.1"),
				envoy_v3.Filters(httpsFilterFor("kuard.example.com")),
			),
		},
		SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
	}

	// verify that p2's TLS 1.3 minimum has NOT been downgraded to 1.2
	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			defaultHTTPListener(),
			l2,
			statsListener(),
		),
		TypeUrl: listenerType,
	})
}

func TestLDSHTTPProxyRootCannotDelegateToAnotherRoot(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	rh.OnAdd(fixture.NewService("marketing/green").
		WithPorts(core_v1.ServicePort{Name: "http", Port: 80}))

	child := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "blog",
			Namespace: "marketing",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "www.containersteve.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: "green",
					Port: 80,
				}},
			}},
		},
	}
	rh.OnAdd(child)

	root := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "root-blog",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "blog.containersteve.com",
			},
			Includes: []contour_v1.Include{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Name:      child.Name,
				Namespace: child.Namespace,
			}},
		},
	}
	rh.OnAdd(root)

	// verify that port 80 is present because while it is not possible to
	// delegate to it, child can host a vhost which opens port 80.
	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			defaultHTTPListener(),
			statsListener(),
		),
		TypeUrl: listenerType,
	})
}

func TestHTTPProxyXffNumTrustedHops(t *testing.T) {
	rh, c, done := setup(t, func(conf *xdscache_v3.ListenerConfig) {
		conf.XffNumTrustedHops = 1
	})

	defer done()
	envoyGen := envoy_v3.NewEnvoyGen(envoy_v3.EnvoyGenOpt{
		XDSClusterName: envoy_v3.DefaultXDSClusterName,
	})

	rh.OnAdd(fixture.NewService("backend").
		WithPorts(core_v1.ServicePort{Name: "http", Port: 80}))

	// p1 is a httpproxy
	p1 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "kuard.example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: "backend",
					Port: 80,
				}},
			}},
		},
	}
	rh.OnAdd(p1)

	// verify that the xff-num-trusted-hops have been set to 1.
	httpListener := defaultHTTPListener()

	httpListener.FilterChains = envoy_v3.FilterChains(envoyGen.HTTPConnectionManagerBuilder().
		RouteConfigName("ingress_http").
		MetricsPrefix("ingress_http").
		AccessLoggers(envoy_v3.FileAccessLogEnvoy("/dev/stdout", "", nil, contour_v1alpha1.LogLevelInfo)).
		RequestTimeout(timeout.DurationSetting(0)).
		NumTrustedHops(1).
		DefaultFilters().
		Get())

	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			httpListener,
			statsListener(),
		),
		TypeUrl: listenerType,
	})
}

func TestHTTPProxyStripTrailingHostDot(t *testing.T) {
	rh, c, done := setup(t, func(conf *xdscache_v3.ListenerConfig) {
		conf.StripTrailingHostDot = true
	})

	defer done()
	envoyGen := envoy_v3.NewEnvoyGen(envoy_v3.EnvoyGenOpt{
		XDSClusterName: envoy_v3.DefaultXDSClusterName,
	})

	rh.OnAdd(fixture.NewService("backend").
		WithPorts(core_v1.ServicePort{Name: "http", Port: 80}))

	// p1 is a httpproxy
	p1 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "kuard.example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: "backend",
					Port: 80,
				}},
			}},
		},
	}
	rh.OnAdd(p1)

	// verify that the xff-num-trusted-hops have been set to 1.
	httpListener := defaultHTTPListener()

	httpListener.FilterChains = envoy_v3.FilterChains(envoyGen.HTTPConnectionManagerBuilder().
		RouteConfigName("ingress_http").
		MetricsPrefix("ingress_http").
		AccessLoggers(envoy_v3.FileAccessLogEnvoy("/dev/stdout", "", nil, contour_v1alpha1.LogLevelInfo)).
		RequestTimeout(timeout.DurationSetting(0)).
		StripTrailingHostDot(true).
		DefaultFilters().
		Get())

	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			httpListener,
			statsListener(),
		),
		TypeUrl: listenerType,
	})
}

func TestHTTPProxyServerHeaderTransformation(t *testing.T) {
	rh, c, done := setup(t, func(conf *xdscache_v3.ListenerConfig) {
		conf.ServerHeaderTransformation = contour_v1alpha1.AppendIfAbsentServerHeader
	})

	defer done()
	envoyGen := envoy_v3.NewEnvoyGen(envoy_v3.EnvoyGenOpt{
		XDSClusterName: envoy_v3.DefaultXDSClusterName,
	})

	rh.OnAdd(fixture.NewService("backend").
		WithPorts(core_v1.ServicePort{Name: "http", Port: 80}))

	// p1 is a httpproxy
	p1 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "kuard.example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: "backend",
					Port: 80,
				}},
			}},
		},
	}
	rh.OnAdd(p1)

	// verify that the server-header-transformation has been set to append_if_absent.
	httpListener := defaultHTTPListener()

	httpListener.FilterChains = envoy_v3.FilterChains(envoyGen.HTTPConnectionManagerBuilder().
		RouteConfigName("ingress_http").
		MetricsPrefix("ingress_http").
		AccessLoggers(envoy_v3.FileAccessLogEnvoy("/dev/stdout", "", nil, contour_v1alpha1.LogLevelInfo)).
		RequestTimeout(timeout.DurationSetting(0)).
		ServerHeaderTransformation(contour_v1alpha1.AppendIfAbsentServerHeader).
		DefaultFilters().
		Get())

	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			httpListener,
			statsListener(),
		),
		TypeUrl: listenerType,
	})
}

func TestGatewayListenersSetAddress(t *testing.T) {
	rh, c, done := setup(t, func(builder *dag.Builder) {
		for _, processor := range builder.Processors {
			if listenerProcessor, ok := processor.(*dag.ListenerProcessor); ok {
				listenerProcessor.HTTPAddress = "127.0.0.100"
				listenerProcessor.HTTPSAddress = "127.0.0.200"
			}
		}
	})
	defer done()

	rh.OnAdd(fixture.NewService("svc1").
		WithPorts(core_v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)}),
	)
	tlssecret := featuretests.TLSSecret(t, "projectcontour/tlscert", &featuretests.ServerCertificate)
	rh.OnAdd(tlssecret)

	rh.OnAdd(gc)

	rh.OnAdd(&gatewayapi_v1.Gateway{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "contour",
			Namespace: "projectcontour",
		},
		Spec: gatewayapi_v1.GatewaySpec{
			GatewayClassName: gatewayapi_v1.ObjectName(gc.Name),
			Listeners: []gatewayapi_v1.Listener{
				{
					Name:     "http",
					Port:     80,
					Protocol: gatewayapi_v1.HTTPProtocolType,
					AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
						Namespaces: &gatewayapi_v1.RouteNamespaces{
							From: ptr.To(gatewayapi_v1.NamespacesFromAll),
						},
					},
				},
				{
					Name:     "https",
					Port:     443,
					Protocol: gatewayapi_v1.HTTPSProtocolType,
					TLS: &gatewayapi_v1.GatewayTLSConfig{
						Mode: ptr.To(gatewayapi_v1.TLSModeTerminate),
						CertificateRefs: []gatewayapi_v1.SecretObjectReference{
							gatewayapi.CertificateRef("tlscert", ""),
						},
					},
					AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
						Namespaces: &gatewayapi_v1.RouteNamespaces{
							From: ptr.To(gatewayapi_v1.NamespacesFromAll),
						},
					},
				},
				{
					Name:     "tls",
					Port:     8443,
					Protocol: gatewayapi_v1.TLSProtocolType,
					TLS: &gatewayapi_v1.GatewayTLSConfig{
						Mode: ptr.To(gatewayapi_v1.TLSModePassthrough),
					},
					AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
						Namespaces: &gatewayapi_v1.RouteNamespaces{
							From: ptr.To(gatewayapi_v1.NamespacesFromAll),
						},
					},
				},
				{
					Name:     "tcp",
					Port:     27017,
					Protocol: gatewayapi_v1.TCPProtocolType,
					AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
						Namespaces: &gatewayapi_v1.RouteNamespaces{
							From: ptr.To(gatewayapi_v1.NamespacesFromAll),
						},
					},
				},
			},
		},
	})

	rh.OnAdd(&gatewayapi_v1.HTTPRoute{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "basic",
			Namespace: "default",
		},
		Spec: gatewayapi_v1.HTTPRouteSpec{
			CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
				ParentRefs: []gatewayapi_v1.ParentReference{
					gatewayapi.GatewayListenerParentRef("projectcontour", "contour", "http", 0),
					gatewayapi.GatewayListenerParentRef("projectcontour", "contour", "https", 0),
				},
			},
			Hostnames: []gatewayapi_v1.Hostname{
				"test.projectcontour.io",
			},
			Rules: []gatewayapi_v1.HTTPRouteRule{{
				Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
				BackendRefs: gatewayapi.HTTPBackendRef("svc1", 80, 10),
			}},
		},
	})

	rh.OnAdd(&gatewayapi_v1alpha2.TLSRoute{
		ObjectMeta: fixture.ObjectMeta("basic"),
		Spec: gatewayapi_v1alpha2.TLSRouteSpec{
			CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
				ParentRefs: []gatewayapi_v1.ParentReference{
					gatewayapi.GatewayListenerParentRef("projectcontour", "contour", "tls", 0),
				},
			},
			Hostnames: []gatewayapi_v1.Hostname{"tcp.projectcontour.io"},
			Rules: []gatewayapi_v1alpha2.TLSRouteRule{{
				BackendRefs: gatewayapi.TLSRouteBackendRef("svc1", 80, nil),
			}},
		},
	})

	rh.OnAdd(&gatewayapi_v1alpha2.TCPRoute{
		ObjectMeta: fixture.ObjectMeta("tcproute-1"),
		Spec: gatewayapi_v1alpha2.TCPRouteSpec{
			CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
				ParentRefs: []gatewayapi_v1.ParentReference{
					gatewayapi.GatewayListenerParentRef("projectcontour", "contour", "tcp", 0),
				},
			},
			Rules: []gatewayapi_v1alpha2.TCPRouteRule{{
				BackendRefs: gatewayapi.TLSRouteBackendRef("svc1", 80, nil),
			}},
		},
	})

	// Address should come from listener HTTP address.
	c.Request(listenerType, "http-80").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: listenerType,
		Resources: resources(t,
			&envoy_config_listener_v3.Listener{
				Name:          "http-80",
				Address:       envoy_v3.SocketAddress("127.0.0.100", 8080),
				FilterChains:  envoy_v3.FilterChains(httpFilterForGateway()),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			},
		),
	})

	// Address should come from listener HTTPS address.
	c.Request(listenerType, "https-443").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: listenerType,
		Resources: resources(t,
			&envoy_config_listener_v3.Listener{
				Name:    "https-443",
				Address: envoy_v3.SocketAddress("127.0.0.200", 8443),
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				FilterChains: appendFilterChains(
					filterchaintls("test.projectcontour.io", tlssecret,
						httpsFilterForGateway("https-443", "test.projectcontour.io"),
						nil, "h2", "http/1.1"),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			},
		),
	})

	// Address should come from listener HTTPS address.
	c.Request(listenerType, "https-8443").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: listenerType,
		Resources: resources(t,
			&envoy_config_listener_v3.Listener{
				Name:    "https-8443",
				Address: envoy_v3.SocketAddress("127.0.0.200", 16443),
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				FilterChains: []*envoy_config_listener_v3.FilterChain{{
					FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
						ServerNames: []string{"tcp.projectcontour.io"},
					},
					Filters: envoy_v3.Filters(
						tcpproxy("https-8443", "default/svc1/80/da39a3ee5e"),
					),
				}},
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			},
		),
	})

	// Address should come from listener HTTP address.
	c.Request(listenerType, "tcp-27017").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			&envoy_config_listener_v3.Listener{
				Name:    "tcp-27017",
				Address: envoy_v3.SocketAddress("127.0.0.100", 35017),
				FilterChains: []*envoy_config_listener_v3.FilterChain{{
					Filters: envoy_v3.Filters(
						tcpproxy("tcp-27017", "default/svc1/80/da39a3ee5e"),
					),
				}},
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			},
		),
		TypeUrl: listenerType,
	})
}

func TestSocketOptions(t *testing.T) {
	// Configure non-default socket options for HTTP and HTTPS listeners.
	rh, c, done := setup(t, func(conf *xdscache_v3.ListenerConfig) {
		conf.SocketOptions = &contour_v1alpha1.SocketOptions{
			TOS:          32,
			TrafficClass: 64,
		}
	})
	defer done()

	envoyGen := envoy_v3.NewEnvoyGen(envoy_v3.EnvoyGenOpt{
		XDSClusterName: envoy_v3.DefaultXDSClusterName,
	})
	svc1 := fixture.NewService("backend").
		WithPorts(core_v1.ServicePort{Name: "http", Port: 80})
	rh.OnAdd(svc1)

	secret1 := featuretests.TLSSecret(t, "secret", &featuretests.ServerCertificate)
	rh.OnAdd(secret1)

	// p1 is a tls httpproxy
	p1 := &contour_v1.HTTPProxy{
		ObjectMeta: fixture.ObjectMeta("simple"),
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "kuard.example.com",
				TLS: &contour_v1.TLS{
					SecretName: "secret",
				},
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: "backend",
					Port: 80,
				}},
			}},
		},
	}
	rh.OnAdd(p1)

	socketOpts := envoy_v3.NewSocketOptions().TCPKeepalive().Build()

	// Verify that the given socket options are set on the HTTP and HTTPS listeners.
	socketOpts = append(socketOpts, &envoy_config_core_v3.SocketOption{
		Description: "Set IPv4 TOS field",
		Level:       envoy.IPPROTO_IP,
		Name:        envoy.IP_TOS,
		State:       envoy_config_core_v3.SocketOption_STATE_LISTENING,
		Value:       &envoy_config_core_v3.SocketOption_IntValue{IntValue: int64(32)},
	}, &envoy_config_core_v3.SocketOption{
		Description: "Set IPv6 Traffic Class field",
		Level:       envoy.IPPROTO_IPV6,
		Name:        envoy.IPV6_TCLASS,
		State:       envoy_config_core_v3.SocketOption_STATE_LISTENING,
		Value:       &envoy_config_core_v3.SocketOption_IntValue{IntValue: int64(64)},
	})

	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			&envoy_config_listener_v3.Listener{
				Name:    "ingress_http",
				Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v3.FilterChains(
					envoyGen.HTTPConnectionManager("ingress_http", envoy_v3.FileAccessLogEnvoy("/dev/stdout", "", nil, contour_v1alpha1.LogLevelInfo), 0),
				),
				SocketOptions: socketOpts,
			},
			&envoy_config_listener_v3.Listener{
				Name:    "ingress_https",
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				FilterChains: []*envoy_config_listener_v3.FilterChain{
					envoy_v3.FilterChainTLS(
						"kuard.example.com",
						envoyGen.DownstreamTLSContext(
							&dag.Secret{Object: secret1},
							envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2,
							envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3,
							nil,
							nil,
							"h2", "http/1.1"),
						envoy_v3.Filters(httpsFilterFor("kuard.example.com")),
					),
				},
				SocketOptions: socketOpts,
			},
			statsListener(),
		),
		TypeUrl: listenerType,
	})
}
