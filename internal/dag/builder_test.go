// Copyright © 2017 Heptio
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

package dag

import (
	"fmt"
	"testing"
	"time"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	"github.com/google/go-cmp/cmp"
	ingressroutev1 "github.com/heptio/contour/apis/contour/v1beta1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestDAGInsert(t *testing.T) {
	// The DAG is sensitive to ordering, adding an ingress, then a service,
	// should have the same result as adding a service, then an ingress.

	sec1 := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret",
			Namespace: "default",
		},
		Type: v1.SecretTypeTLS,
		Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
	}

	// Invalid cert in the secret
	sec2 := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret",
			Namespace: "default",
		},
		Type: v1.SecretTypeTLS,
		Data: secretdata("wrong", "wronger"),
	}

	// weird secret with a blank ca.crt that
	// cert manager creates. #1644
	sec3 := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret",
			Namespace: "default",
		},
		Type: v1.SecretTypeTLS,
		Data: map[string][]byte{
			"ca.crt":            []byte(""),
			v1.TLSCertKey:       []byte(CERTIFICATE),
			v1.TLSPrivateKeyKey: []byte(RSA_PRIVATE_KEY),
		},
	}

	cert1 := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ca",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"ca.crt": []byte(CERTIFICATE),
		},
	}

	i1 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: v1beta1.IngressSpec{
			Backend: backend("kuard", intstr.FromInt(8080))},
	}
	i1a := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
			Annotations: map[string]string{
				"kubernetes.io/ingress.allow-http": "false",
			},
		},
		Spec: v1beta1.IngressSpec{
			Backend: backend("kuard", intstr.FromInt(8080))},
	}

	// i2 is functionally identical to i1
	i2 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: v1beta1.IngressSpec{
			Rules: []v1beta1.IngressRule{{
				IngressRuleValue: ingressrulevalue(backend("kuard", intstr.FromInt(8080))),
			}},
		},
	}

	// i2a is missing a http key from the spec.rule.
	// see issue 606
	i2a := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: v1beta1.IngressSpec{
			Rules: []v1beta1.IngressRule{{
				Host: "test1.test.com",
			}},
		},
	}

	// i3 is similar to i2 but includes a hostname on the ingress rule
	i3 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: v1beta1.IngressSpec{
			TLS: []v1beta1.IngressTLS{{
				Hosts:      []string{"kuard.example.com"},
				SecretName: "secret",
			}},
			Rules: []v1beta1.IngressRule{{
				Host:             "kuard.example.com",
				IngressRuleValue: ingressrulevalue(backend("kuard", intstr.FromInt(8080))),
			}},
		},
	}
	// i4 is like i1 except it uses a named service port
	i4 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: v1beta1.IngressSpec{
			Backend: backend("kuard", intstr.FromString("http"))},
	}
	// i5 is functionally identical to i2
	i5 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: v1beta1.IngressSpec{
			Rules: []v1beta1.IngressRule{{
				IngressRuleValue: ingressrulevalue(backend("kuard", intstr.FromString("http"))),
			}},
		},
	}
	// i6 contains two named vhosts which point to the same service
	// one of those has TLS
	i6 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "two-vhosts",
			Namespace: "default",
		},
		Spec: v1beta1.IngressSpec{
			TLS: []v1beta1.IngressTLS{{
				Hosts:      []string{"b.example.com"},
				SecretName: "secret",
			}},
			Rules: []v1beta1.IngressRule{{
				Host:             "a.example.com",
				IngressRuleValue: ingressrulevalue(backend("kuard", intstr.FromInt(8080))),
			}, {
				Host:             "b.example.com",
				IngressRuleValue: ingressrulevalue(backend("kuard", intstr.FromString("http"))),
			}},
		},
	}
	i6a := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "two-vhosts",
			Namespace: "default",
			Annotations: map[string]string{
				"kubernetes.io/ingress.allow-http": "false",
			},
		},
		Spec: v1beta1.IngressSpec{
			TLS: []v1beta1.IngressTLS{{
				Hosts:      []string{"b.example.com"},
				SecretName: "secret",
			}},
			Rules: []v1beta1.IngressRule{{
				Host:             "a.example.com",
				IngressRuleValue: ingressrulevalue(backend("kuard", intstr.FromInt(8080))),
			}, {
				Host:             "b.example.com",
				IngressRuleValue: ingressrulevalue(backend("kuard", intstr.FromString("http"))),
			}},
		},
	}
	i6b := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "two-vhosts",
			Namespace: "default",
			Annotations: map[string]string{
				"ingress.kubernetes.io/force-ssl-redirect": "true",
			},
		},
		Spec: v1beta1.IngressSpec{
			TLS: []v1beta1.IngressTLS{{
				Hosts:      []string{"b.example.com"},
				SecretName: "secret",
			}},
			Rules: []v1beta1.IngressRule{{
				Host:             "b.example.com",
				IngressRuleValue: ingressrulevalue(backend("kuard", intstr.FromString("http"))),
			}},
		},
	}
	i6c := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "two-vhosts",
			Namespace: "default",
			Annotations: map[string]string{
				"ingress.kubernetes.io/force-ssl-redirect": "true",
				"kubernetes.io/ingress.allow-http":         "false",
			},
		},
		Spec: v1beta1.IngressSpec{
			TLS: []v1beta1.IngressTLS{{
				Hosts:      []string{"b.example.com"},
				SecretName: "secret",
			}},
			Rules: []v1beta1.IngressRule{{
				Host:             "b.example.com",
				IngressRuleValue: ingressrulevalue(backend("kuard", intstr.FromString("http"))),
			}},
		},
	}

	// i7 contains a single vhost with two paths
	i7 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "two-paths",
			Namespace: "default",
		},
		Spec: v1beta1.IngressSpec{
			TLS: []v1beta1.IngressTLS{{
				Hosts:      []string{"b.example.com"},
				SecretName: "secret",
			}},
			Rules: []v1beta1.IngressRule{{
				Host: "b.example.com",
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Backend: v1beta1.IngressBackend{
								ServiceName: "kuard",
								ServicePort: intstr.FromString("http"),
							},
						}, {
							Path: "/kuarder",
							Backend: v1beta1.IngressBackend{
								ServiceName: "kuarder",
								ServicePort: intstr.FromInt(8080),
							},
						}},
					},
				},
			}},
		},
	}

	// i8 is identical to i7 but uses multiple IngressRules
	i8 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "two-rules",
			Namespace: "default",
		},
		Spec: v1beta1.IngressSpec{
			TLS: []v1beta1.IngressTLS{{
				Hosts:      []string{"b.example.com"},
				SecretName: "secret",
			}},
			Rules: []v1beta1.IngressRule{{
				Host: "b.example.com",
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Backend: v1beta1.IngressBackend{
								ServiceName: "kuard",
								ServicePort: intstr.FromString("http"),
							},
						}},
					},
				},
			}, {
				Host: "b.example.com",
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Path: "/kuarder",
							Backend: v1beta1.IngressBackend{
								ServiceName: "kuarder",
								ServicePort: intstr.FromInt(8080),
							},
						}},
					},
				},
			}},
		},
	}
	// i9 is identical to i8 but disables non TLS connections
	i9 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "two-rules",
			Namespace: "default",
			Annotations: map[string]string{
				"kubernetes.io/ingress.allow-http": "false",
			},
		},
		Spec: v1beta1.IngressSpec{
			TLS: []v1beta1.IngressTLS{{
				Hosts:      []string{"b.example.com"},
				SecretName: "secret",
			}},
			Rules: []v1beta1.IngressRule{{
				Host: "b.example.com",
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Backend: v1beta1.IngressBackend{
								ServiceName: "kuard",
								ServicePort: intstr.FromString("http"),
							},
						}},
					},
				},
			}, {
				Host: "b.example.com",
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Path: "/kuarder",
							Backend: v1beta1.IngressBackend{
								ServiceName: "kuarder",
								ServicePort: intstr.FromInt(8080),
							},
						}},
					},
				},
			}},
		},
	}

	// i10 specifies a minimum tls version
	i10 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "two-rules",
			Namespace: "default",
			Annotations: map[string]string{
				"contour.heptio.com/tls-minimum-protocol-version": "1.3",
			},
		},
		Spec: v1beta1.IngressSpec{
			TLS: []v1beta1.IngressTLS{{
				Hosts:      []string{"b.example.com"},
				SecretName: "secret",
			}},
			Rules: []v1beta1.IngressRule{{
				Host: "b.example.com",
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Backend: v1beta1.IngressBackend{
								ServiceName: "kuard",
								ServicePort: intstr.FromString("http"),
							},
						}},
					},
				},
			}},
		},
	}

	// i11 has a websocket route
	i11 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "websocket",
			Namespace: "default",
			Annotations: map[string]string{
				"contour.heptio.com/websocket-routes": "/ws1 , /ws2",
			},
		},
		Spec: v1beta1.IngressSpec{
			Rules: []v1beta1.IngressRule{{
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Backend: v1beta1.IngressBackend{
								ServiceName: "kuard",
								ServicePort: intstr.FromString("http"),
							},
						}, {
							Path: "/ws1",
							Backend: v1beta1.IngressBackend{
								ServiceName: "kuard",
								ServicePort: intstr.FromString("http"),
							},
						}},
					},
				},
			}},
		},
	}

	// i12a has an invalid timeout
	i12a := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "timeout",
			Namespace: "default",
			Annotations: map[string]string{
				"contour.heptio.com/request-timeout": "peanut",
			},
		},
		Spec: v1beta1.IngressSpec{
			Rules: []v1beta1.IngressRule{{
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Path: "/",
							Backend: v1beta1.IngressBackend{
								ServiceName: "kuard",
								ServicePort: intstr.FromString("http"),
							},
						}},
					},
				},
			}},
		},
	}

	// i12b has a reasonable timeout
	i12b := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "timeout",
			Namespace: "default",
			Annotations: map[string]string{
				"contour.heptio.com/request-timeout": "1m30s", // 90 seconds y'all
			},
		},
		Spec: v1beta1.IngressSpec{
			Rules: []v1beta1.IngressRule{{
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Path: "/",
							Backend: v1beta1.IngressBackend{
								ServiceName: "kuard",
								ServicePort: intstr.FromString("http"),
							},
						}},
					},
				},
			}},
		},
	}

	// i12c has an unreasonable timeout
	i12c := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "timeout",
			Namespace: "default",
			Annotations: map[string]string{
				"contour.heptio.com/request-timeout": "infinite",
			},
		},
		Spec: v1beta1.IngressSpec{
			Rules: []v1beta1.IngressRule{{
				IngressRuleValue: v1beta1.IngressRuleValue{HTTP: &v1beta1.HTTPIngressRuleValue{
					Paths: []v1beta1.HTTPIngressPath{{Path: "/",
						Backend: v1beta1.IngressBackend{ServiceName: "kuard",
							ServicePort: intstr.FromString("http")},
					}}},
				}}}},
	}

	// i13 a and b are a pair of ingresses for the same vhost
	// they represent a tricky way over 'overlaying' routes from one
	// ingress onto another
	i13a := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app",
			Namespace: "default",
			Annotations: map[string]string{
				"ingress.kubernetes.io/force-ssl-redirect": "true",
			},
		},
		Spec: v1beta1.IngressSpec{
			TLS: []v1beta1.IngressTLS{{
				Hosts:      []string{"example.com"},
				SecretName: "example-tls",
			}},
			Rules: []v1beta1.IngressRule{{
				Host: "example.com",
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Path: "/",
							Backend: v1beta1.IngressBackend{
								ServiceName: "app-service",
								ServicePort: intstr.FromInt(8080),
							},
						}},
					},
				},
			}},
		},
	}
	i13b := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "challenge", Namespace: "nginx-ingress"},
		Spec: v1beta1.IngressSpec{
			Rules: []v1beta1.IngressRule{{
				Host: "example.com",
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Path: "/.well-known/acme-challenge/gVJl5NWL2owUqZekjHkt_bo3OHYC2XNDURRRgLI5JTk",
							Backend: v1beta1.IngressBackend{
								ServiceName: "challenge-service",
								ServicePort: intstr.FromInt(8009),
							},
						}},
					},
				},
			}},
		},
	}

	i3a := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: v1beta1.IngressSpec{
			Rules: []v1beta1.IngressRule{{
				IngressRuleValue: ingressrulevalue(backend("kuard", intstr.FromInt(80))),
			}},
		},
	}

	i14 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "timeout",
			Namespace: "default",
			Annotations: map[string]string{
				"contour.heptio.com/retry-on":        "gateway-error",
				"contour.heptio.com/num-retries":     "6",
				"contour.heptio.com/per-try-timeout": "10s",
			},
		},
		Spec: v1beta1.IngressSpec{
			Rules: []v1beta1.IngressRule{{
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Path: "/",
							Backend: v1beta1.IngressBackend{
								ServiceName: "kuard",
								ServicePort: intstr.FromString("http"),
							},
						}},
					},
				},
			}},
		},
	}

	i15 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "regex",
			Namespace: "default",
		},
		Spec: v1beta1.IngressSpec{
			Rules: []v1beta1.IngressRule{{
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Path: "/[^/]+/invoices(/.*|/?)", // issue 1243
							Backend: v1beta1.IngressBackend{
								ServiceName: "kuard",
								ServicePort: intstr.FromString("http"),
							},
						}},
					},
				},
			}},
		},
	}

	// s3a and b have http/2 protocol annotations
	s3a := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
			Annotations: map[string]string{
				"contour.heptio.com/upstream-protocol.h2c": "80,http",
			},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8888),
			}},
		},
	}

	s3b := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
			Annotations: map[string]string{
				"contour.heptio.com/upstream-protocol.h2": "80,http",
			},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8888),
			}},
		},
	}

	s3c := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
			Annotations: map[string]string{
				"contour.heptio.com/upstream-protocol.tls": "80,http",
			},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8888),
			}},
		},
	}

	sec13 := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-tls",
			Namespace: "default",
		},
		Type: v1.SecretTypeTLS,
		Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
	}

	s13a := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-service",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}

	s13b := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "challenge-service",
			Namespace: "nginx-ingress",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       8009,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}

	ir1 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "example.com",
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

	// ir1a tcp forwards traffic to default/kuard:8080 by TLS terminating it
	// first.
	ir1a := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard-tcp",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "kuard.example.com",
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
			TCPProxy: &ingressroutev1.TCPProxy{
				Services: []ingressroutev1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			},
		},
	}

	// ir1b tcp forwards traffic to default/kuard:8080 by TLS pass-throughing
	// it.
	ir1b := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard-tcp",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "kuard.example.com",
				TLS: &ingressroutev1.TLS{
					Passthrough: true,
				},
			},
			TCPProxy: &ingressroutev1.TCPProxy{
				Services: []ingressroutev1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			},
		},
	}

	// ir1c tcp delegates to another ingress route, concretely to
	// marketing/kuard-tcp. it.
	ir1c := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard-tcp",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "kuard.example.com",
				TLS: &ingressroutev1.TLS{
					Passthrough: true,
				},
			},
			TCPProxy: &ingressroutev1.TCPProxy{
				Delegate: &ingressroutev1.Delegate{
					Name:      "kuard-tcp",
					Namespace: "marketing",
				},
			},
		},
	}

	// ir1d tcp forwards traffic to default/kuard:8080 by TLS pass-throughing
	// it.
	ir1d := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard-tcp",
			Namespace: "marketing",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			TCPProxy: &ingressroutev1.TCPProxy{
				Services: []ingressroutev1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			},
		},
	}

	ir1e := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Services: []ingressroutev1.Service{{
					Name: "kuard",
					Port: 8080,
					HealthCheck: &ingressroutev1.HealthCheck{
						Path: "/healthz",
					},
				}},
			}},
		},
	}

	// ir2 is like ir1 but refers to two backend services
	ir2 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Services: []ingressroutev1.Service{{
					Name: "kuard",
					Port: 8080,
				}, {
					Name: "kuarder",
					Port: 8080,
				}},
			}},
		},
	}

	// ir3 delegates a route to ir4
	ir3 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []ingressroutev1.Route{{
				Match: "/blog",
				Delegate: &ingressroutev1.Delegate{
					Name:      "blog",
					Namespace: "marketing",
				},
			}},
		},
	}

	// ir4 is a delegate ingressroute, and itself delegates to another one.
	ir4 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "blog",
			Namespace: "marketing",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			Routes: []ingressroutev1.Route{{
				Match: "/blog",
				Services: []ingressroutev1.Service{{
					Name: "blog",
					Port: 8080,
				}},
			}, {
				Match: "/blog/admin",
				Delegate: &ingressroutev1.Delegate{
					Name:      "marketing-admin",
					Namespace: "operations",
				},
			}},
		},
	}

	// ir5 is a delegate ingressroute
	ir5 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "marketing-admin",
			Namespace: "operations",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			Routes: []ingressroutev1.Route{{
				Match: "/blog/admin",
				Services: []ingressroutev1.Service{{
					Name: "blog-admin",
					Port: 8080,
				}},
			}},
		},
	}

	// ir6 has TLS and does not specify min tls version
	ir6 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "foo.com",
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

	// ir7 has TLS and specifies min tls version of 1.2
	ir7 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "foo.com",
				TLS: &ingressroutev1.TLS{
					SecretName:             "secret",
					MinimumProtocolVersion: "1.2",
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

	// ir8 has TLS and specifies min tls version of 1.3
	ir8 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "foo.com",
				TLS: &ingressroutev1.TLS{
					SecretName:             "secret",
					MinimumProtocolVersion: "1.3",
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

	// ir9 has TLS and specifies an invalid min tls version of 0.9999
	ir9 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "foo.com",
				TLS: &ingressroutev1.TLS{
					SecretName:             "secret",
					MinimumProtocolVersion: "0.9999",
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

	// ir10 has a websocket route
	ir10 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Services: []ingressroutev1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}, {
				Match:            "/websocket",
				EnableWebsockets: true,
				Services: []ingressroutev1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	// ir10 has a websocket route w/multiple upstreams
	ir10b := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Services: []ingressroutev1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}, {
				Match:            "/websocket",
				EnableWebsockets: true,
				Services: []ingressroutev1.Service{{
					Name: "kuard",
					Port: 8080,
				}, {
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	// ir11 has a prefix-rewrite route
	ir11 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Services: []ingressroutev1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}, {
				Match:         "/websocket",
				PrefixRewrite: "/",
				Services: []ingressroutev1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	// ir13 has two routes to the same service with different
	// weights
	ir13 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []ingressroutev1.Route{{
				Match: "/a",
				Services: []ingressroutev1.Service{{
					Name:   "kuard",
					Port:   8080,
					Weight: 90,
				}},
			}, {
				Match: "/b",
				Services: []ingressroutev1.Service{{Name: "kuard",
					Port:   8080,
					Weight: 60,
				}},
			}},
		},
	}
	// ir13a has one route to the same service with two different weights
	ir13a := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []ingressroutev1.Route{{
				Match: "/a",
				Services: []ingressroutev1.Service{{
					Name:   "kuard",
					Port:   8080,
					Weight: 90,
				}, {
					Name:   "kuard",
					Port:   8080,
					Weight: 60,
				}},
			}},
		},
	}

	// ir14 has TLS and allows insecure
	ir14 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "foo.com",
				TLS: &ingressroutev1.TLS{
					SecretName: "secret",
				},
			},
			Routes: []ingressroutev1.Route{{
				Match:          "/",
				PermitInsecure: true,
				Services: []ingressroutev1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	ir15 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "bar.com",
			},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				RetryPolicy: &ingressroutev1.RetryPolicy{
					NumRetries:    6,
					PerTryTimeout: "10s",
				},
				Services: []ingressroutev1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	ir15a := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "bar.com",
			},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				RetryPolicy: &ingressroutev1.RetryPolicy{
					NumRetries:    6,
					PerTryTimeout: "please",
				},
				Services: []ingressroutev1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	ir15b := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "bar.com",
			},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				RetryPolicy: &ingressroutev1.RetryPolicy{
					NumRetries:    0,
					PerTryTimeout: "10s",
				},
				Services: []ingressroutev1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	ir16a := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "bar.com",
			},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				TimeoutPolicy: &ingressroutev1.TimeoutPolicy{
					Request: "peanut",
				},
				Services: []ingressroutev1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	ir16b := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "bar.com",
			},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				TimeoutPolicy: &ingressroutev1.TimeoutPolicy{
					Request: "1m30s", // 90 seconds y'all
				},
				Services: []ingressroutev1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	ir16c := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "bar.com",
			},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				TimeoutPolicy: &ingressroutev1.TimeoutPolicy{
					Request: "infinite",
				},
				Services: []ingressroutev1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	ir17 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Services: []ingressroutev1.Service{{
					Name: "kuard",
					Port: 8080,
					UpstreamValidation: &ingressroutev1.UpstreamValidation{
						CACertificate: "ca",
						SubjectName:   "example.com",
					},
				}},
			}},
		},
	}

	s5 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "blog-admin",
			Namespace: "operations",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}

	s1 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}

	// s1a carries the tls annotation
	s1a := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
			Annotations: map[string]string{
				"contour.heptio.com/upstream-protocol.tls": "8080",
			},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}

	// s1b carries all four ingress annotations{
	s1b := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
			Annotations: map[string]string{
				"contour.heptio.com/max-connections":      "9000",
				"contour.heptio.com/max-pending-requests": "4096",
				"contour.heptio.com/max-requests":         "404",
				"contour.heptio.com/max-retries":          "7",
			},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}

	// s2 is like s1 but with a different name
	s2 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuarder",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}
	// s3 is like s1 but has a different port
	s3 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       9999,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}

	s4 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "blog",
			Namespace: "marketing",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}

	s6 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "marketing",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}

	s7 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "home",
			Namespace: "finance",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:     "http",
				Protocol: "TCP",
				Port:     8080,
			}},
		},
	}

	tests := map[string]struct {
		objs                  []interface{}
		disablePermitInsecure bool
		want                  []Vertex
	}{
		"insert ingress w/ default backend w/o matching service": {
			objs: []interface{}{
				i1,
			},
			want: listeners(),
		},
		"insert ingress w/ default backend": {
			objs: []interface{}{
				i1,
				s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*", prefixroute("/", httpService(s1))),
					),
				},
			),
		},
		"insert ingress w/ single unnamed backend w/o matching service": {
			objs: []interface{}{
				i2,
			},
			want: listeners(),
		},
		"insert ingress w/ single unnamed backend": {
			objs: []interface{}{
				i2,
				s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*", prefixroute("/", httpService(s1))),
					),
				},
			),
		},
		"insert ingress with missing spec.rule.http key": {
			objs: []interface{}{
				i2a,
			},
			want: listeners(),
		},
		"insert ingress w/ host name and single backend w/o matching service": {
			objs: []interface{}{
				i3,
			},
			want: listeners(),
		},
		"insert ingress w/ host name and single backend": {
			objs: []interface{}{
				i3,
				s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("kuard.example.com", prefixroute("/", httpService(s1))),
					),
				},
			),
		},
		"insert non matching service then ingress w/ default backend": {
			objs: []interface{}{
				s2,
				i1,
			},
			want: listeners(),
		},
		"insert ingress w/ default backend then matching service with wrong port": {
			objs: []interface{}{
				i1,
				s3,
			},
			want: listeners(),
		},
		"insert unnamed ingress w/ single backend then matching service with wrong port": {
			objs: []interface{}{
				i2,
				s3,
			},
			want: listeners(),
		},
		"insert ingress w/ default backend then matching service w/ named port": {
			objs: []interface{}{
				i4,
				s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*", prefixroute("/", httpService(s1))),
					),
				},
			),
		},
		"insert service w/ named port then ingress w/ default backend": {
			objs: []interface{}{
				s1,
				i4,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*", prefixroute("/", httpService(s1))),
					),
				},
			),
		},
		"insert ingress w/ single unnamed backend w/ named service port then service": {
			objs: []interface{}{
				i5,
				s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*", prefixroute("/", httpService(s1))),
					),
				},
			),
		},
		"insert service then ingress w/ single unnamed backend w/ named service port": {
			objs: []interface{}{
				s1,
				i5,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*", prefixroute("/", httpService(s1))),
					),
				},
			),
		},
		"insert secret": {
			objs: []interface{}{
				sec1,
			},
			want: listeners(),
		},
		"insert secret then ingress w/o tls": {
			objs: []interface{}{
				sec1,
				i1,
			},
			want: listeners(),
		},
		"insert service, secret then ingress w/o tls": {
			objs: []interface{}{
				s1,
				sec1,
				i1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*", prefixroute("/", httpService(s1))),
					),
				},
			),
		},
		"insert secret then ingress w/ tls": {
			objs: []interface{}{
				sec1,
				i3,
			},
			want: listeners(
				&Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						securevirtualhost("kuard.example.com", sec1),
					),
				},
			),
		},
		"insert service, secret then ingress w/ tls": {
			objs: []interface{}{
				s1,
				sec1,
				i3,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("kuard.example.com", prefixroute("/", httpService(s1))),
					),
				},
				&Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						securevirtualhost("kuard.example.com", sec1, prefixroute("/", httpService(s1))),
					),
				},
			),
		},
		"insert service w/ secret with w/ blank ca.crt": {
			objs: []interface{}{
				s1,
				sec3, // issue 1644
				i3,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("kuard.example.com", prefixroute("/", httpService(s1))),
					),
				},
				&Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						securevirtualhost("kuard.example.com", sec3, prefixroute("/", httpService(s1))),
					),
				},
			),
		},
		"insert invalid secret then ingress w/o tls": {
			objs: []interface{}{
				sec2,
				i1,
			},
			want: listeners(),
		},
		"insert service, invalid secret then ingress w/o tls": {
			objs: []interface{}{
				s1,
				sec2,
				i1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*", prefixroute("/", httpService(s1))),
					),
				},
			),
		},
		"insert invalid secret then ingress w/ tls": {
			objs: []interface{}{
				sec2,
				i3,
			},
			want: listeners(),
		},
		"insert service, invalid secret then ingress w/ tls": {
			objs: []interface{}{
				s1,
				sec2,
				i3,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("kuard.example.com", prefixroute("/", httpService(s1))),
					),
				},
			),
		},
		"insert ingress w/ two vhosts": {
			objs: []interface{}{
				i6,
			},
			want: nil, // no matching service
		},
		"insert ingress w/ two vhosts then matching service": {
			objs: []interface{}{
				i6,
				s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("a.example.com", prefixroute("/", httpService(s1))),
						virtualhost("b.example.com", prefixroute("/", httpService(s1))),
					),
				},
			),
		},
		"insert service then ingress w/ two vhosts": {
			objs: []interface{}{
				s1,
				i6,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("a.example.com", prefixroute("/", httpService(s1))),
						virtualhost("b.example.com", prefixroute("/", httpService(s1))),
					),
				},
			),
		},
		"insert ingress w/ two vhosts then service then secret": {
			objs: []interface{}{
				i6,
				s1,
				sec1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("a.example.com", prefixroute("/", httpService(s1))),
						virtualhost("b.example.com", prefixroute("/", httpService(s1))),
					),
				}, &Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						securevirtualhost("b.example.com", sec1, prefixroute("/", httpService(s1))),
					),
				},
			),
		},
		"insert service then secret then ingress w/ two vhosts": {
			objs: []interface{}{
				s1,
				sec1,
				i6,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("a.example.com", prefixroute("/", httpService(s1))),
						virtualhost("b.example.com", prefixroute("/", httpService(s1))),
					),
				}, &Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						securevirtualhost("b.example.com", sec1, prefixroute("/", httpService(s1))),
					),
				},
			),
		},
		"insert ingress w/ two paths then one service": {
			objs: []interface{}{
				i7,
				s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("b.example.com",
							prefixroute("/", httpService(s1)),
						),
					),
				},
			),
		},
		"insert ingress w/ two paths then services": {
			objs: []interface{}{
				i7,
				s2,
				s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("b.example.com",
							prefixroute("/", httpService(s1)),
							prefixroute("/kuarder", httpService(s2)),
						),
					),
				},
			),
		},
		"insert two services then ingress w/ two ingress rules": {
			objs: []interface{}{
				s1, s2, i8,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("b.example.com",
							prefixroute("/", httpService(s1)),
							prefixroute("/kuarder", httpService(s2)),
						),
					),
				},
			),
		},
		"insert ingress w/ two paths httpAllowed: false": {
			objs: []interface{}{
				i9,
			},
			want: []Vertex{},
		},
		"insert ingress w/ two paths httpAllowed: false then tls and service": {
			objs: []interface{}{
				i9,
				sec1,
				s1, s2,
			},
			want: listeners(
				&Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						securevirtualhost("b.example.com", sec1,
							prefixroute("/", httpService(s1)),
							prefixroute("/kuarder", httpService(s2)),
						),
					),
				},
			),
		},
		"insert default ingress httpAllowed: false": {
			objs: []interface{}{
				i1a,
			},
			want: []Vertex{},
		},
		"insert default ingress httpAllowed: false then tls and service": {
			objs: []interface{}{
				i1a, sec1, s1,
			},
			want: []Vertex{}, // default ingress cannot be tls
		},
		"insert ingress w/ two vhosts httpAllowed: false": {
			objs: []interface{}{
				i6a,
			},
			want: []Vertex{},
		},
		"insert ingress w/ two vhosts httpAllowed: false then tls and service": {
			objs: []interface{}{
				i6a, sec1, s1,
			},
			want: listeners(
				&Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						securevirtualhost("b.example.com", sec1, prefixroute("/", httpService(s1))),
					),
				},
			),
		},
		"insert ingress w/ force-ssl-redirect: true": {
			objs: []interface{}{
				i6b, sec1, s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("b.example.com", routeUpgrade("/", httpService(s1))),
					),
				}, &Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						securevirtualhost("b.example.com", sec1, routeUpgrade("/", httpService(s1))),
					),
				},
			),
		},

		"insert ingress w/ force-ssl-redirect: true and allow-http: false": {
			objs: []interface{}{
				i6c, sec1, s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("b.example.com", routeUpgrade("/", httpService(s1))),
					),
				}, &Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						securevirtualhost("b.example.com", sec1, routeUpgrade("/", httpService(s1))),
					),
				},
			),
		},
		"insert ingressroute": {
			objs: []interface{}{
				ir1, s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", prefixroute("/", httpService(s1))),
					),
				},
			),
		},
		"insert ingressroute w/ healthcheck": {
			objs: []interface{}{
				ir1e, s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							routeCluster("/", &Cluster{
								Upstream: httpService(s1),
								HealthCheck: &ingressroutev1.HealthCheck{
									Path: "/healthz",
								},
							}),
						),
					),
				},
			),
		},

		"insert ingressroute with websocket route": {
			objs: []interface{}{
				ir11, s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							prefixroute("/", httpService(s1)),
							routeRewrite("/websocket", "/", httpService(s1)),
						),
					),
				},
			),
		},
		"insert ingressroute with tcp forward with TLS termination": {
			objs: []interface{}{
				ir1a, s1, sec1,
			},
			want: listeners(
				&Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "kuard.example.com",
								TCPProxy: &TCPProxy{
									Clusters: clusters(
										tcpService(s1),
									),
								},
							},
							Secret:          secret(sec1),
							MinProtoVersion: auth.TlsParameters_TLSv1_1,
						},
					),
				},
			),
		},
		"insert ingressroute with tcp forward without TLS termination w/ passthrough": {
			objs: []interface{}{
				ir1b, s1,
			},
			want: listeners(
				&Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "kuard.example.com",
								TCPProxy: &TCPProxy{
									Clusters: clusters(
										tcpService(s1),
									),
								},
							},
						},
					),
				},
			),
		},

		"insert root ingress route and delegate ingress route for a tcp proxy": {
			objs: []interface{}{
				ir1d, s6, ir1c,
			},
			want: listeners(
				&Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "kuard.example.com",
								TCPProxy: &TCPProxy{
									Clusters: clusters(
										tcpService(s6),
									),
								},
							},
						},
					),
				},
			),
		},
		"insert ingressroute with prefix rewrite route": {
			objs: []interface{}{
				ir10, s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							prefixroute("/", httpService(s1)),
							routeWebsocket("/websocket", httpService(s1)),
						),
					),
				},
			),
		},
		"insert ingressroute with multiple upstreams prefix rewrite route, websocket routes are dropped": {
			objs: []interface{}{
				ir10b, s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							prefixroute("/", httpService(s1)),
						),
					),
				},
			),
		},
		"insert ingressroute and service": {
			objs: []interface{}{
				ir1, s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", prefixroute("/", httpService(s1))),
					),
				},
			),
		},
		"insert ingressroute without tls version": {
			objs: []interface{}{
				ir6, s1, sec1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("foo.com", routeUpgrade("/", httpService(s1))),
					),
				}, &Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						securevirtualhost("foo.com", sec1, routeUpgrade("/", httpService(s1))),
					),
				},
			),
		},
		"insert ingressroute with TLS one insecure": {
			objs: []interface{}{
				ir14, s1, sec1,
			},
			disablePermitInsecure: false,
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("foo.com", prefixroute("/", httpService(s1))),
					),
				}, &Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						securevirtualhost("foo.com", sec1, prefixroute("/", httpService(s1))),
					),
				},
			),
		},
		"insert ingressroute with TLS one insecure - disablePermitInsecure=true": {
			objs: []interface{}{
				ir14, s1, sec1,
			},
			disablePermitInsecure: true,
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("foo.com", routeUpgrade("/", httpService(s1))),
					),
				}, &Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						securevirtualhost("foo.com", sec1, routeUpgrade("/", httpService(s1))),
					),
				},
			),
		},
		"insert ingressroute with tls version 1.2": {
			objs: []interface{}{
				ir7, s1, sec1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("foo.com", routeUpgrade("/", httpService(s1))),
					),
				}, &Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "foo.com",
								routes: routes(
									routeUpgrade("/", httpService(s1)),
								),
							},
							MinProtoVersion: auth.TlsParameters_TLSv1_2,
							Secret:          secret(sec1),
						},
					),
				},
			),
		},
		"insert ingressroute with tls version 1.3": {
			objs: []interface{}{
				ir8, s1, sec1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("foo.com", routeUpgrade("/", httpService(s1))),
					),
				}, &Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "foo.com",
								routes: routes(
									routeUpgrade("/", httpService(s1)),
								),
							},
							MinProtoVersion: auth.TlsParameters_TLSv1_3,
							Secret:          secret(sec1),
						},
					),
				},
			),
		},
		"insert ingressroute with invalid tls version": {
			objs: []interface{}{
				ir9, s1, sec1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("foo.com", routeUpgrade("/", httpService(s1))),
					),
				}, &Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						securevirtualhost("foo.com", sec1, routeUpgrade("/", httpService(s1))),
					),
				},
			),
		},
		"insert ingressroute referencing two backends, one missing": {
			objs: []interface{}{
				ir2, s2,
			},
			want: listeners(),
		},
		"insert ingressroute referencing two backends": {
			objs: []interface{}{
				ir2, s1, s2,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", prefixroute("/", httpService(s1), httpService(s2))),
					),
				},
			),
		},
		"insert ingress w/ tls min proto annotation": {
			objs: []interface{}{
				i10,
				sec1,
				s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("b.example.com", prefixroute("/", httpService(s1))),
					),
				}, &Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "b.example.com",
								routes: routes(
									prefixroute("/", httpService(s1)),
								),
							},
							MinProtoVersion: auth.TlsParameters_TLSv1_3,
							Secret:          secret(sec1),
						},
					),
				},
			),
		},
		"insert ingress w/ websocket route annotation": {
			objs: []interface{}{
				i11,
				s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*",
							prefixroute("/", httpService(s1)),
							routeWebsocket("/ws1", httpService(s1)),
						),
					),
				},
			),
		},
		"insert ingress w/ invalid timeout annotation": {
			objs: []interface{}{
				i12a,
				s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*", &PrefixRoute{
							Prefix: "/",
							Route: Route{
								Clusters: clustermap(s1),
								TimeoutPolicy: &TimeoutPolicy{
									Timeout: -1, // invalid timeout equals infinity ¯\_(ツ)_/¯.
								},
							},
						}),
					),
				},
			),
		},
		"insert ingressroute w/ invalid timeoutpolicy": {
			objs: []interface{}{
				ir16a,
				s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("bar.com", &PrefixRoute{
							Prefix: "/",
							Route: Route{
								Clusters: clustermap(s1),
								TimeoutPolicy: &TimeoutPolicy{
									Timeout: -1, // invalid timeout equals infinity ¯\_(ツ)_/¯.
								},
							},
						}),
					),
				},
			),
		},
		"insert ingress w/ valid timeout annotation": {
			objs: []interface{}{
				i12b,
				s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*", &PrefixRoute{
							Prefix: "/",
							Route: Route{
								Clusters: clustermap(s1),
								TimeoutPolicy: &TimeoutPolicy{
									Timeout: 90 * time.Second,
								},
							},
						}),
					),
				},
			),
		},
		"insert ingressroute w/ valid timeoutpolicy": {
			objs: []interface{}{
				ir16b,
				s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("bar.com", &PrefixRoute{
							Prefix: "/",
							Route: Route{
								Clusters: clustermap(s1),
								TimeoutPolicy: &TimeoutPolicy{
									Timeout: 90 * time.Second,
								},
							},
						}),
					),
				},
			),
		},
		"insert ingress w/ infinite timeout annotation": {
			objs: []interface{}{
				i12c,
				s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*", &PrefixRoute{
							Prefix: "/",
							Route: Route{
								Clusters: clustermap(s1),
								TimeoutPolicy: &TimeoutPolicy{
									Timeout: -1,
								},
							},
						}),
					),
				},
			),
		},
		"insert ingressroute w/ infinite timeoutpolicy": {
			objs: []interface{}{
				ir16c,
				s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("bar.com", &PrefixRoute{
							Prefix: "/",
							Route: Route{
								Clusters: clustermap(s1),
								TimeoutPolicy: &TimeoutPolicy{
									Timeout: -1,
								},
							},
						}),
					),
				},
			),
		},
		"insert ingressroute w/ missing tls annotation": {
			objs: []interface{}{
				cert1, ir17, s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							prefixroute("/", httpService(s1)),
						),
					),
				},
			),
		},
		"insert ingressroute w/ missing certificate": {
			objs: []interface{}{
				ir17, s1a,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							routeCluster("/",
								&Cluster{
									Upstream: &HTTPService{
										TCPService: TCPService{
											Name:        s1a.Name,
											Namespace:   s1a.Namespace,
											ServicePort: &s1a.Spec.Ports[0],
										},
										Protocol: "tls",
									},
								},
							),
						),
					),
				},
			),
		},
		"insert ingressroute expecting verification": {
			objs: []interface{}{
				cert1, ir17, s1a,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							routeCluster("/",
								&Cluster{
									Upstream: &HTTPService{
										TCPService: TCPService{
											Name:        s1a.Name,
											Namespace:   s1a.Namespace,
											ServicePort: &s1a.Spec.Ports[0],
										},
										Protocol: "tls",
									},
									UpstreamValidation: &UpstreamValidation{
										CACertificate: secret(cert1),
										SubjectName:   "example.com",
									},
								},
							),
						),
					),
				},
			),
		},

		"insert root ingress route and delegate ingress route": {
			objs: []interface{}{
				ir5, s4, ir4, s5, ir3,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							prefixroute("/blog", httpService(s4)),
							prefixroute("/blog/admin", httpService(s5)),
						),
					),
				},
			),
		},
		"insert ingress with retry annotations": {
			objs: []interface{}{
				ir15,
				s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("bar.com", &PrefixRoute{
							Prefix: "/",
							Route: Route{
								Clusters: clustermap(s1),
								RetryPolicy: &RetryPolicy{
									RetryOn:       "5xx",
									NumRetries:    6,
									PerTryTimeout: 10 * time.Second,
								},
							},
						}),
					),
				},
			),
		},
		"insert ingress with invalid perTryTimeout": {
			objs: []interface{}{
				ir15a,
				s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("bar.com", &PrefixRoute{
							Prefix: "/",
							Route: Route{
								Clusters: clustermap(s1),
								RetryPolicy: &RetryPolicy{
									RetryOn:       "5xx",
									NumRetries:    6,
									PerTryTimeout: 0,
								},
							},
						}),
					),
				},
			),
		},

		"insert ingress with zero retry count": {
			objs: []interface{}{
				ir15b,
				s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("bar.com", &PrefixRoute{
							Prefix: "/",
							Route: Route{
								Clusters: clustermap(s1),
								RetryPolicy: &RetryPolicy{
									RetryOn:       "5xx",
									NumRetries:    1,
									PerTryTimeout: 10 * time.Second,
								},
							},
						}),
					),
				},
			),
		},
		"insert ingressroute with retrypolicy": {
			objs: []interface{}{
				i14,
				s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*", &PrefixRoute{
							Prefix: "/",
							Route: Route{
								Clusters: clustermap(s1),
								RetryPolicy: &RetryPolicy{
									RetryOn:       "gateway-error",
									NumRetries:    6,
									PerTryTimeout: 10 * time.Second,
								},
							},
						}),
					),
				},
			),
		},
		"insert ingressroute with regex route": {
			objs: []interface{}{
				i15,
				s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*", &RegexRoute{
							Regex: "/[^/]+/invoices(/.*|/?)",
							Route: Route{
								Clusters: clustermap(s1),
							},
						}),
					),
				},
			),
		},
		"insert ingress overlay": {
			objs: []interface{}{
				i13a, i13b, sec13, s13a, s13b,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							routeUpgrade("/", httpService(s13a)),
							prefixroute("/.well-known/acme-challenge/gVJl5NWL2owUqZekjHkt_bo3OHYC2XNDURRRgLI5JTk", httpService(s13b)),
						),
					),
				}, &Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						securevirtualhost("example.com", sec13,
							routeUpgrade("/", httpService(s13a)),
							prefixroute("/.well-known/acme-challenge/gVJl5NWL2owUqZekjHkt_bo3OHYC2XNDURRRgLI5JTk", httpService(s13b)),
						),
					),
				},
			),
		},
		"h2c service annotation": {
			objs: []interface{}{
				i3a, s3a,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*",
							prefixroute("/", &HTTPService{
								TCPService: TCPService{
									Name:        s3a.Name,
									Namespace:   s3a.Namespace,
									ServicePort: &s3a.Spec.Ports[0],
								},
								Protocol: "h2c",
							}),
						),
					),
				},
			),
		},
		"h2 service annotation": {
			objs: []interface{}{
				i3a, s3b,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*",
							prefixroute("/", &HTTPService{
								TCPService: TCPService{
									Name:        s3b.Name,
									Namespace:   s3b.Namespace,
									ServicePort: &s3b.Spec.Ports[0],
								},
								Protocol: "h2",
							}),
						),
					),
				},
			),
		},
		"tls service annotation": {
			objs: []interface{}{
				i3a, s3c,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*",
							prefixroute("/", &HTTPService{
								TCPService: TCPService{
									Name:        s3c.Name,
									Namespace:   s3c.Namespace,
									ServicePort: &s3c.Spec.Ports[0],
								},
								Protocol: "tls",
							}),
						),
					),
				},
			),
		},
		"insert ingress then service w/ upstream annotations": {
			objs: []interface{}{
				i1,
				s1b,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*",
							prefixroute("/", &HTTPService{
								TCPService: TCPService{
									Name:               s1b.Name,
									Namespace:          s1b.Namespace,
									ServicePort:        &s1b.Spec.Ports[0],
									MaxConnections:     9000,
									MaxPendingRequests: 4096,
									MaxRequests:        404,
									MaxRetries:         7,
								},
							}),
						),
					),
				},
			),
		},
		"insert ingressroute with two routes to the same service": {
			objs: []interface{}{
				ir13, s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							routeCluster("/a", &Cluster{
								Upstream: &HTTPService{
									TCPService: TCPService{
										Name:        s1.Name,
										Namespace:   s1.Namespace,
										ServicePort: &s1.Spec.Ports[0],
									},
								},
								Weight: 90,
							}),
							routeCluster("/b", &Cluster{
								Upstream: &HTTPService{
									TCPService: TCPService{
										Name:        s1.Name,
										Namespace:   s1.Namespace,
										ServicePort: &s1.Spec.Ports[0],
									},
								},
								Weight: 60,
							}),
						),
					),
				},
			),
		},
		"insert ingressroute with one routes to the same service with two different weights": {
			objs: []interface{}{
				ir13a, s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							routeCluster("/a",
								&Cluster{
									Upstream: &HTTPService{
										TCPService: TCPService{
											Name:        s1.Name,
											Namespace:   s1.Namespace,
											ServicePort: &s1.Spec.Ports[0],
										},
									},
									Weight: 90,
								}, &Cluster{
									Upstream: &HTTPService{
										TCPService: TCPService{
											Name:        s1.Name,
											Namespace:   s1.Namespace,
											ServicePort: &s1.Spec.Ports[0],
										},
									},
									Weight: 60,
								},
							),
						),
					),
				},
			),
		},
		"ingressroute delegated to non existent object": {
			objs: []interface{}{
				&ingressroutev1.IngressRoute{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "example-com",
					},
					Spec: ingressroutev1.IngressRouteSpec{
						VirtualHost: &ingressroutev1.VirtualHost{
							Fqdn: "example.com",
						},
						Routes: []ingressroutev1.Route{{
							Match: "/finance",
							Delegate: &ingressroutev1.Delegate{
								Name:      "non-existent",
								Namespace: "non-existent",
							},
						}},
					},
				},
			},
			want: nil, // no listener created
		},
		"ingressroute delegates to itself": {
			objs: []interface{}{
				&ingressroutev1.IngressRoute{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "example-com",
					},
					Spec: ingressroutev1.IngressRouteSpec{
						VirtualHost: &ingressroutev1.VirtualHost{
							Fqdn: "example.com",
						},
						Routes: []ingressroutev1.Route{{
							Match: "/finance",
							Delegate: &ingressroutev1.Delegate{
								Name:      "example-com",
								Namespace: "default",
							},
						}},
					},
				},
			},
			want: nil, // no listener created
		},
		"ingressroute delegates to incorrect prefix": {
			objs: []interface{}{
				&ingressroutev1.IngressRoute{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "example-com",
					},
					Spec: ingressroutev1.IngressRouteSpec{
						VirtualHost: &ingressroutev1.VirtualHost{
							Fqdn: "example.com",
						},
						Routes: []ingressroutev1.Route{{
							Match: "/finance",
							Delegate: &ingressroutev1.Delegate{
								Name:      "finance-root",
								Namespace: "finance",
							},
						}},
					},
				},
				&ingressroutev1.IngressRoute{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "finance",
						Name:      "finance-root",
					},
					Spec: ingressroutev1.IngressRouteSpec{
						Routes: []ingressroutev1.Route{{
							Match: "/prefixDoesntMatch",
							Services: []ingressroutev1.Service{{
								Name: "home",
							}},
						}},
					},
				},
			},
			want: nil, // no listener created
		},
		"ingressroute delegate to prefix, but no matching path in delegate": {
			objs: []interface{}{
				&ingressroutev1.IngressRoute{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "example-com",
					},
					Spec: ingressroutev1.IngressRouteSpec{
						VirtualHost: &ingressroutev1.VirtualHost{
							Fqdn: "example.com",
						},
						Routes: []ingressroutev1.Route{{
							Match: "/foo",
							Delegate: &ingressroutev1.Delegate{
								Name:      "finance-root",
								Namespace: "finance",
							},
						}},
					},
				},
				&ingressroutev1.IngressRoute{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "finance",
						Name:      "finance-root",
					},
					Spec: ingressroutev1.IngressRouteSpec{
						Routes: []ingressroutev1.Route{{
							Match: "/foobar",
							Services: []ingressroutev1.Service{{
								Name: "home",
							}},
						}, {
							Match: "/foo/bar",
							Services: []ingressroutev1.Service{{
								Name: "home",
							}},
						}},
					},
				},
			},
			want: nil, // no listener created
		},
		"ingressroute cycle": {
			objs: []interface{}{
				&ingressroutev1.IngressRoute{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "example-com",
					},
					Spec: ingressroutev1.IngressRouteSpec{
						VirtualHost: &ingressroutev1.VirtualHost{
							Fqdn: "example.com",
						},
						Routes: []ingressroutev1.Route{{
							Match: "/finance",
							Delegate: &ingressroutev1.Delegate{
								Name:      "finance-root",
								Namespace: "finance",
							},
						}},
					},
				},
				&ingressroutev1.IngressRoute{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "finance",
						Name:      "finance-root",
					},
					Spec: ingressroutev1.IngressRouteSpec{
						Routes: []ingressroutev1.Route{{
							Match: "/finance",
							Services: []ingressroutev1.Service{{
								Name: "home",
								Port: 8080,
							}},
						}, {
							Match: "/finance/stocks",
							Delegate: &ingressroutev1.Delegate{
								Name:      "example-com",
								Namespace: "default",
							},
						}},
					},
				},
				s7,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", prefixroute("/finance", httpService(s7))),
					),
				},
			),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			builder := Builder{
				DisablePermitInsecure: tc.disablePermitInsecure,
				Source: KubernetesCache{
					FieldLogger: testLogger(t),
				},
			}
			for _, o := range tc.objs {
				if !builder.Source.Insert(o) {
					t.Logf("insert %v: failed", o)
				}
			}
			dag := builder.Build()

			got := make(map[int]*Listener)
			dag.Visit(listenerMap(got).Visit)

			want := make(map[int]*Listener)
			for _, v := range tc.want {
				if l, ok := v.(*Listener); ok {
					want[l.Port] = l
				}
			}

			opts := []cmp.Option{
				cmp.AllowUnexported(Listener{}, VirtualHost{}),
			}
			if diff := cmp.Diff(want, got, opts...); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

