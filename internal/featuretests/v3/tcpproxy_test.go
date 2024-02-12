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
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/featuretests"
	"github.com/projectcontour/contour/internal/fixture"
)

func TestTCPProxy(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	s1 := featuretests.TLSSecret(t, "secret", &featuretests.ServerCertificate)

	svc := fixture.NewService("correct-backend").
		WithPorts(core_v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)})

	rh.OnAdd(s1)
	rh.OnAdd(svc)

	hp1 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "simple",
			Namespace: s1.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "kuard-tcp.example.com",
				TLS: &contour_v1.TLS{
					SecretName: s1.Name,
				},
			},
			// TODO(tsaarni)
			// According to HTTPProxy documentation, routes should not be processed if HTTPProxy is in tcpproxy mode.
			// Consider removing routes from this test case, and create separate tests for tcpproxies with routes.
			// See also https://github.com/projectcontour/contour/issues/3800
			Routes: []contour_v1.Route{{
				Conditions: matchconditions(prefixMatchCondition("/")),
				Services: []contour_v1.Service{{
					Name: "wrong-backend",
					Port: 80,
				}},
			}},
			TCPProxy: &contour_v1.TCPProxy{
				Services: []contour_v1.Service{{
					Name: svc.Name,
					Port: 80,
				}},
			},
		},
	}
	rh.OnAdd(hp1)
	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			// TODO(tsaarni)
			// Reference to non-existing backend ("wrong-backend" above) does not anymore prevent processing of routes, since
			// we would generally program 502/503 responses for missing services.
			// Currently in tcpproxy mode, this will trigger creation of HTTP listener for HTTPS upgrade redirect.
			// However, the reason for HTTP listener should have been HTTPS upgrade redirect for tcpproxy, not routes,
			// See also https://github.com/projectcontour/contour/issues/3800
			defaultHTTPListener(),
			&envoy_config_listener_v3.Listener{
				Name:    "ingress_https",
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: appendFilterChains(
					filterchaintls("kuard-tcp.example.com", s1, tcpproxy("ingress_https", "default/correct-backend/80/da39a3ee5e"), nil),
				),
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			},
			statsListener(),
		),
		TypeUrl: listenerType,
	})

	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("kuard-tcp.example.com",
					&envoy_config_route_v3.Route{
						Match: routePrefix("/"),
						Action: &envoy_config_route_v3.Route_Redirect{
							Redirect: &envoy_config_route_v3.RedirectAction{
								SchemeRewriteSpecifier: &envoy_config_route_v3.RedirectAction_HttpsRedirect{
									HttpsRedirect: true,
								},
							},
						},
					},
				),
			),
		),
		TypeUrl: routeType,
	})
}

func TestTCPProxyDelegation(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	s1 := featuretests.TLSSecret(t, "secret", &featuretests.ServerCertificate)

	svc := fixture.NewService("app/backend").
		WithPorts(core_v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)})

	rh.OnAdd(s1)
	rh.OnAdd(svc)

	hp1 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "simple",
			Namespace: svc.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			TCPProxy: &contour_v1.TCPProxy{
				Services: []contour_v1.Service{{
					Name: svc.Name,
					Port: 80,
				}},
			},
		},
	}
	hp2 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "parent",
			Namespace: s1.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "kuard-tcp.example.com",
				TLS: &contour_v1.TLS{
					SecretName: s1.Name,
				},
			},
			TCPProxy: &contour_v1.TCPProxy{
				Include: &contour_v1.TCPProxyInclude{
					Name:      hp1.Name,
					Namespace: hp1.Namespace,
				},
			},
		},
	}

	rh.OnAdd(hp1)
	rh.OnAdd(hp2)

	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			&envoy_config_listener_v3.Listener{
				Name:    "ingress_https",
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: appendFilterChains(
					filterchaintls("kuard-tcp.example.com", s1, tcpproxy("ingress_https", "app/backend/80/da39a3ee5e"), nil),
				),
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			},
			statsListener(),
		),
		TypeUrl: listenerType,
	})

	// check that ingress_http is empty
	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http"),
		),
		TypeUrl: routeType,
	})
}

