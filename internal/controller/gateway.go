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

	"github.com/projectcontour/contour/internal/slice"

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

const gatewayFinalizer = "gateway.projectcontour.io"

type gatewayReconciler struct {
	ctx          context.Context
	client       client.Client
	eventHandler cache.ResourceEventHandler
	log          logrus.FieldLogger
	// controllerName is the configured controller of managed gatewayclasses.
	controllerName string
	// referencedClass is the gatewayclass referenced by managed gateways.
	referencedClass *gatewayapi_v1alpha1.GatewayClass
}

// NewGatewayController creates the gateway controller from mgr. The controller will be pre-configured
// to watch for Gateway objects across all namespaces and reconcile those that match class.
func NewGatewayController(mgr manager.Manager, eventHandler cache.ResourceEventHandler, log logrus.FieldLogger, class string) (controller.Controller, error) {
	r := &gatewayReconciler{
		ctx:            context.Background(),
		client:         mgr.GetClient(),
		eventHandler:   eventHandler,
		log:            log,
		controllerName: class,
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
		r.referencedClass = gc
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

// classForGateway returns nil, error if the gatewayclass referenced by gw does not exist
// or is not owned by Contour. Otherwise, the referenced gatewayclass is returned.
func (r *gatewayReconciler) classForGateway(gw *gatewayapi_v1alpha1.Gateway) (*gatewayapi_v1alpha1.GatewayClass, error) {
	gc := &gatewayapi_v1alpha1.GatewayClass{}
	if err := r.client.Get(r.ctx, types.NamespacedName{Name: gw.Spec.GatewayClassName}, gc); err != nil {
		return nil, fmt.Errorf("failed to get gatewayclass %s: %w", gw.Spec.GatewayClassName, err)
	}
	if !r.matchesControllerName(gc) {
		return nil, fmt.Errorf("gatewayclass %q not owned by contour", gc.Name)
	}
	return gc, nil
}

// matchesControllerName returns true if the controller of the provided gc matches Contour's
// GatewayClass controller string.
func (r *gatewayReconciler) matchesControllerName(gc *gatewayapi_v1alpha1.GatewayClass) bool {
	return gc.Spec.Controller == r.controllerName
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
		if err := r.ensureGatewayDeleted(gw); err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to ensure deletion of gateway %s/%s: %w",
				gw.Namespace, gw.Name, err)
		}

		r.eventHandler.OnDelete(gw)

		return reconcile.Result{}, nil
	}

	// Finalize the gateway.
	if !r.isFinalized(gw) {
		if err := r.ensureFinalizer(gw); err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to finalize gateway %s/%s: %w",
				gw.Namespace, gw.Name, err)
		}
	}

	// If needed, finalize the referenced gatewayclass.
	if !r.isClassFinalized() {
		// Before doing anything with the gateway, ensure the reference gatewayclass is finalized.
		if err := r.ensureClassFinalizer(); err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to finalize gatewayclass %s: %w",
				r.referencedClass.Name, err)
		}
	}

	// TODO: Ensure the gateway by creating manage infrastructure, i.e. the Envoy service.
	// xref: https://github.com/projectcontour/contour/issues/3545

	// Pass the gateway off to the eventHandler.
	r.eventHandler.OnAdd(gw)

	return reconcile.Result{}, nil
}

// isFinalized returns true if gw is finalized.
func (r *gatewayReconciler) isFinalized(gw *gatewayapi_v1alpha1.Gateway) bool {
	for _, f := range gw.Finalizers {
		if f == gatewayFinalizer {
			return true
		}
	}
	return false
}

