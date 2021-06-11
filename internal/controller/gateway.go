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

	internal_errors "github.com/projectcontour/contour/internal/errors"
	"github.com/projectcontour/contour/internal/slice"
	"github.com/projectcontour/contour/internal/status"
	"github.com/projectcontour/contour/internal/validation"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	gatewayapi_v1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

const finalizer = "gateway.projectcontour.io/finalizer"

type gatewayReconciler struct {
	ctx             context.Context
	client          client.Client
	eventHandler    cache.ResourceEventHandler
	log             logrus.FieldLogger
	classController string
}

// NewGatewayController creates the gateway controller from mgr. The controller will be pre-configured
// to watch for Gateway objects across all namespaces and reconcile those that match class.
func NewGatewayController(mgr manager.Manager, eventHandler cache.ResourceEventHandler, log logrus.FieldLogger, class string) (controller.Controller, error) {
	r := &gatewayReconciler{
		ctx:             context.Background(),
		client:          mgr.GetClient(),
		eventHandler:    eventHandler,
		log:             log,
		classController: class,
	}
	c, err := controller.New("gateway-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return nil, err
	}
	if err := c.Watch(&source.Kind{Type: &gatewayapi_v1alpha1.Gateway{}}, r.enqueueRequestForOwnedGateway()); err != nil {
		return nil, err
	}
	return c, nil
}

// enqueueRequestForOwnedGateway returns an event handler that maps events to
// Gateway objects that reference a GatewayClass owned by Contour.
func (r *gatewayReconciler) enqueueRequestForOwnedGateway() handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(a client.Object) []reconcile.Request {
		gw, ok := a.(*gatewayapi_v1alpha1.Gateway)
		if !ok {
			r.log.WithField("name", a.GetName()).WithField("namespace", a.GetNamespace()).Info("invalid object, bypassing reconciliation.")
			return []reconcile.Request{}
		}
		if err := r.classForGateway(gw); err != nil {
			r.log.WithField("namespace", gw.Namespace).WithField("name", gw.Name).Info(err, ", bypassing reconciliation")
			return []reconcile.Request{}
		}
		// The gateway references a gatewayclass that exists and is managed
		// by Contour, so enqueue it for reconciliation.
		r.log.WithField("namespace", gw.Namespace).WithField("name", gw.Name).Info("queueing gateway")
		return []reconcile.Request{
			{
				NamespacedName: types.NamespacedName{
					Namespace: gw.Namespace,
					Name:      gw.Name,
				},
			},
		}
	})
}

// classForGateway returns an error if gw does not exist or is not owned by Contour.
func (r *gatewayReconciler) classForGateway(gw *gatewayapi_v1alpha1.Gateway) error {
	gc := &gatewayapi_v1alpha1.GatewayClass{}
	if err := r.client.Get(r.ctx, types.NamespacedName{Name: gw.Spec.GatewayClassName}, gc); err != nil {
		return fmt.Errorf("failed to get gatewayclass %s: %w", gw.Spec.GatewayClassName, err)
	}
	if !isController(gc, r.classController) {
		return fmt.Errorf("gatewayclass %s not owned by contour", gw.Spec.GatewayClassName)
	}
	return nil
}

// isController returns true if the controller of the provided gc matches Contour's
// GatewayClass controller string.
func isController(gc *gatewayapi_v1alpha1.GatewayClass, controller string) bool {
	return gc.Spec.Controller == controller
}

func (r *gatewayReconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	r.log.WithField("namespace", request.Namespace).WithField("name", request.Name).Info("reconciling gateway")

	// Fetch the Gateway from the cache.
	gw := &gatewayapi_v1alpha1.Gateway{}
	if err := r.client.Get(ctx, request.NamespacedName, gw); err != nil {
		if errors.IsNotFound(err) {
			r.log.WithField("name", request.Name).WithField("namespace", request.Namespace).Info("failed to find gateway")
			return reconcile.Result{}, nil
		}
		// Error reading the object, so requeue the request.
		return reconcile.Result{}, fmt.Errorf("failed to get gateway %s/%s: %w", request.Namespace, request.Name, err)
	}

	// Check if object is deleted.
	if !gw.ObjectMeta.DeletionTimestamp.IsZero() {
		r.eventHandler.OnDelete(gw)
		// TODO: Add a method to remove gateway sub-resources and finalizer.
		return reconcile.Result{}, nil
	}

	// The gateway is safe to process, so check if it's valid.
	errs := validation.ValidateGateway(ctx, r.client, gw)
	if errs != nil {
		r.log.WithField("name", request.Name).WithField("namespace", request.Namespace).
			Error("invalid gateway: ", internal_errors.ParseFieldErrors(errs))
	} else {
		// The gw is valid so finalize and ensure it.
		if !isFinalized(gw) {
			// Before doing anything with the gateway, ensure it has a finalizer so it can cleaned-up later.
			if err := ensureFinalizer(ctx, r.client, gw); err != nil {
				return reconcile.Result{}, fmt.Errorf("failed to finalize gateway %s/%s: %w", gw.Namespace, gw.Name, err)
			}
			r.log.WithField("name", request.Name).WithField("namespace", request.Namespace).Info("finalized gateway")
			// The gateway has been mutated, so get the latest.
			if err := r.client.Get(ctx, request.NamespacedName, gw); err != nil {
				return reconcile.Result{}, fmt.Errorf("failed to get gateway %s/%s: %w", request.Namespace, request.Name, err)
			}
		}
		// Pass the gateway off to the eventHandler.
		r.eventHandler.OnAdd(gw)

		// TODO: Ensure the gateway by creating manage infrastructure, i.e. the Envoy service.
	}

	if err := status.SyncGateway(ctx, r.client, gw, errs); err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to sync gateway %s/%s status: %w", gw.Namespace, gw.Name, err)
	}
	r.log.WithField("name", gw.Name).WithField("namespace", gw.Namespace).Info("synced gateway status")

	return reconcile.Result{}, nil
}

// isFinalized returns true if gw is finalized.
func isFinalized(gw *gatewayapi_v1alpha1.Gateway) bool {
	for _, f := range gw.Finalizers {
		if f == finalizer {
			return true
		}
	}
	return false
}

// ensureFinalizer ensures the finalizer is added to the given gw.
func ensureFinalizer(ctx context.Context, cli client.Client, gw *gatewayapi_v1alpha1.Gateway) error {
	if !slice.ContainsString(gw.Finalizers, finalizer) {
		updated := gw.DeepCopy()
		updated.Finalizers = append(updated.Finalizers, finalizer)
		if err := cli.Update(ctx, updated); err != nil {
			return fmt.Errorf("failed to add finalizer %s: %w", finalizer, err)
		}
	}
	return nil
}
