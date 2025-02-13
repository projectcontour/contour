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
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	core_v1 "k8s.io/api/core/v1"
	apiextensions_v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	gatewayapi_v1 "sigs.k8s.io/gateway-api/apis/v1"

	contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
)

const (
	gatewayAPIBundleVersionAnnotation   = "gateway.networking.k8s.io/bundle-version"
	gatewayAPICRDBundleSupportedVersion = "v1.2.1"
)

// gatewayClassReconciler reconciles GatewayClass objects.
type gatewayClassReconciler struct {
	gatewayController gatewayapi_v1.GatewayController
	client            client.Client
	log               logr.Logger
}

func NewGatewayClassController(mgr manager.Manager, gatewayController string) (controller.Controller, error) {
	r := &gatewayClassReconciler{
		gatewayController: gatewayapi_v1.GatewayController(gatewayController),
		client:            mgr.GetClient(),
		log:               ctrl.Log.WithName("gatewayclass-controller"),
	}

	c, err := controller.New("gatewayclass-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return nil, err
	}

	if err := c.Watch(
		source.Kind(mgr.GetCache(), &gatewayapi_v1.GatewayClass{},
			&handler.TypedEnqueueRequestForObject[*gatewayapi_v1.GatewayClass]{},
			predicate.NewTypedPredicateFuncs(r.hasMatchingController)),
	); err != nil {
		return nil, err
	}

	// Watch ContourDeployments since they can be used as parameters for
	// GatewayClasses.
	if err := c.Watch(
		source.Kind(mgr.GetCache(), &contour_v1alpha1.ContourDeployment{},
			handler.TypedEnqueueRequestsFromMapFunc(r.mapContourDeploymentToGatewayClasses)),
	); err != nil {
		return nil, err
	}

	return c, nil
}

func (r *gatewayClassReconciler) hasMatchingController(gatewayClass *gatewayapi_v1.GatewayClass) bool {
	return gatewayClass.Spec.ControllerName == r.gatewayController
}

// mapContourDeploymentToGatewayClasses returns a list of reconcile requests
// for all provisioner-controlled GatewayClasses that have a ParametersRef to
// the specified ContourDeployment object.
func (r *gatewayClassReconciler) mapContourDeploymentToGatewayClasses(ctx context.Context, contourDeployment *contour_v1alpha1.ContourDeployment) []reconcile.Request {
	var gatewayClasses gatewayapi_v1.GatewayClassList
	if err := r.client.List(ctx, &gatewayClasses); err != nil {
		r.log.Error(err, "error listing gateway classes")
		return nil
	}

	var reconciles []reconcile.Request
	for i := range gatewayClasses.Items {
		gc := &gatewayClasses.Items[i]

		if !r.hasMatchingController(gc) {
			continue
		}
		if !isContourDeploymentRef(gc.Spec.ParametersRef) {
			continue
		}
		if gc.Spec.ParametersRef.Namespace == nil || string(*gc.Spec.ParametersRef.Namespace) != contourDeployment.GetNamespace() {
			continue
		}
		if gc.Spec.ParametersRef.Name != contourDeployment.GetName() {
			continue
		}

		reconciles = append(reconciles, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name: gc.Name,
			},
		})
	}

	return reconciles
}