// ensureFinalizer ensures gw is finalized.
func (r *gatewayReconciler) ensureFinalizer(gw *gatewayapi_v1alpha1.Gateway) error {
	if !slice.ContainsString(gw.Finalizers, gatewayFinalizer) {
		gw.Finalizers = append(gw.Finalizers, gatewayFinalizer)
		if err := r.client.Update(r.ctx, gw); err != nil {
			return fmt.Errorf("failed to update %s/%s: %w", gw.Namespace, gw.Name, err)
		}
		r.log.WithField("namespace", gw.Namespace).WithField("name", gw.Name).Info("finalized gateway")
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

// ensureClassFinalizer ensures a finalizer is applied to the reconciler's referenced gatewayclass.
func (r *gatewayReconciler) ensureClassFinalizer() error {
	if !slice.ContainsString(r.referencedClass.Finalizers, gatewayClassFinalizer) {
		updated := r.referencedClass.DeepCopy()
		updated.Finalizers = append(updated.Finalizers, gatewayClassFinalizer)
		if err := r.client.Update(r.ctx, updated); err != nil {
			return fmt.Errorf("failed to update %s: %w", r.referencedClass.Name, err)
		}
		r.log.WithField("name", r.referencedClass.Name).Info("finalized gatewayclass")
		r.referencedClass = updated
	}
	return nil
}

// ensureGatewayDeleted ensures child resources and finalizers of gw have been deleted.
func (r *gatewayReconciler) ensureGatewayDeleted(gw *gatewayapi_v1alpha1.Gateway) error {
	// TODO: Remove managed Envoy infrastructure when introduced.
	// xref: https://github.com/projectcontour/contour/issues/3545

	// Remove the finalizer from the referenced gateway class, if needed.
	others, err := r.othersRefClass(gw)
	if err != nil {
		return fmt.Errorf("failed to verify if other gateways reference gatewayclass %s: %w",
			r.referencedClass.Name, err)
	}
	if !others {
		if err := r.EnsureClassFinalizerRemoved(); err != nil {
			return fmt.Errorf("failed to remove gatewayclass %s finalizer: %w", r.referencedClass.Name, err)
		}
	}

	// Remove the finalizer from the gateway.
	if err := r.EnsureFinalizerRemoved(gw); err != nil {
		return fmt.Errorf("failed to remove gateway %s/%s finalizer: %w", gw.Namespace, gw.Name, err)
	}

	return nil
}

// EnsureFinalizerRemoved ensures the finalizer is removed for the given gw.
func (r *gatewayReconciler) EnsureFinalizerRemoved(gw *gatewayapi_v1alpha1.Gateway) error {
	if slice.ContainsString(gw.Finalizers, gatewayFinalizer) {
		gw.Finalizers = slice.RemoveString(gw.Finalizers, gatewayFinalizer)
		if err := r.client.Update(r.ctx, gw); err != nil {
			return fmt.Errorf("failed to remove finalizer from gateway %s/%s: %w", gw.Namespace, gw.Name, err)
		}
		r.log.WithField("namespace", gw.Namespace).WithField("name", gw.Name).Info("removed gateway finalizer")
	}
	return nil
}

// EnsureClassFinalizerRemoved ensures the finalizer is removed for the referenced gatewayclass.
func (r *gatewayReconciler) EnsureClassFinalizerRemoved() error {
	if slice.ContainsString(r.referencedClass.Finalizers, gatewayClassFinalizer) {
		updated := r.referencedClass.DeepCopy()
		updated.Finalizers = slice.RemoveString(updated.Finalizers, gatewayClassFinalizer)
		if err := r.client.Update(r.ctx, updated); err != nil {
			return fmt.Errorf("failed to remove finalizer from gatewayclass %s: %w", r.referencedClass.Name, err)
		}
		r.log.WithField("name", r.referencedClass.Name).Info("removed gatewayclass finalizer")
		r.referencedClass = updated
	}
	return nil
}

// othersRefClass returns true if other gateways have the same gatewayClassName as gw.
func (r *gatewayReconciler) othersRefClass(gw *gatewayapi_v1alpha1.Gateway) (bool, error) {
	gwList, err := r.othersExist(gw)
	if err != nil {
		return false, err
	}
	if gwList != nil {
		for _, item := range gwList.Items {
			if item.Namespace != gw.Namespace &&
				item.Name != gw.Name &&
				item.Spec.GatewayClassName == gw.Spec.GatewayClassName {
				return true, nil
			}
		}
	}
	return false, nil
}

// othersExist lists Gateway objects in all namespaces, returning the list
// if any exist other than gw.
func (r *gatewayReconciler) othersExist(gw *gatewayapi_v1alpha1.Gateway) (*gatewayapi_v1alpha1.GatewayList, error) {
	gwList := &gatewayapi_v1alpha1.GatewayList{}
	if err := r.client.List(r.ctx, gwList); err != nil {
		return nil, fmt.Errorf("failed to list gateways: %w", err)
	}
	if len(gwList.Items) == 0 || len(gwList.Items) == 1 && gwList.Items[0].Name == gw.Name {
		return nil, nil
	}
	return gwList, nil
}
