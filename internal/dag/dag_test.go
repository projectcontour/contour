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

func TestInsert(t *testing.T) {
	// The DAG is senstive to ordering, adding an ingress, then a service,
	// should have the same result as adding a sevice, then an ingress, but
	// operationally triggers very different code paths.

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
				host: "*",
				routes: map[string]*Route{
					"/": &Route{
						path:    "/",
						object:  i1,
						backend: i1.Spec.Backend,
					},
				},
				secrets: make(map[meta]*Secret),
			}},
		},
		"insert ingress w/ single unnamed backend": {
			objs: []interface{}{
				i2,
			},
			want: []*VirtualHost{{
				host: "*",
				routes: map[string]*Route{
					"/": &Route{
						path:    "/",
						object:  i2,
						backend: &i2.Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Backend,
					},
				},
				secrets: make(map[meta]*Secret),
			}},
		},
		"insert ingress w/ host name and single backend": {
			objs: []interface{}{
				i3,
			},
			want: []*VirtualHost{{
				host: "kuard.example.com",
				routes: map[string]*Route{
					"/": &Route{
						path:    "/",
						object:  i3,
						backend: &i3.Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Backend,
					},
				},
				secrets: make(map[meta]*Secret),
			}},
		},
		"insert ingress w/ default backend then matching service": {
			objs: []interface{}{
				i1,
				s1,
			},
			want: []*VirtualHost{{
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
				secrets: make(map[meta]*Secret),
			}},
		},
		"insert service then ingress w/ default backend": {
			objs: []interface{}{
				s1,
				i1,
			},
			want: []*VirtualHost{{
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
				secrets: make(map[meta]*Secret),
			}},
		},
		"insert ingress w/ default backend then non-matching service": {
			objs: []interface{}{
				i1,
				s2,
			},
			want: []*VirtualHost{{
				host: "*",
				routes: map[string]*Route{
					"/": &Route{
						path:    "/",
						object:  i1,
						backend: i1.Spec.Backend,
					},
				},
				secrets: make(map[meta]*Secret),
			}},
		},
		"insert non matching service then ingress w/ default backend": {
			objs: []interface{}{
				s2,
				i1,
			},
			want: []*VirtualHost{{
				host: "*",
				routes: map[string]*Route{
					"/": &Route{
						path:    "/",
						object:  i1,
						backend: i1.Spec.Backend,
					},
				},
				secrets: make(map[meta]*Secret),
			}},
		},
		"insert ingress w/ default backend then matching service with wrong port": {
			objs: []interface{}{
				i1,
				s3,
			},
			want: []*VirtualHost{{
				host: "*",
				routes: map[string]*Route{
					"/": &Route{
						path:    "/",
						object:  i1,
						backend: i1.Spec.Backend,
					},
				},
				secrets: make(map[meta]*Secret),
			}},
		},
		"insert service then matching ingress w/ default backend but wrong port": {
			objs: []interface{}{
				s3,
				i1,
			},
			want: []*VirtualHost{{
				host: "*",
				routes: map[string]*Route{
					"/": &Route{
						path:    "/",
						object:  i1,
						backend: i1.Spec.Backend,
					},
				},
				secrets: make(map[meta]*Secret),
			}},
		},
		"insert unnamed ingress w/ single backend then matching service with wrong port": {
			objs: []interface{}{
				i2,
				s3,
			},
			want: []*VirtualHost{{
				host: "*",
				routes: map[string]*Route{
					"/": &Route{
						path:    "/",
						object:  i2,
						backend: &i2.Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Backend,
					},
				},
				secrets: make(map[meta]*Secret),
			}},
		},
		"insert service then matching unnamed ingress w/ single backend but wrong port": {
			objs: []interface{}{
				s3,
				i2,
			},
			want: []*VirtualHost{{
				host: "*",
				routes: map[string]*Route{
					"/": &Route{
						path:    "/",
						object:  i2,
						backend: &i2.Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Backend,
					},
				},
				secrets: make(map[meta]*Secret),
			}},
		},
		"insert ingress w/ default backend then matching service w/ named port": {
			objs: []interface{}{
				i4,
				s1,
			},
			want: []*VirtualHost{{
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
				secrets: make(map[meta]*Secret),
			}},
		},
		"insert service w/ named port then ingress w/ default backend": {
			objs: []interface{}{
				s1,
				i4,
			},
			want: []*VirtualHost{{
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
				secrets: make(map[meta]*Secret),
			}},
		},
		"insert ingress w/ single unnamed backend w/ named service port then service": {
			objs: []interface{}{
				i5,
				s1,
			},
			want: []*VirtualHost{{
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
				secrets: make(map[meta]*Secret),
			}},
		},
		"insert service then ingress w/ single unnamed backend w/ named service port": {
			objs: []interface{}{
				s1,
				i5,
			},
			want: []*VirtualHost{{
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
				secrets: make(map[meta]*Secret),
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
				host: "*",
				routes: map[string]*Route{
					"/": &Route{
						path:    "/",
						object:  i1,
						backend: i1.Spec.Backend,
					},
				},
				secrets: make(map[meta]*Secret),
			}},
		},
		"insert secret then ingress w/ tls": {
			objs: []interface{}{
				sec1,
				i3,
			},
			want: []*VirtualHost{{
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
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var d DAG
			for _, o := range tc.objs {
				d.Insert(o)
			}

			got := make(map[string]*VirtualHost)
			d.Visit(func(v Vertex) {
				if v, ok := v.(*VirtualHost); ok {
					got[v.FQDN()] = v
				}
			})

			want := make(map[string]*VirtualHost)
			for _, vh := range tc.want {
				want[vh.FQDN()] = vh
			}

			if !reflect.DeepEqual(want, got) {
				t.Fatal("expected:\n", want, "\ngot:\n", got)
			}

		})
	}
}

func (v *VirtualHost) String() string {
	return fmt.Sprintf("host: %v {routes: %v, secrets: %v}", v.FQDN(), v.routes, v.secrets)
}

func (r *Route) String() string {
	return fmt.Sprintf("route: %q {services: %v}", r.Prefix(), r.services)
}

func (s *Service) String() string {
	return fmt.Sprintf("service: %s/%s {ports: %v}", s.object.Namespace, s.object.Name, s.object.Spec.Ports)
}

func (s *Secret) String() string {
	return fmt.Sprintf("secret: %s/%s", s.object.Namespace, s.object.Name)
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
