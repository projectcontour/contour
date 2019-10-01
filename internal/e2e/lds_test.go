// Copyright © 2018 Heptio
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
	"time"

	ingressroutev1 "github.com/heptio/contour/apis/contour/v1beta1"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	envoy_config_v2_tcpproxy "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/tcp_proxy/v2"
	"github.com/envoyproxy/go-control-plane/pkg/util"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"github.com/heptio/contour/internal/contour"
	"github.com/heptio/contour/internal/dag"
	"github.com/heptio/contour/internal/envoy"
	"google.golang.org/grpc"
	v1 "k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestNonTLSListener(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	// assert that without any ingress objects registered
	// there are no active listeners
	assertEqual(t, &v2.DiscoveryResponse{
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
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "1",
		Resources: resources(t,
			&v2.Listener{
				Name:         "ingress_http",
				Address:      *envoy.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy.FilterChains(envoy.HTTPConnectionManager("ingress_http", "/dev/stdout")),
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
	assertEqual(t, &v2.DiscoveryResponse{
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
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "3",
		Resources: resources(t,
			&v2.Listener{
				Name:         "ingress_http",
				Address:      *envoy.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy.FilterChains(envoy.HTTPConnectionManager("ingress_http", "/dev/stdout")),
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
			Backend: backend("backend", intstr.FromInt(80)),
			TLS: []v1beta1.IngressTLS{{
				Hosts:      []string{"kuard.example.com"},
				SecretName: "secret",
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
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, staticListener()),
		},
		TypeUrl: listenerType,
		Nonce:   "0",
	}, streamLDS(t, cc))

	// add ingress and assert the existence of ingress_http and ingres_https
	rh.OnAdd(i1)
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "1",
		Resources: []types.Any{
			any(t, &v2.Listener{
				Name:         "ingress_http",
				Address:      *envoy.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy.FilterChains(envoy.HTTPConnectionManager("ingress_http", "/dev/stdout")),
			}),
			any(t, &v2.Listener{
				Name:    "ingress_https",
				Address: *envoy.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: []listener.ListenerFilter{
					envoy.TLSInspector(),
				},
				FilterChains: filterchaintls("kuard.example.com", s1, envoy.HTTPConnectionManager("ingress_https", "/dev/stdout"), "h2", "http/1.1"),
			}),
			any(t, staticListener()),
		},
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
			TLS: []v1beta1.IngressTLS{{
				Hosts:      []string{"kuard.example.com"},
				SecretName: "secret",
			}},
		},
	}

	// update i1 to i2 and verify that ingress_http has gone.
	rh.OnUpdate(i1, i2)
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "2",
		Resources: []types.Any{
			any(t, &v2.Listener{
				Name:    "ingress_https",
				Address: *envoy.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: []listener.ListenerFilter{
					envoy.TLSInspector(),
				},
				FilterChains: filterchaintls("kuard.example.com", s1, envoy.HTTPConnectionManager("ingress_https", "/dev/stdout"), "h2", "http/1.1"),
			}),
			any(t, staticListener()),
		},
		TypeUrl: listenerType,
		Nonce:   "2",
	}, streamLDS(t, cc))

	// delete secret and assert that ingress_https is removed
	rh.OnDelete(s1)
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "3",
		Resources: []types.Any{
			any(t, staticListener()),
		},
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

	// i1 is a tls ingressroute
	i1 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "kuard.example.com",
				TLS: &ingressroutev1.TLS{
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

	// i2 is a tls ingressroute
	i2 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "kuard.example.com",
				TLS: &ingressroutev1.TLS{
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

	// add secret
	rh.OnAdd(secret1)

	// assert that there is only a static listener
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, staticListener()),
		},
		TypeUrl: listenerType,
		Nonce:   "0",
	}, streamLDS(t, cc))

	l1 := &v2.Listener{
		Name:    "ingress_https",
		Address: *envoy.SocketAddress("0.0.0.0", 8443),
		ListenerFilters: []listener.ListenerFilter{
			envoy.TLSInspector(),
		},
		FilterChains: filterchaintls("kuard.example.com", secret1, envoy.HTTPConnectionManager("ingress_https", "/dev/stdout"), "h2", "http/1.1"),
	}

	l1.FilterChains[0].TlsContext.CommonTlsContext.TlsParams.TlsMinimumProtocolVersion = auth.TlsParameters_TLSv1_1

	// add service
	rh.OnAdd(svc1)

	// add ingress and assert the existence of ingress_http and ingres_https
	rh.OnAdd(i1)

	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "1",
		Resources: []types.Any{
			any(t, &v2.Listener{
				Name:         "ingress_http",
				Address:      *envoy.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy.FilterChains(envoy.HTTPConnectionManager("ingress_http", "/dev/stdout")),
			}),
			any(t, l1),
			any(t, staticListener()),
		},
		TypeUrl: listenerType,
		Nonce:   "1",
	}, streamLDS(t, cc))

	// delete secret and assert that ingress_https is removed
	rh.OnDelete(secret1)
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "2",
		Resources: []types.Any{
			any(t, &v2.Listener{
				Name:         "ingress_http",
				Address:      *envoy.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy.FilterChains(envoy.HTTPConnectionManager("ingress_http", "/dev/stdout")),
			}),
			any(t, staticListener()),
		},
		TypeUrl: listenerType,
		Nonce:   "2",
	}, streamLDS(t, cc))

	rh.OnDelete(i1)
	// add secret
	rh.OnAdd(secret1)
	l2 := &v2.Listener{
		Name:    "ingress_https",
		Address: *envoy.SocketAddress("0.0.0.0", 8443),
		ListenerFilters: []listener.ListenerFilter{
			envoy.TLSInspector(),
		},
		FilterChains: filterchaintls("kuard.example.com", secret1, envoy.HTTPConnectionManager("ingress_https", "/dev/stdout"), "h2", "http/1.1"),
	}

	l2.FilterChains[0].TlsContext.CommonTlsContext.TlsParams.TlsMinimumProtocolVersion = auth.TlsParameters_TLSv1_3

	// add ingress and assert the existence of ingress_http and ingres_https
	rh.OnAdd(i2)
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "4",
		Resources: []types.Any{
			any(t, &v2.Listener{
				Name:         "ingress_http",
				Address:      *envoy.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy.FilterChains(envoy.HTTPConnectionManager("ingress_http", "/dev/stdout")),
			}),
			any(t, l2),
			any(t, staticListener()),
		},
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
			Backend: backend("backend", intstr.FromInt(80)),
			TLS: []v1beta1.IngressTLS{{
				Hosts:      []string{"kuard.example.com"},
				SecretName: "secret",
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
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "1",
		Resources: []types.Any{
			any(t, &v2.Listener{
				Name:    "ingress_https",
				Address: *envoy.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: []listener.ListenerFilter{
					envoy.TLSInspector(),
				},
				FilterChains: filterchaintls("kuard.example.com", s1, envoy.HTTPConnectionManager("ingress_https", "/dev/stdout"), "h2", "http/1.1"),
			}),
		},
		TypeUrl: listenerType,
		Nonce:   "1",
	}, streamLDS(t, cc, "ingress_https"))

	// fetch ingress_http
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "1",
		Resources: []types.Any{
			any(t, &v2.Listener{
				Name:         "ingress_http",
				Address:      *envoy.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy.FilterChains(envoy.HTTPConnectionManager("ingress_http", "/dev/stdout")),
			}),
		},
		TypeUrl: listenerType,
		Nonce:   "1",
	}, streamLDS(t, cc, "ingress_http"))

	// fetch something non existent.
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "1",
		TypeUrl:     listenerType,
		Nonce:       "1",
	}, streamLDS(t, cc, "HTTP"))
}

