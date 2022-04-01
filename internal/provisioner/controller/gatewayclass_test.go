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

package controller

import (
	"testing"

	"github.com/projectcontour/contour/internal/gatewayapi"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func TestHasMatchingController(t *testing.T) {
	tests := map[string]struct {
		obj               client.Object
		gatewayController string
		want              bool
	}{
		"GatewayClass has matching controller": {
			obj: &gatewayv1alpha2.GatewayClass{
				Spec: gatewayv1alpha2.GatewayClassSpec{
					ControllerName: gatewayv1alpha2.GatewayController("projectcontour.io/gateway-provisioner"),
				},
			},
			gatewayController: "projectcontour.io/gateway-provisioner",
			want:              true,
		},
		"object is not a GatewayClass": {
			obj:  &corev1.Service{},
			want: false,
		},
		"GatewayClass has a non-matching controller": {
			obj: &gatewayv1alpha2.GatewayClass{
				Spec: gatewayv1alpha2.GatewayClassSpec{
					ControllerName: gatewayv1alpha2.GatewayController("projectcontour.io/some-other-controller"),
				},
			},
			gatewayController: "projectcontour.io/gateway-provisioner",
			want:              false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			r := &gatewayClassReconciler{
				gatewayController: v1alpha2.GatewayController(tc.gatewayController),
			}

			assert.Equal(t, tc.want, r.hasMatchingController(tc.obj))
		})
	}
}

func TestIsContourDeploymentRef(t *testing.T) {
	tests := map[string]struct {
		ref  *gatewayv1alpha2.ParametersReference
		want bool
	}{
		"valid ref": {
			ref: &gatewayv1alpha2.ParametersReference{
				Group:     "projectcontour.io",
				Kind:      "ContourDeployment",
				Namespace: gatewayapi.NamespacePtr("namespace-1"),
			},
			want: true,
		},
		"nil ref": {
			want: false,
		},
		"group is not projectcontour.io": {
			ref: &gatewayv1alpha2.ParametersReference{
				Group:     "some-other-group.io",
				Kind:      "ContourDeployment",
				Namespace: gatewayapi.NamespacePtr("namespace-1"),
			},
			want: false,
		},
		"kind is not ContourDeployment": {
			ref: &gatewayv1alpha2.ParametersReference{
				Group:     "projectcontour.io",
				Kind:      "SomeOtherKind",
				Namespace: gatewayapi.NamespacePtr("namespace-1"),
			},
			want: false,
		},
		"namespace is nil": {
			ref: &gatewayv1alpha2.ParametersReference{
				Group:     "projectcontour.io",
				Kind:      "ContourDeployment",
				Namespace: nil,
			},
			want: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, isContourDeploymentRef(tc.ref))
		})
	}
}
