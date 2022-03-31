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

	"github.com/go-logr/logr"
	"github.com/projectcontour/contour/internal/provisioner/model"
	"github.com/projectcontour/contour/internal/provisioner/objects/configmap"
	"github.com/projectcontour/contour/internal/provisioner/objects/daemonset"
	"github.com/projectcontour/contour/internal/provisioner/objects/deployment"
	"github.com/projectcontour/contour/internal/provisioner/objects/rbac"
	"github.com/projectcontour/contour/internal/provisioner/objects/secret"
	"github.com/projectcontour/contour/internal/provisioner/objects/service"
	retryable "github.com/projectcontour/contour/internal/provisioner/retryableerror"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

// gatewayReconciler reconciles Gateway objects.
type gatewayReconciler struct {
	gatewayController gatewayapi_v1alpha2.GatewayController
	contourImage      string
	envoyImage        string
	client            client.Client
	log               logr.Logger
}

func NewGatewayController(mgr manager.Manager, gatewayController, contourImage, envoyImage string) (controller.Controller, error) {
	r := &gatewayReconciler{
		gatewayController: gatewayapi_v1alpha2.GatewayController(gatewayController),
		contourImage:      contourImage,
		envoyImage:        envoyImage,
		client:            mgr.GetClient(),
		log:               ctrl.Log.WithName("gateway-controller"),
	}

	c, err := controller.New("gateway-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return nil, err
	}

	if err := c.Watch(
		&source.Kind{Type: &gatewayapi_v1alpha2.Gateway{}},
		&handler.EnqueueRequestForObject{},
		predicate.NewPredicateFuncs(r.hasMatchingController),
	); err != nil {
		return nil, err
	}

	return c, nil
}

func (r *gatewayReconciler) hasMatchingController(obj client.Object) bool {
	gw, ok := obj.(*gatewayapi_v1alpha2.Gateway)
	if !ok {
		return false
	}

	gatewayClass := &gatewayapi_v1alpha2.GatewayClass{}
	if err := r.client.Get(context.Background(), client.ObjectKey{Name: string(gw.Spec.GatewayClassName)}, gatewayClass); err != nil {
		return false
	}

	return gatewayClass.Spec.ControllerName == r.gatewayController
}

func (r *gatewayReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	gateway := &gatewayapi_v1alpha2.Gateway{}
	if err := r.client.Get(ctx, req.NamespacedName, gateway); err != nil {
		if errors.IsNotFound(err) {
			r.log.WithValues("gateway-namespace", req.Namespace, "gateway-name", req.Name).Info("deleting gateway resources")

			contour := &model.Contour{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: req.Namespace,
					Name:      req.Name,
				},
			}

			if errs := r.ensureContourDeleted(ctx, contour); len(errs) > 0 {
				err := utilerrors.NewAggregate(errs)
				r.log.Error(err, "unable to delete contour for gateway %s/%s", req.Namespace, req.Name)
			}

			return ctrl.Result{}, nil
		}
		// Error reading the object, so requeue the request.
		return ctrl.Result{}, fmt.Errorf("failed to get gateway %s: %w", req, err)
	}

	r.log.WithValues("gateway-namespace", req.Namespace, "gateway-name", req.Name).Info("ensuring gateway resources")

	gatewayContour := &model.Contour{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: gateway.Namespace,
			Name:      gateway.Name,
		},
		Spec: model.ContourSpec{
			Replicas: 2,
			NetworkPublishing: model.NetworkPublishing{
				Envoy: model.EnvoyNetworkPublishing{
					Type: model.LoadBalancerServicePublishingType,
					ContainerPorts: []model.ContainerPort{
						{
							Name:       "http",
							PortNumber: 8080,
						},
						{
							Name:       "https",
							PortNumber: 8443,
						},
					},
				},
			},
		},
	}

	for _, listener := range gateway.Spec.Listeners {
		port := model.ServicePort{
			Name:       string(listener.Name),
			PortNumber: int32(listener.Port),
		}

		gatewayContour.Spec.NetworkPublishing.Envoy.ServicePorts = append(gatewayContour.Spec.NetworkPublishing.Envoy.ServicePorts, port)
	}

	if errs := r.ensureContour(ctx, gatewayContour); len(errs) > 0 {
		return ctrl.Result{}, fmt.Errorf("failed to ensure Contour for gateway: %w", retryable.NewMaybeRetryableAggregate(errs))
	}

	var newConds []metav1.Condition
	for _, cond := range gateway.Status.Conditions {
		if cond.Type == string(gatewayapi_v1alpha2.GatewayConditionScheduled) {
			if cond.Status == metav1.ConditionTrue {
				return ctrl.Result{}, nil
			}

			continue
		}

		newConds = append(newConds, cond)
	}

	r.log.WithValues("gateway-namespace", req.Namespace, "gateway-name", req.Name).Info("setting gateway's Scheduled condition to true")

	// nolint:gocritic
	gateway.Status.Conditions = append(newConds, metav1.Condition{
		Type:               string(gatewayapi_v1alpha2.GatewayConditionScheduled),
		Status:             metav1.ConditionTrue,
		ObservedGeneration: gateway.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             string(gatewayapi_v1alpha2.GatewayReasonScheduled),
		Message:            "Gateway is scheduled",
	})

	if err := r.client.Status().Update(ctx, gateway); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to set gateway %s scheduled condition: %w", req, err)
	}

	return ctrl.Result{}, nil
}

