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

	"github.com/projectcontour/contour/internal/errors"
	"github.com/projectcontour/contour/internal/status"
	"github.com/projectcontour/contour/internal/validation"

	"github.com/sirupsen/logrus"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

type gatewayClassReconciler struct {
	client       client.Client
	eventHandler cache.ResourceEventHandler
	log          logrus.FieldLogger
	controller   string
}

// NewGatewayClassController creates the gatewayclass controller. The controller
// will be pre-configured to watch for cluster-scoped GatewayClass objects with
// a controller field that matches name.
func NewGatewayClassController(mgr manager.Manager, eventHandler cache.ResourceEventHandler, log logrus.FieldLogger, name string) (controller.Controller, error) {
	r := &gatewayClassReconciler{
		client:       mgr.GetClient(),
		eventHandler: eventHandler,
		log:          log,
		controller:   name,
	}

	c, err := controller.New("gatewayclass-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return nil, err
	}

	// Only enqueue GatewayClass objects that match name.
	if err := c.Watch(
		&source.Kind{Type: &gatewayapi_v1alpha1.GatewayClass{}},
		&handler.EnqueueRequestForObject{},
		predicate.NewPredicateFuncs(r.hasMatchingController),
	); err != nil {
		return nil, err
	}

	return c, nil
}

// hasMatchingController returns true if the provided object is a GatewayClass
// with a Spec.Controller string matching this Contour's controller string,
// or false otherwise.
func (r *gatewayClassReconciler) hasMatchingController(obj client.Object) bool {
	log := r.log.WithFields(logrus.Fields{
		"name": obj.GetName(),
	})

	gc, ok := obj.(*gatewayapi_v1alpha1.GatewayClass)
	if !ok {
		log.Info("invalid object, bypassing reconciliation.")
		return false
	}

	if gc.Spec.Controller == r.controller {
		log.Info("enqueueing gatewayclass")
		return true
	}

	log.Infof("controller is not %s; bypassing reconciliation", r.controller)
	return false
}

func (r *gatewayClassReconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	r.log.WithField("name", request.Name).Info("reconciling gatewayclass")

	// Fetch the Gateway from the cache.
	gc := &gatewayapi_v1alpha1.GatewayClass{}
	if err := r.client.Get(ctx, request.NamespacedName, gc); api_errors.IsNotFound(err) {
		// Not-found error, so trigger an OnDelete.
		r.log.WithField("name", request.Name).Info("failed to find gatewayclass")

		r.eventHandler.OnDelete(&gatewayapi_v1alpha1.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: request.Namespace,
				Name:      request.Name,
			}})
		return reconcile.Result{}, nil
	} else if err != nil {
		// Error reading the object, so requeue the request.
		return reconcile.Result{}, fmt.Errorf("failed to get gatewayclass %q: %w", request.Name, err)
	}

	// The gatewayclass is safe to process, so check if it's valid.
	errs := validation.ValidateGatewayClass(ctx, r.client, gc, r.controller)
	if errs != nil {
		r.log.WithField("name", gc.Name).Error("invalid gatewayclass: ", errors.ParseFieldErrors(errs))
	}

	// Pass the new changed object off to the eventHandler.
	r.eventHandler.OnAdd(gc)

	if err := status.SyncGatewayClass(ctx, r.client, gc, errs); err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to sync gatewayclass %q status: %w", gc.Name, err)
	}
	r.log.WithField("name", gc.Name).Info("synced gatewayclass status")

	return reconcile.Result{}, nil
}
