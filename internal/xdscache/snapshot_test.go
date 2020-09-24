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

package xdscache

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetNewSnapshotVersion(t *testing.T) {
	type testcase struct {
		startingVersion int64
		want            string
	}

	run := func(t *testing.T, name string, tc testcase) {
		t.Helper()

		t.Run(name, func(t *testing.T) {
			t.Helper()

			sh := SnapshotHandler{
				snapshotVersion: tc.startingVersion,
			}
			got := sh.newSnapshotVersion()
			assert.Equal(t, tc.want, got)
		})
	}

	run(t, "simple", testcase{
		startingVersion: 0,
		want:            "1",
	})

	run(t, "big version", testcase{
		startingVersion: math.MaxInt64 - 1,
		want:            "9223372036854775807",
	})

	run(t, "resets if max hit", testcase{
		startingVersion: math.MaxInt64,
		want:            "1",
	})
}
