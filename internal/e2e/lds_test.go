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

package e2e

import (
	"context"
	"testing"

	ingressroutev1 "github.com/projectcontour/contour/apis/contour/v1beta1"
	projcontour "github.com/projectcontour/contour/apis/projectcontour/v1"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_api_v2_auth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	envoy_api_v2_listener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	"github.com/projectcontour/contour/internal/assert"
	"github.com/projectcontour/contour/internal/contour"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/envoy"
	"google.golang.org/grpc"
	v1 "k8s.io/api/core/v1"
	"k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestNonTLSListener(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	// assert that without any ingress objects registered
	// there are no active listeners
	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: resources(t,
			staticListener(),
		),
		TypeUrl: listenerType,
		Nonce:   "0",
	}, streamLDS(t, cc))

	// i1 is a simple ingress, no hostname, no tls.
	i1 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: v1beta1.IngressSpec{
			Backend: backend("backend", intstr.FromInt(80)),
		},
	}

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "backend",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:     "http",
				Protocol: "TCP",
				Port:     80,
			}},
		},
	})

	// add it and assert that we now have a ingress_http listener
	rh.OnAdd(i1)
	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "1",
		Resources: resources(t,
			&v2.Listener{
				Name:         "ingress_http",
				Address:      envoy.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy.FilterChains(envoy.HTTPConnectionManager("ingress_http", envoy.FileAccessLogEnvoy("/dev/stdout"), 0)),
			},
			staticListener(),
		),
		TypeUrl: listenerType,
		Nonce:   "1",
	}, streamLDS(t, cc))

	// i2 is the same as i1 but has the kubernetes.io/ingress.allow-http: "false" annotation
	i2 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
			Annotations: map[string]string{
				"kubernetes.io/ingress.allow-http": "false",
			},
		},
		Spec: v1beta1.IngressSpec{
			Backend: backend("backend", intstr.FromInt(80)),
		},
	}

	// update i1 to i2 and verify that ingress_http has gone.
	rh.OnUpdate(i1, i2)
	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "2",
		Resources: resources(t,
			staticListener(),
		),
		TypeUrl: listenerType,
		Nonce:   "2",
	}, streamLDS(t, cc))

	// i3 is similar to i2, but uses the ingress.kubernetes.io/force-ssl-redirect: "true" annotation
	// to force 80 -> 443 upgrade
	i3 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
			Annotations: map[string]string{
				"ingress.kubernetes.io/force-ssl-redirect": "true",
			},
		},
		Spec: v1beta1.IngressSpec{
			Backend: backend("backend", intstr.FromInt(80)),
		},
	}

	// update i2 to i3 and check that ingress_http has returned
	rh.OnUpdate(i2, i3)
	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "3",
		Resources: resources(t,
			&v2.Listener{
				Name:         "ingress_http",
				Address:      envoy.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy.FilterChains(envoy.HTTPConnectionManager("ingress_http", envoy.FileAccessLogEnvoy("/dev/stdout"), 0)),
			},
			staticListener(),
		),
		TypeUrl: listenerType,
		Nonce:   "3",
	}, streamLDS(t, cc))
}