func (r *gatewayClassReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	gatewayClass := &gatewayapi_v1.GatewayClass{}
	if err := r.client.Get(ctx, req.NamespacedName, gatewayClass); err != nil {
		// GatewayClass no longer exists, nothing to do.
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		// Error reading the object, so requeue the request.
		return ctrl.Result{}, fmt.Errorf("failed to get gatewayclass %s: %w", req, err)
	}

	// Theoretically all event sources should be filtered already, but doesn't hurt
	// to double-check this here to ensure we only reconcile gateway classes the
	// provisioner controls.
	if !r.hasMatchingController(gatewayClass) {
		return ctrl.Result{}, nil
	}

	// Collect various status conditions here so we can update using
	// setConditions.
	statusConditions := map[string]meta_v1.Condition{}

	statusConditions[string(gatewayapi_v1.GatewayClassConditionStatusSupportedVersion)] = r.getSupportedVersionCondition(ctx)

	ok, params, err := r.isValidParametersRef(ctx, gatewayClass.Spec.ParametersRef)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error checking gateway class's parametersRef: %w", err)
	}
	if !ok {
		statusConditions[string(gatewayapi_v1.GatewayClassConditionStatusAccepted)] = meta_v1.Condition{
			Type:    string(gatewayapi_v1.GatewayClassConditionStatusAccepted),
			Status:  meta_v1.ConditionFalse,
			Reason:  string(gatewayapi_v1.GatewayClassReasonInvalidParameters),
			Message: "Invalid ParametersRef, must be a reference to an existing namespaced projectcontour.io/ContourDeployment resource",
		}
		if err := r.setConditions(ctx, gatewayClass, statusConditions); err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	}

	// If parameters are referenced, validate the values.
	if params != nil {
		var invalidParamsMessages []string

		if params.Spec.Envoy != nil {
			switch params.Spec.Envoy.WorkloadType {
			// valid values, nothing to do
			case "", contour_v1alpha1.WorkloadTypeDaemonSet, contour_v1alpha1.WorkloadTypeDeployment:
			// invalid value, set message
			default:
				msg := fmt.Sprintf("invalid ContourDeployment spec.envoy.workloadType %q, must be DaemonSet or Deployment", params.Spec.Envoy.WorkloadType)
				invalidParamsMessages = append(invalidParamsMessages, msg)
			}

			if params.Spec.Envoy.NetworkPublishing != nil {
				switch params.Spec.Envoy.NetworkPublishing.Type {
				// valid values, nothing to do
				case "", contour_v1alpha1.LoadBalancerServicePublishingType, contour_v1alpha1.NodePortServicePublishingType, contour_v1alpha1.ClusterIPServicePublishingType:
				// invalid value, set message
				default:
					msg := fmt.Sprintf("invalid ContourDeployment spec.envoy.networkPublishing.type %q, must be LoadBalancerService, NoderPortService or ClusterIPService",
						params.Spec.Envoy.NetworkPublishing.Type)
					invalidParamsMessages = append(invalidParamsMessages, msg)
				}

				switch params.Spec.Envoy.NetworkPublishing.IPFamilyPolicy {
				case "", core_v1.IPFamilyPolicySingleStack, core_v1.IPFamilyPolicyPreferDualStack, core_v1.IPFamilyPolicyRequireDualStack:
				default:
					msg := fmt.Sprintf("invalid ContourDeployment spec.envoy.networkPublishing.ipFamilyPolicy %q, must be SingleStack, PreferDualStack or RequireDualStack",
						params.Spec.Envoy.NetworkPublishing.IPFamilyPolicy)
					invalidParamsMessages = append(invalidParamsMessages, msg)
				}

				switch params.Spec.Envoy.NetworkPublishing.ExternalTrafficPolicy {
				case "", core_v1.ServiceExternalTrafficPolicyTypeCluster, core_v1.ServiceExternalTrafficPolicyTypeLocal:
				default:
					msg := fmt.Sprintf("invalid ContourDeployment spec.envoy.networkPublishing.externalTrafficPolicy %q, must be Local or Cluster",
						params.Spec.Envoy.NetworkPublishing.ExternalTrafficPolicy)
					invalidParamsMessages = append(invalidParamsMessages, msg)
				}
			}

			if params.Spec.Envoy.ExtraVolumeMounts != nil {
				volumes := map[string]struct{}{}
				for _, vol := range params.Spec.Envoy.ExtraVolumes {
					volumes[vol.Name] = struct{}{}
				}
				for _, mnt := range params.Spec.Envoy.ExtraVolumeMounts {
					if _, ok := volumes[mnt.Name]; !ok {
						msg := fmt.Sprintf("invalid ContourDeployment spec.envoy.extraVolumeMounts, mount to unknown volume: %q", mnt.Name)
						invalidParamsMessages = append(invalidParamsMessages, msg)
					}
				}
			}

			switch params.Spec.Envoy.LogLevel {
			// valid values, nothing to do.
			case "", contour_v1alpha1.TraceLog, contour_v1alpha1.DebugLog, contour_v1alpha1.InfoLog,
				contour_v1alpha1.WarnLog, contour_v1alpha1.ErrorLog, contour_v1alpha1.CriticalLog, contour_v1alpha1.OffLog:
			// invalid value, set message.
			default:
				msg := fmt.Sprintf("invalid ContourDeployment spec.envoy.logLevel %q, must be trace, debug, info, warn, error, critical or off",
					params.Spec.Envoy.LogLevel)
				invalidParamsMessages = append(invalidParamsMessages, msg)
			}
		}

		if len(invalidParamsMessages) > 0 {
			statusConditions[string(gatewayapi_v1.GatewayClassConditionStatusAccepted)] = meta_v1.Condition{
				Type:    string(gatewayapi_v1.GatewayClassConditionStatusAccepted),
				Status:  meta_v1.ConditionFalse,
				Reason:  string(gatewayapi_v1.GatewayClassReasonInvalidParameters),
				Message: strings.Join(invalidParamsMessages, "; "),
			}
			if err := r.setConditions(ctx, gatewayClass, statusConditions); err != nil {
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, nil
		}
	}

	statusConditions[string(gatewayapi_v1.GatewayClassConditionStatusAccepted)] = meta_v1.Condition{
		Type:    string(gatewayapi_v1.GatewayClassConditionStatusAccepted),
		Status:  meta_v1.ConditionTrue,
		Reason:  string(gatewayapi_v1.GatewayClassReasonAccepted),
		Message: "GatewayClass has been accepted by the controller",
	}
	if err := r.setConditions(ctx, gatewayClass, statusConditions); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *gatewayClassReconciler) setConditions(ctx context.Context, gatewayClass *gatewayapi_v1.GatewayClass, newConds map[string]meta_v1.Condition) error {
	var unchangedConds, updatedConds []meta_v1.Condition
	for _, existing := range gatewayClass.Status.Conditions {
		if cond, ok := newConds[existing.Type]; ok {
			if existing.Status == cond.Status {
				// If status hasn't changed, don't change the condition, just
				// update the generation.
				changed := existing
				changed.ObservedGeneration = gatewayClass.Generation
				updatedConds = append(updatedConds, changed)
				delete(newConds, cond.Type)
			}
		} else {
			unchangedConds = append(unchangedConds, existing)
		}
	}

	transitionTime := meta_v1.Now()
	for _, c := range newConds {
		r.log.WithValues("gatewayclass-name", gatewayClass.Name).Info(fmt.Sprintf("setting gateway class's %s condition to %s", c.Type, c.Status))
		c.ObservedGeneration = gatewayClass.Generation
		c.LastTransitionTime = transitionTime
		updatedConds = append(updatedConds, c)
	}

	// nolint:gocritic
	gatewayClass.Status.Conditions = append(unchangedConds, updatedConds...)

	if err := r.client.Status().Update(ctx, gatewayClass); err != nil {
		return fmt.Errorf("failed to update gateway class %s status: %w", gatewayClass.Name, err)
	}
	return nil
}

