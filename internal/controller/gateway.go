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
	"github.com/projectcontour/contour/internal/leadership"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

type gatewayReconciler struct {
	client        client.Client
	eventHandler  cache.ResourceEventHandler
	statusUpdater k8s.StatusUpdater
	log           logrus.FieldLogger
	// gatewayClassControllerName is the configured controller of managed gatewayclasses.
	gatewayClassControllerName gatewayapi_v1alpha2.GatewayController
	eventSource                chan event.GenericEvent
}

// RegisterGatewayController creates the gateway controller from mgr. The controller will be pre-configured
// to watch for Gateway objects across all namespaces and reconcile those that match class.
func RegisterGatewayController(
	log logrus.FieldLogger,
	mgr manager.Manager,
	eventHandler cache.ResourceEventHandler,
	statusUpdater k8s.StatusUpdater,
	gatewayClassControllerName string,
) (leadership.NeedLeaderElectionNotification, error) {
	r := &gatewayReconciler{
		log:                        log,
		client:                     mgr.GetClient(),
		eventHandler:               eventHandler,
		statusUpdater:              statusUpdater,
		gatewayClassControllerName: gatewayapi_v1alpha2.GatewayController(gatewayClassControllerName),
		// Set up a source.Channel that will trigger reconciles
		// for all GatewayClasses when this Contour process is
		// elected leader, to ensure that their statuses are up
		// to date.
		eventSource: make(chan event.GenericEvent),
	}
	c, err := controller.NewUnmanaged("gateway-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return nil, err
	}
	if err := mgr.Add(&noLeaderElectionController{c}); err != nil {
		return nil, err
	}

	if err := c.Watch(
		&source.Kind{Type: &gatewayapi_v1alpha2.Gateway{}},
		&handler.EnqueueRequestForObject{},
		predicate.NewPredicateFuncs(r.hasMatchingController),
	); err != nil {
		return nil, err
	}

	// Watch GatewayClasses and reconcile their associated Gateways
	// to handle changes in the GatewayClasses' "Accepted" conditions.
	if err := c.Watch(
		&source.Kind{Type: &gatewayapi_v1alpha2.GatewayClass{}},
		handler.EnqueueRequestsFromMapFunc(r.mapGatewayClassToGateways),
		predicate.NewPredicateFuncs(r.gatewayClassHasMatchingController),
	); err != nil {
		return nil, err
	}

	// Set up a source.Channel that will trigger reconciles
	// for all Gateways when this Contour process is
	// elected leader, to ensure that their statuses are up
	// to date.
	if err := c.Watch(
		&source.Channel{Source: r.eventSource},
		&handler.EnqueueRequestForObject{},
		predicate.NewPredicateFuncs(r.hasMatchingController),
	); err != nil {
		return nil, err
	}

	return r, nil
}

func (r *gatewayReconciler) OnElectedLeader() {
	r.log.Info("elected leader, triggering reconciles for all gateways")

	var gateways gatewayapi_v1alpha2.GatewayList
	if err := r.client.List(context.Background(), &gateways); err != nil {
		r.log.WithError(err).Error("error listing gateways")
		return
	}

	for i := range gateways.Items {
		r.eventSource <- event.GenericEvent{Object: &gateways.Items[i]}
	}
}

func (r *gatewayReconciler) mapGatewayClassToGateways(gatewayClass client.Object) []reconcile.Request {
	var gateways gatewayapi_v1alpha2.GatewayList
	if err := r.client.List(context.Background(), &gateways); err != nil {
		r.log.WithError(err).Error("error listing gateways")
		return nil
	}

	var reconciles []reconcile.Request
	for _, gw := range gateways.Items {
		if string(gw.Spec.GatewayClassName) == gatewayClass.GetName() {
			reconciles = append(reconciles, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: gw.Namespace,
					Name:      gw.Name,
				},
			})
		}
	}

	return reconciles
}

// hasMatchingController returns true if the provided object is a Gateway
// using a GatewayClass with a Spec.Controller string matching this Contour's
// controller string, or false otherwise.
func (r *gatewayReconciler) hasMatchingController(obj client.Object) bool {
	log := r.log.WithFields(logrus.Fields{
		"namespace": obj.GetNamespace(),
		"name":      obj.GetName(),
	})

	gw, ok := obj.(*gatewayapi_v1alpha2.Gateway)
	if !ok {
		log.Debugf("unexpected object type %T, bypassing reconciliation.", obj)
		return false
	}

	gc := &gatewayapi_v1alpha2.GatewayClass{}
	if err := r.client.Get(context.Background(), types.NamespacedName{Name: string(gw.Spec.GatewayClassName)}, gc); err != nil {
		log.WithError(err).Errorf("failed to get gatewayclass %s", gw.Spec.GatewayClassName)
		return false
	}
	if gc.Spec.ControllerName != r.gatewayClassControllerName {
		log.Debugf("gateway's class controller is not %s; bypassing reconciliation", r.gatewayClassControllerName)
		return false
	}

	return true
}

func (r *gatewayReconciler) gatewayClassHasMatchingController(obj client.Object) bool {
	gc, ok := obj.(*gatewayapi_v1alpha2.GatewayClass)
	if !ok {
		r.log.Infof("expected GatewayClass, got %T", obj)
		return false
	}

	return gc.Spec.ControllerName == r.gatewayClassControllerName
}

