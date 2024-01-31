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
	"sort"
	"testing"

	"github.com/bombsimon/logrusr/v4"
	contourv1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/fixture"
	"github.com/projectcontour/contour/internal/provisioner"
	"github.com/projectcontour/contour/internal/ref"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apiextensions_v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayapi_v1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

func TestGatewayClassReconcile(t *testing.T) {
	tests := map[string]struct {
		gatewayClass    *gatewayv1beta1.GatewayClass
		gatewayClassCRD *apiextensions_v1.CustomResourceDefinition
		params          *contourv1alpha1.ContourDeployment
		req             *reconcile.Request
		wantConditions  []*metav1.Condition
		assertions      func(t *testing.T, r *gatewayClassReconciler, gc *gatewayv1beta1.GatewayClass, reconcileErr error)
	}{
		"reconcile request for non-existent gatewayclass results in no error": {
			req: &reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "nonexistent"},
			},
			assertions: func(t *testing.T, r *gatewayClassReconciler, gc *gatewayv1beta1.GatewayClass, reconcileErr error) {
				require.NoError(t, reconcileErr)

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
				require.NoError(t, reconcileErr)

				res := &gatewayv1beta1.GatewayClass{}
				require.NoError(t, r.client.Get(context.Background(), keyFor(gc), res))

				assert.Empty(t, res.Status.Conditions)
			},
		},
		"gatewayclass controlled by us with no parameters gets Accepted: true condition and SupportedVersion: true": {
			gatewayClass: &gatewayv1beta1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gatewayclass-1",
				},
				Spec: gatewayv1beta1.GatewayClassSpec{
					ControllerName: "projectcontour.io/gateway-controller",
				},
			},
			wantConditions: []*metav1.Condition{
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusAccepted),
					Status: metav1.ConditionTrue,
					Reason: string(gatewayapi_v1.GatewayClassReasonAccepted),
				},
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusSupportedVersion),
					Status: metav1.ConditionTrue,
					Reason: string(gatewayapi_v1.GatewayClassReasonSupportedVersion),
				},
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
						Namespace: ref.To(gatewayv1beta1.Namespace("projectcontour")),
					},
				},
			},
			wantConditions: []*metav1.Condition{
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusAccepted),
					Status: metav1.ConditionFalse,
					Reason: string(gatewayapi_v1.GatewayClassReasonInvalidParameters),
				},
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusSupportedVersion),
					Status: metav1.ConditionTrue,
					Reason: string(gatewayapi_v1.GatewayClassReasonSupportedVersion),
				},
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
						Namespace: ref.To(gatewayv1beta1.Namespace("projectcontour")),
					},
				},
			},
			params: &contourv1alpha1.ContourDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "gatewayclass-params",
				},
			},
			wantConditions: []*metav1.Condition{
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusAccepted),
					Status: metav1.ConditionFalse,
					Reason: string(gatewayapi_v1.GatewayClassReasonInvalidParameters),
				},
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusSupportedVersion),
					Status: metav1.ConditionTrue,
					Reason: string(gatewayapi_v1.GatewayClassReasonSupportedVersion),
				},
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
						Namespace: ref.To(gatewayv1beta1.Namespace("projectcontour")),
					},
				},
			},
			params: &contourv1alpha1.ContourDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "gatewayclass-params",
				},
			},
			wantConditions: []*metav1.Condition{
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusAccepted),
					Status: metav1.ConditionFalse,
					Reason: string(gatewayapi_v1.GatewayClassReasonInvalidParameters),
				},
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusSupportedVersion),
					Status: metav1.ConditionTrue,
					Reason: string(gatewayapi_v1.GatewayClassReasonSupportedVersion),
				},
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
						Namespace: ref.To(gatewayv1beta1.Namespace("projectcontour")),
					},
				},
			},
			params: &contourv1alpha1.ContourDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "gatewayclass-params",
				},
			},
			wantConditions: []*metav1.Condition{
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusAccepted),
					Status: metav1.ConditionFalse,
					Reason: string(gatewayapi_v1.GatewayClassReasonInvalidParameters),
				},
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusSupportedVersion),
					Status: metav1.ConditionTrue,
					Reason: string(gatewayapi_v1.GatewayClassReasonSupportedVersion),
				},
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
						Namespace: ref.To(gatewayv1beta1.Namespace("invalid-namespace")),
					},
				},
			},
			params: &contourv1alpha1.ContourDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "gatewayclass-params",
				},
			},
			wantConditions: []*metav1.Condition{
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusAccepted),
					Status: metav1.ConditionFalse,
					Reason: string(gatewayapi_v1.GatewayClassReasonInvalidParameters),
				},
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusSupportedVersion),
					Status: metav1.ConditionTrue,
					Reason: string(gatewayapi_v1.GatewayClassReasonSupportedVersion),
				},
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
						Namespace: ref.To(gatewayv1beta1.Namespace("projectcontour")),
					},
				},
			},
			params: &contourv1alpha1.ContourDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "gatewayclass-params",
				},
			},
			wantConditions: []*metav1.Condition{
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusAccepted),
					Status: metav1.ConditionTrue,
					Reason: string(gatewayapi_v1.GatewayClassReasonAccepted),
				},
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusSupportedVersion),
					Status: metav1.ConditionTrue,
					Reason: string(gatewayapi_v1.GatewayClassReasonSupportedVersion),
				},
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
						Namespace: ref.To(gatewayv1beta1.Namespace("projectcontour")),
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
			wantConditions: []*metav1.Condition{
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusAccepted),
					Status: metav1.ConditionFalse,
					Reason: string(gatewayapi_v1.GatewayClassReasonInvalidParameters),
				},
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusSupportedVersion),
					Status: metav1.ConditionTrue,
					Reason: string(gatewayapi_v1.GatewayClassReasonSupportedVersion),
				},
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
						Namespace: ref.To(gatewayv1beta1.Namespace("projectcontour")),
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
			wantConditions: []*metav1.Condition{
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusAccepted),
					Status: metav1.ConditionFalse,
					Reason: string(gatewayapi_v1.GatewayClassReasonInvalidParameters),
				},
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusSupportedVersion),
					Status: metav1.ConditionTrue,
					Reason: string(gatewayapi_v1.GatewayClassReasonSupportedVersion),
				},
			},
		},
		"gatewayclass controlled by us with a valid parametersRef but invalid parameter values for LogLevel gets Accepted: false condition": {
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
						Namespace: ref.To(gatewayv1beta1.Namespace("projectcontour")),
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
						LogLevel: "invalidLevel",
					},
				},
			},
			wantConditions: []*metav1.Condition{
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusAccepted),
					Status: metav1.ConditionFalse,
					Reason: string(gatewayapi_v1.GatewayClassReasonInvalidParameters),
				},
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusSupportedVersion),
					Status: metav1.ConditionTrue,
					Reason: string(gatewayapi_v1.GatewayClassReasonSupportedVersion),
				},
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
						Namespace: ref.To(gatewayv1beta1.Namespace("projectcontour")),
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
			wantConditions: []*metav1.Condition{
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusAccepted),
					Status: metav1.ConditionFalse,
					Reason: string(gatewayapi_v1.GatewayClassReasonInvalidParameters),
				},
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusSupportedVersion),
					Status: metav1.ConditionTrue,
					Reason: string(gatewayapi_v1.GatewayClassReasonSupportedVersion),
				},
			},
		},
		"gatewayclass controlled by us with a valid parametersRef but invalid parameter values for IPFamilyPolicy gets Accepted: false condition": {
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
						Namespace: ref.To(gatewayv1beta1.Namespace("projectcontour")),
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
							IPFamilyPolicy: "invalid-external-traffic-policy",
						},
					},
				},
			},
			wantConditions: []*metav1.Condition{
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusAccepted),
					Status: metav1.ConditionFalse,
					Reason: string(gatewayapi_v1.GatewayClassReasonInvalidParameters),
				},
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusSupportedVersion),
					Status: metav1.ConditionTrue,
					Reason: string(gatewayapi_v1.GatewayClassReasonSupportedVersion),
				},
			},
		},
		"gatewayclass controlled by us with gatewayclass CRD with unsupported version sets Accepted: true, SupportedVersion: False": {
			gatewayClass: &gatewayv1beta1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gatewayclass-1",
				},
				Spec: gatewayv1beta1.GatewayClassSpec{
					ControllerName: "projectcontour.io/gateway-controller",
				},
			},
			gatewayClassCRD: &apiextensions_v1.CustomResourceDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gatewayclasses.gateway.networking.k8s.io",
					Annotations: map[string]string{
						"gateway.networking.k8s.io/bundle-version": "v9.9.9",
					},
				},
			},
			wantConditions: []*metav1.Condition{
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusAccepted),
					Status: metav1.ConditionTrue,
					Reason: string(gatewayapi_v1.GatewayClassReasonAccepted),
				},
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusSupportedVersion),
					Status: metav1.ConditionFalse,
					Reason: string(gatewayapi_v1.GatewayClassReasonUnsupportedVersion),
				},
			},
			assertions: func(t *testing.T, r *gatewayClassReconciler, gc *gatewayv1beta1.GatewayClass, reconcileErr error) {
				require.NoError(t, reconcileErr)
			},
		},
		"gatewayclass controlled by us with gatewayclass CRD fetch failed sets SupportedVersion: false": {
			gatewayClass: &gatewayv1beta1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gatewayclass-1",
				},
				Spec: gatewayv1beta1.GatewayClassSpec{
					ControllerName: "projectcontour.io/gateway-controller",
				},
			},
			gatewayClassCRD: &apiextensions_v1.CustomResourceDefinition{
				ObjectMeta: metav1.ObjectMeta{
					// Use the wrong name so we fail to fetch the CRD,
					// contrived way to cause this scenario.
					Name: "gatewayclasses-wrong.gateway.networking.k8s.io",
					Annotations: map[string]string{
						"gateway.networking.k8s.io/bundle-version": "v1.0.0",
					},
				},
			},
			wantConditions: []*metav1.Condition{
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusAccepted),
					Status: metav1.ConditionTrue,
					Reason: string(gatewayapi_v1.GatewayClassReasonAccepted),
				},
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusSupportedVersion),
					Status: metav1.ConditionFalse,
					Reason: string(gatewayapi_v1.GatewayClassReasonUnsupportedVersion),
				},
			},
			assertions: func(t *testing.T, r *gatewayClassReconciler, gc *gatewayv1beta1.GatewayClass, reconcileErr error) {
				require.NoError(t, reconcileErr)
			},
		},
		"gatewayclass controlled by us with gatewayclass CRD without version annotation sets SupportedVersion: false": {
			gatewayClass: &gatewayv1beta1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gatewayclass-1",
				},
				Spec: gatewayv1beta1.GatewayClassSpec{
					ControllerName: "projectcontour.io/gateway-controller",
				},
			},
			gatewayClassCRD: &apiextensions_v1.CustomResourceDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gatewayclasses.gateway.networking.k8s.io",
					Annotations: map[string]string{
						"gateway.networking.k8s.io/bundle-version-wrong": "v1.0.0",
					},
				},
			},
			wantConditions: []*metav1.Condition{
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusAccepted),
					Status: metav1.ConditionTrue,
					Reason: string(gatewayapi_v1.GatewayClassReasonAccepted),
				},
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusSupportedVersion),
					Status: metav1.ConditionFalse,
					Reason: string(gatewayapi_v1.GatewayClassReasonUnsupportedVersion),
				},
			},
			assertions: func(t *testing.T, r *gatewayClassReconciler, gc *gatewayv1beta1.GatewayClass, reconcileErr error) {
				require.NoError(t, reconcileErr)
			},
		},
		"gatewayclass with status from previous generation is updated, only conditions we own are changed": {
			gatewayClass: &gatewayv1beta1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "gatewayclass-1",
					Generation: 2,
				},
				Spec: gatewayv1beta1.GatewayClassSpec{
					ControllerName: "projectcontour.io/gateway-controller",
				},
				Status: gatewayv1beta1.GatewayClassStatus{
					Conditions: []metav1.Condition{
						{
							Type:               string(gatewayapi_v1.GatewayClassConditionStatusAccepted),
							Status:             metav1.ConditionTrue,
							Reason:             string(gatewayapi_v1.GatewayClassReasonAccepted),
							ObservedGeneration: 1,
						},
						{
							Type:               "SomeOtherCondition",
							Status:             metav1.ConditionTrue,
							Reason:             "FooReason",
							ObservedGeneration: 1,
						},
					},
				},
			},
			wantConditions: []*metav1.Condition{
				{
					Type:               string(gatewayapi_v1.GatewayClassConditionStatusAccepted),
					Status:             metav1.ConditionTrue,
					Reason:             string(gatewayapi_v1.GatewayClassReasonAccepted),
					ObservedGeneration: 2,
				},
				{
					Type:               string(gatewayapi_v1.GatewayClassConditionStatusSupportedVersion),
					Status:             metav1.ConditionTrue,
					Reason:             string(gatewayapi_v1.GatewayClassReasonSupportedVersion),
					ObservedGeneration: 2,
				},
				{
					Type:               "SomeOtherCondition",
					Status:             metav1.ConditionTrue,
					Reason:             "FooReason",
					ObservedGeneration: 1,
				},
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
				client.WithStatusSubresource(tc.gatewayClass)
			}

			if tc.gatewayClassCRD != nil {
				client.WithObjects(tc.gatewayClassCRD)
			} else {
				client.WithObjects(&apiextensions_v1.CustomResourceDefinition{
					ObjectMeta: metav1.ObjectMeta{
						Name: "gatewayclasses.gateway.networking.k8s.io",
						Annotations: map[string]string{
							"gateway.networking.k8s.io/bundle-version": "v1.0.0",
						},
					},
				})
			}

			if tc.params != nil {
				client.WithObjects(tc.params)
			}

			log.SetLogger(logrusr.New(fixture.NewTestLogger(t)))
			r := &gatewayClassReconciler{
				gatewayController: "projectcontour.io/gateway-controller",
				client:            client.Build(),
				log:               ctrl.Log.WithName("gatewayclass-controller-test"),
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

			if len(tc.wantConditions) > 0 {
				res := &gatewayv1beta1.GatewayClass{}
				require.NoError(t, r.client.Get(context.Background(), keyFor(tc.gatewayClass), res))

				require.Len(t, res.Status.Conditions, len(tc.wantConditions))

				sort.Slice(tc.wantConditions, func(i, j int) bool {
					return tc.wantConditions[i].Type < tc.wantConditions[j].Type
				})
				sort.Slice(res.Status.Conditions, func(i, j int) bool {
					return res.Status.Conditions[i].Type < res.Status.Conditions[j].Type
				})

				for i := range tc.wantConditions {
					assert.Equal(t, tc.wantConditions[i].Type, res.Status.Conditions[i].Type)
					assert.Equal(t, tc.wantConditions[i].Status, res.Status.Conditions[i].Status)
					assert.Equal(t, tc.wantConditions[i].Reason, res.Status.Conditions[i].Reason)
					assert.Equal(t, tc.wantConditions[i].ObservedGeneration, res.Status.Conditions[i].ObservedGeneration)
				}
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
