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

	"github.com/projectcontour/contour/internal/gatewayapi"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilclock "k8s.io/utils/clock"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayapi_v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

const (
	ConditionNotImplemented   gatewayapi_v1beta1.RouteConditionType = "NotImplemented"
	ConditionValidBackendRefs gatewayapi_v1beta1.RouteConditionType = "ValidBackendRefs"
	ConditionValidMatches     gatewayapi_v1beta1.RouteConditionType = "ValidMatches"
)

const (
	ReasonNotImplemented                gatewayapi_v1beta1.RouteConditionReason = "NotImplemented"
	ReasonPathMatchType                 gatewayapi_v1beta1.RouteConditionReason = "PathMatchType"
	ReasonHeaderMatchType               gatewayapi_v1beta1.RouteConditionReason = "HeaderMatchType"
	ReasonQueryParamMatchType           gatewayapi_v1beta1.RouteConditionReason = "QueryParamMatchType"
	ReasonHTTPRouteFilterType           gatewayapi_v1beta1.RouteConditionReason = "HTTPRouteFilterType"
	ReasonDegraded                      gatewayapi_v1beta1.RouteConditionReason = "Degraded"
	ReasonErrorsExist                   gatewayapi_v1beta1.RouteConditionReason = "ErrorsExist"
	ReasonAllBackendRefsHaveZeroWeights gatewayapi_v1beta1.RouteConditionReason = "AllBackendRefsHaveZeroWeights"
	ReasonInvalidPathMatch              gatewayapi_v1beta1.RouteConditionReason = "InvalidPathMatch"
	ReasonInvalidGateway                gatewayapi_v1beta1.RouteConditionReason = "InvalidGateway"
	ReasonListenersNotReady             gatewayapi_v1beta1.RouteConditionReason = "ListenersNotReady"
)

// clock is used to set lastTransitionTime on status conditions.
var clock utilclock.Clock = utilclock.RealClock{}

// RouteStatusUpdate represents an atomic update to a
// Route's status.
type RouteStatusUpdate struct {
	FullName            types.NamespacedName
	RouteParentStatuses []*gatewayapi_v1beta1.RouteParentStatus
	GatewayRef          types.NamespacedName
	GatewayController   gatewayapi_v1beta1.GatewayController
	Resource            client.Object
	Generation          int64
	TransitionTime      metav1.Time
}

// RouteParentStatusUpdate helps update a specific
// parent ref's RouteParentStatus.
type RouteParentStatusUpdate struct {
	*RouteStatusUpdate
	parentRef gatewayapi_v1beta1.ParentReference
}

// StatusUpdateFor returns a RouteParentStatusUpdate for the given parent ref.
func (r *RouteStatusUpdate) StatusUpdateFor(parentRef gatewayapi_v1beta1.ParentReference) *RouteParentStatusUpdate {
	return &RouteParentStatusUpdate{
		RouteStatusUpdate: r,
		parentRef:         parentRef,
	}
}

// AddCondition adds a condition with the given properties
// to the RouteParentStatus.
func (r *RouteParentStatusUpdate) AddCondition(conditionType gatewayapi_v1beta1.RouteConditionType, status metav1.ConditionStatus, reason gatewayapi_v1beta1.RouteConditionReason, message string) metav1.Condition {
	var rps *gatewayapi_v1beta1.RouteParentStatus

	for _, v := range r.RouteParentStatuses {
		if v.ParentRef == r.parentRef {
			rps = v
			break
		}
	}

	if rps == nil {
		rps = &gatewayapi_v1beta1.RouteParentStatus{
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

	cond := metav1.Condition{
		Reason:             string(reason),
		Status:             status,
		Type:               string(conditionType),
		Message:            message,
		LastTransitionTime: metav1.NewTime(clock.Now()),
		ObservedGeneration: r.Generation,
	}

	if idx > -1 {
		rps.Conditions[idx] = cond
	} else {
		rps.Conditions = append(rps.Conditions, cond)
	}

	return cond
}

func (r *RouteStatusUpdate) ConditionsForParentRef(parentRef gatewayapi_v1beta1.ParentReference) []metav1.Condition {
	for _, rps := range r.RouteParentStatuses {
		if rps.ParentRef == parentRef {
			return rps.Conditions
		}
	}

	return nil
}

func (r *RouteStatusUpdate) Mutate(obj client.Object) client.Object {
	var newRouteParentStatuses []gatewayapi_v1beta1.RouteParentStatus

	for _, rps := range r.RouteParentStatuses {
		for i := range rps.Conditions {
			cond := &rps.Conditions[i]

			cond.ObservedGeneration = r.Generation
			cond.LastTransitionTime = r.TransitionTime
		}

		newRouteParentStatuses = append(newRouteParentStatuses, *rps)
	}

	switch o := obj.(type) {
	case *gatewayapi_v1beta1.HTTPRoute:
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
			if !gatewayapi.IsRefToGateway(gatewayapi.UpgradeParentRef(rps.ParentRef), r.GatewayRef) {
				newRouteParentStatuses = append(newRouteParentStatuses, gatewayapi.UpgradeRouteParentStatus(rps))
			}
		}

		route.Status.Parents = gatewayapi.DowngradeRouteParentStatuses(newRouteParentStatuses)

		return route
	default:
		panic(fmt.Sprintf("Unsupported %T object %s/%s in RouteConditionsUpdate status mutator", obj, r.FullName.Namespace, r.FullName.Name))
	}
}
