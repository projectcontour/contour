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
	"k8s.io/utils/pointer"
	gatewayapi_v1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

const ResourceHTTPRoute = "httproutes"
const ResourceTLSRoute = "tlsroutes"

const ConditionNotImplemented gatewayapi_v1alpha1.RouteConditionType = "NotImplemented"
const ConditionResolvedRefs gatewayapi_v1alpha1.RouteConditionType = "ResolvedRefs"

type RouteReasonType string

const ReasonNotImplemented RouteReasonType = "NotImplemented"
const ReasonPathMatchType RouteReasonType = "PathMatchType"
const ReasonHeaderMatchType RouteReasonType = "HeaderMatchType"
const ReasonHTTPRouteFilterType RouteReasonType = "HTTPRouteFilterType"
const ReasonDegraded RouteReasonType = "Degraded"
const ReasonValid RouteReasonType = "Valid"
const ReasonErrorsExist RouteReasonType = "ErrorsExist"
const ReasonGatewayAllowMismatch RouteReasonType = "GatewayAllowMismatch"

// clock is used to set lastTransitionTime on status conditions.
var clock utilclock.Clock = utilclock.RealClock{}

type RouteConditionsUpdate struct {
	FullName           types.NamespacedName
	Conditions         map[gatewayapi_v1alpha1.RouteConditionType]metav1.Condition
	ExistingConditions map[gatewayapi_v1alpha1.RouteConditionType]metav1.Condition
	GatewayRef         types.NamespacedName
	Resource           string
	Generation         int64
	TransitionTime     metav1.Time
}

// AddCondition returns a metav1.Condition for a given ConditionType.
func (routeUpdate *RouteConditionsUpdate) AddCondition(cond gatewayapi_v1alpha1.RouteConditionType, status metav1.ConditionStatus, reason RouteReasonType, message string) metav1.Condition {

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

// RouteConditionsAccessor returns a RouteConditionsUpdate that allows a client to build up a list of
// metav1.Conditions as well as a function to commit the change back to the cache when everything
// is done. The commit function pattern is used so that the RouteConditionsUpdate does not need
// to know anything the cache internals.
func (c *Cache) RouteConditionsAccessor(nsName types.NamespacedName, generation int64, resource string, gateways []gatewayapi_v1alpha1.RouteGatewayStatus) (*RouteConditionsUpdate, func()) {
	pu := &RouteConditionsUpdate{
		FullName:           nsName,
		Conditions:         make(map[gatewayapi_v1alpha1.RouteConditionType]metav1.Condition),
		ExistingConditions: c.getRouteGatewayConditions(gateways),
		GatewayRef:         c.gatewayRef,
		Generation:         generation,
		TransitionTime:     metav1.NewTime(clock.Now()),
		Resource:           resource,
	}

	return pu, func() {
		c.commitRoute(pu)
	}
}

func (c *Cache) commitRoute(pu *RouteConditionsUpdate) {
	if len(pu.Conditions) == 0 {
		return
	}
	c.routeUpdates[pu.FullName] = pu
}

func (routeUpdate *RouteConditionsUpdate) Mutate(obj interface{}) interface{} {

	var gatewayStatuses []gatewayapi_v1alpha1.RouteGatewayStatus
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

	gatewayStatuses = append(gatewayStatuses, gatewayapi_v1alpha1.RouteGatewayStatus{
		GatewayRef: gatewayapi_v1alpha1.RouteStatusGatewayReference{
			Name:      routeUpdate.GatewayRef.Name,
			Namespace: routeUpdate.GatewayRef.Namespace,

			// TODO(3689) the value of this field should probably come from
			// the GatewayClass. Plumb that through once Contour handles
			// GatewayClasses.
			Controller: pointer.String("projectcontour.io/contour"),
		},
		Conditions: conditionsToWrite,
	})

	switch o := obj.(type) {
	case *gatewayapi_v1alpha1.HTTPRoute:
		route := o.DeepCopy()

		// Set the HTTPRoute status.
		route.Status.RouteStatus.Gateways = append(gatewayStatuses, routeUpdate.combineConditions(route.Status.Gateways)...)
		return route
	case *gatewayapi_v1alpha1.TLSRoute:
		route := o.DeepCopy()

		// Set the TLSRoute status.
		route.Status.RouteStatus.Gateways = append(gatewayStatuses, routeUpdate.combineConditions(route.Status.Gateways)...)
		return route
	default:
		panic(fmt.Sprintf("Unsupported %T object %s/%s in RouteConditionsUpdate status mutator",
			obj, routeUpdate.FullName.Namespace, routeUpdate.FullName.Name,
		))
	}
}

func (routeUpdate *RouteConditionsUpdate) combineConditions(gwStatus []gatewayapi_v1alpha1.RouteGatewayStatus) []gatewayapi_v1alpha1.RouteGatewayStatus {

	var gatewayStatuses []gatewayapi_v1alpha1.RouteGatewayStatus

	// Now that we have all the conditions, add them back to the object
	// to get written out.
	for _, rgs := range gwStatus {
		if rgs.GatewayRef.Name == routeUpdate.GatewayRef.Name && rgs.GatewayRef.Namespace == routeUpdate.GatewayRef.Namespace {
			continue
		} else {
			gatewayStatuses = append(gatewayStatuses, rgs)
		}
	}

	return gatewayStatuses
}

func (c *Cache) getRouteGatewayConditions(gatewayStatus []gatewayapi_v1alpha1.RouteGatewayStatus) map[gatewayapi_v1alpha1.RouteConditionType]metav1.Condition {
	for _, gs := range gatewayStatus {
		if c.gatewayRef.Name == gs.GatewayRef.Name &&
			c.gatewayRef.Namespace == gs.GatewayRef.Namespace {

			conditions := make(map[gatewayapi_v1alpha1.RouteConditionType]metav1.Condition)
			for _, gsCondition := range gs.Conditions {
				if val, ok := conditions[gatewayapi_v1alpha1.RouteConditionType(gsCondition.Type)]; !ok {
					conditions[gatewayapi_v1alpha1.RouteConditionType(gsCondition.Type)] = val
				}
			}
			return conditions
		}
	}
	return map[gatewayapi_v1alpha1.RouteConditionType]metav1.Condition{}
}
