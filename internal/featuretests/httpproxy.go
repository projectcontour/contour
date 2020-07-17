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

package featuretests

// HTTPProxy helpers

import (
	projcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
)

func matchconditions(first projcontour.MatchCondition, rest ...projcontour.MatchCondition) []projcontour.MatchCondition {
	return append([]projcontour.MatchCondition{first}, rest...)
}

func prefixMatchCondition(prefix string) projcontour.MatchCondition {
	return projcontour.MatchCondition{
		Prefix: prefix,
	}
}

func headerContainsMatchCondition(name, value string) projcontour.MatchCondition {
	return projcontour.MatchCondition{
		Header: &projcontour.HeaderMatchCondition{
			Name:     name,
			Contains: value,
		},
	}
}

func headerNotContainsMatchCondition(name, value string) projcontour.MatchCondition {
	return projcontour.MatchCondition{
		Header: &projcontour.HeaderMatchCondition{
			Name:        name,
			NotContains: value,
		},
	}
}

func headerExactMatchCondition(name, value string) projcontour.MatchCondition {
	return projcontour.MatchCondition{
		Header: &projcontour.HeaderMatchCondition{
			Name:  name,
			Exact: value,
		},
	}
}

func headerNotExactMatchCondition(name, value string) projcontour.MatchCondition {
	return projcontour.MatchCondition{
		Header: &projcontour.HeaderMatchCondition{
			Name:     name,
			NotExact: value,
		},
	}
}

func headerPresentMatchCondition(name string) projcontour.MatchCondition {
	return projcontour.MatchCondition{
		Header: &projcontour.HeaderMatchCondition{
			Name:    name,
			Present: true,
		},
	}
}
