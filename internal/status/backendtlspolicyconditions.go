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
	gatewayapi_v1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"

	"github.com/projectcontour/contour/internal/gatewayapi"
)

// BackendTLSPolicyStatusUpdate represents an atomic update to a
// BackendTLSPolicy's status.
type BackendTLSPolicyStatusUpdate struct {
	FullName               types.NamespacedName
	PolicyAncestorStatuses []*gatewayapi_v1alpha2.PolicyAncestorStatus
	GatewayRef             types.NamespacedName
	GatewayController      gatewayapi_v1.GatewayController
	Generation             int64
	TransitionTime         meta_v1.Time
}

// BackendTLSPolicyAncestorStatusUpdate helps update a specific ancestor ref's
// PolicyAncestorStatus.
type BackendTLSPolicyAncestorStatusUpdate struct {
	*BackendTLSPolicyStatusUpdate
	ancestorRef gatewayapi_v1.ParentReference
}

// StatusUpdateFor returns a BackendTLSPolicyAncestorStatusUpdate for the given
// ancestor ref.
func (b *BackendTLSPolicyStatusUpdate) StatusUpdateFor(ancestorRef gatewayapi_v1.ParentReference) *BackendTLSPolicyAncestorStatusUpdate {
	return &BackendTLSPolicyAncestorStatusUpdate{
		BackendTLSPolicyStatusUpdate: b,
		ancestorRef:                  ancestorRef,
	}
}

// AddCondition adds a condition with the given properties to the
// BackendTLSPolicyAncestorStatus.
func (b *BackendTLSPolicyAncestorStatusUpdate) AddCondition(conditionType gatewayapi_v1alpha2.PolicyConditionType, status meta_v1.ConditionStatus, reason gatewayapi_v1alpha2.PolicyConditionReason, message string) meta_v1.Condition {
	var pas *gatewayapi_v1alpha2.PolicyAncestorStatus

	for _, v := range b.PolicyAncestorStatuses {
		if v.AncestorRef == b.ancestorRef {
			pas = v
			break
		}
	}

	if pas == nil {
		pas = &gatewayapi_v1alpha2.PolicyAncestorStatus{
			AncestorRef:    b.ancestorRef,
			ControllerName: b.GatewayController,
		}

		b.PolicyAncestorStatuses = append(b.PolicyAncestorStatuses, pas)
	}

	idx := -1
	for i, c := range pas.Conditions {
		if c.Type == string(conditionType) {
			idx = i
			break
		}
	}

	if idx > -1 {
		message = pas.Conditions[idx].Message + ", " + message
	}

	cond := meta_v1.Condition{
		Reason:             string(reason),
		Status:             status,
		Type:               string(conditionType),
		Message:            message,
		LastTransitionTime: meta_v1.NewTime(time.Now()),
		ObservedGeneration: b.Generation,
	}

	if idx > -1 {
		pas.Conditions[idx] = cond
	} else {
		pas.Conditions = append(pas.Conditions, cond)
	}

	return cond
}

// ConditionsForAncestorRef returns the list of conditions for a given ancestor
// if it exists.
func (b *BackendTLSPolicyStatusUpdate) ConditionsForAncestorRef(ancestorRef gatewayapi_v1.ParentReference) []meta_v1.Condition {
	for _, pas := range b.PolicyAncestorStatuses {
		if pas.AncestorRef == ancestorRef {
			return pas.Conditions
		}
	}

	return nil
}

func (b *BackendTLSPolicyStatusUpdate) Mutate(obj client.Object) client.Object {
	o, ok := obj.(*gatewayapi_v1alpha3.BackendTLSPolicy)
	if !ok {
		panic(fmt.Sprintf("Unsupported %T object %s/%s in status mutator",
			obj, b.FullName.Namespace, b.FullName.Name,
		))
	}

	var newPolicyAncestorStatuses []gatewayapi_v1alpha2.PolicyAncestorStatus
	for _, pas := range b.PolicyAncestorStatuses {
		for i := range pas.Conditions {
			cond := &pas.Conditions[i]

			cond.ObservedGeneration = b.Generation
			cond.LastTransitionTime = b.TransitionTime
		}

		newPolicyAncestorStatuses = append(newPolicyAncestorStatuses, *pas)
	}

	btp := o.DeepCopy()

	// Get all the PolicyAncestorStatuses that are for other Gateways.
	for _, pas := range o.Status.Ancestors {
		if !gatewayapi.IsRefToGateway(pas.AncestorRef, b.GatewayRef) {
			newPolicyAncestorStatuses = append(newPolicyAncestorStatuses, pas)
		}
	}

	btp.Status.Ancestors = newPolicyAncestorStatuses

	return btp
}
