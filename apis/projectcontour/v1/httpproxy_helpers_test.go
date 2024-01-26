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
	"reflect"
	"testing"
)

func TestNamespacesToStrings(t *testing.T) {
	testCases := []struct {
		description   string
		namespaces    []Namespace
		expectStrings []string
	}{
		{
			description:   "namespace 1",
			namespaces:    []Namespace{},
			expectStrings: []string{},
		},
		{
			description:   "namespace 2",
			namespaces:    []Namespace{"ns1", "ns2"},
			expectStrings: []string{"ns1", "ns2"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			if !reflect.DeepEqual(NamespacesToStrings(tc.namespaces), tc.expectStrings) {
				t.Errorf("expect converted strings %v is the same as %v", NamespacesToStrings(tc.namespaces), tc.expectStrings)
			}
		})
	}
}
