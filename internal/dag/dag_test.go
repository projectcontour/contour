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

package dag

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	ingressroutev1 "github.com/heptio/contour/apis/contour/v1beta1"
	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestDAGInsert(t *testing.T) {
	// The DAG is senstive to ordering, adding an ingress, then a service,
	// should have the same result as adding a sevice, then an ingress.

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
	// i6 contains two named vhosts which point to the same sevice
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
				Delegate: ingressroutev1.Delegate{
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
				Delegate: ingressroutev1.Delegate{
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

	sec1 := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret",
			Namespace: "default",
		},
		Data: secretdata("certificate", "key"),
	}

	tests := map[string]struct {
		objs []interface{}
		want []Vertex
	}{
		"insert ingress w/ default backend": {
			objs: []interface{}{
				i1,
			},
			want: []Vertex{
				&VirtualHost{
					Port: 80,
					host: "*",
					routes: routemap(
						&Route{
							path:   "/",
							object: i1,
						},
					),
				},
			},
		},
		"insert ingress w/ single unnamed backend": {
			objs: []interface{}{
				i2,
			},
			want: []Vertex{
				&VirtualHost{
					Port: 80,
					host: "*",
					routes: routemap(
						&Route{
							path:   "/",
							object: i2,
						},
					),
				},
			},
		},
		"insert ingress w/ host name and single backend": {
			objs: []interface{}{
				i3,
			},
			want: []Vertex{
				&VirtualHost{
					Port: 80,
					host: "kuard.example.com",
					routes: routemap(
						&Route{
							path:   "/",
							object: i3,
						},
					),
				},
			},
		},
		"insert ingress w/ default backend then matching service": {
			objs: []interface{}{
				i1,
				s1,
			},
			want: []Vertex{
				&VirtualHost{
					Port: 80,
					host: "*",
					routes: routemap(
						&Route{
							path:   "/",
							object: i1,
							services: servicemap(
								&Service{
									object:      s1,
									ServicePort: &s1.Spec.Ports[0],
								},
							),
						},
					),
				},
			},
		},
		"insert service then ingress w/ default backend": {
			objs: []interface{}{
				s1,
				i1,
			},
			want: []Vertex{
				&VirtualHost{
					Port: 80,
					host: "*",
					routes: routemap(
						&Route{
							path:   "/",
							object: i1,
							services: servicemap(
								&Service{
									object:      s1,
									ServicePort: &s1.Spec.Ports[0],
								},
							),
						},
					),
				},
			},
		},
		"insert ingress w/ default backend then non-matching service": {
			objs: []interface{}{
				i1,
				s2,
			},
			want: []Vertex{
				&VirtualHost{
					Port: 80,
					host: "*",
					routes: routemap(
						&Route{
							path:   "/",
							object: i1,
						},
					),
				},
			},
		},
		"insert non matching service then ingress w/ default backend": {
			objs: []interface{}{
				s2,
				i1,
			},
			want: []Vertex{
				&VirtualHost{
					Port: 80,
					host: "*",
					routes: routemap(
						&Route{
							path:   "/",
							object: i1,
						},
					),
				},
			},
		},
		"insert ingress w/ default backend then matching service with wrong port": {
			objs: []interface{}{
				i1,
				s3,
			},
			want: []Vertex{
				&VirtualHost{
					Port: 80,
					host: "*",
					routes: routemap(
						&Route{
							path:   "/",
							object: i1,
						},
					),
				},
			},
		},
		"insert unnamed ingress w/ single backend then matching service with wrong port": {
			objs: []interface{}{
				i2,
				s3,
			},
			want: []Vertex{
				&VirtualHost{
					Port: 80,
					host: "*",
					routes: routemap(
						&Route{
							path:   "/",
							object: i2,
						},
					),
				},
			},
		},
		"insert service then matching unnamed ingress w/ single backend but wrong port": {
			objs: []interface{}{
				s3,
				i2,
			},
			want: []Vertex{
				&VirtualHost{
					Port: 80,
					host: "*",
					routes: routemap(
						&Route{
							path:   "/",
							object: i2,
						},
					),
				},
			},
		},
		"insert ingress w/ default backend then matching service w/ named port": {
			objs: []interface{}{
				i4,
				s1,
			},
			want: []Vertex{
				&VirtualHost{
					Port: 80,
					host: "*",
					routes: routemap(
						&Route{
							path:   "/",
							object: i4,
							services: servicemap(
								&Service{
									object:      s1,
									ServicePort: &s1.Spec.Ports[0],
								},
							),
						},
					),
				},
			},
		},
		"insert service w/ named port then ingress w/ default backend": {
			objs: []interface{}{
				s1,
				i4,
			},
			want: []Vertex{
				&VirtualHost{
					Port: 80,
					host: "*",
					routes: routemap(
						&Route{
							path:   "/",
							object: i4,
							services: servicemap(
								&Service{
									object:      s1,
									ServicePort: &s1.Spec.Ports[0],
								},
							),
						},
					),
				},
			},
		},
		"insert ingress w/ single unnamed backend w/ named service port then service": {
			objs: []interface{}{
				i5,
				s1,
			},
			want: []Vertex{
				&VirtualHost{
					Port: 80,
					host: "*",
					routes: routemap(
						&Route{
							path:   "/",
							object: i5,
							services: servicemap(
								&Service{
									object:      s1,
									ServicePort: &s1.Spec.Ports[0],
								},
							),
						},
					),
				},
			},
		},
		"insert service then ingress w/ single unnamed backend w/ named service port": {
			objs: []interface{}{
				s1,
				i5,
			},
			want: []Vertex{
				&VirtualHost{
					Port: 80,
					host: "*",
					routes: routemap(
						&Route{
							path:   "/",
							object: i5,
							services: servicemap(
								&Service{
									object:      s1,
									ServicePort: &s1.Spec.Ports[0],
								},
							),
						},
					),
				},
			},
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
			want: []Vertex{
				&VirtualHost{
					Port: 80,
					host: "*",
					routes: routemap(
						&Route{
							path:   "/",
							object: i1,
						},
					),
				}},
		},
		"insert secret then ingress w/ tls": {
			objs: []interface{}{
				sec1,
				i3,
			},
			want: []Vertex{
				&VirtualHost{
					Port: 80,
					host: "kuard.example.com",
					routes: routemap(
						&Route{
							path:   "/",
							object: i3,
						},
					),
				},
				&SecureVirtualHost{
					Port:            443,
					MinProtoVersion: auth.TlsParameters_TLSv1_1,
					host:            "kuard.example.com",
					routes: routemap(
						&Route{
							path:   "/",
							object: i3,
						},
					),
					secret: &Secret{
						object: sec1,
					},
				},
			},
		},
		"insert ingress w/ tls then secret": {
			objs: []interface{}{
				i3,
				sec1,
			},
			want: []Vertex{
				&VirtualHost{
					Port: 80,
					host: "kuard.example.com",
					routes: routemap(
						&Route{
							path:   "/",
							object: i3,
						},
					),
				},
				&SecureVirtualHost{
					Port:            443,
					MinProtoVersion: auth.TlsParameters_TLSv1_1,
					host:            "kuard.example.com",
					routes: routemap(
						&Route{
							path:   "/",
							object: i3,
						},
					),
					secret: &Secret{
						object: sec1,
					},
				},
			},
		},
		"insert ingress w/ two vhosts": {
			objs: []interface{}{
				i6,
			},
			want: []Vertex{
				&VirtualHost{
					Port: 80,
					host: "a.example.com",
					routes: routemap(
						&Route{
							path:   "/",
							object: i6,
						},
					),
				},
				&VirtualHost{
					Port: 80,
					host: "b.example.com",
					routes: routemap(
						&Route{
							path:   "/",
							object: i6,
						},
					),
				},
			},
		},
		"insert ingress w/ two vhosts then matching service": {
			objs: []interface{}{
				i6,
				s1,
			},
			want: []Vertex{
				&VirtualHost{
					Port: 80,
					host: "a.example.com",
					routes: routemap(
						&Route{
							path:   "/",
							object: i6,
							services: servicemap(
								&Service{
									object:      s1,
									ServicePort: &s1.Spec.Ports[0],
								},
							),
						},
					),
				},
				&VirtualHost{
					Port: 80,
					host: "b.example.com",
					routes: routemap(
						&Route{
							path:   "/",
							object: i6,
							services: servicemap(
								&Service{
									object:      s1,
									ServicePort: &s1.Spec.Ports[0],
								},
							),
						},
					),
				},
			},
		},
		"insert service then ingress w/ two vhosts": {
			objs: []interface{}{
				s1,
				i6,
			},
			want: []Vertex{
				&VirtualHost{
					Port: 80,
					host: "a.example.com",
					routes: routemap(
						&Route{
							path:   "/",
							object: i6,
							services: servicemap(
								&Service{
									object:      s1,
									ServicePort: &s1.Spec.Ports[0],
								},
							),
						},
					),
				},
				&VirtualHost{
					Port: 80,
					host: "b.example.com",
					routes: routemap(
						&Route{
							path:   "/",
							object: i6,
							services: servicemap(
								&Service{
									object:      s1,
									ServicePort: &s1.Spec.Ports[0],
								},
							),
						},
					),
				},
			},
		},
		"insert ingress w/ two vhosts then service then secret": {
			objs: []interface{}{
				i6,
				s1,
				sec1,
			},
			want: []Vertex{
				&VirtualHost{
					Port: 80,
					host: "a.example.com",
					routes: routemap(
						&Route{
							path:   "/",
							object: i6,
							services: servicemap(
								&Service{
									object:      s1,
									ServicePort: &s1.Spec.Ports[0],
								},
							),
						},
					),
				},
				&VirtualHost{
					Port: 80,
					host: "b.example.com",
					routes: routemap(
						&Route{
							path:   "/",
							object: i6,
							services: servicemap(
								&Service{
									object:      s1,
									ServicePort: &s1.Spec.Ports[0],
								},
							),
						},
					),
				}, &SecureVirtualHost{
					Port:            443,
					MinProtoVersion: auth.TlsParameters_TLSv1_1,
					host:            "b.example.com",
					routes: routemap(
						&Route{
							path:   "/",
							object: i6,
							services: servicemap(
								&Service{
									object:      s1,
									ServicePort: &s1.Spec.Ports[0],
								},
							),
						},
					),
					secret: &Secret{
						object: sec1,
					},
				},
			},
		},
		"insert service then secret then ingress w/ two vhosts": {
			objs: []interface{}{
				s1,
				sec1,
				i6,
			},
			want: []Vertex{
				&VirtualHost{
					Port: 80,
					host: "a.example.com",
					routes: routemap(
						&Route{
							path:   "/",
							object: i6,
							services: servicemap(
								&Service{
									object:      s1,
									ServicePort: &s1.Spec.Ports[0],
								},
							),
						},
					),
				}, &VirtualHost{
					Port: 80,
					host: "b.example.com",
					routes: routemap(
						&Route{
							path:   "/",
							object: i6,
							services: servicemap(
								&Service{
									object:      s1,
									ServicePort: &s1.Spec.Ports[0],
								},
							),
						},
					),
				}, &SecureVirtualHost{
					Port:            443,
					MinProtoVersion: auth.TlsParameters_TLSv1_1,
					host:            "b.example.com",
					routes: routemap(
						&Route{
							path:   "/",
							object: i6,
							services: servicemap(
								&Service{
									object:      s1,
									ServicePort: &s1.Spec.Ports[0],
								},
							),
						},
					),
					secret: &Secret{
						object: sec1,
					},
				},
			},
		},
		"insert ingress w/ two paths": {
			objs: []interface{}{
				i7,
			},
			want: []Vertex{
				&VirtualHost{
					Port: 80,
					host: "b.example.com",
					routes: routemap(
						&Route{
							path:   "/",
							object: i7,
						},
						&Route{
							path:   "/kuarder",
							object: i7,
						},
					),
				}},
		},
		"insert ingress w/ two paths then services": {
			objs: []interface{}{
				i7,
				s2,
				s1,
			},
			want: []Vertex{
				&VirtualHost{
					Port: 80,
					host: "b.example.com",
					routes: routemap(
						&Route{
							path:   "/",
							object: i7,
							services: servicemap(
								&Service{
									object:      s1,
									ServicePort: &s1.Spec.Ports[0],
								},
							),
						},
						&Route{
							path:   "/kuarder",
							object: i7,
							services: servicemap(
								&Service{
									object:      s2,
									ServicePort: &s1.Spec.Ports[0],
								},
							),
						},
					),
				},
			},
		},
		"insert two services then ingress w/ two ingress rules": {
			objs: []interface{}{
				s1, s2, i8,
			},
			want: []Vertex{
				&VirtualHost{
					Port: 80,
					host: "b.example.com",
					routes: routemap(
						&Route{
							path:   "/",
							object: i8,
							services: servicemap(
								&Service{
									object:      s1,
									ServicePort: &s1.Spec.Ports[0],
								},
							),
						},
						&Route{
							path:   "/kuarder",
							object: i8,
							services: servicemap(
								&Service{
									object:      s2,
									ServicePort: &s2.Spec.Ports[0],
								},
							),
						},
					),
				},
			},
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
			want: []Vertex{
				&SecureVirtualHost{
					Port:            443,
					MinProtoVersion: auth.TlsParameters_TLSv1_1,
					host:            "b.example.com",
					routes: routemap(
						&Route{
							path:   "/",
							object: i9,
							services: servicemap(
								&Service{
									object:      s1,
									ServicePort: &s1.Spec.Ports[0],
								},
							),
						},
						&Route{
							path:   "/kuarder",
							object: i9,
							services: servicemap(
								&Service{
									object:      s2,
									ServicePort: &s2.Spec.Ports[0],
								},
							),
						},
					),
					secret: &Secret{
						object: sec1,
					},
				},
			},
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
			want: []Vertex{
				&SecureVirtualHost{
					Port:            443,
					MinProtoVersion: auth.TlsParameters_TLSv1_1,
					host:            "b.example.com",
					routes: routemap(
						&Route{
							path:   "/",
							object: i6a,
							services: servicemap(
								&Service{
									object:      s1,
									ServicePort: &s1.Spec.Ports[0],
								},
							),
						},
					),
					secret: &Secret{
						object: sec1,
					},
				},
			},
		},
		"insert ingress w/ force-ssl-redirect: true": {
			objs: []interface{}{
				i6b, sec1, s1,
			},
			want: []Vertex{
				&VirtualHost{
					Port: 80,
					host: "b.example.com",
					routes: routemap(
						&Route{
							path:   "/",
							object: i6b,
							services: servicemap(
								&Service{
									object:      s1,
									ServicePort: &s1.Spec.Ports[0],
								},
							),
							HTTPSUpgrade: true,
						},
					),
				},
				&SecureVirtualHost{
					Port:            443,
					MinProtoVersion: auth.TlsParameters_TLSv1_1,
					host:            "b.example.com",
					routes: routemap(
						&Route{
							path:   "/",
							object: i6b,
							services: servicemap(
								&Service{
									object:      s1,
									ServicePort: &s1.Spec.Ports[0],
								},
							),
							HTTPSUpgrade: true,
						},
					),
					secret: &Secret{
						object: sec1,
					},
				}},
		},

		"insert ingressroute": {
			objs: []interface{}{
				ir1,
			},
			want: []Vertex{
				&VirtualHost{
					Port: 80,
					host: "example.com",
					routes: routemap(
						&Route{
							path:   "/",
							object: ir1,
						},
					),
				}},
		},
		"insert ingressroute and service": {
			objs: []interface{}{
				ir1, s1,
			},
			want: []Vertex{
				&VirtualHost{
					Port: 80,
					host: "example.com",
					routes: routemap(
						&Route{
							path:   "/",
							object: ir1,
							services: servicemap(
								&Service{
									object:      s1,
									ServicePort: &s1.Spec.Ports[0],
								},
							),
						},
					),
				}},
		},
		"insert ingressroute referencing two backends, one missing": {
			objs: []interface{}{
				ir2, s2,
			},
			want: []Vertex{
				&VirtualHost{
					Port: 80,
					host: "example.com",
					routes: routemap(
						&Route{
							path:   "/",
							object: ir2,
							services: servicemap(
								&Service{
									object:      s2,
									ServicePort: &s2.Spec.Ports[0],
								},
							),
						},
					),
				}},
		},
		"insert ingressroute referencing two backends": {
			objs: []interface{}{
				ir2, s1, s2,
			},
			want: []Vertex{
				&VirtualHost{
					Port: 80,
					host: "example.com",
					routes: routemap(
						&Route{
							path:   "/",
							object: ir2,
							services: servicemap(
								&Service{
									object:      s1,
									ServicePort: &s1.Spec.Ports[0],
								},
								&Service{
									object:      s2,
									ServicePort: &s2.Spec.Ports[0],
								},
							),
						},
					),
				}},
		},
		"insert ingress w/ tls min proto annotation": {
			objs: []interface{}{
				i10,
				sec1,
				s1,
			},
			want: []Vertex{
				&VirtualHost{
					Port: 80,
					host: "b.example.com",
					routes: routemap(
						&Route{
							path:   "/",
							object: i10,
							services: servicemap(
								&Service{
									object:      s1,
									ServicePort: &s1.Spec.Ports[0],
								},
							),
						},
					),
				},
				&SecureVirtualHost{
					Port:            443,
					MinProtoVersion: auth.TlsParameters_TLSv1_3,
					host:            "b.example.com",
					routes: routemap(
						&Route{
							path:   "/",
							object: i10,
							services: servicemap(
								&Service{
									object:      s1,
									ServicePort: &s1.Spec.Ports[0],
								},
							),
						},
					),
					secret: &Secret{
						object: sec1,
					},
				},
			},
		},
		"insert ingress w/ websocket route annotation": {
			objs: []interface{}{
				i11,
				s1,
			},
			want: []Vertex{
				&VirtualHost{
					Port: 80,
					host: "*",
					routes: routemap(
						&Route{
							path:   "/",
							object: i11,
							services: servicemap(
								&Service{
									object:      s1,
									ServicePort: &s1.Spec.Ports[0],
								},
							),
						},
						&Route{
							path:   "/ws1",
							object: i11,
							services: servicemap(
								&Service{
									object:      s1,
									ServicePort: &s1.Spec.Ports[0],
								},
							),
							Websocket: true,
						},
					),
				},
			},
		},
		"insert ingress w/ invalid timeout annotation": {
			objs: []interface{}{
				i12a,
				s1,
			},
			want: []Vertex{
				&VirtualHost{
					Port: 80,
					host: "*",
					routes: routemap(
						&Route{
							path:   "/",
							object: i12a,
							services: servicemap(
								&Service{
									object:      s1,
									ServicePort: &s1.Spec.Ports[0],
								},
							),
							Timeout: -1, // invalid timeout equals infinity ¯\_(ツ)_/¯.
						},
					),
				},
			},
		},
		"insert ingress w/ valid timeout annotation": {
			objs: []interface{}{
				i12b,
				s1,
			},
			want: []Vertex{
				&VirtualHost{
					Port: 80,
					host: "*",
					routes: routemap(
						&Route{
							path:   "/",
							object: i12b,
							services: servicemap(
								&Service{
									object:      s1,
									ServicePort: &s1.Spec.Ports[0],
								},
							),
							Timeout: 90 * time.Second,
						},
					),
				},
			},
		},
		"insert ingress w/ infinite timeout annotation": {
			objs: []interface{}{
				i12c,
				s1,
			},
			want: []Vertex{
				&VirtualHost{
					Port: 80,
					host: "*",
					routes: routemap(
						&Route{
							path:   "/",
							object: i12c,
							services: servicemap(
								&Service{
									object:      s1,
									ServicePort: &s1.Spec.Ports[0],
								},
							),
							Timeout: -1,
						},
					),
				},
			},
		},
		"insert root ingress route and delegate ingress route": {
			objs: []interface{}{
				ir5, s4, ir4, s5, ir3,
			},
			want: []Vertex{
				&VirtualHost{
					Port: 80,
					host: "example.com",
					routes: routemap(
						&Route{
							path:   "/blog",
							object: ir4,
							services: servicemap(
								&Service{
									object:      s4,
									ServicePort: &s4.Spec.Ports[0],
								},
							),
						},
						&Route{
							path:   "/blog/admin",
							object: ir5,
							services: servicemap(
								&Service{
									object:      s5,
									ServicePort: &s5.Spec.Ports[0],
								},
							),
						},
					),
				},
			},
		},
		"insert ingress overlay": {
			objs: []interface{}{
				i13a, i13b, sec13, s13a, s13b,
			},
			want: []Vertex{
				&VirtualHost{
					Port: 80,
					host: "example.com",
					routes: routemap(
						&Route{
							path:   "/",
							object: i13a,
							services: servicemap(
								&Service{
									object:      s13a,
									ServicePort: &s13a.Spec.Ports[0],
								},
							),
							HTTPSUpgrade: true,
						},
						&Route{
							path:   "/.well-known/acme-challenge/gVJl5NWL2owUqZekjHkt_bo3OHYC2XNDURRRgLI5JTk",
							object: i13b,
							services: servicemap(
								&Service{
									object:      s13b,
									ServicePort: &s13b.Spec.Ports[0],
								},
							),
						},
					),
				},
				&SecureVirtualHost{
					Port:            443,
					MinProtoVersion: auth.TlsParameters_TLSv1_1,
					host:            "example.com",
					routes: routemap(
						&Route{
							path:   "/",
							object: i13a,
							services: servicemap(
								&Service{
									object:      s13a,
									ServicePort: &s13a.Spec.Ports[0],
								},
							),
							HTTPSUpgrade: true,
						},
						&Route{
							path:   "/.well-known/acme-challenge/gVJl5NWL2owUqZekjHkt_bo3OHYC2XNDURRRgLI5JTk",
							object: i13b,
							services: servicemap(
								&Service{
									object:      s13b,
									ServicePort: &s13b.Spec.Ports[0],
								},
							),
						},
					),
					secret: &Secret{
						object: sec13,
					},
				},
			},
		},
		"h2c service annotation": {
			objs: []interface{}{
				i3a, s3a,
			},
			want: []Vertex{
				&VirtualHost{
					Port: 80,
					host: "*",
					routes: routemap(
						&Route{
							path:   "/",
							object: i3a,
							services: servicemap(
								&Service{
									object:      s3a,
									Protocol:    "h2c",
									ServicePort: &s3a.Spec.Ports[0],
								},
							),
						},
					),
				},
			},
		},
		"h2 service annotation": {
			objs: []interface{}{
				i3a, s3b,
			},
			want: []Vertex{
				&VirtualHost{
					Port: 80,
					host: "*",
					routes: routemap(
						&Route{
							path:   "/",
							object: i3a,
							services: servicemap(
								&Service{
									object:      s3b,
									Protocol:    "h2",
									ServicePort: &s3b.Spec.Ports[0],
								},
							),
						},
					),
				},
			},
		},
		"insert ingress then service w/ upstream annotations": {
			objs: []interface{}{
				i1,
				s1b,
			},
			want: []Vertex{
				&VirtualHost{
					Port: 80,
					host: "*",
					routes: routemap(
						&Route{
							path:   "/",
							object: i1,
							services: servicemap(
								&Service{
									object:             s1b,
									ServicePort:        &s1b.Spec.Ports[0],
									MaxConnections:     9000,
									MaxPendingRequests: 4096,
									MaxRequests:        404,
									MaxRetries:         7,
								},
							),
						},
					),
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var d DAG
			for _, o := range tc.objs {
				d.Insert(o)
			}
			d.Recompute()

			got := make(map[hostport]Vertex)
			d.Visit(func(v Vertex) {
				switch v := v.(type) {
				case *VirtualHost:
					got[hostport{host: v.FQDN(), port: v.Port}] = v
				case *SecureVirtualHost:
					got[hostport{host: v.FQDN(), port: v.Port}] = v
				}
			})

			want := make(map[hostport]Vertex)
			for _, v := range tc.want {
				switch v := v.(type) {
				case *VirtualHost:
					want[hostport{host: v.FQDN(), port: v.Port}] = v
				case *SecureVirtualHost:
					want[hostport{host: v.FQDN(), port: v.Port}] = v
				}
			}

			if !reflect.DeepEqual(want, got) {
				t.Fatal("expected:\n", want, "\ngot:\n", got)
			}
		})
	}
}

type hostport struct {
	host string
	port int
}

func TestDAGRemove(t *testing.T) {
	// The DAG is senstive to ordering, removing an ingress, then a service,
	// has a different effect than removing a sevice, then an ingress.

	i1 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
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
	// i6 contains two named vhosts which point to the same sevice
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

	ir1 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "kuard.example.com",
			},
			Routes: []ingressroutev1.Route{{
				Match:    "/",
				Services: []ingressroutev1.Service{{Name: "kuard", Port: 8080}},
			}},
		},
	}

	// ir2 is similar to ir1, but it contains two backend services
	ir2 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "kuard.example.com",
			},
			Routes: []ingressroutev1.Route{{
				Match:    "/",
				Services: []ingressroutev1.Service{{Name: "kuard", Port: 8080}, {Name: "nginx", Port: 80}},
			}},
		},
	}

	// ir3 is similar to ir1, but it contains TLS configuration
	ir3 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
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
				Match:    "/",
				Services: []ingressroutev1.Service{{Name: "kuard", Port: 8080}},
			}},
		},
	}

	// ir4 contains two vhosts which point to the same service
	ir4 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "two-vhosts",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn:    "b.example.com",
				Aliases: []string{"a.example.com"},
			},
			Routes: []ingressroutev1.Route{{
				Match:    "/",
				Services: []ingressroutev1.Service{{Name: "kuard", Port: 8080}},
			}},
		},
	}

	// ir5 contains a single vhost with two paths
	ir5 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "two-paths",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "b.example.com",
			},
			Routes: []ingressroutev1.Route{{
				Match:    "/",
				Services: []ingressroutev1.Service{{Name: "kuard", Port: 8080}},
			}, {
				Match:    "/kuarder",
				Services: []ingressroutev1.Service{{Name: "kuarder", Port: 8080}},
			}},
		},
	}

	// ir6 contains a single vhost that delegates to ir7
	ir6 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "delegate",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "b.example.com",
			},
			Routes: []ingressroutev1.Route{{
				Match:    "/",
				Delegate: ingressroutev1.Delegate{Name: "delegated"},
			}},
		},
	}

	// ir7 is a delegated ingressroute with a single route pointing to s1
	ir7 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "delegated",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			Routes: []ingressroutev1.Route{{
				Match:    "/",
				Services: []ingressroutev1.Service{{Name: "kuard", Port: 8080}},
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

	sec1 := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret",
			Namespace: "default",
		},
		Data: secretdata("certificate", "key"),
	}

	tests := map[string]struct {
		insert []interface{}
		remove []interface{}
		want   []Vertex
	}{
		"remove ingress w/ default backend": {
			insert: []interface{}{
				i1,
			},
			remove: []interface{}{
				i1,
			},
			want: []Vertex{},
		},
		"remove ingress w/ single unnamed backend": {
			insert: []interface{}{
				i2,
			},
			remove: []interface{}{
				i2,
			},
			want: []Vertex{},
		},
		"insert ingress w/ host name and single backend": {
			insert: []interface{}{
				i3,
			},
			want: []Vertex{
				&VirtualHost{
					Port: 80,
					host: "kuard.example.com",
					routes: routemap(
						&Route{
							path:   "/",
							object: i3,
						},
					),
				}},
		},
		"remove ingress w/ default backend leaving matching service": {
			insert: []interface{}{
				i1,
				s1,
			},
			remove: []interface{}{
				i1,
			},
			want: []Vertex{},
		},
		"remove service leaving ingress w/ default backend": {
			insert: []interface{}{
				s1,
				i1,
			},
			remove: []interface{}{
				s1,
			},
			want: []Vertex{
				&VirtualHost{
					Port: 80,
					host: "*",
					routes: routemap(
						&Route{
							path:   "/",
							object: i1,
						},
					),
				}},
		},
		"remove non matching service leaving ingress w/ default backend": {
			insert: []interface{}{
				i1,
				s2,
			},
			remove: []interface{}{
				s2,
			},
			want: []Vertex{
				&VirtualHost{
					Port: 80,
					host: "*",
					routes: routemap(
						&Route{
							path:   "/",
							object: i1,
						},
					),
				}},
		},
		"remove ingress w/ default backend leaving non matching service": {
			insert: []interface{}{
				s2,
				i1,
			},
			remove: []interface{}{
				i1,
			},
			want: []Vertex{},
		},
		"remove service w/ named service port leaving ingress w/ single unnamed backend": {
			insert: []interface{}{
				i5,
				s1,
			},
			remove: []interface{}{
				s1,
			},
			want: []Vertex{
				&VirtualHost{
					Port: 80,
					host: "*",
					routes: routemap(
						&Route{
							path:   "/",
							object: i5,
						},
					),
				}},
		},
		"remove secret leaving ingress w/ tls": {
			insert: []interface{}{
				sec1,
				i3,
			},
			remove: []interface{}{
				sec1,
			},
			want: []Vertex{
				&VirtualHost{
					Port: 80,
					host: "kuard.example.com",
					routes: routemap(
						&Route{
							path:   "/",
							object: i3,
						},
					),
				}},
		},
		"remove ingress w/ two vhosts": {
			insert: []interface{}{
				i6,
			},
			remove: []interface{}{
				i6,
			},
			want: []Vertex{},
		},
		"remove service leaving ingress w/ two vhosts": {
			insert: []interface{}{
				i6,
				s1,
			},
			remove: []interface{}{
				s1,
			},
			want: []Vertex{
				&VirtualHost{
					Port: 80,
					host: "a.example.com",
					routes: routemap(
						&Route{
							path:   "/",
							object: i6,
						},
					),
				},
				&VirtualHost{
					Port: 80,
					host: "b.example.com",
					routes: routemap(
						&Route{
							path:   "/",
							object: i6,
						},
					),
				}},
		},
		"remove secret from ingress w/ two vhosts and service": {
			insert: []interface{}{
				i6,
				s1,
				sec1,
			},
			remove: []interface{}{
				sec1,
			},
			want: []Vertex{
				&VirtualHost{
					Port: 80,
					host: "a.example.com",
					routes: routemap(
						&Route{
							path:   "/",
							object: i6,
							services: servicemap(
								&Service{
									object:      s1,
									ServicePort: &s1.Spec.Ports[0],
								},
							),
						},
					),
				}, &VirtualHost{
					Port: 80,
					host: "b.example.com",
					routes: routemap(
						&Route{
							path:   "/",
							object: i6,
							services: servicemap(
								&Service{
									object:      s1,
									ServicePort: &s1.Spec.Ports[0],
								},
							),
						},
					),
				}},
		},
		"remove service from ingress w/ two vhosts and secret": {
			insert: []interface{}{
				s1,
				sec1,
				i6,
			},
			remove: []interface{}{
				s1,
			},
			want: []Vertex{
				&VirtualHost{
					Port: 80,
					host: "a.example.com",
					routes: routemap(
						&Route{
							path:   "/",
							object: i6,
						},
					),
				},
				&VirtualHost{
					Port: 80,
					host: "b.example.com",
					routes: routemap(
						&Route{
							path:   "/",
							object: i6,
						},
					),
				},
				&SecureVirtualHost{
					Port:            443,
					MinProtoVersion: auth.TlsParameters_TLSv1_1,
					host:            "b.example.com",
					routes: routemap(
						&Route{
							path:   "/",
							object: i6,
						},
					),
					secret: &Secret{
						object: sec1,
					},
				}},
		},
		"remove service from ingress w/ two paths": {
			insert: []interface{}{
				i7,
				s2,
				s1,
			},
			remove: []interface{}{
				s2,
			},
			want: []Vertex{
				&VirtualHost{
					Port: 80,
					host: "b.example.com",
					routes: routemap(
						&Route{
							path:   "/",
							object: i7,
							services: servicemap(
								&Service{
									object:      s1,
									ServicePort: &s1.Spec.Ports[0],
								},
							),
						},
						&Route{
							path:   "/kuarder",
							object: i7,
						},
					),
				}},
		},
		"remove ingressroute w/ default backend": {
			insert: []interface{}{
				ir1,
			},
			remove: []interface{}{
				ir1,
			},
			want: []Vertex{},
		},
		"remove ingressroute w/ two backends": {
			insert: []interface{}{
				ir2,
			},
			remove: []interface{}{
				ir2,
			},
			want: []Vertex{},
		},
		"remove ingressroute w/ default backend leaving matching service": {
			insert: []interface{}{
				ir1,
				s1,
			},
			remove: []interface{}{
				ir1,
			},
			want: []Vertex{},
		},
		"remove service leaving ingressroute w/ default backend": {
			insert: []interface{}{
				s1,
				ir1,
			},
			remove: []interface{}{
				s1,
			},
			want: []Vertex{
				&VirtualHost{
					Port: 80,
					host: "kuard.example.com",
					routes: routemap(
						&Route{
							path:   "/",
							object: ir1,
						},
					),
				}},
		},
		"remove non matching service leaving ingressroute w/ default backend": {
			insert: []interface{}{
				ir1,
				s2,
			},
			remove: []interface{}{
				s2,
			},
			want: []Vertex{
				&VirtualHost{
					Port: 80,
					host: "kuard.example.com",
					routes: routemap(
						&Route{
							path:   "/",
							object: ir1,
						},
					),
				}},
		},
		"remove ingressroute w/ default backend leaving non matching service": {
			insert: []interface{}{
				s2,
				ir1,
			},
			remove: []interface{}{
				ir1,
			},
			want: []Vertex{},
		},
		"remove secret leaving ingressroute w/ tls": {
			insert: []interface{}{
				sec1,
				ir3,
			},
			remove: []interface{}{
				sec1,
			},
			want: []Vertex{
				&VirtualHost{
					Port: 80,
					host: "kuard.example.com",
					routes: routemap(
						&Route{
							path:   "/",
							object: ir3,
						},
					),
				}},
		},
		"remove ingressroute w/ two vhosts": {
			insert: []interface{}{
				ir4,
			},
			remove: []interface{}{
				ir4,
			},
			want: []Vertex{},
		},
		"remove service from ingressroute w/ two paths": {
			insert: []interface{}{
				ir5,
				s2,
				s1,
			},
			remove: []interface{}{
				s2,
			},
			want: []Vertex{
				&VirtualHost{
					Port: 80,
					host: "b.example.com",
					routes: routemap(
						&Route{
							path:   "/",
							object: ir5,
							services: servicemap(
								&Service{
									object:      s1,
									ServicePort: &s1.Spec.Ports[0],
								},
							),
						},
						&Route{
							path:   "/kuarder",
							object: ir5,
						},
					),
				}},
		},
		"delegated ingressroute: remove parent ingressroute": {
			insert: []interface{}{
				ir6, ir7, s1,
			},
			remove: []interface{}{
				ir6,
			},
			want: []Vertex{},
		},
		"delegated ingressroute: remove child ingressroute": {
			insert: []interface{}{
				ir6, ir7, s1,
			},
			remove: []interface{}{
				ir7,
			},
			want: []Vertex{},
		},
		"delegated ingressroute: remove service that matches child ingressroute": {
			insert: []interface{}{
				ir6, ir7, s1,
			},
			remove: []interface{}{
				s1,
			},
			want: []Vertex{
				&VirtualHost{
					Port: 80,
					host: "b.example.com",
					routes: routemap(
						&Route{
							path:   "/",
							object: ir7,
						},
					),
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var d DAG
			for _, o := range tc.insert {
				d.Insert(o)
			}
			d.Recompute()
			for _, o := range tc.remove {
				d.Remove(o)
			}
			d.Recompute()

			got := make(map[hostport]Vertex)
			d.Visit(func(v Vertex) {
				switch v := v.(type) {
				case *VirtualHost:
					got[hostport{host: v.FQDN(), port: v.Port}] = v
				case *SecureVirtualHost:
					got[hostport{host: v.FQDN(), port: v.Port}] = v
				}
			})

			want := make(map[hostport]Vertex)
			for _, v := range tc.want {
				switch v := v.(type) {
				case *VirtualHost:
					want[hostport{host: v.FQDN(), port: v.Port}] = v
				case *SecureVirtualHost:
					want[hostport{host: v.FQDN(), port: v.Port}] = v
				}
			}

			if !reflect.DeepEqual(want, got) {
				t.Fatal("\nexpected:\n", want, "\ngot:\n", got)
			}

		})
	}
}