func TestLDSStreamEmpty(t *testing.T) {
	_, cc, done := setup(t)
	defer done()

	// assert that streaming LDS with no ingresses does not stall.
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		TypeUrl:     listenerType,
		Nonce:       "0",
	}, streamLDS(t, cc, "HTTP"))
}

func TestLDSTLSMinimumProtocolVersion(t *testing.T) {
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
	rh.OnAdd(s1)

	// i1 is a tls ingress
	i1 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: v1beta1.IngressSpec{
			Backend: backend("backend", intstr.FromInt(80)),
			TLS: []v1beta1.IngressTLS{{
				Hosts:      []string{"kuard.example.com"},
				SecretName: "secret",
			}},
		},
	}

	rh.OnAdd(i1)

	// add ingress and fetch ingress_https
	rh.OnAdd(i1)
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "2",
		Resources: []types.Any{
			any(t, &v2.Listener{
				Name:    "ingress_https",
				Address: *envoy.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: []listener.ListenerFilter{
					envoy.TLSInspector(),
				},
				FilterChains: filterchaintls("kuard.example.com", s1, envoy.HTTPConnectionManager("ingress_https", "/dev/stdout"), "h2", "http/1.1"),
			}),
		},
		TypeUrl: listenerType,
		Nonce:   "2",
	}, streamLDS(t, cc, "ingress_https"))

	i2 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
			Annotations: map[string]string{
				"contour.heptio.com/tls-minimum-protocol-version": "1.3",
			},
		},
		Spec: v1beta1.IngressSpec{
			Backend: backend("backend", intstr.FromInt(80)),
			TLS: []v1beta1.IngressTLS{{
				Hosts:      []string{"kuard.example.com"},
				SecretName: "secret",
			}},
		},
	}

	// update tls version and fetch ingress_https
	rh.OnUpdate(i1, i2)

	l1 := &v2.Listener{
		Name:    "ingress_https",
		Address: *envoy.SocketAddress("0.0.0.0", 8443),
		ListenerFilters: []listener.ListenerFilter{
			envoy.TLSInspector(),
		},
		FilterChains: filterchaintls("kuard.example.com", s1, envoy.HTTPConnectionManager("ingress_https", "/dev/stdout"), "h2", "http/1.1"),
	}
	// easier to patch this up than add more params to filterchaintls
	l1.FilterChains[0].TlsContext.CommonTlsContext.TlsParams.TlsMinimumProtocolVersion = auth.TlsParameters_TLSv1_3

	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "3",
		Resources: []types.Any{
			any(t, l1),
		},
		TypeUrl: listenerType,
		Nonce:   "3",
	}, streamLDS(t, cc, "ingress_https"))
}

