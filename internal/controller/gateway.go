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

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	gatewayapi_v1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

type gatewayReconciler struct {
	ctx          context.Context
	client       client.Client
	eventHandler cache.ResourceEventHandler
	log          logrus.FieldLogger

	// gatewayClassControllerName is the configured controller of managed gatewayclasses.
	gatewayClassControllerName string
}

// NewGatewayController creates the gateway controller from mgr. The controller will be pre-configured
// to watch for Gateway objects across all namespaces and reconcile those that match class.
func NewGatewayController(mgr manager.Manager, eventHandler cache.ResourceEventHandler, log logrus.FieldLogger, gatewayClassControllerName string) (controller.Controller, error) {
	r := &gatewayReconciler{
		ctx:                        context.Background(),
		client:                     mgr.GetClient(),
		eventHandler:               eventHandler,
		log:                        log,
		gatewayClassControllerName: gatewayClassControllerName,
	}
	c, err := controller.New("gateway-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return nil, err
	}

	if err := c.Watch(
		&source.Kind{Type: &gatewayapi_v1alpha1.Gateway{}},
		&handler.EnqueueRequestForObject{},
		predicate.NewPredicateFuncs(r.hasMatchingController),
	); err != nil {
		return nil, err
	}
	return c, nil
}

// hasMatchingController returns true if the provided object is a Gateway
// using a GatewayClass with a Spec.Controller string matching this Contour's
// controller string, or false otherwise.
func (r *gatewayReconciler) hasMatchingController(obj client.Object) bool {
	log := r.log.WithFields(logrus.Fields{
		"namespace": obj.GetNamespace(),
		"name":      obj.GetName(),
	})

	gw, ok := obj.(*gatewayapi_v1alpha1.Gateway)
	if !ok {
		log.Info("invalid object, bypassing reconciliation.")
		return false
	}

	matches, err := r.hasContourOwnedClass(gw)
	if err != nil {
		log.Error(err)
		return false
	}
	if matches {
		log.Info("enqueueing gateway")
		return true
	}

	log.Info("configured controllerName doesn't match an existing GatewayClass")
	return false
}

// hasContourOwnedClass returns true if the class referenced by gateway exists
// and matches the configured controllerName.
func (r *gatewayReconciler) hasContourOwnedClass(gw *gatewayapi_v1alpha1.Gateway) (bool, error) {
	gc := &gatewayapi_v1alpha1.GatewayClass{}
	if err := r.client.Get(r.ctx, types.NamespacedName{Name: gw.Spec.GatewayClassName}, gc); err != nil {
		return false, fmt.Errorf("failed to get gatewayclass %s: %w", gw.Spec.GatewayClassName, err)
	}
	if gc.Spec.Controller != r.gatewayClassControllerName {
		return false, nil
	}
	return true, nil
}

func (r *gatewayReconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	r.log.WithField("namespace", request.Namespace).WithField("name", request.Name).Info("reconciling gateway")

	// Fetch the Gateway.
	gw := &gatewayapi_v1alpha1.Gateway{}
	if err := r.client.Get(ctx, request.NamespacedName, gw); errors.IsNotFound(err) {
		// Not-found error, so trigger an OnDelete.
		r.log.WithField("name", request.Name).WithField("namespace", request.Namespace).Info("failed to find gateway")

		r.eventHandler.OnDelete(&gatewayapi_v1alpha1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: request.Namespace,
				Name:      request.Name,
			}})
		return reconcile.Result{}, nil
	} else if err != nil {
		// Error reading the object, so requeue the request.
		return reconcile.Result{}, fmt.Errorf("failed to get gateway %s/%s: %w", request.Namespace, request.Name, err)
	}

	// TODO: Ensure the gateway by creating manage infrastructure, i.e. the Envoy service.
	// xref: https://github.com/projectcontour/contour/issues/3545

	// Pass the gateway off to the eventHandler.
	r.eventHandler.OnAdd(gw)

	return reconcile.Result{}, nil
}
