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
	"bytes"
	"context"
	"testing"
	"time"

	ingressroutev1 "github.com/heptio/contour/apis/contour/v1beta1"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	accesslog "github.com/envoyproxy/go-control-plane/envoy/config/filter/accesslog/v2"
	envoy_config_v2_http_conn_mgr "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"github.com/heptio/contour/internal/contour"
	"github.com/heptio/contour/internal/generated/clientset/versioned/fake"
	"github.com/heptio/contour/internal/k8s"
	"google.golang.org/grpc"
	"k8s.io/api/core/v1"
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
		Resources:   []types.Any{},
		TypeUrl:     listenerType,
		Nonce:       "0",
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

	// add it and assert that we now have a ingress_http listener
	rh.OnAdd(i1)
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, &v2.Listener{
				Name:    "ingress_http",
				Address: socketaddress("0.0.0.0", 8080),
				FilterChains: []listener.FilterChain{
					filterchain(false, httpfilter("ingress_http")),
				},
			}),
		},
		TypeUrl: listenerType,
		Nonce:   "0",
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
		VersionInfo: "0",
		Resources:   []types.Any{},
		TypeUrl:     listenerType,
		Nonce:       "0",
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
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, &v2.Listener{
				Name:    "ingress_http",
				Address: socketaddress("0.0.0.0", 8080),
				FilterChains: []listener.FilterChain{
					filterchain(false, httpfilter("ingress_http")),
				},
			}),
		},
		TypeUrl: listenerType,
		Nonce:   "0",
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
		Data: map[string][]byte{
			v1.TLSCertKey:       []byte("certificate"),
			v1.TLSPrivateKeyKey: []byte("key"),
		},
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

	// assert that there are no active listeners
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources:   []types.Any{},
		TypeUrl:     listenerType,
		Nonce:       "0",
	}, streamLDS(t, cc))

	// add ingress and assert the existence of ingress_http and ingres_https
	rh.OnAdd(i1)
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, &v2.Listener{
				Name:    "ingress_http",
				Address: socketaddress("0.0.0.0", 8080),
				FilterChains: []listener.FilterChain{
					filterchain(false, httpfilter("ingress_http")),
				},
			}),
			any(t, &v2.Listener{
				Name:    "ingress_https",
				Address: socketaddress("0.0.0.0", 8443),
				FilterChains: []listener.FilterChain{
					filterchaintls([]string{"kuard.example.com"}, "certificate", "key", false, httpfilter("ingress_https")),
				},
			}),
		},
		TypeUrl: listenerType,
		Nonce:   "0",
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
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, &v2.Listener{
				Name:    "ingress_https",
				Address: socketaddress("0.0.0.0", 8443),
				FilterChains: []listener.FilterChain{
					filterchaintls([]string{"kuard.example.com"}, "certificate", "key", false, httpfilter("ingress_https")),
				},
			}),
		},
		TypeUrl: listenerType,
		Nonce:   "0",
	}, streamLDS(t, cc))

	// delete secret and assert that ingress_https is removed
	rh.OnDelete(s1)
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources:   []types.Any{},
		TypeUrl:     listenerType,
		Nonce:       "0",
	}, streamLDS(t, cc))
}

