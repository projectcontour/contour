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

package v3

import (
	"testing"

	envoy_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"

	"github.com/projectcontour/contour/internal/protobuf"
)

func TestSafeRegexMatch(t *testing.T) {
	tests := map[string]struct {
		regex string
		want  *envoy_matcher_v3.RegexMatcher
	}{
		"blank regex": {
			regex: "",
			want:  &envoy_matcher_v3.RegexMatcher{},
		},
		"simple": {
			regex: "chrome",
			want: &envoy_matcher_v3.RegexMatcher{
				Regex: "chrome",
			},
		},
		"regex meta": {
			regex: "[a-z]+$",
			want: &envoy_matcher_v3.RegexMatcher{
				Regex: "[a-z]+$", // meta characters are not escaped.
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := safeRegexMatch(tc.regex)
			protobuf.ExpectEqual(t, tc.want, got)
		})
	}
}
