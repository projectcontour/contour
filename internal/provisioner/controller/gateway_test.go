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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubernetesscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func TestGatewayReconcile(t *testing.T) {
	const controller = "projectcontour.io/gateway-provisioner"

	reconcilableGatewayClass := func(name, controller string) *gatewayv1alpha2.GatewayClass {
		return &gatewayv1alpha2.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Spec: gatewayv1alpha2.GatewayClassSpec{
				ControllerName: gatewayv1alpha2.GatewayController(controller),
			},
			// the fake client lets us create resources with a status set
			Status: gatewayv1alpha2.GatewayClassStatus{
				Conditions: []metav1.Condition{
					{
						Type:   string(gatewayv1alpha2.GatewayClassConditionStatusAccepted),
						Status: metav1.ConditionTrue,
						Reason: string(gatewayv1alpha2.GatewayClassReasonAccepted),
					},
				},
			},
		}
	}

	tests := map[string]struct {
		gatewayClass *gatewayv1alpha2.GatewayClass
		gateway      *gatewayv1alpha2.Gateway
		req          *reconcile.Request
		assertions   func(t *testing.T, r *gatewayReconciler, gw *gatewayv1alpha2.Gateway, reconcileErr error)
	}{
		"A gateway for a reconcilable gatewayclass is reconciled": {
			gatewayClass: reconcilableGatewayClass("gatewayclass-1", controller),
			gateway: &gatewayv1alpha2.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "gateway-1",
					Name:      "gateway-1",
				},
				Spec: gatewayv1alpha2.GatewaySpec{
					GatewayClassName: gatewayv1alpha2.ObjectName("gatewayclass-1"),
				},
			},
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayv1alpha2.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)

				// Verify the Gateway has a "Scheduled: true" condition
				require.NoError(t, r.client.Get(context.Background(), keyFor(gw), gw))
				require.Len(t, gw.Status.Conditions, 1)
				assert.Equal(t, string(gatewayv1alpha2.GatewayConditionScheduled), gw.Status.Conditions[0].Type)
				assert.Equal(t, metav1.ConditionTrue, gw.Status.Conditions[0].Status)

				// Verify the Contour deployment has been created
				deploy := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "gateway-1",
						Name:      "contour-gateway-1",
					},
				}
				require.NoError(t, r.client.Get(context.Background(), keyFor(deploy), deploy))
			},
		},
		"A gateway for a non-reconcilable gatewayclass (not accepted) is not reconciled": {
			gatewayClass: &gatewayv1alpha2.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gatewayclass-1",
				},
				Spec: gatewayv1alpha2.GatewayClassSpec{
					ControllerName: gatewayv1alpha2.GatewayController(controller),
				},
				Status: gatewayv1alpha2.GatewayClassStatus{
					Conditions: []metav1.Condition{
						{
							Type:   string(gatewayv1alpha2.GatewayClassConditionStatusAccepted),
							Status: metav1.ConditionFalse,
							Reason: string(gatewayv1alpha2.GatewayClassReasonInvalidParameters),
						},
					},
				},
			},
			gateway: &gatewayv1alpha2.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "gateway-1",
					Name:      "gateway-1",
				},
				Spec: gatewayv1alpha2.GatewaySpec{
					GatewayClassName: gatewayv1alpha2.ObjectName("gatewayclass-1"),
				},
			},
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayv1alpha2.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)

				// Verify that the Gateway has not had a "Scheduled: true" condition set
				require.NoError(t, r.client.Get(context.Background(), keyFor(gw), gw))
				require.Empty(t, gw.Status.Conditions, 0)

				// Verify the Contour deployment has not been created
				deploy := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "gateway-1",
						Name:      "contour-gateway-1",
					},
				}
				err := r.client.Get(context.Background(), keyFor(deploy), deploy)
				assert.True(t, errors.IsNotFound(err))
			},
		},
		"A gateway for a non-reconcilable gatewayclass (non-matching controller) is not reconciled": {
			gatewayClass: &gatewayv1alpha2.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gatewayclass-1",
				},
				Spec: gatewayv1alpha2.GatewayClassSpec{
					ControllerName: "someothercontroller.io/controller",
				},
				Status: gatewayv1alpha2.GatewayClassStatus{
					Conditions: []metav1.Condition{
						{
							Type:   string(gatewayv1alpha2.GatewayClassConditionStatusAccepted),
							Status: metav1.ConditionTrue,
							Reason: string(gatewayv1alpha2.GatewayClassReasonAccepted),
						},
					},
				},
			},
			gateway: &gatewayv1alpha2.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "gateway-1",
					Name:      "gateway-1",
				},
				Spec: gatewayv1alpha2.GatewaySpec{
					GatewayClassName: gatewayv1alpha2.ObjectName("gatewayclass-1"),
				},
			},
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayv1alpha2.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)

				// Verify that the Gateway has not had a "Scheduled: true" condition set
				require.NoError(t, r.client.Get(context.Background(), keyFor(gw), gw))
				require.Empty(t, gw.Status.Conditions, 0)

				// Verify the Contour deployment has not been created
				deploy := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "gateway-1",
						Name:      "contour-gateway-1",
					},
				}
				err := r.client.Get(context.Background(), keyFor(deploy), deploy)
				assert.True(t, errors.IsNotFound(err))
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			require.NoError(t, kubernetesscheme.AddToScheme(scheme))
			require.NoError(t, gatewayv1alpha2.AddToScheme(scheme))
			require.NoError(t, contourv1alpha1.AddToScheme(scheme))

			client := fake.NewClientBuilder().WithScheme(scheme)
			if tc.gatewayClass != nil {
				client.WithObjects(tc.gatewayClass)
			}
			if tc.gateway != nil {
				client.WithObjects(tc.gateway)
			}

			r := &gatewayReconciler{
				gatewayController: controller,
				client:            client.Build(),
				log:               logr.Discard(),
			}

			var req reconcile.Request
			if tc.req != nil {
				req = *tc.req
			} else {
				req = reconcile.Request{
					NamespacedName: keyFor(tc.gateway),
				}
			}

			_, err := r.Reconcile(context.Background(), req)

			tc.assertions(t, r, tc.gateway, err)
		})
	}
}