func TestIngressRouteTLSListener(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	// s1 is a tls secret
	s1 := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			v1.TLSCertKey:       []byte("certificate"),
			v1.TLSPrivateKeyKey: []byte("key"),
		},
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

	// add secret
	rh.OnAdd(s1)

	// assert that there are no active listeners
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources:   []types.Any{},
		TypeUrl:     listenerType,
		Nonce:       "0",
	}, streamLDS(t, cc))

	l1 := &v2.Listener{
		Name:    "ingress_https",
		Address: socketaddress("0.0.0.0", 8443),
		FilterChains: []listener.FilterChain{
			filterchaintls([]string{"kuard.example.com"}, "certificate", "key", false, httpfilter("ingress_https")),
		},
	}

	l1.FilterChains[0].TlsContext.CommonTlsContext.TlsParams.TlsMinimumProtocolVersion = auth.TlsParameters_TLSv1_1

	// add ingress and assert the existence of ingress_http and ingres_https
	rh.OnAdd(i1)
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, &v2.Listener{
				Name:    "ingress_http",
				Address: socketaddress("0.0.0.0", 8080),
				FilterChains: []listener.FilterChain{
					filterchain(false, httpfilter("ingress_http")),
				},
			}),
			any(t, l1),
		},
		TypeUrl: listenerType,
		Nonce:   "0",
	}, streamLDS(t, cc))

	// delete secret and assert that ingress_https is removed
	rh.OnDelete(s1)
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, &v2.Listener{
				Name:    "ingress_http",
				Address: socketaddress("0.0.0.0", 8080),
				FilterChains: []listener.FilterChain{
					filterchain(false, httpfilter("ingress_http")),
				},
			}),
		},
		TypeUrl: listenerType,
		Nonce:   "0",
	}, streamLDS(t, cc))

	rh.OnDelete(i1)
	// add secret
	rh.OnAdd(s1)
	l2 := &v2.Listener{
		Name:    "ingress_https",
		Address: socketaddress("0.0.0.0", 8443),
		FilterChains: []listener.FilterChain{
			filterchaintls([]string{"kuard.example.com"}, "certificate", "key", false, httpfilter("ingress_https")),
		},
	}

	l2.FilterChains[0].TlsContext.CommonTlsContext.TlsParams.TlsMinimumProtocolVersion = auth.TlsParameters_TLSv1_3

	// add ingress and assert the existence of ingress_http and ingres_https
	rh.OnAdd(i2)
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, &v2.Listener{
				Name:    "ingress_http",
				Address: socketaddress("0.0.0.0", 8080),
				FilterChains: []listener.FilterChain{
					filterchain(false, httpfilter("ingress_http")),
				},
			}),
			any(t, l2),
		},
		TypeUrl: listenerType,
		Nonce:   "0",
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
		Data: map[string][]byte{
			v1.TLSCertKey:       []byte("certificate"),
			v1.TLSPrivateKeyKey: []byte("key"),
		},
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

	// add ingress and fetch ingress_https
	rh.OnAdd(i1)
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, &v2.Listener{
				Name:    "ingress_https",
				Address: socketaddress("0.0.0.0", 8443),
				FilterChains: []listener.FilterChain{
					filterchaintls([]string{"kuard.example.com"}, "certificate", "key", false, httpfilter("ingress_https")),
				},
			}),
		},
		TypeUrl: listenerType,
		Nonce:   "0",
	}, streamLDS(t, cc, "ingress_https"))

	// fetch ingress_http
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{

			any(t, &v2.Listener{
				Name:    "ingress_http",
				Address: socketaddress("0.0.0.0", 8080),
				FilterChains: []listener.FilterChain{
					filterchain(false, httpfilter("ingress_http")),
				},
			}),
		},
		TypeUrl: listenerType,
		Nonce:   "0",
	}, streamLDS(t, cc, "ingress_http"))

	// fetch something non existent.
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		TypeUrl:     listenerType, Nonce: "0",
	}, streamLDS(t, cc, "HTTP"))
}

func TestLDSStreamEmpty(t *testing.T) {
	_, cc, done := setup(t)
	defer done()

	// assert that streaming LDS with no ingresses does not stall.
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		TypeUrl:     listenerType, Nonce: "0",
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
		Data: map[string][]byte{
			v1.TLSCertKey:       []byte("certificate"),
			v1.TLSPrivateKeyKey: []byte("key"),
		},
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
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, &v2.Listener{
				Name:    "ingress_https",
				Address: socketaddress("0.0.0.0", 8443),
				FilterChains: []listener.FilterChain{
					filterchaintls([]string{"kuard.example.com"}, "certificate", "key", false, httpfilter("ingress_https")),
				},
			}),
		},
		TypeUrl: listenerType,
		Nonce:   "0",
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
		Address: socketaddress("0.0.0.0", 8443),
		FilterChains: []listener.FilterChain{
			filterchaintls([]string{"kuard.example.com"}, "certificate", "key", false, httpfilter("ingress_https")),
		},
	}
	// easier to patch this up than add more params to filterchaintls
	l1.FilterChains[0].TlsContext.CommonTlsContext.TlsParams.TlsMinimumProtocolVersion = auth.TlsParameters_TLSv1_3

	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, l1),
		},
		TypeUrl: listenerType,
		Nonce:   "0",
	}, streamLDS(t, cc, "ingress_https"))
}