// Assert that when a spec.vhost.tls spec is present with tls.passthrough
// set to true we configure envoy to forward the TLS session to the cluster
// after using SNI to determine the target.
func TestTCPProxyTLSPassthrough(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	svc := fixture.NewService("correct-backend").
		WithPorts(core_v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)})

	rh.OnAdd(svc)

	hp1 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "simple",
			Namespace: svc.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "kuard-tcp.example.com",
				TLS: &contour_v1.TLS{
					Passthrough: true,
				},
			},
			// TODO(tsaarni)
			// According to HTTPProxy documentation, routes should not be processed if HTTPProxy is in tcpproxy mode.
			// Consider removing routes from this test case, and create separate tests for tcpproxies with routes.
			// See also https://github.com/projectcontour/contour/issues/3800
			Routes: []contour_v1.Route{{
				Conditions: matchconditions(prefixMatchCondition("/")),
				Services: []contour_v1.Service{{
					Name: "wrong-backend",
					Port: 80,
				}},
			}},
			TCPProxy: &contour_v1.TCPProxy{
				Services: []contour_v1.Service{{
					Name: svc.Name,
					Port: 80,
				}},
			},
		},
	}
	rh.OnAdd(hp1)

	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			// TODO(tsaarni)
			// Reference to non-existing backend ("wrong-backend" above) does not anymore prevent processing of routes, since
			// we would generally program 502/503 responses for missing services.
			// Currently in tcpproxy mode, this will trigger creation of HTTP listener for HTTPS upgrade redirect.
			// However, the reason for HTTP listener should have been HTTPS upgrade redirect for tcpproxy, not routes,
			// See also https://github.com/projectcontour/contour/issues/3800
			defaultHTTPListener(),
			&envoy_config_listener_v3.Listener{
				Name:    "ingress_https",
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_config_listener_v3.FilterChain{{
					Filters: envoy_v3.Filters(
						tcpproxy("ingress_https", "default/correct-backend/80/da39a3ee5e"),
					),
					FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
						ServerNames: []string{"kuard-tcp.example.com"},
					},
				}},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			},
			statsListener(),
		),
		TypeUrl: listenerType,
	})

	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("kuard-tcp.example.com",
					&envoy_config_route_v3.Route{
						Match: routePrefix("/"),
						Action: &envoy_config_route_v3.Route_Redirect{
							Redirect: &envoy_config_route_v3.RedirectAction{
								SchemeRewriteSpecifier: &envoy_config_route_v3.RedirectAction_HttpsRedirect{
									HttpsRedirect: true,
								},
							},
						},
					},
				),
			),
		),
		TypeUrl: routeType,
	})
}

// issue 1916. Assert that tcp proxying to backends using
// projectcontour.io/upstream-protocol.tls configure envoy
// to use TLS between envoy and the backend pod.
func TestTCPProxyTLSBackend(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	s1 := featuretests.TLSSecret(t, "k8s-tls", &featuretests.ServerCertificate)

	svc := fixture.NewService("kubernetes").
		Annotate("projectcontour.io/upstream-protocol.tls", "https,443").
		WithPorts(core_v1.ServicePort{Name: "https", Port: 443, TargetPort: intstr.FromInt(6443)})

	hp1 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kubernetesb",
			Namespace: s1.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "k8s.run.ubisoft.org",
				TLS: &contour_v1.TLS{
					SecretName: s1.Name,
				},
			},
			TCPProxy: &contour_v1.TCPProxy{
				Services: []contour_v1.Service{{
					Name: svc.Name,
					Port: 443,
				}},
			},
		},
	}

	rh.OnAdd(s1)
	rh.OnAdd(svc)
	rh.OnAdd(hp1)

	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			&envoy_config_listener_v3.Listener{
				Name:    "ingress_https",
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: appendFilterChains(
					filterchaintls("k8s.run.ubisoft.org", s1,
						tcpproxy("ingress_https", svc.Namespace+"/"+svc.Name+"/443/4929fca9d4"), nil),
				),
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			},
			statsListener(),
		),
		TypeUrl: listenerType,
	})
	c.Request(clusterType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			tlsCluster(cluster(
				svc.Namespace+"/"+svc.Name+"/443/4929fca9d4",
				svc.Namespace+"/"+svc.Name+"/https",
				svc.Namespace+"_"+svc.Name+"_443",
			), nil, "", "", nil, nil),
		),
		TypeUrl: clusterType,
	})

	// check that ingress_http is empty
	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http"),
		),
		TypeUrl: routeType,
	})
}

