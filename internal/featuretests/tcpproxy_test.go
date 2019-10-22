// Copyright © 2019 VMware
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
	ingressroutev1 "github.com/projectcontour/contour/apis/contour/v1beta1"
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

	i1 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: s1.Namespace,
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "kuard-tcp.example.com",
				TLS: &projcontour.TLS{
					SecretName: s1.Name,
				},
			},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Services: []ingressroutev1.Service{{
					Name: "wrong-backend",
					Port: 80,
				}},
			}},
			TCPProxy: &ingressroutev1.TCPProxy{
				Services: []ingressroutev1.Service{{
					Name: svc.Name,
					Port: 80,
				}},
			},
		},
	}
	rh.OnAdd(s1)
	rh.OnAdd(svc)
	rh.OnAdd(i1)

	c.Request(listenerType, "ingress_https").Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			&v2.Listener{
				Name:         "ingress_https",
				Address:      envoy.SocketAddress("0.0.0.0", 8443),
				FilterChains: filterchaintls("kuard-tcp.example.com", s1, tcpproxy(t, "ingress_https", "default/correct-backend/80/da39a3ee5e")),
				ListenerFilters: envoy.ListenerFilters(
					envoy.TLSInspector(),
				),
			}),
		TypeUrl: listenerType,
	})

	rh.OnDelete(i1)

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
				Conditions: prefixCondition("/"),
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

	c.Request(listenerType, "ingress_https").Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			&v2.Listener{
				Name:         "ingress_https",
				Address:      envoy.SocketAddress("0.0.0.0", 8443),
				FilterChains: filterchaintls("kuard-tcp.example.com", s1, tcpproxy(t, "ingress_https", "default/correct-backend/80/da39a3ee5e")),
				ListenerFilters: envoy.ListenerFilters(
					envoy.TLSInspector(),
				),
			}),
		TypeUrl: listenerType,
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

	i1 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "child",
			Namespace: svc.Namespace,
		},
		Spec: ingressroutev1.IngressRouteSpec{
			TCPProxy: &ingressroutev1.TCPProxy{
				Services: []ingressroutev1.Service{{
					Name: svc.Name,
					Port: 80,
				}},
			},
		},
	}
	i2 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "parent",
			Namespace: s1.Namespace,
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "kuard-tcp.example.com",
				TLS: &projcontour.TLS{
					SecretName: s1.Name,
				},
			},
			TCPProxy: &ingressroutev1.TCPProxy{
				Delegate: &ingressroutev1.Delegate{
					Name:      i1.Name,
					Namespace: i1.Namespace,
				},
			},
		},
	}
	rh.OnAdd(s1)
	rh.OnAdd(svc)
	rh.OnAdd(i1)
	rh.OnAdd(i2)

	c.Request(listenerType, "ingress_https").Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			&v2.Listener{
				Name:         "ingress_https",
				Address:      envoy.SocketAddress("0.0.0.0", 8443),
				FilterChains: filterchaintls("kuard-tcp.example.com", s1, tcpproxy(t, "ingress_https", "app/backend/80/da39a3ee5e")),
				ListenerFilters: envoy.ListenerFilters(
					envoy.TLSInspector(),
				),
			}),
		TypeUrl: listenerType,
	})

	rh.OnDelete(i1)

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

	c.Request(listenerType, "ingress_https").Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			&v2.Listener{
				Name:         "ingress_https",
				Address:      envoy.SocketAddress("0.0.0.0", 8443),
				FilterChains: filterchaintls("kuard-tcp.example.com", s1, tcpproxy(t, "ingress_https", "app/backend/80/da39a3ee5e")),
				ListenerFilters: envoy.ListenerFilters(
					envoy.TLSInspector(),
				),
			}),
		TypeUrl: listenerType,
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

	i1 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: svc.Namespace,
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "kuard-tcp.example.com",
				TLS: &projcontour.TLS{
					Passthrough: true,
				},
			},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Services: []ingressroutev1.Service{{
					Name: "wrong-backend",
					Port: 80,
				}},
			}},
			TCPProxy: &ingressroutev1.TCPProxy{
				Services: []ingressroutev1.Service{{
					Name: svc.Name,
					Port: 80,
				}},
			},
		},
	}
	rh.OnAdd(svc)
	rh.OnAdd(i1)

	c.Request(listenerType, "ingress_https").Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			&v2.Listener{
				Name:    "ingress_https",
				Address: envoy.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_api_v2_listener.FilterChain{{
					Filters: envoy.Filters(
						tcpproxy(t, "ingress_https", "default/correct-backend/80/da39a3ee5e"),
					),
					FilterChainMatch: &envoy_api_v2_listener.FilterChainMatch{
						ServerNames: []string{"kuard-tcp.example.com"},
					},
				}},
				ListenerFilters: envoy.ListenerFilters(
					envoy.TLSInspector(),
				),
			},
		),
		TypeUrl: listenerType,
	})

	rh.OnDelete(i1)

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
				Conditions: prefixCondition("/"),
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

	c.Request(listenerType, "ingress_https").Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			&v2.Listener{
				Name:    "ingress_https",
				Address: envoy.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_api_v2_listener.FilterChain{{
					Filters: envoy.Filters(
						tcpproxy(t, "ingress_https", "default/correct-backend/80/da39a3ee5e"),
					),
					FilterChainMatch: &envoy_api_v2_listener.FilterChainMatch{
						ServerNames: []string{"kuard-tcp.example.com"},
					},
				}},
				ListenerFilters: envoy.ListenerFilters(
					envoy.TLSInspector(),
				),
			},
		),
		TypeUrl: listenerType,
	})
}
