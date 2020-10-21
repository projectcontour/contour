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

package xds

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCounterNext(t *testing.T) {
	var c Counter
	// not a map this time as we want tests to execute
	// in sequence.
	tests := []struct {
		fn   func() uint64
		want uint64
	}{{
		fn:   c.Next,
		want: 1,
	}, {
		fn:   c.Next,
		want: 2,
	}, {
		fn:   c.Next,
		want: 3,
	}}

	for _, tc := range tests {
		got := tc.fn()
		assert.Equal(t, tc.want, got)
	}
}
