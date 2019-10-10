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

package dag

import (
	"testing"
	"time"

	envoy_api_v2_auth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	"github.com/google/go-cmp/cmp"
	ingressroutev1 "github.com/projectcontour/contour/apis/contour/v1beta1"
	projcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
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
				SecretName: sec1.Name,
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
				SecretName: sec1.Name,
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
				SecretName: sec1.Name,
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
				SecretName: sec1.Name,
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
				SecretName: sec1.Name,
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
				SecretName: sec1.Name,
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
				SecretName: sec1.Name,
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
				SecretName: sec1.Name,
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
				SecretName: sec1.Name,
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

	i12d := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "timeout",
			Namespace: "default",
			Annotations: map[string]string{
				"projectcontour.io/response-timeout": "peanut",
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

	i12e := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "timeout",
			Namespace: "default",
			Annotations: map[string]string{
				"projectcontour.io/response-timeout": "1m30s", // 90 seconds y'all
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

	i12f := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "timeout",
			Namespace: "default",
			Annotations: map[string]string{
				"projectcontour.io/response-timeout": "infinite",
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

	i14a := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "timeout",
			Namespace: "default",
			Annotations: map[string]string{
				"contour.heptio.com/retry-on":        "gateway-error",
				"projectcontour.io/num-retries":      "6",
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
	i14b := &v1beta1.Ingress{
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
	i14c := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "timeout",
			Namespace: "default",
			Annotations: map[string]string{
				"contour.heptio.com/retry-on":       "gateway-error",
				"projectcontour.io/num-retries":     "6",
				"projectcontour.io/per-try-timeout": "10s",
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

	i16 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "wildcards",
			Namespace: "default",
		},
		Spec: v1beta1.IngressSpec{
			Rules: []v1beta1.IngressRule{{
				// no hostname
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Backend: v1beta1.IngressBackend{
								ServiceName: "kuard",
								ServicePort: intstr.FromString("http")},
						}},
					},
				},
			}, {
				Host: "*",
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
				Host: "*.example.com",
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
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
			Name:      s3a.Name,
			Namespace: s3a.Namespace,
			Annotations: map[string]string{
				"contour.heptio.com/upstream-protocol.h2": "80,http",
			},
		},
		Spec: s3a.Spec,
	}

	s3c := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s3b.Name,
			Namespace: s3b.Namespace,
			Annotations: map[string]string{
				"contour.heptio.com/upstream-protocol.tls": "80,http",
			},
		},
		Spec: s3b.Spec,
	}

	s3d := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s3b.Name,
			Namespace: s3b.Namespace,
			Annotations: map[string]string{
				"projectcontour.io/upstream-protocol.h2c": "80,http",
			},
		},
		Spec: s3b.Spec,
	}

	s3e := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s3b.Name,
			Namespace: s3b.Namespace,
			Annotations: map[string]string{
				"projectcontour.io/upstream-protocol.h2": "80,http",
			},
		},
		Spec: s3b.Spec,
	}

	s3f := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s3b.Name,
			Namespace: s3b.Namespace,
			Annotations: map[string]string{
				"projectcontour.io/upstream-protocol.tls": "80,http",
			},
		},
		Spec: s3b.Spec,
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
			VirtualHost: &projcontour.VirtualHost{
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
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "kuard.example.com",
				TLS: &projcontour.TLS{
					SecretName: sec1.Name,
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

	// ir1b tcp forwards traffic to default/kuard:8080 by TLS pass-throughing
	// it.
	ir1b := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard-tcp",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "kuard.example.com",
				TLS: &projcontour.TLS{
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
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "kuard.example.com",
				TLS: &projcontour.TLS{
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
			VirtualHost: &projcontour.VirtualHost{
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
			VirtualHost: &projcontour.VirtualHost{
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
			VirtualHost: &projcontour.VirtualHost{
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
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "foo.com",
				TLS: &projcontour.TLS{
					SecretName: sec1.Name,
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
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "foo.com",
				TLS: &projcontour.TLS{
					SecretName:             sec1.Name,
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
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "foo.com",
				TLS: &projcontour.TLS{
					SecretName:             sec1.Name,
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
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "foo.com",
				TLS: &projcontour.TLS{
					SecretName:             sec1.Name,
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
			VirtualHost: &projcontour.VirtualHost{
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
			VirtualHost: &projcontour.VirtualHost{
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
			VirtualHost: &projcontour.VirtualHost{
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
			VirtualHost: &projcontour.VirtualHost{
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
			VirtualHost: &projcontour.VirtualHost{
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
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "foo.com",
				TLS: &projcontour.TLS{
					SecretName: sec1.Name,
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
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "bar.com",
			},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				RetryPolicy: &projcontour.RetryPolicy{
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
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "bar.com",
			},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				RetryPolicy: &projcontour.RetryPolicy{
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
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "bar.com",
			},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				RetryPolicy: &projcontour.RetryPolicy{
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
			VirtualHost: &projcontour.VirtualHost{
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
			VirtualHost: &projcontour.VirtualHost{
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
			VirtualHost: &projcontour.VirtualHost{
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
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Services: []ingressroutev1.Service{{
					Name: "kuard",
					Port: 8080,
					UpstreamValidation: &projcontour.UpstreamValidation{
						CACertificate: cert1.Name,
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

	s8 := &v1.Service{
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

	s9 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol: "TCP",
				Port:     80,
			}},
		},
	}

	s10 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tls-passthrough",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "https",
				Protocol:   "TCP",
				Port:       443,
				TargetPort: intstr.FromInt(443),
			}, {
				Name:       "http",
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(80),
			}},
		},
	}

	s11 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "blog",
			Namespace: "it",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:     "blog",
				Protocol: "TCP",
				Port:     8080,
			}},
		},
	}

	// ir18 tcp forwards traffic to by TLS pass-throughing
	// it. It also exposes non HTTP traffic to the the non secure port of the
	// application so it can give an informational message
	ir18 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard-tcp",
			Namespace: s10.Namespace,
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "kuard.example.com",
				TLS: &projcontour.TLS{
					Passthrough: true,
				},
			},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Services: []ingressroutev1.Service{{
					Name: s10.Name,
					Port: 80, // proxy non secure traffic to port 80
				}},
			}},
			TCPProxy: &ingressroutev1.TCPProxy{
				Services: []ingressroutev1.Service{{
					Name: s10.Name,
					Port: 443, // ssl passthrough to secure port
				}},
			},
		},
	}

	ir19 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-with-tls-delegation",
			Namespace: s10.Namespace,
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "app-with-tls-delegation.127.0.0.1.nip.io",
				TLS: &projcontour.TLS{
					SecretName: "heptio-contour/ssl-cert", // not delegated
				},
			},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Services: []ingressroutev1.Service{{
					Name: s10.Name,
					Port: 80,
				}},
			}},
		},
	}

	proxy1 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []projcontour.Route{{
				Conditions: []projcontour.Condition{{
					Prefix: "/",
				}},
				Services: []projcontour.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	// proxy1a tcp forwards traffic to default/kuard:8080 by TLS pass-through it.
	proxy1a := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard-tcp",
			Namespace: "default",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "kuard.example.com",
				TLS: &projcontour.TLS{
					Passthrough: true,
				},
			},
			TCPProxy: &projcontour.TCPProxy{
				Services: []projcontour.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			},
		},
	}

	proxy1b := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}
	proxy1c := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []projcontour.Route{{
				Conditions: []projcontour.Condition{{
					Header: &projcontour.HeaderCondition{
						Name:    "x-request-id",
						Present: true,
					},
				}, {
					Prefix: "/kuard",
				}, {
					Header: &projcontour.HeaderCondition{
						Name:     "e-tag",
						Contains: "abcdef",
					},
				}, {
					Header: &projcontour.HeaderCondition{
						Name:        "x-timeout",
						NotContains: "infinity",
					},
				}, {
					Header: &projcontour.HeaderCondition{
						Name:  "digest-auth",
						Exact: "scott",
					},
				}, {
					Header: &projcontour.HeaderCondition{
						Name:     "digest-password",
						NotExact: "tiger",
					},
				}},
				Services: []projcontour.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	proxy2a := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "kubesystem",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []projcontour.Include{{
				Conditions: []projcontour.Condition{{
					Header: &projcontour.HeaderCondition{
						Name:    "x-request-id",
						Present: true,
					},
				}, {
					Header: &projcontour.HeaderCondition{
						Name:        "x-timeout",
						NotContains: "infinity",
					},
				}, {
					Header: &projcontour.HeaderCondition{
						Name:  "digest-auth",
						Exact: "scott",
					},
				}},
				Name:      "kuard",
				Namespace: "default",
			}},
		},
	}
	proxy2b := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: projcontour.HTTPProxySpec{
			Routes: []projcontour.Route{{
				Conditions: []projcontour.Condition{{
					Prefix: "/kuard",
				}, {
					Header: &projcontour.HeaderCondition{
						Name:     "e-tag",
						Contains: "abcdef",
					},
				}, {
					Header: &projcontour.HeaderCondition{
						Name:     "digest-password",
						NotExact: "tiger",
					},
				}},
				Services: []projcontour.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	proxy1e := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []projcontour.Route{{
				Conditions: []projcontour.Condition{{
					Prefix: "/",
				}},
				HealthCheckPolicy: &projcontour.HTTPHealthCheckPolicy{
					Path: "/healthz",
				},
				Services: []projcontour.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	// proxy6 has TLS and does not specify min tls version
	proxy6 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "foo.com",
				TLS: &projcontour.TLS{
					SecretName: sec1.Name,
				},
			},
			Routes: []projcontour.Route{{
				Conditions: []projcontour.Condition{{
					Prefix: "/",
				}},
				Services: []projcontour.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	proxy17 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []projcontour.Route{{
				Conditions: []projcontour.Condition{{
					Prefix: "/",
				}},
				Services: []projcontour.Service{{
					Name: "kuard",
					Port: 8080,
					UpstreamValidation: &projcontour.UpstreamValidation{
						CACertificate: cert1.Name,
						SubjectName:   "example.com",
					},
				}},
			}},
		},
	}

	// proxy10 has a websocket route
	proxy10 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []projcontour.Route{{
				Conditions: []projcontour.Condition{{
					Prefix: "/",
				}},
				Services: []projcontour.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}, {
				Conditions: []projcontour.Condition{{
					Prefix: "/websocket",
				}},
				EnableWebsockets: true,
				Services: []projcontour.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	// proxy10b has a websocket route w/multiple upstreams
	proxy10b := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []projcontour.Route{{
				Conditions: []projcontour.Condition{{
					Prefix: "/",
				}},
				Services: []projcontour.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}, {
				Conditions: []projcontour.Condition{{
					Prefix: "/websocket",
				}},
				EnableWebsockets: true,
				Services: []projcontour.Service{{
					Name: "kuard",
					Port: 8080,
				}, {
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	proxy12 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: s1.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []projcontour.Route{{
				Conditions: []projcontour.Condition{{
					Prefix: "/",
				}},
				Services: []projcontour.Service{{
					Name: s1.Name,
					Port: 8080,
				}, {
					Name:   s2.Name,
					Port:   8080,
					Mirror: true,
				}},
			}},
		},
	}

	proxy13 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: s1.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []projcontour.Route{{
				Conditions: []projcontour.Condition{{
					Prefix: "/",
				}},
				Services: []projcontour.Service{{
					Name: s1.Name,
					Port: 8080,
				}, {
					Name:   s2.Name,
					Port:   8080,
					Mirror: true,
				}, {
					// it is legal to mention a service more that
					// once, however it is not legal for more than one
					// service to be marked as mirror.
					Name:   s2.Name,
					Port:   8080,
					Mirror: true,
				}},
			}},
		},
	}

	proxy100 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: s1.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []projcontour.Include{{
				Name:      "marketingwww",
				Namespace: "marketing",
				Conditions: []projcontour.Condition{{
					Prefix: "/blog",
				}},
			}},
			Routes: []projcontour.Route{{
				Conditions: []projcontour.Condition{{
					Prefix: "/",
				}},
				Services: []projcontour.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxy100a := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "marketingwww",
			Namespace: "marketing",
		},
		Spec: projcontour.HTTPProxySpec{
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: "blog",
					Port: 8080,
				}},
			}},
		},
	}

	proxy100b := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "marketingwww",
			Namespace: "marketing",
		},
		Spec: projcontour.HTTPProxySpec{
			Routes: []projcontour.Route{{
				Conditions: []projcontour.Condition{{
					Prefix: "/infotech",
				}},
				Services: []projcontour.Service{{
					Name: "blog",
					Port: 8080,
				}},
			}},
		},
	}

	proxy100c := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "marketingwww",
			Namespace: "marketing",
		},
		Spec: projcontour.HTTPProxySpec{
			Includes: []projcontour.Include{{
				Name:      "marketingit",
				Namespace: "it",
				Conditions: []projcontour.Condition{{
					Prefix: "/it",
				}},
			}},
			Routes: []projcontour.Route{{
				Conditions: []projcontour.Condition{{
					Prefix: "/infotech",
				}},
				Services: []projcontour.Service{{
					Name: "blog",
					Port: 8080,
				}},
			}, {
				Services: []projcontour.Service{{
					Name: "blog",
					Port: 8080,
				}},
			}},
		},
	}

	proxy100d := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "marketingit",
			Namespace: "it",
		},
		Spec: projcontour.HTTPProxySpec{
			Routes: []projcontour.Route{{
				Conditions: []projcontour.Condition{{
					Prefix: "/foo",
				}},
				Services: []projcontour.Service{{
					Name: "blog",
					Port: 8080,
				}},
			}},
		},
	}

	// proxy101 and proxy101a test inclusion without a specified namespace.
	proxy101 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: s1.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []projcontour.Include{{
				Name: "kuarder",
				Conditions: []projcontour.Condition{{
					Prefix: "/kuarder",
				}},
			}},
			Routes: []projcontour.Route{{
				Conditions: []projcontour.Condition{{
					Prefix: "/",
				}},
				Services: []projcontour.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxy101a := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuarder",
			Namespace: proxy101.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: s2.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxy102 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: s1.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []projcontour.Route{{
				Conditions: []projcontour.Condition{{
					Prefix: "/v1",
				}, {
					Prefix: "/api",
				}},
				Services: []projcontour.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxy103 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: s1.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []projcontour.Include{{
				Name:      "www",
				Namespace: "teama",
				Conditions: []projcontour.Condition{{
					Prefix: "/v1",
				}, {
					Prefix: "/api",
				}},
			}},
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxy103a := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "www",
			Namespace: "teama",
		},
		Spec: projcontour.HTTPProxySpec{
			Routes: []projcontour.Route{{
				Conditions: []projcontour.Condition{{
					Prefix: "/v1",
				}, {
					Prefix: "/api",
				}},
				Services: []projcontour.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxy104 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: s1.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []projcontour.Include{{
				Name: "kuarder",
				Conditions: []projcontour.Condition{{
					Prefix: "/kuarder",
				}},
			}},
			Routes: []projcontour.Route{{
				Conditions: []projcontour.Condition{{
					Prefix: "/",
				}},
				Services: []projcontour.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxy104a := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuarder",
			Namespace: proxy104.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: s2.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxy105 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: s1.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []projcontour.Include{{
				Name: "kuarder",
				Conditions: []projcontour.Condition{{
					Prefix: "/kuarder",
				}},
			}},
			Routes: []projcontour.Route{{
				Conditions: []projcontour.Condition{{
					Prefix: "/",
				}},
				Services: []projcontour.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxy105a := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuarder",
			Namespace: proxy105.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			Routes: []projcontour.Route{{
				Conditions: []projcontour.Condition{{
					Prefix: "/",
				}},
				Services: []projcontour.Service{{
					Name: s2.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxy106 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: s1.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []projcontour.Include{{
				Name: "kuarder",
				Conditions: []projcontour.Condition{{
					Prefix: "/kuarder/",
				}},
			}},
			Routes: []projcontour.Route{{
				Conditions: []projcontour.Condition{{
					Prefix: "/",
				}},
				Services: []projcontour.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxy106a := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuarder",
			Namespace: proxy105.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			Routes: []projcontour.Route{{
				Conditions: []projcontour.Condition{{
					Prefix: "/",
				}},
				Services: []projcontour.Service{{
					Name: s2.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxy107 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: s1.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []projcontour.Include{{
				Name: "kuarder",
				Conditions: []projcontour.Condition{{
					Prefix: "/kuarder",
				}},
			}},
			Routes: []projcontour.Route{{
				Conditions: []projcontour.Condition{{
					Prefix: "/",
				}},
				Services: []projcontour.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxy107a := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuarder",
			Namespace: proxy105.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			Routes: []projcontour.Route{{
				Conditions: []projcontour.Condition{{
					Prefix: "/withavengeance",
				}},
				Services: []projcontour.Service{{
					Name: s2.Name,
					Port: 8080,
				}},
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
						virtualhost("*", prefixroute("/", service(s1))),
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
						virtualhost("*", prefixroute("/", service(s1))),
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
						virtualhost("kuard.example.com", prefixroute("/", service(s1))),
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
						virtualhost("*", prefixroute("/", service(s1))),
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
						virtualhost("*", prefixroute("/", service(s1))),
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
						virtualhost("*", prefixroute("/", service(s1))),
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
						virtualhost("*", prefixroute("/", service(s1))),
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
						virtualhost("*", prefixroute("/", service(s1))),
					),
				},
			),
		},
		"insert secret then ingress w/ tls": {
			objs: []interface{}{
				sec1,
				i3,
			},
			want: listeners(),
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
						virtualhost("kuard.example.com", prefixroute("/", service(s1))),
					),
				},
				&Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						securevirtualhost("kuard.example.com", sec1, prefixroute("/", service(s1))),
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
						virtualhost("kuard.example.com", prefixroute("/", service(s1))),
					),
				},
				&Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						securevirtualhost("kuard.example.com", sec3, prefixroute("/", service(s1))),
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
						virtualhost("*", prefixroute("/", service(s1))),
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
						virtualhost("kuard.example.com", prefixroute("/", service(s1))),
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
						virtualhost("a.example.com", prefixroute("/", service(s1))),
						virtualhost("b.example.com", prefixroute("/", service(s1))),
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
						virtualhost("a.example.com", prefixroute("/", service(s1))),
						virtualhost("b.example.com", prefixroute("/", service(s1))),
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
						virtualhost("a.example.com", prefixroute("/", service(s1))),
						virtualhost("b.example.com", prefixroute("/", service(s1))),
					),
				}, &Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						securevirtualhost("b.example.com", sec1, prefixroute("/", service(s1))),
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
						virtualhost("a.example.com", prefixroute("/", service(s1))),
						virtualhost("b.example.com", prefixroute("/", service(s1))),
					),
				}, &Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						securevirtualhost("b.example.com", sec1, prefixroute("/", service(s1))),
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
							prefixroute("/", service(s1)),
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
							prefixroute("/", service(s1)),
							prefixroute("/kuarder", service(s2)),
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
							prefixroute("/", service(s1)),
							prefixroute("/kuarder", service(s2)),
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
							prefixroute("/", service(s1)),
							prefixroute("/kuarder", service(s2)),
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
						securevirtualhost("b.example.com", sec1, prefixroute("/", service(s1))),
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
						virtualhost("b.example.com", routeUpgrade("/", service(s1))),
					),
				}, &Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						securevirtualhost("b.example.com", sec1, routeUpgrade("/", service(s1))),
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
						virtualhost("b.example.com", routeUpgrade("/", service(s1))),
					),
				}, &Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						securevirtualhost("b.example.com", sec1, routeUpgrade("/", service(s1))),
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
						virtualhost("example.com", prefixroute("/", service(s1))),
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
								Upstream: service(s1),
								HealthCheckPolicy: &HealthCheckPolicy{
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
							prefixroute("/", service(s1)),
							routeRewrite("/websocket", "/", service(s1)),
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
							},
							TCPProxy: &TCPProxy{
								Clusters: clusters(
									service(s1),
								),
							},
							Secret:          secret(sec1),
							MinProtoVersion: envoy_api_v2_auth.TlsParameters_TLSv1_1,
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
							},
							TCPProxy: &TCPProxy{
								Clusters: clusters(
									service(s1),
								),
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
							},
							TCPProxy: &TCPProxy{
								Clusters: clusters(
									service(s6),
								),
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
							prefixroute("/", service(s1)),
							routeWebsocket("/websocket", service(s1)),
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
							prefixroute("/", service(s1)),
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
						virtualhost("example.com", prefixroute("/", service(s1))),
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
						virtualhost("foo.com", routeUpgrade("/", service(s1))),
					),
				}, &Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						securevirtualhost("foo.com", sec1, routeUpgrade("/", service(s1))),
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
						virtualhost("foo.com", prefixroute("/", service(s1))),
					),
				}, &Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						securevirtualhost("foo.com", sec1, prefixroute("/", service(s1))),
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
						virtualhost("foo.com", routeUpgrade("/", service(s1))),
					),
				}, &Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						securevirtualhost("foo.com", sec1, routeUpgrade("/", service(s1))),
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
						virtualhost("foo.com", routeUpgrade("/", service(s1))),
					),
				}, &Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "foo.com",
								routes: routes(
									routeUpgrade("/", service(s1)),
								),
							},
							MinProtoVersion: envoy_api_v2_auth.TlsParameters_TLSv1_2,
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
						virtualhost("foo.com", routeUpgrade("/", service(s1))),
					),
				}, &Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "foo.com",
								routes: routes(
									routeUpgrade("/", service(s1)),
								),
							},
							MinProtoVersion: envoy_api_v2_auth.TlsParameters_TLSv1_3,
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
						virtualhost("foo.com", routeUpgrade("/", service(s1))),
					),
				}, &Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						securevirtualhost("foo.com", sec1, routeUpgrade("/", service(s1))),
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
						virtualhost("example.com", prefixroute("/", service(s1), service(s2))),
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
						virtualhost("b.example.com", prefixroute("/", service(s1))),
					),
				}, &Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "b.example.com",
								routes: routes(
									prefixroute("/", service(s1)),
								),
							},
							MinProtoVersion: envoy_api_v2_auth.TlsParameters_TLSv1_3,
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
							prefixroute("/", service(s1)),
							routeWebsocket("/ws1", service(s1)),
						),
					),
				},
			),
		},
		"insert ingress w/ invalid legacy timeout annotation": {
			objs: []interface{}{
				i12a,
				s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*", &Route{
							PathCondition: prefix("/"),
							Clusters:      clustermap(s1),
							TimeoutPolicy: &TimeoutPolicy{
								ResponseTimeout: -1, // invalid timeout equals infinity ¯\_(ツ)_/¯.
							},
						}),
					),
				},
			),
		},
		"insert ingress w/ invalid timeout annotation": {
			objs: []interface{}{
				i12d,
				s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*", &Route{
							PathCondition: prefix("/"),
							Clusters:      clustermap(s1),
							TimeoutPolicy: &TimeoutPolicy{
								ResponseTimeout: -1, // invalid timeout equals infinity ¯\_(ツ)_/¯.
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
						virtualhost("bar.com", &Route{
							PathCondition: prefix("/"),
							Clusters:      clustermap(s1),
							TimeoutPolicy: &TimeoutPolicy{
								ResponseTimeout: -1, // invalid timeout equals infinity ¯\_(ツ)_/¯.
							},
						}),
					),
				},
			),
		},
		"insert ingress w/ valid legacy timeout annotation": {
			objs: []interface{}{
				i12b,
				s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*", &Route{
							PathCondition: prefix("/"),
							Clusters:      clustermap(s1),
							TimeoutPolicy: &TimeoutPolicy{
								ResponseTimeout: 90 * time.Second,
							},
						}),
					),
				},
			),
		},
		"insert ingress w/ valid timeout annotation": {
			objs: []interface{}{
				i12e,
				s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*", &Route{
							PathCondition: prefix("/"),
							Clusters:      clustermap(s1),
							TimeoutPolicy: &TimeoutPolicy{
								ResponseTimeout: 90 * time.Second,
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
						virtualhost("bar.com", &Route{
							PathCondition: prefix("/"),
							Clusters:      clustermap(s1),
							TimeoutPolicy: &TimeoutPolicy{
								ResponseTimeout: 90 * time.Second,
							},
						}),
					),
				},
			),
		},
		"insert ingress w/ legacy infinite timeout annotation": {
			objs: []interface{}{
				i12c,
				s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*", &Route{
							PathCondition: prefix("/"),
							Clusters:      clustermap(s1),
							TimeoutPolicy: &TimeoutPolicy{
								ResponseTimeout: -1,
							},
						}),
					),
				},
			),
		},
		"insert ingress w/ infinite timeout annotation": {
			objs: []interface{}{
				i12f,
				s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*", &Route{
							PathCondition: prefix("/"),
							Clusters:      clustermap(s1),
							TimeoutPolicy: &TimeoutPolicy{
								ResponseTimeout: -1,
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
						virtualhost("bar.com", &Route{
							PathCondition: prefix("/"),
							Clusters:      clustermap(s1),
							TimeoutPolicy: &TimeoutPolicy{
								ResponseTimeout: -1,
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
							prefixroute("/", service(s1)),
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
									Upstream: &Service{
										Name:        s1a.Name,
										Namespace:   s1a.Namespace,
										ServicePort: &s1a.Spec.Ports[0],
										Protocol:    "tls",
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
									Upstream: &Service{
										Name:        s1a.Name,
										Namespace:   s1a.Namespace,
										ServicePort: &s1a.Spec.Ports[0],
										Protocol:    "tls",
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
		"insert ingressroute routing and tcpproxying": {
			objs: []interface{}{
				s10, ir18,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost(ir18.Spec.VirtualHost.Fqdn,
							routeCluster("/",
								&Cluster{
									Upstream: &Service{
										Name:        s10.Name,
										Namespace:   s10.Namespace,
										ServicePort: &s10.Spec.Ports[1],
									},
								},
							),
						),
					),
				},
				&Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: ir18.Spec.VirtualHost.Fqdn,
							},
							TCPProxy: &TCPProxy{
								Clusters: clusters(service(s10)),
							},
							MinProtoVersion: envoy_api_v2_auth.TlsParameters_TLS_AUTO, // tls passthrough does not specify a TLS version; that's the domain of the backend
						},
					),
				},
			),
		},
		"insert ingressroute with missing tls delegation should not present port 80": {
			objs: []interface{}{
				s10, ir19,
			},
			want: listeners(), // no listeners, ir19 is invalid
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
							prefixroute("/blog", service(s4)),
							prefixroute("/blog/admin", service(s5)),
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
						virtualhost("bar.com", &Route{
							PathCondition: prefix("/"),
							Clusters:      clustermap(s1),
							RetryPolicy: &RetryPolicy{
								RetryOn:       "5xx",
								NumRetries:    6,
								PerTryTimeout: 10 * time.Second,
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
						virtualhost("bar.com", &Route{
							PathCondition: prefix("/"),
							Clusters:      clustermap(s1),
							RetryPolicy: &RetryPolicy{
								RetryOn:       "5xx",
								NumRetries:    6,
								PerTryTimeout: 0,
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
						virtualhost("bar.com", &Route{
							PathCondition: prefix("/"),
							Clusters:      clustermap(s1),
							RetryPolicy: &RetryPolicy{
								RetryOn:       "5xx",
								NumRetries:    1,
								PerTryTimeout: 10 * time.Second,
							},
						}),
					),
				},
			),
		},
		"insert ingressroute with retrypolicy": {
			objs: []interface{}{
				i14a,
				s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*", &Route{
							PathCondition: prefix("/"),
							Clusters:      clustermap(s1),
							RetryPolicy: &RetryPolicy{
								RetryOn:       "gateway-error",
								NumRetries:    6,
								PerTryTimeout: 10 * time.Second,
							},
						}),
					),
				},
			),
		},
		"insert ingressroute with legacy retrypolicy": {
			objs: []interface{}{
				i14b,
				s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*", &Route{
							PathCondition: prefix("/"),
							Clusters:      clustermap(s1),
							RetryPolicy: &RetryPolicy{
								RetryOn:       "gateway-error",
								NumRetries:    6,
								PerTryTimeout: 10 * time.Second,
							},
						}),
					),
				},
			),
		},
		"insert ingressroute with timeout policy": {
			objs: []interface{}{
				i14c,
				s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*", &Route{
							PathCondition: prefix("/"),
							Clusters:      clustermap(s1),
							RetryPolicy: &RetryPolicy{
								RetryOn:       "gateway-error",
								NumRetries:    6,
								PerTryTimeout: 10 * time.Second,
							},
						}),
					),
				},
			),
		},

		"insert ingress with regex route": {
			objs: []interface{}{
				i15,
				s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*", &Route{
							PathCondition: regex("/[^/]+/invoices(/.*|/?)"),
							Clusters:      clustermap(s1),
						}),
					),
				},
			),
		},
		// issue 1234
		"insert ingress with wildcard hostnames": {
			objs: []interface{}{
				s1,
				i16,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*", prefixroute("/", service(s1))),
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
							routeUpgrade("/", service(s13a)),
							prefixroute("/.well-known/acme-challenge/gVJl5NWL2owUqZekjHkt_bo3OHYC2XNDURRRgLI5JTk", service(s13b)),
						),
					),
				}, &Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						securevirtualhost("example.com", sec13,
							routeUpgrade("/", service(s13a)),
							prefixroute("/.well-known/acme-challenge/gVJl5NWL2owUqZekjHkt_bo3OHYC2XNDURRRgLI5JTk", service(s13b)),
						),
					),
				},
			),
		},
		"deprecated h2c service annotation": {
			objs: []interface{}{
				i3a, s3a,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*",
							prefixroute("/", &Service{
								Name:        s3a.Name,
								Namespace:   s3a.Namespace,
								ServicePort: &s3a.Spec.Ports[0],
								Protocol:    "h2c",
							}),
						),
					),
				},
			),
		},
		"deprecated h2 service annotation": {
			objs: []interface{}{
				i3a, s3b,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*",
							prefixroute("/", &Service{
								Name:        s3b.Name,
								Namespace:   s3b.Namespace,
								ServicePort: &s3b.Spec.Ports[0],
								Protocol:    "h2",
							}),
						),
					),
				},
			),
		},
		"deprecated tls service annotation": {
			objs: []interface{}{
				i3a, s3c,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*",
							prefixroute("/", &Service{
								Name:        s3c.Name,
								Namespace:   s3c.Namespace,
								ServicePort: &s3c.Spec.Ports[0],
								Protocol:    "tls",
							}),
						),
					),
				},
			),
		},

		"h2c service annotation": {
			objs: []interface{}{
				i3a, s3d,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*",
							prefixroute("/", &Service{
								Name:        s3a.Name,
								Namespace:   s3a.Namespace,
								ServicePort: &s3a.Spec.Ports[0],
								Protocol:    "h2c",
							}),
						),
					),
				},
			),
		},
		"h2 service annotation": {
			objs: []interface{}{
				i3a, s3e,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*",
							prefixroute("/", &Service{
								Name:        s3b.Name,
								Namespace:   s3b.Namespace,
								ServicePort: &s3b.Spec.Ports[0],
								Protocol:    "h2",
							}),
						),
					),
				},
			),
		},
		"tls service annotation": {
			objs: []interface{}{
				i3a, s3f,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*",
							prefixroute("/", &Service{
								Name:        s3c.Name,
								Namespace:   s3c.Namespace,
								ServicePort: &s3c.Spec.Ports[0],
								Protocol:    "tls",
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
							prefixroute("/", &Service{
								Name:               s1b.Name,
								Namespace:          s1b.Namespace,
								ServicePort:        &s1b.Spec.Ports[0],
								MaxConnections:     9000,
								MaxPendingRequests: 4096,
								MaxRequests:        404,
								MaxRetries:         7,
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
								Upstream: &Service{
									Name:        s1.Name,
									Namespace:   s1.Namespace,
									ServicePort: &s1.Spec.Ports[0],
								},
								Weight: 90,
							}),
							routeCluster("/b", &Cluster{
								Upstream: &Service{
									Name:        s1.Name,
									Namespace:   s1.Namespace,
									ServicePort: &s1.Spec.Ports[0],
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
									Upstream: &Service{
										Name:        s1.Name,
										Namespace:   s1.Namespace,
										ServicePort: &s1.Spec.Ports[0],
									},
									Weight: 90,
								}, &Cluster{
									Upstream: &Service{
										Name:        s1.Name,
										Namespace:   s1.Namespace,
										ServicePort: &s1.Spec.Ports[0],
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
						VirtualHost: &projcontour.VirtualHost{
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
						VirtualHost: &projcontour.VirtualHost{
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
						VirtualHost: &projcontour.VirtualHost{
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
						VirtualHost: &projcontour.VirtualHost{
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
						VirtualHost: &projcontour.VirtualHost{
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
						virtualhost("example.com", prefixroute("/finance", service(s7))),
					),
				},
			),
		},
		"ingressroute root delegates to another ingressroute root": {
			objs: []interface{}{
				&ingressroutev1.IngressRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "root-blog",
						Namespace: "roots",
					},
					Spec: ingressroutev1.IngressRouteSpec{
						VirtualHost: &projcontour.VirtualHost{
							Fqdn: "blog.containersteve.com",
						},
						Routes: []ingressroutev1.Route{{
							Match: "/",
							Delegate: &ingressroutev1.Delegate{
								Name:      "blog",
								Namespace: "marketing",
							},
						}},
					},
				},
				&ingressroutev1.IngressRoute{
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
								Name: "green",
								Port: 80,
							}},
						}},
					},
				},
				s8,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("www.containersteve.com", prefixroute("/", service(s8))),
					),
				},
			),
		},
		// issue 1399
		"service shared across ingress and ingressroute tcpproxy": {
			objs: []interface{}{
				sec1,
				s9,
				&v1beta1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "nginx",
						Namespace: "default",
					},
					Spec: v1beta1.IngressSpec{
						TLS: []v1beta1.IngressTLS{{
							Hosts:      []string{"example.com"},
							SecretName: s1.Name,
						}},
						Rules: []v1beta1.IngressRule{{
							Host:             "example.com",
							IngressRuleValue: ingressrulevalue(backend(s9.Name, intstr.FromInt(80))),
						}},
					},
				},
				&ingressroutev1.IngressRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "nginx",
						Namespace: "default",
					},
					Spec: ingressroutev1.IngressRouteSpec{
						VirtualHost: &projcontour.VirtualHost{
							Fqdn: "example.com",
							TLS: &projcontour.TLS{
								SecretName: sec1.Name,
							},
						},
						TCPProxy: &ingressroutev1.TCPProxy{
							Services: []ingressroutev1.Service{{
								Name: s9.Name,
								Port: 80,
							}},
						},
					},
				},
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", prefixroute("/", service(s9))),
					),
				},
				&Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "example.com",
							},
							MinProtoVersion: envoy_api_v2_auth.TlsParameters_TLSv1_1,
							Secret:          secret(sec1),
							TCPProxy: &TCPProxy{
								Clusters: clusters(service(s9)),
							},
						},
					),
				},
			),
		},
		"insert httproxy": {
			objs: []interface{}{
				proxy1, s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", prefixroute("/", service(s1))),
					),
				},
			),
		},
		"insert httproxy w/o condition": {
			objs: []interface{}{
				proxy1b, s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", prefixroute("/", service(s1))),
					),
				},
			),
		},
		"insert httproxy w/ conditions": {
			objs: []interface{}{
				proxy1c, s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", &Route{
							PathCondition: &PrefixCondition{Prefix: "/kuard"},
							HeaderConditions: []HeaderCondition{
								{Name: "x-request-id", MatchType: "present"},
								{Name: "e-tag", Value: "abcdef", MatchType: "contains"},
								{Name: "x-timeout", Value: "infinity", MatchType: "contains", Invert: true},
								{Name: "digest-auth", Value: "scott", MatchType: "exact"},
								{Name: "digest-password", Value: "tiger", MatchType: "exact", Invert: true},
							},
							Clusters: clusters(service(s1)),
						}),
					),
				},
			),
		},
		"insert httproxy w/ included conditions": {
			objs: []interface{}{
				proxy2a, proxy2b, s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", &Route{
							PathCondition: &PrefixCondition{Prefix: "/kuard"},
							HeaderConditions: []HeaderCondition{
								{Name: "x-request-id", MatchType: "present"},
								{Name: "x-timeout", Value: "infinity", MatchType: "contains", Invert: true},
								{Name: "digest-auth", Value: "scott", MatchType: "exact"},
								{Name: "e-tag", Value: "abcdef", MatchType: "contains"},
								{Name: "digest-password", Value: "tiger", MatchType: "exact", Invert: true},
							},
							Clusters: clusters(service(s1)),
						}),
					),
				},
			),
		},
		"insert httpproxy w/ healthcheck": {
			objs: []interface{}{
				proxy1e, s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							routeCluster("/", &Cluster{
								Upstream: service(s1),
								HealthCheckPolicy: &HealthCheckPolicy{
									Path: "/healthz",
								},
							}),
						),
					),
				},
			),
		},
		"insert httpproxy with mirroring route": {
			objs: []interface{}{
				proxy12, s1, s2,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							withMirror(prefixroute("/", service(s1)), service(s2)),
						),
					),
				},
			),
		},
		"insert httpproxy with two mirrors": {
			objs: []interface{}{
				proxy13, s1, s2,
			},
			want: listeners(),
		},
		"insert httpproxy with prefix rewrite route": {
			objs: []interface{}{
				proxy10, s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							prefixroute("/", service(s1)),
							routeWebsocket("/websocket", service(s1)),
						),
					),
				},
			),
		},
		"insert httpproxy with multiple upstreams prefix rewrite route, websocket routes are dropped": {
			objs: []interface{}{
				proxy10b, s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							prefixroute("/", service(s1)),
						),
					),
				},
			),
		},
		"insert httpproxy and service": {
			objs: []interface{}{
				proxy1, s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", prefixroute("/", service(s1))),
					),
				},
			),
		},
		"insert httpproxy without tls version": {
			objs: []interface{}{
				proxy6, s1, sec1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("foo.com", routeUpgrade("/", service(s1))),
					),
				}, &Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						securevirtualhost("foo.com", sec1, routeUpgrade("/", service(s1))),
					),
				},
			),
		},
		"insert httpproxy expecting verification": {
			objs: []interface{}{
				cert1, proxy17, s1a,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							routeCluster("/",
								&Cluster{
									Upstream: &Service{
										Name:        s1a.Name,
										Namespace:   s1a.Namespace,
										ServicePort: &s1a.Spec.Ports[0],
										Protocol:    "tls",
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
		"insert httpproxy with pathPrefix include": {
			objs: []interface{}{
				proxy100, proxy100a, s1, s4,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							routeCluster("/",
								&Cluster{
									Upstream: &Service{
										Name:        s1.Name,
										Namespace:   s1.Namespace,
										ServicePort: &s1.Spec.Ports[0],
									},
								},
							),
							routeCluster("/blog",
								&Cluster{
									Upstream: &Service{
										Name:        s4.Name,
										Namespace:   s4.Namespace,
										ServicePort: &s4.Spec.Ports[0],
									},
								},
							),
						),
					),
				},
			),
		},
		"insert httpproxy with pathPrefix include, child adds to pathPrefix": {
			objs: []interface{}{
				proxy100, proxy100b, s1, s4,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							routeCluster("/",
								&Cluster{
									Upstream: &Service{
										Name:        s1.Name,
										Namespace:   s1.Namespace,
										ServicePort: &s1.Spec.Ports[0],
									},
								},
							),
							&Route{
								PathCondition: prefix("/blog/infotech"),
								Clusters: []*Cluster{{
									Upstream: &Service{
										Name:        s4.Name,
										Namespace:   s4.Namespace,
										ServicePort: &s4.Spec.Ports[0],
									},
								}},
							},
						),
					),
				},
			),
		},
		"insert httpproxy with pathPrefix include, child adds to pathPrefix, delegates again": {
			objs: []interface{}{
				proxy100, proxy100c, proxy100d, s1, s4, s11,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							routeCluster("/",
								&Cluster{
									Upstream: &Service{
										Name:        s1.Name,
										Namespace:   s1.Namespace,
										ServicePort: &s1.Spec.Ports[0],
									},
								},
							),
							&Route{
								PathCondition: prefix("/blog/infotech"),
								Clusters: []*Cluster{{
									Upstream: &Service{
										Name:        s4.Name,
										Namespace:   s4.Namespace,
										ServicePort: &s4.Spec.Ports[0],
									},
								}},
							},
							routeCluster("/blog",
								&Cluster{
									Upstream: &Service{
										Name:        s4.Name,
										Namespace:   s4.Namespace,
										ServicePort: &s4.Spec.Ports[0],
									},
								},
							),
							&Route{
								PathCondition: prefix("/blog/it/foo"),
								Clusters: []*Cluster{{
									Upstream: &Service{
										Name:        s11.Name,
										Namespace:   s11.Namespace,
										ServicePort: &s11.Spec.Ports[0],
									},
								}},
							},
						),
					),
				},
			),
		},
		"insert httpproxy with no namespace for include": {
			objs: []interface{}{
				proxy101, proxy101a, s1, s2,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							routeCluster("/",
								&Cluster{
									Upstream: &Service{
										Name:        s1.Name,
										Namespace:   s1.Namespace,
										ServicePort: &s1.Spec.Ports[0],
									},
								},
							),
							routeCluster("/kuarder",
								&Cluster{
									Upstream: &Service{
										Name:        s2.Name,
										Namespace:   s2.Namespace,
										ServicePort: &s2.Spec.Ports[0],
									},
								},
							),
						),
					),
				},
			),
		},
		"insert httpproxy with include, no prefix condition on included proxy": {
			objs: []interface{}{
				proxy104, proxy104a, s1, s2,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							routeCluster("/",
								&Cluster{
									Upstream: &Service{
										Name:        s1.Name,
										Namespace:   s1.Namespace,
										ServicePort: &s1.Spec.Ports[0],
									},
								},
							),
							routeCluster("/kuarder",
								&Cluster{
									Upstream: &Service{
										Name:        s2.Name,
										Namespace:   s2.Namespace,
										ServicePort: &s2.Spec.Ports[0],
									},
								},
							),
						),
					),
				},
			),
		},
		"insert httpproxy with include, / on included proxy": {
			objs: []interface{}{
				proxy105, proxy105a, s1, s2,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							routeCluster("/",
								&Cluster{
									Upstream: &Service{
										Name:        s1.Name,
										Namespace:   s1.Namespace,
										ServicePort: &s1.Spec.Ports[0],
									},
								},
							),
							routeCluster("/kuarder/",
								&Cluster{
									Upstream: &Service{
										Name:        s2.Name,
										Namespace:   s2.Namespace,
										ServicePort: &s2.Spec.Ports[0],
									},
								},
							),
						),
					),
				},
			),
		},
		"insert httpproxy with include, full prefix on included proxy": {
			objs: []interface{}{
				proxy107, proxy107a, s1, s2,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							routeCluster("/",
								&Cluster{
									Upstream: &Service{
										Name:        s1.Name,
										Namespace:   s1.Namespace,
										ServicePort: &s1.Spec.Ports[0],
									},
								},
							),
							routeCluster("/kuarder/withavengeance",
								&Cluster{
									Upstream: &Service{
										Name:        s2.Name,
										Namespace:   s2.Namespace,
										ServicePort: &s2.Spec.Ports[0],
									},
								},
							),
						),
					),
				},
			),
		},
		"insert httpproxy with include ending with /, / on included proxy": {
			objs: []interface{}{
				proxy106, proxy106a, s1, s2,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							routeCluster("/",
								&Cluster{
									Upstream: &Service{
										Name:        s1.Name,
										Namespace:   s1.Namespace,
										ServicePort: &s1.Spec.Ports[0],
									},
								},
							),
							routeCluster("/kuarder/",
								&Cluster{
									Upstream: &Service{
										Name:        s2.Name,
										Namespace:   s2.Namespace,
										ServicePort: &s2.Spec.Ports[0],
									},
								},
							),
						),
					),
				},
			),
		},
		"insert httpproxy with multiple prefix conditions on route": {
			objs: []interface{}{
				proxy102, s1,
			},
			want: listeners(),
		},
		"insert httpproxy with multiple prefix conditions on include": {
			objs: []interface{}{
				proxy103, proxy103a, s1,
			},
			want: listeners(),
		},
		"insert proxy with tcp forward without TLS termination w/ passthrough": {
			objs: []interface{}{
				proxy1a, s1,
			},
			want: listeners(
				&Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "kuard.example.com",
							},
							TCPProxy: &TCPProxy{
								Clusters: clusters(
									service(s1),
								),
							},
						},
					),
				},
			),
		},
		// issue 1399
		"service shared across ingress and httpproxy tcpproxy": {
			objs: []interface{}{
				sec1,
				s9,
				&v1beta1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "nginx",
						Namespace: "default",
					},
					Spec: v1beta1.IngressSpec{
						TLS: []v1beta1.IngressTLS{{
							Hosts:      []string{"example.com"},
							SecretName: s1.Name,
						}},
						Rules: []v1beta1.IngressRule{{
							Host:             "example.com",
							IngressRuleValue: ingressrulevalue(backend(s9.Name, intstr.FromInt(80))),
						}},
					},
				},
				&projcontour.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "nginx",
						Namespace: "default",
					},
					Spec: projcontour.HTTPProxySpec{
						VirtualHost: &projcontour.VirtualHost{
							Fqdn: "example.com",
							TLS: &projcontour.TLS{
								SecretName: sec1.Name,
							},
						},
						TCPProxy: &projcontour.TCPProxy{
							Services: []projcontour.Service{{
								Name: s9.Name,
								Port: 80,
							}},
						},
					},
				},
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", prefixroute("/", service(s9))),
					),
				},
				&Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "example.com",
							},
							MinProtoVersion: envoy_api_v2_auth.TlsParameters_TLSv1_1,
							Secret:          secret(sec1),
							TCPProxy: &TCPProxy{
								Clusters: clusters(service(s9)),
							},
						},
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
				cmp.AllowUnexported(VirtualHost{}),
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

