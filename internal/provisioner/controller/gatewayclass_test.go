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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	core_v1 "k8s.io/api/core/v1"
	apiextensions_v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayapi_v1 "sigs.k8s.io/gateway-api/apis/v1"

	contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/fixture"
	"github.com/projectcontour/contour/internal/provisioner"
)

func TestGatewayClassReconcile(t *testing.T) {
	tests := map[string]struct {
		gatewayClass    *gatewayapi_v1.GatewayClass
		gatewayClassCRD *apiextensions_v1.CustomResourceDefinition
		params          *contour_v1alpha1.ContourDeployment
		req             *reconcile.Request
		wantConditions  []*meta_v1.Condition
		assertions      func(t *testing.T, r *gatewayClassReconciler, gc *gatewayapi_v1.GatewayClass, reconcileErr error)
	}{
		"reconcile request for non-existent gatewayclass results in no error": {
			req: &reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "nonexistent"},
			},
			assertions: func(t *testing.T, r *gatewayClassReconciler, _ *gatewayapi_v1.GatewayClass, reconcileErr error) {
				require.NoError(t, reconcileErr)

				gatewayClasses := &gatewayapi_v1.GatewayClassList{}
				require.NoError(t, r.client.List(context.Background(), gatewayClasses))
				assert.Empty(t, gatewayClasses.Items)
			},
		},
		"gatewayclass not controlled by us does not get conditions set": {
			gatewayClass: &gatewayapi_v1.GatewayClass{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: "gatewayclass-1",
				},
				Spec: gatewayapi_v1.GatewayClassSpec{
					ControllerName: gatewayapi_v1.GatewayController("someothercontroller.io/controller"),
				},
			},
			assertions: func(t *testing.T, r *gatewayClassReconciler, gc *gatewayapi_v1.GatewayClass, reconcileErr error) {
				require.NoError(t, reconcileErr)

				res := &gatewayapi_v1.GatewayClass{}
				require.NoError(t, r.client.Get(context.Background(), keyFor(gc), res))

				assert.Empty(t, res.Status.Conditions)
			},
		},
		"gatewayclass controlled by us with no parameters gets Accepted: true condition and SupportedVersion: true": {
			gatewayClass: &gatewayapi_v1.GatewayClass{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: "gatewayclass-1",
				},
				Spec: gatewayapi_v1.GatewayClassSpec{
					ControllerName: "projectcontour.io/gateway-controller",
				},
			},
			wantConditions: []*meta_v1.Condition{
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusAccepted),
					Status: meta_v1.ConditionTrue,
					Reason: string(gatewayapi_v1.GatewayClassReasonAccepted),
				},
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusSupportedVersion),
					Status: meta_v1.ConditionTrue,
					Reason: string(gatewayapi_v1.GatewayClassReasonSupportedVersion),
				},
			},
		},
		"gatewayclass controlled by us with an invalid parametersRef (target does not exist) gets Accepted: false condition": {
			gatewayClass: &gatewayapi_v1.GatewayClass{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: "gatewayclass-1",
				},
				Spec: gatewayapi_v1.GatewayClassSpec{
					ControllerName: "projectcontour.io/gateway-controller",
					ParametersRef: &gatewayapi_v1.ParametersReference{
						Group:     "projectcontour.io",
						Kind:      "ContourDeployment",
						Name:      "gatewayclass-params",
						Namespace: ptr.To(gatewayapi_v1.Namespace("projectcontour")),
					},
				},
			},
			wantConditions: []*meta_v1.Condition{
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusAccepted),
					Status: meta_v1.ConditionFalse,
					Reason: string(gatewayapi_v1.GatewayClassReasonInvalidParameters),
				},
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusSupportedVersion),
					Status: meta_v1.ConditionTrue,
					Reason: string(gatewayapi_v1.GatewayClassReasonSupportedVersion),
				},
			},
		},
		"gatewayclass controlled by us with an invalid parametersRef (invalid group) gets Accepted: false condition": {
			gatewayClass: &gatewayapi_v1.GatewayClass{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: "gatewayclass-1",
				},
				Spec: gatewayapi_v1.GatewayClassSpec{
					ControllerName: "projectcontour.io/gateway-controller",
					ParametersRef: &gatewayapi_v1.ParametersReference{
						Group:     "invalidgroup.io",
						Kind:      "ContourDeployment",
						Name:      "gatewayclass-params",
						Namespace: ptr.To(gatewayapi_v1.Namespace("projectcontour")),
					},
				},
			},
			params: &contour_v1alpha1.ContourDeployment{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "gatewayclass-params",
				},
			},
			wantConditions: []*meta_v1.Condition{
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusAccepted),
					Status: meta_v1.ConditionFalse,
					Reason: string(gatewayapi_v1.GatewayClassReasonInvalidParameters),
				},
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusSupportedVersion),
					Status: meta_v1.ConditionTrue,
					Reason: string(gatewayapi_v1.GatewayClassReasonSupportedVersion),
				},
			},
		},
		"gatewayclass controlled by us with an invalid parametersRef (invalid kind) gets Accepted: false condition": {
			gatewayClass: &gatewayapi_v1.GatewayClass{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: "gatewayclass-1",
				},
				Spec: gatewayapi_v1.GatewayClassSpec{
					ControllerName: "projectcontour.io/gateway-controller",
					ParametersRef: &gatewayapi_v1.ParametersReference{
						Group:     "projectcontour.io",
						Kind:      "InvalidKind",
						Name:      "gatewayclass-params",
						Namespace: ptr.To(gatewayapi_v1.Namespace("projectcontour")),
					},
				},
			},
			params: &contour_v1alpha1.ContourDeployment{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "gatewayclass-params",
				},
			},
			wantConditions: []*meta_v1.Condition{
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusAccepted),
					Status: meta_v1.ConditionFalse,
					Reason: string(gatewayapi_v1.GatewayClassReasonInvalidParameters),
				},
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusSupportedVersion),
					Status: meta_v1.ConditionTrue,
					Reason: string(gatewayapi_v1.GatewayClassReasonSupportedVersion),
				},
			},
		},
		"gatewayclass controlled by us with an invalid parametersRef (invalid name) gets Accepted: false condition": {
			gatewayClass: &gatewayapi_v1.GatewayClass{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: "gatewayclass-1",
				},
				Spec: gatewayapi_v1.GatewayClassSpec{
					ControllerName: "projectcontour.io/gateway-controller",
					ParametersRef: &gatewayapi_v1.ParametersReference{
						Group:     "projectcontour.io",
						Kind:      "ContourDeployment",
						Name:      "invalid-name",
						Namespace: ptr.To(gatewayapi_v1.Namespace("projectcontour")),
					},
				},
			},
			params: &contour_v1alpha1.ContourDeployment{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "gatewayclass-params",
				},
			},
			wantConditions: []*meta_v1.Condition{
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusAccepted),
					Status: meta_v1.ConditionFalse,
					Reason: string(gatewayapi_v1.GatewayClassReasonInvalidParameters),
				},
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusSupportedVersion),
					Status: meta_v1.ConditionTrue,
					Reason: string(gatewayapi_v1.GatewayClassReasonSupportedVersion),
				},
			},
		},
		"gatewayclass controlled by us with an invalid parametersRef (invalid namespace) gets Accepted: false condition": {
			gatewayClass: &gatewayapi_v1.GatewayClass{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: "gatewayclass-1",
				},
				Spec: gatewayapi_v1.GatewayClassSpec{
					ControllerName: "projectcontour.io/gateway-controller",
					ParametersRef: &gatewayapi_v1.ParametersReference{
						Group:     "projectcontour.io",
						Kind:      "ContourDeployment",
						Name:      "gatewayclass-params",
						Namespace: ptr.To(gatewayapi_v1.Namespace("invalid-namespace")),
					},
				},
			},
			params: &contour_v1alpha1.ContourDeployment{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "gatewayclass-params",
				},
			},
			wantConditions: []*meta_v1.Condition{
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusAccepted),
					Status: meta_v1.ConditionFalse,
					Reason: string(gatewayapi_v1.GatewayClassReasonInvalidParameters),
				},
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusSupportedVersion),
					Status: meta_v1.ConditionTrue,
					Reason: string(gatewayapi_v1.GatewayClassReasonSupportedVersion),
				},
			},
		},
		"gatewayclass controlled by us with a valid parametersRef gets Accepted: true condition": {
			gatewayClass: &gatewayapi_v1.GatewayClass{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: "gatewayclass-1",
				},
				Spec: gatewayapi_v1.GatewayClassSpec{
					ControllerName: "projectcontour.io/gateway-controller",
					ParametersRef: &gatewayapi_v1.ParametersReference{
						Group:     "projectcontour.io",
						Kind:      "ContourDeployment",
						Name:      "gatewayclass-params",
						Namespace: ptr.To(gatewayapi_v1.Namespace("projectcontour")),
					},
				},
			},
			params: &contour_v1alpha1.ContourDeployment{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "gatewayclass-params",
				},
			},
			wantConditions: []*meta_v1.Condition{
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusAccepted),
					Status: meta_v1.ConditionTrue,
					Reason: string(gatewayapi_v1.GatewayClassReasonAccepted),
				},
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusSupportedVersion),
					Status: meta_v1.ConditionTrue,
					Reason: string(gatewayapi_v1.GatewayClassReasonSupportedVersion),
				},
			},
		},
		"gatewayclass controlled by us with a valid parametersRef but invalid parameter values for NetworkPublishing gets Accepted: false condition": {
			gatewayClass: &gatewayapi_v1.GatewayClass{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: "gatewayclass-1",
				},
				Spec: gatewayapi_v1.GatewayClassSpec{
					ControllerName: "projectcontour.io/gateway-controller",
					ParametersRef: &gatewayapi_v1.ParametersReference{
						Group:     "projectcontour.io",
						Kind:      "ContourDeployment",
						Name:      "gatewayclass-params",
						Namespace: ptr.To(gatewayapi_v1.Namespace("projectcontour")),
					},
				},
			},
			params: &contour_v1alpha1.ContourDeployment{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "gatewayclass-params",
				},
				Spec: contour_v1alpha1.ContourDeploymentSpec{
					Envoy: &contour_v1alpha1.EnvoySettings{
						WorkloadType: "invalid-workload-type",
						NetworkPublishing: &contour_v1alpha1.NetworkPublishing{
							Type: "invalid-networkpublishing-type",
						},
					},
				},
			},
			wantConditions: []*meta_v1.Condition{
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusAccepted),
					Status: meta_v1.ConditionFalse,
					Reason: string(gatewayapi_v1.GatewayClassReasonInvalidParameters),
				},
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusSupportedVersion),
					Status: meta_v1.ConditionTrue,
					Reason: string(gatewayapi_v1.GatewayClassReasonSupportedVersion),
				},
			},
		},
		"gatewayclass controlled by us with a valid parametersRef but invalid parameter values for ExtraVolumeMounts gets Accepted: false condition": {
			gatewayClass: &gatewayapi_v1.GatewayClass{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: "gatewayclass-1",
				},
				Spec: gatewayapi_v1.GatewayClassSpec{
					ControllerName: "projectcontour.io/gateway-controller",
					ParametersRef: &gatewayapi_v1.ParametersReference{
						Group:     "projectcontour.io",
						Kind:      "ContourDeployment",
						Name:      "gatewayclass-params",
						Namespace: ptr.To(gatewayapi_v1.Namespace("projectcontour")),
					},
				},
			},
			params: &contour_v1alpha1.ContourDeployment{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "gatewayclass-params",
				},
				Spec: contour_v1alpha1.ContourDeploymentSpec{
					Envoy: &contour_v1alpha1.EnvoySettings{
						ExtraVolumeMounts: []core_v1.VolumeMount{
							{
								Name: "volume-a",
							},
						},
						ExtraVolumes: []core_v1.Volume{
							{
								Name: "volume-b",
							},
						},
					},
				},
			},
			wantConditions: []*meta_v1.Condition{
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusAccepted),
					Status: meta_v1.ConditionFalse,
					Reason: string(gatewayapi_v1.GatewayClassReasonInvalidParameters),
				},
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusSupportedVersion),
					Status: meta_v1.ConditionTrue,
					Reason: string(gatewayapi_v1.GatewayClassReasonSupportedVersion),
				},
			},
		},
		"gatewayclass controlled by us with a valid parametersRef but invalid parameter values for LogLevel gets Accepted: false condition": {
			gatewayClass: &gatewayapi_v1.GatewayClass{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: "gatewayclass-1",
				},
				Spec: gatewayapi_v1.GatewayClassSpec{
					ControllerName: "projectcontour.io/gateway-controller",
					ParametersRef: &gatewayapi_v1.ParametersReference{
						Group:     "projectcontour.io",
						Kind:      "ContourDeployment",
						Name:      "gatewayclass-params",
						Namespace: ptr.To(gatewayapi_v1.Namespace("projectcontour")),
					},
				},
			},
			params: &contour_v1alpha1.ContourDeployment{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "gatewayclass-params",
				},
				Spec: contour_v1alpha1.ContourDeploymentSpec{
					Envoy: &contour_v1alpha1.EnvoySettings{
						LogLevel: "invalidLevel",
					},
				},
			},
			wantConditions: []*meta_v1.Condition{
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusAccepted),
					Status: meta_v1.ConditionFalse,
					Reason: string(gatewayapi_v1.GatewayClassReasonInvalidParameters),
				},
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusSupportedVersion),
					Status: meta_v1.ConditionTrue,
					Reason: string(gatewayapi_v1.GatewayClassReasonSupportedVersion),
				},
			},
		},
		"gatewayclass controlled by us with a valid parametersRef but invalid parameter values for ExternalTrafficPolicy gets Accepted: false condition": {
			gatewayClass: &gatewayapi_v1.GatewayClass{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: "gatewayclass-1",
				},
				Spec: gatewayapi_v1.GatewayClassSpec{
					ControllerName: "projectcontour.io/gateway-controller",
					ParametersRef: &gatewayapi_v1.ParametersReference{
						Group:     "projectcontour.io",
						Kind:      "ContourDeployment",
						Name:      "gatewayclass-params",
						Namespace: ptr.To(gatewayapi_v1.Namespace("projectcontour")),
					},
				},
			},
			params: &contour_v1alpha1.ContourDeployment{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "gatewayclass-params",
				},
				Spec: contour_v1alpha1.ContourDeploymentSpec{
					Envoy: &contour_v1alpha1.EnvoySettings{
						NetworkPublishing: &contour_v1alpha1.NetworkPublishing{
							ExternalTrafficPolicy: "invalid-external-traffic-policy",
						},
					},
				},
			},
			wantConditions: []*meta_v1.Condition{
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusAccepted),
					Status: meta_v1.ConditionFalse,
					Reason: string(gatewayapi_v1.GatewayClassReasonInvalidParameters),
				},
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusSupportedVersion),
					Status: meta_v1.ConditionTrue,
					Reason: string(gatewayapi_v1.GatewayClassReasonSupportedVersion),
				},
			},
		},
		"gatewayclass controlled by us with a valid parametersRef but invalid parameter values for IPFamilyPolicy gets Accepted: false condition": {
			gatewayClass: &gatewayapi_v1.GatewayClass{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: "gatewayclass-1",
				},
				Spec: gatewayapi_v1.GatewayClassSpec{
					ControllerName: "projectcontour.io/gateway-controller",
					ParametersRef: &gatewayapi_v1.ParametersReference{
						Group:     "projectcontour.io",
						Kind:      "ContourDeployment",
						Name:      "gatewayclass-params",
						Namespace: ptr.To(gatewayapi_v1.Namespace("projectcontour")),
					},
				},
			},
			params: &contour_v1alpha1.ContourDeployment{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "gatewayclass-params",
				},
				Spec: contour_v1alpha1.ContourDeploymentSpec{
					Envoy: &contour_v1alpha1.EnvoySettings{
						NetworkPublishing: &contour_v1alpha1.NetworkPublishing{
							IPFamilyPolicy: "invalid-external-traffic-policy",
						},
					},
				},
			},
			wantConditions: []*meta_v1.Condition{
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusAccepted),
					Status: meta_v1.ConditionFalse,
					Reason: string(gatewayapi_v1.GatewayClassReasonInvalidParameters),
				},
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusSupportedVersion),
					Status: meta_v1.ConditionTrue,
					Reason: string(gatewayapi_v1.GatewayClassReasonSupportedVersion),
				},
			},
		},
		"gatewayclass controlled by us with gatewayclass CRD with unsupported version sets Accepted: true, SupportedVersion: False": {
			gatewayClass: &gatewayapi_v1.GatewayClass{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: "gatewayclass-1",
				},
				Spec: gatewayapi_v1.GatewayClassSpec{
					ControllerName: "projectcontour.io/gateway-controller",
				},
			},
			gatewayClassCRD: &apiextensions_v1.CustomResourceDefinition{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: "gatewayclasses.gateway.networking.k8s.io",
					Annotations: map[string]string{
						"gateway.networking.k8s.io/bundle-version": "v9.9.9",
					},
				},
			},
			wantConditions: []*meta_v1.Condition{
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusAccepted),
					Status: meta_v1.ConditionTrue,
					Reason: string(gatewayapi_v1.GatewayClassReasonAccepted),
				},
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusSupportedVersion),
					Status: meta_v1.ConditionFalse,
					Reason: string(gatewayapi_v1.GatewayClassReasonUnsupportedVersion),
				},
			},
			assertions: func(t *testing.T, _ *gatewayClassReconciler, _ *gatewayapi_v1.GatewayClass, reconcileErr error) {
				require.NoError(t, reconcileErr)
			},
		},
		"gatewayclass controlled by us with gatewayclass CRD fetch failed sets SupportedVersion: false": {
			gatewayClass: &gatewayapi_v1.GatewayClass{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: "gatewayclass-1",
				},
				Spec: gatewayapi_v1.GatewayClassSpec{
					ControllerName: "projectcontour.io/gateway-controller",
				},
			},
			gatewayClassCRD: &apiextensions_v1.CustomResourceDefinition{
				ObjectMeta: meta_v1.ObjectMeta{
					// Use the wrong name so we fail to fetch the CRD,
					// contrived way to cause this scenario.
					Name: "gatewayclasses-wrong.gateway.networking.k8s.io",
					Annotations: map[string]string{
						gatewayAPIBundleVersionAnnotation: gatewayAPICRDBundleSupportedVersion,
					},
				},
			},
			wantConditions: []*meta_v1.Condition{
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusAccepted),
					Status: meta_v1.ConditionTrue,
					Reason: string(gatewayapi_v1.GatewayClassReasonAccepted),
				},
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusSupportedVersion),
					Status: meta_v1.ConditionFalse,
					Reason: string(gatewayapi_v1.GatewayClassReasonUnsupportedVersion),
				},
			},
			assertions: func(t *testing.T, _ *gatewayClassReconciler, _ *gatewayapi_v1.GatewayClass, reconcileErr error) {
				require.NoError(t, reconcileErr)
			},
		},
		"gatewayclass controlled by us with gatewayclass CRD without version annotation sets SupportedVersion: false": {
			gatewayClass: &gatewayapi_v1.GatewayClass{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: "gatewayclass-1",
				},
				Spec: gatewayapi_v1.GatewayClassSpec{
					ControllerName: "projectcontour.io/gateway-controller",
				},
			},
			gatewayClassCRD: &apiextensions_v1.CustomResourceDefinition{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: "gatewayclasses.gateway.networking.k8s.io",
					Annotations: map[string]string{
						"gateway.networking.k8s.io/bundle-version-wrong": gatewayAPICRDBundleSupportedVersion,
					},
				},
			},
			wantConditions: []*meta_v1.Condition{
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusAccepted),
					Status: meta_v1.ConditionTrue,
					Reason: string(gatewayapi_v1.GatewayClassReasonAccepted),
				},
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusSupportedVersion),
					Status: meta_v1.ConditionFalse,
					Reason: string(gatewayapi_v1.GatewayClassReasonUnsupportedVersion),
				},
			},
			assertions: func(t *testing.T, _ *gatewayClassReconciler, _ *gatewayapi_v1.GatewayClass, reconcileErr error) {
				require.NoError(t, reconcileErr)
			},
		},
		"gatewayclass with status from previous generation is updated, only conditions we own are changed": {
			gatewayClass: &gatewayapi_v1.GatewayClass{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:       "gatewayclass-1",
					Generation: 2,
				},
				Spec: gatewayapi_v1.GatewayClassSpec{
					ControllerName: "projectcontour.io/gateway-controller",
				},
				Status: gatewayapi_v1.GatewayClassStatus{
					Conditions: []meta_v1.Condition{
						{
							Type:               string(gatewayapi_v1.GatewayClassConditionStatusAccepted),
							Status:             meta_v1.ConditionTrue,
							Reason:             string(gatewayapi_v1.GatewayClassReasonAccepted),
							ObservedGeneration: 1,
						},
						{
							Type:               "SomeOtherCondition",
							Status:             meta_v1.ConditionTrue,
							Reason:             "FooReason",
							ObservedGeneration: 1,
						},
					},
				},
			},
			wantConditions: []*meta_v1.Condition{
				{
					Type:               string(gatewayapi_v1.GatewayClassConditionStatusAccepted),
					Status:             meta_v1.ConditionTrue,
					Reason:             string(gatewayapi_v1.GatewayClassReasonAccepted),
					ObservedGeneration: 2,
				},
				{
					Type:               string(gatewayapi_v1.GatewayClassConditionStatusSupportedVersion),
					Status:             meta_v1.ConditionTrue,
					Reason:             string(gatewayapi_v1.GatewayClassReasonSupportedVersion),
					ObservedGeneration: 2,
				},
				{
					Type:               "SomeOtherCondition",
					Status:             meta_v1.ConditionTrue,
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
					ObjectMeta: meta_v1.ObjectMeta{
						Name: "gatewayclasses.gateway.networking.k8s.io",
						Annotations: map[string]string{
							gatewayAPIBundleVersionAnnotation: gatewayAPICRDBundleSupportedVersion,
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
				res := &gatewayapi_v1.GatewayClass{}
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