func TestLDSIngressHTTPUseProxyProtocol(t *testing.T) {
	rh, cc, done := setup(t, func(reh *contour.EventHandler) {
		reh.CacheHandler.UseProxyProto = true
	})
	defer done()

	// assert that without any ingress objects registered
	// there is only a static listener
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, staticListener()),
		},
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
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "1",
		Resources: []types.Any{
			any(t, &v2.Listener{
				Name:    "ingress_http",
				Address: *envoy.SocketAddress("0.0.0.0", 8080),
				ListenerFilters: []listener.ListenerFilter{
					envoy.ProxyProtocol(),
				},
				FilterChains: envoy.FilterChains(envoy.HTTPConnectionManager("ingress_http", "/dev/stdout")),
			}),
			any(t, staticListener()),
		},
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
			Backend: backend("backend", intstr.FromInt(80)),
			TLS: []v1beta1.IngressTLS{{
				Hosts:      []string{"kuard.example.com"},
				SecretName: "secret",
			}},
		},
	}

	// add secret
	rh.OnAdd(s1)

	// assert that there is only a static listener
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, staticListener()),
		},
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
		Address: *envoy.SocketAddress("0.0.0.0", 8443),
		ListenerFilters: []listener.ListenerFilter{
			envoy.ProxyProtocol(),
			envoy.TLSInspector(),
		},
		FilterChains: filterchaintls("kuard.example.com", s1, envoy.HTTPConnectionManager("ingress_https", "/dev/stdout"), "h2", "http/1.1"),
	}
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "1",
		Resources: []types.Any{
			any(t, &v2.Listener{
				Name:    "ingress_http",
				Address: *envoy.SocketAddress("0.0.0.0", 8080),
				ListenerFilters: []listener.ListenerFilter{
					envoy.ProxyProtocol(),
				},
				FilterChains: envoy.FilterChains(envoy.HTTPConnectionManager("ingress_http", "/dev/stdout")),
			}),
			any(t, ingress_https),
			any(t, staticListener()),
		},
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
			Backend: backend("backend", intstr.FromInt(80)),
			TLS: []v1beta1.IngressTLS{{
				Hosts:      []string{"kuard.example.com"},
				SecretName: "secret",
			}},
		},
	}

	// add secret
	rh.OnAdd(s1)

	// assert that there is only a static listener
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, staticListener()),
		},
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
		Name:         "ingress_http",
		Address:      *envoy.SocketAddress("127.0.0.100", 9100),
		FilterChains: envoy.FilterChains(envoy.HTTPConnectionManager("ingress_http", "/dev/stdout")),
	}
	ingress_https := &v2.Listener{
		Name:    "ingress_https",
		Address: *envoy.SocketAddress("127.0.0.200", 9200),
		ListenerFilters: []listener.ListenerFilter{
			envoy.TLSInspector(),
		},
		FilterChains: filterchaintls("kuard.example.com", s1, envoy.HTTPConnectionManager("ingress_https", "/dev/stdout"), "h2", "http/1.1"),
	}
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "1",
		Resources: []types.Any{
			any(t, ingress_http),
			any(t, ingress_https),
			any(t, staticListener()),
		},
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
			Backend: backend("backend", intstr.FromInt(80)),
			TLS: []v1beta1.IngressTLS{{
				Hosts:      []string{"kuard.example.com"},
				SecretName: "secret",
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
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, staticListener()),
		},
		TypeUrl: listenerType,
		Nonce:   "0",
	}, streamLDS(t, cc))

	rh.OnAdd(i1)

	ingress_http := &v2.Listener{
		Name:         "ingress_http",
		Address:      *envoy.SocketAddress("0.0.0.0", 8080),
		FilterChains: envoy.FilterChains(envoy.HTTPConnectionManager("ingress_http", "/tmp/http_access.log")),
	}
	ingress_https := &v2.Listener{
		Name:    "ingress_https",
		Address: *envoy.SocketAddress("0.0.0.0", 8443),
		ListenerFilters: []listener.ListenerFilter{
			envoy.TLSInspector(),
		},
		FilterChains: filterchaintls("kuard.example.com", s1, envoy.HTTPConnectionManager("ingress_https", "/tmp/https_access.log"), "h2", "http/1.1"),
	}
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "1",
		Resources: []types.Any{
			any(t, ingress_http),
			any(t, ingress_https),
			any(t, staticListener()),
		},
		TypeUrl: listenerType,
		Nonce:   "1",
	}, streamLDS(t, cc))
}

