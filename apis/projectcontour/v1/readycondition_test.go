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

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestReadyDetailedConditionFromValid(t *testing.T) {
	now := meta_v1.NewTime(time.Now())

	tests := map[string]struct {
		valid *DetailedCondition
		want  *DetailedCondition
	}{
		"nil input returns nil": {
			valid: nil,
			want:  nil,
		},
		"valid condition with errors and warnings": {
			valid: &DetailedCondition{
				Condition: Condition{
					Type:               ValidConditionType,
					Status:             ConditionFalse,
					Reason:             "MultipleReasons",
					Message:            "Multiple errors and warnings",
					LastTransitionTime: now,
					ObservedGeneration: 7,
				},
				Errors: []SubCondition{
					{
						Type:    ConditionTypeServiceError,
						Status:  ConditionTrue,
						Reason:  "ServiceNotFound",
						Message: "Service 'foo' not found",
					},
				},
				Warnings: []SubCondition{
					{
						Type:    ConditionTypeTLSError,
						Status:  ConditionTrue,
						Reason:  "TLSDeprecation",
						Message: "TLS 1.0 is deprecated",
					},
				},
			},
			want: &DetailedCondition{
				Condition: Condition{
					Type:               ReadyConditionType,
					Status:             ConditionFalse,
					Reason:             "MultipleReasons",
					Message:            "Multiple errors and warnings",
					LastTransitionTime: now,
					ObservedGeneration: 7,
				},
				Errors: []SubCondition{
					{
						Type:    ConditionTypeServiceError,
						Status:  ConditionTrue,
						Reason:  "ServiceNotFound",
						Message: "Service 'foo' not found",
					},
				},
				Warnings: []SubCondition{
					{
						Type:    ConditionTypeTLSError,
						Status:  ConditionTrue,
						Reason:  "TLSDeprecation",
						Message: "TLS 1.0 is deprecated",
					},
				},
			},
		},
		"valid condition without errors or warnings": {
			valid: &DetailedCondition{
				Condition: Condition{
					Type:               ValidConditionType,
					Status:             ConditionTrue,
					Reason:             "Valid",
					Message:            "Valid HTTPProxy",
					LastTransitionTime: now,
					ObservedGeneration: 1,
				},
			},
			want: &DetailedCondition{
				Condition: Condition{
					Type:               ReadyConditionType,
					Status:             ConditionTrue,
					Reason:             "Valid",
					Message:            "Valid HTTPProxy",
					LastTransitionTime: now,
					ObservedGeneration: 1,
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := ReadyDetailedConditionFromValid(tc.valid)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestReadyConditionTypeConstant(t *testing.T) {
	// kstatus specifically looks for "Ready" condition type
	// This test ensures the constant is correct for kstatus compatibility
	require.Equal(t, "Ready", ReadyConditionType, "ReadyConditionType must be 'Ready' for kstatus compatibility")
}

func TestReadyConditionIsIndependentCopy(t *testing.T) {
	// Ensure that modifying the returned Ready condition doesn't affect the original Valid condition
	now := meta_v1.NewTime(time.Now())

	valid := &DetailedCondition{
		Condition: Condition{
			Type:               ValidConditionType,
			Status:             ConditionTrue,
			Reason:             "Valid",
			Message:            "Valid HTTPProxy",
			LastTransitionTime: now,
			ObservedGeneration: 1,
		},
		Errors: []SubCondition{
			{
				Type:    ConditionTypeServiceError,
				Status:  ConditionTrue,
				Reason:  "TestReason",
				Message: "Test message",
			},
		},
	}

	ready := ReadyDetailedConditionFromValid(valid)
	require.NotNil(t, ready)

	// Modify the ready condition
	ready.Message = "Modified message"
	ready.Errors[0].Message = "Modified error message"

	// Verify original is unchanged
	assert.Equal(t, "Valid HTTPProxy", valid.Message)
	assert.Equal(t, "Test message", valid.Errors[0].Message)
}