type listenerMap map[int]*Listener

func (lm listenerMap) Visit(v Vertex) {
	if l, ok := v.(*Listener); ok {
		lm[l.Port] = l
	}
}

func backend(name string, port intstr.IntOrString) *v1beta1.IngressBackend {
	return &v1beta1.IngressBackend{
		ServiceName: name,
		ServicePort: port,
	}
}

func ingressrulevalue(backend *v1beta1.IngressBackend) v1beta1.IngressRuleValue {
	return v1beta1.IngressRuleValue{
		HTTP: &v1beta1.HTTPIngressRuleValue{
			Paths: []v1beta1.HTTPIngressPath{{
				Backend: *backend,
			}},
		},
	}
}

func TestBuilderLookupHTTPService(t *testing.T) {
	s1 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}
	services := map[Meta]*v1.Service{
		{name: "service1", namespace: "default"}: s1,
	}

	tests := map[string]struct {
		Meta
		port intstr.IntOrString
		want *HTTPService
	}{
		"lookup service by port number": {
			Meta: Meta{name: "service1", namespace: "default"},
			port: intstr.FromInt(8080),
			want: httpService(s1),
		},
		"lookup service by port name": {
			Meta: Meta{name: "service1", namespace: "default"},
			port: intstr.FromString("http"),
			want: httpService(s1),
		},
		"lookup service by port number (as string)": {
			Meta: Meta{name: "service1", namespace: "default"},
			port: intstr.Parse("8080"),
			want: httpService(s1),
		},
		"lookup service by port number (from string)": {
			Meta: Meta{name: "service1", namespace: "default"},
			port: intstr.FromString("8080"),
			want: httpService(s1),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			b := Builder{
				Source: KubernetesCache{
					services:    services,
					FieldLogger: testLogger(t),
				},
			}
			b.reset()
			got := b.lookupHTTPService(tc.Meta, tc.port)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func TestDAGRootNamespaces(t *testing.T) {
	ir1 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "allowed1",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "example.com",
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

	// ir2 is like ir1, but in a different namespace
	ir2 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "allowed2",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "example2.com",
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

	s2 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "allowed1",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:     "http",
				Protocol: "TCP",
				Port:     8080,
			}},
		},
	}

	s3 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "allowed2",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:     "http",
				Protocol: "TCP",
				Port:     8080,
			}},
		},
	}

	tests := map[string]struct {
		rootNamespaces []string
		objs           []interface{}
		want           int
	}{
		"nil root namespaces": {
			objs: []interface{}{ir1, s2},
			want: 1,
		},
		"empty root namespaces": {
			objs: []interface{}{ir1, s2},
			want: 1,
		},
		"single root namespace with root ingressroute": {
			rootNamespaces: []string{"allowed1"},
			objs:           []interface{}{ir1, s2},
			want:           1,
		},
		"multiple root namespaces, one with a root ingressroute": {
			rootNamespaces: []string{"foo", "allowed1", "bar"},
			objs:           []interface{}{ir1, s2},
			want:           1,
		},
		"multiple root namespaces, each with a root ingressroute": {
			rootNamespaces: []string{"foo", "allowed1", "allowed2"},
			objs:           []interface{}{ir1, ir2, s2, s3},
			want:           2,
		},
		"root ingressroute defined outside single root namespaces": {
			rootNamespaces: []string{"foo"},
			objs:           []interface{}{ir1},
			want:           0,
		},
		"root ingressroute defined outside multiple root namespaces": {
			rootNamespaces: []string{"foo", "bar"},
			objs:           []interface{}{ir1},
			want:           0,
		},
		"two root ingressroutes, one inside root namespace, one outside": {
			rootNamespaces: []string{"foo", "allowed2"},
			objs:           []interface{}{ir1, ir2, s3},
			want:           1,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			builder := Builder{
				Source: KubernetesCache{
					IngressRouteRootNamespaces: tc.rootNamespaces,
					FieldLogger:                testLogger(t),
				},
			}

			for _, o := range tc.objs {
				builder.Source.Insert(o)
			}
			dag := builder.Build()

			var count int
			dag.Visit(func(v Vertex) {
				v.Visit(func(v Vertex) {
					if _, ok := v.(*VirtualHost); ok {
						count++
					}
				})
			})

			if tc.want != count {
				t.Errorf("wanted %d vertices, but got %d", tc.want, count)
			}
		})
	}
}

