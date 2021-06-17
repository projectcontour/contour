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

	projectcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/k8s"

	"github.com/stretchr/testify/assert"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilclock "k8s.io/apimachinery/pkg/util/clock"
	"k8s.io/apimachinery/pkg/util/validation/field"
	gatewayapi_v1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

func TestHTTPRouteAddCondition(t *testing.T) {

	var testGeneration int64 = 7

	simpleValidCondition := metav1.Condition{
		Type:               string(gatewayapi_v1alpha1.ConditionRouteAdmitted),
		Status:             projectcontour.ConditionTrue,
		Reason:             "Valid",
		Message:            "Valid HTTPRoute",
		ObservedGeneration: testGeneration,
	}

	httpRouteUpdate := ConditionsUpdate{
		FullName:   k8s.NamespacedNameFrom("test/test"),
		Generation: testGeneration,
		Conditions: make(map[gatewayapi_v1alpha1.RouteConditionType]metav1.Condition),
	}

	got := httpRouteUpdate.AddCondition(gatewayapi_v1alpha1.ConditionRouteAdmitted, metav1.ConditionTrue, "Valid", "Valid HTTPRoute")

	assert.Equal(t, simpleValidCondition.Message, got.Message)
	assert.Equal(t, simpleValidCondition.Reason, got.Reason)
	assert.Equal(t, simpleValidCondition.Type, got.Type)
	assert.Equal(t, simpleValidCondition.Status, got.Status)
	assert.Equal(t, simpleValidCondition.ObservedGeneration, got.ObservedGeneration)
}

func newCondition(t string, status metav1.ConditionStatus, reason, msg string, lt time.Time) metav1.Condition {
	return metav1.Condition{
		Type:               t,
		Status:             status,
		Reason:             reason,
		Message:            msg,
		LastTransitionTime: metav1.NewTime(lt),
	}
}

func TestComputeGatewayClassAdmittedCondition(t *testing.T) {
	var errs field.ErrorList
	testCases := []struct {
		name   string
		errs   field.ErrorList
		expect metav1.Condition
	}{
		{
			name: "valid gatewayclass",
			errs: nil,
			expect: metav1.Condition{
				Type:   string(gatewayapi_v1alpha1.GatewayClassConditionStatusAdmitted),
				Status: metav1.ConditionTrue,
				Reason: reasonValidGatewayClass,
			},
		},
		{
			name: "invalid gatewayclass",
			errs: append(errs, field.InternalError(field.NewPath("spec"), fmt.Errorf("foo"))),
			expect: metav1.Condition{
				Type:   string(gatewayapi_v1alpha1.GatewayClassConditionStatusAdmitted),
				Status: metav1.ConditionFalse,
				Reason: reasonInvalidGatewayClass,
			},
		},
	}

	for _, tc := range testCases {
		got := computeGatewayClassAdmittedCondition(tc.errs)
		if !apiequality.Semantic.DeepEqual(got.Type, tc.expect.Type) ||
			!apiequality.Semantic.DeepEqual(got.Status, tc.expect.Status) ||
			!apiequality.Semantic.DeepEqual(got.Reason, tc.expect.Reason) {
			assert.Equal(t, tc.expect, got, tc.name)
		}
	}
}

func TestComputeGatewayScheduledCondition(t *testing.T) {
	var errs field.ErrorList

	testCases := map[string]struct {
		errs   field.ErrorList
		expect metav1.Condition
	}{
		"valid gateway": {
			errs: nil,
			expect: metav1.Condition{
				Type:   string(gatewayapi_v1alpha1.GatewayConditionScheduled),
				Status: metav1.ConditionTrue,
				Reason: reasonValidGateway,
			},
		},
		"invalid gateway": {
			errs: append(errs, field.InternalError(field.NewPath("spec"), fmt.Errorf("foo"))),
			expect: metav1.Condition{
				Type:   string(gatewayapi_v1alpha1.GatewayConditionScheduled),
				Status: metav1.ConditionFalse,
				Reason: reasonInvalidGateway,
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got := computeGatewayScheduledCondition(tc.errs)
			if !apiequality.Semantic.DeepEqual(got.Type, tc.expect.Type) ||
				!apiequality.Semantic.DeepEqual(got.Status, tc.expect.Status) ||
				!apiequality.Semantic.DeepEqual(got.Reason, tc.expect.Reason) {
				assert.Equal(t, tc.expect, got, name)
			}
		})
	}
}