func TestTLSListener(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	// s1 is a tls secret
	s1 := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret",
			Namespace: "default",
		},
		Type: "kubernetes.io/tls",
		Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
	}

	// i1 is a tls ingress
	i1 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: v1beta1.IngressSpec{
			TLS: []v1beta1.IngressTLS{{
				Hosts:      []string{"kuard.example.com"},
				SecretName: "secret",
			}},
			Rules: []v1beta1.IngressRule{{
				Host: "kuard.example.com",
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Backend: *backend("backend", intstr.FromInt(80)),
						}},
					},
				},
			}},
		},
	}

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "backend",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:     "http",
				Protocol: "TCP",
				Port:     80,
			}},
		},
	})

	// add secret
	rh.OnAdd(s1)

	// assert that there is only a static listener
	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: resources(t,
			staticListener(),
		),
		TypeUrl: listenerType,
		Nonce:   "0",
	}, streamLDS(t, cc))

	// add ingress and assert the existence of ingress_http and ingres_https
	rh.OnAdd(i1)
	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "1",
		Resources: resources(t,
			&v2.Listener{
				Name:         "ingress_http",
				Address:      envoy.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy.FilterChains(envoy.HTTPConnectionManager("ingress_http", envoy.FileAccessLogEnvoy("/dev/stdout"), 0)),
			},
			&v2.Listener{
				Name:    "ingress_https",
				Address: envoy.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: envoy.ListenerFilters(
					envoy.TLSInspector(),
				),
				FilterChains: filterchaintls("kuard.example.com", s1, envoy.HTTPConnectionManager("ingress_https", envoy.FileAccessLogEnvoy("/dev/stdout"), 0), "h2", "http/1.1"),
			},
			staticListener(),
		),
		TypeUrl: listenerType,
		Nonce:   "1",
	}, streamLDS(t, cc))

	// i2 is the same as i1 but has the kubernetes.io/ingress.allow-http: "false" annotation
	i2 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
			Annotations: map[string]string{
				"kubernetes.io/ingress.allow-http": "false",
			},
		},
		Spec: v1beta1.IngressSpec{
			TLS: []v1beta1.IngressTLS{{
				Hosts:      []string{"kuard.example.com"},
				SecretName: "secret",
			}},
			Rules: []v1beta1.IngressRule{{
				Host: "kuard.example.com",
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Backend: *backend("backend", intstr.FromInt(80)),
						}},
					},
				},
			}},
		},
	}

	// update i1 to i2 and verify that ingress_http has gone.
	rh.OnUpdate(i1, i2)
	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "2",
		Resources: resources(t,
			&v2.Listener{
				Name:    "ingress_https",
				Address: envoy.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: envoy.ListenerFilters(
					envoy.TLSInspector(),
				),
				FilterChains: filterchaintls("kuard.example.com", s1, envoy.HTTPConnectionManager("ingress_https", envoy.FileAccessLogEnvoy("/dev/stdout"), 0), "h2", "http/1.1"),
			},
			staticListener(),
		),
		TypeUrl: listenerType,
		Nonce:   "2",
	}, streamLDS(t, cc))

	// delete secret and assert that ingress_https is removed
	rh.OnDelete(s1)
	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "3",
		Resources: resources(t,
			staticListener(),
		),
		TypeUrl: listenerType,
		Nonce:   "3",
	}, streamLDS(t, cc))
}

