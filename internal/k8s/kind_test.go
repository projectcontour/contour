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

package k8s

import (
	"testing"

	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/stretchr/testify/assert"
	core_v1 "k8s.io/api/core/v1"
	networking_v1 "k8s.io/api/networking/v1"
	"k8s.io/api/networking/v1beta1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestKindOf(t *testing.T) {
	cases := []struct {
		Kind string
		Obj  interface{}
	}{
		{"Secret", &core_v1.Secret{}},
		{"Service", &core_v1.Service{}},
		{"Endpoints", &core_v1.Endpoints{}},
		{"Pod", &core_v1.Pod{}},
		{"Ingress", &v1beta1.Ingress{}},
		{"Ingress", &networking_v1.Ingress{}},
		{"HTTPProxy", &contour_api_v1.HTTPProxy{}},
		{"TLSCertificateDelegation", &contour_api_v1.TLSCertificateDelegation{}},
		{"ExtensionService", &v1alpha1.ExtensionService{}},
		{"Foo", &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "test.projectcontour.io/v1",
				"kind":       "Foo",
			}},
		},
	}

	for _, c := range cases {
		assert.Equal(t, c.Kind, KindOf(c.Obj))
	}
}

func TestVersionOf(t *testing.T) {
	cases := []struct {
		Version string
		Obj     interface{}
	}{
		{"v1", &core_v1.Secret{}},
		{"v1", &core_v1.Service{}},
		{"v1", &core_v1.Endpoints{}},
		{"networking.k8s.io/v1beta1", &v1beta1.Ingress{}},
		{"projectcontour.io/v1", &contour_api_v1.HTTPProxy{}},
		{"projectcontour.io/v1", &contour_api_v1.TLSCertificateDelegation{}},
		{"projectcontour.io/v1alpha1", &v1alpha1.ExtensionService{}},
		{"test.projectcontour.io/v1", &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "test.projectcontour.io/v1",
				"kind":       "Foo",
			}},
		},
	}

	for _, c := range cases {
		assert.Equal(t, c.Version, VersionOf(c.Obj))
	}
}
