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
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayapi_v1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestComputeGatewayClassAcceptedCondition(t *testing.T) {
	testCases := []struct {
		name     string
		accepted bool
		expect   meta_v1.Condition
	}{
		{
			name:     "accepted gatewayclass",
			accepted: true,
			expect: meta_v1.Condition{
				Type:   string(gatewayapi_v1.GatewayClassConditionStatusAccepted),
				Status: meta_v1.ConditionTrue,
				Reason: string(gatewayapi_v1.GatewayClassReasonAccepted),
			},
		},
		{
			name:     "not accepted gatewayclass",
			accepted: false,
			expect: meta_v1.Condition{
				Type:   string(gatewayapi_v1.GatewayClassConditionStatusAccepted),
				Status: meta_v1.ConditionFalse,
				Reason: string(ReasonOlderGatewayClassExists),
			},
		},
	}

	for _, tc := range testCases {
		gc := &gatewayapi_v1.GatewayClass{
			ObjectMeta: meta_v1.ObjectMeta{
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
		a, b     meta_v1.Condition
	}{
		{
			name:     "nil and non-nil current are equal",
			expected: false,
			a:        meta_v1.Condition{},
		},
		{
			name:     "empty slices should be equal",
			expected: false,
			a:        meta_v1.Condition{},
			b:        meta_v1.Condition{},
		},
		{
			name:     "condition LastTransitionTime should be ignored",
			expected: false,
			a: meta_v1.Condition{
				Type:               string(gatewayapi_v1.GatewayClassConditionStatusAccepted),
				Status:             meta_v1.ConditionTrue,
				LastTransitionTime: meta_v1.Unix(0, 0),
			},
			b: meta_v1.Condition{
				Type:               string(gatewayapi_v1.GatewayClassConditionStatusAccepted),
				Status:             meta_v1.ConditionTrue,
				LastTransitionTime: meta_v1.Unix(1, 0),
			},
		},
		{
			name:     "check condition reason differs",
			expected: true,
			a: meta_v1.Condition{
				Type:   string(gatewayapi_v1.GatewayConditionProgrammed),
				Status: meta_v1.ConditionFalse,
				Reason: "foo",
			},
			b: meta_v1.Condition{
				Type:   string(gatewayapi_v1.GatewayConditionProgrammed),
				Status: meta_v1.ConditionFalse,
				Reason: "bar",
			},
		},
		{
			name:     "condition status differs",
			expected: true,
			a: meta_v1.Condition{
				Type:   string(gatewayapi_v1.GatewayClassConditionStatusAccepted),
				Status: meta_v1.ConditionTrue,
			},
			b: meta_v1.Condition{
				Type:   string(gatewayapi_v1.GatewayClassConditionStatusAccepted),
				Status: meta_v1.ConditionFalse,
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
	start := time.Now()
	later := start.Add(2 * time.Minute)

	testCases := []struct {
		name     string
		current  []meta_v1.Condition
		updates  []meta_v1.Condition
		expected []meta_v1.Condition
	}{
		{
			name: "status updated",
			current: []meta_v1.Condition{
				newCondition("available", "false", "Reason", "Message", start),
			},
			updates: []meta_v1.Condition{
				newCondition("available", "true", "Reason", "Message", later),
			},
			expected: []meta_v1.Condition{
				newCondition("available", "true", "Reason", "Message", later),
			},
		},
		{
			name: "reason updated",
			current: []meta_v1.Condition{
				newCondition("available", "false", "Reason", "Message", start),
			},
			updates: []meta_v1.Condition{
				newCondition("available", "false", "New Reason", "Message", later),
			},
			expected: []meta_v1.Condition{
				newCondition("available", "false", "New Reason", "Message", start),
			},
		},
		{
			name: "message updated",
			current: []meta_v1.Condition{
				newCondition("available", "false", "Reason", "Message", start),
			},
			updates: []meta_v1.Condition{
				newCondition("available", "false", "Reason", "New Message", later),
			},
			expected: []meta_v1.Condition{
				newCondition("available", "false", "Reason", "New Message", start),
			},
		},
		{
			name:    "new status",
			current: []meta_v1.Condition{},
			updates: []meta_v1.Condition{
				newCondition("available", "false", "Reason", "New Message", later),
			},
			expected: []meta_v1.Condition{
				newCondition("available", "false", "Reason", "New Message", later),
			},
		},
	}

	for _, tc := range testCases {
		got := mergeConditions(tc.current, tc.updates...)
		assert.Equal(t, tc.expected, got, tc.name)
	}
}

func TestConditionsEqual(t *testing.T) {
	testCases := []struct {
		name     string
		expected bool
		a, b     []meta_v1.Condition
	}{
		{
			name:     "zero-valued status should be equal",
			expected: true,
		},
		{
			name:     "nil and non-nil slices should be equal",
			expected: true,
			a:        []meta_v1.Condition{},
		},
		{
			name:     "empty slices should be equal",
			expected: true,
			a:        []meta_v1.Condition{},
			b:        []meta_v1.Condition{},
		},
		{
			name:     "condition LastTransitionTime should not be ignored",
			expected: false,
			a: []meta_v1.Condition{
				{
					Type:               "foo",
					Status:             meta_v1.ConditionTrue,
					LastTransitionTime: meta_v1.Unix(0, 0),
				},
			},
			b: []meta_v1.Condition{
				{
					Type:               "foo",
					Status:             meta_v1.ConditionTrue,
					LastTransitionTime: meta_v1.Unix(1, 0),
				},
			},
		},
		{
			name:     "check condition types differ",
			expected: false,
			a: []meta_v1.Condition{
				{
					Type:   "foo",
					Status: meta_v1.ConditionTrue,
				},
			},
			b: []meta_v1.Condition{
				{
					Type:   "bar",
					Status: meta_v1.ConditionTrue,
				},
			},
		},
		{
			name:     "check condition status differs",
			expected: false,
			a: []meta_v1.Condition{
				{
					Type:   "foo",
					Status: meta_v1.ConditionTrue,
				},
			},
			b: []meta_v1.Condition{
				{
					Type:   "foo",
					Status: meta_v1.ConditionFalse,
				},
			},
		},
		{
			name:     "check condition reasons differ",
			expected: false,
			a: []meta_v1.Condition{
				{
					Type:   "foo",
					Status: meta_v1.ConditionFalse,
					Reason: "foo",
				},
			},
			b: []meta_v1.Condition{
				{
					Type:   "foo",
					Status: meta_v1.ConditionFalse,
					Reason: "bar",
				},
			},
		},
		{
			name:     "check duplicate of a single condition type",
			expected: false,
			a: []meta_v1.Condition{
				{
					Type: "foo",
				},
			},
			b: []meta_v1.Condition{
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
			a: []meta_v1.Condition{
				{
					Type:   "foo",
					Status: meta_v1.ConditionTrue,
				},
			},
			b: []meta_v1.Condition{
				{
					Type:   "foo",
					Status: meta_v1.ConditionTrue,
				},
				{
					Type:   "bar",
					Status: meta_v1.ConditionTrue,
				},
			},
		},
		{
			name:     "check condition removed",
			expected: false,
			a: []meta_v1.Condition{
				{
					Type:   "foo",
					Status: meta_v1.ConditionTrue,
				},
				{
					Type:   "bar",
					Status: meta_v1.ConditionTrue,
				},
			},
			b: []meta_v1.Condition{
				{
					Type:   "foo",
					Status: meta_v1.ConditionTrue,
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