// Assert that TCPProxy + a http service can be used to expose a ingress_http
// route on the same vhost that port ingress_https is tls passthrough + proxying.
func TestTCPProxyAndHTTPService(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	s1 := featuretests.TLSSecret(t, "secret", &featuretests.ServerCertificate)

	svc := fixture.NewService("backend").
		WithPorts(core_v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)})

	hp1 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "simple",
			Namespace: svc.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "kuard-tcp.example.com",
				TLS: &contour_v1.TLS{
					SecretName: s1.Name,
				},
			},
			Routes: []contour_v1.Route{{
				Conditions: matchconditions(prefixMatchCondition("/")),
				Services: []contour_v1.Service{{
					Name: svc.Name,
					Port: 80,
				}},
			}},
			TCPProxy: &contour_v1.TCPProxy{
				Services: []contour_v1.Service{{
					Name: svc.Name,
					Port: 80,
				}},
			},
		},
	}
	rh.OnAdd(s1)
	rh.OnAdd(svc)
	rh.OnAdd(hp1)

	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			// ingress_http is present for
			// http://kuard-tcp.example.com/ -> default/backend:80
			defaultHTTPListener(),

			// ingress_https is present for
			// kuard-tcp.example.com:443 terminated at envoy then forwarded to default/backend:80
			&envoy_config_listener_v3.Listener{
				Name:    "ingress_https",
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: appendFilterChains(
					filterchaintls("kuard-tcp.example.com", s1, tcpproxy("ingress_https", "default/backend/80/da39a3ee5e"), nil),
				),
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			},
			statsListener(),
		),
		TypeUrl: listenerType,
	})

	// check that routes exist on port 80 (ingress_http) only.
	// There should be an unconditional 301 HTTPS upgrade for http://kuard-tcp.example.com/.
	// ingress_https should be empty, no route should be present as kuard-tcp.example.com:443
	// is in tcpproxy mode.
	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("kuard-tcp.example.com",
					upgradeHTTPS(routePrefix("/")),
				),
			),
		),
		TypeUrl: routeType,
	})
}

// Assert that TCPProxy + a http service can be used to expose a ingress_http
// route on the same vhost without 301 upgrade,
func TestTCPProxyAndHTTPServicePermitInsecure(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	s1 := featuretests.TLSSecret(t, "secret", &featuretests.ServerCertificate)

	svc := fixture.NewService("backend").
		WithPorts(core_v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)})

	hp1 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "simple",
			Namespace: svc.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "kuard-tcp.example.com",
				TLS: &contour_v1.TLS{
					SecretName: s1.Name,
				},
			},
			Routes: []contour_v1.Route{{
				Conditions:     matchconditions(prefixMatchCondition("/")),
				PermitInsecure: true,
				Services: []contour_v1.Service{{
					Name: svc.Name,
					Port: 80,
				}},
			}},
			TCPProxy: &contour_v1.TCPProxy{
				Services: []contour_v1.Service{{
					Name: svc.Name,
					Port: 80,
				}},
			},
		},
	}
	rh.OnAdd(s1)
	rh.OnAdd(svc)
	rh.OnAdd(hp1)

	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			// ingress_http is present for
			// http://kuard-tcp.example.com/ -> default/backend:80
			defaultHTTPListener(),

			// ingress_https is present for
			// kuard-tcp.example.com:443 terminated at envoy then tcpproxied to default/backend:80
			&envoy_config_listener_v3.Listener{
				Name:    "ingress_https",
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: appendFilterChains(
					filterchaintls("kuard-tcp.example.com", s1, tcpproxy("ingress_https", "default/backend/80/da39a3ee5e"), nil),
				),
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			},
			statsListener(),
		),
		TypeUrl: listenerType,
	})

	// check that routes exist on port 80 (ingress_http) only.
	// ingress_https should be empty, no route should be present as kuard-tcp.example.com:443
	// is in tcpproxy mode.
	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("kuard-tcp.example.com",
					&envoy_config_route_v3.Route{
						Match: routePrefix("/"),
						// this is a regular route cluster, not a 301 upgrade as
						// permitInsecure: true was set.
						Action: routeCluster("default/backend/80/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
	})
}

