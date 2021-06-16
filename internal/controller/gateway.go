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

	int_errors "github.com/projectcontour/contour/internal/errors"
	"github.com/projectcontour/contour/internal/gateway"
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

type gatewayReconciler struct {
	ctx          context.Context
	client       client.Client
	eventHandler cache.ResourceEventHandler
	log          logrus.FieldLogger
	// classController is the configured controller of managed gatewayclasses.
	classController string
	// referencedClass is the gatewayclass referenced by managed gateways.
	referencedClass *gatewayapi_v1alpha1.GatewayClass
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
			r.log.WithField("name", a.GetName()).WithField("namespace", a.GetNamespace()).
				Info("invalid object, bypassing reconciliation.")
			return []reconcile.Request{}
		}
		gc, err := r.classForGateway(gw)
		if err != nil {
			r.log.WithField("namespace", gw.Namespace).WithField("name", gw.Name).Error(err)
			return []reconcile.Request{}
		}
		if gc != nil {
			r.referencedClass = gc
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
		}
		r.log.WithField("name", a.GetName()).WithField("namespace", a.GetNamespace()).
			Info("gateway not owned by contour, bypassing reconciliation.")
		return []reconcile.Request{}
	})
}

// classForGateway returns an nil, error if the gatewayclass referenced by gw does not exist
// or nil, nil if gw is not owned by Contour. Otherwise, the referenced gatewayclass is returned.
func (r *gatewayReconciler) classForGateway(gw *gatewayapi_v1alpha1.Gateway) (*gatewayapi_v1alpha1.GatewayClass, error) {
	gc := &gatewayapi_v1alpha1.GatewayClass{}
	if err := r.client.Get(r.ctx, types.NamespacedName{Name: gw.Spec.GatewayClassName}, gc); err != nil {
		return nil, fmt.Errorf("failed to get gatewayclass %s: %w", gw.Spec.GatewayClassName, err)
	}
	if !r.isController(gc) {
		return nil, nil
	}
	return gc, nil
}

// isController returns true if the controller of the provided gc matches Contour's
// GatewayClass controller string.
func (r *gatewayReconciler) isController(gc *gatewayapi_v1alpha1.GatewayClass) bool {
	return gc.Spec.Controller == r.classController
}

func (r *gatewayReconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	r.log.WithField("namespace", request.Namespace).WithField("name", request.Name).Info("reconciling gateway")

	// Fetch the Gateway.
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
		if err := r.ensureGatewayDeleted(gw); err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to ensure deletion of gateway %s/%s: %w",
				gw.Namespace, gw.Name, err)
		}
		return reconcile.Result{}, nil
	}

	// The gateway is safe to process, so check if it's valid.
	errs := validation.ValidateGateway(ctx, r.client, gw)
	if errs != nil {
		r.log.WithField("name", request.Name).WithField("namespace", request.Namespace).
			Error("invalid gateway: ", int_errors.ParseFieldErrors(errs))
	} else {
		// TODO: Finalize the gateway when managed infrastructure is added.

		// If needed, finalize the referenced gatewayclass.
		if !r.isClassFinalized() {
			if err := r.ensureClassFinalizer(); err != nil {
				return reconcile.Result{}, fmt.Errorf("failed to finalize gatewayclass %s: %w",
					r.referencedClass.Name, err)
			}
			r.log.WithField("name", r.referencedClass.Name).Info("finalized gatewayclass")
			// The gatewayclass has been mutated, so get the latest.
			key := types.NamespacedName{
				Namespace: r.referencedClass.Namespace,
				Name:      r.referencedClass.Name,
			}
			if err := r.client.Get(ctx, key, r.referencedClass); err != nil {
				return reconcile.Result{}, fmt.Errorf("failed to get gatewayclass %s: %w",
					r.referencedClass.Name, err)
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

// ensureGatewayDeleted ensures child resources and finalizers of gw have been deleted.
func (r *gatewayReconciler) ensureGatewayDeleted(gw *gatewayapi_v1alpha1.Gateway) error {
	// TODO: Remove managed Envoy infrastructure when introduced.

	others, err := gateway.OthersRefClass(r.ctx, r.client, gw)
	if err != nil {
		return fmt.Errorf("failed to verify if other gateways reference gatewayclass %s: %w",
			r.referencedClass.Name, err)
	}
	if !others {
		if err := r.EnsureClassFinalizerRemoved(); err != nil {
			return fmt.Errorf("failed to remove finalizer from gatewayclass %s: %w", r.referencedClass.Name, err)
		}
		r.log.Info("removed finalizer from gatewayclass", "name", r.referencedClass.Name)
	}
	return nil
}

// isClassFinalized returns true if gc is finalized.
func (r *gatewayReconciler) isClassFinalized() bool {
	for _, f := range r.referencedClass.Finalizers {
		if f == gatewayClassFinalizer {
			return true
		}
	}
	return false
}

// ensureClassFinalizer ensures the finalizer is added to the referenced gatewayclass.
func (r *gatewayReconciler) ensureClassFinalizer() error {
	if !slice.ContainsString(r.referencedClass.Finalizers, gatewayClassFinalizer) {
		updated := r.referencedClass.DeepCopy()
		updated.Finalizers = append(updated.Finalizers, gatewayClassFinalizer)
		if err := r.client.Update(r.ctx, updated); err != nil {
			return fmt.Errorf("failed to add finalizer %s: %w", gatewayClassFinalizer, err)
		}
	}
	return nil
}

// EnsureClassFinalizerRemoved ensures the finalizer is removed for the given gc.
func (r *gatewayReconciler) EnsureClassFinalizerRemoved() error {
	if slice.ContainsString(r.referencedClass.Finalizers, gatewayClassFinalizer) {
		updated := r.referencedClass.DeepCopy()
		updated.Finalizers = slice.RemoveString(updated.Finalizers, gatewayClassFinalizer)
		if err := r.client.Update(r.ctx, updated); err != nil {
			return fmt.Errorf("failed to remove finalizer %s: %w", gatewayClassFinalizer, err)
		}
	}
	return nil
}
