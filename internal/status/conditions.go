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
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
)

// computeConditions merges the condition updates into the existing list.
// It returns the new list.
func computeConditions(
	existing []contour_v1.DetailedCondition,
	generation int64,
	transitionTime meta_v1.Time,
	updates map[ConditionType]*contour_v1.DetailedCondition,
) []contour_v1.DetailedCondition {
	conditions := existing

	for _, cond := range updates {
		cond.ObservedGeneration = generation
		cond.LastTransitionTime = transitionTime
		conditions = setCondition(conditions, *cond)
	}

	// The kstatus library reads the Stalled condition to calculate reconciliation status.
	// https://github.com/kubernetes-sigs/cli-utils/tree/master/pkg/kstatus
	if valid := getConditionFor(conditions, contour_v1.ValidConditionType); valid != nil {
		conditions = setCondition(conditions, stalledConditionFor(valid))
	}

	return conditions
}

// stalledConditionFor returns a Stalled condition derived from Valid with opposite polarity: Valid=true gives Stalled=false.
func stalledConditionFor(valid *contour_v1.DetailedCondition) contour_v1.DetailedCondition {
	stalled := contour_v1.DetailedCondition{
		Condition: contour_v1.Condition{
			Type:               contour_v1.StalledConditionType,
			Status:             contour_v1.ConditionFalse,
			Reason:             "Valid",
			Message:            "No errors present",
			ObservedGeneration: valid.ObservedGeneration,
			LastTransitionTime: valid.LastTransitionTime,
		},
	}

	if valid.Status == contour_v1.ConditionFalse {
		stalled.Status = contour_v1.ConditionTrue
		stalled.Reason = valid.Reason
		stalled.Message = valid.Message
	}

	return stalled
}

// setCondition adds or replaces a condition of the same type.
func setCondition(conditions []contour_v1.DetailedCondition, cond contour_v1.DetailedCondition) []contour_v1.DetailedCondition {
	for i := range conditions {
		if conditions[i].Type != cond.Type {
			continue
		}
		if conditions[i].ObservedGeneration > cond.ObservedGeneration {
			return conditions
		}
		cond.DeepCopyInto(&conditions[i])
		return conditions
	}
	return append(conditions, cond)
}

// getConditionFor returns the condition with the given type, or nil.
func getConditionFor(conditions []contour_v1.DetailedCondition, condType string) *contour_v1.DetailedCondition {
	for i := range conditions {
		if conditions[i].Type == condType {
			return &conditions[i]
		}
	}
	return nil
}