func (v *VirtualHost) String() string {
	return fmt.Sprintf("host: %v:%d {routes: %v}", v.FQDN(), v.Port, v.routes)
}

func (s *SecureVirtualHost) String() string {
	return fmt.Sprintf("secure host: %v:%d {routes: %v, secret: %v}", s.FQDN(), s.Port, s.routes, s.secret)
}

func (r *Route) String() string {
	return fmt.Sprintf("route: %q {services: %v, object: %p, upgrade: %v}", r.Prefix(), r.services, r.object, r.HTTPSUpgrade)
}

func (s *Service) String() string {
	return fmt.Sprintf("service: %s/%s {serviceport: %v}", s.object.Namespace, s.object.Name, s.ServicePort)
}

func (s *Secret) String() string {
	return fmt.Sprintf("secret: %s/%s {object: %p}", s.object.Namespace, s.object.Name, s.object)
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

func TestServiceMapLookup(t *testing.T) {
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
		meta{name: "service1", namespace: "default"}: s1,
	}

	tests := map[string]struct {
		meta
		port intstr.IntOrString
		want *Service
	}{
		"lookup sevice by port number": {
			meta: meta{name: "service1", namespace: "default"},
			port: intstr.FromInt(8080),
			want: &Service{
				object:      s1,
				ServicePort: &s1.Spec.Ports[0],
			},
		},
		"lookup service by port name": {
			meta: meta{name: "service1", namespace: "default"},
			port: intstr.FromString("http"),
			want: &Service{
				object:      s1,
				ServicePort: &s1.Spec.Ports[0],
			},
		},
		"lookup service by port number (as string)": {
			meta: meta{name: "service1", namespace: "default"},
			port: intstr.Parse("8080"),
			want: &Service{
				object:      s1,
				ServicePort: &s1.Spec.Ports[0],
			},
		},
		"lookup service by port number (from string)": {
			meta: meta{name: "service1", namespace: "default"},
			port: intstr.FromString("8080"),
			want: &Service{
				object:      s1,
				ServicePort: &s1.Spec.Ports[0],
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			sm := serviceMap{services: services}
			got := sm.lookup(tc.meta, tc.port)
			if !reflect.DeepEqual(tc.want, got) {
				t.Fatalf("expected:\n%+v\ngot:\n%+v", tc.want, got)
			}
		})
	}
}

