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

package dag

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/types"
)

func TestGetNamespacedName(t *testing.T) {
	testCases := []struct {
		name                   string
		input                  string
		expectedNamespacedName types.NamespacedName
		expectError            bool
	}{
		{
			name:                   "valid namespaced name",
			input:                  "foo/bar",
			expectedNamespacedName: types.NamespacedName{Namespace: "foo", Name: "bar"},
			expectError:            false,
		},
		{
			name:                   "invalid namespaced name",
			input:                  "foo",
			expectedNamespacedName: types.NamespacedName{},
			expectError:            true,
		},
		{
			name:                   "invalid namespaced name",
			input:                  "foo/bar/baz",
			expectedNamespacedName: types.NamespacedName{},
			expectError:            true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			a := assert.New(t)
			actual, err := getNamespacedName(tc.input)
			a.Equal(tc.expectedNamespacedName, actual)
			a.Equal(tc.expectError, err != nil)
		})
	}
}
