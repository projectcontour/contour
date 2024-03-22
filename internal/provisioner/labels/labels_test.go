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

	"github.com/stretchr/testify/assert"

	"github.com/projectcontour/contour/internal/provisioner/model"
)

func TestAnyExist(t *testing.T) {
	testCases := []struct {
		description string
		current     map[string]string
		exist       map[string]string
		expected    bool
	}{
		{
			description: "nil labels",
			current:     nil,
			exist:       map[string]string{"name": "foo"},
			expected:    false,
		},
		{
			description: "empty labels",
			current:     map[string]string{},
			exist:       map[string]string{"name": "foo"},
			expected:    false,
		},
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
			expected:    true,
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
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			contour := model.Contour{}
			contour.Labels = tc.current

			assert.Equal(t, tc.expected, AnyExist(&contour, tc.exist))
		})
	}
}
