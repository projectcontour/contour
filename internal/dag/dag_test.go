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

// The DAG is senstive to ordering, adding an ingress, then a service,
// should have the same result as adding a sevice, then an ingress, but
// operationally triggers very different code paths. This Test case attemps
// to cover all the permulations.
func TestDAG(t *testing.T) {
	i1 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: v1beta1.IngressSpec{
			Backend: backend("kuard", intstr.FromInt(8080))},
	}
	s1 := service("default", "kuard",
		v1.ServicePort{
			Protocol: "TCP",
			Port:     8080,
		},
	)

	tests := map[string]struct {
		objs  []interface{}
		roots []*VirtualHost
	}{
		"insert service then default vhost": {
			objs: []interface{}{
				s1,
				i1,
			},
			roots: []*VirtualHost{{
				object: i1,
				host:   "*",
				children: []Vertex{
					&Route{
						path: "/",
						children: []Vertex{
							&Service{
								object: s1,
							},
						},
					},
				},
			}},
		},
		"insert default vhost then service": {
			objs: []interface{}{
				i1,
				s1,
			},
			roots: []*VirtualHost{{
				object: i1,
				host:   "*",
				children: []Vertex{
					&Route{
						path: "/",
						children: []Vertex{
							&Service{
								object: s1,
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

			got := make(map[string]Vertex)
			d.Visit(func(v Vertex) {
				if v, ok := v.(*VirtualHost); ok {
					got[v.FQDN()] = v
				}
			})

			want := make(map[string]Vertex)
			for _, r := range tc.roots {
				want[r.FQDN()] = r
			}

			if !reflect.DeepEqual(want, got) {
				t.Fatalf("expected: %v, got: %v", want, got)
			}

		})

	}
}

func (v *VirtualHost) String() string {
	return fmt.Sprintf("%T: %q %v", v, v.FQDN(), v.children)
}

func (r *Route) String() string {
	return fmt.Sprintf("%T: %q %v", r, r.Prefix(), r.children)
}

func (s *Service) String() string {
	return fmt.Sprintf("%T: %q", s, s.Name())
}

func backend(name string, port intstr.IntOrString) *v1beta1.IngressBackend {
	return &v1beta1.IngressBackend{
		ServiceName: name,
		ServicePort: port,
	}
}

func service(ns, name string, ports ...v1.ServicePort) *v1.Service {
	return &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: v1.ServiceSpec{
			Ports: ports,
		},
	}
}