func (r *gatewayReconciler) ensureContour(ctx context.Context, contour *model.Contour) []error {
	var errs []error

	handleResult := func(resource string, err error) {
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to ensure %s for contour %s/%s: %w", resource, contour.Namespace, contour.Name, err))
		} else {
			r.log.Info(fmt.Sprintf("ensured %s for contour", resource), "namespace", contour.Namespace, "name", contour.Name)
		}
	}

	handleResult("rbac", rbac.EnsureRBAC(ctx, r.client, contour))

	if len(errs) > 0 {
		return errs
	}

	handleResult("configmap", configmap.EnsureConfigMap(ctx, r.client, contour))
	handleResult("xDS TLS secrets", secret.EnsureXDSSecrets(ctx, r.client, contour, r.contourImage))
	handleResult("deployment", deployment.EnsureDeployment(ctx, r.client, contour, r.contourImage))
	handleResult("daemonset", daemonset.EnsureDaemonSet(ctx, r.client, contour, r.contourImage, r.envoyImage))
	handleResult("contour service", service.EnsureContourService(ctx, r.client, contour))

	switch contour.Spec.NetworkPublishing.Envoy.Type {
	case model.LoadBalancerServicePublishingType, model.NodePortServicePublishingType, model.ClusterIPServicePublishingType:
		handleResult("envoy service", service.EnsureEnvoyService(ctx, r.client, contour))
	}

	return errs
}

func (r *gatewayReconciler) ensureContourDeleted(ctx context.Context, contour *model.Contour) []error {
	var errs []error

	handleResult := func(resource string, err error) {
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to delete %s for contour %s/%s: %w", resource, contour.Namespace, contour.Name, err))
		} else {
			r.log.Info(fmt.Sprintf("deleted %s for contour", resource), "namespace", contour.Namespace, "name", contour.Name)
		}
	}

	handleResult("envoy service", service.EnsureEnvoyServiceDeleted(ctx, r.client, contour))
	handleResult("service", service.EnsureContourServiceDeleted(ctx, r.client, contour))
	handleResult("daemonset", daemonset.EnsureDaemonSetDeleted(ctx, r.client, contour))
	handleResult("deployment", deployment.EnsureDeploymentDeleted(ctx, r.client, contour))
	handleResult("xDS TLS Secrets", secret.EnsureXDSSecretsDeleted(ctx, r.client, contour))
	handleResult("configmap", configmap.EnsureConfigMapDeleted(ctx, r.client, contour))
	handleResult("rbac", rbac.EnsureRBACDeleted(ctx, r.client, contour))

	return errs
}