func TestLDSIngressHTTPUseProxyProtocol(t *testing.T) {
	rh, cc, done := setup(t, func(reh *contour.ResourceEventHandler) {
		reh.Notifier.(*contour.CacheHandler).UseProxyProto = true
	})
	defer done()

	// assert that without any ingress objects registered
	// there are no active listeners
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources:   []types.Any{},
		TypeUrl:     listenerType,
		Nonce:       "0",
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

	// add it and assert that we now have a ingress_http listener using
	// the proxy protocol (the true param to filterchain)
	rh.OnAdd(i1)
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, &v2.Listener{
				Name:    "ingress_http",
				Address: socketaddress("0.0.0.0", 8080),
				FilterChains: []listener.FilterChain{
					filterchain(true, httpfilter("ingress_http")),
				},
			}),
		},
		TypeUrl: listenerType,
		Nonce:   "0",
	}, streamLDS(t, cc))
}

func TestLDSIngressHTTPSUseProxyProtocol(t *testing.T) {
	rh, cc, done := setup(t, func(reh *contour.ResourceEventHandler) {
		reh.Notifier.(*contour.CacheHandler).UseProxyProto = true
	})
	defer done()

	// s1 is a tls secret
	s1 := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			v1.TLSCertKey:       []byte("certificate"),
			v1.TLSPrivateKeyKey: []byte("key"),
		},
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

	// assert that there are no active listeners
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources:   []types.Any{},
		TypeUrl:     listenerType,
		Nonce:       "0",
	}, streamLDS(t, cc))

	// add ingress and assert the existence of ingress_http and ingres_https and both
	// are using proxy protocol
	rh.OnAdd(i1)

	ingress_https := &v2.Listener{
		Name:    "ingress_https",
		Address: socketaddress("0.0.0.0", 8443),
		FilterChains: []listener.FilterChain{
			filterchaintls([]string{"kuard.example.com"}, "certificate", "key", false, httpfilter("ingress_https")),
		},
	}
	ingress_https.FilterChains[0].UseProxyProto = &types.BoolValue{Value: true}
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, &v2.Listener{
				Name:    "ingress_http",
				Address: socketaddress("0.0.0.0", 8080),
				FilterChains: []listener.FilterChain{
					filterchain(true, httpfilter("ingress_http")),
				},
			}),
			any(t, ingress_https),
		},
		TypeUrl: listenerType,
		Nonce:   "0",
	}, streamLDS(t, cc))
}

func TestLDSCustomAddressAndPort(t *testing.T) {
	rh, cc, done := setup(t, func(reh *contour.ResourceEventHandler) {
		reh.Notifier.(*contour.CacheHandler).HTTPAddress = "127.0.0.100"
		reh.Notifier.(*contour.CacheHandler).HTTPPort = 9100
		reh.Notifier.(*contour.CacheHandler).HTTPSAddress = "127.0.0.200"
		reh.Notifier.(*contour.CacheHandler).HTTPSPort = 9200
	})
	defer done()

	// s1 is a tls secret
	s1 := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			v1.TLSCertKey:       []byte("certificate"),
			v1.TLSPrivateKeyKey: []byte("key"),
		},
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

	// assert that there are no active listeners
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources:   []types.Any{},
		TypeUrl:     listenerType,
		Nonce:       "0",
	}, streamLDS(t, cc))

	// add ingress and assert the existence of ingress_http and ingres_https and both
	// are using proxy protocol
	rh.OnAdd(i1)

	ingress_http := &v2.Listener{
		Name:    "ingress_http",
		Address: socketaddress("127.0.0.100", 9100),
		FilterChains: []listener.FilterChain{
			filterchain(false, httpfilter("ingress_http")),
		},
	}
	ingress_https := &v2.Listener{
		Name:    "ingress_https",
		Address: socketaddress("127.0.0.200", 9200),
		FilterChains: []listener.FilterChain{
			filterchaintls([]string{"kuard.example.com"}, "certificate", "key", false, httpfilter("ingress_https")),
		},
	}
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, ingress_http),
			any(t, ingress_https),
		},
		TypeUrl: listenerType,
		Nonce:   "0",
	}, streamLDS(t, cc))
}

