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
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

type GatewayReasonType string

const ReasonValidGateway = "Valid"
const ReasonInvalidGateway = "Invalid"

const MessageValidGateway = "Valid Gateway"

// GatewayStatusUpdate represents an atomic update to a
// Gateway's status.
type GatewayStatusUpdate struct {
	FullName           types.NamespacedName
	Conditions         map[gatewayapi_v1alpha2.GatewayConditionType]metav1.Condition
	ExistingConditions map[gatewayapi_v1alpha2.GatewayConditionType]metav1.Condition
	ListenerStatus     map[string]*gatewayapi_v1alpha2.ListenerStatus
	Generation         int64
	TransitionTime     metav1.Time
}

// AddCondition returns a metav1.Condition for a given GatewayConditionType.
func (gatewayUpdate *GatewayStatusUpdate) AddCondition(
	cond gatewayapi_v1alpha2.GatewayConditionType,
	status metav1.ConditionStatus,
	reason GatewayReasonType,
	message string,
) metav1.Condition {

	if c, ok := gatewayUpdate.Conditions[cond]; ok {
		message = fmt.Sprintf("%s, %s", c.Message, message)
	}

	newCond := metav1.Condition{
		Reason:             string(reason),
		Status:             status,
		Type:               string(cond),
		Message:            message,
		LastTransitionTime: metav1.NewTime(clock.Now()),
		ObservedGeneration: gatewayUpdate.Generation,
	}
	gatewayUpdate.Conditions[cond] = newCond
	return newCond
}

func (gatewayUpdate *GatewayStatusUpdate) SetListenerSupportedKinds(listenerName string, kinds []gatewayapi_v1alpha2.Kind) {
	if gatewayUpdate.ListenerStatus == nil {
		gatewayUpdate.ListenerStatus = map[string]*gatewayapi_v1alpha2.ListenerStatus{}
	}
	if gatewayUpdate.ListenerStatus[listenerName] == nil {
		gatewayUpdate.ListenerStatus[listenerName] = &gatewayapi_v1alpha2.ListenerStatus{
			Name: gatewayapi_v1alpha2.SectionName(listenerName),
		}
	}

	for _, kind := range kinds {
		groupKind := gatewayapi_v1alpha2.RouteGroupKind{
			Group: gatewayapi.GroupPtr(gatewayapi_v1alpha2.GroupName),
			Kind:  kind,
		}

		gatewayUpdate.ListenerStatus[listenerName].SupportedKinds = append(gatewayUpdate.ListenerStatus[listenerName].SupportedKinds, groupKind)
	}
}

func (gatewayUpdate *GatewayStatusUpdate) SetListenerAttachedRoutes(listenerName string, numRoutes int) {
	if gatewayUpdate.ListenerStatus == nil {
		gatewayUpdate.ListenerStatus = map[string]*gatewayapi_v1alpha2.ListenerStatus{}
	}
	if gatewayUpdate.ListenerStatus[listenerName] == nil {
		gatewayUpdate.ListenerStatus[listenerName] = &gatewayapi_v1alpha2.ListenerStatus{
			Name: gatewayapi_v1alpha2.SectionName(listenerName),
		}
	}

	gatewayUpdate.ListenerStatus[listenerName].AttachedRoutes = int32(numRoutes)
}

// AddListenerCondition adds a Condition for the specified listener.
func (gatewayUpdate *GatewayStatusUpdate) AddListenerCondition(
	listenerName string,
	cond gatewayapi_v1alpha2.ListenerConditionType,
	status metav1.ConditionStatus,
	reason gatewayapi_v1alpha2.ListenerConditionReason,
	message string,
) metav1.Condition {
	if gatewayUpdate.ListenerStatus == nil {
		gatewayUpdate.ListenerStatus = map[string]*gatewayapi_v1alpha2.ListenerStatus{}
	}
	if gatewayUpdate.ListenerStatus[listenerName] == nil {
		gatewayUpdate.ListenerStatus[listenerName] = &gatewayapi_v1alpha2.ListenerStatus{
			Name: gatewayapi_v1alpha2.SectionName(listenerName),
		}
	}

	listenerStatus := gatewayUpdate.ListenerStatus[listenerName]

	idx := -1
	for i, existing := range listenerStatus.Conditions {
		if existing.Type == string(cond) {
			idx = i
			message = fmt.Sprintf("%s, %s", existing.Message, message)
			break
		}
	}

	newCond := metav1.Condition{
		Reason:             string(reason),
		Status:             status,
		Type:               string(cond),
		Message:            message,
		LastTransitionTime: metav1.NewTime(clock.Now()),
		ObservedGeneration: gatewayUpdate.Generation,
	}

	if idx > -1 {
		listenerStatus.Conditions[idx] = newCond
	} else {
		listenerStatus.Conditions = append(listenerStatus.Conditions, newCond)
	}

	return newCond
}

func getGatewayConditions(gs *gatewayapi_v1alpha2.GatewayStatus) map[gatewayapi_v1alpha2.GatewayConditionType]metav1.Condition {
	conditions := make(map[gatewayapi_v1alpha2.GatewayConditionType]metav1.Condition)
	for _, cond := range gs.Conditions {
		if val, ok := conditions[gatewayapi_v1alpha2.GatewayConditionType(cond.Type)]; !ok {
			conditions[gatewayapi_v1alpha2.GatewayConditionType(cond.Type)] = val
		}
	}
	return conditions
}

func (gatewayUpdate *GatewayStatusUpdate) Mutate(obj client.Object) client.Object {
	o, ok := obj.(*gatewayapi_v1alpha2.Gateway)
	if !ok {
		panic(fmt.Sprintf("Unsupported %T object %s/%s in GatewayStatusUpdate status mutator",
			obj, gatewayUpdate.FullName.Namespace, gatewayUpdate.FullName.Name,
		))
	}

	updated := o.DeepCopy()

	var conditionsToWrite []metav1.Condition

	for _, cond := range gatewayUpdate.Conditions {

		// Set the Condition's observed generation based on
		// the generation of the gateway we looked at.
		cond.ObservedGeneration = gatewayUpdate.Generation
		cond.LastTransitionTime = gatewayUpdate.TransitionTime

		// is there a newer Condition on the gateway matching
		// this condition's type? If so, our observation is stale,
		// so don't write it, keep the newer one instead.
		var newerConditionExists bool
		for _, existingCond := range gatewayUpdate.ExistingConditions {
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
		// gateway, then write the one we computed.
		if !newerConditionExists {
			conditionsToWrite = append(conditionsToWrite, cond)
		}
	}

	updated.Status.Conditions = conditionsToWrite

	// Overwrite all listener statuses since we re-compute all of them
	// for each Gateway status update.
	var listenerStatusToWrite []gatewayapi_v1alpha2.ListenerStatus
	for _, status := range gatewayUpdate.ListenerStatus {
		if status.Conditions == nil {
			// Conditions is a required field so we have to specify an empty slice here
			status.Conditions = []metav1.Condition{}
		}
		if status.SupportedKinds == nil {
			// SupportedKinds is a required field so we have to specify an empty slice here
			status.SupportedKinds = []gatewayapi_v1alpha2.RouteGroupKind{}
		}
		listenerStatusToWrite = append(listenerStatusToWrite, *status)
	}

	updated.Status.Listeners = listenerStatusToWrite

	// TODO: Manage addresses.
	// xref: https://github.com/projectcontour/contour/issues/3828

	return updated
}
