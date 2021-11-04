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
	"time"

	"github.com/projectcontour/contour/internal/k8s"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

type udpRouteReconciler struct {
	client                     client.Client
	statusUpdater              k8s.StatusUpdater
	gatewayClassControllerName gatewayapi_v1alpha2.GatewayController
	log                        logrus.FieldLogger
}

// NewUDPRouteController creates the udproute controller from mgr. The controller will be pre-configured
// to watch for UDPRoute objects across all namespaces.
func NewUDPRouteController(
	mgr manager.Manager,
	statusUpdater k8s.StatusUpdater,
	gatewayClassControllerName string,
	log logrus.FieldLogger) (controller.Controller, error) {
	r := &udpRouteReconciler{
		client:                     mgr.GetClient(),
		statusUpdater:              statusUpdater,
		gatewayClassControllerName: gatewayapi_v1alpha2.GatewayController(gatewayClassControllerName),
		log:                        log,
	}
	c, err := controller.NewUnmanaged("udproute-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return nil, err
	}
	if err := mgr.Add(&noLeaderElectionController{c}); err != nil {
		return nil, err
	}

	// Watch UDPRoutes to update their status whenever they
	// change.
	if err := c.Watch(
		&source.Kind{Type: &gatewayapi_v1alpha2.UDPRoute{}},
		&handler.EnqueueRequestForObject{},
	); err != nil {
		return nil, err
	}

	// Watch Gateways to update associated UDPRoutes' status
	// when Contour-controlled Gateways change. This allows
	// UDPRoutes' status to be updated properly if their
	// Gateway is created *after* the UDPRoute itself.
	if err := c.Watch(
		&source.Kind{Type: &gatewayapi_v1alpha2.Gateway{}},
		handler.EnqueueRequestsFromMapFunc(r.mapGatewayToUDPRoutes),
		predicate.NewPredicateFuncs(r.gatewayHasMatchingController),
	); err != nil {
		return nil, err
	}

	return c, nil
}

// mapGatewayToUDPRoutes returns a list of reconcile requests for all UDPRoutes
// that have a parentRef to the provided Gateway.
func (r *udpRouteReconciler) mapGatewayToUDPRoutes(gateway client.Object) []reconcile.Request {
	var udpRoutes gatewayapi_v1alpha2.UDPRouteList
	if err := r.client.List(context.Background(), &udpRoutes); err != nil {
		r.log.WithError(err).Error("error listing UDPRoutes")
		return nil
	}

	var reconciles []reconcile.Request
	for _, udpRoute := range udpRoutes.Items {
		for _, parentRef := range udpRoute.Spec.ParentRefs {
			if isRefToGateway(parentRef, k8s.NamespacedNameOf(gateway)) {
				reconciles = append(reconciles, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: udpRoute.Namespace,
						Name:      udpRoute.Name,
					},
				})
			}
		}
	}

	return reconciles
}

// hasMatchingController returns true if the provided object is a Gateway
// using a GatewayClass with a Spec.Controller string matching this Contour's
// controller string, or false otherwise.
func (r *udpRouteReconciler) gatewayHasMatchingController(obj client.Object) bool {
	log := r.log.WithFields(logrus.Fields{
		"namespace": obj.GetNamespace(),
		"name":      obj.GetName(),
	})

	return gatewayHasMatchingController(obj, r.gatewayClassControllerName, r.client, log)
}

// referencesContourGateway returns the first Gateway from the UDPRoute's
// ParentRefs with a GatewayClass controlled by this Contour, if one exists.
func (r *udpRouteReconciler) referencesContourGateway(obj client.Object) (*gatewayapi_v1alpha2.Gateway, bool) {
	log := r.log.WithFields(logrus.Fields{
		"namespace": obj.GetNamespace(),
		"name":      obj.GetName(),
	})

	route, ok := obj.(*gatewayapi_v1alpha2.UDPRoute)
	if !ok {
		log.Debugf("unexpected object type %T, bypassing reconciliation.", obj)
		return nil, false
	}

	return referencesContourGateway(
		route.Spec.ParentRefs,
		route.Namespace,
		r.gatewayClassControllerName,
		r.client,
		log,
	)
}

// Reconcile sets "Accepted: false" on any UDPRoutes that have a Gateway
// controlled by this Contour.
func (r *udpRouteReconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	log := r.log.WithField("namespace", request.Namespace).WithField("name", request.Name)

	udpRoute := &gatewayapi_v1alpha2.UDPRoute{}
	if err := r.client.Get(ctx, request.NamespacedName, udpRoute); err != nil {
		// not-found errors mean the UDPRoute has been deleted, OK
		// to ignore.
		if !errors.IsNotFound(err) {
			log.WithError(err).Error("error getting TCPRoute")
		}
		return reconcile.Result{}, nil
	}

	// Check if the UDPRoute is referencing a Gateway
	// controlled by this Contour.
	gw, ok := r.referencesContourGateway(udpRoute)
	if !ok {
		return reconcile.Result{}, nil
	}
	log.Info("reconciling UDPRoute")

	// If so, set the UDPRoute's status to indicate it's
	// an unsupported route type.
	r.statusUpdater.Send(k8s.StatusUpdate{
		NamespacedName: k8s.NamespacedNameOf(udpRoute),
		Resource:       &gatewayapi_v1alpha2.UDPRoute{},
		Mutator: k8s.StatusMutatorFunc(func(obj client.Object) client.Object {
			route, ok := obj.(*gatewayapi_v1alpha2.UDPRoute)
			if !ok {
				panic(fmt.Sprintf("unsupported object type %T", obj))
			}

			// Note that the statusUpdater will filter out no-op status updates,
			// so we don't have to worry here about sending updates that don't
			// actually change anything and triggering cascading reconciliations.
			return setUDPRouteNotAccepted(route.DeepCopy(), gw, r.gatewayClassControllerName)
		}),
	})

	return reconcile.Result{}, nil
}

func setUDPRouteNotAccepted(udpRoute *gatewayapi_v1alpha2.UDPRoute, gateway *gatewayapi_v1alpha2.Gateway, controllerName gatewayapi_v1alpha2.GatewayController) *gatewayapi_v1alpha2.UDPRoute {
	notAcceptedCondition := metav1.Condition{
		Type:               string(gatewayapi_v1alpha2.ConditionRouteAccepted),
		Status:             metav1.ConditionFalse,
		Reason:             "UnsupportedRouteType",
		Message:            "UDPRoutes are not supported by this Gateway",
		LastTransitionTime: metav1.NewTime(time.Now()),
		ObservedGeneration: udpRoute.Generation,
	}

	udpRoute.Status.Parents = setRouteCondition(
		udpRoute.Status.Parents,
		notAcceptedCondition,
		gateway,
		controllerName,
	)

	return udpRoute
}