func TestLDSIngressRouteInsideRootNamespaces(t *testing.T) {
	rh, cc, done := setup(t, func(reh *contour.ResourceEventHandler) {
		reh.IngressRouteRootNamespaces = []string{"roots"}
		reh.Notifier.(*contour.CacheHandler).IngressRouteStatus = &k8s.IngressRouteStatus{
			Client: fake.NewSimpleClientset(),
		}
	})
	defer done()

	// assert that there are no active listeners
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources:   []types.Any{},
		TypeUrl:     listenerType,
		Nonce:       "0",
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

	// add ingressroute
	rh.OnAdd(ir1)

	// assert there is an active listener
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, &v2.Listener{
				Name:    "ingress_http",
				Address: socketaddress("0.0.0.0", 8080),
				FilterChains: []listener.FilterChain{
					filterchain(false, httpfilter("ingress_http")),
				},
			}),
		},
		TypeUrl: listenerType,
		Nonce:   "0",
	}, streamLDS(t, cc))
}

func TestLDSIngressRouteOutsideRootNamespaces(t *testing.T) {
	rh, cc, done := setup(t, func(reh *contour.ResourceEventHandler) {
		reh.IngressRouteRootNamespaces = []string{"roots"}
		reh.Notifier.(*contour.CacheHandler).IngressRouteStatus = &k8s.IngressRouteStatus{
			Client: fake.NewSimpleClientset(),
		}
	})
	defer done()

	// assert that there are no active listeners
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources:   []types.Any{},
		TypeUrl:     listenerType,
		Nonce:       "0",
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

	// assert that there are no active listeners
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources:   []types.Any{},
		TypeUrl:     listenerType,
		Nonce:       "0",
	}, streamLDS(t, cc))
}

func TestIngressRouteHTTPS(t *testing.T) {
	rh, cc, done := setup(t, func(reh *contour.ResourceEventHandler) {
		reh.IngressRouteRootNamespaces = []string{}
		reh.Notifier.(*contour.CacheHandler).IngressRouteStatus = &k8s.IngressRouteStatus{
			Client: fake.NewSimpleClientset(),
		}
	})
	defer done()

	// assert that there are no active listeners
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources:   []types.Any{},
		TypeUrl:     listenerType,
		Nonce:       "0",
	}, streamLDS(t, cc))

	// s1 is a tls secret
	s1 := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			v1.TLSCertKey:       []byte("certificate"),
			v1.TLSPrivateKeyKey: []byte("key"),
		},
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

	// add secret
	rh.OnAdd(s1)

	// add ingressroute
	rh.OnAdd(ir1)

	ingressHTTP := &v2.Listener{
		Name:    "ingress_http",
		Address: socketaddress("0.0.0.0", 8080),
		FilterChains: []listener.FilterChain{
			filterchain(false, httpfilter("ingress_http")),
		},
	}

	ingressHTTPS := &v2.Listener{
		Name:    "ingress_https",
		Address: socketaddress("0.0.0.0", 8443),
		FilterChains: []listener.FilterChain{
			filterchaintls([]string{"example.com"}, "certificate", "key", false, httpfilter("ingress_https")),
		},
	}
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, ingressHTTP),
			any(t, ingressHTTPS),
		},
		TypeUrl: listenerType,
		Nonce:   "0",
	}, streamLDS(t, cc))
}