func TestLDSIngressRouteInsideRootNamespaces(t *testing.T) {
	rh, cc, done := setup(t, func(reh *contour.EventHandler) {
		reh.Builder.Source.IngressRouteRootNamespaces = []string{"roots"}
	})
	defer done()

	// assert that there is only a static listener
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, staticListener()),
		},
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
			VirtualHost: &ingressroutev1.VirtualHost{Fqdn: "example.com"},
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
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "1",
		Resources: []types.Any{
			any(t, &v2.Listener{
				Name:         "ingress_http",
				Address:      *envoy.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy.FilterChains(envoy.HTTPConnectionManager("ingress_http", "/dev/stdout")),
			}),
			any(t, staticListener()),
		},
		TypeUrl: listenerType,
		Nonce:   "1",
	}, streamLDS(t, cc))
}

func TestLDSIngressRouteOutsideRootNamespaces(t *testing.T) {
	rh, cc, done := setup(t, func(reh *contour.EventHandler) {
		reh.Builder.Source.IngressRouteRootNamespaces = []string{"roots"}
	})
	defer done()

	// assert that there is only a static listener
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, staticListener()),
		},
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
			VirtualHost: &ingressroutev1.VirtualHost{Fqdn: "example.com"},
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
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "1",
		Resources: []types.Any{
			any(t, staticListener()),
		},
		TypeUrl: listenerType,
		Nonce:   "1",
	}, streamLDS(t, cc))
}

