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
	"context"
	"testing"

	contourv1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/gatewayapi"
	"github.com/projectcontour/contour/internal/provisioner"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

func TestGatewayClassReconcile(t *testing.T) {
	tests := map[string]struct {
		gatewayClass  *gatewayv1beta1.GatewayClass
		params        *contourv1alpha1.ContourDeployment
		req           *reconcile.Request
		wantCondition *metav1.Condition
		assertions    func(t *testing.T, r *gatewayClassReconciler, gc *gatewayv1beta1.GatewayClass, reconcileErr error)
	}{
		"reconcile request for non-existent gatewayclass results in no error": {
			req: &reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "nonexistent"},
			},
			assertions: func(t *testing.T, r *gatewayClassReconciler, gc *gatewayv1beta1.GatewayClass, reconcileErr error) {
				assert.NoError(t, reconcileErr)

				gatewayClasses := &gatewayv1beta1.GatewayClassList{}
				require.NoError(t, r.client.List(context.Background(), gatewayClasses))
				assert.Empty(t, gatewayClasses.Items)
			},
		},
		"gatewayclass not controlled by us does not get conditions set": {
			gatewayClass: &gatewayv1beta1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gatewayclass-1",
				},
				Spec: gatewayv1beta1.GatewayClassSpec{
					ControllerName: gatewayv1beta1.GatewayController("someothercontroller.io/controller"),
				},
			},
			assertions: func(t *testing.T, r *gatewayClassReconciler, gc *gatewayv1beta1.GatewayClass, reconcileErr error) {
				assert.NoError(t, reconcileErr)

				res := &gatewayv1beta1.GatewayClass{}
				require.NoError(t, r.client.Get(context.Background(), keyFor(gc), res))

				assert.Empty(t, res.Status.Conditions)
			},
		},
		"gatewayclass controlled by us with no parameters gets Accepted: true condition": {
			gatewayClass: &gatewayv1beta1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gatewayclass-1",
				},
				Spec: gatewayv1beta1.GatewayClassSpec{
					ControllerName: "projectcontour.io/gateway-controller",
				},
			},
			wantCondition: &metav1.Condition{
				Type:   string(gatewayv1beta1.GatewayClassConditionStatusAccepted),
				Status: metav1.ConditionTrue,
				Reason: string(gatewayv1beta1.GatewayClassReasonAccepted),
			},
		},
		"gatewayclass controlled by us with an invalid parametersRef (target does not exist) gets Accepted: false condition": {
			gatewayClass: &gatewayv1beta1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gatewayclass-1",
				},
				Spec: gatewayv1beta1.GatewayClassSpec{
					ControllerName: "projectcontour.io/gateway-controller",
					ParametersRef: &gatewayv1beta1.ParametersReference{
						Group:     "projectcontour.io",
						Kind:      "ContourDeployment",
						Name:      "gatewayclass-params",
						Namespace: gatewayapi.NamespacePtr("projectcontour"),
					},
				},
			},
			wantCondition: &metav1.Condition{
				Type:   string(gatewayv1beta1.GatewayClassConditionStatusAccepted),
				Status: metav1.ConditionFalse,
				Reason: string(gatewayv1beta1.GatewayClassReasonInvalidParameters),
			},
		},
		"gatewayclass controlled by us with an invalid parametersRef (invalid group) gets Accepted: false condition": {
			gatewayClass: &gatewayv1beta1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gatewayclass-1",
				},
				Spec: gatewayv1beta1.GatewayClassSpec{
					ControllerName: "projectcontour.io/gateway-controller",
					ParametersRef: &gatewayv1beta1.ParametersReference{
						Group:     "invalidgroup.io",
						Kind:      "ContourDeployment",
						Name:      "gatewayclass-params",
						Namespace: gatewayapi.NamespacePtr("projectcontour"),
					},
				},
			},
			params: &contourv1alpha1.ContourDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "gatewayclass-params",
				},
			},
			wantCondition: &metav1.Condition{
				Type:   string(gatewayv1beta1.GatewayClassConditionStatusAccepted),
				Status: metav1.ConditionFalse,
				Reason: string(gatewayv1beta1.GatewayClassReasonInvalidParameters),
			},
		},
		"gatewayclass controlled by us with an invalid parametersRef (invalid kind) gets Accepted: false condition": {
			gatewayClass: &gatewayv1beta1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gatewayclass-1",
				},
				Spec: gatewayv1beta1.GatewayClassSpec{
					ControllerName: "projectcontour.io/gateway-controller",
					ParametersRef: &gatewayv1beta1.ParametersReference{
						Group:     "projectcontour.io",
						Kind:      "InvalidKind",
						Name:      "gatewayclass-params",
						Namespace: gatewayapi.NamespacePtr("projectcontour"),
					},
				},
			},
			params: &contourv1alpha1.ContourDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "gatewayclass-params",
				},
			},
			wantCondition: &metav1.Condition{
				Type:   string(gatewayv1beta1.GatewayClassConditionStatusAccepted),
				Status: metav1.ConditionFalse,
				Reason: string(gatewayv1beta1.GatewayClassReasonInvalidParameters),
			},
		},
		"gatewayclass controlled by us with an invalid parametersRef (invalid name) gets Accepted: false condition": {
			gatewayClass: &gatewayv1beta1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gatewayclass-1",
				},
				Spec: gatewayv1beta1.GatewayClassSpec{
					ControllerName: "projectcontour.io/gateway-controller",
					ParametersRef: &gatewayv1beta1.ParametersReference{
						Group:     "projectcontour.io",
						Kind:      "ContourDeployment",
						Name:      "invalid-name",
						Namespace: gatewayapi.NamespacePtr("projectcontour"),
					},
				},
			},
			params: &contourv1alpha1.ContourDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "gatewayclass-params",
				},
			},
			wantCondition: &metav1.Condition{
				Type:   string(gatewayv1beta1.GatewayClassConditionStatusAccepted),
				Status: metav1.ConditionFalse,
				Reason: string(gatewayv1beta1.GatewayClassReasonInvalidParameters),
			},
		},
		"gatewayclass controlled by us with an invalid parametersRef (invalid namespace) gets Accepted: false condition": {
			gatewayClass: &gatewayv1beta1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gatewayclass-1",
				},
				Spec: gatewayv1beta1.GatewayClassSpec{
					ControllerName: "projectcontour.io/gateway-controller",
					ParametersRef: &gatewayv1beta1.ParametersReference{
						Group:     "projectcontour.io",
						Kind:      "ContourDeployment",
						Name:      "gatewayclass-params",
						Namespace: gatewayapi.NamespacePtr("invalid-namespace"),
					},
				},
			},
			params: &contourv1alpha1.ContourDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "gatewayclass-params",
				},
			},
			wantCondition: &metav1.Condition{
				Type:   string(gatewayv1beta1.GatewayClassConditionStatusAccepted),
				Status: metav1.ConditionFalse,
				Reason: string(gatewayv1beta1.GatewayClassReasonInvalidParameters),
			},
		},
		"gatewayclass controlled by us with a valid parametersRef gets Accepted: true condition": {
			gatewayClass: &gatewayv1beta1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gatewayclass-1",
				},
				Spec: gatewayv1beta1.GatewayClassSpec{
					ControllerName: "projectcontour.io/gateway-controller",
					ParametersRef: &gatewayv1beta1.ParametersReference{
						Group:     "projectcontour.io",
						Kind:      "ContourDeployment",
						Name:      "gatewayclass-params",
						Namespace: gatewayapi.NamespacePtr("projectcontour"),
					},
				},
			},
			params: &contourv1alpha1.ContourDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "gatewayclass-params",
				},
			},
			wantCondition: &metav1.Condition{
				Type:   string(gatewayv1beta1.GatewayClassConditionStatusAccepted),
				Status: metav1.ConditionTrue,
				Reason: string(gatewayv1beta1.GatewayClassReasonAccepted),
			},
		},
		"gatewayclass controlled by us with a valid parametersRef but invalid parameter values for NetworkPublishing gets Accepted: false condition": {
			gatewayClass: &gatewayv1beta1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gatewayclass-1",
				},
				Spec: gatewayv1beta1.GatewayClassSpec{
					ControllerName: "projectcontour.io/gateway-controller",
					ParametersRef: &gatewayv1beta1.ParametersReference{
						Group:     "projectcontour.io",
						Kind:      "ContourDeployment",
						Name:      "gatewayclass-params",
						Namespace: gatewayapi.NamespacePtr("projectcontour"),
					},
				},
			},
			params: &contourv1alpha1.ContourDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "gatewayclass-params",
				},
				Spec: contourv1alpha1.ContourDeploymentSpec{
					Envoy: &contourv1alpha1.EnvoySettings{
						WorkloadType: "invalid-workload-type",
						NetworkPublishing: &contourv1alpha1.NetworkPublishing{
							Type: "invalid-networkpublishing-type",
						},
					},
				},
			},
			wantCondition: &metav1.Condition{
				Type:   string(gatewayv1beta1.GatewayClassConditionStatusAccepted),
				Status: metav1.ConditionFalse,
				Reason: string(gatewayv1beta1.GatewayClassReasonInvalidParameters),
			},
		},
		"gatewayclass controlled by us with a valid parametersRef but invalid parameter values for ExtraVolumeMounts gets Accepted: false condition": {
			gatewayClass: &gatewayv1beta1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gatewayclass-1",
				},
				Spec: gatewayv1beta1.GatewayClassSpec{
					ControllerName: "projectcontour.io/gateway-controller",
					ParametersRef: &gatewayv1beta1.ParametersReference{
						Group:     "projectcontour.io",
						Kind:      "ContourDeployment",
						Name:      "gatewayclass-params",
						Namespace: gatewayapi.NamespacePtr("projectcontour"),
					},
				},
			},
			params: &contourv1alpha1.ContourDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "gatewayclass-params",
				},
				Spec: contourv1alpha1.ContourDeploymentSpec{
					Envoy: &contourv1alpha1.EnvoySettings{
						ExtraVolumeMounts: []corev1.VolumeMount{
							{
								Name: "volume-a",
							},
						},
						ExtraVolumes: []corev1.Volume{
							{
								Name: "volume-b",
							},
						},
					},
				},
			},
			wantCondition: &metav1.Condition{
				Type:   string(gatewayv1beta1.GatewayClassConditionStatusAccepted),
				Status: metav1.ConditionFalse,
				Reason: string(gatewayv1beta1.GatewayClassReasonInvalidParameters),
			},
		},
		"gatewayclass controlled by us with a valid parametersRef but invalid parameter values for ExternalTrafficPolicy gets Accepted: false condition": {
			gatewayClass: &gatewayv1beta1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gatewayclass-1",
				},
				Spec: gatewayv1beta1.GatewayClassSpec{
					ControllerName: "projectcontour.io/gateway-controller",
					ParametersRef: &gatewayv1beta1.ParametersReference{
						Group:     "projectcontour.io",
						Kind:      "ContourDeployment",
						Name:      "gatewayclass-params",
						Namespace: gatewayapi.NamespacePtr("projectcontour"),
					},
				},
			},
			params: &contourv1alpha1.ContourDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "gatewayclass-params",
				},
				Spec: contourv1alpha1.ContourDeploymentSpec{
					Envoy: &contourv1alpha1.EnvoySettings{
						NetworkPublishing: &contourv1alpha1.NetworkPublishing{
							ExternalTrafficPolicy: "invalid-external-traffic-policy",
						},
					},
				},
			},
			wantCondition: &metav1.Condition{
				Type:   string(gatewayv1beta1.GatewayClassConditionStatusAccepted),
				Status: metav1.ConditionFalse,
				Reason: string(gatewayv1beta1.GatewayClassReasonInvalidParameters),
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			scheme, err := provisioner.CreateScheme()
			require.NoError(t, err)

			client := fake.NewClientBuilder().WithScheme(scheme)
			if tc.gatewayClass != nil {
				client.WithObjects(tc.gatewayClass)
			}
			if tc.params != nil {
				client.WithObjects(tc.params)
			}

			r := &gatewayClassReconciler{
				gatewayController: "projectcontour.io/gateway-controller",
				client:            client.Build(),
				log:               logr.Discard(),
			}

			var req reconcile.Request
			if tc.req != nil {
				req = *tc.req
			} else {
				req = reconcile.Request{
					NamespacedName: keyFor(tc.gatewayClass),
				}
			}

			_, err = r.Reconcile(context.Background(), req)

			if tc.wantCondition != nil {
				res := &gatewayv1beta1.GatewayClass{}
				require.NoError(t, r.client.Get(context.Background(), keyFor(tc.gatewayClass), res))

				require.Len(t, res.Status.Conditions, 1)
				assert.Equal(t, tc.wantCondition.Type, res.Status.Conditions[0].Type)
				assert.Equal(t, tc.wantCondition.Status, res.Status.Conditions[0].Status)
				assert.Equal(t, tc.wantCondition.Reason, res.Status.Conditions[0].Reason)
			}

			if tc.assertions != nil {
				tc.assertions(t, r, tc.gatewayClass, err)
			}
		})
	}
}

func keyFor(obj client.Object) types.NamespacedName {
	return client.ObjectKeyFromObject(obj)
}
