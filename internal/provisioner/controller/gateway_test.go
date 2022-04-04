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

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func TestIsGatewayClassReconcilable(t *testing.T) {
	tests := map[string]struct {
		obj               client.Object
		gatewayController string
		want              bool
	}{
		"GatewayClass is reconcilable": {
			obj: &gatewayv1alpha2.GatewayClass{
				Spec: gatewayv1alpha2.GatewayClassSpec{
					ControllerName: gatewayv1alpha2.GatewayController("projectcontour.io/gateway-provisioner"),
				},
				Status: gatewayv1alpha2.GatewayClassStatus{
					Conditions: []metav1.Condition{
						{
							Type:   string(gatewayv1alpha2.GatewayClassConditionStatusAccepted),
							Status: metav1.ConditionTrue,
						},
					},
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
				Status: gatewayv1alpha2.GatewayClassStatus{
					Conditions: []metav1.Condition{
						{
							Type:   string(gatewayv1alpha2.GatewayClassConditionStatusAccepted),
							Status: metav1.ConditionTrue,
						},
					},
				},
			},
			gatewayController: "projectcontour.io/gateway-provisioner",
			want:              false,
		},
		"GatewayClass has a condition of Accepted: false": {
			obj: &gatewayv1alpha2.GatewayClass{
				Spec: gatewayv1alpha2.GatewayClassSpec{
					ControllerName: gatewayv1alpha2.GatewayController("projectcontour.io/gateway-provisioner"),
				},
				Status: gatewayv1alpha2.GatewayClassStatus{
					Conditions: []metav1.Condition{
						{
							Type:   string(gatewayv1alpha2.GatewayClassConditionStatusAccepted),
							Status: metav1.ConditionFalse,
						},
					},
				},
			},
			gatewayController: "projectcontour.io/gateway-provisioner",
			want:              false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			r := &gatewayReconciler{
				gatewayController: v1alpha2.GatewayController(tc.gatewayController),
			}

			assert.Equal(t, tc.want, r.isGatewayClassReconcilable(tc.obj))
		})
	}
}