func TestMatchesPathPrefix(t *testing.T) {
	tests := map[string]struct {
		path    string
		prefix  string
		matches bool
	}{
		"no path cannot match the prefix": {
			prefix:  "/foo",
			path:    "",
			matches: false,
		},
		"any path has the empty string as the prefix": {
			prefix:  "",
			path:    "/foo",
			matches: true,
		},
		"strict match": {
			prefix:  "/foo",
			path:    "/foo",
			matches: true,
		},
		"strict match with / at the end": {
			prefix:  "/foo/",
			path:    "/foo/",
			matches: true,
		},
		"no match": {
			prefix:  "/foo",
			path:    "/bar",
			matches: false,
		},
		"string prefix match should not match": {
			prefix:  "/foo",
			path:    "/foobar",
			matches: false,
		},
		"prefix match": {
			prefix:  "/foo",
			path:    "/foo/bar",
			matches: true,
		},
		"prefix match with trailing slash in prefix": {
			prefix:  "/foo/",
			path:    "/foo/bar",
			matches: true,
		},
		"prefix match with trailing slash in path": {
			prefix:  "/foo",
			path:    "/foo/bar/",
			matches: true,
		},
		"prefix match with trailing slashes": {
			prefix:  "/foo/",
			path:    "/foo/bar/",
			matches: true,
		},
		"prefix match two levels": {
			prefix:  "/foo/bar",
			path:    "/foo/bar",
			matches: true,
		},
		"prefix match two levels trailing slash in prefix": {
			prefix:  "/foo/bar/",
			path:    "/foo/bar",
			matches: true,
		},
		"prefix match two levels trailing slash in path": {
			prefix:  "/foo/bar",
			path:    "/foo/bar/",
			matches: true,
		},
		"no match two levels": {
			prefix:  "/foo/bar",
			path:    "/foo/baz",
			matches: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := matchesPathPrefix(tc.path, tc.prefix)
			if got != tc.matches {
				t.Errorf("expected %v but got %v", tc.matches, got)
			}
		})
	}
}

