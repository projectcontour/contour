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

package slice

import (
	"reflect"
	"testing"
)

func TestRemoveString(t *testing.T) {
	testCases := map[string]struct {
		in     []string
		remove string
		out    []string
	}{
		"one string, remove one": {
			in:     []string{"one"},
			remove: "one",
			out:    nil,
		},
		"two strings, remove first string": {
			in:     []string{"one", "two"},
			remove: "one",
			out:    []string{"two"},
		},
		"two strings, remove second string": {
			in:     []string{"one", "two"},
			remove: "two",
			out:    []string{"one"},
		},
		"two strings, remove one that doesn't exist": {
			in:     []string{"one", "two"},
			remove: "three",
			out:    []string{"one", "two"},
		},
		"three strings, remove the second string": {
			in:     []string{"one", "two", "three"},
			remove: "two",
			out:    []string{"one", "three"},
		},
		"three strings, remove empty string": {
			in:     []string{"one", "two", "three"},
			remove: "",
			out:    []string{"one", "two", "three"},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			out := RemoveString(tc.in, tc.remove)
			if !reflect.DeepEqual(out, tc.out) {
				t.Errorf("expected slice to be %v, got %v", tc.out, out)
			}
		})
	}
}

func TestContainsString(t *testing.T) {
	testCases := map[string]struct {
		in       []string
		contains string
		expect   bool
	}{
		"one string": {
			in:       []string{"one"},
			contains: "one",
			expect:   true,
		},
		"nil slice": {
			in:       nil,
			contains: "one",
			expect:   false,
		},
		"empty slice": {
			in:       []string{},
			contains: "one",
			expect:   false,
		},
		"three strings, find first string": {
			in:       []string{"one", "two", "three"},
			contains: "one",
			expect:   true,
		},
		"three strings, find second string": {
			in:       []string{"one", "two", "three"},
			contains: "two",
			expect:   true,
		},
		"three strings, find third string": {
			in:       []string{"one", "two", "three"},
			contains: "three",
			expect:   true,
		},
		"three strings, doesn't contain string": {
			in:       []string{"one", "two", "three"},
			contains: "four",
			expect:   false,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			out := ContainsString(tc.in, tc.contains)
			if out != tc.expect {
				t.Errorf("expected slice to be %v, got %v", tc.expect, out)
			}
		})
	}
}
