// Copyright © 2018 Heptio
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

package grpc

import (
	"reflect"
	"testing"
)

func TestToFilter(t *testing.T) {
	tests := map[string]struct {
		names []string
		input []string
		want  []string
	}{
		"empty names": {
			names: nil,
			input: []string{"a", "b", "c"},
			want:  []string{"a", "b", "c"},
		},
		"empty input": {
			names: []string{"a", "b", "c"},
			input: nil,
			want:  []string{},
		},
		"fully matching filter": {
			names: []string{"a", "b", "c"},
			input: []string{"a", "b", "c"},
			want:  []string{"a", "b", "c"},
		},
		"non matching filter": {
			names: []string{"d", "e"},
			input: []string{"a", "b", "c"},
			want:  []string{},
		},
		"partially matching filter": {
			names: []string{"c", "e"},
			input: []string{"a", "b", "c", "d"},
			want:  []string{"c"},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := []string{}
			filter := toFilter(tc.names)
			for _, i := range tc.input {
				if filter(i) {
					got = append(got, i)
				}
			}
			if !reflect.DeepEqual(tc.want, got) {
				t.Fatalf("expected: %v, got: %v", tc.want, got)
			}
		})
	}
}

func TestCounterNext(t *testing.T) {
	var c counter
	// not a map this time as we want tests to execute
	// in sequence.
	tests := []struct {
		fn   func() uint64
		want uint64
	}{{
		fn:   c.next,
		want: 1,
	}, {
		fn:   c.next,
		want: 2,
	}, {
		fn:   c.next,
		want: 3,
	}}

	for _, tc := range tests {
		got := tc.fn()
		if tc.want != got {
			t.Fatalf("expected %d, got %d", tc.want, got)
		}
	}
}