func TestIngressRouteTLSListener(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	// secret1 is a tls secret
	secret1 := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret",
			Namespace: "default",
		},
		Type: "kubernetes.io/tls",
		Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
	}

	svc1 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "backend",
			Namespace: secret1.Namespace,
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:     "http",
				Protocol: "TCP",
				Port:     80,
			}},
		},
	}

	// i1 is a tls ingressroute
	i1 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: secret1.Namespace,
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "kuard.example.com",
				TLS: &projcontour.TLS{
					SecretName:             secret1.Name,
					MinimumProtocolVersion: "1.1",
				},
			},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Services: []ingressroutev1.Service{{
					Name: svc1.Name,
					Port: int(svc1.Spec.Ports[0].Port),
				}},
			}},
		},
	}

	// i2 is a tls ingressroute
	i2 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: secret1.Namespace,
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "kuard.example.com",
				TLS: &projcontour.TLS{
					SecretName:             secret1.Name,
					MinimumProtocolVersion: "1.3",
				},
			},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Services: []ingressroutev1.Service{{
					Name: svc1.Name,
					Port: int(svc1.Spec.Ports[0].Port),
				}},
			}},
		},
	}

	// add secret
	rh.OnAdd(secret1)

	// assert that there is only a static listener
	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: resources(t,
			staticListener(),
		),
		TypeUrl: listenerType,
		Nonce:   "0",
	}, streamLDS(t, cc))

	l1 := &v2.Listener{
		Name:    "ingress_https",
		Address: envoy.SocketAddress("0.0.0.0", 8443),
		ListenerFilters: envoy.ListenerFilters(
			envoy.TLSInspector(),
		),
		FilterChains: filterchaintls("kuard.example.com", secret1, envoy.HTTPConnectionManager("ingress_https", envoy.FileAccessLogEnvoy("/dev/stdout"), 0), "h2", "http/1.1"),
	}

	// add service
	rh.OnAdd(svc1)

	// add ingress and assert the existence of ingress_http and ingres_https
	rh.OnAdd(i1)

	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "1",
		Resources: resources(t,
			&v2.Listener{
				Name:    "ingress_http",
				Address: envoy.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy.FilterChains(
					envoy.HTTPConnectionManager("ingress_http", envoy.FileAccessLogEnvoy("/dev/stdout"), 0),
				),
			},
			l1,
			staticListener(),
		),
		TypeUrl: listenerType,
		Nonce:   "1",
	}, streamLDS(t, cc))

	// delete secret and assert both listeners are removed because the
	// ingressroute is no longer valid.
	rh.OnDelete(secret1)
	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "2",
		Resources: resources(t,
			staticListener(),
		),
		TypeUrl: listenerType,
		Nonce:   "2",
	}, streamLDS(t, cc))

	rh.OnDelete(i1)
	// add secret
	rh.OnAdd(secret1)
	l2 := &v2.Listener{
		Name:    "ingress_https",
		Address: envoy.SocketAddress("0.0.0.0", 8443),
		ListenerFilters: envoy.ListenerFilters(
			envoy.TLSInspector(),
		),
		FilterChains: []*envoy_api_v2_listener.FilterChain{
			envoy.FilterChainTLS(
				"kuard.example.com",
				&dag.Secret{Object: secret1},
				envoy.Filters(
					envoy.HTTPConnectionManager("ingress_https", envoy.FileAccessLogEnvoy("/dev/stdout"), 0),
				),
				envoy_api_v2_auth.TlsParameters_TLSv1_3,
				"h2", "http/1.1",
			),
		},
	}

	// add ingress and assert the existence of ingress_http and ingres_https
	rh.OnAdd(i2)
	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "4",
		Resources: resources(t,
			&v2.Listener{
				Name:    "ingress_http",
				Address: envoy.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy.FilterChains(
					envoy.HTTPConnectionManager("ingress_http", envoy.FileAccessLogEnvoy("/dev/stdout"), 0),
				),
			},
			l2,
			staticListener(),
		),
		TypeUrl: listenerType,
		Nonce:   "4",
	}, streamLDS(t, cc))
}

func TestLDSFilter(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	// s1 is a tls secret
	s1 := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret",
			Namespace: "default",
		},
		Type: "kubernetes.io/tls",
		Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
	}

	// i1 is a tls ingress
	i1 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: v1beta1.IngressSpec{
			TLS: []v1beta1.IngressTLS{{
				Hosts:      []string{"kuard.example.com"},
				SecretName: "secret",
			}},
			Rules: []v1beta1.IngressRule{{
				Host: "kuard.example.com",
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Backend: *backend("backend", intstr.FromInt(80)),
						}},
					},
				},
			}},
		},
	}

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "backend",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:     "http",
				Protocol: "TCP",
				Port:     80,
			}},
		},
	})

	// add secret
	rh.OnAdd(s1)

	// add ingress and fetch ingress_https
	rh.OnAdd(i1)
	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "1",
		Resources: resources(t,
			&v2.Listener{
				Name:    "ingress_https",
				Address: envoy.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: envoy.ListenerFilters(
					envoy.TLSInspector(),
				),
				FilterChains: filterchaintls("kuard.example.com", s1, envoy.HTTPConnectionManager("ingress_https", envoy.FileAccessLogEnvoy("/dev/stdout"), 0), "h2", "http/1.1"),
			},
		),
		TypeUrl: listenerType,
		Nonce:   "1",
	}, streamLDS(t, cc, "ingress_https"))

	// fetch ingress_http
	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "1",
		Resources: resources(t,
			&v2.Listener{
				Name:    "ingress_http",
				Address: envoy.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy.FilterChains(
					envoy.HTTPConnectionManager("ingress_http", envoy.FileAccessLogEnvoy("/dev/stdout"), 0),
				),
			},
		),
		TypeUrl: listenerType,
		Nonce:   "1",
	}, streamLDS(t, cc, "ingress_http"))

	// fetch something non existent.
	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "1",
		TypeUrl:     listenerType,
		Nonce:       "1",
	}, streamLDS(t, cc, "HTTP"))
}

