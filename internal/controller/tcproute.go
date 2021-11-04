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

type tcpRouteReconciler struct {
	client                     client.Client
	statusUpdater              k8s.StatusUpdater
	gatewayClassControllerName gatewayapi_v1alpha2.GatewayController
	log                        logrus.FieldLogger
}

// NewTCPRouteController creates the tcproute controller from mgr. The controller will be pre-configured
// to watch for TCPRoute objects across all namespaces.
func NewTCPRouteController(
	mgr manager.Manager,
	statusUpdater k8s.StatusUpdater,
	gatewayClassControllerName string,
	log logrus.FieldLogger) (controller.Controller, error) {
	r := &tcpRouteReconciler{
		client:                     mgr.GetClient(),
		statusUpdater:              statusUpdater,
		gatewayClassControllerName: gatewayapi_v1alpha2.GatewayController(gatewayClassControllerName),
		log:                        log,
	}
	c, err := controller.New("tcproute-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return nil, err
	}

	// Watch TCPRoutes to update their status whenever they
	// change.
	if err := c.Watch(
		&source.Kind{Type: &gatewayapi_v1alpha2.TCPRoute{}},
		&handler.EnqueueRequestForObject{},
	); err != nil {
		return nil, err
	}

	// Watch Gateways to update associated TCPRoutes' status
	// when Contour-controlled Gateways change. This allows
	// TCPRoutes' status to be updated properly if their
	// Gateway is created *after* the TCPRoute itself.
	if err := c.Watch(
		&source.Kind{Type: &gatewayapi_v1alpha2.Gateway{}},
		handler.EnqueueRequestsFromMapFunc(r.mapGatewayToTCPRoutes),
		predicate.NewPredicateFuncs(r.gatewayHasMatchingController),
	); err != nil {
		return nil, err
	}

	return c, nil
}

// mapGatewayToTCPRoutes returns a list of reconcile requests for all TCPRoutes
// that have a parentRef to the provided Gateway.
func (r *tcpRouteReconciler) mapGatewayToTCPRoutes(gateway client.Object) []reconcile.Request {
	var tcpRoutes gatewayapi_v1alpha2.TCPRouteList
	if err := r.client.List(context.Background(), &tcpRoutes); err != nil {
		r.log.WithError(err).Error("error listing TCPRoutes")
		return nil
	}

	var reconciles []reconcile.Request
	for _, tcpRoute := range tcpRoutes.Items {
		for _, parentRef := range tcpRoute.Spec.ParentRefs {
			if isRefToGateway(parentRef, k8s.NamespacedNameOf(gateway)) {
				reconciles = append(reconciles, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: tcpRoute.Namespace,
						Name:      tcpRoute.Name,
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
func (r *tcpRouteReconciler) gatewayHasMatchingController(obj client.Object) bool {
	log := r.log.WithFields(logrus.Fields{
		"namespace": obj.GetNamespace(),
		"name":      obj.GetName(),
	})

	return gatewayHasMatchingController(obj, r.gatewayClassControllerName, r.client, log)
}

// referencesContourGateway returns the first Gateway from the TCPRoute's
// ParentRefs with a GatewayClass controlled by this Contour, if one exists.
func (r *tcpRouteReconciler) referencesContourGateway(obj client.Object) (*gatewayapi_v1alpha2.Gateway, bool) {
	log := r.log.WithFields(logrus.Fields{
		"namespace": obj.GetNamespace(),
		"name":      obj.GetName(),
	})

	route, ok := obj.(*gatewayapi_v1alpha2.TCPRoute)
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

// Reconcile sets "Accepted: false" on any TCPRoutes that have a Gateway
// controlled by this Contour.
func (r *tcpRouteReconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	log := r.log.WithField("namespace", request.Namespace).WithField("name", request.Name)

	tcpRoute := &gatewayapi_v1alpha2.TCPRoute{}
	if err := r.client.Get(ctx, request.NamespacedName, tcpRoute); err != nil {
		// not-found errors mean the TCPRoute has been deleted, OK
		// to ignore.
		if !errors.IsNotFound(err) {
			log.WithError(err).Error("error getting TCPRoute")
		}
		return reconcile.Result{}, nil
	}

	// Check if the TCPRoute is referencing a Gateway
	// controlled by this Contour.
	gw, ok := r.referencesContourGateway(tcpRoute)
	if !ok {
		return reconcile.Result{}, nil
	}
	log.Info("reconciling TCPRoute")

	// If so, set the TCPRoute's status to indicate it's
	// an unsupported route type.
	r.statusUpdater.Send(k8s.StatusUpdate{
		NamespacedName: k8s.NamespacedNameOf(tcpRoute),
		Resource:       &gatewayapi_v1alpha2.TCPRoute{},
		Mutator: k8s.StatusMutatorFunc(func(obj client.Object) client.Object {
			route, ok := obj.(*gatewayapi_v1alpha2.TCPRoute)
			if !ok {
				panic(fmt.Sprintf("unsupported object type %T", obj))
			}

			// Note that the statusUpdater will filter out no-op status updates,
			// so we don't have to worry here about sending updates that don't
			// actually change anything and triggering cascading reconciliations.
			return setTCPRouteNotAccepted(route.DeepCopy(), gw, r.gatewayClassControllerName)
		}),
	})

	return reconcile.Result{}, nil
}

func setTCPRouteNotAccepted(tcpRoute *gatewayapi_v1alpha2.TCPRoute, gateway *gatewayapi_v1alpha2.Gateway, controllerName gatewayapi_v1alpha2.GatewayController) *gatewayapi_v1alpha2.TCPRoute {
	notAcceptedCondition := metav1.Condition{
		Type:               string(gatewayapi_v1alpha2.ConditionRouteAccepted),
		Status:             metav1.ConditionFalse,
		Reason:             "UnsupportedRouteType",
		Message:            "TCPRoutes are not supported by this Gateway",
		LastTransitionTime: metav1.NewTime(time.Now()),
		ObservedGeneration: tcpRoute.Generation,
	}

	tcpRoute.Status.Parents = setRouteCondition(
		tcpRoute.Status.Parents,
		notAcceptedCondition,
		gateway,
		controllerName,
	)

	return tcpRoute
}
