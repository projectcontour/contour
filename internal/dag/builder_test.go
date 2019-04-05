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
	"sort"
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
		Data: map[string][]byte{
			v1.TLSCertKey:       []byte("certificate"),
			v1.TLSPrivateKeyKey: []byte("key"),
		},
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

	sec1 := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret",
			Namespace: "default",
		},
		Data: secretdata("certificate", "key"),
	}

	tests := map[string]struct {
		*Builder
		objs []interface{}
		want []Vertex
	}{
		"insert ingress w/ default backend": {
			objs: []interface{}{
				i1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						&VirtualHost{
							Name: "*",
							routes: routemap(
								route("/", i1),
							),
						},
					),
				},
			),
		},
		"insert ingress w/ single unnamed backend": {
			objs: []interface{}{
				i2,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						&VirtualHost{
							Name: "*",
							routes: routemap(
								route("/", i2),
							),
						},
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
		"insert ingress w/ host name and single backend": {
			objs: []interface{}{
				i3,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						&VirtualHost{
							Name: "kuard.example.com",
							routes: routemap(
								route("/", i3),
							),
						},
					),
				},
			),
		},
		"insert ingress w/ default backend then matching service": {
			objs: []interface{}{
				i1,
				s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						&VirtualHost{
							Name: "*",
							routes: routemap(
								route("/", i1, servicemap(
									httpService(s1),
								)),
							),
						},
					),
				},
			),
		},
		"insert service then ingress w/ default backend": {
			objs: []interface{}{
				s1,
				i1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						&VirtualHost{
							Name: "*",
							routes: routemap(
								route("/", i1, servicemap(
									httpService(s1),
								)),
							),
						},
					),
				},
			),
		},
		"insert ingress w/ default backend then non-matching service": {
			objs: []interface{}{
				i1,
				s2,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						&VirtualHost{
							Name: "*",
							routes: routemap(
								route("/", i1),
							),
						},
					),
				},
			),
		},
		"insert non matching service then ingress w/ default backend": {
			objs: []interface{}{
				s2,
				i1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						&VirtualHost{
							Name: "*",
							routes: routemap(
								route("/", i1),
							),
						},
					),
				},
			),
		},
		"insert ingress w/ default backend then matching service with wrong port": {
			objs: []interface{}{
				i1,
				s3,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						&VirtualHost{
							Name: "*",
							routes: routemap(
								route("/", i1),
							),
						},
					),
				},
			),
		},
		"insert unnamed ingress w/ single backend then matching service with wrong port": {
			objs: []interface{}{
				i2,
				s3,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						&VirtualHost{
							Name: "*",
							routes: routemap(
								route("/", i2),
							),
						},
					),
				},
			),
		},
		"insert service then matching unnamed ingress w/ single backend but wrong port": {
			objs: []interface{}{
				s3,
				i2,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						&VirtualHost{
							Name: "*",
							routes: routemap(
								route("/", i2),
							),
						},
					),
				},
			),
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
						&VirtualHost{
							Name: "*",
							routes: routemap(
								route("/", i4, servicemap(
									httpService(s1),
								)),
							),
						},
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
						&VirtualHost{
							Name: "*",
							routes: routemap(
								route("/", i4, servicemap(
									httpService(s1),
								)),
							),
						},
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
						&VirtualHost{
							Name: "*",
							routes: routemap(
								route("/", i5, servicemap(
									httpService(s1),
								)),
							),
						},
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
						&VirtualHost{
							Name: "*",
							routes: routemap(
								route("/", i5, servicemap(
									httpService(s1),
								)),
							),
						},
					),
				},
			),
		},
		"insert secret": {
			objs: []interface{}{
				sec1,
			},
			want: []Vertex{},
		},
		"insert secret then ingress w/o tls": {
			objs: []interface{}{
				sec1,
				i1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						&VirtualHost{
							Name: "*",
							routes: routemap(
								route("/", i1),
							),
						},
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
					Port: 80,
					VirtualHosts: virtualhosts(
						&VirtualHost{
							Name: "kuard.example.com",
							routes: routemap(
								route("/", i3),
							),
						},
					),
				},
				&Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "kuard.example.com",
								routes: routemap(
									route("/", i3),
								),
							},
							MinProtoVersion: auth.TlsParameters_TLSv1_1,
							Secret:          secret(sec1),
						},
					),
				},
			),
		},
		"insert ingress w/ tls then secret": {
			objs: []interface{}{
				i3,
				sec1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						&VirtualHost{
							Name: "kuard.example.com",
							routes: routemap(
								route("/", i3),
							),
						},
					),
				},
				&Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "kuard.example.com",
								routes: routemap(
									route("/", i3),
								),
							},
							MinProtoVersion: auth.TlsParameters_TLSv1_1,
							Secret:          secret(sec1),
						},
					),
				},
			),
		},
		"insert ingress w/ tls with different secure port": {
			Builder: &Builder{
				ExternalSecurePort: 8443,
			},
			objs: []interface{}{
				i3,
				sec1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						&VirtualHost{
							Name: "kuard.example.com",
							routes: routemap(
								route("/", i3),
							),
						},
					),
				},
				&Listener{
					Port: 8443,
					VirtualHosts: virtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "kuard.example.com",
								routes: routemap(
									route("/", i3),
								),
							},
							MinProtoVersion: auth.TlsParameters_TLSv1_1,
							Secret:          secret(sec1),
						},
					),
				},
			),
		},
		"insert ingress w/ two vhosts": {
			objs: []interface{}{
				i6,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						&VirtualHost{
							Name: "a.example.com",
							routes: routemap(
								route("/", i6),
							),
						},
						&VirtualHost{
							Name: "b.example.com",
							routes: routemap(
								route("/", i6),
							),
						},
					),
				},
			),
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
						&VirtualHost{
							Name: "a.example.com",
							routes: routemap(
								route("/", i6, servicemap(
									httpService(s1),
								)),
							),
						},
						&VirtualHost{
							Name: "b.example.com",
							routes: routemap(
								route("/", i6, servicemap(
									httpService(s1),
								)),
							),
						},
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
						&VirtualHost{
							Name: "a.example.com",
							routes: routemap(
								route("/", i6, servicemap(
									httpService(s1),
								)),
							),
						},
						&VirtualHost{
							Name: "b.example.com",
							routes: routemap(
								route("/", i6, servicemap(
									httpService(s1),
								)),
							),
						},
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
						&VirtualHost{
							Name: "a.example.com",
							routes: routemap(
								route("/", i6, servicemap(
									httpService(s1),
								)),
							),
						},
						&VirtualHost{
							Name: "b.example.com",
							routes: routemap(
								route("/", i6, servicemap(
									httpService(s1),
								)),
							),
						},
					),
				}, &Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "b.example.com",
								routes: routemap(
									route("/", i6, servicemap(
										httpService(s1),
									)),
								),
							},
							MinProtoVersion: auth.TlsParameters_TLSv1_1,
							Secret:          secret(sec1),
						},
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
						&VirtualHost{
							Name: "a.example.com",
							routes: routemap(
								route("/", i6, servicemap(
									httpService(s1),
								)),
							),
						}, &VirtualHost{
							Name: "b.example.com",
							routes: routemap(
								route("/", i6, servicemap(
									httpService(s1),
								)),
							),
						},
					),
				}, &Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "b.example.com",
								routes: routemap(
									route("/", i6, servicemap(
										httpService(s1),
									)),
								),
							},
							MinProtoVersion: auth.TlsParameters_TLSv1_1,
							Secret:          secret(sec1),
						},
					),
				},
			),
		},
		"insert ingress w/ two paths": {
			objs: []interface{}{
				i7,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						&VirtualHost{
							Name: "b.example.com",
							routes: routemap(
								route("/", i7),
								route("/kuarder", i7),
							),
						},
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
						&VirtualHost{
							Name: "b.example.com",
							routes: routemap(
								route("/", i7, servicemap(
									httpService(s1),
								)),
								route("/kuarder", i7, servicemap(
									httpService(s2),
								)),
							),
						},
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
						&VirtualHost{
							Name: "b.example.com",
							routes: routemap(
								route("/", i8, servicemap(
									httpService(s1),
								)),
								route("/kuarder", i8, servicemap(
									httpService(s2),
								)),
							),
						},
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
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "b.example.com",
								routes: routemap(
									route("/", i9, servicemap(
										httpService(s1),
									)),
									route("/kuarder", i9, servicemap(
										httpService(s2),
									)),
								),
							},
							MinProtoVersion: auth.TlsParameters_TLSv1_1,
							Secret:          secret(sec1),
						},
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
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "b.example.com",
								routes: routemap(
									route("/", i6a, servicemap(
										httpService(s1),
									)),
								),
							},
							MinProtoVersion: auth.TlsParameters_TLSv1_1,
							Secret:          secret(sec1),
						},
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
						&VirtualHost{
							Name: "b.example.com",
							routes: routemap(
								&Route{
									Prefix: "/",
									object: i6b,
									httpServices: servicemap(
										httpService(s1),
									),
									HTTPSUpgrade: true,
								},
							),
						},
					),
				}, &Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "b.example.com",
								routes: routemap(
									&Route{
										Prefix: "/",
										object: i6b,
										httpServices: servicemap(
											httpService(s1),
										),
										HTTPSUpgrade: true,
									},
								),
							},
							MinProtoVersion: auth.TlsParameters_TLSv1_1,
							Secret:          secret(sec1),
						},
					),
				},
			),
		},

		"insert ingressroute": {
			objs: []interface{}{
				ir1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						&VirtualHost{
							Name: "example.com",
							routes: routemap(
								route("/", ir1),
							),
						},
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
						&VirtualHost{
							Name: "example.com",
							routes: routemap(
								route("/", ir11, servicemap(
									httpService(s1),
								)),
								&Route{
									Prefix: "/websocket",
									object: ir11,
									httpServices: servicemap(
										httpService(s1),
									),
									PrefixRewrite: "/",
								},
							),
						},
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
									Services: []*TCPService{
										tcpService(s1),
									},
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
									Services: []*TCPService{
										tcpService(s1),
									},
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
									Services: []*TCPService{
										tcpService(s6),
									},
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
						&VirtualHost{
							Name: "example.com",
							routes: routemap(
								route("/", ir10, servicemap(
									httpService(s1),
								)),
								&Route{
									Prefix: "/websocket",
									object: ir10,
									httpServices: servicemap(
										httpService(s1),
									),
									Websocket: true,
								},
							),
						},
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
						&VirtualHost{
							Name: "example.com",
							routes: routemap(
								route("/", ir1, servicemap(
									httpService(s1),
								)),
							),
						},
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
						&VirtualHost{
							Name: "foo.com",
							routes: routemap(
								&Route{
									Prefix: "/",
									object: ir6,
									httpServices: servicemap(
										httpService(s1),
									),
									HTTPSUpgrade: true,
								},
							),
						},
					),
				}, &Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						&SecureVirtualHost{
							MinProtoVersion: auth.TlsParameters_TLSv1_1,
							VirtualHost: VirtualHost{
								Name: "foo.com",
								routes: routemap(
									&Route{
										Prefix: "/",
										object: ir6,
										httpServices: servicemap(
											httpService(s1),
										),
										HTTPSUpgrade: true,
									}),
							},
							Secret: secret(sec1),
						},
					),
				},
			),
		},
		"insert ingressroute with TLS one insecure": {
			objs: []interface{}{
				ir14, s1, sec1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						&VirtualHost{
							Name: "foo.com",
							routes: routemap(
								&Route{
									Prefix: "/",
									object: ir14,
									httpServices: servicemap(
										httpService(s1),
									),
								},
							),
						},
					),
				}, &Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						&SecureVirtualHost{
							MinProtoVersion: auth.TlsParameters_TLSv1_1,
							VirtualHost: VirtualHost{
								Name: "foo.com",
								routes: routemap(
									&Route{
										Prefix: "/",
										object: ir14,
										httpServices: servicemap(
											httpService(s1),
										),
									}),
							},
							Secret: secret(sec1),
						},
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
						&VirtualHost{
							Name: "foo.com",
							routes: routemap(
								&Route{
									Prefix: "/",
									object: ir7,
									httpServices: servicemap(
										httpService(s1),
									),
									HTTPSUpgrade: true,
								},
							),
						},
					),
				}, &Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "foo.com",
								routes: routemap(
									&Route{
										Prefix: "/",
										object: ir7,
										httpServices: servicemap(
											httpService(s1),
										),
										HTTPSUpgrade: true,
									},
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
						&VirtualHost{
							Name: "foo.com",
							routes: routemap(&Route{
								Prefix: "/",
								object: ir8,
								httpServices: servicemap(
									httpService(s1),
								),
								HTTPSUpgrade: true,
							}),
						},
					),
				}, &Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "foo.com",
								routes: routemap(
									&Route{
										Prefix: "/",
										object: ir8,
										httpServices: servicemap(
											httpService(s1),
										),
										HTTPSUpgrade: true,
									},
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
						&VirtualHost{
							Name: "foo.com",
							routes: routemap(
								&Route{
									Prefix: "/",
									object: ir9,
									httpServices: servicemap(
										httpService(s1),
									),
									HTTPSUpgrade: true,
								}),
						},
					),
				}, &Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "foo.com",
								routes: routemap(
									&Route{
										Prefix: "/",
										object: ir9,
										httpServices: servicemap(
											httpService(s1),
										),
										HTTPSUpgrade: true,
									}),
							},
							MinProtoVersion: auth.TlsParameters_TLSv1_1,
							Secret:          secret(sec1),
						},
					),
				},
			),
		},
		"insert ingressroute referencing two backends, one missing": {
			objs: []interface{}{
				ir2, s2,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						&VirtualHost{
							Name: "example.com",
							routes: routemap(
								route("/", ir2, servicemap(
									httpService(s2),
								)),
							),
						},
					),
				},
			),
		},
		"insert ingressroute referencing two backends": {
			objs: []interface{}{
				ir2, s1, s2,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						&VirtualHost{
							Name: "example.com",
							routes: routemap(
								route("/", ir2, servicemap(
									httpService(s1),
									httpService(s2),
								)),
							),
						},
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
						&VirtualHost{
							Name: "b.example.com",
							routes: routemap(
								route("/", i10, servicemap(
									httpService(s1),
								)),
							),
						},
					),
				}, &Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "b.example.com",
								routes: routemap(
									route("/", i10, servicemap(
										httpService(s1),
									)),
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
						&VirtualHost{
							Name: "*",
							routes: routemap(
								route("/", i11, servicemap(
									httpService(s1),
								)),
								&Route{
									Prefix: "/ws1",
									object: i11,
									httpServices: servicemap(
										httpService(s1),
									),
									Websocket: true,
								},
							),
						},
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
						&VirtualHost{
							Name: "*",
							routes: routemap(
								&Route{
									Prefix: "/",
									object: i12a,
									httpServices: servicemap(
										httpService(s1),
									),
									Timeout: -1, // invalid timeout equals infinity ¯\_(ツ)_/¯.
								},
							),
						},
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
						&VirtualHost{
							Name: "*",
							routes: routemap(
								&Route{
									Prefix: "/",
									object: i12b,
									httpServices: servicemap(
										httpService(s1),
									),
									Timeout: 90 * time.Second,
								},
							),
						},
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
						&VirtualHost{
							Name: "*",
							routes: routemap(
								&Route{
									Prefix: "/",
									object: i12c,
									httpServices: servicemap(
										httpService(s1),
									),
									Timeout: -1,
								},
							),
						},
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
						&VirtualHost{
							Name: "example.com",
							routes: routemap(
								route("/blog", ir4, servicemap(
									httpService(s4),
								)),
								route("/blog/admin", ir5, servicemap(
									httpService(s5),
								)),
							),
						},
					),
				},
			),
		},
		"insert ingress with retry annotations": {
			objs: []interface{}{
				i14,
				s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						&VirtualHost{
							Name: "*",
							routes: routemap(
								&Route{
									Prefix: "/",
									object: i14,
									httpServices: servicemap(
										httpService(s1),
									),
									RetryOn:       "gateway-error",
									NumRetries:    6,
									PerTryTimeout: 10 * time.Second,
								},
							),
						},
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
						&VirtualHost{
							Name: "example.com",
							routes: routemap(
								&Route{
									Prefix: "/",
									object: i13a,
									httpServices: servicemap(
										httpService(s13a),
									),
									HTTPSUpgrade: true,
								},
								route("/.well-known/acme-challenge/gVJl5NWL2owUqZekjHkt_bo3OHYC2XNDURRRgLI5JTk", i13b, servicemap(
									httpService(s13b),
								)),
							),
						},
					),
				}, &Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "example.com",
								routes: routemap(
									&Route{
										Prefix: "/",
										object: i13a,
										httpServices: servicemap(
											httpService(s13a),
										),
										HTTPSUpgrade: true,
									},
									route("/.well-known/acme-challenge/gVJl5NWL2owUqZekjHkt_bo3OHYC2XNDURRRgLI5JTk", i13b, servicemap(
										httpService(s13b),
									)),
								),
							},
							MinProtoVersion: auth.TlsParameters_TLSv1_1,
							Secret:          secret(sec13),
						},
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
						&VirtualHost{
							Name: "*",
							routes: routemap(
								route("/", i3a, servicemap(
									&HTTPService{
										TCPService: TCPService{
											Name:        s3a.Name,
											Namespace:   s3a.Namespace,
											ServicePort: &s3a.Spec.Ports[0],
										},
										Protocol: "h2c",
									},
								)),
							),
						},
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
						&VirtualHost{
							Name: "*",
							routes: routemap(
								route("/", i3a, servicemap(
									&HTTPService{
										TCPService: TCPService{
											Name:        s3b.Name,
											Namespace:   s3b.Namespace,
											ServicePort: &s3b.Spec.Ports[0],
										},
										Protocol: "h2",
									},
								)),
							),
						},
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
						&VirtualHost{
							Name: "*",
							routes: routemap(
								route("/", i3a, servicemap(
									&HTTPService{
										TCPService: TCPService{
											Name:        s3c.Name,
											Namespace:   s3c.Namespace,
											ServicePort: &s3c.Spec.Ports[0],
										},
										Protocol: "tls",
									},
								)),
							),
						},
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
						&VirtualHost{
							Name: "*",
							routes: routemap(
								route("/", i1, servicemap(
									&HTTPService{
										TCPService: TCPService{
											Name:               s1b.Name,
											Namespace:          s1b.Namespace,
											ServicePort:        &s1b.Spec.Ports[0],
											MaxConnections:     9000,
											MaxPendingRequests: 4096,
											MaxRequests:        404,
											MaxRetries:         7,
										},
									},
								)),
							),
						},
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
						&VirtualHost{
							Name: "example.com",
							routes: routemap(
								route("/a", ir13, servicemap(
									&HTTPService{
										TCPService: TCPService{
											Name:        s1.Name,
											Namespace:   s1.Namespace,
											ServicePort: &s1.Spec.Ports[0],
											Weight:      90,
										},
									}),
								),
								route("/b", ir13, servicemap(
									&HTTPService{
										TCPService: TCPService{
											Name:        s1.Name,
											Namespace:   s1.Namespace,
											ServicePort: &s1.Spec.Ports[0],
											Weight:      60,
										},
									}),
								),
							),
						},
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
						&VirtualHost{
							Name: "example.com",
							routes: routemap(
								route("/a", ir13a, servicemap(
									&HTTPService{
										TCPService: TCPService{
											Name:        s1.Name,
											Namespace:   s1.Namespace,
											ServicePort: &s1.Spec.Ports[0],
											Weight:      90,
										},
									},
									&HTTPService{
										TCPService: TCPService{
											Name:        s1.Name,
											Namespace:   s1.Namespace,
											ServicePort: &s1.Spec.Ports[0],
											Weight:      60,
										},
									}),
								),
							),
						},
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
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						&VirtualHost{
							Name: "example.com",
							routes: routemap(&Route{Prefix: "/finance", object: &ingressroutev1.IngressRoute{
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
							}}),
						},
					),
				},
			),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			b := tc.Builder
			if b == nil {
				b = new(Builder)
			}

			for _, o := range tc.objs {
				b.Insert(o)
			}
			dag := b.Build()

			got := make(map[int]*Listener)
			dag.Visit(listenerMap(got).Visit)

			want := make(map[int]*Listener)
			for _, v := range tc.want {
				if l, ok := v.(*Listener); ok {
					want[l.Port] = l
				}
			}

			opts := []cmp.Option{
				cmp.AllowUnexported(Listener{}, VirtualHost{}, Route{}),
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

func secretdata(cert, key string) map[string][]byte {
	return map[string][]byte{
		v1.TLSCertKey:       []byte(cert),
		v1.TLSPrivateKeyKey: []byte(key),
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
	services := map[meta]*v1.Service{
		{name: "service1", namespace: "default"}: s1,
	}

	tests := map[string]struct {
		meta
		port        intstr.IntOrString
		weight      int
		strategy    string
		healthcheck *ingressroutev1.HealthCheck
		want        *HTTPService
	}{
		"lookup service by port number": {
			meta: meta{name: "service1", namespace: "default"},
			port: intstr.FromInt(8080),
			want: httpService(s1),
		},
		"lookup service by port name": {
			meta: meta{name: "service1", namespace: "default"},
			port: intstr.FromString("http"),
			want: httpService(s1),
		},
		"lookup service by port number (as string)": {
			meta: meta{name: "service1", namespace: "default"},
			port: intstr.Parse("8080"),
			want: httpService(s1),
		},
		"lookup service by port number (from string)": {
			meta: meta{name: "service1", namespace: "default"},
			port: intstr.FromString("8080"),
			want: httpService(s1),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			b := builder{
				source: &Builder{
					KubernetesCache: KubernetesCache{
						services: services,
					},
				},
			}
			got := b.lookupHTTPService(tc.meta, tc.port, tc.weight, tc.strategy, tc.healthcheck)
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

	tests := map[string]struct {
		rootNamespaces []string
		objs           []interface{}
		want           int
	}{
		"nil root namespaces": {
			objs: []interface{}{ir1},
			want: 1,
		},
		"empty root namespaces": {
			objs: []interface{}{ir1},
			want: 1,
		},
		"single root namespace with root ingressroute": {
			rootNamespaces: []string{"allowed1"},
			objs:           []interface{}{ir1},
			want:           1,
		},
		"multiple root namespaces, one with a root ingressroute": {
			rootNamespaces: []string{"foo", "allowed1", "bar"},
			objs:           []interface{}{ir1},
			want:           1,
		},
		"multiple root namespaces, each with a root ingressroute": {
			rootNamespaces: []string{"foo", "allowed1", "allowed2"},
			objs:           []interface{}{ir1, ir2},
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
			objs:           []interface{}{ir1, ir2},
			want:           1,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			b := Builder{
				KubernetesCache: KubernetesCache{
					IngressRouteRootNamespaces: tc.rootNamespaces,
				},
			}
			for _, o := range tc.objs {
				b.Insert(o)
			}
			dag := b.Build()

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
					Name: "foo",
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
					Name: "foo",
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

	tests := map[string]struct {
		objs []*ingressroutev1.IngressRoute
		want []Status
	}{
		"valid ingressroute": {
			objs: []*ingressroutev1.IngressRoute{ir1},
			want: []Status{{Object: ir1, Status: "valid", Description: "valid IngressRoute", Vhost: "example.com"}},
		},
		"invalid port in service": {
			objs: []*ingressroutev1.IngressRoute{ir2},
			want: []Status{{Object: ir2, Status: "invalid", Description: `route "/foo": service "home": port must be in the range 1-65535`, Vhost: "example.com"}},
		},
		"root ingressroute outside of roots namespace": {
			objs: []*ingressroutev1.IngressRoute{ir3},
			want: []Status{{Object: ir3, Status: "invalid", Description: "root IngressRoute cannot be defined in this namespace"}},
		},
		"delegated route's match prefix does not match parent's prefix": {
			objs: []*ingressroutev1.IngressRoute{ir1, ir4},
			want: []Status{
				{Object: ir1, Status: "valid", Description: "valid IngressRoute", Vhost: "example.com"},
				{Object: ir4, Status: "invalid", Description: `the path prefix "/doesnotmatch" does not match the parent's path prefix "/prefix"`, Vhost: "example.com"},
			},
		},
		"invalid weight in service": {
			objs: []*ingressroutev1.IngressRoute{ir5},
			want: []Status{{Object: ir5, Status: "invalid", Description: `route "/foo": service "home": weight must be greater than or equal to zero`, Vhost: "example.com"}},
		},
		"root ingressroute does not specify FQDN": {
			objs: []*ingressroutev1.IngressRoute{ir13},
			want: []Status{{Object: ir13, Status: "invalid", Description: "Spec.VirtualHost.Fqdn must be specified"}},
		},
		"self-edge produces a cycle": {
			objs: []*ingressroutev1.IngressRoute{ir6},
			want: []Status{{Object: ir6, Status: "invalid", Description: "route creates a delegation cycle: roots/self -> roots/self", Vhost: "example.com"}},
		},
		"child delegates to parent, producing a cycle": {
			objs: []*ingressroutev1.IngressRoute{ir7, ir8},
			want: []Status{
				{Object: ir7, Status: "valid", Description: "valid IngressRoute", Vhost: "example.com"},
				{Object: ir8, Status: "invalid", Description: "route creates a delegation cycle: roots/parent -> roots/child -> roots/parent", Vhost: "example.com"},
			},
		},
		"route has a list of services and also delegates": {
			objs: []*ingressroutev1.IngressRoute{ir9},
			want: []Status{{Object: ir9, Status: "invalid", Description: `route "/foo": cannot specify services and delegate in the same route`, Vhost: "example.com"}},
		},
		"ingressroute is an orphaned route": {
			objs: []*ingressroutev1.IngressRoute{ir8},
			want: []Status{{Object: ir8, Status: "orphaned", Description: "this IngressRoute is not part of a delegation chain from a root IngressRoute"}},
		},
		"ingressroute delegates to multiple ingressroutes, one is invalid": {
			objs: []*ingressroutev1.IngressRoute{ir10, ir11, ir12},
			want: []Status{
				{Object: ir11, Status: "valid", Description: "valid IngressRoute", Vhost: "example.com"},
				{Object: ir12, Status: "invalid", Description: `route "/bar": service "foo": port must be in the range 1-65535`, Vhost: "example.com"},
				{Object: ir10, Status: "valid", Description: "valid IngressRoute", Vhost: "example.com"},
			},
		},
		"invalid parent orphans children": {
			objs: []*ingressroutev1.IngressRoute{ir14, ir11},
			want: []Status{
				{Object: ir14, Status: "invalid", Description: "Spec.VirtualHost.Fqdn must be specified"},
				{Object: ir11, Status: "orphaned", Description: "this IngressRoute is not part of a delegation chain from a root IngressRoute"},
			},
		},
		"multi-parent children is not orphaned when one of the parents is invalid": {
			objs: []*ingressroutev1.IngressRoute{ir14, ir11, ir10},
			want: []Status{
				{Object: ir14, Status: "invalid", Description: "Spec.VirtualHost.Fqdn must be specified"},
				{Object: ir11, Status: "valid", Description: "valid IngressRoute", Vhost: "example.com"},
				{Object: ir10, Status: "valid", Description: "valid IngressRoute", Vhost: "example.com"},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			b := Builder{
				KubernetesCache: KubernetesCache{
					IngressRouteRootNamespaces: []string{"roots"},
				},
			}
			for _, o := range tc.objs {
				b.Insert(o)
			}
			got := b.Build().Statuses()
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

	tests := map[string]struct {
		objs       []interface{}
		want       []Vertex
		wantStatus []Status
	}{
		"insert ingressroute": {
			objs: []interface{}{
				ir1,
			},
			want: listeners(
				&Listener{
					Port: 10080,
					VirtualHosts: virtualhosts(
						&VirtualHost{
							Name: "example.com",
							routes: routemap(
								route("/", ir1),
							),
						},
					),
				},
			),
			wantStatus: []Status{
				{
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
			wantStatus: []Status{
				{
					Object:      ir1,
					Status:      StatusInvalid,
					Description: `fqdn "example.com" is used in multiple IngressRoutes: default/example-com, default/other-example`,
					Vhost:       "example.com",
				},
				{
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
			b := Builder{
				ExternalInsecurePort: 10080,
				ExternalSecurePort:   10443,
			}
			for _, o := range tc.objs {
				b.Insert(o)
			}
			dag := b.Build()
			got := make(map[int]*Listener)
			dag.Visit(listenerMap(got).Visit)

			want := make(map[int]*Listener)
			for _, v := range tc.want {
				if l, ok := v.(*Listener); ok {
					want[l.Port] = l
				}
			}

			opts := []cmp.Option{
				cmp.AllowUnexported(Listener{}, VirtualHost{}, Route{}),
			}
			if diff := cmp.Diff(want, got, opts...); diff != "" {
				t.Fatal(diff)
			}

			gotStatus := dag.statuses
			sort.Stable(statusByNamespaceAndName(gotStatus))
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
		want          meta
	}{
		"no namespace": {
			secret: "secret",
			defns:  "default",
			want: meta{
				name:      "secret",
				namespace: "default",
			},
		},
		"with namespace": {
			secret: "ns1/secret",
			defns:  "default",
			want: meta{
				name:      "secret",
				namespace: "ns1",
			},
		},
		"missing namespace": {
			secret: "/secret",
			defns:  "default",
			want: meta{
				name:      "secret",
				namespace: "default",
			},
		},
		"missing secret name": {
			secret: "secret/",
			defns:  "default",
			want: meta{
				name:      "",
				namespace: "secret",
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := splitSecret(tc.secret, tc.defns)
			opts := []cmp.Option{
				cmp.AllowUnexported(meta{}),
			}
			if diff := cmp.Diff(tc.want, got, opts...); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func routemap(routes ...*Route) map[string]*Route {
	m := make(map[string]*Route)
	for _, r := range routes {
		m[r.Prefix] = r
	}
	return m
}

func route(prefix string, obj interface{}, httpServices ...map[servicemeta]*HTTPService) *Route {
	route := Route{
		Prefix: prefix,
		object: obj,
	}
	switch len(httpServices) {
	case 0:
		// nothing to do
	case 1:
		route.httpServices = httpServices[0]
	default:
		panic("only pass one servicemap to route")
	}
	return &route
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

func servicemap(services ...*HTTPService) map[servicemeta]*HTTPService {
	m := make(map[servicemeta]*HTTPService)
	for _, s := range services {
		m[s.toMeta()] = s
	}
	return m
}

type statusByNamespaceAndName []Status

func (s statusByNamespaceAndName) Len() int      { return len(s) }
func (s statusByNamespaceAndName) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s statusByNamespaceAndName) Less(i, j int) bool {
	return s[i].Object.Namespace+s[i].Object.Name < s[j].Object.Namespace+s[j].Object.Name
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
		}
	}
	return m
}

func listeners(ls ...*Listener) []Vertex {
	var v []Vertex
	for _, l := range ls {
		v = append(v, l)
	}
	return v
}