func TestLDSStreamEmpty(t *testing.T) {
	_, cc, done := setup(t)
	defer done()

	// assert that streaming LDS with no ingresses does not stall.
	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		TypeUrl:     listenerType,
		Nonce:       "0",
	}, streamLDS(t, cc, "HTTP"))
}

func TestLDSIngressHTTPUseProxyProtocol(t *testing.T) {
	rh, cc, done := setup(t, func(reh *contour.EventHandler) {
		reh.CacheHandler.UseProxyProto = true
	})
	defer done()

	// assert that without any ingress objects registered
	// there is only a static listener
	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: resources(t,
			staticListener(),
		),
		TypeUrl: listenerType,
		Nonce:   "0",
	}, streamLDS(t, cc))

	// i1 is a simple ingress, no hostname, no tls.
	i1 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: v1beta1.IngressSpec{
			Backend: backend("backend", intstr.FromInt(80)),
		},
	}
	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "backend",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:     "http",
				Protocol: "TCP",
				Port:     80,
			}},
		},
	})

	// add it and assert that we now have a ingress_http listener using
	// the proxy protocol (the true param to filterchain)
	rh.OnAdd(i1)
	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "1",
		Resources: resources(t,
			&v2.Listener{
				Name:    "ingress_http",
				Address: envoy.SocketAddress("0.0.0.0", 8080),
				ListenerFilters: envoy.ListenerFilters(
					envoy.ProxyProtocol(),
				),
				FilterChains: envoy.FilterChains(envoy.HTTPConnectionManager("ingress_http", envoy.FileAccessLogEnvoy("/dev/stdout"), 0)),
			},
			staticListener(),
		),
		TypeUrl: listenerType,
		Nonce:   "1",
	}, streamLDS(t, cc))
}

func TestLDSIngressHTTPSUseProxyProtocol(t *testing.T) {
	rh, cc, done := setup(t, func(reh *contour.EventHandler) {
		reh.CacheHandler.UseProxyProto = true
	})
	defer done()

	// s1 is a tls secret
	s1 := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret",
			Namespace: "default",
		},
		Type: "kubernetes.io/tls",
		Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
	}

	// i1 is a tls ingress
	i1 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: v1beta1.IngressSpec{
			TLS: []v1beta1.IngressTLS{{
				Hosts:      []string{"kuard.example.com"},
				SecretName: "secret",
			}},
			Rules: []v1beta1.IngressRule{{
				Host: "kuard.example.com",
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Backend: *backend("backend", intstr.FromInt(80)),
						}},
					},
				},
			}},
		},
	}

	// add secret
	rh.OnAdd(s1)

	// assert that there is only a static listener
	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: resources(t,
			staticListener(),
		),
		TypeUrl: listenerType,
		Nonce:   "0",
	}, streamLDS(t, cc))

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "backend",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:     "http",
				Protocol: "TCP",
				Port:     80,
			}},
		},
	})

	// add ingress and assert the existence of ingress_http and ingres_https and both
	// are using proxy protocol
	rh.OnAdd(i1)

	ingress_https := &v2.Listener{
		Name:    "ingress_https",
		Address: envoy.SocketAddress("0.0.0.0", 8443),
		ListenerFilters: envoy.ListenerFilters(
			envoy.ProxyProtocol(),
			envoy.TLSInspector(),
		),
		FilterChains: filterchaintls("kuard.example.com", s1, envoy.HTTPConnectionManager("ingress_https", envoy.FileAccessLogEnvoy("/dev/stdout"), 0), "h2", "http/1.1"),
	}
	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "1",
		Resources: resources(t,
			&v2.Listener{
				Name:    "ingress_http",
				Address: envoy.SocketAddress("0.0.0.0", 8080),
				ListenerFilters: envoy.ListenerFilters(
					envoy.ProxyProtocol(),
				),
				FilterChains: envoy.FilterChains(envoy.HTTPConnectionManager("ingress_http", envoy.FileAccessLogEnvoy("/dev/stdout"), 0)),
			},
			ingress_https,
			staticListener(),
		),
		TypeUrl: listenerType,
		Nonce:   "1",
	}, streamLDS(t, cc))
}

