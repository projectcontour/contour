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
	"github.com/projectcontour/contour/internal/status"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
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
	gatewayapi_v1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

var gatewayClassGVR = schema.GroupVersionResource{
	Group:    gatewayapi_v1alpha1.GroupVersion.Group,
	Version:  gatewayapi_v1alpha1.GroupVersion.Version,
	Resource: "gatewayclasses",
}

type gatewayClassReconciler struct {
	client        client.Client
	eventHandler  cache.ResourceEventHandler
	statusUpdater k8s.StatusUpdater
	log           logrus.FieldLogger
	controller    string
}

// NewGatewayClassController creates the gatewayclass controller. The controller
// will be pre-configured to watch for cluster-scoped GatewayClass objects with
// a controller field that matches name.
func NewGatewayClassController(
	mgr manager.Manager,
	eventHandler cache.ResourceEventHandler,
	statusUpdater k8s.StatusUpdater,
	log logrus.FieldLogger,
	name string,
	isLeader <-chan struct{},
) (controller.Controller, error) {
	r := &gatewayClassReconciler{
		client:        mgr.GetClient(),
		eventHandler:  eventHandler,
		statusUpdater: statusUpdater,
		log:           log,
		controller:    name,
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

	// Set up a source.Channel that will trigger reconciles
	// for all GatewayClasses when this Contour process is
	// elected leader, to ensure that their statuses are up
	// to date.
	eventSource := make(chan event.GenericEvent)
	go func() {
		<-isLeader
		log.Info("elected leader, triggering reconciles for all gatewayclasses")

		var gatewayClasses gatewayapi_v1alpha1.GatewayClassList
		if err := r.client.List(context.Background(), &gatewayClasses); err != nil {
			log.WithError(err).Error("error listing gatewayclasses")
			return
		}

		for i := range gatewayClasses.Items {
			eventSource <- event.GenericEvent{Object: &gatewayClasses.Items[i]}
		}
	}()

	if err := c.Watch(
		&source.Channel{Source: eventSource},
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
		log.Debugf("unexpected object type %T, bypassing reconciliation.", obj)
		return false
	}

	if gc.Spec.Controller == r.controller {
		log.Debug("enqueueing gatewayclass")
		return true
	}

	log.Debugf("controller is not %s; bypassing reconciliation", r.controller)
	return false
}

func (r *gatewayClassReconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	r.log.WithField("name", request.Name).Info("reconciling gatewayclass")

	var gatewayClasses gatewayapi_v1alpha1.GatewayClassList
	if err := r.client.List(context.Background(), &gatewayClasses); err != nil {
		return reconcile.Result{}, fmt.Errorf("error listing gatewayclasses: %w", err)
	}

	var controlledClasses controlledClasses

	for i := range gatewayClasses.Items {
		// avoid loop pointer issues
		gc := gatewayClasses.Items[i]

		if gc.Spec.Controller != r.controller {
			// different controller, ignore.
			continue
		}

		controlledClasses.add(&gc)
	}

	// no controlled gatewayclasses, trigger a delete
	if controlledClasses.len() == 0 {
		r.log.WithField("name", request.Name).Info("failed to find gatewayclass")

		r.eventHandler.OnDelete(&gatewayapi_v1alpha1.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: request.Namespace,
				Name:      request.Name,
			}})
		return reconcile.Result{}, nil
	}

	for _, gc := range controlledClasses.notAdmittedClasses() {
		if r.statusUpdater != nil {
			r.statusUpdater.Send(k8s.StatusUpdate{
				NamespacedName: types.NamespacedName{Name: gc.Name},
				Resource:       gatewayClassGVR,
				Mutator: k8s.StatusMutatorFunc(func(obj interface{}) interface{} {
					gc, ok := obj.(*gatewayapi_v1alpha1.GatewayClass)
					if !ok {
						panic(fmt.Sprintf("unsupported object type %T", obj))
					}

					copy := gc.DeepCopy()
					return status.SetGatewayClassAdmitted(context.Background(), r.client, copy, false)
				}),
			})
		} else {
			// this branch makes testing easier by not going through the StatusUpdater.
			copy := status.SetGatewayClassAdmitted(context.Background(), r.client, gc.DeepCopy(), false)

			if err := r.client.Status().Update(context.Background(), copy); err != nil {
				return reconcile.Result{}, fmt.Errorf("error updating status of gateway class %s: %v", copy.Name, err)
			}
		}
	}

	if r.statusUpdater != nil {
		r.statusUpdater.Send(k8s.StatusUpdate{
			NamespacedName: types.NamespacedName{Name: controlledClasses.admittedClass().Name},
			Resource:       gatewayClassGVR,
			Mutator: k8s.StatusMutatorFunc(func(obj interface{}) interface{} {
				gc, ok := obj.(*gatewayapi_v1alpha1.GatewayClass)
				if !ok {
					panic(fmt.Sprintf("unsupported object type %T", obj))
				}

				return status.SetGatewayClassAdmitted(context.Background(), r.client, gc.DeepCopy(), true)
			}),
		})
	} else {
		// this branch makes testing easier by not going through the StatusUpdater.
		copy := status.SetGatewayClassAdmitted(context.Background(), r.client, controlledClasses.admittedClass().DeepCopy(), true)
		if err := r.client.Status().Update(context.Background(), copy); err != nil {
			return reconcile.Result{}, fmt.Errorf("error updating status of gateway class %s: %v", copy.Name, err)
		}
	}

	r.eventHandler.OnAdd(controlledClasses.admittedClass())

	return reconcile.Result{}, nil
}

// controlledClasses helps organize a list of GatewayClasses
// with the same controller string.
type controlledClasses struct {
	allClasses  []*gatewayapi_v1alpha1.GatewayClass
	oldestClass *gatewayapi_v1alpha1.GatewayClass
}

func (cc *controlledClasses) len() int {
	return len(cc.allClasses)
}

func (cc *controlledClasses) add(class *gatewayapi_v1alpha1.GatewayClass) {
	cc.allClasses = append(cc.allClasses, class)

	switch {
	case cc.oldestClass == nil:
		cc.oldestClass = class
	case class.CreationTimestamp.Time.Before(cc.oldestClass.CreationTimestamp.Time):
		cc.oldestClass = class
	case class.CreationTimestamp.Time.Equal(cc.oldestClass.CreationTimestamp.Time) && class.Name < cc.oldestClass.Name:
		// tie-breaker: first one in alphabetical order is considered oldest/admitted
		cc.oldestClass = class
	}
}

func (cc *controlledClasses) admittedClass() *gatewayapi_v1alpha1.GatewayClass {
	return cc.oldestClass
}

func (cc *controlledClasses) notAdmittedClasses() []*gatewayapi_v1alpha1.GatewayClass {
	var res []*gatewayapi_v1alpha1.GatewayClass
	for _, gc := range cc.allClasses {
		// skip the oldest one since it will be admitted.
		if gc.Name != cc.oldestClass.Name {
			res = append(res, gc)
		}
	}

	return res
}
