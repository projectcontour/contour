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

	envoy_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/featuretests"
	"github.com/projectcontour/contour/internal/fixture"
	v1 "k8s.io/api/core/v1"
	networking_v1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestTLSCertificateDelegation(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	// assert that there is only a static listener
	c.Request(listenerType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			statsListener(),
		),
		TypeUrl: listenerType,
	})

	sec1 := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "wildcard",
			Namespace: "secret",
		},
		Type: "kubernetes.io/tls",
		Data: featuretests.Secretdata(featuretests.CERTIFICATE, featuretests.RSA_PRIVATE_KEY),
	}

	// add a secret object secret/wildcard.
	rh.OnAdd(sec1)

	s1 := fixture.NewService("kuard").
		WithPorts(v1.ServicePort{Port: 8080})
	rh.OnAdd(s1)

	// add an httpproxy in a different namespace mentioning secret/wildcard.
	p1 := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: s1.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_api_v1.TLS{
					SecretName: sec1.Namespace + "/" + sec1.Name,
				},
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_api_v1.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			}},
		},
	}
	rh.OnAdd(p1)

	// assert there are no listeners
	c.Request(listenerType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			statsListener(),
		),
		TypeUrl: listenerType,
	})

	// t1 is a TLSCertificateDelegation that permits default to access secret/wildcard
	t1 := &contour_api_v1.TLSCertificateDelegation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "delegation",
			Namespace: sec1.Namespace,
		},
		Spec: contour_api_v1.TLSCertificateDelegationSpec{
			Delegations: []contour_api_v1.CertificateDelegation{{
				SecretName: sec1.Name,
				TargetNamespaces: []string{
					s1.Namespace,
				},
			}},
		},
	}
	rh.OnAdd(t1)

	ingressHTTPS := &envoy_listener_v3.Listener{
		Name:    "ingress_https",
		Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
		ListenerFilters: envoy_v3.ListenerFilters(
			envoy_v3.TLSInspector(),
		),
		FilterChains: appendFilterChains(
			filterchaintls("example.com", sec1,
				httpsFilterFor("example.com"),
				nil, "h2", "http/1.1"),
		),
		SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
	}

	c.Request(listenerType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			defaultHTTPListener(),
			ingressHTTPS,
			statsListener(),
		),
		TypeUrl: listenerType,
	})

	// t2 is a TLSCertificateDelegation that permits access to secret/wildcard from all namespaces.
	t2 := &contour_api_v1.TLSCertificateDelegation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "delegation",
			Namespace: sec1.Namespace,
		},
		Spec: contour_api_v1.TLSCertificateDelegationSpec{
			Delegations: []contour_api_v1.CertificateDelegation{{
				SecretName: sec1.Name,
				TargetNamespaces: []string{
					"*",
				},
			}},
		},
	}
	rh.OnUpdate(t1, t2)

	c.Request(listenerType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			defaultHTTPListener(),
			ingressHTTPS,
			statsListener(),
		),
		TypeUrl: listenerType,
	})

	// t3 is a TLSCertificateDelegation that permits access to secret/different all namespaces.
	t3 := &contour_api_v1.TLSCertificateDelegation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "delegation",
			Namespace: sec1.Namespace,
		},
		Spec: contour_api_v1.TLSCertificateDelegationSpec{
			Delegations: []contour_api_v1.CertificateDelegation{{
				SecretName: "different",
				TargetNamespaces: []string{
					"*",
				},
			}},
		},
	}
	rh.OnUpdate(t2, t3)

	c.Request(listenerType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			statsListener(),
		),
		TypeUrl: listenerType,
	})

	// t4 is a TLSCertificateDelegation that permits access to secret/wildcard from the kube-secret namespace.
	t4 := &contour_api_v1.TLSCertificateDelegation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "delegation",
			Namespace: sec1.Namespace,
		},
		Spec: contour_api_v1.TLSCertificateDelegationSpec{
			Delegations: []contour_api_v1.CertificateDelegation{{
				SecretName: sec1.Name,
				TargetNamespaces: []string{
					"kube-secret",
				},
			}},
		},
	}
	rh.OnUpdate(t3, t4)

	c.Request(listenerType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			statsListener(),
		),
		TypeUrl: listenerType,
	})

	rh.OnDelete(p1)
	rh.OnDelete(t4)

	// add a httpproxy in a different namespace mentioning secret/wildcard.
	hp1 := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: s1.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_api_v1.TLS{
					SecretName: sec1.Namespace + "/" + sec1.Name,
				},
			},
			Routes: []contour_api_v1.Route{{
				Conditions: matchconditions(prefixMatchCondition("/")),
				Services: []contour_api_v1.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			}},
		},
	}
	rh.OnAdd(hp1)

	// assert there are no listeners
	c.Request(listenerType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			statsListener(),
		),
		TypeUrl: listenerType,
	})

	t5 := &contour_api_v1.TLSCertificateDelegation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "delegation",
			Namespace: sec1.Namespace,
		},
		Spec: contour_api_v1.TLSCertificateDelegationSpec{
			Delegations: []contour_api_v1.CertificateDelegation{{
				SecretName: sec1.Name,
				TargetNamespaces: []string{
					s1.Namespace,
				},
			}},
		},
	}
	rh.OnAdd(t5)

	c.Request(listenerType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			defaultHTTPListener(),
			ingressHTTPS,
			statsListener(),
		),
		TypeUrl: listenerType,
	})

	rh.OnDelete(hp1)

	c.Request(listenerType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			statsListener(),
		),
		TypeUrl: listenerType,
	})

	// add an ingress in a different namespace mentioning secret wildcard from namespace secret via annotation.
	i1 := &networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: s1.Namespace,
			Annotations: map[string]string{
				"projectcontour.io/tls-cert-namespace": sec1.Namespace,
			},
		},
		Spec: networking_v1.IngressSpec{
			TLS: []networking_v1.IngressTLS{{
				Hosts:      []string{"example.com"},
				SecretName: sec1.Name,
			}},
			Rules: []networking_v1.IngressRule{{
				Host: "example.com",
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Backend: networking_v1.IngressBackend{
								Service: &networking_v1.IngressServiceBackend{
									Name: s1.Name,
									Port: networking_v1.ServiceBackendPort{
										Number: 8080,
									},
								},
							},
						}},
					},
				},
			}},
		},
	}
	rh.OnAdd(i1)

	c.Request(listenerType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			defaultHTTPListener(),
			ingressHTTPS,
			statsListener(),
		),
		TypeUrl: listenerType,
	})
}
