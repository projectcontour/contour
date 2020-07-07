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

package envoy

import (
	"testing"

	matcher "github.com/envoyproxy/go-control-plane/envoy/type/matcher"
	"github.com/projectcontour/contour/internal/assert"
	"github.com/projectcontour/contour/internal/protobuf"
)

func TestSafeRegexMatch(t *testing.T) {
	tests := map[string]struct {
		regex string
		want  *matcher.RegexMatcher
	}{
		"blank regex": {
			regex: "",
			want: &matcher.RegexMatcher{
				EngineType: &matcher.RegexMatcher_GoogleRe2{
					GoogleRe2: &matcher.RegexMatcher_GoogleRE2{
						MaxProgramSize: protobuf.UInt32(maxRegexProgramSize),
					},
				},
			},
		},
		"simple": {
			regex: "chrome",
			want: &matcher.RegexMatcher{
				EngineType: &matcher.RegexMatcher_GoogleRe2{
					GoogleRe2: &matcher.RegexMatcher_GoogleRE2{
						MaxProgramSize: protobuf.UInt32(maxRegexProgramSize),
					},
				},
				Regex: "chrome",
			},
		},
		"regex meta": {
			regex: "[a-z]+$",
			want: &matcher.RegexMatcher{
				EngineType: &matcher.RegexMatcher_GoogleRe2{
					GoogleRe2: &matcher.RegexMatcher_GoogleRE2{
						MaxProgramSize: protobuf.UInt32(maxRegexProgramSize),
					},
				},
				Regex: "[a-z]+$", // meta characters are not escaped.
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := SafeRegexMatch(tc.regex)
			assert.Equal(t, tc.want, got)
		})
	}
}
