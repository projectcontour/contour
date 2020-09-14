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

package v1

// This file is intended to lock in the API for the code in helpers.go
// If you change the tests in this file, you must consider whether you need
// to update the version to v2alpha1 at least.

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type subConditionDetails struct {
	condType string
	reason   string
	message  string
}

func TestAddErrorConditions(t *testing.T) {

	tests := map[string]struct {
		dc            *DetailedCondition
		subconditions []subConditionDetails
		want          *DetailedCondition
	}{
		"basic error add, negative polarity": {
			dc: &DetailedCondition{
				Condition: Condition{
					Type: "AnError",
				},
			},
			subconditions: []subConditionDetails{
				{
					condType: "SimpleTest",
					reason:   "TestReason",
					message:  "We had a straightforward error",
				},
			},
			want: &DetailedCondition{
				Condition: Condition{
					Type:    "AnError",
					Status:  ConditionTrue,
					Reason:  "SimpleTestTestReason",
					Message: "We had a straightforward error",
				},
				Errors: []SubCondition{
					{
						Type:    "SimpleTest",
						Reason:  "TestReason",
						Message: "We had a straightforward error",
						Status:  ConditionTrue,
					},
				},
			},
		},
		"basic error add, Positive polarity": {
			dc: &DetailedCondition{
				Condition: Condition{
					Type: "Valid",
				},
			},
			subconditions: []subConditionDetails{
				{
					condType: "SimpleTest",
					reason:   "TestReason",
					message:  "We had a straightforward error",
				},
			},
			want: &DetailedCondition{
				Condition: Condition{
					Type:    "Valid",
					Status:  ConditionFalse,
					Reason:  "SimpleTestTestReason",
					Message: "We had a straightforward error",
				},
				Errors: []SubCondition{
					{
						Type:    "SimpleTest",
						Reason:  "TestReason",
						Message: "We had a straightforward error",
						Status:  ConditionTrue,
					},
				},
			},
		},

		"multiple reason, multiple type": {
			dc: &DetailedCondition{
				Condition: Condition{
					Type: "Valid",
				},
			},
			subconditions: []subConditionDetails{
				{
					condType: "SimpleTest",
					reason:   "TestReason",
					message:  "We had a straightforward error",
				},
				{
					condType: "SecondTest",
					reason:   "TestReason2",
					message:  "We had an extra straightforward error",
				},
			},
			want: &DetailedCondition{
				Condition: Condition{
					Type:    "Valid",
					Status:  ConditionFalse,
					Reason:  "MultipleProblems",
					Message: "Multiple problems were found, see errors for details",
				},
				Errors: []SubCondition{
					{
						Type:    "SimpleTest",
						Reason:  "TestReason",
						Message: "We had a straightforward error",
						Status:  ConditionTrue,
					},
					{
						Type:    "SecondTest",
						Reason:  "TestReason2",
						Message: "We had an extra straightforward error",
						Status:  ConditionTrue,
					},
				},
			},
		},
		"same reason, multiple type": {
			dc: &DetailedCondition{
				Condition: Condition{
					Type: "Valid",
				},
			},
			subconditions: []subConditionDetails{
				{
					condType: "SimpleTest",
					reason:   "TestReason",
					message:  "We had a straightforward error",
				},
				{
					condType: "SecondTest",
					reason:   "TestReason",
					message:  "We had an extra straightforward error",
				},
			},
			want: &DetailedCondition{
				Condition: Condition{
					Type:    "Valid",
					Status:  ConditionFalse,
					Reason:  "MultipleProblems",
					Message: "Multiple problems were found, see errors for details",
				},
				Errors: []SubCondition{
					{
						Type:    "SimpleTest",
						Reason:  "TestReason",
						Message: "We had a straightforward error",
						Status:  ConditionTrue,
					},
					{
						Type:    "SecondTest",
						Reason:  "TestReason",
						Message: "We had an extra straightforward error",
						Status:  ConditionTrue,
					},
				},
			},
		},
		"same reason, same type": {
			dc: &DetailedCondition{
				Condition: Condition{
					Type: "Valid",
				},
			},
			subconditions: []subConditionDetails{
				{
					condType: "SimpleTest",
					reason:   "TestReason",
					message:  "We had a straightforward error",
				},
				{
					condType: "SimpleTest",
					reason:   "TestReason",
					message:  "We had an extra straightforward error",
				},
			},
			want: &DetailedCondition{
				Condition: Condition{
					Type:    "Valid",
					Status:  ConditionFalse,
					Reason:  "SimpleTestTestReason",
					Message: "We had a straightforward error, We had an extra straightforward error",
				},
				Errors: []SubCondition{
					{
						Type:    "SimpleTest",
						Reason:  "TestReason",
						Message: "We had a straightforward error, We had an extra straightforward error",
						Status:  ConditionTrue,
					},
				},
			},
		},
		"multiple different reason, same type": {
			dc: &DetailedCondition{
				Condition: Condition{
					Type: "Valid",
				},
			},
			subconditions: []subConditionDetails{
				{
					condType: "SimpleTest",
					reason:   "TestReason",
					message:  "We had a straightforward error",
				},
				{
					condType: "SimpleTest",
					reason:   "TestReason2",
					message:  "We had an extra straightforward error",
				},
			},
			want: &DetailedCondition{
				Condition: Condition{
					Type:    "Valid",
					Status:  ConditionFalse,
					Reason:  "MultipleProblems",
					Message: "Multiple problems were found, see errors for details",
				},
				Errors: []SubCondition{
					{
						Type:    "SimpleTest",
						Reason:  "MultipleReasons",
						Message: "We had a straightforward error, We had an extra straightforward error",
						Status:  ConditionTrue,
					},
				},
			},
		},
	}

	for name, tc := range tests {

		for _, cond := range tc.subconditions {
			tc.dc.AddError(cond.condType, cond.reason, cond.message)
		}

		assert.Equalf(t, tc.want, tc.dc, "Add error condition failed in test %s", name)
	}
}