func TestDAGIngressRouteCycle(t *testing.T) {
	ir1 := &ingressroutev1.IngressRoute{
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
				Delegate: ingressroutev1.Delegate{
					Name:      "finance-root",
					Namespace: "finance",
				},
			}},
		},
	}
	ir2 := &ingressroutev1.IngressRoute{
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
				Delegate: ingressroutev1.Delegate{
					Name:      "example-com",
					Namespace: "default",
				},
			}},
		},
	}

	var d DAG
	d.Insert(ir2)
	d.Insert(ir1)
	d.Recompute()

	got := make(map[hostport]*VirtualHost)
	d.Visit(func(v Vertex) {
		if v, ok := v.(*VirtualHost); ok {
			got[hostport{host: v.FQDN(), port: v.Port}] = v
		}
	})

	want := make(map[hostport]*VirtualHost)
	want[hostport{host: "example.com", port: 80}] = &VirtualHost{
		Port:   80,
		host:   "example.com",
		routes: routemap(&Route{path: "/finance", object: ir2}),
	}

	if !reflect.DeepEqual(want, got) {
		t.Fatal("expected:\n", want, "\ngot:\n", got)
	}
}

func TestDAGIngressRouteCycleSelfEdge(t *testing.T) {
	ir1 := &ingressroutev1.IngressRoute{
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
				Delegate: ingressroutev1.Delegate{
					Name:      "example-com",
					Namespace: "default",
				},
			}},
		},
	}

	var d DAG
	d.Insert(ir1)
	d.Recompute()

	got := make(map[hostport]*VirtualHost)
	d.Visit(func(v Vertex) {
		if v, ok := v.(*VirtualHost); ok {
			got[hostport{host: v.FQDN(), port: v.Port}] = v
		}
	})

	want := make(map[hostport]*VirtualHost)

	if !reflect.DeepEqual(want, got) {
		t.Fatal("expected:\n", want, "\ngot:\n", got)
	}
}

