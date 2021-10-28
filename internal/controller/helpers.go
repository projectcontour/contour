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

	"github.com/projectcontour/contour/internal/gatewayapi"
	"github.com/projectcontour/contour/internal/k8s"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

// setRouteCondition sets a condition on a Gateway API route
// for a given gateway & controller. It performs an upsert,
// updating the existing condition (if there has been a change),
// otherwise adding a new condition.
func setRouteCondition(
	routeParentStatuses []gatewayapi_v1alpha2.RouteParentStatus,
	condition metav1.Condition,
	gateway *gatewayapi_v1alpha2.Gateway,
	controllerName gatewayapi_v1alpha2.GatewayController,
) []gatewayapi_v1alpha2.RouteParentStatus {
	// Look for a RouteParentStatus for the relevant Gateway.
	for _, parentStatus := range routeParentStatuses {
		if !isRefToGateway(parentStatus.ParentRef, k8s.NamespacedNameOf(gateway)) {
			continue
		}

		for i := range parentStatus.Conditions {
			cond := &parentStatus.Conditions[i]

			if cond.Type != condition.Type {
				continue
			}

			// Update only if something has changed.
			if cond.Status != condition.Status || cond.Reason != condition.Reason || cond.Message != condition.Message {
				cond.Status = condition.Status
				cond.Reason = condition.Reason
				cond.Message = condition.Message
				cond.LastTransitionTime = condition.LastTransitionTime
				cond.ObservedGeneration = condition.ObservedGeneration
			}

			return routeParentStatuses
		}

		// condition of type "Accepted" does not already exist for the parent:
		// append it.
		parentStatus.Conditions = append(parentStatus.Conditions, condition)
		return routeParentStatuses
	}

	// RouteParentStatus for the Gateway does not already exist: add it.
	routeParentStatuses = append(routeParentStatuses, gatewayapi_v1alpha2.RouteParentStatus{
		ParentRef:      gatewayapi.GatewayParentRef(gateway.Namespace, gateway.Name),
		ControllerName: controllerName,
		Conditions:     []metav1.Condition{condition},
	})

	return routeParentStatuses
}

// isRefToGateway returns whether the given ref refers to a Gateway with the
// specified name and namespace.
func isRefToGateway(ref gatewayapi_v1alpha2.ParentRef, gateway types.NamespacedName) bool {
	return ref.Group != nil && *ref.Group == gatewayapi_v1alpha2.GroupName &&
		ref.Kind != nil && *ref.Kind == "Gateway" &&
		ref.Namespace != nil && *ref.Namespace == gatewayapi_v1alpha2.Namespace(gateway.Namespace) &&
		string(ref.Name) == gateway.Name
}

// referencesContourGateway returns whether a given list of
// ParentRefs references a Gateway controlled by this Contour.
func referencesContourGateway(
	parentRefs []gatewayapi_v1alpha2.ParentRef,
	routeNamespace string,
	controllerName gatewayapi_v1alpha2.GatewayController,
	kubeClient client.Client,
	log logrus.FieldLogger,
) (bool, *gatewayapi_v1alpha2.Gateway) {
	for _, parentRef := range parentRefs {
		if !(parentRef.Group == nil || string(*parentRef.Group) == "" || string(*parentRef.Group) == gatewayapi_v1alpha2.GroupName) {
			continue
		}
		if !(parentRef.Kind == nil || string(*parentRef.Kind) == "" || string(*parentRef.Kind) == "Gateway") {
			continue
		}

		key := client.ObjectKey{
			Namespace: routeNamespace,
			Name:      string(parentRef.Name),
		}
		if parentRef.Namespace != nil {
			key.Namespace = string(*parentRef.Namespace)
		}

		gateway := &gatewayapi_v1alpha2.Gateway{}
		if err := kubeClient.Get(context.Background(), key, gateway); err != nil {
			log.WithError(err).Error("error getting gateway")
			continue
		}

		gatewayClass := &gatewayapi_v1alpha2.GatewayClass{}
		if err := kubeClient.Get(context.Background(), client.ObjectKey{Name: string(gateway.Spec.GatewayClassName)}, gatewayClass); err != nil {
			log.WithError(err).Error("error getting gatewayclass")
			continue
		}

		if gatewayClass.Spec.ControllerName == controllerName {
			return true, gateway
		}
	}

	return false, nil
}

// gatewayHasMatchingController returns true if the provided Gateway
// uses a GatewayClass with a Spec.Controller string matching this Contour's
// controller string, or false otherwise.
func gatewayHasMatchingController(obj client.Object, controllerName gatewayapi_v1alpha2.GatewayController, kubeClient client.Client, log logrus.FieldLogger) bool {
	gw, ok := obj.(*gatewayapi_v1alpha2.Gateway)
	if !ok {
		log.Debugf("unexpected object type %T, bypassing reconciliation.", obj)
		return false
	}

	gc := &gatewayapi_v1alpha2.GatewayClass{}
	if err := kubeClient.Get(context.Background(), types.NamespacedName{Name: string(gw.Spec.GatewayClassName)}, gc); err != nil {
		log.WithError(err).Errorf("failed to get gatewayclass %s", gw.Spec.GatewayClassName)
		return false
	}
	if gc.Spec.ControllerName != controllerName {
		log.Debugf("gateway's class controller is not %s; bypassing reconciliation", controllerName)
		return false
	}

	return true
}