func streamLDS(t *testing.T, cc *grpc.ClientConn, rn ...string) *v2.DiscoveryResponse {
	t.Helper()
	rds := v2.NewListenerDiscoveryServiceClient(cc)
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	st, err := rds.StreamListeners(ctx)
	if err != nil {
		t.Fatal(err)
	}
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

func socketaddress(address string, port uint32) core.Address {
	return core.Address{
		Address: &core.Address_SocketAddress{
			SocketAddress: &core.SocketAddress{
				Protocol: core.TCP,
				Address:  address,
				PortSpecifier: &core.SocketAddress_PortValue{
					PortValue: port,
				},
			},
		},
	}
}

func filterchain(useproxy bool, filters ...listener.Filter) listener.FilterChain {
	fc := listener.FilterChain{
		Filters: filters,
	}
	if useproxy {
		fc.UseProxyProto = &types.BoolValue{Value: true}
	}
	return fc
}

func filterchaintls(domains []string, cert, key string, useproxy bool, filters ...listener.Filter) listener.FilterChain {
	fc := filterchain(useproxy, filters...)
	fc.FilterChainMatch = &listener.FilterChainMatch{
		SniDomains: domains,
	}
	fc.TlsContext = &auth.DownstreamTlsContext{
		CommonTlsContext: &auth.CommonTlsContext{
			TlsParams: &auth.TlsParameters{
				TlsMinimumProtocolVersion: auth.TlsParameters_TLSv1_1,
			},
			TlsCertificates: []*auth.TlsCertificate{{
				CertificateChain: &core.DataSource{
					Specifier: &core.DataSource_InlineBytes{
						InlineBytes: []byte(cert),
					},
				},
				PrivateKey: &core.DataSource{
					Specifier: &core.DataSource_InlineBytes{
						InlineBytes: []byte(key),
					},
				},
			}},
			AlpnProtocols: []string{"h2", "http/1.1"},
		},
	}
	return fc
}

func httpfilter(routename string) listener.Filter {
	return listener.Filter{
		Name: "envoy.http_connection_manager",
		Config: messageToStruct(&envoy_config_v2_http_conn_mgr.HttpConnectionManager{
			StatPrefix: routename,
			RouteSpecifier: &envoy_config_v2_http_conn_mgr.HttpConnectionManager_Rds{
				Rds: &envoy_config_v2_http_conn_mgr.Rds{
					ConfigSource: core.ConfigSource{
						ConfigSourceSpecifier: &core.ConfigSource_ApiConfigSource{
							ApiConfigSource: &core.ApiConfigSource{
								ApiType:      core.ApiConfigSource_GRPC,
								ClusterNames: []string{"contour"},
								GrpcServices: []*core.GrpcService{{
									TargetSpecifier: &core.GrpcService_EnvoyGrpc_{
										EnvoyGrpc: &core.GrpcService_EnvoyGrpc{
											ClusterName: "contour",
										},
									},
								}},
							},
						},
					},
					RouteConfigName: routename,
				},
			},
			AccessLog: []*accesslog.AccessLog{{
				Name:   "envoy.file_access_log",
				Config: messageToStruct(fileAccessLog("/dev/stdout")),
			}},
			UseRemoteAddress: &types.BoolValue{Value: true},
			HttpFilters: []*envoy_config_v2_http_conn_mgr.HttpFilter{
				{Name: "envoy.grpc_web"},
				{Name: "envoy.router"},
			},
		}),
	}
}

// messageToStruct encodes a protobuf Message into a Struct.
// Hilariously, it uses JSON as the intermediary.
// author:glen@turbinelabs.io
func messageToStruct(msg proto.Message) *types.Struct {
	buf := &bytes.Buffer{}
	if err := (&jsonpb.Marshaler{OrigName: true}).Marshal(buf, msg); err != nil {
		panic(err)
	}

	pbs := &types.Struct{}
	if err := jsonpb.Unmarshal(buf, pbs); err != nil {
		panic(err)
	}

	return pbs
}