// Reconcile finds all the Gateways for the GatewayClass with an "Accepted: true" condition.
// It passes the oldest such Gateway to the DAG for processing, and sets a "Scheduled: false"
// condition on all other Gateways for the accepted GatewayClass.
func (r *gatewayReconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	r.log.WithField("namespace", request.Namespace).WithField("name", request.Name).Info("reconciling gateway")

	var gatewayClasses gatewayapi_v1alpha2.GatewayClassList
	if err := r.client.List(context.Background(), &gatewayClasses); err != nil {
		return reconcile.Result{}, fmt.Errorf("error listing gateway classes")
	}

	// Find the GatewayClass for this controller with Accepted=true.
	var acceptedGatewayClass *gatewayapi_v1alpha2.GatewayClass
	for i := range gatewayClasses.Items {
		gatewayClass := &gatewayClasses.Items[i]

		if gatewayClass.Spec.ControllerName != r.gatewayClassControllerName {
			continue
		}
		if !isAccepted(gatewayClass) {
			continue
		}

		acceptedGatewayClass = gatewayClass
		break
	}

	if acceptedGatewayClass == nil {
		r.log.Info("No accepted gateway class found")
		r.eventHandler.OnDelete(&gatewayapi_v1alpha2.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: request.Namespace,
				Name:      request.Name,
			}})
		return reconcile.Result{}, nil
	}

	var allGateways gatewayapi_v1alpha2.GatewayList
	if err := r.client.List(context.Background(), &allGateways); err != nil {
		return reconcile.Result{}, fmt.Errorf("error listing gateways")
	}

	// Get all the Gateways for the Accepted=true GatewayClass.
	var gatewaysForClass []*gatewayapi_v1alpha2.Gateway
	for i := range allGateways.Items {
		if string(allGateways.Items[i].Spec.GatewayClassName) == acceptedGatewayClass.Name {
			gatewaysForClass = append(gatewaysForClass, &allGateways.Items[i])
		}
	}

	if len(gatewaysForClass) == 0 {
		r.log.Info("No gateways found for accepted gateway class")
		r.eventHandler.OnDelete(&gatewayapi_v1alpha2.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: request.Namespace,
				Name:      request.Name,
			}})
		return reconcile.Result{}, nil
	}

	// Find the oldest Gateway, using alphabetical order
	// as a tiebreaker.
	var oldest *gatewayapi_v1alpha2.Gateway
	for _, gw := range gatewaysForClass {
		switch {
		case oldest == nil:
			oldest = gw
		case gw.CreationTimestamp.Before(&oldest.CreationTimestamp):
			oldest = gw
		case gw.CreationTimestamp.Equal(&oldest.CreationTimestamp):
			if fmt.Sprintf("%s/%s", gw.Namespace, gw.Name) < fmt.Sprintf("%s/%s", oldest.Namespace, oldest.Name) {
				oldest = gw
			}
		}
	}

	// Set the "Scheduled" condition to false for all gateways
	// except the oldest. The oldest will have its status set
	// by the DAG processor, so don't set it here.
	for _, gw := range gatewaysForClass {
		if gw == oldest {
			continue
		}

		if r.statusUpdater != nil {
			r.statusUpdater.Send(k8s.StatusUpdate{
				NamespacedName: k8s.NamespacedNameOf(gw),
				Resource:       &gatewayapi_v1alpha2.Gateway{},
				Mutator: k8s.StatusMutatorFunc(func(obj client.Object) client.Object {
					gw, ok := obj.(*gatewayapi_v1alpha2.Gateway)
					if !ok {
						panic(fmt.Sprintf("unsupported object type %T", obj))
					}

					return setGatewayNotScheduled(gw.DeepCopy())
				}),
			})
		} else {
			// this branch makes testing easier by not going through the StatusUpdater.
			copy := setGatewayNotScheduled(gw.DeepCopy())
			if err := r.client.Status().Update(context.Background(), copy); err != nil {
				r.log.WithError(err).Error("error updating gateway status")
				return reconcile.Result{}, fmt.Errorf("error updating status of gateway %s/%s: %v", gw.Namespace, gw.Name, err)
			}
		}
	}

	// TODO: Ensure the gateway by creating manage infrastructure, i.e. the Envoy service.
	// xref: https://github.com/projectcontour/contour/issues/3545

	r.log.WithField("namespace", oldest.Namespace).WithField("name", oldest.Name).Info("assigning gateway to DAG")
	r.eventHandler.OnAdd(oldest)
	return reconcile.Result{}, nil
}

func isAccepted(gatewayClass *gatewayapi_v1alpha2.GatewayClass) bool {
	for _, cond := range gatewayClass.Status.Conditions {
		if cond.Type == string(gatewayapi_v1alpha2.GatewayClassConditionStatusAccepted) && cond.Status == metav1.ConditionTrue {
			return true
		}
	}

	return false
}

func setGatewayNotScheduled(gateway *gatewayapi_v1alpha2.Gateway) *gatewayapi_v1alpha2.Gateway {
	newCond := metav1.Condition{
		Type:               "Scheduled",
		Status:             metav1.ConditionFalse,
		Reason:             "OlderGatewayExists",
		Message:            "An older Gateway exists for the accepted GatewayClass",
		LastTransitionTime: metav1.NewTime(time.Now()),
		ObservedGeneration: gateway.Generation,
	}

	for i := range gateway.Status.Conditions {
		cond := &gateway.Status.Conditions[i]

		if cond.Type != "Scheduled" {
			continue
		}

		// Update only if something has changed.
		if cond.Status != newCond.Status || cond.Reason != newCond.Reason || cond.Message != newCond.Message {
			cond.Status = newCond.Status
			cond.Reason = newCond.Reason
			cond.Message = newCond.Message
			cond.LastTransitionTime = newCond.LastTransitionTime
			cond.ObservedGeneration = newCond.ObservedGeneration
		}

		return gateway
	}

	gateway.Status.Conditions = append(gateway.Status.Conditions, newCond)
	return gateway
}
