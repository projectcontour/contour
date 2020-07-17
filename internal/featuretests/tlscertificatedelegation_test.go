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
	projcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/envoy"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestTLSCertificateDelegation(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	// assert that there is only a static listener
	c.Request(listenerType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			staticListener(),
		),
		TypeUrl: listenerType,
	})

	sec1 := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "wildcard",
			Namespace: "secret",
		},
		Type: "kubernetes.io/tls",
		Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
	}

	// add a secret object secret/wildcard.
	rh.OnAdd(sec1)

	s1 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:     "http",
				Protocol: "TCP",
				Port:     8080,
			}},
		},
	}
	rh.OnAdd(s1)

	// add an httpproxy in a different namespace mentioning secret/wildcard.
	p1 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: s1.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
				TLS: &projcontour.TLS{
					SecretName: sec1.Namespace + "/" + sec1.Name,
				},
			},
			Routes: []projcontour.Route{{
				Conditions: []projcontour.MatchCondition{{
					Prefix: "/",
				}},
				Services: []projcontour.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			}},
		},
	}
	rh.OnAdd(p1)

	// assert there are no listeners
	c.Request(listenerType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			staticListener(),
		),
		TypeUrl: listenerType,
	})

	// t1 is a TLSCertificateDelegation that permits default to access secret/wildcard
	t1 := &projcontour.TLSCertificateDelegation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "delegation",
			Namespace: sec1.Namespace,
		},
		Spec: projcontour.TLSCertificateDelegationSpec{
			Delegations: []projcontour.CertificateDelegation{{
				SecretName: sec1.Name,
				TargetNamespaces: []string{
					s1.Namespace,
				},
			}},
		},
	}
	rh.OnAdd(t1)

	ingress_http := &v2.Listener{
		Name:    "ingress_http",
		Address: envoy.SocketAddress("0.0.0.0", 8080),
		FilterChains: envoy.FilterChains(
			envoy.HTTPConnectionManager("ingress_http", envoy.FileAccessLogEnvoy("/dev/stdout"), 0),
		),
		SocketOptions: envoy.TCPKeepaliveSocketOptions(),
	}

	ingress_https := &v2.Listener{
		Name:    "ingress_https",
		Address: envoy.SocketAddress("0.0.0.0", 8443),
		ListenerFilters: envoy.ListenerFilters(
			envoy.TLSInspector(),
		),
		FilterChains: appendFilterChains(
			filterchaintls("example.com", sec1,
				httpsFilterFor("example.com"),
				nil, "h2", "http/1.1"),
		),
		SocketOptions: envoy.TCPKeepaliveSocketOptions(),
	}

	c.Request(listenerType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			ingress_http,
			ingress_https,
			staticListener(),
		),
		TypeUrl: listenerType,
	})

	// t2 is a TLSCertificateDelegation that permits access to secret/wildcard from all namespaces.
	t2 := &projcontour.TLSCertificateDelegation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "delegation",
			Namespace: sec1.Namespace,
		},
		Spec: projcontour.TLSCertificateDelegationSpec{
			Delegations: []projcontour.CertificateDelegation{{
				SecretName: sec1.Name,
				TargetNamespaces: []string{
					"*",
				},
			}},
		},
	}
	rh.OnUpdate(t1, t2)

	c.Request(listenerType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			ingress_http,
			ingress_https,
			staticListener(),
		),
		TypeUrl: listenerType,
	})

	// t3 is a TLSCertificateDelegation that permits access to secret/different all namespaces.
	t3 := &projcontour.TLSCertificateDelegation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "delegation",
			Namespace: sec1.Namespace,
		},
		Spec: projcontour.TLSCertificateDelegationSpec{
			Delegations: []projcontour.CertificateDelegation{{
				SecretName: "different",
				TargetNamespaces: []string{
					"*",
				},
			}},
		},
	}
	rh.OnUpdate(t2, t3)

	c.Request(listenerType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			staticListener(),
		),
		TypeUrl: listenerType,
	})

	// t4 is a TLSCertificateDelegation that permits access to secret/wildcard from the kube-secret namespace.
	t4 := &projcontour.TLSCertificateDelegation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "delegation",
			Namespace: sec1.Namespace,
		},
		Spec: projcontour.TLSCertificateDelegationSpec{
			Delegations: []projcontour.CertificateDelegation{{
				SecretName: sec1.Name,
				TargetNamespaces: []string{
					"kube-secret",
				},
			}},
		},
	}
	rh.OnUpdate(t3, t4)

	c.Request(listenerType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			staticListener(),
		),
		TypeUrl: listenerType,
	})

	rh.OnDelete(p1)
	rh.OnDelete(t4)

	// add a httpproxy in a different namespace mentioning secret/wildcard.
	hp1 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: s1.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
				TLS: &projcontour.TLS{
					SecretName: sec1.Namespace + "/" + sec1.Name,
				},
			},
			Routes: []projcontour.Route{{
				Conditions: matchconditions(prefixMatchCondition("/")),
				Services: []projcontour.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			}},
		},
	}
	rh.OnAdd(hp1)

	// assert there are no listeners
	c.Request(listenerType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			staticListener(),
		),
		TypeUrl: listenerType,
	})

	t5 := &projcontour.TLSCertificateDelegation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "delegation",
			Namespace: sec1.Namespace,
		},
		Spec: projcontour.TLSCertificateDelegationSpec{
			Delegations: []projcontour.CertificateDelegation{{
				SecretName: sec1.Name,
				TargetNamespaces: []string{
					s1.Namespace,
				},
			}},
		},
	}
	rh.OnAdd(t5)

	c.Request(listenerType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			ingress_http,
			ingress_https,
			staticListener(),
		),
		TypeUrl: listenerType,
	})
}
