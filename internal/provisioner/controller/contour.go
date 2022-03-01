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

	operatorv1alpha1 "github.com/projectcontour/contour/internal/provisioner/api"
	objcontour "github.com/projectcontour/contour/internal/provisioner/objects/contour"
	retryable "github.com/projectcontour/contour/internal/provisioner/retryableerror"
	"github.com/projectcontour/contour/internal/provisioner/status"
	"github.com/projectcontour/contour/internal/provisioner/validation"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// reconciler reconciles a Contour object.
type reconciler struct {
	client  client.Client
	ensurer *ensurer
	log     logr.Logger
}

// NewContourController creates the contour controller from mgr and cfg. The controller will be pre-configured
// to watch for Contour objects across all namespaces.
func NewContourController(mgr manager.Manager, contourImage, envoyImage string) (controller.Controller, error) {
	r := &reconciler{
		client: mgr.GetClient(),
		log:    ctrl.Log.WithName("contour-controller"),
	}

	r.ensurer = &ensurer{
		log:          r.log,
		client:       r.client,
		contourImage: contourImage,
		envoyImage:   envoyImage,
	}

	c, err := controller.New("contour-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return nil, err
	}
	if err := c.Watch(&source.Kind{Type: &operatorv1alpha1.Contour{}}, &handler.EnqueueRequestForObject{}); err != nil {
		return nil, err
	}
	// Watch the Contour deployment and Envoy daemonset to properly surface Contour status conditions.
	if err := c.Watch(&source.Kind{Type: &appsv1.Deployment{}}, r.enqueueRequestForOwningContour()); err != nil {
		return nil, err
	}
	if err := c.Watch(&source.Kind{Type: &appsv1.DaemonSet{}}, r.enqueueRequestForOwningContour()); err != nil {
		return nil, err
	}
	return c, nil
}

// enqueueRequestForOwningContour returns an event handler that maps events to
// objects containing Contour owner labels.
func (r *reconciler) enqueueRequestForOwningContour() handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(a client.Object) []reconcile.Request {
		labels := a.GetLabels()
		ns, nsFound := labels[operatorv1alpha1.OwningContourNsLabel]
		name, nameFound := labels[operatorv1alpha1.OwningContourNameLabel]
		if nsFound && nameFound {
			r.log.Info("queueing contour", "namespace", ns, "name", name, "related", a.GetSelfLink())
			return []reconcile.Request{
				{
					NamespacedName: types.NamespacedName{
						Namespace: ns,
						Name:      name,
					},
				},
			}
		}
		return []reconcile.Request{}
	})
}

// Reconcile reconciles watched objects and attempts to make the current state of
// the object match the desired state.
func (r *reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = r.log.WithValues("contour", req.NamespacedName)
	r.log.Info("reconciling", "request", req)
	// Only proceed if we can get the state of contour.
	contour := &operatorv1alpha1.Contour{}
	if err := r.client.Get(ctx, req.NamespacedName, contour); err != nil {
		if errors.IsNotFound(err) {
			// This means the contour was already deleted/finalized and there are
			// stale queue entries (or something edge triggering from a related
			// resource that got deleted async).
			r.log.Info("contour not found; reconciliation will be skipped", "request", req)
			return ctrl.Result{}, nil
		}
		// Error reading the object, so requeue the request.
		return ctrl.Result{}, fmt.Errorf("failed to get contour %s: %w", req, err)
	}
	// The contour is safe to process, so ensure current state matches desired state.
	desired := contour.ObjectMeta.DeletionTimestamp.IsZero()
	if desired {
		if err := validation.Contour(ctx, r.client, contour); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to validate contour %s/%s: %w", contour.Namespace, contour.Name, err)
		}
		if !contour.IsFinalized() {
			// Before doing anything with the contour, ensure it has a finalizer
			// so it can cleaned-up later.
			if err := objcontour.EnsureFinalizer(ctx, r.client, contour); err != nil {
				return ctrl.Result{}, err
			}
			r.log.Info("finalized contour", "namespace", contour.Namespace, "name", contour.Name)
		} else {
			r.log.Info("contour finalized", "namespace", contour.Namespace, "name", contour.Name)
			if err := r.ensureContour(ctx, contour); err != nil {
				switch e := err.(type) {
				case retryable.Error:
					r.log.Error(e, "got retryable error; requeueing", "after", e.After())
					return ctrl.Result{RequeueAfter: e.After()}, nil
				default:
					return ctrl.Result{}, err
				}
			}
			r.log.Info("ensured contour", "namespace", contour.Namespace, "name", contour.Name)
		}
	} else {
		if err := r.ensureContourDeleted(ctx, contour); err != nil {
			switch e := err.(type) {
			case retryable.Error:
				r.log.Error(e, "got retryable error; requeueing", "after", e.After())
				return ctrl.Result{RequeueAfter: e.After()}, nil
			default:
				return ctrl.Result{}, err
			}
		}
		r.log.Info("deleted contour", "namespace", contour.Namespace, "name", contour.Name)
	}
	return ctrl.Result{}, nil
}

// ensureContour ensures all necessary resources exist for the given contour.
func (r *reconciler) ensureContour(ctx context.Context, contour *operatorv1alpha1.Contour) error {
	errs := r.ensurer.ensureContour(ctx, contour, nil)

	if err := status.SyncContour(ctx, r.client, contour); err != nil {
		errs = append(errs, fmt.Errorf("failed to sync status for contour %s/%s: %w", contour.Namespace, contour.Name, err))
	} else {
		r.log.Info("synced status for contour", "namespace", contour.Namespace, "name", contour.Name)
	}

	return retryable.NewMaybeRetryableAggregate(errs)
}

// ensureContourDeleted ensures contour and all child resources have been deleted.
func (r *reconciler) ensureContourDeleted(ctx context.Context, contour *operatorv1alpha1.Contour) error {
	if errs := r.ensurer.ensureContourDeleted(ctx, contour); len(errs) > 0 {
		return utilerrors.NewAggregate(errs)
	}

	if err := objcontour.EnsureFinalizerRemoved(ctx, r.client, contour); err != nil {
		return fmt.Errorf("failed to remove finalizer from contour %s/%s: %w", contour.Namespace, contour.Name, err)
	}

	r.log.Info("removed finalizer from contour", "namespace", contour.Namespace, "name", contour.Name)
	return nil
}
