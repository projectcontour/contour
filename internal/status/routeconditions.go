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

package status

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilclock "k8s.io/apimachinery/pkg/util/clock"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

const ConditionNotImplemented gatewayapi_v1alpha2.RouteConditionType = "NotImplemented"
const ConditionResolvedRefs gatewayapi_v1alpha2.RouteConditionType = "ResolvedRefs"
const ConditionValidBackendRefs gatewayapi_v1alpha2.RouteConditionType = "ValidBackendRefs"

type RouteReasonType string

const ReasonNotImplemented RouteReasonType = "NotImplemented"
const ReasonPathMatchType RouteReasonType = "PathMatchType"
const ReasonHeaderMatchType RouteReasonType = "HeaderMatchType"
const ReasonHTTPRouteFilterType RouteReasonType = "HTTPRouteFilterType"
const ReasonDegraded RouteReasonType = "Degraded"
const ReasonValid RouteReasonType = "Valid"
const ReasonErrorsExist RouteReasonType = "ErrorsExist"
const ReasonGatewayAllowMismatch RouteReasonType = "GatewayAllowMismatch"
const ReasonAllBackendRefsHaveZeroWeights = "AllBackendRefsHaveZeroWeights"

// clock is used to set lastTransitionTime on status conditions.
var clock utilclock.Clock = utilclock.RealClock{}

type RouteConditionsUpdate struct {
	FullName           types.NamespacedName
	Conditions         map[gatewayapi_v1alpha2.RouteConditionType]metav1.Condition
	ExistingConditions map[gatewayapi_v1alpha2.RouteConditionType]metav1.Condition
	GatewayRef         types.NamespacedName
	GatewayController  gatewayapi_v1alpha2.GatewayController
	Resource           client.Object
	Generation         int64
	TransitionTime     metav1.Time
}

// AddCondition returns a metav1.Condition for a given ConditionType.
func (routeUpdate *RouteConditionsUpdate) AddCondition(cond gatewayapi_v1alpha2.RouteConditionType, status metav1.ConditionStatus, reason RouteReasonType, message string) metav1.Condition {

	if c, ok := routeUpdate.Conditions[cond]; ok {
		message = fmt.Sprintf("%s, %s", c.Message, message)
	}

	newDc := metav1.Condition{
		Reason:             string(reason),
		Status:             status,
		Type:               string(cond),
		Message:            message,
		LastTransitionTime: metav1.NewTime(clock.Now()),
		ObservedGeneration: routeUpdate.Generation,
	}
	routeUpdate.Conditions[cond] = newDc
	return newDc
}

func (routeUpdate *RouteConditionsUpdate) Mutate(obj client.Object) client.Object {

	var gatewayStatuses []gatewayapi_v1alpha2.RouteParentStatus
	var conditionsToWrite []metav1.Condition

	for _, cond := range routeUpdate.Conditions {

		// set the Condition's observed generation based on
		// the generation of the route we looked at.
		cond.ObservedGeneration = routeUpdate.Generation
		cond.LastTransitionTime = routeUpdate.TransitionTime

		// is there a newer Condition on the route matching
		// this condition's type? If so, our observation is stale,
		// so don't write it, keep the newer one instead.
		var newerConditionExists bool
		for _, existingCond := range routeUpdate.ExistingConditions {
			if existingCond.Type != cond.Type {
				continue
			}

			if existingCond.ObservedGeneration > cond.ObservedGeneration {
				conditionsToWrite = append(conditionsToWrite, existingCond)
				newerConditionExists = true
				break
			}
		}

		// if we didn't find a newer version of the Condition on the
		// route, then write the one we computed.
		if !newerConditionExists {
			conditionsToWrite = append(conditionsToWrite, cond)
		}
	}

	gatewayStatuses = append(gatewayStatuses, gatewayapi_v1alpha2.RouteParentStatus{
		ParentRef:      parentRefForGateway(routeUpdate.GatewayRef),
		ControllerName: routeUpdate.GatewayController,
		Conditions:     conditionsToWrite,
	})

	switch o := obj.(type) {
	case *gatewayapi_v1alpha2.HTTPRoute:
		route := o.DeepCopy()

		// Set the HTTPRoute status.
		gatewayStatuses = append(gatewayStatuses, routeUpdate.combineConditions(route.Status.Parents)...)
		route.Status.RouteStatus.Parents = gatewayStatuses
		return route
	case *gatewayapi_v1alpha2.TLSRoute:
		route := o.DeepCopy()

		// Set the TLSRoute status.
		gatewayStatuses = append(gatewayStatuses, routeUpdate.combineConditions(route.Status.Parents)...)
		route.Status.RouteStatus.Parents = gatewayStatuses
		return route
	default:
		panic(fmt.Sprintf("Unsupported %T object %s/%s in RouteConditionsUpdate status mutator",
			obj, routeUpdate.FullName.Namespace, routeUpdate.FullName.Name,
		))
	}
}