func TestDAGIngressRouteStatus(t *testing.T) {
	// ir1 is a valid ingressroute
	ir1 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []ingressroutev1.Route{{
				Match: "/foo",
				Services: []ingressroutev1.Service{{
					Name: "home",
					Port: 8080,
				}},
			}, {
				Match: "/prefix",
				Delegate: &ingressroutev1.Delegate{
					Name: "delegated",
				}},
			},
		},
	}

	// ir2 is invalid because it contains a service with negative port
	ir2 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []ingressroutev1.Route{{
				Match: "/foo",
				Services: []ingressroutev1.Service{{
					Name: "home",
					Port: -80,
				}},
			}},
		},
	}

	// ir3 is invalid because it lives outside the roots namespace
	ir3 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "finance",
			Name:      "example",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []ingressroutev1.Route{{
				Match: "/foobar",
				Services: []ingressroutev1.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	// ir4 is invalid because its match prefix does not match its parent's (ir1)
	ir4 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "delegated",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			Routes: []ingressroutev1.Route{{
				Match: "/doesnotmatch",
				Services: []ingressroutev1.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	// ir5 is invalid because its service weight is less than zero
	ir5 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "delegated",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []ingressroutev1.Route{{
				Match: "/foo",
				Services: []ingressroutev1.Service{{
					Name:   "home",
					Port:   8080,
					Weight: -10,
				}},
			}},
		},
	}

	// ir6 is invalid because it delegates to itself, producing a cycle
	ir6 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "self",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []ingressroutev1.Route{{
				Match: "/foo",
				Delegate: &ingressroutev1.Delegate{
					Name: "self",
				},
			}},
		},
	}

	// ir7 delegates to ir8, which is invalid because it delegates back to ir7
	ir7 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "parent",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []ingressroutev1.Route{{
				Match: "/foo",
				Delegate: &ingressroutev1.Delegate{
					Name: "child",
				},
			}},
		},
	}

	ir8 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "child",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			Routes: []ingressroutev1.Route{{
				Match: "/foo",
				Delegate: &ingressroutev1.Delegate{
					Name: "parent",
				},
			}},
		},
	}

	// ir9 is invalid because it has a route that both delegates and has a list of services
	ir9 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "parent",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []ingressroutev1.Route{{
				Match: "/foo",
				Delegate: &ingressroutev1.Delegate{
					Name: "child",
				},
				Services: []ingressroutev1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	// ir10 delegates to ir11 and ir 12.
	ir10 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "parent",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []ingressroutev1.Route{{
				Match: "/foo",
				Delegate: &ingressroutev1.Delegate{
					Name: "validChild",
				},
			}, {
				Match: "/bar",
				Delegate: &ingressroutev1.Delegate{
					Name: "invalidChild",
				},
			}},
		},
	}

	ir11 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "validChild",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			Routes: []ingressroutev1.Route{{
				Match: "/foo",
				Services: []ingressroutev1.Service{{
					Name: "foo2",
					Port: 8080,
				}},
			}},
		},
	}

	// ir12 is invalid because it contains an invalid port
	ir12 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "invalidChild",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			Routes: []ingressroutev1.Route{{
				Match: "/bar",
				Services: []ingressroutev1.Service{{
					Name: "foo3",
					Port: 12345678,
				}},
			}},
		},
	}

	// ir13 is invalid because it does not specify and FQDN
	ir13 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "parent",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{},
			Routes: []ingressroutev1.Route{{
				Match: "/foo",
				Services: []ingressroutev1.Service{{
					Name: "foo",
					Port: 8080,
				}},
			}},
		},
	}

	// ir14 delegates tp ir15 but it is invalid because it is missing fqdn
	ir14 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "invalidParent",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{},
			Routes: []ingressroutev1.Route{{
				Match: "/foo",
				Delegate: &ingressroutev1.Delegate{
					Name: "validChild",
				},
			}},
		},
	}

	// ir15 is invalid because it contains a wildcarded fqdn
	ir15 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "example.*.com",
			},
			Routes: []ingressroutev1.Route{{
				Match: "/foo",
				Services: []ingressroutev1.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	// ir16 is invalid because it references an invalid service
	ir16 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "invalidir",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []ingressroutev1.Route{{
				Match: "/foo",
				Services: []ingressroutev1.Service{{
					Name: "invalid",
					Port: 8080,
				}},
			}},
		},
	}

	s4 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "home",
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

	s5 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "parent",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:     "http",
				Protocol: "TCP",
				Port:     8080,
			}},
		},
	}

	s6 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "foo2",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:     "http",
				Protocol: "TCP",
				Port:     8080,
			}},
		},
	}

	s7 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo3",
			Namespace: "roots",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:     "http",
				Protocol: "TCP",
				Port:     12345678,
			}},
		},
	}

	tests := map[string]struct {
		objs []interface{}
		want []Status
	}{
		"valid ingressroute": {
			objs: []interface{}{ir1, s4},
			want: []Status{{Object: ir1, Status: "valid", Description: "valid IngressRoute", Vhost: "example.com"}},
		},
		"invalid port in service": {
			objs: []interface{}{ir2},
			want: []Status{{Object: ir2, Status: "invalid", Description: `route "/foo": service "home": port must be in the range 1-65535`, Vhost: "example.com"}},
		},
		"root ingressroute outside of roots namespace": {
			objs: []interface{}{ir3},
			want: []Status{{Object: ir3, Status: "invalid", Description: "root IngressRoute cannot be defined in this namespace"}},
		},
		"delegated route's match prefix does not match parent's prefix": {
			objs: []interface{}{ir1, ir4, s4},
			want: []Status{
				{Object: ir1, Status: "valid", Description: "valid IngressRoute", Vhost: "example.com"},
				{Object: ir4, Status: "invalid", Description: `the path prefix "/doesnotmatch" does not match the parent's path prefix "/prefix"`, Vhost: "example.com"},
			},
		},
		"invalid weight in service": {
			objs: []interface{}{ir5},
			want: []Status{{Object: ir5, Status: "invalid", Description: `route "/foo": service "home": weight must be greater than or equal to zero`, Vhost: "example.com"}},
		},
		"root ingressroute does not specify FQDN": {
			objs: []interface{}{ir13},
			want: []Status{{Object: ir13, Status: "invalid", Description: "Spec.VirtualHost.Fqdn must be specified"}},
		},
		"self-edge produces a cycle": {
			objs: []interface{}{ir6},
			want: []Status{{Object: ir6, Status: "invalid", Description: "route creates a delegation cycle: roots/self -> roots/self", Vhost: "example.com"}},
		},
		"child delegates to parent, producing a cycle": {
			objs: []interface{}{ir7, ir8},
			want: []Status{
				{Object: ir7, Status: "valid", Description: "valid IngressRoute", Vhost: "example.com"},
				{Object: ir8, Status: "invalid", Description: "route creates a delegation cycle: roots/parent -> roots/child -> roots/parent", Vhost: "example.com"},
			},
		},
		"route has a list of services and also delegates": {
			objs: []interface{}{ir9},
			want: []Status{{Object: ir9, Status: "invalid", Description: `route "/foo": cannot specify services and delegate in the same route`, Vhost: "example.com"}},
		},
		"ingressroute is an orphaned route": {
			objs: []interface{}{ir8},
			want: []Status{{Object: ir8, Status: "orphaned", Description: "this IngressRoute is not part of a delegation chain from a root IngressRoute"}},
		},
		"ingressroute delegates to multiple ingressroutes, one is invalid": {
			objs: []interface{}{ir10, ir11, ir12, s6, s7},
			want: []Status{
				{Object: ir11, Status: "valid", Description: "valid IngressRoute", Vhost: "example.com"},
				{Object: ir12, Status: "invalid", Description: `route "/bar": service "foo3": port must be in the range 1-65535`, Vhost: "example.com"},
				{Object: ir10, Status: "valid", Description: "valid IngressRoute", Vhost: "example.com"},
			},
		},
		"invalid parent orphans children": {
			objs: []interface{}{ir14, ir11},
			want: []Status{
				{Object: ir14, Status: "invalid", Description: "Spec.VirtualHost.Fqdn must be specified"},
				{Object: ir11, Status: "orphaned", Description: "this IngressRoute is not part of a delegation chain from a root IngressRoute"},
			},
		},
		"multi-parent children is not orphaned when one of the parents is invalid": {
			objs: []interface{}{ir14, ir11, ir10, s5, s6},
			want: []Status{
				{Object: ir14, Status: "invalid", Description: "Spec.VirtualHost.Fqdn must be specified"},
				{Object: ir11, Status: "valid", Description: "valid IngressRoute", Vhost: "example.com"},
				{Object: ir10, Status: "valid", Description: "valid IngressRoute", Vhost: "example.com"},
			},
		},
		"invalid FQDN contains wildcard": {
			objs: []interface{}{ir15},
			want: []Status{{Object: ir15, Status: "invalid", Description: `Spec.VirtualHost.Fqdn "example.*.com" cannot use wildcards`, Vhost: "example.*.com"}},
		},
		"missing service shows invalid status": {
			objs: []interface{}{ir16},
			want: []Status{{Object: ir16, Status: "invalid", Description: `Service [invalid:8080] is invalid or missing`, Vhost: ""}},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			builder := Builder{
				Source: KubernetesCache{
					IngressRouteRootNamespaces: []string{"roots"},
					FieldLogger:                testLogger(t),
				},
			}
			for _, o := range tc.objs {
				builder.Source.Insert(o)
			}
			dag := builder.Build()

			got := dag.Statuses()
			if len(tc.want) != len(got) {
				t.Fatalf("expected:\n%v\ngot\n%v", tc.want, got)
			}

			for _, ex := range tc.want {
				var found bool
				for _, g := range got {
					if cmp.Equal(ex, g) {
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("expected to find:\n%v\nbut did not find it in:\n%v", ex, got)
				}
			}
		})
	}
}