// Assert that TCPProxy + TLSPassthrough and a HTTP service can be used to expose a ingress_http
// route on the same vhost that port ingress_https is tls passthrough + proxying.
func TestTCPProxyTLSPassthroughAndHTTPService(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	svc := fixture.NewService("backend").
		WithPorts(core_v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)})

	hp1 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "simple",
			Namespace: svc.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "kuard-tcp.example.com",
				TLS: &contour_v1.TLS{
					Passthrough: true,
				},
			},
			Routes: []contour_v1.Route{{
				Conditions: matchconditions(prefixMatchCondition("/")),
				Services: []contour_v1.Service{{
					Name: svc.Name,
					Port: 80,
				}},
			}},
			TCPProxy: &contour_v1.TCPProxy{
				Services: []contour_v1.Service{{
					Name: svc.Name,
					Port: 80,
				}},
			},
		},
	}
	rh.OnAdd(svc)
	rh.OnAdd(hp1)

	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			// ingress_http is present for
			// http://kuard-tcp.example.com/ -> default/backend:80
			defaultHTTPListener(),

			// ingress_https is present for
			// kuard-tcp.example.com:443 direct to default/backend:80
			&envoy_config_listener_v3.Listener{
				Name:    "ingress_https",
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_config_listener_v3.FilterChain{{
					Filters: envoy_v3.Filters(
						tcpproxy("ingress_https", "default/backend/80/da39a3ee5e"),
					),
					FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
						ServerNames: []string{"kuard-tcp.example.com"},
					},
				}},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			},
			statsListener(),
		),
		TypeUrl: listenerType,
	})

	// check port 80 is open and the route is a 301 upgrade.
	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				// 301 upgrade because permitInsecure is false, thus
				// the route is present on port 80, but unconditionally
				// upgrades to HTTPS.
				envoy_v3.VirtualHost("kuard-tcp.example.com",
					upgradeHTTPS(routePrefix("/")),
				),
			),
			// ingress_https should be empty.
		),
		TypeUrl: routeType,
	})
}

// Assert that TCPProxy + TLSPassthrough and a HTTP service using permitInsecure can be used
// to expose a ingress_http route on the same vhost that port ingress_https is tls
// passthrough + proxying.
func TestTCPProxyTLSPassthroughAndHTTPServicePermitInsecure(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	svc := fixture.NewService("backend").
		WithPorts(core_v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)})

	hp1 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "simple",
			Namespace: svc.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "kuard-tcp.example.com",
				TLS: &contour_v1.TLS{
					Passthrough: true,
				},
			},
			Routes: []contour_v1.Route{{
				Conditions:     matchconditions(prefixMatchCondition("/")),
				PermitInsecure: true,
				Services: []contour_v1.Service{{
					Name: svc.Name,
					Port: 80,
				}},
			}},
			TCPProxy: &contour_v1.TCPProxy{
				Services: []contour_v1.Service{{
					Name: svc.Name,
					Port: 80,
				}},
			},
		},
	}
	rh.OnAdd(svc)
	rh.OnAdd(hp1)

	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			// ingress_http is present for
			// http://kuard-tcp.example.com/ -> default/backend:80, this is not 301 upgraded
			// because permitInsecure: true is in use.
			defaultHTTPListener(),

			// ingress_https is present for
			// kuard-tcp.example.com:443 direct to default/backend:80, envoy does not handle
			// the TLS handshake beyond SNI demux because passthrough: true is in use.
			&envoy_config_listener_v3.Listener{
				Name:    "ingress_https",
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_config_listener_v3.FilterChain{{
					Filters: envoy_v3.Filters(
						tcpproxy("ingress_https", "default/backend/80/da39a3ee5e"),
					),
					FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
						ServerNames: []string{"kuard-tcp.example.com"},
					},
				}},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			},
			statsListener(),
		),
		TypeUrl: listenerType,
	})

	// check that routes exist on port 80 (ingress_http) only.
	// ingress_https should be empty, no route should be present as kuard-tcp.example.com:443
	// is in tcpproxy mode.
	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("kuard-tcp.example.com",
					&envoy_config_route_v3.Route{
						Match: routePrefix("/"),
						// not a 301 upgrade because permitInsecure: true is in use.
						Action: routeCluster("default/backend/80/da39a3ee5e"),
					},
				),
			),
			// ingress_https should be empty.
		),
		TypeUrl: routeType,
	})
}

