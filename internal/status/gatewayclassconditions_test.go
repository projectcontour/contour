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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	realclock "k8s.io/utils/clock"
	fakeclock "k8s.io/utils/clock/testing"
	gatewayapi_v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

func TestComputeGatewayClassAcceptedCondition(t *testing.T) {
	testCases := []struct {
		name     string
		accepted bool
		expect   metav1.Condition
	}{
		{
			name: "valid gatewayclass",

			accepted: true,
			expect: metav1.Condition{
				Type:   string(gatewayapi_v1beta1.GatewayClassConditionStatusAccepted),
				Status: metav1.ConditionTrue,
				Reason: reasonValidGatewayClass,
			},
		},
		{
			name:     "invalid gatewayclass",
			accepted: false,
			expect: metav1.Condition{
				Type:   string(gatewayapi_v1beta1.GatewayClassConditionStatusAccepted),
				Status: metav1.ConditionFalse,
				Reason: reasonInvalidGatewayClass,
			},
		},
	}

	for _, tc := range testCases {
		gc := &gatewayapi_v1beta1.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{
				Generation: 7,
			},
		}

		got := computeGatewayClassAcceptedCondition(gc, tc.accepted)

		assert.Equal(t, tc.expect.Type, got.Type)
		assert.Equal(t, tc.expect.Status, got.Status)
		assert.Equal(t, tc.expect.Reason, got.Reason)
		assert.Equal(t, gc.Generation, got.ObservedGeneration)
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
				Type:               string(gatewayapi_v1beta1.GatewayClassConditionStatusAccepted),
				Status:             metav1.ConditionTrue,
				LastTransitionTime: metav1.Unix(0, 0),
			},
			b: metav1.Condition{
				Type:               string(gatewayapi_v1beta1.GatewayClassConditionStatusAccepted),
				Status:             metav1.ConditionTrue,
				LastTransitionTime: metav1.Unix(1, 0),
			},
		},
		{
			name:     "check condition reason differs",
			expected: true,
			a: metav1.Condition{
				Type:   string(gatewayapi_v1beta1.GatewayConditionReady),
				Status: metav1.ConditionFalse,
				Reason: "foo",
			},
			b: metav1.Condition{
				Type:   string(gatewayapi_v1beta1.GatewayConditionReady),
				Status: metav1.ConditionFalse,
				Reason: "bar",
			},
		},
		{
			name:     "condition status differs",
			expected: true,
			a: metav1.Condition{
				Type:   string(gatewayapi_v1beta1.GatewayClassConditionStatusAccepted),
				Status: metav1.ConditionTrue,
			},
			b: metav1.Condition{
				Type:   string(gatewayapi_v1beta1.GatewayClassConditionStatusAccepted),
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
	fakeClock := fakeclock.NewFakeClock(time.Time{})
	clock = fakeClock
	defer func() {
		clock = realclock.RealClock{}
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
