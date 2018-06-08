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
	sec1 := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret",
			Namespace: "default",
		},
		Data: secretdata("certificate", "key"),
	}

	tests := map[string]struct {
		objs []interface{}
		want []*VirtualHost
	}{
		"insert ingress w/ default backend": {
			objs: []interface{}{
				i1,
			},
			want: []*VirtualHost{{
				Port: 80,
				host: "*",
				routes: map[string]*Route{
					"/": &Route{
						path:    "/",
						object:  i1,
						backend: i1.Spec.Backend,
					},
				},
			}},
		},
		"insert ingress w/ single unnamed backend": {
			objs: []interface{}{
				i2,
			},
			want: []*VirtualHost{{
				Port: 80,
				host: "*",
				routes: map[string]*Route{
					"/": &Route{
						path:    "/",
						object:  i2,
						backend: &i2.Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Backend,
					},
				},
			}},
		},
		"insert ingress w/ host name and single backend": {
			objs: []interface{}{
				i3,
			},
			want: []*VirtualHost{{
				Port: 80,
				host: "kuard.example.com",
				routes: map[string]*Route{
					"/": &Route{
						path:    "/",
						object:  i3,
						backend: &i3.Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Backend,
					},
				},
			}},
		},
		"insert ingress w/ default backend then matching service": {
			objs: []interface{}{
				i1,
				s1,
			},
			want: []*VirtualHost{{
				Port: 80,
				host: "*",
				routes: map[string]*Route{
					"/": &Route{
						path:    "/",
						object:  i1,
						backend: i1.Spec.Backend,
						services: map[meta]*Service{
							meta{
								name:      "kuard",
								namespace: "default",
							}: &Service{
								object: s1,
							},
						},
					},
				},
			}},
		},
		"insert service then ingress w/ default backend": {
			objs: []interface{}{
				s1,
				i1,
			},
			want: []*VirtualHost{{
				Port: 80,
				host: "*",
				routes: map[string]*Route{
					"/": &Route{
						path:    "/",
						object:  i1,
						backend: i1.Spec.Backend,
						services: map[meta]*Service{
							meta{
								name:      "kuard",
								namespace: "default",
							}: &Service{
								object: s1,
							},
						},
					},
				},
			}},
		},
		"insert ingress w/ default backend then non-matching service": {
			objs: []interface{}{
				i1,
				s2,
			},
			want: []*VirtualHost{{
				Port: 80,
				host: "*",
				routes: map[string]*Route{
					"/": &Route{
						path:    "/",
						object:  i1,
						backend: i1.Spec.Backend,
					},
				},
			}},
		},
		"insert non matching service then ingress w/ default backend": {
			objs: []interface{}{
				s2,
				i1,
			},
			want: []*VirtualHost{{
				Port: 80,
				host: "*",
				routes: map[string]*Route{
					"/": &Route{
						path:    "/",
						object:  i1,
						backend: i1.Spec.Backend,
					},
				},
			}},
		},
		"insert ingress w/ default backend then matching service with wrong port": {
			objs: []interface{}{
				i1,
				s3,
			},
			want: []*VirtualHost{{
				Port: 80,
				host: "*",
				routes: map[string]*Route{
					"/": &Route{
						path:    "/",
						object:  i1,
						backend: i1.Spec.Backend,
					},
				},
			}},
		},
		"insert service then matching ingress w/ default backend but wrong port": {
			objs: []interface{}{
				s3,
				i1,
			},
			want: []*VirtualHost{{
				Port: 80,
				host: "*",
				routes: map[string]*Route{
					"/": &Route{
						path:    "/",
						object:  i1,
						backend: i1.Spec.Backend,
					},
				},
			}},
		},
		"insert unnamed ingress w/ single backend then matching service with wrong port": {
			objs: []interface{}{
				i2,
				s3,
			},
			want: []*VirtualHost{{
				Port: 80,
				host: "*",
				routes: map[string]*Route{
					"/": &Route{
						path:    "/",
						object:  i2,
						backend: &i2.Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Backend,
					},
				},
			}},
		},
		"insert service then matching unnamed ingress w/ single backend but wrong port": {
			objs: []interface{}{
				s3,
				i2,
			},
			want: []*VirtualHost{{
				Port: 80,
				host: "*",
				routes: map[string]*Route{
					"/": &Route{
						path:    "/",
						object:  i2,
						backend: &i2.Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Backend,
					},
				},
			}},
		},
		"insert ingress w/ default backend then matching service w/ named port": {
			objs: []interface{}{
				i4,
				s1,
			},
			want: []*VirtualHost{{
				Port: 80,
				host: "*",
				routes: map[string]*Route{
					"/": &Route{
						path:    "/",
						object:  i4,
						backend: i4.Spec.Backend,
						services: map[meta]*Service{
							meta{
								name:      "kuard",
								namespace: "default",
							}: &Service{
								object: s1,
							},
						},
					},
				},
			}},
		},
		"insert service w/ named port then ingress w/ default backend": {
			objs: []interface{}{
				s1,
				i4,
			},
			want: []*VirtualHost{{
				Port: 80,
				host: "*",
				routes: map[string]*Route{
					"/": &Route{
						path:    "/",
						object:  i4,
						backend: i4.Spec.Backend,
						services: map[meta]*Service{
							meta{
								name:      "kuard",
								namespace: "default",
							}: &Service{
								object: s1,
							},
						},
					},
				},
			}},
		},
		"insert ingress w/ single unnamed backend w/ named service port then service": {
			objs: []interface{}{
				i5,
				s1,
			},
			want: []*VirtualHost{{
				Port: 80,
				host: "*",
				routes: map[string]*Route{
					"/": &Route{
						path:    "/",
						object:  i5,
						backend: &i5.Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Backend,
						services: map[meta]*Service{
							meta{
								name:      "kuard",
								namespace: "default",
							}: &Service{
								object: s1,
							},
						},
					},
				},
			}},
		},
		"insert service then ingress w/ single unnamed backend w/ named service port": {
			objs: []interface{}{
				s1,
				i5,
			},
			want: []*VirtualHost{{
				Port: 80,
				host: "*",
				routes: map[string]*Route{
					"/": &Route{
						path:    "/",
						object:  i5,
						backend: &i5.Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Backend,
						services: map[meta]*Service{
							meta{
								name:      "kuard",
								namespace: "default",
							}: &Service{
								object: s1,
							},
						},
					},
				},
			}},
		},
		"insert secret": {
			objs: []interface{}{
				sec1,
			},
			want: []*VirtualHost{},
		},
		"insert secret then ingress w/o tls": {
			objs: []interface{}{
				sec1,
				i1,
			},
			want: []*VirtualHost{{
				Port: 80,
				host: "*",
				routes: map[string]*Route{
					"/": &Route{
						path:    "/",
						object:  i1,
						backend: i1.Spec.Backend,
					},
				},
			}},
		},
		"insert secret then ingress w/ tls": {
			objs: []interface{}{
				sec1,
				i3,
			},
			want: []*VirtualHost{{
				Port: 80,
				host: "kuard.example.com",
				routes: map[string]*Route{
					"/": &Route{
						path:    "/",
						object:  i3,
						backend: &i3.Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Backend,
					},
				},
			}, {
				Port: 443,
				host: "kuard.example.com",
				routes: map[string]*Route{
					"/": &Route{
						path:    "/",
						object:  i3,
						backend: &i3.Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Backend,
					},
				},
				secrets: map[meta]*Secret{
					meta{
						name:      "secret",
						namespace: "default",
					}: &Secret{
						object: sec1,
					},
				},
			}},
		},
		"insert ingress w/ tls then secret": {
			objs: []interface{}{
				i3,
				sec1,
			},
			want: []*VirtualHost{{
				Port: 80,
				host: "kuard.example.com",
				routes: map[string]*Route{
					"/": &Route{
						path:    "/",
						object:  i3,
						backend: &i3.Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Backend,
					},
				},
			}, {
				Port: 443,
				host: "kuard.example.com",
				routes: map[string]*Route{
					"/": &Route{
						path:    "/",
						object:  i3,
						backend: &i3.Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Backend,
					},
				},
				secrets: map[meta]*Secret{
					meta{
						name:      "secret",
						namespace: "default",
					}: &Secret{
						object: sec1,
					},
				},
			}},
		},
		"insert ingress w/ two vhosts": {
			objs: []interface{}{
				i6,
			},
			want: []*VirtualHost{{
				Port: 80,
				host: "a.example.com",
				routes: map[string]*Route{
					"/": &Route{
						path:    "/",
						object:  i6,
						backend: &i6.Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Backend,
					},
				},
			}, {
				Port: 80,
				host: "b.example.com",
				routes: map[string]*Route{
					"/": &Route{
						path:    "/",
						object:  i6,
						backend: &i6.Spec.Rules[1].IngressRuleValue.HTTP.Paths[0].Backend,
					},
				},
			}},
		},
		"insert ingress w/ two vhosts then matching service": {
			objs: []interface{}{
				i6,
				s1,
			},
			want: []*VirtualHost{{
				Port: 80,
				host: "a.example.com",
				routes: map[string]*Route{
					"/": &Route{
						path:    "/",
						object:  i6,
						backend: &i6.Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Backend,
						services: map[meta]*Service{
							meta{
								name:      "kuard",
								namespace: "default",
							}: &Service{
								object: s1,
							},
						},
					},
				},
			}, {
				Port: 80,
				host: "b.example.com",
				routes: map[string]*Route{
					"/": &Route{
						path:    "/",
						object:  i6,
						backend: &i6.Spec.Rules[1].IngressRuleValue.HTTP.Paths[0].Backend,
						services: map[meta]*Service{
							meta{
								name:      "kuard",
								namespace: "default",
							}: &Service{
								object: s1,
							},
						},
					},
				},
			}},
		},
		"insert service then ingress w/ two vhosts": {
			objs: []interface{}{
				s1,
				i6,
			},
			want: []*VirtualHost{{
				Port: 80,
				host: "a.example.com",
				routes: map[string]*Route{
					"/": &Route{
						path:    "/",
						object:  i6,
						backend: &i6.Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Backend,
						services: map[meta]*Service{
							meta{
								name:      "kuard",
								namespace: "default",
							}: &Service{
								object: s1,
							},
						},
					},
				},
			}, {
				Port: 80,
				host: "b.example.com",
				routes: map[string]*Route{
					"/": &Route{
						path:    "/",
						object:  i6,
						backend: &i6.Spec.Rules[1].IngressRuleValue.HTTP.Paths[0].Backend,
						services: map[meta]*Service{
							meta{
								name:      "kuard",
								namespace: "default",
							}: &Service{
								object: s1,
							},
						},
					},
				},
			}},
		},
		"insert ingress w/ two vhosts then service then secret": {
			objs: []interface{}{
				i6,
				s1,
				sec1,
			},
			want: []*VirtualHost{{
				Port: 80,
				host: "a.example.com",
				routes: map[string]*Route{
					"/": &Route{
						path:    "/",
						object:  i6,
						backend: &i6.Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Backend,
						services: map[meta]*Service{
							meta{
								name:      "kuard",
								namespace: "default",
							}: &Service{
								object: s1,
							},
						},
					},
				},
			}, {
				Port: 80,
				host: "b.example.com",
				routes: map[string]*Route{
					"/": &Route{
						path:    "/",
						object:  i6,
						backend: &i6.Spec.Rules[1].IngressRuleValue.HTTP.Paths[0].Backend,
						services: map[meta]*Service{
							meta{
								name:      "kuard",
								namespace: "default",
							}: &Service{
								object: s1,
							},
						},
					},
				},
			}, {
				Port: 443,
				host: "b.example.com",
				routes: map[string]*Route{
					"/": &Route{
						path:    "/",
						object:  i6,
						backend: &i6.Spec.Rules[1].IngressRuleValue.HTTP.Paths[0].Backend,
						services: map[meta]*Service{
							meta{
								name:      "kuard",
								namespace: "default",
							}: &Service{
								object: s1,
							},
						},
					},
				},
				secrets: map[meta]*Secret{
					meta{
						name:      "secret",
						namespace: "default",
					}: &Secret{
						object: sec1,
					},
				},
			}},
		},
		"insert service then secret then ingress w/ two vhosts": {
			objs: []interface{}{
				s1,
				sec1,
				i6,
			},
			want: []*VirtualHost{{
				Port: 80,
				host: "a.example.com",
				routes: map[string]*Route{
					"/": &Route{
						path:    "/",
						object:  i6,
						backend: &i6.Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Backend,
						services: map[meta]*Service{
							meta{
								name:      "kuard",
								namespace: "default",
							}: &Service{
								object: s1,
							},
						},
					},
				},
			}, {
				Port: 80,
				host: "b.example.com",
				routes: map[string]*Route{
					"/": &Route{
						path:    "/",
						object:  i6,
						backend: &i6.Spec.Rules[1].IngressRuleValue.HTTP.Paths[0].Backend,
						services: map[meta]*Service{
							meta{
								name:      "kuard",
								namespace: "default",
							}: &Service{
								object: s1,
							},
						},
					},
				},
			}, {
				Port: 443,
				host: "b.example.com",
				routes: map[string]*Route{
					"/": &Route{
						path:    "/",
						object:  i6,
						backend: &i6.Spec.Rules[1].IngressRuleValue.HTTP.Paths[0].Backend,
						services: map[meta]*Service{
							meta{
								name:      "kuard",
								namespace: "default",
							}: &Service{
								object: s1,
							},
						},
					},
				},
				secrets: map[meta]*Secret{
					meta{
						name:      "secret",
						namespace: "default",
					}: &Secret{
						object: sec1,
					},
				},
			}},
		},
		"insert ingress w/ two paths": {
			objs: []interface{}{
				i7,
			},
			want: []*VirtualHost{{
				Port: 80,
				host: "b.example.com",
				routes: map[string]*Route{
					"/": &Route{
						path:    "/",
						object:  i7,
						backend: &i7.Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Backend,
					},
					"/kuarder": &Route{
						path:    "/kuarder",
						object:  i7,
						backend: &i7.Spec.Rules[0].IngressRuleValue.HTTP.Paths[1].Backend,
					},
				},
			}},
		},
		"insert ingress w/ two paths then services": {
			objs: []interface{}{
				i7,
				s2,
				s1,
			},
			want: []*VirtualHost{{
				Port: 80,
				host: "b.example.com",
				routes: map[string]*Route{
					"/": &Route{
						path:    "/",
						object:  i7,
						backend: &i7.Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Backend,
						services: map[meta]*Service{
							meta{
								name:      "kuard",
								namespace: "default",
							}: &Service{
								object: s1,
							},
						},
					},
					"/kuarder": &Route{
						path:    "/kuarder",
						object:  i7,
						backend: &i7.Spec.Rules[0].IngressRuleValue.HTTP.Paths[1].Backend,
						services: map[meta]*Service{
							meta{
								name:      "kuarder",
								namespace: "default",
							}: &Service{
								object: s2,
							},
						},
					},
				},
			}},
		},
		"insert two services then ingress w/ two ingress rules": {
			objs: []interface{}{
				s1, s2, i8,
			},
			want: []*VirtualHost{{
				Port: 80,
				host: "b.example.com",
				routes: map[string]*Route{
					"/": &Route{
						path:    "/",
						object:  i8,
						backend: &i8.Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Backend,
						services: map[meta]*Service{
							meta{
								name:      "kuard",
								namespace: "default",
							}: &Service{
								object: s1,
							},
						},
					},
					"/kuarder": &Route{
						path:    "/kuarder",
						object:  i8,
						backend: &i8.Spec.Rules[1].IngressRuleValue.HTTP.Paths[0].Backend,
						services: map[meta]*Service{
							meta{
								name:      "kuarder",
								namespace: "default",
							}: &Service{
								object: s2,
							},
						},
					},
				},
			}},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var d DAG
			for _, o := range tc.objs {
				d.Insert(o)
			}
			d.Recompute()

			got := make(map[hostport]*VirtualHost)
			d.Visit(func(v Vertex) {
				if v, ok := v.(*VirtualHost); ok {
					got[hostport{host: v.FQDN(), port: v.Port}] = v
				}
			})

			want := make(map[hostport]*VirtualHost)
			for _, v := range tc.want {
				want[hostport{host: v.FQDN(), port: v.Port}] = v
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
		want   []*VirtualHost
	}{
		"remove ingress w/ default backend": {
			insert: []interface{}{
				i1,
			},
			remove: []interface{}{
				i1,
			},
			want: []*VirtualHost{},
		},
		"remove ingress w/ single unnamed backend": {
			insert: []interface{}{
				i2,
			},
			remove: []interface{}{
				i2,
			},
			want: []*VirtualHost{},
		},
		"insert ingress w/ host name and single backend": {
			insert: []interface{}{
				i3,
			},
			want: []*VirtualHost{{
				Port: 80,
				host: "kuard.example.com",
				routes: map[string]*Route{
					"/": &Route{
						path:    "/",
						object:  i3,
						backend: &i3.Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Backend,
					},
				},
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
			want: []*VirtualHost{},
		},
		"remove service leaving ingress w/ default backend": {
			insert: []interface{}{
				s1,
				i1,
			},
			remove: []interface{}{
				s1,
			},
			want: []*VirtualHost{{
				Port: 80,
				host: "*",
				routes: map[string]*Route{
					"/": &Route{
						path:    "/",
						object:  i1,
						backend: i1.Spec.Backend,
					},
				},
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
			want: []*VirtualHost{{
				Port: 80,
				host: "*",
				routes: map[string]*Route{
					"/": &Route{
						path:    "/",
						object:  i1,
						backend: i1.Spec.Backend,
					},
				},
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
			want: []*VirtualHost{},
		},
		"remove service w/ named service port leaving ingress w/ single unnamed backend": {
			insert: []interface{}{
				i5,
				s1,
			},
			remove: []interface{}{
				s1,
			},
			want: []*VirtualHost{{
				Port: 80,
				host: "*",
				routes: map[string]*Route{
					"/": &Route{
						path:    "/",
						object:  i5,
						backend: &i5.Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Backend,
					},
				},
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
			want: []*VirtualHost{{
				Port: 80,
				host: "kuard.example.com",
				routes: map[string]*Route{
					"/": &Route{
						path:    "/",
						object:  i3,
						backend: &i3.Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Backend,
					},
				},
			}},
		},
		"remove ingress w/ two vhosts": {
			insert: []interface{}{
				i6,
			},
			remove: []interface{}{
				i6,
			},
			want: []*VirtualHost{},
		},
		"remove service leaving ingress w/ two vhosts": {
			insert: []interface{}{
				i6,
				s1,
			},
			remove: []interface{}{
				s1,
			},
			want: []*VirtualHost{{
				Port: 80,
				host: "a.example.com",
				routes: map[string]*Route{
					"/": &Route{
						path:    "/",
						object:  i6,
						backend: &i6.Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Backend,
					},
				},
			}, {
				Port: 80,
				host: "b.example.com",
				routes: map[string]*Route{
					"/": &Route{
						path:    "/",
						object:  i6,
						backend: &i6.Spec.Rules[1].IngressRuleValue.HTTP.Paths[0].Backend,
					},
				},
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
			want: []*VirtualHost{{
				Port: 80,
				host: "a.example.com",
				routes: map[string]*Route{
					"/": &Route{
						path:    "/",
						object:  i6,
						backend: &i6.Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Backend,
						services: map[meta]*Service{
							meta{
								name:      "kuard",
								namespace: "default",
							}: &Service{
								object: s1,
							},
						},
					},
				},
			}, {
				Port: 80,
				host: "b.example.com",
				routes: map[string]*Route{
					"/": &Route{
						path:    "/",
						object:  i6,
						backend: &i6.Spec.Rules[1].IngressRuleValue.HTTP.Paths[0].Backend,
						services: map[meta]*Service{
							meta{
								name:      "kuard",
								namespace: "default",
							}: &Service{
								object: s1,
							},
						},
					},
				},
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
			want: []*VirtualHost{{
				Port: 80,
				host: "a.example.com",
				routes: map[string]*Route{
					"/": &Route{
						path:    "/",
						object:  i6,
						backend: &i6.Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Backend,
					},
				},
			}, {
				Port: 80,
				host: "b.example.com",
				routes: map[string]*Route{
					"/": &Route{
						path:    "/",
						object:  i6,
						backend: &i6.Spec.Rules[1].IngressRuleValue.HTTP.Paths[0].Backend,
					},
				},
			}, {
				Port: 443,
				host: "b.example.com",
				routes: map[string]*Route{
					"/": &Route{
						path:    "/",
						object:  i6,
						backend: &i6.Spec.Rules[1].IngressRuleValue.HTTP.Paths[0].Backend,
					},
				},
				secrets: map[meta]*Secret{
					meta{
						name:      "secret",
						namespace: "default",
					}: &Secret{
						object: sec1,
					},
				},
			}},
		},
		"remove service from  ingress w/ two paths": {
			insert: []interface{}{
				i7,
				s2,
				s1,
			},
			remove: []interface{}{
				s2,
			},
			want: []*VirtualHost{{
				Port: 80,
				host: "b.example.com",
				routes: map[string]*Route{
					"/": &Route{
						path:    "/",
						object:  i7,
						backend: &i7.Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Backend,
						services: map[meta]*Service{
							meta{
								name:      "kuard",
								namespace: "default",
							}: &Service{
								object: s1,
							},
						},
					},
					"/kuarder": &Route{
						path:    "/kuarder",
						object:  i7,
						backend: &i7.Spec.Rules[0].IngressRuleValue.HTTP.Paths[1].Backend,
					},
				},
			}},
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

			got := make(map[hostport]*VirtualHost)
			d.Visit(func(v Vertex) {
				if v, ok := v.(*VirtualHost); ok {
					got[hostport{host: v.FQDN(), port: v.Port}] = v
				}
			})

			want := make(map[hostport]*VirtualHost)
			for _, v := range tc.want {
				want[hostport{host: v.FQDN(), port: v.Port}] = v
			}

			if !reflect.DeepEqual(want, got) {
				t.Fatal("expected:\n", want, "\ngot:\n", got)
			}

		})
	}
}

func (v *VirtualHost) String() string {
	return fmt.Sprintf("host: %v:%d {routes: %v, secrets: %v}", v.FQDN(), v.Port, v.routes, v.secrets)
}

func (r *Route) String() string {
	return fmt.Sprintf("route: %q {services: %v, object: %p, backend: %+v}", r.Prefix(), r.services, r.object, *r.backend)
}

func (s *Service) String() string {
	return fmt.Sprintf("service: %s/%s {ports: %v}", s.object.Namespace, s.object.Name, s.object.Spec.Ports)
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
