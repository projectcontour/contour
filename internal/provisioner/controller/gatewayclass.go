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
	"github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	contour_api_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	gatewayapi_v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

// gatewayClassReconciler reconciles GatewayClass objects.
type gatewayClassReconciler struct {
	gatewayController gatewayapi_v1beta1.GatewayController
	client            client.Client
	log               logr.Logger
}

func NewGatewayClassController(mgr manager.Manager, gatewayController string) (controller.Controller, error) {
	r := &gatewayClassReconciler{
		gatewayController: gatewayapi_v1beta1.GatewayController(gatewayController),
		client:            mgr.GetClient(),
		log:               ctrl.Log.WithName("gatewayclass-controller"),
	}

	c, err := controller.New("gatewayclass-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return nil, err
	}

	if err := c.Watch(
		&source.Kind{Type: &gatewayapi_v1beta1.GatewayClass{}},
		&handler.EnqueueRequestForObject{},
		predicate.NewPredicateFuncs(r.hasMatchingController),
	); err != nil {
		return nil, err
	}

	// Watch ContourDeployments since they can be used as parameters for
	// GatewayClasses.
	if err := c.Watch(
		&source.Kind{Type: &contour_api_v1alpha1.ContourDeployment{}},
		handler.EnqueueRequestsFromMapFunc(r.mapContourDeploymentToGatewayClasses),
	); err != nil {
		return nil, err
	}

	return c, nil
}

func (r *gatewayClassReconciler) hasMatchingController(obj client.Object) bool {
	gatewayClass, ok := obj.(*gatewayapi_v1beta1.GatewayClass)
	if !ok {
		return false
	}

	return gatewayClass.Spec.ControllerName == r.gatewayController
}

// mapContourDeploymentToGatewayClasses returns a list of reconcile requests
// for all provisioner-controlled GatewayClasses that have a ParametersRef to
// the specified ContourDeployment object.
func (r *gatewayClassReconciler) mapContourDeploymentToGatewayClasses(contourDeployment client.Object) []reconcile.Request {
	var gatewayClasses gatewayapi_v1beta1.GatewayClassList
	if err := r.client.List(context.Background(), &gatewayClasses); err != nil {
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
	gatewayClass := &gatewayapi_v1beta1.GatewayClass{}
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

	ok, params, err := r.isValidParametersRef(ctx, gatewayClass.Spec.ParametersRef)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error checking gateway class's parametersRef: %w", err)
	}
	if !ok {
		if err := r.setAcceptedCondition(
			ctx,
			gatewayClass,
			metav1.ConditionFalse,
			gatewayapi_v1beta1.GatewayClassReasonInvalidParameters,
			"Invalid ParametersRef, must be a reference to an existing namespaced projectcontour.io/ContourDeployment resource",
		); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to set gateway class %s Accepted condition: %w", req, err)
		}

		return ctrl.Result{}, nil
	}

	// If parameters are referenced, validate the values.
	if params != nil {
		var invalidParamsMessages []string

		if params.Spec.Envoy != nil {
			switch params.Spec.Envoy.WorkloadType {
			// valid values, nothing to do
			case "", contour_api_v1alpha1.WorkloadTypeDaemonSet, contour_api_v1alpha1.WorkloadTypeDeployment:
			// invalid value, set message
			default:
				msg := fmt.Sprintf("invalid ContourDeployment spec.envoy.workloadType %q, must be DaemonSet or Deployment", params.Spec.Envoy.WorkloadType)
				invalidParamsMessages = append(invalidParamsMessages, msg)
			}

			if params.Spec.Envoy.NetworkPublishing != nil {
				switch params.Spec.Envoy.NetworkPublishing.Type {
				// valid values, nothing to do
				case "", contour_api_v1alpha1.LoadBalancerServicePublishingType, contour_api_v1alpha1.NodePortServicePublishingType, contour_api_v1alpha1.ClusterIPServicePublishingType:
				// invalid value, set message
				default:
					msg := fmt.Sprintf("invalid ContourDeployment spec.envoy.networkPublishing.type %q, must be LoadBalancerService, NoderPortService or ClusterIPService",
						params.Spec.Envoy.NetworkPublishing.Type)
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
			case "", v1alpha1.TraceLog, v1alpha1.DebugLog, v1alpha1.InfoLog, v1alpha1.WarnLog, v1alpha1.ErrorLog, v1alpha1.CriticalLog, v1alpha1.OffLog:
			// invalid value, set message.
			default:
				msg := fmt.Sprintf("invalid ContourDeployment spec.envoy.logLevel %q, must be trace, debug, info, warn, error, critical or off",
					params.Spec.Envoy.LogLevel)
				invalidParamsMessages = append(invalidParamsMessages, msg)
			}
		}

		if len(invalidParamsMessages) > 0 {
			if err := r.setAcceptedCondition(
				ctx,
				gatewayClass,
				metav1.ConditionFalse,
				gatewayapi_v1beta1.GatewayClassReasonInvalidParameters,
				strings.Join(invalidParamsMessages, "; "),
			); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to set gateway class %s Accepted condition: %w", req, err)
			}

			return ctrl.Result{}, nil
		}
	}

	if err := r.setAcceptedCondition(ctx, gatewayClass, metav1.ConditionTrue, gatewayapi_v1beta1.GatewayClassReasonAccepted, "GatewayClass has been accepted by the controller"); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to set gateway class %s Accepted condition: %w", req, err)
	}

	return ctrl.Result{}, nil
}

func (r *gatewayClassReconciler) setAcceptedCondition(
	ctx context.Context,
	gatewayClass *gatewayapi_v1beta1.GatewayClass,
	status metav1.ConditionStatus,
	reason gatewayapi_v1beta1.GatewayClassConditionReason,
	message string,
) error {
	var newConds []metav1.Condition
	for _, cond := range gatewayClass.Status.Conditions {
		if cond.Type == string(gatewayapi_v1beta1.GatewayClassConditionStatusAccepted) {
			if cond.Status == status {
				return nil
			}

			continue
		}

		newConds = append(newConds, cond)
	}

	r.log.WithValues("gatewayclass-name", gatewayClass.Name).Info(fmt.Sprintf("setting gateway class's Accepted condition to %s", status))

	// nolint:gocritic
	gatewayClass.Status.Conditions = append(newConds, metav1.Condition{
		Type:               string(gatewayapi_v1beta1.GatewayClassConditionStatusAccepted),
		Status:             status,
		ObservedGeneration: gatewayClass.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             string(reason),
		Message:            message,
	})

	if err := r.client.Status().Update(ctx, gatewayClass); err != nil {
		return fmt.Errorf("failed to set gatewayclass %s accepted condition: %w", gatewayClass.Name, err)
	}

	return nil
}

// isValidParametersRef returns true if the provided ParametersReference is
// to a ContourDeployment resource that exists.
func (r *gatewayClassReconciler) isValidParametersRef(ctx context.Context, ref *gatewayapi_v1beta1.ParametersReference) (bool, *contour_api_v1alpha1.ContourDeployment, error) {
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

	params := &contour_api_v1alpha1.ContourDeployment{}
	if err := r.client.Get(ctx, key, params); err != nil {
		if errors.IsNotFound(err) {
			return false, nil, nil
		}
		return false, nil, err
	}

	return true, params, nil
}

func isContourDeploymentRef(ref *gatewayapi_v1beta1.ParametersReference) bool {
	if ref == nil {
		return false
	}
	if string(ref.Group) != contour_api_v1alpha1.GroupVersion.Group {
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