func TestLDSCustomAddressAndPort(t *testing.T) {
	rh, cc, done := setup(t, func(reh *contour.EventHandler) {
		reh.CacheHandler.HTTPAddress = "127.0.0.100"
		reh.CacheHandler.HTTPPort = 9100
		reh.CacheHandler.HTTPSAddress = "127.0.0.200"
		reh.CacheHandler.HTTPSPort = 9200
	})
	defer done()

	// s1 is a tls secret
	s1 := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret",
			Namespace: "default",
		},
		Type: "kubernetes.io/tls",
		Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
	}

	// i1 is a tls ingress
	i1 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: v1beta1.IngressSpec{
			TLS: []v1beta1.IngressTLS{{
				Hosts:      []string{"kuard.example.com"},
				SecretName: "secret",
			}},
			Rules: []v1beta1.IngressRule{{
				Host: "kuard.example.com",
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Backend: *backend("backend", intstr.FromInt(80)),
						}},
					},
				},
			}},
		},
	}

	// add secret
	rh.OnAdd(s1)

	// assert that there is only a static listener
	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: resources(t,
			staticListener(),
		),
		TypeUrl: listenerType,
		Nonce:   "0",
	}, streamLDS(t, cc))

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "backend",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:     "http",
				Protocol: "TCP",
				Port:     80,
			}},
		},
	})

	// add ingress and assert the existence of ingress_http and ingres_https and both
	// are using proxy protocol
	rh.OnAdd(i1)

	ingress_http := &v2.Listener{
		Name:    "ingress_http",
		Address: envoy.SocketAddress("127.0.0.100", 9100),
		FilterChains: envoy.FilterChains(
			envoy.HTTPConnectionManager("ingress_http", envoy.FileAccessLogEnvoy("/dev/stdout"), 0),
		),
	}
	ingress_https := &v2.Listener{
		Name:    "ingress_https",
		Address: envoy.SocketAddress("127.0.0.200", 9200),
		ListenerFilters: envoy.ListenerFilters(
			envoy.TLSInspector(),
		),
		FilterChains: filterchaintls("kuard.example.com", s1, envoy.HTTPConnectionManager("ingress_https", envoy.FileAccessLogEnvoy("/dev/stdout"), 0), "h2", "http/1.1"),
	}
	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "1",
		Resources: resources(t,
			ingress_http,
			ingress_https,
			staticListener(),
		),
		TypeUrl: listenerType,
		Nonce:   "1",
	}, streamLDS(t, cc))
}

