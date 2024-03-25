// Copyright Project Contour Authors
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

package routegen

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/fixture"
)

func TestReadManifestFiles(t *testing.T) {
	log := fixture.NewTestLogger(t)

	tests := []struct {
		name  string
		input []string
		want  []runtime.Object
	}{
		{
			name:  "empty source",
			input: []string{"testdata/empty.yaml"},
			want:  nil,
		},
		{
			name:  "single source",
			input: []string{"testdata/httpproxy_a.yaml"},
			want: []runtime.Object{
				&core_v1.Service{
					TypeMeta: meta_v1.TypeMeta{
						APIVersion: "v1",
						Kind:       "Service",
					},
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "s1",
						Namespace: "service-a",
					},
					Spec: core_v1.ServiceSpec{
						Selector: map[string]string{
							"app.kubernetes.io/name": "test",
						},
						Ports: []core_v1.ServicePort{
							{Protocol: "TCP", Port: 80, TargetPort: intstr.FromInt(8080)},
						},
					},
				},
				&contour_v1.HTTPProxy{
					TypeMeta: meta_v1.TypeMeta{
						APIVersion: "projectcontour.io/v1",
						Kind:       "HTTPProxy",
					},
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "proxy-a",
						Namespace: "service-a",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "foo-basic.bar.com",
						},
						Routes: []contour_v1.Route{
							{
								Conditions: []contour_v1.MatchCondition{
									{Prefix: "/"},
								},
								Services: []contour_v1.Service{
									{Name: "s1", Port: 80},
								},
							},
						},
					},
				},
			},
		},
		{
			name:  "multiple sources",
			input: []string{"testdata/httpproxy_a.yaml", "testdata/httpproxy_b.yaml"},
			want: []runtime.Object{
				&core_v1.Service{
					TypeMeta: meta_v1.TypeMeta{
						APIVersion: "v1",
						Kind:       "Service",
					},
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "s1",
						Namespace: "service-a",
					},
					Spec: core_v1.ServiceSpec{
						Selector: map[string]string{
							"app.kubernetes.io/name": "test",
						},
						Ports: []core_v1.ServicePort{
							{Protocol: "TCP", Port: 80, TargetPort: intstr.FromInt(8080)},
						},
					},
				},
				&contour_v1.HTTPProxy{
					TypeMeta: meta_v1.TypeMeta{
						APIVersion: "projectcontour.io/v1",
						Kind:       "HTTPProxy",
					},
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "proxy-a",
						Namespace: "service-a",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "foo-basic.bar.com",
						},
						Routes: []contour_v1.Route{
							{
								Conditions: []contour_v1.MatchCondition{
									{Prefix: "/"},
								},
								Services: []contour_v1.Service{
									{Name: "s1", Port: 80},
								},
							},
						},
					},
				},
				&core_v1.Service{
					TypeMeta: meta_v1.TypeMeta{
						APIVersion: "v1",
						Kind:       "Service",
					},
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "s2",
						Namespace: "service-b",
					},
					Spec: core_v1.ServiceSpec{
						Selector: map[string]string{
							"app.kubernetes.io/name": "test",
						},
						Ports: []core_v1.ServicePort{
							{Protocol: "TCP", Port: 80, TargetPort: intstr.FromInt(8080)},
						},
					},
				},
				&contour_v1.HTTPProxy{
					TypeMeta: meta_v1.TypeMeta{
						APIVersion: "projectcontour.io/v1",
						Kind:       "HTTPProxy",
					},
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "proxy-b",
						Namespace: "service-b",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "bar-basic.foo.com",
						},
						Routes: []contour_v1.Route{
							{
								Conditions: []contour_v1.MatchCondition{
									{Prefix: "/"},
								},
								Services: []contour_v1.Service{
									{Name: "s2", Port: 80},
								},
							},
						},
					},
				},
			},
		},
		{
			name:  "unsupported resources",
			input: []string{"testdata/unsupported_spec.yaml"},
			want:  nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ReadManifestFiles(test.input, log)
			if err != nil {
				t.Errorf("expected no errors, got %s", err.Error())
			}
			if diff := cmp.Diff(got, test.want); diff != "" {
				t.Errorf("ReadManifestFiles - %s: (-got +want)\n%s", test.name, diff)
			}
		})
	}
}

func TestReadManifestFilesErrors(t *testing.T) {
	log := fixture.NewTestLogger(t)

	tests := []struct {
		name  string
		input []string
	}{
		{
			name:  "non-existent file",
			input: []string{"fake_file.yaml"},
		},
		{
			name:  "arbitrary yaml source",
			input: []string{"testdata/non_k8s.yaml"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ReadManifestFiles(test.input, log)
			if err == nil {
				t.Errorf("expected errors, got none")
			}
		})
	}
}