// combineConditions (due for a rename) returns all RouteParentStatuses
// from gwStatus that are *not* for the routeUpdate's Gateway.
func (routeUpdate *RouteConditionsUpdate) combineConditions(gwStatus []gatewayapi_v1alpha2.RouteParentStatus) []gatewayapi_v1alpha2.RouteParentStatus {
	var gatewayStatuses []gatewayapi_v1alpha2.RouteParentStatus

	// Now that we have all the conditions, add them back to the object
	// to get written out.
	for _, rgs := range gwStatus {
		if !isRefToGateway(rgs.ParentRef, routeUpdate.GatewayRef) {
			gatewayStatuses = append(gatewayStatuses, rgs)
		}
	}

	return gatewayStatuses
}

// isRefToGateway returns whether or not ref is a reference
// to a Gateway with the given namespace & name.
func isRefToGateway(ref gatewayapi_v1alpha2.ParentRef, gateway types.NamespacedName) bool {
	return ref.Group != nil && *ref.Group == gatewayapi_v1alpha2.GroupName &&
		ref.Kind != nil && *ref.Kind == "Gateway" &&
		ref.Namespace != nil && *ref.Namespace == gatewayapi_v1alpha2.Namespace(gateway.Namespace) &&
		string(ref.Name) == gateway.Name
}

// parentRefForGateway returns a ParentRef for a Gateway with
// the given namespace and name.
func parentRefForGateway(gateway types.NamespacedName) gatewayapi_v1alpha2.ParentRef {
	var (
		group     = gatewayapi_v1alpha2.Group(gatewayapi_v1alpha2.GroupName)
		kind      = gatewayapi_v1alpha2.Kind("Gateway")
		namespace = gatewayapi_v1alpha2.Namespace(gateway.Namespace)
	)

	return gatewayapi_v1alpha2.ParentRef{
		Group:     &group,
		Kind:      &kind,
		Namespace: &namespace,
		Name:      gatewayapi_v1alpha2.ObjectName(gateway.Name),
	}
}

func (c *Cache) getRouteGatewayConditions(gatewayStatus []gatewayapi_v1alpha2.RouteParentStatus) map[gatewayapi_v1alpha2.RouteConditionType]metav1.Condition {
	for _, gs := range gatewayStatus {
		if isRefToGateway(gs.ParentRef, c.gatewayRef) {

			conditions := make(map[gatewayapi_v1alpha2.RouteConditionType]metav1.Condition)
			for _, gsCondition := range gs.Conditions {
				if val, ok := conditions[gatewayapi_v1alpha2.RouteConditionType(gsCondition.Type)]; !ok {
					conditions[gatewayapi_v1alpha2.RouteConditionType(gsCondition.Type)] = val
				}
			}
			return conditions
		}
	}
	return map[gatewayapi_v1alpha2.RouteConditionType]metav1.Condition{}
}
