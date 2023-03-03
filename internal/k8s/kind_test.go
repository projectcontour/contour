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
	v1 "k8s.io/api/core/v1"
	networking_v1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayapi_v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

func TestKindOf(t *testing.T) {
	cases := []struct {
		Kind string
		Obj  interface{}
	}{
		{"Secret", &v1.Secret{}},
		{"Service", &v1.Service{}},
		{"Namespace", &v1.Namespace{}},
		{"Endpoints", &v1.Endpoints{}},
		{"Pod", &v1.Pod{}},
		{"Ingress", &networking_v1.Ingress{}},
		{"HTTPProxy", &contour_api_v1.HTTPProxy{}},
		{"TLSCertificateDelegation", &contour_api_v1.TLSCertificateDelegation{}},
		{"ExtensionService", &v1alpha1.ExtensionService{}},
		{"ContourConfiguration", &v1alpha1.ContourConfiguration{}},
		{"ContourDeployment", &v1alpha1.ContourDeployment{}},
		{"GRPCRoute", &gatewayapi_v1alpha2.GRPCRoute{}},
		{"HTTPRoute", &gatewayapi_v1beta1.HTTPRoute{}},
		{"TLSRoute", &gatewayapi_v1alpha2.TLSRoute{}},
		{"Gateway", &gatewayapi_v1beta1.Gateway{}},
		{"GatewayClass", &gatewayapi_v1beta1.GatewayClass{}},
		{"ReferenceGrant", &gatewayapi_v1beta1.ReferenceGrant{}},
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
		{"v1", &v1.Secret{}},
		{"v1", &v1.Service{}},
		{"v1", &v1.Endpoints{}},
		{"networking.k8s.io/v1", &networking_v1.Ingress{}},
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