func TestDAGIngressRouteDelegatesToNonExistent(t *testing.T) {
	ir1 := &ingressroutev1.IngressRoute{
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
				Delegate: ingressroutev1.Delegate{
					Name:      "non-existent",
					Namespace: "non-existent",
				},
			}},
		},
	}

	var d DAG
	d.Insert(ir1)
	d.Recompute()

	got := make(map[hostport]*VirtualHost)
	d.Visit(func(v Vertex) {
		if v, ok := v.(*VirtualHost); ok {
			got[hostport{host: v.FQDN(), port: v.Port}] = v
		}
	})

	want := make(map[hostport]*VirtualHost)

	if !reflect.DeepEqual(want, got) {
		t.Fatal("expected:\n", want, "\ngot:\n", got)
	}
}

func TestDAGIngressRouteDelegatePrefixDoesntMatch(t *testing.T) {
	ir1 := &ingressroutev1.IngressRoute{
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
				Delegate: ingressroutev1.Delegate{
					Name:      "finance-root",
					Namespace: "finance",
				},
			}},
		},
	}
	ir2 := &ingressroutev1.IngressRoute{
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
	}

	var d DAG
	d.Insert(ir2)
	d.Insert(ir1)
	d.Recompute()

	got := make(map[hostport]*VirtualHost)
	d.Visit(func(v Vertex) {
		if v, ok := v.(*VirtualHost); ok {
			got[hostport{host: v.FQDN(), port: v.Port}] = v
		}
	})

	want := make(map[hostport]*VirtualHost)

	if !reflect.DeepEqual(want, got) {
		t.Fatal("expected:\n", want, "\ngot:\n", got)
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
			var d DAG
			d.IngressRouteRootNamespaces = tc.rootNamespaces
			for _, o := range tc.objs {
				d.Insert(o)
			}
			d.Recompute()

			var count int
			d.Visit(func(v Vertex) {
				if _, ok := v.(*VirtualHost); ok {
					count++
				}
			})

			if tc.want != count {
				t.Errorf("wanted %d vertices, but got %d", tc.want, count)
			}
		})
	}
}