func TestDAGIngressRouteUniqueFQDNs(t *testing.T) {
	ir1 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "example.com",
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

	// ir2 reuses the fqdn used in ir1
	ir2 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "other-example",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "example.com",
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

	s1 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}

	tests := map[string]struct {
		objs       []interface{}
		want       []Vertex
		wantStatus map[Meta]Status
	}{
		"insert ingressroute": {
			objs: []interface{}{
				s1, ir1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						&VirtualHost{
							Name: "example.com",
							routes: routes(
								prefixroute("/", httpService(s1)),
							),
						},
					),
				},
			),
			wantStatus: map[Meta]Status{
				{name: ir1.Name, namespace: ir1.Namespace}: {
					Object:      ir1,
					Status:      StatusValid,
					Description: "valid IngressRoute",
					Vhost:       "example.com",
				},
			},
		},
		"insert conflicting ingressroutes due to fqdn reuse": {
			objs: []interface{}{
				ir1, ir2,
			},
			want: []Vertex{},
			wantStatus: map[Meta]Status{
				{name: ir1.Name, namespace: ir1.Namespace}: {
					Object:      ir1,
					Status:      StatusInvalid,
					Description: `fqdn "example.com" is used in multiple IngressRoutes: default/example-com, default/other-example`,
					Vhost:       "example.com",
				},
				{name: ir2.Name, namespace: ir2.Namespace}: {
					Object:      ir2,
					Status:      StatusInvalid,
					Description: `fqdn "example.com" is used in multiple IngressRoutes: default/example-com, default/other-example`,
					Vhost:       "example.com",
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			builder := Builder{
				Source: KubernetesCache{
					FieldLogger: testLogger(t),
				},
			}
			for _, o := range tc.objs {
				builder.Source.Insert(o)
			}
			dag := builder.Build()

			got := make(map[int]*Listener)
			dag.Visit(listenerMap(got).Visit)

			want := make(map[int]*Listener)
			for _, v := range tc.want {
				if l, ok := v.(*Listener); ok {
					want[l.Port] = l
				}
			}

			opts := []cmp.Option{
				cmp.AllowUnexported(Listener{}, VirtualHost{}),
			}
			if diff := cmp.Diff(want, got, opts...); diff != "" {
				t.Fatal(diff)
			}

			gotStatus := dag.statuses
			if diff := cmp.Diff(tc.wantStatus, gotStatus); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func TestHttpPaths(t *testing.T) {
	tests := map[string]struct {
		rule v1beta1.IngressRule
		want []v1beta1.HTTPIngressPath
	}{
		"zero value": {
			rule: v1beta1.IngressRule{},
			want: nil,
		},
		"empty paths": {
			rule: v1beta1.IngressRule{
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{},
				},
			},
			want: nil,
		},
		"several paths": {
			rule: v1beta1.IngressRule{
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Backend: v1beta1.IngressBackend{
								ServiceName: "kuard",
								ServicePort: intstr.FromString("http"),
							},
						}, {
							Path: "/kuarder",
							Backend: v1beta1.IngressBackend{
								ServiceName: "kuarder",
								ServicePort: intstr.FromInt(8080),
							},
						}},
					},
				},
			},
			want: []v1beta1.HTTPIngressPath{{
				Backend: v1beta1.IngressBackend{
					ServiceName: "kuard",
					ServicePort: intstr.FromString("http"),
				},
			}, {
				Path: "/kuarder",
				Backend: v1beta1.IngressBackend{ServiceName: "kuarder",
					ServicePort: intstr.FromInt(8080),
				},
			}},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := httppaths(tc.rule)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf(diff)
			}
		})
	}
}
func TestEnforceRoute(t *testing.T) {
	tests := map[string]struct {
		tlsEnabled     bool
		permitInsecure bool
		want           bool
	}{
		"tls not enabled": {
			tlsEnabled:     false,
			permitInsecure: false,
			want:           false,
		},
		"tls enabled": {
			tlsEnabled:     true,
			permitInsecure: false,
			want:           true,
		},
		"tls enabled but insecure requested": {
			tlsEnabled:     true,
			permitInsecure: true,
			want:           false,
		},
		"tls not enabled but insecure requested": {
			tlsEnabled:     false,
			permitInsecure: true,
			want:           false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := routeEnforceTLS(tc.tlsEnabled, tc.permitInsecure)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf(diff)
			}
		})
	}
}

