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
	envoy_api_v2_listener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	envoy_api_v2_route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	projcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/envoy"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestTCPProxy(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	s1 := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret",
			Namespace: "default",
		},
		Type: "kubernetes.io/tls",
		Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
	}

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "correct-backend",
			Namespace: s1.Namespace,
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}

	rh.OnAdd(s1)
	rh.OnAdd(svc)

	hp1 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: s1.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "kuard-tcp.example.com",
				TLS: &projcontour.TLS{
					SecretName: s1.Name,
				},
			},
			Routes: []projcontour.Route{{
				Conditions: matchconditions(prefixMatchCondition("/")),
				Services: []projcontour.Service{{
					Name: "wrong-backend",
					Port: 80,
				}},
			}},
			TCPProxy: &projcontour.TCPProxy{
				Services: []projcontour.Service{{
					Name: svc.Name,
					Port: 80,
				}},
			},
		},
	}
	rh.OnAdd(hp1)

	c.Request(listenerType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			&v2.Listener{
				Name:    "ingress_https",
				Address: envoy.SocketAddress("0.0.0.0", 8443),
				FilterChains: appendFilterChains(
					filterchaintls("kuard-tcp.example.com", s1, tcpproxy("ingress_https", "default/correct-backend/80/da39a3ee5e"), nil),
				),
				ListenerFilters: envoy.ListenerFilters(
					envoy.TLSInspector(),
				),
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			},
			staticListener(),
		),
		TypeUrl: listenerType,
	})

	// check that ingress_http is empty
	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http"),
		),
		TypeUrl: routeType,
	})
}

func TestTCPProxyDelegation(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	s1 := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret",
			Namespace: "default",
		},
		Type: "kubernetes.io/tls",
		Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
	}

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "backend",
			Namespace: "app",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}
	rh.OnAdd(s1)
	rh.OnAdd(svc)

	hp1 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: svc.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			TCPProxy: &projcontour.TCPProxy{
				Services: []projcontour.Service{{
					Name: svc.Name,
					Port: 80,
				}},
			},
		},
	}
	hp2 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "parent",
			Namespace: s1.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "kuard-tcp.example.com",
				TLS: &projcontour.TLS{
					SecretName: s1.Name,
				},
			},
			TCPProxy: &projcontour.TCPProxy{
				Include: &projcontour.TCPProxyInclude{
					Name:      hp1.Name,
					Namespace: hp1.Namespace,
				},
			},
		},
	}

	rh.OnAdd(hp1)
	rh.OnAdd(hp2)

	c.Request(listenerType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			&v2.Listener{
				Name:    "ingress_https",
				Address: envoy.SocketAddress("0.0.0.0", 8443),
				FilterChains: appendFilterChains(
					filterchaintls("kuard-tcp.example.com", s1, tcpproxy("ingress_https", "app/backend/80/da39a3ee5e"), nil),
				),
				ListenerFilters: envoy.ListenerFilters(
					envoy.TLSInspector(),
				),
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			},
			staticListener(),
		),
		TypeUrl: listenerType,
	})

	// check that ingress_http is empty
	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http"),
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

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "correct-backend",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}

	rh.OnAdd(svc)

	hp1 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: svc.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "kuard-tcp.example.com",
				TLS: &projcontour.TLS{
					Passthrough: true,
				},
			},
			Routes: []projcontour.Route{{
				Conditions: matchconditions(prefixMatchCondition("/")),
				Services: []projcontour.Service{{
					Name: "wrong-backend",
					Port: 80,
				}},
			}},
			TCPProxy: &projcontour.TCPProxy{
				Services: []projcontour.Service{{
					Name: svc.Name,
					Port: 80,
				}},
			},
		},
	}
	rh.OnAdd(hp1)

	c.Request(listenerType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			&v2.Listener{
				Name:    "ingress_https",
				Address: envoy.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_api_v2_listener.FilterChain{{
					Filters: envoy.Filters(
						tcpproxy("ingress_https", "default/correct-backend/80/da39a3ee5e"),
					),
					FilterChainMatch: &envoy_api_v2_listener.FilterChainMatch{
						ServerNames: []string{"kuard-tcp.example.com"},
					},
				}},
				ListenerFilters: envoy.ListenerFilters(
					envoy.TLSInspector(),
				),
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			},
			staticListener(),
		),
		TypeUrl: listenerType,
	})

	// check that ingress_http is empty
	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http"),
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

	s1 := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "k8s-tls",
			Namespace: "default",
		},
		Type: "kubernetes.io/tls",
		Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
	}

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kubernetes",
			Namespace: s1.Namespace,
			Annotations: map[string]string{
				"projectcontour.io/upstream-protocol.tls": "https,443",
			},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "https",
				Protocol:   "TCP",
				Port:       443,
				TargetPort: intstr.FromInt(6443),
			}},
		},
	}

	hp1 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kubernetesb",
			Namespace: s1.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "k8s.run.ubisoft.org",
				TLS: &projcontour.TLS{
					SecretName: s1.Name,
				},
			},
			TCPProxy: &projcontour.TCPProxy{
				Services: []projcontour.Service{{
					Name: svc.Name,
					Port: 443,
				}},
			},
		},
	}

	rh.OnAdd(s1)
	rh.OnAdd(svc)
	rh.OnAdd(hp1)

	c.Request(listenerType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			&v2.Listener{
				Name:    "ingress_https",
				Address: envoy.SocketAddress("0.0.0.0", 8443),
				FilterChains: appendFilterChains(
					filterchaintls("k8s.run.ubisoft.org", s1,
						tcpproxy("ingress_https", svc.Namespace+"/"+svc.Name+"/443/da39a3ee5e"), nil),
				),
				ListenerFilters: envoy.ListenerFilters(
					envoy.TLSInspector(),
				),
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			},
			staticListener(),
		),
		TypeUrl: listenerType,
	})
	c.Request(clusterType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			tlsCluster(cluster(
				svc.Namespace+"/"+svc.Name+"/443/da39a3ee5e",
				svc.Namespace+"/"+svc.Name+"/https",
				svc.Namespace+"_"+svc.Name+"_443",
			), nil, "", ""),
		),
		TypeUrl: clusterType,
	})

	// check that ingress_http is empty
	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http"),
		),
		TypeUrl: routeType,
	})
}

