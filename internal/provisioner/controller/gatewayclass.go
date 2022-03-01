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

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

// gatewayClassReconciler reconciles GatewayClass objects.
type gatewayClassReconciler struct {
	gatewayController gatewayapi_v1alpha2.GatewayController
	client            client.Client
	log               logr.Logger
}

func NewGatewayClassController(mgr manager.Manager, gatewayController string) (controller.Controller, error) {
	r := &gatewayClassReconciler{
		gatewayController: gatewayapi_v1alpha2.GatewayController(gatewayController),
		client:            mgr.GetClient(),
		log:               ctrl.Log.WithName("gatewayclass-controller"),
	}

	c, err := controller.New("gatewayclass-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return nil, err
	}

	if err := c.Watch(
		&source.Kind{Type: &gatewayapi_v1alpha2.GatewayClass{}},
		&handler.EnqueueRequestForObject{},
		predicate.NewPredicateFuncs(r.hasMatchingController),
	); err != nil {
		return nil, err
	}

	return c, nil
}

func (r *gatewayClassReconciler) hasMatchingController(obj client.Object) bool {
	gatewayClass, ok := obj.(*gatewayapi_v1alpha2.GatewayClass)
	if !ok {
		return false
	}

	return gatewayClass.Spec.ControllerName == r.gatewayController
}

func (r *gatewayClassReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	gatewayClass := &gatewayapi_v1alpha2.GatewayClass{}
	if err := r.client.Get(ctx, req.NamespacedName, gatewayClass); err != nil {
		// GatewayClass no longer exists, nothing to do.
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		// Error reading the object, so requeue the request.
		return ctrl.Result{}, fmt.Errorf("failed to get gatewayclass %s: %w", req, err)
	}

	var newConds []metav1.Condition
	for _, cond := range gatewayClass.Status.Conditions {
		if cond.Type == string(gatewayapi_v1alpha2.GatewayClassConditionStatusAccepted) {
			if cond.Status == metav1.ConditionTrue {
				return ctrl.Result{}, nil
			}

			continue
		}

		newConds = append(newConds, cond)
	}

	r.log.WithValues("gatewayclass-name", req.Name).Info("setting gateway class's Accepted condition to true")

	gatewayClass.Status.Conditions = append(newConds, metav1.Condition{
		Type:               string(gatewayapi_v1alpha2.GatewayClassConditionStatusAccepted),
		Status:             metav1.ConditionTrue,
		ObservedGeneration: gatewayClass.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             string(gatewayapi_v1alpha2.GatewayClassReasonAccepted),
		Message:            "GatewayClass has been accepted by the controller",
	})

	if err := r.client.Status().Update(ctx, gatewayClass); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to set gatewayclass %s accepted condition: %w", req, err)
	}

	return ctrl.Result{}, nil
}