func TestAddWarningConditions(t *testing.T) {

	tests := map[string]struct {
		dc            *DetailedCondition
		subconditions []subConditionDetails
		want          *DetailedCondition
	}{
		"basic warning add": {
			dc: &DetailedCondition{},
			subconditions: []subConditionDetails{
				{
					condType: "SimpleTest",
					reason:   "TestReason",
					message:  "We had a straightforward warning",
				},
			},
			want: &DetailedCondition{
				Condition: Condition{
					Reason:  "SimpleTestTestReason",
					Message: "We had a straightforward warning",
				},
				Warnings: []SubCondition{
					{
						Type:    "SimpleTest",
						Reason:  "TestReason",
						Message: "We had a straightforward warning",
						Status:  ConditionTrue,
					},
				},
			},
		},
		"multiple reason, multiple type": {
			dc: &DetailedCondition{},
			subconditions: []subConditionDetails{
				{
					condType: "SimpleTest",
					reason:   "TestReason",
					message:  "We had a straightforward warning",
				},
				{
					condType: "SecondTest",
					reason:   "TestReason2",
					message:  "We had an extra straightforward warning",
				},
			},
			want: &DetailedCondition{
				Condition: Condition{
					Reason:  "MultipleProblems",
					Message: "Multiple problems were found, see errors for details",
				},
				Warnings: []SubCondition{
					{
						Type:    "SimpleTest",
						Reason:  "TestReason",
						Message: "We had a straightforward warning",
						Status:  ConditionTrue,
					},
					{
						Type:    "SecondTest",
						Reason:  "TestReason2",
						Message: "We had an extra straightforward warning",
						Status:  ConditionTrue,
					},
				},
			},
		},
		"same reason, multiple type": {
			dc: &DetailedCondition{},
			subconditions: []subConditionDetails{
				{
					condType: "SimpleTest",
					reason:   "TestReason",
					message:  "We had a straightforward warning",
				},
				{
					condType: "SecondTest",
					reason:   "TestReason",
					message:  "We had an extra straightforward warning",
				},
			},
			want: &DetailedCondition{
				Condition: Condition{
					Reason:  "MultipleProblems",
					Message: "Multiple problems were found, see errors for details",
				},
				Warnings: []SubCondition{
					{
						Type:    "SimpleTest",
						Reason:  "TestReason",
						Message: "We had a straightforward warning",
						Status:  ConditionTrue,
					},
					{
						Type:    "SecondTest",
						Reason:  "TestReason",
						Message: "We had an extra straightforward warning",
						Status:  ConditionTrue,
					},
				},
			},
		},
		"same reason, same type": {
			dc: &DetailedCondition{},
			subconditions: []subConditionDetails{
				{
					condType: "SimpleTest",
					reason:   "TestReason",
					message:  "We had a straightforward warning",
				},
				{
					condType: "SimpleTest",
					reason:   "TestReason",
					message:  "We had an extra straightforward warning",
				},
			},
			want: &DetailedCondition{
				Condition: Condition{
					Reason:  "SimpleTestTestReason",
					Message: "We had a straightforward warning, We had an extra straightforward warning",
				},
				Warnings: []SubCondition{
					{
						Type:    "SimpleTest",
						Reason:  "TestReason",
						Message: "We had a straightforward warning, We had an extra straightforward warning",
						Status:  ConditionTrue,
					},
				},
			},
		},
		"multiple different reason, same type": {
			dc: &DetailedCondition{},
			subconditions: []subConditionDetails{
				{
					condType: "SimpleTest",
					reason:   "TestReason",
					message:  "We had a straightforward warning",
				},
				{
					condType: "SimpleTest",
					reason:   "TestReason2",
					message:  "We had an extra straightforward warning",
				},
			},
			want: &DetailedCondition{
				Condition: Condition{
					Reason:  "MultipleProblems",
					Message: "Multiple problems were found, see errors for details",
				},
				Warnings: []SubCondition{
					{
						Type:    "SimpleTest",
						Reason:  "MultipleReasons",
						Message: "We had a straightforward warning, We had an extra straightforward warning",
						Status:  ConditionTrue,
					},
				},
			},
		},
	}

	for name, tc := range tests {

		for _, cond := range tc.subconditions {
			tc.dc.AddWarning(cond.condType, cond.reason, cond.message)
		}

		assert.Equalf(t, tc.want, tc.dc, "Add error condition failed in test %s", name)
	}
}
