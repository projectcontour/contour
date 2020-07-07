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
	matcher "github.com/envoyproxy/go-control-plane/envoy/type/matcher"
	"github.com/projectcontour/contour/internal/protobuf"
)

// maxRegexProgramSize is the default value for the Envoy regex max
// program size tunable. There's no way to really know what a good
// value for this is, except that the RE2 maintainer thinks that 100
// is low. As a rule of thumb, each '.*' expression costs about 15
// units of program size. AFAIK, there's no obvious correlation
// between regex size and execution time.
//
// See also https://github.com/envoyproxy/envoy/pull/9171#discussion_r351974033
// and https://github.com/projectcontour/contour/issues/2240
const maxRegexProgramSize = 1 << 20

// SafeRegexMatch retruns a matcher.RegexMatcher for the supplied regex.
// SafeRegexMatch does not escape regex meta characters.
func SafeRegexMatch(regex string) *matcher.RegexMatcher {
	return &matcher.RegexMatcher{
		EngineType: &matcher.RegexMatcher_GoogleRe2{
			GoogleRe2: &matcher.RegexMatcher_GoogleRE2{
				MaxProgramSize: protobuf.UInt32(maxRegexProgramSize),
			},
		},
		Regex: regex,
	}
}
