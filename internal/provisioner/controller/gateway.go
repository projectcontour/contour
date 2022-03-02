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
	ensurer           *ensurer
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

	r.ensurer = &ensurer{
		log:          r.log,
		client:       r.client,
		contourImage: contourImage,
		envoyImage:   envoyImage,
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
				Spec: model.ContourSpec{
					Namespace: model.NamespaceSpec{
						Name:             req.Namespace,
						RemoveOnDeletion: false,
					},
				},
			}

			if errs := r.ensurer.ensureContourDeleted(ctx, contour); len(errs) > 0 {
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
			Namespace: model.NamespaceSpec{
				Name: gateway.Namespace,
			},
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

	servicePorts := map[string]int32{}
	for _, listener := range gateway.Spec.Listeners {
		servicePorts[string(listener.Name)] = int32(listener.Port)
	}

	if errs := r.ensurer.ensureContour(ctx, gatewayContour, servicePorts); len(errs) > 0 {
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