func TestLDSCustomAccessLogPaths(t *testing.T) {
	rh, cc, done := setup(t, func(reh *contour.EventHandler) {
		reh.CacheHandler.HTTPAccessLog = "/tmp/http_access.log"
		reh.CacheHandler.HTTPSAccessLog = "/tmp/https_access.log"
	})
	defer done()

	// s1 is a tls secret
	s1 := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret",
			Namespace: "default",
		},
		Type: "kubernetes.io/tls",
		Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
	}

	// i1 is a tls ingress
	i1 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: v1beta1.IngressSpec{
			TLS: []v1beta1.IngressTLS{{
				Hosts:      []string{"kuard.example.com"},
				SecretName: "secret",
			}},
			Rules: []v1beta1.IngressRule{{
				Host: "kuard.example.com",
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Backend: *backend("backend", intstr.FromInt(80)),
						}},
					},
				},
			}},
		},
	}

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "backend",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:     "http",
				Protocol: "TCP",
				Port:     80,
			}},
		},
	})

	// add secret
	rh.OnAdd(s1)

	// assert that there is only a static listener
	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: resources(t,
			staticListener(),
		),
		TypeUrl: listenerType,
		Nonce:   "0",
	}, streamLDS(t, cc))

	rh.OnAdd(i1)

	ingress_http := &v2.Listener{
		Name:    "ingress_http",
		Address: envoy.SocketAddress("0.0.0.0", 8080),
		FilterChains: envoy.FilterChains(
			envoy.HTTPConnectionManager("ingress_http", envoy.FileAccessLogEnvoy("/tmp/http_access.log"), 0),
		),
	}
	ingress_https := &v2.Listener{
		Name:    "ingress_https",
		Address: envoy.SocketAddress("0.0.0.0", 8443),
		ListenerFilters: envoy.ListenerFilters(
			envoy.TLSInspector(),
		),
		FilterChains: filterchaintls("kuard.example.com", s1, envoy.HTTPConnectionManager("ingress_https", envoy.FileAccessLogEnvoy("/tmp/https_access.log"), 0), "h2", "http/1.1"),
	}
	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "1",
		Resources: resources(t,
			ingress_http,
			ingress_https,
			staticListener(),
		),
		TypeUrl: listenerType,
		Nonce:   "1",
	}, streamLDS(t, cc))
}

func TestLDSIngressRouteInsideRootNamespaces(t *testing.T) {
	rh, cc, done := setup(t, func(reh *contour.EventHandler) {
		reh.Builder.Source.RootNamespaces = []string{"roots"}
	})
	defer done()

	// assert that there is only a static listener
	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: resources(t,
			staticListener(),
		),
		TypeUrl: listenerType,
		Nonce:   "0",
	}, streamLDS(t, cc))

	// ir1 is an ingressroute that is in the root namespace
	ir1 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "roots",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &projcontour.VirtualHost{Fqdn: "example.com"},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Services: []ingressroutev1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	svc1 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "roots",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:     "http",
				Protocol: "TCP",
				Port:     8080,
			}},
		},
	}

	// add ingressroute & service
	rh.OnAdd(svc1)
	rh.OnAdd(ir1)

	// assert there is an active listener
	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "1",
		Resources: resources(t,
			&v2.Listener{
				Name:    "ingress_http",
				Address: envoy.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy.FilterChains(
					envoy.HTTPConnectionManager("ingress_http", envoy.FileAccessLogEnvoy("/dev/stdout"), 0),
				),
			},
			staticListener(),
		),
		TypeUrl: listenerType,
		Nonce:   "1",
	}, streamLDS(t, cc))
}

func TestLDSIngressRouteOutsideRootNamespaces(t *testing.T) {
	rh, cc, done := setup(t, func(reh *contour.EventHandler) {
		reh.Builder.Source.RootNamespaces = []string{"roots"}
	})
	defer done()

	// assert that there is only a static listener
	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: resources(t,
			staticListener(),
		),
		TypeUrl: listenerType,
		Nonce:   "0",
	}, streamLDS(t, cc))

	// ir1 is an ingressroute that is not in the root namespaces
	ir1 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &projcontour.VirtualHost{Fqdn: "example.com"},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Services: []ingressroutev1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	// add ingressroute
	rh.OnAdd(ir1)

	// assert that there is only a static listener
	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "1",
		Resources: resources(t,
			staticListener(),
		),
		TypeUrl: listenerType,
		Nonce:   "1",
	}, streamLDS(t, cc))
}