func TestDAGIngressRouteDelegatePrefixMatchesStringPrefixButNotPathPrefix(t *testing.T) {
	ir1 := &ingressroutev1.IngressRoute{
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
				Delegate: ingressroutev1.Delegate{
					Name:      "finance-root",
					Namespace: "finance",
				},
			}},
		},
	}
	ir2 := &ingressroutev1.IngressRoute{
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
	}

	var d DAG
	d.Insert(ir2)
	d.Insert(ir1)
	d.Recompute()

	got := make(map[hostport]*VirtualHost)
	d.Visit(func(v Vertex) {
		if v, ok := v.(*VirtualHost); ok {
			got[hostport{host: v.FQDN(), port: v.Port}] = v
		}
	})

	want := make(map[hostport]*VirtualHost)

	if !reflect.DeepEqual(want, got) {
		t.Fatal("expected:\n", want, "\ngot:\n", got)
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
				Delegate: ingressroutev1.Delegate{
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
				Delegate: ingressroutev1.Delegate{
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
				Delegate: ingressroutev1.Delegate{
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
				Delegate: ingressroutev1.Delegate{
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
				Delegate: ingressroutev1.Delegate{
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
				Delegate: ingressroutev1.Delegate{
					Name: "validChild",
				},
			}, {
				Match: "/bar",
				Delegate: ingressroutev1.Delegate{
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
				Delegate: ingressroutev1.Delegate{
					Name: "validChild",
				},
			}},
		},
	}

	tests := map[string]struct {
		objs  []*ingressroutev1.IngressRoute
		objs2 []*ingressroutev1.IngressRoute
		want  IngressrouteStatus
	}{
		"valid ingressroute": {
			objs: []*ingressroutev1.IngressRoute{ir1},
			want: IngressrouteStatus{statuses: []Status{{object: ir1, status: "valid", msg: "valid IngressRoute"}}},
		},
		"invalid port in service": {
			objs: []*ingressroutev1.IngressRoute{ir2},
			want: IngressrouteStatus{statuses: []Status{{object: ir2, status: "invalid", msg: `route "/foo": service "home": port must be in the range 1-65535`}}},
		},
		"root ingressroute outside of roots namespace": {
			objs: []*ingressroutev1.IngressRoute{ir3},
			want: IngressrouteStatus{statuses: []Status{{object: ir3, status: "invalid", msg: "root IngressRoute cannot be defined in this namespace"}}},
		},
		"delegated route's match prefix does not match parent's prefix": {
			objs: []*ingressroutev1.IngressRoute{ir1, ir4},
			want: IngressrouteStatus{
				statuses: []Status{{object: ir4, status: "invalid", msg: `the path prefix "/doesnotmatch" does not match the parent's path prefix "/prefix"`},
					{object: ir1, status: "valid", msg: "valid IngressRoute"}},
			},
		},
		"invalid weight in service": {
			objs: []*ingressroutev1.IngressRoute{ir5},
			want: IngressrouteStatus{statuses: []Status{{object: ir5, status: "invalid", msg: `route "/foo": service "home": weight must be greater than or equal to zero`}}},
		},
		"root ingressroute does not specify FQDN": {
			objs: []*ingressroutev1.IngressRoute{ir13},
			want: IngressrouteStatus{statuses: []Status{{object: ir13, status: "invalid", msg: "Spec.VirtualHost.Fqdn must be specified"}}},
		},
		"self-edge produces a cycle": {
			objs: []*ingressroutev1.IngressRoute{ir6},
			want: IngressrouteStatus{statuses: []Status{{object: ir6, status: "invalid", msg: "route creates a delegation cycle: roots/self -> roots/self"}}},
		},
		"child delegates to parent, producing a cycle": {
			objs: []*ingressroutev1.IngressRoute{ir7, ir8},
			want: IngressrouteStatus{
				statuses: []Status{
					{object: ir8, status: "invalid", msg: "route creates a delegation cycle: roots/parent -> roots/child -> roots/parent"},
					{object: ir7, status: "valid", msg: "valid IngressRoute"}},
			},
		},
		"route has a list of services and also delegates": {
			objs: []*ingressroutev1.IngressRoute{ir9},
			want: IngressrouteStatus{statuses: []Status{{object: ir9, status: "invalid", msg: `route "/foo": cannot specify services and delegate in the same route`}}},
		},
		"ingressroute is an orphaned route": {
			objs: []*ingressroutev1.IngressRoute{ir8},
			want: IngressrouteStatus{statuses: []Status{{object: ir8, status: "orphaned", msg: "this IngressRoute is not part of a delegation chain from a root IngressRoute"}}},
		},
		"ingressroute delegates to multiple ingressroutes, one is invalid": {
			objs: []*ingressroutev1.IngressRoute{ir10, ir11, ir12},
			want: IngressrouteStatus{
				statuses: []Status{
					{object: ir11, status: "valid", msg: "valid IngressRoute"},
					{object: ir12, status: "invalid", msg: `route "/bar": service "foo": port must be in the range 1-65535`},
					{object: ir10, status: "valid", msg: "valid IngressRoute"}},
			},
		},
		"invalid parent orphans children": {
			objs: []*ingressroutev1.IngressRoute{ir14, ir11},
			want: IngressrouteStatus{
				statuses: []Status{
					{object: ir14, status: "invalid", msg: "Spec.VirtualHost.Fqdn must be specified"},
					{object: ir11, status: "orphaned", msg: "this IngressRoute is not part of a delegation chain from a root IngressRoute"}},
			},
		},
		"multi-parent children is not orphaned when one of the parents is invalid": {
			objs: []*ingressroutev1.IngressRoute{ir14, ir11, ir10},
			want: IngressrouteStatus{
				statuses: []Status{
					{object: ir14, status: "invalid", msg: "Spec.VirtualHost.Fqdn must be specified"},
					{object: ir11, status: "valid", msg: "valid IngressRoute"},
					{object: ir10, status: "valid", msg: "valid IngressRoute"}},
			},
		},
		"dag version": {
			objs:  []*ingressroutev1.IngressRoute{ir1},
			objs2: []*ingressroutev1.IngressRoute{ir1},
			want:  IngressrouteStatus{version: 1, statuses: []Status{{object: ir1, status: "valid", msg: "valid IngressRoute"}}},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var d DAG
			d.IngressRouteRootNamespaces = []string{"roots"}
			for _, o := range tc.objs {
				d.Insert(o)
			}
			got := d.Recompute()
			if len(tc.want.statuses) != len(got.statuses) {
				t.Fatalf("expected %d statuses, but got %d", len(tc.want.statuses), len(got.statuses))
			}

			for _, ex := range tc.want.statuses {
				var found bool
				for _, g := range got.statuses {
					if reflect.DeepEqual(ex, g) {
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("expected to find:\n%v\nbut did not find it in:\n%v", ex, got)
				}
			}

			// Run 2nd recompute
			if tc.objs2 != nil {
				for _, o := range tc.objs2 {
					d.Insert(o)
				}
				got = d.Recompute()
				if tc.want.version != got.version {
					t.Fatalf("expected version %d, but got %d", tc.want.version, got.version)
				}
			}
		})
	}
}

func routemap(routes ...*Route) map[string]*Route {
	m := make(map[string]*Route)
	for _, r := range routes {
		m[r.path] = r
	}
	return m
}

func servicemap(services ...*Service) map[portmeta]*Service {
	m := make(map[portmeta]*Service)
	for _, s := range services {
		m[s.toMeta()] = s
	}
	return m
}