// Assert that TCPProxy with a missing tls key, and/or missing passthrough or secretname
// does not generate a tcpproxy configuration.
func TestTCPProxyMissingTLS(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	s1 := featuretests.TLSSecret(t, "secret", &featuretests.ServerCertificate)

	svc := fixture.NewService("backend").
		WithPorts(core_v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)})

	hp1 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "simple",
			Namespace: svc.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "tcpproxy.example.com",
				// missing TLS:
			},
			Routes: []contour_v1.Route{{
				Conditions: matchconditions(prefixMatchCondition("/")),
				Services: []contour_v1.Service{{
					Name: svc.Name,
					Port: 80,
				}},
			}},
			TCPProxy: &contour_v1.TCPProxy{
				Services: []contour_v1.Service{{
					Name: svc.Name,
					Port: 80,
				}},
			},
		},
	}
	rh.OnAdd(s1)
	rh.OnAdd(svc)
	rh.OnAdd(hp1)

	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			// ingress_http and ingress_https should be missing
			// as hp1 is not valid.
			statsListener(),
		),
		TypeUrl: listenerType,
	})

	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			// ingress_http and ingress_https should be empty
			// as hp1 is not valid.
			envoy_v3.RouteConfiguration("ingress_http"),
		),
		TypeUrl: routeType,
	})

	hp2 := &contour_v1.HTTPProxy{
		ObjectMeta: hp1.ObjectMeta,
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "tcpproxy.example.com",
				TLS: &contour_v1.TLS{
					// invalid, one of Passthrough or SecretName must be provided.
					Passthrough: false,
					SecretName:  "",
				},
			},
			Routes: []contour_v1.Route{{
				Conditions: matchconditions(prefixMatchCondition("/")),
				Services: []contour_v1.Service{{
					Name: svc.Name,
					Port: 80,
				}},
			}},
			TCPProxy: &contour_v1.TCPProxy{
				Services: []contour_v1.Service{{
					Name: svc.Name,
					Port: 80,
				}},
			},
		},
	}
	rh.OnUpdate(hp1, hp2)

	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			// ingress_http and ingress_https should be missing
			// as hp2 is not valid.
			statsListener(),
		),
		TypeUrl: listenerType,
	})

	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			// ingress_http and ingress_https should be empty
			// as hp2 is not valid.
			envoy_v3.RouteConfiguration("ingress_http"),
		),
		TypeUrl: routeType,
	})
}

// "Cookie" and "RequestHash" policies are not valid on TCPProxy.
func TestTCPProxyInvalidLoadBalancerPolicy(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	s1 := featuretests.TLSSecret(t, "secret", &featuretests.ServerCertificate)

	svc := fixture.NewService("backend").
		WithPorts(core_v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)})

	rh.OnAdd(s1)
	rh.OnAdd(svc)

	hp1 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "simple",
			Namespace: s1.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "kuard-tcp.example.com",
				TLS: &contour_v1.TLS{
					SecretName: s1.Name,
				},
			},
			TCPProxy: &contour_v1.TCPProxy{
				Services: []contour_v1.Service{{
					Name: svc.Name,
					Port: 80,
				}},
				LoadBalancerPolicy: &contour_v1.LoadBalancerPolicy{
					Strategy: "Cookie",
				},
			},
		},
	}
	rh.OnAdd(hp1)

	// Check that a basic cluster is produced with the default load balancer
	// policy.
	c.Request(clusterType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			cluster(
				svc.Namespace+"/"+svc.Name+"/80/da39a3ee5e",
				svc.Namespace+"/"+svc.Name,
				svc.Namespace+"_"+svc.Name+"_80",
			),
		),
		TypeUrl: clusterType,
	})

	rh.OnUpdate(hp1, &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "simple",
			Namespace: s1.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "kuard-tcp.example.com",
				TLS: &contour_v1.TLS{
					SecretName: s1.Name,
				},
			},
			TCPProxy: &contour_v1.TCPProxy{
				Services: []contour_v1.Service{{
					Name: svc.Name,
					Port: 80,
				}},
				LoadBalancerPolicy: &contour_v1.LoadBalancerPolicy{
					Strategy: "RequestHash",
				},
			},
		},
	})

	// Check that a basic cluster is produced with the default load balancer
	// policy.
	c.Request(clusterType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			cluster(
				svc.Namespace+"/"+svc.Name+"/80/da39a3ee5e",
				svc.Namespace+"/"+svc.Name,
				svc.Namespace+"_"+svc.Name+"_80",
			),
		),
		TypeUrl: clusterType,
	})
}