// Assert that TCPProxy + a http service can be used to expose a ingress_http
// route on the same vhost that port ingress_https is tls passthrough + proxying.
func TestTCPProxyAndHTTPService(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	s1 := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret",
			Namespace: "default",
		},
		Type: "kubernetes.io/tls",
		Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
	}

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "backend",
			Namespace: s1.Namespace,
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}

	hp1 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: svc.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "kuard-tcp.example.com",
				TLS: &projcontour.TLS{
					SecretName: s1.Name,
				},
			},
			Routes: []projcontour.Route{{
				Conditions: matchconditions(prefixMatchCondition("/")),
				Services: []projcontour.Service{{
					Name: svc.Name,
					Port: 80,
				}},
			}},
			TCPProxy: &projcontour.TCPProxy{
				Services: []projcontour.Service{{
					Name: svc.Name,
					Port: 80,
				}},
			},
		},
	}
	rh.OnAdd(s1)
	rh.OnAdd(svc)
	rh.OnAdd(hp1)

	c.Request(listenerType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			&v2.Listener{
				// ingress_http is present for
				// http://kuard-tcp.example.com/ -> default/backend:80
				Name:    "ingress_http",
				Address: envoy.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy.FilterChains(
					envoy.HTTPConnectionManager("ingress_http", envoy.FileAccessLogEnvoy("/dev/stdout"), 0),
				),
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			},
			&v2.Listener{
				// ingress_https is present for
				// kuard-tcp.example.com:443 terminated at envoy then forwarded to default/backend:80
				Name:    "ingress_https",
				Address: envoy.SocketAddress("0.0.0.0", 8443),
				FilterChains: appendFilterChains(
					filterchaintls("kuard-tcp.example.com", s1, tcpproxy("ingress_https", "default/backend/80/da39a3ee5e"), nil),
				),
				ListenerFilters: envoy.ListenerFilters(
					envoy.TLSInspector(),
				),
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			},
			staticListener(),
		),
		TypeUrl: listenerType,
	})

	// check that routes exist on port 80 (ingress_http) only.
	// There should be an unconditional 301 HTTPS upgrade for http://kuard-tcp.example.com/.
	// ingress_https should be empty, no route should be present as kuard-tcp.example.com:443
	// is in tcpproxy mode.
	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("kuard-tcp.example.com",
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

	s1 := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret",
			Namespace: "default",
		},
		Type: "kubernetes.io/tls",
		Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
	}

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "backend",
			Namespace: s1.Namespace,
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}

	hp1 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: svc.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "kuard-tcp.example.com",
				TLS: &projcontour.TLS{
					SecretName: s1.Name,
				},
			},
			Routes: []projcontour.Route{{
				Conditions:     matchconditions(prefixMatchCondition("/")),
				PermitInsecure: true,
				Services: []projcontour.Service{{
					Name: svc.Name,
					Port: 80,
				}},
			}},
			TCPProxy: &projcontour.TCPProxy{
				Services: []projcontour.Service{{
					Name: svc.Name,
					Port: 80,
				}},
			},
		},
	}
	rh.OnAdd(s1)
	rh.OnAdd(svc)
	rh.OnAdd(hp1)

	c.Request(listenerType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			&v2.Listener{
				// ingress_http is present for
				// http://kuard-tcp.example.com/ -> default/backend:80
				Name:    "ingress_http",
				Address: envoy.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy.FilterChains(
					envoy.HTTPConnectionManager("ingress_http", envoy.FileAccessLogEnvoy("/dev/stdout"), 0),
				),
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			},
			&v2.Listener{
				// ingress_https is present for
				// kuard-tcp.example.com:443 terminated at envoy then tcpproxied to default/backend:80
				Name:    "ingress_https",
				Address: envoy.SocketAddress("0.0.0.0", 8443),
				FilterChains: appendFilterChains(
					filterchaintls("kuard-tcp.example.com", s1, tcpproxy("ingress_https", "default/backend/80/da39a3ee5e"), nil),
				),
				ListenerFilters: envoy.ListenerFilters(
					envoy.TLSInspector(),
				),
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			},
			staticListener(),
		),
		TypeUrl: listenerType,
	})

	// check that routes exist on port 80 (ingress_http) only.
	// ingress_https should be empty, no route should be present as kuard-tcp.example.com:443
	// is in tcpproxy mode.
	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("kuard-tcp.example.com",
					&envoy_api_v2_route.Route{
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

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "backend",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}

	hp1 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: svc.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "kuard-tcp.example.com",
				TLS: &projcontour.TLS{
					Passthrough: true,
				},
			},
			Routes: []projcontour.Route{{
				Conditions: matchconditions(prefixMatchCondition("/")),
				Services: []projcontour.Service{{
					Name: svc.Name,
					Port: 80,
				}},
			}},
			TCPProxy: &projcontour.TCPProxy{
				Services: []projcontour.Service{{
					Name: svc.Name,
					Port: 80,
				}},
			},
		},
	}
	rh.OnAdd(svc)
	rh.OnAdd(hp1)

	c.Request(listenerType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			&v2.Listener{
				// ingress_http is present for
				// http://kuard-tcp.example.com/ -> default/backend:80
				Name:    "ingress_http",
				Address: envoy.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy.FilterChains(
					envoy.HTTPConnectionManager("ingress_http", envoy.FileAccessLogEnvoy("/dev/stdout"), 0),
				),
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			},
			&v2.Listener{
				// ingress_https is present for
				// kuard-tcp.example.com:443 direct to default/backend:80
				Name:    "ingress_https",
				Address: envoy.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_api_v2_listener.FilterChain{{
					Filters: envoy.Filters(
						tcpproxy("ingress_https", "default/backend/80/da39a3ee5e"),
					),
					FilterChainMatch: &envoy_api_v2_listener.FilterChainMatch{
						ServerNames: []string{"kuard-tcp.example.com"},
					},
				}},
				ListenerFilters: envoy.ListenerFilters(
					envoy.TLSInspector(),
				),
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			},
			staticListener(),
		),
		TypeUrl: listenerType,
	})

	// check port 80 is open and the route is a 301 upgrade.
	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				// 301 upgrade because permitInsecure is false, thus
				// the route is present on port 80, but unconditionally
				// upgrades to HTTPS.
				envoy.VirtualHost("kuard-tcp.example.com",
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

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "backend",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}

	hp1 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: svc.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "kuard-tcp.example.com",
				TLS: &projcontour.TLS{
					Passthrough: true,
				},
			},
			Routes: []projcontour.Route{{
				Conditions:     matchconditions(prefixMatchCondition("/")),
				PermitInsecure: true,
				Services: []projcontour.Service{{
					Name: svc.Name,
					Port: 80,
				}},
			}},
			TCPProxy: &projcontour.TCPProxy{
				Services: []projcontour.Service{{
					Name: svc.Name,
					Port: 80,
				}},
			},
		},
	}
	rh.OnAdd(svc)
	rh.OnAdd(hp1)

	c.Request(listenerType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			&v2.Listener{
				// ingress_http is present for
				// http://kuard-tcp.example.com/ -> default/backend:80, this is not 301 upgraded
				// because permitInsecure: true is in use.
				Name:    "ingress_http",
				Address: envoy.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy.FilterChains(
					envoy.HTTPConnectionManager("ingress_http", envoy.FileAccessLogEnvoy("/dev/stdout"), 0),
				),
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			},
			&v2.Listener{
				// ingress_https is present for
				// kuard-tcp.example.com:443 direct to default/backend:80, envoy does not handle
				// the TLS handshake beyond SNI demux because passthrough: true is in use.
				Name:    "ingress_https",
				Address: envoy.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_api_v2_listener.FilterChain{{
					Filters: envoy.Filters(
						tcpproxy("ingress_https", "default/backend/80/da39a3ee5e"),
					),
					FilterChainMatch: &envoy_api_v2_listener.FilterChainMatch{
						ServerNames: []string{"kuard-tcp.example.com"},
					},
				}},
				ListenerFilters: envoy.ListenerFilters(
					envoy.TLSInspector(),
				),
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			},
			staticListener(),
		),
		TypeUrl: listenerType,
	})

	// check that routes exist on port 80 (ingress_http) only.
	// ingress_https should be empty, no route should be present as kuard-tcp.example.com:443
	// is in tcpproxy mode.
	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("kuard-tcp.example.com",
					&envoy_api_v2_route.Route{
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

	s1 := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret",
			Namespace: "default",
		},
		Type: "kubernetes.io/tls",
		Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
	}

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "backend",
			Namespace: s1.Name,
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}

	hp1 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: svc.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "tcpproxy.example.com",
				// missing TLS:
			},
			Routes: []projcontour.Route{{
				Conditions: matchconditions(prefixMatchCondition("/")),
				Services: []projcontour.Service{{
					Name: svc.Name,
					Port: 80,
				}},
			}},
			TCPProxy: &projcontour.TCPProxy{
				Services: []projcontour.Service{{
					Name: svc.Name,
					Port: 80,
				}},
			},
		},
	}
	rh.OnAdd(s1)
	rh.OnAdd(svc)
	rh.OnAdd(hp1)

	c.Request(listenerType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			// ingress_http and ingress_https should be missing
			// as hp1 is not valid.
			staticListener(),
		),
		TypeUrl: listenerType,
	})

	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			// ingress_http and ingress_https should be empty
			// as hp1 is not valid.
			envoy.RouteConfiguration("ingress_http"),
		),
		TypeUrl: routeType,
	})

	hp2 := &projcontour.HTTPProxy{
		ObjectMeta: hp1.ObjectMeta,
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "tcpproxy.example.com",
				TLS: &projcontour.TLS{
					// invalid, one of Passthrough or SecretName must be provided.
					Passthrough: false,
					SecretName:  "",
				},
			},
			Routes: []projcontour.Route{{
				Conditions: matchconditions(prefixMatchCondition("/")),
				Services: []projcontour.Service{{
					Name: svc.Name,
					Port: 80,
				}},
			}},
			TCPProxy: &projcontour.TCPProxy{
				Services: []projcontour.Service{{
					Name: svc.Name,
					Port: 80,
				}},
			},
		},
	}
	rh.OnUpdate(hp1, hp2)

	c.Request(listenerType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			// ingress_http and ingress_https should be missing
			// as hp2 is not valid.
			staticListener(),
		),
		TypeUrl: listenerType,
	})

	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			// ingress_http and ingress_https should be empty
			// as hp2 is not valid.
			envoy.RouteConfiguration("ingress_http"),
		),
		TypeUrl: routeType,
	})
}
