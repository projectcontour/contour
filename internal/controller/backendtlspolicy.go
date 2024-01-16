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

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

type backendTLSPolicyReconciler struct {
	client       client.Client
	eventHandler cache.ResourceEventHandler
	logrus.FieldLogger
}

// RegisterBackendTLSPolicyController creates the backendtlspolicy controller from mgr. The controller will be pre-configured
// to watch for BackendTLSPolicy objects across all namespaces.
func RegisterBackendTLSPolicyController(log logrus.FieldLogger, mgr manager.Manager, eventHandler cache.ResourceEventHandler) error {
	r := &backendTLSPolicyReconciler{
		client:       mgr.GetClient(),
		eventHandler: eventHandler,
		FieldLogger:  log,
	}
	c, err := controller.NewUnmanaged("backendtlspolicy-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}
	if err := mgr.Add(&noLeaderElectionController{c}); err != nil {
		return err
	}

	return c.Watch(source.Kind(mgr.GetCache(), &gatewayapi_v1alpha2.BackendTLSPolicy{}), &handler.EnqueueRequestForObject{})
}

func (r *backendTLSPolicyReconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	// Fetch the BackendTLSPolicy from the cache.
	backendTLSPolicy := &gatewayapi_v1alpha2.BackendTLSPolicy{}
	err := r.client.Get(ctx, request.NamespacedName, backendTLSPolicy)
	if errors.IsNotFound(err) {
		r.eventHandler.OnDelete(&gatewayapi_v1alpha2.BackendTLSPolicy{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      request.Name,
				Namespace: request.Namespace,
			},
		})
		return reconcile.Result{}, nil
	}

	// Pass the new changed object off to the eventHandler.
	r.eventHandler.OnAdd(backendTLSPolicy, false)

	return reconcile.Result{}, nil
}
