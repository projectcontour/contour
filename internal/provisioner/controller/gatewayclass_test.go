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

	"github.com/go-logr/logr"
	contourv1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/gatewayapi"
	"github.com/projectcontour/contour/internal/provisioner"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func TestGatewayClassReconcile(t *testing.T) {
	tests := map[string]struct {
		gatewayClass *gatewayv1alpha2.GatewayClass
		params       *contourv1alpha1.ContourDeployment
		req          *reconcile.Request
		assertions   func(t *testing.T, r *gatewayClassReconciler, gc *gatewayv1alpha2.GatewayClass, reconcileErr error)
	}{
		"reconcile request for non-existent gatewayclass results in no error": {
			req: &reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "nonexistent"},
			},
			assertions: func(t *testing.T, r *gatewayClassReconciler, gc *gatewayv1alpha2.GatewayClass, reconcileErr error) {
				assert.NoError(t, reconcileErr)

				gatewayClasses := &gatewayv1alpha2.GatewayClassList{}
				require.NoError(t, r.client.List(context.Background(), gatewayClasses))
				assert.Empty(t, gatewayClasses.Items)
			},
		},
		"gatewayclass not controlled by us does not get conditions set": {
			gatewayClass: &gatewayv1alpha2.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gatewayclass-1",
				},
				Spec: gatewayv1alpha2.GatewayClassSpec{
					ControllerName: gatewayv1alpha2.GatewayController("someothercontroller.io/controller"),
				},
			},
			assertions: func(t *testing.T, r *gatewayClassReconciler, gc *gatewayv1alpha2.GatewayClass, reconcileErr error) {
				assert.NoError(t, reconcileErr)

				res := &gatewayv1alpha2.GatewayClass{}
				require.NoError(t, r.client.Get(context.Background(), keyFor(gc), res))

				assert.Empty(t, res.Status.Conditions)
			},
		},
		"gatewayclass controlled by us with no parameters gets Accepted: true condition": {
			gatewayClass: &gatewayv1alpha2.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gatewayclass-1",
				},
				Spec: gatewayv1alpha2.GatewayClassSpec{
					ControllerName: "projectcontour.io/gateway-provisioner",
				},
			},
			assertions: func(t *testing.T, r *gatewayClassReconciler, gc *gatewayv1alpha2.GatewayClass, reconcileErr error) {
				assert.NoError(t, reconcileErr)

				res := &gatewayv1alpha2.GatewayClass{}
				require.NoError(t, r.client.Get(context.Background(), keyFor(gc), res))

				assert.Len(t, res.Status.Conditions, 1)
				assert.Equal(t, string(gatewayv1alpha2.GatewayClassConditionStatusAccepted), res.Status.Conditions[0].Type)
				assert.Equal(t, metav1.ConditionTrue, res.Status.Conditions[0].Status)
				assert.Equal(t, string(gatewayv1alpha2.GatewayClassReasonAccepted), res.Status.Conditions[0].Reason)
			},
		},
		"gatewayclass controlled by us with an invalid parametersRef (target does not exist) gets Accepted: false condition": {
			gatewayClass: &gatewayv1alpha2.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gatewayclass-1",
				},
				Spec: gatewayv1alpha2.GatewayClassSpec{
					ControllerName: "projectcontour.io/gateway-provisioner",
					ParametersRef: &gatewayv1alpha2.ParametersReference{
						Group:     "projectcontour.io",
						Kind:      "ContourDeployment",
						Name:      "gatewayclass-params",
						Namespace: gatewayapi.NamespacePtr("projectcontour"),
					},
				},
			},
			assertions: func(t *testing.T, r *gatewayClassReconciler, gc *gatewayv1alpha2.GatewayClass, reconcileErr error) {
				assert.NoError(t, reconcileErr)

				res := &gatewayv1alpha2.GatewayClass{}
				require.NoError(t, r.client.Get(context.Background(), keyFor(gc), res))

				assert.Len(t, res.Status.Conditions, 1)
				assert.Equal(t, string(gatewayv1alpha2.GatewayClassConditionStatusAccepted), res.Status.Conditions[0].Type)
				assert.Equal(t, metav1.ConditionFalse, res.Status.Conditions[0].Status)
				assert.Equal(t, string(gatewayv1alpha2.GatewayClassReasonInvalidParameters), res.Status.Conditions[0].Reason)
			},
		},
		"gatewayclass controlled by us with an invalid parametersRef (invalid group) gets Accepted: false condition": {
			gatewayClass: &gatewayv1alpha2.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gatewayclass-1",
				},
				Spec: gatewayv1alpha2.GatewayClassSpec{
					ControllerName: "projectcontour.io/gateway-provisioner",
					ParametersRef: &gatewayv1alpha2.ParametersReference{
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
			assertions: func(t *testing.T, r *gatewayClassReconciler, gc *gatewayv1alpha2.GatewayClass, reconcileErr error) {
				assert.NoError(t, reconcileErr)

				res := &gatewayv1alpha2.GatewayClass{}
				require.NoError(t, r.client.Get(context.Background(), keyFor(gc), res))

				assert.Len(t, res.Status.Conditions, 1)
				assert.Equal(t, string(gatewayv1alpha2.GatewayClassConditionStatusAccepted), res.Status.Conditions[0].Type)
				assert.Equal(t, metav1.ConditionFalse, res.Status.Conditions[0].Status)
				assert.Equal(t, string(gatewayv1alpha2.GatewayClassReasonInvalidParameters), res.Status.Conditions[0].Reason)
			},
		},
		"gatewayclass controlled by us with an invalid parametersRef (invalid kind) gets Accepted: false condition": {
			gatewayClass: &gatewayv1alpha2.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gatewayclass-1",
				},
				Spec: gatewayv1alpha2.GatewayClassSpec{
					ControllerName: "projectcontour.io/gateway-provisioner",
					ParametersRef: &gatewayv1alpha2.ParametersReference{
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
			assertions: func(t *testing.T, r *gatewayClassReconciler, gc *gatewayv1alpha2.GatewayClass, reconcileErr error) {
				assert.NoError(t, reconcileErr)

				res := &gatewayv1alpha2.GatewayClass{}
				require.NoError(t, r.client.Get(context.Background(), keyFor(gc), res))

				assert.Len(t, res.Status.Conditions, 1)
				assert.Equal(t, string(gatewayv1alpha2.GatewayClassConditionStatusAccepted), res.Status.Conditions[0].Type)
				assert.Equal(t, metav1.ConditionFalse, res.Status.Conditions[0].Status)
				assert.Equal(t, string(gatewayv1alpha2.GatewayClassReasonInvalidParameters), res.Status.Conditions[0].Reason)
			},
		},
		"gatewayclass controlled by us with an invalid parametersRef (invalid name) gets Accepted: false condition": {
			gatewayClass: &gatewayv1alpha2.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gatewayclass-1",
				},
				Spec: gatewayv1alpha2.GatewayClassSpec{
					ControllerName: "projectcontour.io/gateway-provisioner",
					ParametersRef: &gatewayv1alpha2.ParametersReference{
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
			assertions: func(t *testing.T, r *gatewayClassReconciler, gc *gatewayv1alpha2.GatewayClass, reconcileErr error) {
				assert.NoError(t, reconcileErr)

				res := &gatewayv1alpha2.GatewayClass{}
				require.NoError(t, r.client.Get(context.Background(), keyFor(gc), res))

				assert.Len(t, res.Status.Conditions, 1)
				assert.Equal(t, string(gatewayv1alpha2.GatewayClassConditionStatusAccepted), res.Status.Conditions[0].Type)
				assert.Equal(t, metav1.ConditionFalse, res.Status.Conditions[0].Status)
				assert.Equal(t, string(gatewayv1alpha2.GatewayClassReasonInvalidParameters), res.Status.Conditions[0].Reason)
			},
		},
		"gatewayclass controlled by us with an invalid parametersRef (invalid namespace) gets Accepted: false condition": {
			gatewayClass: &gatewayv1alpha2.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gatewayclass-1",
				},
				Spec: gatewayv1alpha2.GatewayClassSpec{
					ControllerName: "projectcontour.io/gateway-provisioner",
					ParametersRef: &gatewayv1alpha2.ParametersReference{
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
			assertions: func(t *testing.T, r *gatewayClassReconciler, gc *gatewayv1alpha2.GatewayClass, reconcileErr error) {
				assert.NoError(t, reconcileErr)

				res := &gatewayv1alpha2.GatewayClass{}
				require.NoError(t, r.client.Get(context.Background(), keyFor(gc), res))

				assert.Len(t, res.Status.Conditions, 1)
				assert.Equal(t, string(gatewayv1alpha2.GatewayClassConditionStatusAccepted), res.Status.Conditions[0].Type)
				assert.Equal(t, metav1.ConditionFalse, res.Status.Conditions[0].Status)
				assert.Equal(t, string(gatewayv1alpha2.GatewayClassReasonInvalidParameters), res.Status.Conditions[0].Reason)
			},
		},
		"gatewayclass controlled by us with a valid parametersRef gets Accepted: true condition": {
			gatewayClass: &gatewayv1alpha2.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gatewayclass-1",
				},
				Spec: gatewayv1alpha2.GatewayClassSpec{
					ControllerName: "projectcontour.io/gateway-provisioner",
					ParametersRef: &gatewayv1alpha2.ParametersReference{
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
			assertions: func(t *testing.T, r *gatewayClassReconciler, gc *gatewayv1alpha2.GatewayClass, reconcileErr error) {
				assert.NoError(t, reconcileErr)

				res := &gatewayv1alpha2.GatewayClass{}
				require.NoError(t, r.client.Get(context.Background(), keyFor(gc), res))

				assert.Len(t, res.Status.Conditions, 1)
				assert.Equal(t, string(gatewayv1alpha2.GatewayClassConditionStatusAccepted), res.Status.Conditions[0].Type)
				assert.Equal(t, metav1.ConditionTrue, res.Status.Conditions[0].Status)
				assert.Equal(t, string(gatewayv1alpha2.GatewayClassReasonAccepted), res.Status.Conditions[0].Reason)
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
				gatewayController: "projectcontour.io/gateway-provisioner",
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

			tc.assertions(t, r, tc.gatewayClass, err)
		})
	}
}

func keyFor(obj client.Object) types.NamespacedName {
	return client.ObjectKeyFromObject(obj)
}
