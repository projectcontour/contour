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

	operatorv1alpha1 "github.com/projectcontour/contour-operator/api/v1alpha1"
	objutil "github.com/projectcontour/contour-operator/internal/objects"
	objcm "github.com/projectcontour/contour-operator/internal/objects/configmap"
	objcontour "github.com/projectcontour/contour-operator/internal/objects/contour"
	objds "github.com/projectcontour/contour-operator/internal/objects/daemonset"
	objdeploy "github.com/projectcontour/contour-operator/internal/objects/deployment"
	objjob "github.com/projectcontour/contour-operator/internal/objects/job"
	objns "github.com/projectcontour/contour-operator/internal/objects/namespace"
	objsvc "github.com/projectcontour/contour-operator/internal/objects/service"
	retryable "github.com/projectcontour/contour-operator/internal/retryableerror"
	"github.com/projectcontour/contour-operator/internal/status"
	"github.com/projectcontour/contour-operator/pkg/validation"

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

const (
	controllerName = "contour_controller"
)

// Config holds all the things necessary for the controller to run.
type Config struct {
	// ContourImage is the name of the Contour container image.
	ContourImage string
	// EnvoyImage is the name of the Envoy container image.
	EnvoyImage string
}

// reconciler reconciles a Contour object.
type reconciler struct {
	config Config
	client client.Client
	log    logr.Logger
}

// New creates the contour controller from mgr and cfg. The controller will be pre-configured
// to watch for Contour objects across all namespaces.
func New(mgr manager.Manager, cfg Config) (controller.Controller, error) {
	r := &reconciler{
		config: cfg,
		client: mgr.GetClient(),
		log:    ctrl.Log.WithName(controllerName),
	}
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r})
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
	var errs []error
	cli := r.client

	handleResult := func(resource string, err error) {
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to ensure %s for contour %s/%s: %w", resource, contour.Namespace, contour.Name, err))
		} else {
			r.log.Info(fmt.Sprintf("ensured %s for contour", resource), "namespace", contour.Namespace, "name", contour.Name)
		}
	}

	syncContourStatus := func() error {
		if err := status.SyncContour(ctx, cli, contour); err != nil {
			errs = append(errs, fmt.Errorf("failed to sync status for contour %s/%s: %w", contour.Namespace, contour.Name, err))
		} else {
			r.log.Info("synced status for contour", "namespace", contour.Namespace, "name", contour.Name)
		}
		return retryable.NewMaybeRetryableAggregate(errs)
	}

	handleResult("namespace", objns.EnsureNamespace(ctx, cli, contour))
	handleResult("rbac", objutil.EnsureRBAC(ctx, cli, contour))

	if len(errs) > 0 {
		return syncContourStatus()
	}

	contourImage := r.config.ContourImage
	envoyImage := r.config.EnvoyImage

	handleResult("configmap", objcm.EnsureConfigMap(ctx, cli, contour))
	handleResult("job", objjob.EnsureJob(ctx, cli, contour, contourImage))
	handleResult("deployment", objdeploy.EnsureDeployment(ctx, cli, contour, contourImage))
	handleResult("daemonset", objds.EnsureDaemonSet(ctx, cli, contour, contourImage, envoyImage))
	handleResult("contour service", objsvc.EnsureContourService(ctx, cli, contour))

	switch contour.Spec.NetworkPublishing.Envoy.Type {
	case operatorv1alpha1.LoadBalancerServicePublishingType, operatorv1alpha1.NodePortServicePublishingType, operatorv1alpha1.ClusterIPServicePublishingType:
		handleResult("envoy service", objsvc.EnsureEnvoyService(ctx, cli, contour))
	}

	return syncContourStatus()
}

// ensureContourDeleted ensures contour and all child resources have been deleted.
func (r *reconciler) ensureContourDeleted(ctx context.Context, contour *operatorv1alpha1.Contour) error {
	var errs []error
	cli := r.client

	handleResult := func(resource string, err error) {
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to delete %s for contour %s/%s: %w", resource, contour.Namespace, contour.Name, err))
		} else {
			r.log.Info(fmt.Sprintf("deleted %s for contour", resource), "namespace", contour.Namespace, "name", contour.Name)
		}
	}

	switch contour.Spec.NetworkPublishing.Envoy.Type {
	case operatorv1alpha1.LoadBalancerServicePublishingType, operatorv1alpha1.NodePortServicePublishingType, operatorv1alpha1.ClusterIPServicePublishingType:
		handleResult("envoy service", objsvc.EnsureEnvoyServiceDeleted(ctx, cli, contour))
	}

	handleResult("service", objsvc.EnsureContourServiceDeleted(ctx, cli, contour))
	handleResult("daemonset", objds.EnsureDaemonSetDeleted(ctx, cli, contour))
	handleResult("deployment", objdeploy.EnsureDeploymentDeleted(ctx, cli, contour))
	handleResult("job", objjob.EnsureJobDeleted(ctx, cli, contour, r.config.ContourImage))
	handleResult("configmap", objcm.EnsureConfigMapDeleted(ctx, cli, contour))
	handleResult("rbac", objutil.EnsureRBACDeleted(ctx, cli, contour))
	if deleteExpected, err := objns.EnsureNamespaceDeleted(ctx, cli, contour); deleteExpected {
		handleResult("namespace", err)
	} else {
		r.log.Info("bypassing namespace deletion", "namespace", contour.Namespace, "name", contour.Name)
	}

	if len(errs) == 0 {
		if err := objcontour.EnsureFinalizerRemoved(ctx, cli, contour); err != nil {
			errs = append(errs, fmt.Errorf("failed to remove finalizer from contour %s/%s: %w", contour.Namespace, contour.Name, err))
		} else {
			r.log.Info("removed finalizer from contour", "namespace", contour.Namespace, "name", contour.Name)
		}
	}

	return utilerrors.NewAggregate(errs)
}
