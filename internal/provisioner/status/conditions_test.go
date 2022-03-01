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
	"testing"
	"time"

	operatorv1alpha1 "github.com/projectcontour/contour-operator/api/v1alpha1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilclock "k8s.io/apimachinery/pkg/util/clock"
)

func newCondition(t string, status metav1.ConditionStatus, reason, msg string, lt time.Time) metav1.Condition {
	return metav1.Condition{
		Type:               t,
		Status:             status,
		Reason:             reason,
		Message:            msg,
		LastTransitionTime: metav1.NewTime(lt),
	}
}

func TestComputeContourAvailableCondition(t *testing.T) {
	testCases := []struct {
		description      string
		deployConditions []appsv1.DeploymentCondition
		dsAvailable      int32
		expect           metav1.Condition
	}{
		{
			description: "deployment available condition unknown, daemonset unavailable",
			deployConditions: []appsv1.DeploymentCondition{
				{Type: appsv1.DeploymentProgressing, Status: corev1.ConditionTrue},
			},
			dsAvailable: int32(0),
			expect: metav1.Condition{
				Type:   operatorv1alpha1.ContourAvailableConditionType,
				Status: metav1.ConditionUnknown,
			},
		},
		{
			description: "deployment failure, daemonset unavailable",
			deployConditions: []appsv1.DeploymentCondition{
				{Type: appsv1.DeploymentReplicaFailure, Status: corev1.ConditionTrue},
			},
			dsAvailable: int32(0),
			expect: metav1.Condition{
				Type:   operatorv1alpha1.ContourAvailableConditionType,
				Status: metav1.ConditionUnknown,
			},
		},
		{
			description: "deployment available, daemonset unavailable",
			deployConditions: []appsv1.DeploymentCondition{
				{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue},
			},
			dsAvailable: int32(0),
			expect: metav1.Condition{
				Type:   operatorv1alpha1.ContourAvailableConditionType,
				Status: metav1.ConditionFalse,
			},
		},
		{
			description: "deployment unavailable, daemonset available",
			deployConditions: []appsv1.DeploymentCondition{
				{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionFalse},
			},
			dsAvailable: int32(1),
			expect: metav1.Condition{
				Type:   operatorv1alpha1.ContourAvailableConditionType,
				Status: metav1.ConditionFalse,
			},
		},
		{
			description: "deployment available, daemonset available",
			deployConditions: []appsv1.DeploymentCondition{
				{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue},
			},
			dsAvailable: int32(1),
			expect: metav1.Condition{
				Type:   operatorv1alpha1.ContourAvailableConditionType,
				Status: metav1.ConditionTrue,
			},
		},
	}

	for i, tc := range testCases {
		deploy := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("contour-%d", i+1),
			},
			Status: appsv1.DeploymentStatus{
				Conditions: tc.deployConditions,
			},
		}

		ds := &appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("contour-%d", i+1),
			},
			Status: appsv1.DaemonSetStatus{
				NumberAvailable: tc.dsAvailable,
			},
		}

		actual := computeContourAvailableCondition(deploy, ds)
		if !apiequality.Semantic.DeepEqual(actual.Type, tc.expect.Type) ||
			!apiequality.Semantic.DeepEqual(actual.Status, tc.expect.Status) {
			t.Fatalf("%q: expected %#v, got %#v", tc.description, tc.expect, actual)
		}
	}
}

func TestContourConditionChanged(t *testing.T) {
	testCases := []struct {
		description string
		expected    bool
		a, b        metav1.Condition
	}{
		{
			description: "nil and non-nil current are equal",
			expected:    false,
			a:           metav1.Condition{},
		},
		{
			description: "empty slices should be equal",
			expected:    false,
			a:           metav1.Condition{},
			b:           metav1.Condition{},
		},
		{
			description: "condition LastTransitionTime should be ignored",
			expected:    false,
			a: metav1.Condition{
				Type:               operatorv1alpha1.ContourAvailableConditionType,
				Status:             metav1.ConditionTrue,
				LastTransitionTime: metav1.Unix(0, 0),
			},
			b: metav1.Condition{
				Type:               operatorv1alpha1.ContourAvailableConditionType,
				Status:             metav1.ConditionTrue,
				LastTransitionTime: metav1.Unix(1, 0),
			},
		},
		{
			description: "check condition reason differs",
			expected:    true,
			a: metav1.Condition{
				Type:   operatorv1alpha1.ContourAvailableConditionType,
				Status: metav1.ConditionFalse,
				Reason: "foo",
			},
			b: metav1.Condition{
				Type:   operatorv1alpha1.ContourAvailableConditionType,
				Status: metav1.ConditionFalse,
				Reason: "bar",
			},
		},
		{
			description: "condition status differs",
			expected:    true,
			a: metav1.Condition{
				Type:   operatorv1alpha1.ContourAvailableConditionType,
				Status: metav1.ConditionTrue,
			},
			b: metav1.Condition{
				Type:   operatorv1alpha1.ContourAvailableConditionType,
				Status: metav1.ConditionFalse,
			},
		},
	}

	for _, tc := range testCases {
		if actual := conditionChanged(tc.a, tc.b); actual != tc.expected {
			t.Fatalf("%q: expected %v, got %v", tc.description, tc.expected, actual)
		}
	}
}

func TestMergeConditions(t *testing.T) {
	// Inject a fake clock and don't forget to reset it
	fakeClock := utilclock.NewFakeClock(time.Time{})
	clock = fakeClock
	defer func() {
		clock = utilclock.RealClock{}
	}()

	start := fakeClock.Now()
	middle := start.Add(1 * time.Minute)
	later := start.Add(2 * time.Minute)

	testCases := []struct {
		description string
		current     []metav1.Condition
		updates     []metav1.Condition
		expected    []metav1.Condition
	}{
		{
			description: "status updated",
			current: []metav1.Condition{
				newCondition("available", "false", "Reason", "Message", start),
			},
			updates: []metav1.Condition{
				newCondition("available", "true", "Reason", "Message", middle),
			},
			expected: []metav1.Condition{
				newCondition("available", "true", "Reason", "Message", later),
			},
		},
		{
			description: "reason updated",
			current: []metav1.Condition{
				newCondition("available", "false", "Reason", "Message", start),
			},
			updates: []metav1.Condition{
				newCondition("available", "false", "New Reason", "Message", middle),
			},
			expected: []metav1.Condition{
				newCondition("available", "false", "New Reason", "Message", start),
			},
		},
		{
			description: "message updated",
			current: []metav1.Condition{
				newCondition("available", "false", "Reason", "Message", start),
			},
			updates: []metav1.Condition{
				newCondition("available", "false", "Reason", "New Message", middle),
			},
			expected: []metav1.Condition{
				newCondition("available", "false", "Reason", "New Message", start),
			},
		},
	}

	// Simulate the passage of time between original condition creation
	// and update processing
	fakeClock.SetTime(later)

	for _, tc := range testCases {
		actual := mergeConditions(tc.current, tc.updates...)
		if conditionChanged(tc.expected[0], actual[0]) {
			t.Errorf("expected:\n%v\nactual:\n%v", tc.expected, actual)
		}
	}
}
