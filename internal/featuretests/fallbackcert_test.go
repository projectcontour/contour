// Copyright © 2020 VMware
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

	projcontour "github.com/projectcontour/contour/apis/projectcontour/v1"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/projectcontour/contour/internal/contour"
	"github.com/projectcontour/contour/internal/envoy"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestFallbackCertificate(t *testing.T) {
	rh, c, done := setupWithFallbackCert(t, "fallbacksecret", "admin")
	defer done()

	sec1 := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret",
			Namespace: "default",
		},
		Type: "kubernetes.io/tls",
		Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
	}
	rh.OnAdd(sec1)

	fallbackSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "fallbacksecret",
			Namespace: "admin",
		},
		Type: "kubernetes.io/tls",
		Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
	}

	rh.OnAdd(fallbackSecret)

	s1 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "backend",
			Namespace: sec1.Namespace,
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:     "http",
				Protocol: "TCP",
				Port:     80,
			}},
		},
	}
	rh.OnAdd(s1)

	// Valid HTTPProxy without FallbackCertificate enabled
	proxy1 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: s1.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "fallback.example.com",
				TLS: &projcontour.TLS{
					SecretName:                "secret",
					EnableFallbackCertificate: false,
				},
			},
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: s1.Name,
					Port: 80,
				}},
			}},
		},
	}
	rh.OnAdd(proxy1)

	c.Request(listenerType, "ingress_https").Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			&v2.Listener{
				Name:    "ingress_https",
				Address: envoy.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: envoy.ListenerFilters(
					envoy.TLSInspector(),
				),
				FilterChains: filterchaintls("fallback.example.com", sec1,
					envoy.HTTPConnectionManagerBuilder().
						RouteConfigName("https/fallback.example.com").
						MetricsPrefix(contour.ENVOY_HTTPS_LISTENER).
						AccessLoggers(envoy.FileAccessLogEnvoy("/dev/stdout")).
						Get(),
					nil,
					"h2", "http/1.1"),
			},
		),
		TypeUrl: listenerType,
	})

	// Valid HTTPProxy with FallbackCertificate enabled
	proxy2 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: s1.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "fallback.example.com",
				TLS: &projcontour.TLS{
					SecretName:                "secret",
					EnableFallbackCertificate: true,
				},
			},
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: s1.Name,
					Port: 80,
				}},
			}},
		},
	}

	rh.OnUpdate(proxy1, proxy2)

	c.Request(listenerType, "ingress_https").Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			&v2.Listener{
				Name:    "ingress_https",
				Address: envoy.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: envoy.ListenerFilters(
					envoy.TLSInspector(),
				),
				FilterChains: filterchaintlsfallback("fallback.example.com", sec1, fallbackSecret,
					envoy.HTTPConnectionManagerBuilder().
						RouteConfigName("https/fallback.example.com").
						MetricsPrefix(contour.ENVOY_HTTPS_LISTENER).
						AccessLoggers(envoy.FileAccessLogEnvoy("/dev/stdout")).
						Get(),
					nil,
					"h2", "http/1.1"),
			},
		),
		TypeUrl: listenerType,
	})

	//// InvValid HTTPProxy with FallbackCertificate enabled along with ClientValidation
	//proxy3 := &projcontour.HTTPProxy{
	//	ObjectMeta: metav1.ObjectMeta{
	//		Name:      "simple",
	//		Namespace: s1.Namespace,
	//	},
	//	Spec: projcontour.HTTPProxySpec{
	//		VirtualHost: &projcontour.VirtualHost{
	//			Fqdn: "fallback.example.com",
	//			TLS: &projcontour.TLS{
	//				SecretName:                "secret",
	//				EnableFallbackCertificate: true,
	//				ClientValidation: &projcontour.DownstreamValidation{
	//					CACertificate: "something",
	//				},
	//			},
	//		},
	//		Routes: []projcontour.Route{{
	//			Services: []projcontour.Service{{
	//				Name: s1.Name,
	//				Port: 80,
	//			}},
	//		}},
	//	},
	//}
	//
	//rh.OnUpdate(proxy2, proxy3)
	//
	//c.Request(listenerType, "ingress_https").Equals(&v2.DiscoveryResponse{
	//	Resources: nil,
	//	TypeUrl:   listenerType,
	//})

	// Valid HTTPProxy with FallbackCertificate enabled
	proxy4 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple-two",
			Namespace: s1.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "anotherfallback.example.com",
				TLS: &projcontour.TLS{
					SecretName:                "secret",
					EnableFallbackCertificate: true,
				},
			},
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: s1.Name,
					Port: 80,
				}},
			}},
		},
	}

	rh.OnAdd(proxy4)

	c.Request(listenerType, "ingress_https").Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			&v2.Listener{
				Name:    "ingress_https",
				Address: envoy.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: envoy.ListenerFilters(
					envoy.TLSInspector(),
				),
				FilterChains: append(filterchaintls("anotherfallback.example.com", sec1,
					envoy.HTTPConnectionManagerBuilder().
						RouteConfigName("https/anotherfallback.example.com").
						MetricsPrefix(contour.ENVOY_HTTPS_LISTENER).
						AccessLoggers(envoy.FileAccessLogEnvoy("/dev/stdout")).
						Get(),
					nil,
					"h2", "http/1.1"), filterchaintlsfallback("fallback.example.com", sec1, fallbackSecret,
					envoy.HTTPConnectionManagerBuilder().
						RouteConfigName("https/fallback.example.com").
						MetricsPrefix(contour.ENVOY_HTTPS_LISTENER).
						AccessLoggers(envoy.FileAccessLogEnvoy("/dev/stdout")).
						Get(),
					nil,
					"h2", "http/1.1")...),
			},
		),
		TypeUrl: listenerType,
	})

	rh.OnDelete(fallbackSecret)

	c.Request(listenerType, "ingress_https").Equals(&v2.DiscoveryResponse{
		Resources: nil,
		TypeUrl:   listenerType,
	})
}
