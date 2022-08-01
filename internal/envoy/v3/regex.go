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
	matcher "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
)

// SafeRegexMatch returns a matcher.RegexMatcher for the supplied regex.
// SafeRegexMatch does not escape regex meta characters.
func SafeRegexMatch(regex string) *matcher.RegexMatcher {
	return &matcher.RegexMatcher{
		Regex: regex,
	}
}
