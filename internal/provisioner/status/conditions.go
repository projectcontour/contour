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
	"strings"

	operatorv1alpha1 "github.com/projectcontour/contour/internal/provisioner/api"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilclock "k8s.io/apimachinery/pkg/util/clock"
)

// clock is to enable unit testing
var clock utilclock.Clock = utilclock.RealClock{}

// computeContourAvailableCondition computes the contour Available status condition
// type based on deployment, ds, set, exists and admitted.
func computeContourAvailableCondition(deployment *appsv1.Deployment, ds *appsv1.DaemonSet) metav1.Condition {
	switch {
	default:
		if deployment == nil {
			return metav1.Condition{
				Type:    operatorv1alpha1.ContourAvailableConditionType,
				Status:  metav1.ConditionFalse,
				Reason:  "ContourUnavailable",
				Message: "Contour deployment does not exist.",
			}
		}
		if ds == nil {
			return metav1.Condition{
				Type:    operatorv1alpha1.ContourAvailableConditionType,
				Status:  metav1.ConditionFalse,
				Reason:  "ContourUnavailable",
				Message: "Envoy daemonset does not exist.",
			}
		}
		dsAvailable := ds.Status.NumberAvailable > 0
		for _, cond := range deployment.Status.Conditions {
			if cond.Type != appsv1.DeploymentAvailable {
				continue
			}
			switch {
			case cond.Status == corev1.ConditionTrue:
				if dsAvailable {
					return metav1.Condition{
						Type:    operatorv1alpha1.ContourAvailableConditionType,
						Status:  metav1.ConditionTrue,
						Reason:  "ContourAvailable",
						Message: "Contour has minimum availability.",
					}
				}
				return metav1.Condition{
					Type:    operatorv1alpha1.ContourAvailableConditionType,
					Status:  metav1.ConditionFalse,
					Reason:  "ContourUnavailable",
					Message: "Envoy daemonset does not have minimum availability.",
				}
			case cond.Status == corev1.ConditionFalse:
				if dsAvailable {
					return metav1.Condition{
						Type:    operatorv1alpha1.ContourAvailableConditionType,
						Status:  metav1.ConditionFalse,
						Reason:  "ContourUnavailable",
						Message: fmt.Sprintf("Contour %s", strings.ToLower(cond.Message)),
					}
				}
				return metav1.Condition{
					Type:   operatorv1alpha1.ContourAvailableConditionType,
					Status: metav1.ConditionFalse,
					Reason: "ContourUnavailable",
					Message: fmt.Sprintf("Envoy daemonset does not have minimum availability. Contour %s",
						strings.ToLower(cond.Message)),
				}
			case cond.Status == corev1.ConditionUnknown:
				return metav1.Condition{
					Type:    operatorv1alpha1.ContourAvailableConditionType,
					Status:  metav1.ConditionUnknown,
					Reason:  fmt.Sprintf("ContourUnknown: %s", cond.Message),
					Message: fmt.Sprintf("Contour status unknown. %s", cond.Message),
				}
			}
		}
	}

	return metav1.Condition{
		Type:    operatorv1alpha1.ContourAvailableConditionType,
		Status:  metav1.ConditionUnknown,
		Reason:  "ContourUnknown",
		Message: "Contour status unknown.",
	}
}

// mergeConditions adds or updates matching conditions, and updates
// the transition time if details of a condition have changed. Returns
// the updated condition array.
func mergeConditions(conditions []metav1.Condition, updates ...metav1.Condition) []metav1.Condition {
	now := metav1.NewTime(clock.Now())
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
					if cond.Status != update.Status {
						conditions[j].LastTransitionTime = now
					}
					break
				}
			}
		}
		if add {
			updates[i].LastTransitionTime = now
			additions = append(additions, updates[i])
		}
	}
	conditions = append(conditions, additions...)
	return conditions
}

func conditionChanged(a, b metav1.Condition) bool {
	return a.Status != b.Status || a.Reason != b.Reason || a.Message != b.Message
}