func TestConditionChanged(t *testing.T) {
	testCases := []struct {
		name     string
		expected bool
		a, b     metav1.Condition
	}{
		{
			name:     "nil and non-nil current are equal",
			expected: false,
			a:        metav1.Condition{},
		},
		{
			name:     "empty slices should be equal",
			expected: false,
			a:        metav1.Condition{},
			b:        metav1.Condition{},
		},
		{
			name:     "condition LastTransitionTime should be ignored",
			expected: false,
			a: metav1.Condition{
				Type:               string(gatewayapi_v1alpha1.GatewayClassConditionStatusAdmitted),
				Status:             metav1.ConditionTrue,
				LastTransitionTime: metav1.Unix(0, 0),
			},
			b: metav1.Condition{
				Type:               string(gatewayapi_v1alpha1.GatewayClassConditionStatusAdmitted),
				Status:             metav1.ConditionTrue,
				LastTransitionTime: metav1.Unix(1, 0),
			},
		},
		{
			name:     "check condition reason differs",
			expected: true,
			a: metav1.Condition{
				Type:   string(gatewayapi_v1alpha1.GatewayConditionReady),
				Status: metav1.ConditionFalse,
				Reason: "foo",
			},
			b: metav1.Condition{
				Type:   string(gatewayapi_v1alpha1.GatewayConditionReady),
				Status: metav1.ConditionFalse,
				Reason: "bar",
			},
		},
		{
			name:     "condition status differs",
			expected: true,
			a: metav1.Condition{
				Type:   string(gatewayapi_v1alpha1.GatewayClassConditionStatusAdmitted),
				Status: metav1.ConditionTrue,
			},
			b: metav1.Condition{
				Type:   string(gatewayapi_v1alpha1.GatewayClassConditionStatusAdmitted),
				Status: metav1.ConditionFalse,
			},
		},
	}

	for _, tc := range testCases {
		if got := conditionChanged(tc.a, tc.b); got != tc.expected {
			assert.Equal(t, tc.expected, got, tc.name)
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
		name     string
		current  []metav1.Condition
		updates  []metav1.Condition
		expected []metav1.Condition
	}{
		{
			name: "status updated",
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
			name: "reason updated",
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
			name: "message updated",
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
		got := mergeConditions(tc.current, tc.updates...)
		if conditionChanged(tc.expected[0], got[0]) {
			assert.Equal(t, tc.expected, got, tc.name)
		}
	}
}

func TestConditionsEqual(t *testing.T) {
	testCases := []struct {
		name     string
		expected bool
		a, b     []metav1.Condition
	}{
		{
			name:     "zero-valued status should be equal",
			expected: true,
		},
		{
			name:     "nil and non-nil slices should be equal",
			expected: true,
			a:        []metav1.Condition{},
		},
		{
			name:     "empty slices should be equal",
			expected: true,
			a:        []metav1.Condition{},
			b:        []metav1.Condition{},
		},
		{
			name:     "condition LastTransitionTime should not be ignored",
			expected: false,
			a: []metav1.Condition{
				{
					Type:               "foo",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: metav1.Unix(0, 0),
				},
			},
			b: []metav1.Condition{
				{
					Type:               "foo",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: metav1.Unix(1, 0),
				},
			},
		},
		{
			name:     "check condition types differ",
			expected: false,
			a: []metav1.Condition{
				{
					Type:   "foo",
					Status: metav1.ConditionTrue,
				},
			},
			b: []metav1.Condition{
				{
					Type:   "bar",
					Status: metav1.ConditionTrue,
				},
			},
		},
		{
			name:     "check condition status differs",
			expected: false,
			a: []metav1.Condition{
				{
					Type:   "foo",
					Status: metav1.ConditionTrue,
				},
			},
			b: []metav1.Condition{
				{
					Type:   "foo",
					Status: metav1.ConditionFalse,
				},
			},
		},
		{
			name:     "check condition reasons differ",
			expected: false,
			a: []metav1.Condition{
				{
					Type:   "foo",
					Status: metav1.ConditionFalse,
					Reason: "foo",
				},
			},
			b: []metav1.Condition{
				{
					Type:   "foo",
					Status: metav1.ConditionFalse,
					Reason: "bar",
				},
			},
		},
		{
			name:     "check duplicate of a single condition type",
			expected: false,
			a: []metav1.Condition{
				{
					Type: "foo",
				},
			},
			b: []metav1.Condition{
				{
					Type: "foo",
				},
				{
					Type: "foo",
				},
			},
		},
		{
			name:     "check new condition added",
			expected: false,
			a: []metav1.Condition{
				{
					Type:   "foo",
					Status: metav1.ConditionTrue,
				},
			},
			b: []metav1.Condition{
				{
					Type:   "foo",
					Status: metav1.ConditionTrue,
				},
				{
					Type:   "bar",
					Status: metav1.ConditionTrue,
				},
			},
		},
		{
			name:     "check condition removed",
			expected: false,
			a: []metav1.Condition{
				{
					Type:   "foo",
					Status: metav1.ConditionTrue,
				},
				{
					Type:   "bar",
					Status: metav1.ConditionTrue,
				},
			},
			b: []metav1.Condition{
				{
					Type:   "foo",
					Status: metav1.ConditionTrue,
				},
			},
		},
	}

	for _, tc := range testCases {
		if got := conditionsEqual(tc.a, tc.b); got != tc.expected {
			assert.Equal(t, tc.expected, got, tc.name)
		}
	}
}