func TestIngressRouteHTTPS(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	// assert that there is only a static listener
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, staticListener()),
		},
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
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "example.com",
				TLS: &ingressroutev1.TLS{
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
		Name:         "ingress_http",
		Address:      *envoy.SocketAddress("0.0.0.0", 8080),
		FilterChains: envoy.FilterChains(envoy.HTTPConnectionManager("ingress_http", "/dev/stdout")),
	}

	ingressHTTPS := &v2.Listener{
		Name:    "ingress_https",
		Address: *envoy.SocketAddress("0.0.0.0", 8443),
		ListenerFilters: []listener.ListenerFilter{
			envoy.TLSInspector(),
		},
		FilterChains: filterchaintls("example.com", s1, envoy.HTTPConnectionManager("ingress_https", "/dev/stdout"), "h2", "http/1.1"),
	}
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "1",
		Resources: []types.Any{
			any(t, ingressHTTP),
			any(t, ingressHTTPS),
			any(t, staticListener()),
		},
		TypeUrl: listenerType,
		Nonce:   "1",
	}, streamLDS(t, cc))
}

// Assert that when a spec.vhost.tls spec is present with tls.passthrough
// set to true we configure envoy to forward the TLS session to the cluster
// after using SNI to determine the target.
func TestLDSIngressRouteTCPProxyTLSPassthrough(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	i1 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "kuard-tcp.example.com",
				TLS: &ingressroutev1.TLS{
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
					Name: "correct-backend",
					Port: 80,
				}},
			},
		},
	}
	svc := service("default", "correct-backend", v1.ServicePort{
		Protocol:   "TCP",
		Port:       80,
		TargetPort: intstr.FromInt(8080),
	})
	rh.OnAdd(svc)
	rh.OnAdd(i1)

	ingressHTTPS := &v2.Listener{
		Name:    "ingress_https",
		Address: *envoy.SocketAddress("0.0.0.0", 8443),
		FilterChains: []listener.FilterChain{{
			Filters: []listener.Filter{
				tcpproxy(t, "ingress_https", "default/correct-backend/80/da39a3ee5e"),
			},
			FilterChainMatch: &listener.FilterChainMatch{
				ServerNames: []string{"kuard-tcp.example.com"},
			},
		}},
		ListenerFilters: []listener.ListenerFilter{
			envoy.TLSInspector(),
		},
	}

	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "1",
		Resources: []types.Any{
			any(t, ingressHTTPS),
			any(t, staticListener()),
		},
		TypeUrl: listenerType,
		Nonce:   "1",
	}, streamLDS(t, cc))
}

func TestLDSIngressRouteTCPForward(t *testing.T) {
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

	i1 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "kuard-tcp.example.com",
				TLS: &ingressroutev1.TLS{
					SecretName: "secret",
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
					Name: "correct-backend",
					Port: 80,
				}},
			},
		},
	}
	rh.OnAdd(s1)
	svc := service("default", "correct-backend", v1.ServicePort{
		Protocol:   "TCP",
		Port:       80,
		TargetPort: intstr.FromInt(8080),
	})
	rh.OnAdd(svc)
	rh.OnAdd(i1)

	ingressHTTPS := &v2.Listener{
		Name:         "ingress_https",
		Address:      *envoy.SocketAddress("0.0.0.0", 8443),
		FilterChains: filterchaintls("kuard-tcp.example.com", s1, tcpproxy(t, "ingress_https", "default/correct-backend/80/da39a3ee5e")),
		ListenerFilters: []listener.ListenerFilter{
			envoy.TLSInspector(),
		},
	}

	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "1",
		Resources: []types.Any{
			any(t, ingressHTTPS),
			any(t, staticListener()),
		},
		TypeUrl: listenerType,
		Nonce:   "1",
	}, streamLDS(t, cc))
}

