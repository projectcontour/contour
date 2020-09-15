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

package dag

import (
	"errors"
	"testing"

	"github.com/projectcontour/contour/internal/fixture"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
)

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
	services := map[types.NamespacedName]*v1.Service{
		{Name: "service1", Namespace: "default"}: s1,
	}

	tests := map[string]struct {
		types.NamespacedName
		port    intstr.IntOrString
		want    *Service
		wantErr error
	}{
		"lookup service by port number": {
			NamespacedName: types.NamespacedName{Name: "service1", Namespace: "default"},
			port:           intstr.FromInt(8080),
			want:           service(s1),
		},
		"lookup service by port name": {
			NamespacedName: types.NamespacedName{Name: "service1", Namespace: "default"},
			port:           intstr.FromString("http"),
			want:           service(s1),
		},
		"lookup service by port number (as string)": {
			NamespacedName: types.NamespacedName{Name: "service1", Namespace: "default"},
			port:           intstr.Parse("8080"),
			want:           service(s1),
		},
		"lookup service by port number (from string)": {
			NamespacedName: types.NamespacedName{Name: "service1", Namespace: "default"},
			port:           intstr.FromString("8080"),
			want:           service(s1),
		},
		"when service does not exist an error is returned": {
			NamespacedName: types.NamespacedName{Name: "nonexistent-service", Namespace: "default"},
			port:           intstr.FromString("8080"),
			wantErr:        errors.New(`service "default/nonexistent-service" not found`),
		},
		"when port does not exist an error is returned": {
			NamespacedName: types.NamespacedName{Name: "service1", Namespace: "default"},
			port:           intstr.FromString("9999"),
			wantErr:        errors.New(`port "9999" on service "default/service1" not matched`),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			b := Builder{
				Source: KubernetesCache{
					services:    services,
					FieldLogger: fixture.NewTestLogger(t),
				},
			}

			var dag DAG

			got, gotErr := dag.EnsureService(tc.NamespacedName, tc.port, &b.Source)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.wantErr, gotErr)
		})
	}
}