func (r *gatewayClassReconciler) getSupportedVersionCondition(ctx context.Context) meta_v1.Condition {
	cond := meta_v1.Condition{
		Type: string(gatewayapi_v1.GatewayClassConditionStatusSupportedVersion),
		// Assume false until we get to the happy case.
		Status: meta_v1.ConditionFalse,
		Reason: string(gatewayapi_v1.GatewayClassReasonUnsupportedVersion),
	}
	gatewayClassCRD := &apiextensions_v1.CustomResourceDefinition{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "gatewayclasses." + gatewayapi_v1.GroupName,
		},
	}
	if err := r.client.Get(ctx, client.ObjectKeyFromObject(gatewayClassCRD), gatewayClassCRD); err != nil {
		errorMsg := "Error fetching gatewayclass CRD resource to validate supported version"
		r.log.Error(err, errorMsg)
		cond.Message = fmt.Sprintf("%s: %s. Resources will be reconciled with best-effort.", errorMsg, err)
		return cond
	}

	version, ok := gatewayClassCRD.Annotations[gatewayAPIBundleVersionAnnotation]
	if !ok {
		cond.Message = fmt.Sprintf("Bundle version annotation %s not found. Resources will be reconciled with best-effort.", gatewayAPIBundleVersionAnnotation)
		return cond
	}
	if version != gatewayAPICRDBundleSupportedVersion {
		cond.Message = fmt.Sprintf("Gateway API CRD bundle version %s is not supported. Resources will be reconciled with best-effort.", version)
		return cond
	}

	// No errors found, we can return true.
	cond.Status = meta_v1.ConditionTrue
	cond.Reason = string(gatewayapi_v1.GatewayClassReasonSupportedVersion)
	cond.Message = fmt.Sprintf("Gateway API CRD bundle version %s is supported.", gatewayAPICRDBundleSupportedVersion)
	return cond
}

// isValidParametersRef returns true if the provided ParametersReference is
// to a ContourDeployment resource that exists.
func (r *gatewayClassReconciler) isValidParametersRef(ctx context.Context, ref *gatewayapi_v1.ParametersReference) (bool, *contour_v1alpha1.ContourDeployment, error) {
	if ref == nil {
		return true, nil, nil
	}

	if !isContourDeploymentRef(ref) {
		return false, nil, nil
	}

	key := client.ObjectKey{
		Namespace: string(*ref.Namespace),
		Name:      ref.Name,
	}

	params := &contour_v1alpha1.ContourDeployment{}
	if err := r.client.Get(ctx, key, params); err != nil {
		if errors.IsNotFound(err) {
			return false, nil, nil
		}
		return false, nil, err
	}

	return true, params, nil
}

func isContourDeploymentRef(ref *gatewayapi_v1.ParametersReference) bool {
	if ref == nil {
		return false
	}
	if string(ref.Group) != contour_v1alpha1.GroupVersion.Group {
		return false
	}
	if string(ref.Kind) != "ContourDeployment" {
		return false
	}
	if ref.Namespace == nil {
		return false
	}

	return true
}