// Test that TLS Cerfiticate delegation works correctly.
func TestIngressRouteTLSCertificateDelegation(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	// assert that there is only a static listener
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, staticListener()),
		},
		TypeUrl: listenerType,
		Nonce:   "0",
	}, streamLDS(t, cc))

	s1 := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "wildcard",
			Namespace: "secret",
		},
		Type: "kubernetes.io/tls",
		Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
	}

	// add a secret object secret/wildcard.
	rh.OnAdd(s1)

	rh.OnAdd(&v1.Service{
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
	})

	// add an ingressroute in a different namespace mentioning secret/wildcard.
	rh.OnAdd(&ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "example.com",
				TLS: &ingressroutev1.TLS{
					SecretName: "secret/wildcard",
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
	})

	ingress_http := &v2.Listener{
		Name:         "ingress_http",
		Address:      *envoy.SocketAddress("0.0.0.0", 8080),
		FilterChains: envoy.FilterChains(envoy.HTTPConnectionManager("ingress_http", "/dev/stdout")),
	}

	// assert there is no ingress_https because there is no matching secret.
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "1",
		Resources: []types.Any{
			any(t, ingress_http),
			any(t, staticListener()),
		},
		TypeUrl: listenerType,
		Nonce:   "1",
	}, streamLDS(t, cc))

	// t1 is a TLSCertificateDelegation that permits default to access secret/wildcard
	t1 := &ingressroutev1.TLSCertificateDelegation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "delegation",
			Namespace: "secret",
		},
		Spec: ingressroutev1.TLSCertificateDelegationSpec{
			Delegations: []ingressroutev1.CertificateDelegation{{
				SecretName: "wildcard",
				TargetNamespaces: []string{
					"default",
				},
			}},
		},
	}
	rh.OnAdd(t1)

	ingress_https := &v2.Listener{
		Name:    "ingress_https",
		Address: *envoy.SocketAddress("0.0.0.0", 8443),
		ListenerFilters: []listener.ListenerFilter{
			envoy.TLSInspector(),
		},
		FilterChains: filterchaintls("example.com", s1, envoy.HTTPConnectionManager("ingress_https", "/dev/stdout"), "h2", "http/1.1"),
	}

	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "2",
		Resources: []types.Any{
			any(t, ingress_http),
			any(t, ingress_https),
			any(t, staticListener()),
		},
		TypeUrl: listenerType,
		Nonce:   "2",
	}, streamLDS(t, cc))

	// t2 is a TLSCertificateDelegation that permits access to secret/wildcard from all namespaces.
	t2 := &ingressroutev1.TLSCertificateDelegation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "delegation",
			Namespace: "secret",
		},
		Spec: ingressroutev1.TLSCertificateDelegationSpec{
			Delegations: []ingressroutev1.CertificateDelegation{{
				SecretName: "wildcard",
				TargetNamespaces: []string{
					"*",
				},
			}},
		},
	}
	rh.OnUpdate(t1, t2)

	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "3",
		Resources: []types.Any{
			any(t, ingress_http),
			any(t, ingress_https),
			any(t, staticListener()),
		},
		TypeUrl: listenerType,
		Nonce:   "3",
	}, streamLDS(t, cc))

	// t3 is a TLSCertificateDelegation that permits access to secret/different all namespaces.
	t3 := &ingressroutev1.TLSCertificateDelegation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "delegation",
			Namespace: "secret",
		},
		Spec: ingressroutev1.TLSCertificateDelegationSpec{
			Delegations: []ingressroutev1.CertificateDelegation{{
				SecretName: "different",
				TargetNamespaces: []string{
					"*",
				},
			}},
		},
	}
	rh.OnUpdate(t2, t3)

	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "4",
		Resources: []types.Any{
			any(t, ingress_http),
			any(t, staticListener()),
		},
		TypeUrl: listenerType,
		Nonce:   "4",
	}, streamLDS(t, cc))

	// t4 is a TLSCertificateDelegation that permits access to secret/wildcard from the kube-secret namespace.
	t4 := &ingressroutev1.TLSCertificateDelegation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "delegation",
			Namespace: "secret",
		},
		Spec: ingressroutev1.TLSCertificateDelegationSpec{
			Delegations: []ingressroutev1.CertificateDelegation{{
				SecretName: "wildcard",
				TargetNamespaces: []string{
					"kube-secret",
				},
			}},
		},
	}
	rh.OnUpdate(t3, t4)

	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "5",
		Resources: []types.Any{
			any(t, ingress_http),
			any(t, staticListener()),
		},
		TypeUrl: listenerType,
		Nonce:   "5",
	}, streamLDS(t, cc))

}

