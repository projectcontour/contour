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
	"time"

	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayapi_v1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/projectcontour/contour/internal/gatewayapi"
)

const (
	ConditionValidBackendRefs gatewayapi_v1.RouteConditionType = "ValidBackendRefs"
	ConditionValidMatches     gatewayapi_v1.RouteConditionType = "ValidMatches"
)

const (
	ReasonDegraded                        gatewayapi_v1.RouteConditionReason = "Degraded"
	ReasonAllBackendRefsHaveZeroWeights   gatewayapi_v1.RouteConditionReason = "AllBackendRefsHaveZeroWeights"
	ReasonInvalidPathMatch                gatewayapi_v1.RouteConditionReason = "InvalidPathMatch"
	ReasonInvalidMethodMatch              gatewayapi_v1.RouteConditionReason = "InvalidMethodMatch"
	ReasonInvalidGateway                  gatewayapi_v1.RouteConditionReason = "InvalidGateway"
	ReasonRouteRuleMatchConflict          gatewayapi_v1.RouteConditionReason = "RuleMatchConflict"
	ReasonRouteRuleMatchPartiallyConflict gatewayapi_v1.RouteConditionReason = "RuleMatchPartiallyConflict"

	MessageRouteRuleMatchConflict          string = "%s's Match has conflict with other %s's Match"
	MessageRouteRuleMatchPartiallyConflict string = "Dropped Rule: some of %s's rule(s) has(ve) been dropped because of conflict against other %s's rule(s)"
)

// RouteStatusUpdate represents an atomic update to a
// Route's status.
type RouteStatusUpdate struct {
	FullName            types.NamespacedName
	RouteParentStatuses []*gatewayapi_v1.RouteParentStatus
	GatewayRef          types.NamespacedName
	GatewayController   gatewayapi_v1.GatewayController
	Resource            client.Object
	Generation          int64
	TransitionTime      meta_v1.Time
}

// RouteParentStatusUpdate helps update a specific
// parent ref's RouteParentStatus.
type RouteParentStatusUpdate struct {
	*RouteStatusUpdate
	parentRef gatewayapi_v1.ParentReference
}

// StatusUpdateFor returns a RouteParentStatusUpdate for the given parent ref.
func (r *RouteStatusUpdate) StatusUpdateFor(parentRef gatewayapi_v1.ParentReference) *RouteParentStatusUpdate {
	return &RouteParentStatusUpdate{
		RouteStatusUpdate: r,
		parentRef:         parentRef,
	}
}

// AddCondition adds a condition with the given properties
// to the RouteParentStatus.
func (r *RouteParentStatusUpdate) AddCondition(conditionType gatewayapi_v1.RouteConditionType, status meta_v1.ConditionStatus, reason gatewayapi_v1.RouteConditionReason, message string) meta_v1.Condition {
	var rps *gatewayapi_v1.RouteParentStatus

	for _, v := range r.RouteParentStatuses {
		if v.ParentRef == r.parentRef {
			rps = v
			break
		}
	}

	if rps == nil {
		rps = &gatewayapi_v1.RouteParentStatus{
			ParentRef:      r.parentRef,
			ControllerName: r.GatewayController,
		}

		r.RouteParentStatuses = append(r.RouteParentStatuses, rps)
	}

	idx := -1
	for i, c := range rps.Conditions {
		if c.Type == string(conditionType) {
			idx = i
			break
		}
	}

	if idx > -1 {
		message = rps.Conditions[idx].Message + ", " + message
	}

	cond := meta_v1.Condition{
		Reason:             string(reason),
		Status:             status,
		Type:               string(conditionType),
		Message:            message,
		LastTransitionTime: meta_v1.NewTime(time.Now()),
		ObservedGeneration: r.Generation,
	}

	if idx > -1 {
		rps.Conditions[idx] = cond
	} else {
		rps.Conditions = append(rps.Conditions, cond)
	}

	return cond
}

// ConditionExists returns whether or not a condition with the given type exists.
func (r *RouteParentStatusUpdate) ConditionExists(conditionType gatewayapi_v1.RouteConditionType) bool {
	for _, c := range r.ConditionsForParentRef(r.parentRef) {
		if c.Type == string(conditionType) {
			return true
		}
	}
	return false
}

func (r *RouteStatusUpdate) ConditionsForParentRef(parentRef gatewayapi_v1.ParentReference) []meta_v1.Condition {
	for _, rps := range r.RouteParentStatuses {
		if rps.ParentRef == parentRef {
			return rps.Conditions
		}
	}

	return nil
}

func (r *RouteStatusUpdate) Mutate(obj client.Object) client.Object {
	var newRouteParentStatuses []gatewayapi_v1.RouteParentStatus

	for _, rps := range r.RouteParentStatuses {
		for i := range rps.Conditions {
			cond := &rps.Conditions[i]

			cond.ObservedGeneration = r.Generation
			cond.LastTransitionTime = r.TransitionTime
		}

		newRouteParentStatuses = append(newRouteParentStatuses, *rps)
	}

	switch o := obj.(type) {
	case *gatewayapi_v1.HTTPRoute:
		route := o.DeepCopy()

		// Get all the RouteParentStatuses that are for other Gateways.
		for _, rps := range o.Status.Parents {
			if !gatewayapi.IsRefToGateway(rps.ParentRef, r.GatewayRef) {
				newRouteParentStatuses = append(newRouteParentStatuses, rps)
			}
		}

		route.Status.Parents = newRouteParentStatuses

		return route
	case *gatewayapi_v1alpha2.TLSRoute:
		route := o.DeepCopy()

		// Get all the RouteParentStatuses that are for other Gateways.
		for _, rps := range o.Status.Parents {
			if !gatewayapi.IsRefToGateway(rps.ParentRef, r.GatewayRef) {
				newRouteParentStatuses = append(newRouteParentStatuses, rps)
			}
		}

		route.Status.Parents = newRouteParentStatuses

		return route
	case *gatewayapi_v1.GRPCRoute:
		route := o.DeepCopy()

		// Get all the RouteParentStatuses that are for other Gateways.
		for _, rps := range o.Status.Parents {
			if !gatewayapi.IsRefToGateway(rps.ParentRef, r.GatewayRef) {
				newRouteParentStatuses = append(newRouteParentStatuses, rps)
			}
		}

		route.Status.Parents = newRouteParentStatuses

		return route

	case *gatewayapi_v1alpha2.TCPRoute:
		route := o.DeepCopy()

		// Get all the RouteParentStatuses that are for other Gateways.
		for _, rps := range o.Status.Parents {
			if !gatewayapi.IsRefToGateway(rps.ParentRef, r.GatewayRef) {
				newRouteParentStatuses = append(newRouteParentStatuses, rps)
			}
		}

		route.Status.Parents = newRouteParentStatuses

		return route

	default:
		panic(fmt.Sprintf("Unsupported %T object %s/%s in RouteConditionsUpdate status mutator", obj, r.FullName.Namespace, r.FullName.Name))
	}
}
