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
	"time"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayapi_v1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

const reasonValidGatewayClass = "Valid"
const reasonInvalidGatewayClass = "Invalid"

// computeGatewayClassAdmittedCondition computes the GatewayClass Admitted status condition.
func computeGatewayClassAdmittedCondition(gatewayClass *gatewayapi_v1alpha1.GatewayClass, admitted bool) metav1.Condition {
	switch admitted {
	case true:
		return metav1.Condition{
			Type:               string(gatewayapi_v1alpha1.GatewayClassConditionStatusAdmitted),
			Status:             metav1.ConditionTrue,
			Reason:             "Valid",
			Message:            "Valid GatewayClass",
			ObservedGeneration: gatewayClass.Generation,
			LastTransitionTime: metav1.NewTime(time.Now()),
		}
	default:
		return metav1.Condition{
			Type:               string(gatewayapi_v1alpha1.GatewayClassConditionStatusAdmitted),
			Status:             metav1.ConditionFalse,
			Reason:             "Invalid",
			Message:            "Invalid GatewayClass: another older GatewayClass with the same Spec.Controller exists",
			ObservedGeneration: gatewayClass.Generation,
			LastTransitionTime: metav1.NewTime(time.Now()),
		}
	}
}

// mergeConditions adds or updates matching conditions, and updates the transition
// time if details of a condition have changed. Returns the updated condition array.
func mergeConditions(conditions []metav1.Condition, updates ...metav1.Condition) []metav1.Condition {
	var additions []metav1.Condition
	for i, update := range updates {
		add := true
		for j, cond := range conditions {
			if cond.Type == update.Type {
				add = false
				if conditionChanged(cond, update) {
					conditions[j].Status = update.Status
					conditions[j].Reason = update.Reason
					conditions[j].Message = update.Message
					conditions[j].ObservedGeneration = update.ObservedGeneration
					conditions[j].LastTransitionTime = update.LastTransitionTime
					break
				}
			}
		}
		if add {
			additions = append(additions, updates[i])
		}
	}
	conditions = append(conditions, additions...)
	return conditions
}

func conditionChanged(a, b metav1.Condition) bool {
	return a.Status != b.Status || a.Reason != b.Reason || a.Message != b.Message
}

func conditionsEqual(a, b []metav1.Condition) bool {
	return apiequality.Semantic.DeepEqual(a, b)
}
