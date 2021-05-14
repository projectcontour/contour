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

package equality

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayapi_v1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

func TestGatewayClassStatusChanged(t *testing.T) {
	testCases := []struct {
		name     string
		expected bool
		a, b     gatewayapi_v1alpha1.GatewayClassStatus
	}{
		{
			name:     "zero-valued status should be equal",
			expected: false,
		},
		{
			name:     "nil and non-nil slices should be equal",
			expected: false,
			a: gatewayapi_v1alpha1.GatewayClassStatus{
				Conditions: []metav1.Condition{},
			},
		},
		{
			name:     "empty slices should be equal",
			expected: false,
			a: gatewayapi_v1alpha1.GatewayClassStatus{
				Conditions: []metav1.Condition{},
			},
			b: gatewayapi_v1alpha1.GatewayClassStatus{
				Conditions: []metav1.Condition{},
			},
		},
		{
			name:     "condition LastTransitionTime should not be ignored",
			expected: true,
			a: gatewayapi_v1alpha1.GatewayClassStatus{
				Conditions: []metav1.Condition{
					{
						Type:               string(gatewayapi_v1alpha1.GatewayClassConditionStatusAdmitted),
						Status:             metav1.ConditionTrue,
						LastTransitionTime: metav1.Unix(0, 0),
					},
				},
			},
			b: gatewayapi_v1alpha1.GatewayClassStatus{
				Conditions: []metav1.Condition{
					{
						Type:               string(gatewayapi_v1alpha1.GatewayClassConditionStatusAdmitted),
						Status:             metav1.ConditionTrue,
						LastTransitionTime: metav1.Unix(1, 0),
					},
				},
			},
		},
		{
			name:     "check condition status differs",
			expected: true,
			a: gatewayapi_v1alpha1.GatewayClassStatus{
				Conditions: []metav1.Condition{
					{
						Type:   string(gatewayapi_v1alpha1.GatewayClassConditionStatusAdmitted),
						Status: metav1.ConditionTrue,
					},
				},
			},
			b: gatewayapi_v1alpha1.GatewayClassStatus{
				Conditions: []metav1.Condition{
					{
						Type:   string(gatewayapi_v1alpha1.GatewayClassConditionStatusAdmitted),
						Status: metav1.ConditionFalse,
					},
				},
			},
		},
		{
			name:     "check condition reason differs",
			expected: true,
			a: gatewayapi_v1alpha1.GatewayClassStatus{
				Conditions: []metav1.Condition{
					{
						Type:   string(gatewayapi_v1alpha1.GatewayClassConditionStatusAdmitted),
						Status: metav1.ConditionFalse,
						Reason: "foo",
					},
				},
			},
			b: gatewayapi_v1alpha1.GatewayClassStatus{
				Conditions: []metav1.Condition{
					{
						Type:   string(gatewayapi_v1alpha1.GatewayClassConditionStatusAdmitted),
						Status: metav1.ConditionFalse,
						Reason: "bar",
					},
				},
			},
		},
		{
			name:     "check duplicate with single condition",
			expected: true,
			a: gatewayapi_v1alpha1.GatewayClassStatus{
				Conditions: []metav1.Condition{
					{
						Type:    string(gatewayapi_v1alpha1.GatewayClassConditionStatusAdmitted),
						Message: "foo",
					},
				},
			},
			b: gatewayapi_v1alpha1.GatewayClassStatus{
				Conditions: []metav1.Condition{
					{
						Type:    string(gatewayapi_v1alpha1.GatewayClassConditionStatusAdmitted),
						Message: "foo",
					},
					{
						Type:    string(gatewayapi_v1alpha1.GatewayClassConditionStatusAdmitted),
						Message: "foo",
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		if actual := GatewayClassStatusChanged(tc.a, tc.b); actual != tc.expected {
			t.Fatalf("%q: expected %v, got %v", tc.name, tc.expected, actual)
		}
	}
}
