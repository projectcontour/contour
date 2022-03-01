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

package labels

import (
	"testing"

	operatorv1alpha1 "github.com/projectcontour/contour/internal/provisioner/api"
)

func TestExist(t *testing.T) {
	testCases := []struct {
		description string
		current     map[string]string
		exist       map[string]string
		expected    bool
	}{
		{
			description: "one matched label",
			current:     map[string]string{"name": "foo"},
			exist:       map[string]string{"name": "foo"},
			expected:    true,
		},
		{
			description: "one of two matched labels",
			current:     map[string]string{"name": "foo"},
			exist:       map[string]string{"name": "foo", "ns": "foo-ns"},
			expected:    false,
		},
		{
			description: "two matched labels",
			current:     map[string]string{"name": "foo", "ns": "foo-ns"},
			exist:       map[string]string{"name": "foo", "ns": "foo-ns"},
			expected:    true,
		},
		{
			description: "four labels, two matched",
			current:     map[string]string{"name": "foo", "ns": "foo-ns", "bar": "baz", "biz": "bar"},
			exist:       map[string]string{"name": "foo", "ns": "foo-ns"},
			expected:    true,
		},
		{
			description: "one unmatched label",
			current:     map[string]string{"foo": "baz"},
			exist:       map[string]string{"foo": "bar"},
			expected:    false,
		},
		{
			description: "two unmatched labels",
			current:     map[string]string{"name": "bar"},
			exist:       map[string]string{"name": "bar", "ns": "foo-ns"},
			expected:    false,
		},
	}

	contour := operatorv1alpha1.Contour{}
	for _, tc := range testCases {
		contour.Labels = tc.current
		result := Exist(&contour, tc.exist)
		if result != tc.expected {
			t.Fatalf("%q: returned %t, expected %t.", tc.description, result, tc.expected)
		}
	}
}
