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

	s2 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "includehealth",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:       "http",
					Protocol:   "TCP",
					Port:       8080,
					TargetPort: intstr.FromInt(8080),
				},
				{
					Name:       "health",
					Protocol:   "TCP",
					Port:       8998,
					TargetPort: intstr.FromInt(8998),
				},
			},
		},
	}

	externalNameValid := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "externalnamevalid",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Type:         v1.ServiceTypeExternalName,
			ExternalName: "external.projectcontour.io",
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(80),
			}},
		},
	}

	externalNameLocalhost := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "externalnamelocalhost",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Type:         v1.ServiceTypeExternalName,
			ExternalName: "localhost",
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(80),
			}},
		},
	}

	services := map[types.NamespacedName]*v1.Service{
		{Name: "service1", Namespace: "default"}:              s1,
		{Name: "servicehealthcheck", Namespace: "default"}:    s2,
		{Name: "externalnamevalid", Namespace: "default"}:     externalNameValid,
		{Name: "externalnamelocalhost", Namespace: "default"}: externalNameLocalhost,
	}

	tests := map[string]struct {
		types.NamespacedName
		port                  int
		healthPort            int
		enableExternalNameSvc bool
		want                  *Service
		wantErr               error
	}{
		"lookup service by port number": {
			NamespacedName: types.NamespacedName{Name: "service1", Namespace: "default"},
			port:           8080,
			want:           service(s1),
		},

		"when service does not exist an error is returned": {
			NamespacedName: types.NamespacedName{Name: "nonexistent-service", Namespace: "default"},
			port:           8080,
			wantErr:        errors.New(`service "default/nonexistent-service" not found`),
		},
		"when service port does not exist an error is returned": {
			NamespacedName: types.NamespacedName{Name: "service1", Namespace: "default"},
			port:           9999,
			wantErr:        errors.New(`port "9999" on service "default/service1" not matched`),
		},
		"when health port and service port are different": {
			NamespacedName: types.NamespacedName{Name: "servicehealthcheck", Namespace: "default"},
			port:           8080,
			healthPort:     8998,
			want:           healthService(s2),
		},
		"when health port does not exist an error is returned": {
			NamespacedName: types.NamespacedName{Name: "servicehealthcheck", Namespace: "default"},
			port:           8080,
			healthPort:     8999,
			wantErr:        errors.New(`port "8999" on service "default/servicehealthcheck" not matched`),
		},
		"When ExternalName Services are not disabled no error is returned": {
			NamespacedName: types.NamespacedName{Name: "externalnamevalid", Namespace: "default"},
			port:           80,
			want: &Service{
				Weighted: WeightedService{
					Weight:           1,
					ServiceName:      "externalnamevalid",
					ServiceNamespace: "default",
					ServicePort: v1.ServicePort{
						Name:       "http",
						Protocol:   "TCP",
						Port:       80,
						TargetPort: intstr.FromInt(80),
					},
					HealthPort: v1.ServicePort{
						Name:       "http",
						Protocol:   "TCP",
						Port:       80,
						TargetPort: intstr.FromInt(80),
					},
				},
				ExternalName: "external.projectcontour.io",
			},
			enableExternalNameSvc: true,
		},
		"When ExternalName Services are disabled an error is returned": {
			NamespacedName: types.NamespacedName{Name: "externalnamevalid", Namespace: "default"},
			port:           80,
			wantErr:        errors.New(`default/externalnamevalid is an ExternalName service, these are not currently enabled. See the config.enableExternalNameService config file setting`),
		},
		"When ExternalName Services are enabled but a localhost ExternalName is used an error is returned": {
			NamespacedName:        types.NamespacedName{Name: "externalnamelocalhost", Namespace: "default"},
			port:                  80,
			wantErr:               errors.New(`default/externalnamelocalhost is an ExternalName service that points to localhost, this is not allowed`),
			enableExternalNameSvc: true,
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

			got, gotErr := dag.EnsureService(tc.NamespacedName, tc.port, tc.healthPort, &b.Source, tc.enableExternalNameSvc)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.wantErr, gotErr)
		})
	}
}
