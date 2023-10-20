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

	"github.com/projectcontour/contour/internal/k8s"
	"github.com/projectcontour/contour/internal/leadership"
	"github.com/projectcontour/contour/internal/status"
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
	gatewayapi_v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

type gatewayClassReconciler struct {
	client        client.Client
	eventHandler  cache.ResourceEventHandler
	statusUpdater k8s.StatusUpdater
	log           logrus.FieldLogger
	controller    gatewayapi_v1beta1.GatewayController
	eventSource   chan event.GenericEvent
}

// RegisterGatewayClassController creates the gatewayclass controller. The controller
// will be pre-configured to watch for cluster-scoped GatewayClass objects with
// a controller field that matches name.
func RegisterGatewayClassController(
	log logrus.FieldLogger,
	mgr manager.Manager,
	eventHandler cache.ResourceEventHandler,
	statusUpdater k8s.StatusUpdater,
	name string,
) (leadership.NeedLeaderElectionNotification, error) {
	r := &gatewayClassReconciler{
		client:        mgr.GetClient(),
		eventHandler:  eventHandler,
		statusUpdater: statusUpdater,
		log:           log,
		controller:    gatewayapi_v1beta1.GatewayController(name),
		// Set up a source.Channel that will trigger reconciles
		// for all GatewayClasses when this Contour process is
		// elected leader, to ensure that their statuses are up
		// to date.
		eventSource: make(chan event.GenericEvent),
	}

	c, err := controller.NewUnmanaged("gatewayclass-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return nil, err
	}
	if err := mgr.Add(&noLeaderElectionController{c}); err != nil {
		return nil, err
	}

	// Only enqueue GatewayClass objects that match name.
	if err := c.Watch(
		source.Kind(mgr.GetCache(), &gatewayapi_v1beta1.GatewayClass{}),
		&handler.EnqueueRequestForObject{},
		predicate.NewPredicateFuncs(r.hasMatchingController),
	); err != nil {
		return nil, err
	}

	if err := c.Watch(
		&source.Channel{Source: r.eventSource},
		&handler.EnqueueRequestForObject{},
		predicate.NewPredicateFuncs(r.hasMatchingController),
	); err != nil {
		return nil, err
	}

	return r, nil
}

func (r *gatewayClassReconciler) OnElectedLeader() {
	r.log.Info("elected leader, triggering reconciles for all gatewayclasses")

	var gatewayClasses gatewayapi_v1beta1.GatewayClassList
	if err := r.client.List(context.Background(), &gatewayClasses); err != nil {
		r.log.WithError(err).Error("error listing gatewayclasses")
		return
	}

	for i := range gatewayClasses.Items {
		r.eventSource <- event.GenericEvent{Object: &gatewayClasses.Items[i]}
	}
}

// hasMatchingController returns true if the provided object is a GatewayClass
// with a Spec.Controller string matching this Contour's controller string,
// or false otherwise.
func (r *gatewayClassReconciler) hasMatchingController(obj client.Object) bool {
	log := r.log.WithFields(logrus.Fields{
		"name": obj.GetName(),
	})

	gc, ok := obj.(*gatewayapi_v1beta1.GatewayClass)
	if !ok {
		log.Debugf("unexpected object type %T, bypassing reconciliation.", obj)
		return false
	}

	if gc.Spec.ControllerName == r.controller {
		log.Debug("enqueueing gatewayclass")
		return true
	}

	log.Debugf("controller is not %s; bypassing reconciliation", r.controller)
	return false
}

func (r *gatewayClassReconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	r.log.WithField("name", request.Name).Info("reconciling gatewayclass")

	var gatewayClasses gatewayapi_v1beta1.GatewayClassList
	if err := r.client.List(ctx, &gatewayClasses); err != nil {
		return reconcile.Result{}, fmt.Errorf("error listing gatewayclasses: %w", err)
	}

	var controlledClasses controlledClasses

	for i := range gatewayClasses.Items {
		// avoid loop pointer issues
		gc := gatewayClasses.Items[i]

		if gc.Spec.ControllerName != r.controller {
			// different controller, ignore.
			continue
		}

		controlledClasses.add(&gc)
	}

	// no controlled gatewayclasses, trigger a delete
	if controlledClasses.len() == 0 {
		r.log.WithField("name", request.Name).Info("failed to find gatewayclass")

		r.eventHandler.OnDelete(&gatewayapi_v1beta1.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: request.Namespace,
				Name:      request.Name,
			}})
		return reconcile.Result{}, nil
	}

	updater := func(gc *gatewayapi_v1beta1.GatewayClass, accepted bool) error {
		if r.statusUpdater != nil {
			r.statusUpdater.Send(k8s.StatusUpdate{
				NamespacedName: types.NamespacedName{Name: gc.Name},
				Resource:       &gatewayapi_v1beta1.GatewayClass{},
				Mutator: k8s.StatusMutatorFunc(func(obj client.Object) client.Object {
					gwc, ok := obj.(*gatewayapi_v1beta1.GatewayClass)
					if !ok {
						panic(fmt.Sprintf("unsupported object type %T", obj))
					}

					return status.SetGatewayClassAccepted(gwc.DeepCopy(), accepted)
				}),
			})
		} else {
			// this branch makes testing easier by not going through the StatusUpdater.
			gcCopy := status.SetGatewayClassAccepted(gc.DeepCopy(), accepted)

			if err := r.client.Status().Update(ctx, gcCopy); err != nil {
				return fmt.Errorf("error updating status of gateway class %s: %v", gcCopy.Name, err)
			}
		}
		return nil
	}

	for _, gc := range controlledClasses.notAcceptedClasses() {
		if err := updater(gc, false); err != nil {
			return reconcile.Result{}, err
		}
	}

	if err := updater(controlledClasses.acceptedClass(), true); err != nil {
		return reconcile.Result{}, err
	}

	r.eventHandler.OnAdd(controlledClasses.acceptedClass(), false)

	return reconcile.Result{}, nil
}

// controlledClasses helps organize a list of GatewayClasses
// with the same controller string.
type controlledClasses struct {
	allClasses  []*gatewayapi_v1beta1.GatewayClass
	oldestClass *gatewayapi_v1beta1.GatewayClass
}

func (cc *controlledClasses) len() int {
	return len(cc.allClasses)
}

func (cc *controlledClasses) add(class *gatewayapi_v1beta1.GatewayClass) {
	cc.allClasses = append(cc.allClasses, class)

	switch {
	case cc.oldestClass == nil:
		cc.oldestClass = class
	case class.CreationTimestamp.Time.Before(cc.oldestClass.CreationTimestamp.Time):
		cc.oldestClass = class
	case class.CreationTimestamp.Time.Equal(cc.oldestClass.CreationTimestamp.Time) && class.Name < cc.oldestClass.Name:
		// tie-breaker: first one in alphabetical order is considered oldest/accepted
		cc.oldestClass = class
	}
}

func (cc *controlledClasses) acceptedClass() *gatewayapi_v1beta1.GatewayClass {
	return cc.oldestClass
}

func (cc *controlledClasses) notAcceptedClasses() []*gatewayapi_v1beta1.GatewayClass {
	var res []*gatewayapi_v1beta1.GatewayClass
	for _, gc := range cc.allClasses {
		// skip the oldest one since it will be accepted.
		if gc.Name != cc.oldestClass.Name {
			res = append(res, gc)
		}
	}

	return res
}
