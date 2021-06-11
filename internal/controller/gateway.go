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
		r.eventHandler.OnDelete(gw)
		return reconcile.Result{}, nil
	}

	// TODO: Ensure the gateway by creating manage infrastructure, i.e. the Envoy service.
	// xref: https://github.com/projectcontour/contour/issues/3545

	// Pass the gateway off to the eventHandler.
	r.eventHandler.OnAdd(gw)

	return reconcile.Result{}, nil
}