func TestSplitSecret(t *testing.T) {
	tests := map[string]struct {
		secret, defns string
		want          Meta
	}{
		"no namespace": {
			secret: "secret",
			defns:  "default",
			want: Meta{
				name:      "secret",
				namespace: "default",
			},
		},
		"with namespace": {
			secret: "ns1/secret",
			defns:  "default",
			want: Meta{
				name:      "secret",
				namespace: "ns1",
			},
		},
		"missing namespace": {
			secret: "/secret",
			defns:  "default",
			want: Meta{
				name:      "secret",
				namespace: "default",
			},
		},
		"missing secret name": {
			secret: "secret/",
			defns:  "default",
			want: Meta{
				name:      "",
				namespace: "secret",
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := splitSecret(tc.secret, tc.defns)
			opts := []cmp.Option{
				cmp.AllowUnexported(Meta{}),
			}
			if diff := cmp.Diff(tc.want, got, opts...); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func routes(v ...Vertex) map[string]Vertex {
	if len(v) == 0 {
		return nil
	}
	m := make(map[string]Vertex)
	for _, r := range v {
		switch r := r.(type) {
		case *PrefixRoute:
			m[r.Prefix] = r
		case *RegexRoute:
			m[r.Regex] = r
		default:
			panic(fmt.Sprintf("unexpected route type: %T %#v", r, r))
		}
	}
	return m
}

func prefixroute(prefix string, first *HTTPService, rest ...*HTTPService) *PrefixRoute {
	services := append([]*HTTPService{first}, rest...)
	route := PrefixRoute{
		Prefix: prefix,
	}
	for _, s := range services {
		route.Clusters = append(route.Clusters, &Cluster{
			Upstream: s,
		})
	}
	return &route
}

func routeCluster(prefix string, clusters ...*Cluster) *PrefixRoute {
	route := PrefixRoute{
		Prefix: prefix,
		Route: Route{
			Clusters: clusters,
		},
	}
	return &route
}

func routeUpgrade(prefix string, first *HTTPService, rest ...*HTTPService) *PrefixRoute {
	r := prefixroute(prefix, first, rest...)
	r.HTTPSUpgrade = true
	return r
}

func routeRewrite(prefix, rewrite string, first *HTTPService, rest ...*HTTPService) *PrefixRoute {
	r := prefixroute(prefix, first, rest...)
	r.PrefixRewrite = rewrite
	return r
}

func routeWebsocket(prefix string, first *HTTPService, rest ...*HTTPService) *PrefixRoute {
	r := prefixroute(prefix, first, rest...)
	r.Websocket = true
	return r
}

func clusters(services ...Service) (c []*Cluster) {
	for _, s := range services {
		c = append(c, &Cluster{
			Upstream: s,
		})
	}
	return c
}

func tcpService(s *v1.Service) *TCPService {
	return &TCPService{
		Name:        s.Name,
		Namespace:   s.Namespace,
		ServicePort: &s.Spec.Ports[0],
	}
}

func httpService(s *v1.Service) *HTTPService {
	return &HTTPService{
		TCPService: TCPService{
			Name:        s.Name,
			Namespace:   s.Namespace,
			ServicePort: &s.Spec.Ports[0],
		},
	}
}

func clustermap(services ...*v1.Service) []*Cluster {
	var c []*Cluster
	for _, s := range services {
		c = append(c, &Cluster{
			Upstream: httpService(s),
		})
	}
	return c
}

func secret(s *v1.Secret) *Secret {
	return &Secret{
		Object: s,
	}
}

func virtualhosts(vx ...Vertex) map[string]Vertex {
	m := make(map[string]Vertex)
	for _, v := range vx {
		switch v := v.(type) {
		case *VirtualHost:
			m[v.Name] = v
		case *SecureVirtualHost:
			m[v.VirtualHost.Name] = v
		default:
			panic(fmt.Sprintf("unable to handle Vertex type %T", v))
		}
	}
	return m
}

func virtualhost(name string, v ...Vertex) *VirtualHost {
	return &VirtualHost{
		Name:   name,
		routes: routes(v...),
	}
}

func securevirtualhost(name string, sec *v1.Secret, v ...Vertex) *SecureVirtualHost {
	return &SecureVirtualHost{
		VirtualHost: VirtualHost{
			Name:   name,
			routes: routes(v...),
		},
		MinProtoVersion: auth.TlsParameters_TLSv1_1,
		Secret:          secret(sec),
	}
}

func listeners(ls ...*Listener) []Vertex {
	var v []Vertex
	for _, l := range ls {
		v = append(v, l)
	}
	return v
}
