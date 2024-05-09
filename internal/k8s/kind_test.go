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

	"github.com/stretchr/testify/assert"
	core_v1 "k8s.io/api/core/v1"
	networking_v1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	gatewayapi_v1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayapi_v1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"
	gatewayapi_v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
)

func TestKindOf(t *testing.T) {
	cases := []struct {
		Kind string
		Obj  any
	}{
		{"Secret", &core_v1.Secret{}},
		{"Service", &core_v1.Service{}},
		{"Namespace", &core_v1.Namespace{}},
		{"Endpoints", &core_v1.Endpoints{}},
		{"Pod", &core_v1.Pod{}},
		{"Ingress", &networking_v1.Ingress{}},
		{"HTTPProxy", &contour_v1.HTTPProxy{}},
		{"TLSCertificateDelegation", &contour_v1.TLSCertificateDelegation{}},
		{"ExtensionService", &contour_v1alpha1.ExtensionService{}},
		{"ContourConfiguration", &contour_v1alpha1.ContourConfiguration{}},
		{"ContourDeployment", &contour_v1alpha1.ContourDeployment{}},
		{"GRPCRoute", &gatewayapi_v1.GRPCRoute{}},
		{"HTTPRoute", &gatewayapi_v1.HTTPRoute{}},
		{"TLSRoute", &gatewayapi_v1alpha2.TLSRoute{}},
		{"TCPRoute", &gatewayapi_v1alpha2.TCPRoute{}},
		{"Gateway", &gatewayapi_v1.Gateway{}},
		{"GatewayClass", &gatewayapi_v1.GatewayClass{}},
		{"ReferenceGrant", &gatewayapi_v1beta1.ReferenceGrant{}},
		{"BackendTLSPolicy", &gatewayapi_v1alpha3.BackendTLSPolicy{}},
		{
			"Foo", &unstructured.Unstructured{
				Object: map[string]any{
					"apiVersion": "test.projectcontour.io/v1",
					"kind":       "Foo",
				},
			},
		},
	}

	for _, c := range cases {
		assert.Equal(t, c.Kind, KindOf(c.Obj))
	}
}

func TestVersionOf(t *testing.T) {
	cases := []struct {
		Version string
		Obj     any
	}{
		{"v1", &core_v1.Secret{}},
		{"v1", &core_v1.Service{}},
		{"v1", &core_v1.Endpoints{}},
		{"networking.k8s.io/v1", &networking_v1.Ingress{}},
		{"projectcontour.io/v1", &contour_v1.HTTPProxy{}},
		{"projectcontour.io/v1", &contour_v1.TLSCertificateDelegation{}},
		{"projectcontour.io/v1alpha1", &contour_v1alpha1.ExtensionService{}},
		{
			"test.projectcontour.io/v1", &unstructured.Unstructured{
				Object: map[string]any{
					"apiVersion": "test.projectcontour.io/v1",
					"kind":       "Foo",
				},
			},
		},
	}

	for _, c := range cases {
		assert.Equal(t, c.Version, VersionOf(c.Obj))
	}
}