func TestBuilderLookupService(t *testing.T) {
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
		want *Service
	}{
		"lookup service by port number": {
			Meta: Meta{name: "service1", namespace: "default"},
			port: intstr.FromInt(8080),
			want: service(s1),
		},
		"lookup service by port name": {
			Meta: Meta{name: "service1", namespace: "default"},
			port: intstr.FromString("http"),
			want: service(s1),
		},
		"lookup service by port number (as string)": {
			Meta: Meta{name: "service1", namespace: "default"},
			port: intstr.Parse("8080"),
			want: service(s1),
		},
		"lookup service by port number (from string)": {
			Meta: Meta{name: "service1", namespace: "default"},
			port: intstr.FromString("8080"),
			want: service(s1),
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
			got := b.lookupService(tc.Meta, tc.port)
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
			VirtualHost: &projcontour.VirtualHost{
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
			VirtualHost: &projcontour.VirtualHost{
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

	proxy1 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "allowed1",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	// proxy2 is like proxy1, but in a different namespace
	proxy2 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "allowed2",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example2.com",
			},
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
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
		"nil root httpproxy namespaces": {
			objs: []interface{}{proxy1, s2},
			want: 1,
		},
		"empty root httpproxy namespaces": {
			objs: []interface{}{proxy1, s2},
			want: 1,
		},
		"single root namespace with root httpproxy": {
			rootNamespaces: []string{"allowed1"},
			objs:           []interface{}{proxy1, s2},
			want:           1,
		},
		"multiple root namespaces, one with a root httpproxy": {
			rootNamespaces: []string{"foo", "allowed1", "bar"},
			objs:           []interface{}{proxy1, s2},
			want:           1,
		},
		"multiple root namespaces, each with a root httpproxy": {
			rootNamespaces: []string{"foo", "allowed1", "allowed2"},
			objs:           []interface{}{proxy1, proxy2, s2, s3},
			want:           2,
		},
		"root httpproxy defined outside single root namespaces": {
			rootNamespaces: []string{"foo"},
			objs:           []interface{}{proxy1},
			want:           0,
		},
		"root httpproxy defined outside multiple root namespaces": {
			rootNamespaces: []string{"foo", "bar"},
			objs:           []interface{}{proxy1},
			want:           0,
		},
		"two root httpproxy, one inside root namespace, one outside": {
			rootNamespaces: []string{"foo", "allowed2"},
			objs:           []interface{}{proxy1, proxy2, s3},
			want:           1,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			builder := Builder{
				Source: KubernetesCache{
					RootNamespaces: tc.rootNamespaces,
					FieldLogger:    testLogger(t),
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

func routes(routes ...*Route) map[string]*Route {
	if len(routes) == 0 {
		return nil
	}
	m := make(map[string]*Route)
	for _, r := range routes {
		m[conditionsToString(r)] = r
	}
	return m
}

func prefixroute(prefix string, first *Service, rest ...*Service) *Route {
	services := append([]*Service{first}, rest...)
	return &Route{
		PathCondition: &PrefixCondition{Prefix: prefix},
		Clusters:      clusters(services...),
	}
}

func routeCluster(prefix string, first *Cluster, rest ...*Cluster) *Route {
	return &Route{
		PathCondition: &PrefixCondition{Prefix: prefix},
		Clusters:      append([]*Cluster{first}, rest...),
	}
}

func routeUpgrade(prefix string, first *Service, rest ...*Service) *Route {
	r := prefixroute(prefix, first, rest...)
	r.HTTPSUpgrade = true
	return r
}

func routeRewrite(prefix, rewrite string, first *Service, rest ...*Service) *Route {
	r := prefixroute(prefix, first, rest...)
	r.PrefixRewrite = rewrite
	return r
}

func routeWebsocket(prefix string, first *Service, rest ...*Service) *Route {
	r := prefixroute(prefix, first, rest...)
	r.Websocket = true
	return r
}

func clusters(services ...*Service) (c []*Cluster) {
	for _, s := range services {
		c = append(c, &Cluster{
			Upstream: s,
		})
	}
	return c
}

func service(s *v1.Service) *Service {
	return &Service{
		Name:        s.Name,
		Namespace:   s.Namespace,
		ServicePort: &s.Spec.Ports[0],
	}
}

func clustermap(services ...*v1.Service) []*Cluster {
	var c []*Cluster
	for _, s := range services {
		c = append(c, &Cluster{
			Upstream: service(s),
		})
	}
	return c
}

func secret(s *v1.Secret) *Secret {
	return &Secret{
		Object: s,
	}
}

func virtualhosts(vx ...Vertex) []Vertex {
	return vx
}

func virtualhost(name string, first *Route, rest ...*Route) *VirtualHost {
	return &VirtualHost{
		Name:   name,
		routes: routes(append([]*Route{first}, rest...)...),
	}
}

func securevirtualhost(name string, sec *v1.Secret, first *Route, rest ...*Route) *SecureVirtualHost {
	return &SecureVirtualHost{
		VirtualHost: VirtualHost{
			Name:   name,
			routes: routes(append([]*Route{first}, rest...)...),
		},
		MinProtoVersion: envoy_api_v2_auth.TlsParameters_TLSv1_1,
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

func prefix(prefix string) Condition { return &PrefixCondition{Prefix: prefix} }
func regex(regex string) Condition   { return &RegexCondition{Regex: regex} }

func withMirror(r *Route, mirror *Service) *Route {
	r.MirrorPolicy = &MirrorPolicy{
		Cluster: &Cluster{
			Upstream: mirror,
		},
	}
	return r

}