func TestIngressRouteMinimumTLSVersion(t *testing.T) {
	rh, cc, done := setup(t, func(reh *contour.EventHandler) {
		reh.CacheHandler.MinimumProtocolVersion = auth.TlsParameters_TLSv1_2
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
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "kuard.example.com",
				TLS: &ingressroutev1.TLS{
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
		Address: *envoy.SocketAddress("0.0.0.0", 8443),
		ListenerFilters: []listener.ListenerFilter{
			envoy.TLSInspector(),
		},
		FilterChains: filterchaintls("kuard.example.com", secret1, envoy.HTTPConnectionManager("ingress_https", "/dev/stdout"), "h2", "http/1.1"),
	}
	l1.FilterChains[0].TlsContext.CommonTlsContext.TlsParams.TlsMinimumProtocolVersion = auth.TlsParameters_TLSv1_2

	// verify that i1's TLS 1.1 minimum has been upgraded to 1.2
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "1",
		Resources: []types.Any{
			any(t, &v2.Listener{
				Name:         "ingress_http",
				Address:      *envoy.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy.FilterChains(envoy.HTTPConnectionManager("ingress_http", "/dev/stdout")),
			}),
			any(t, l1),
			any(t, staticListener()),
		},
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
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "kuard.example.com",
				TLS: &ingressroutev1.TLS{
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
		Address: *envoy.SocketAddress("0.0.0.0", 8443),
		ListenerFilters: []listener.ListenerFilter{
			envoy.TLSInspector(),
		},
		FilterChains: filterchaintls("kuard.example.com", secret1, envoy.HTTPConnectionManager("ingress_https", "/dev/stdout"), "h2", "http/1.1"),
	}
	l2.FilterChains[0].TlsContext.CommonTlsContext.TlsParams.TlsMinimumProtocolVersion = auth.TlsParameters_TLSv1_3

	// verify that i2's TLS 1.3 minimum has NOT been downgraded to 1.2
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "2",
		Resources: []types.Any{
			any(t, &v2.Listener{
				Name:         "ingress_http",
				Address:      *envoy.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy.FilterChains(envoy.HTTPConnectionManager("ingress_http", "/dev/stdout")),
			}),
			any(t, l2),
			any(t, staticListener()),
		},
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

func filterchaintls(domain string, secret *v1.Secret, filter listener.Filter, alpn ...string) []listener.FilterChain {
	return []listener.FilterChain{
		envoy.FilterChainTLS(
			domain,
			&dag.Secret{Object: secret},
			[]listener.Filter{
				filter,
			},
			auth.TlsParameters_TLSv1_1,
			alpn...,
		),
	}
}

func tcpproxy(t *testing.T, statPrefix, cluster string) listener.Filter {
	// shadow the package level any function as TypedConfig needs a
	// *types.Any whereas the other callers of e2e.any require a value
	// type.
	// TODO(dfc) unify the callers to any.
	any := func(t *testing.T, pb proto.Message) *types.Any {
		t.Helper()
		any, err := types.MarshalAny(pb)
		check(t, err)
		return any
	}

	return listener.Filter{
		Name: util.TCPProxy,
		ConfigType: &listener.Filter_TypedConfig{
			TypedConfig: any(t, &envoy_config_v2_tcpproxy.TcpProxy{
				StatPrefix: statPrefix,
				ClusterSpecifier: &envoy_config_v2_tcpproxy.TcpProxy_Cluster{
					Cluster: cluster,
				},
				AccessLog:   envoy.FileAccessLog("/dev/stdout"),
				IdleTimeout: idleTimeout(envoy.TCPDefaultIdleTimeout),
			}),
		},
	}
}

func staticListener() *v2.Listener {
	return envoy.StatsListener(statsAddress, statsPort)
}

func idleTimeout(d time.Duration) *time.Duration {
	return &d
}