func TestIngressRouteHTTPS(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	// assert that there is only a static listener
	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: resources(t,
			staticListener(),
		),
		TypeUrl: listenerType,
		Nonce:   "0",
	}, streamLDS(t, cc))

	// s1 is a tls secret
	s1 := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret",
			Namespace: "default",
		},
		Type: "kubernetes.io/tls",
		Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
	}

	// ir1 is an ingressroute that has TLS
	ir1 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
				TLS: &projcontour.TLS{
					SecretName: "secret",
				},
			},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Services: []ingressroutev1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	svc1 := &v1.Service{
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

	// add secret
	rh.OnAdd(s1)

	// add service
	rh.OnAdd(svc1)

	// add ingressroute
	rh.OnAdd(ir1)

	ingressHTTP := &v2.Listener{
		Name:    "ingress_http",
		Address: envoy.SocketAddress("0.0.0.0", 8080),
		FilterChains: envoy.FilterChains(
			envoy.HTTPConnectionManager("ingress_http", envoy.FileAccessLogEnvoy("/dev/stdout"), 0),
		),
	}

	ingressHTTPS := &v2.Listener{
		Name:    "ingress_https",
		Address: envoy.SocketAddress("0.0.0.0", 8443),
		ListenerFilters: envoy.ListenerFilters(
			envoy.TLSInspector(),
		),
		FilterChains: filterchaintls("example.com", s1, envoy.HTTPConnectionManager("ingress_https", envoy.FileAccessLogEnvoy("/dev/stdout"), 0), "h2", "http/1.1"),
	}
	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "1",
		Resources: resources(t,
			ingressHTTP,
			ingressHTTPS,
			staticListener(),
		),
		TypeUrl: listenerType,
		Nonce:   "1",
	}, streamLDS(t, cc))
}

func TestIngressRouteMinimumTLSVersion(t *testing.T) {
	rh, cc, done := setup(t, func(reh *contour.EventHandler) {
		reh.CacheHandler.MinimumProtocolVersion = envoy_api_v2_auth.TlsParameters_TLSv1_2
	})

	defer done()

	// secret1 is a tls secret
	secret1 := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret",
			Namespace: "default",
		},
		Type: "kubernetes.io/tls",
		Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
	}
	rh.OnAdd(secret1)

	svc1 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "backend",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:     "http",
				Protocol: "TCP",
				Port:     80,
			}},
		},
	}
	rh.OnAdd(svc1)

	// i1 is a tls ingressroute
	i1 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "kuard.example.com",
				TLS: &projcontour.TLS{
					SecretName:             "secret",
					MinimumProtocolVersion: "1.1",
				},
			},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Services: []ingressroutev1.Service{{
					Name: "backend",
					Port: 80,
				}},
			}},
		},
	}
	rh.OnAdd(i1)

	l1 := &v2.Listener{
		Name:    "ingress_https",
		Address: envoy.SocketAddress("0.0.0.0", 8443),
		ListenerFilters: envoy.ListenerFilters(
			envoy.TLSInspector(),
		),
		FilterChains: []*envoy_api_v2_listener.FilterChain{
			envoy.FilterChainTLS(
				"kuard.example.com",
				&dag.Secret{Object: secret1},
				envoy.Filters(
					envoy.HTTPConnectionManager("ingress_https", envoy.FileAccessLogEnvoy("/dev/stdout"), 0),
				),
				envoy_api_v2_auth.TlsParameters_TLSv1_2,
				"h2", "http/1.1",
			),
		},
	}

	// verify that i1's TLS 1.1 minimum has been upgraded to 1.2
	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "1",
		Resources: resources(t,
			&v2.Listener{
				Name:    "ingress_http",
				Address: envoy.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy.FilterChains(
					envoy.HTTPConnectionManager("ingress_http", envoy.FileAccessLogEnvoy("/dev/stdout"), 0),
				),
			},
			l1,
			staticListener(),
		),
		TypeUrl: listenerType,
		Nonce:   "1",
	}, streamLDS(t, cc))

	// i2 is a tls ingressroute
	i2 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "kuard.example.com",
				TLS: &projcontour.TLS{
					SecretName:             "secret",
					MinimumProtocolVersion: "1.3",
				},
			},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Services: []ingressroutev1.Service{{
					Name: "backend",
					Port: 80,
				}},
			}},
		},
	}
	rh.OnUpdate(i1, i2)

	l2 := &v2.Listener{
		Name:    "ingress_https",
		Address: envoy.SocketAddress("0.0.0.0", 8443),
		ListenerFilters: envoy.ListenerFilters(
			envoy.TLSInspector(),
		),
		FilterChains: []*envoy_api_v2_listener.FilterChain{
			envoy.FilterChainTLS(
				"kuard.example.com",
				&dag.Secret{Object: secret1},
				envoy.Filters(
					envoy.HTTPConnectionManager("ingress_https", envoy.FileAccessLogEnvoy("/dev/stdout"), 0),
				),
				envoy_api_v2_auth.TlsParameters_TLSv1_3,
				"h2", "http/1.1",
			),
		},
	}

	// verify that i2's TLS 1.3 minimum has NOT been downgraded to 1.2
	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "2",
		Resources: resources(t,
			&v2.Listener{
				Name:    "ingress_http",
				Address: envoy.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy.FilterChains(
					envoy.HTTPConnectionManager("ingress_http", envoy.FileAccessLogEnvoy("/dev/stdout"), 0),
				),
			},
			l2,
			staticListener(),
		),
		TypeUrl: listenerType,
		Nonce:   "2",
	}, streamLDS(t, cc))
}

func TestLDSIngressRouteRootCannotDelegateToAnotherRoot(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	svc1 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "green",
			Namespace: "marketing",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:     "http",
				Protocol: "TCP",
				Port:     80,
			}},
		},
	}
	rh.OnAdd(svc1)

	child := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "blog",
			Namespace: "marketing",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "www.containersteve.com",
			},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Services: []ingressroutev1.Service{{
					Name: svc1.Name,
					Port: 80,
				}},
			}},
		},
	}
	rh.OnAdd(child)

	root := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "root-blog",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "blog.containersteve.com",
			},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Delegate: &ingressroutev1.Delegate{
					Name:      child.Name,
					Namespace: child.Namespace,
				},
			}},
		},
	}
	rh.OnAdd(root)

	// verify that port 80 is present because while it is not possible to
	// delegate to it, child can host a vhost which opens port 80.
	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "2",
		Resources: resources(t,
			&v2.Listener{
				Name:    "ingress_http",
				Address: envoy.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy.FilterChains(
					envoy.HTTPConnectionManager("ingress_http", envoy.FileAccessLogEnvoy("/dev/stdout"), 0),
				),
			},
			staticListener(),
		),
		TypeUrl: listenerType,
		Nonce:   "2",
	}, streamLDS(t, cc))
}

func streamLDS(t *testing.T, cc *grpc.ClientConn, rn ...string) *v2.DiscoveryResponse {
	t.Helper()
	rds := v2.NewListenerDiscoveryServiceClient(cc)
	st, err := rds.StreamListeners(context.TODO())
	check(t, err)
	return stream(t, st, &v2.DiscoveryRequest{
		TypeUrl:       listenerType,
		ResourceNames: rn,
	})
}

func backend(name string, port intstr.IntOrString) *v1beta1.IngressBackend {
	return &v1beta1.IngressBackend{
		ServiceName: name,
		ServicePort: port,
	}
}

func filterchaintls(domain string, secret *v1.Secret, filter *envoy_api_v2_listener.Filter, alpn ...string) []*envoy_api_v2_listener.FilterChain {
	return []*envoy_api_v2_listener.FilterChain{
		envoy.FilterChainTLS(
			domain,
			&dag.Secret{Object: secret},
			envoy.Filters(filter),
			envoy_api_v2_auth.TlsParameters_TLSv1_1,
			alpn...,
		),
	}
}

func staticListener() *v2.Listener {
	return envoy.StatsListener(statsAddress, statsPort)
}
