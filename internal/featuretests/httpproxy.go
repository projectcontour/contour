// Copyright © 2019 VMware
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

func conditions(first projcontour.Condition, rest ...projcontour.Condition) []projcontour.Condition {
	return append([]projcontour.Condition{first}, rest...)
}

func prefixCondition(prefix string) projcontour.Condition {
	return projcontour.Condition{
		Prefix: prefix,
	}
}

func headerContainsCondition(name, value string) projcontour.Condition {
	return projcontour.Condition{
		Header: &projcontour.HeaderCondition{
			Name:     name,
			Contains: value,
		},
	}
}

func headerNotContainsCondition(name, value string) projcontour.Condition {
	return projcontour.Condition{
		Header: &projcontour.HeaderCondition{
			Name:        name,
			NotContains: value,
		},
	}
}

func headerExactCondition(name, value string) projcontour.Condition {
	return projcontour.Condition{
		Header: &projcontour.HeaderCondition{
			Name:  name,
			Exact: value,
		},
	}
}

func headerNotExactCondition(name, value string) projcontour.Condition {
	return projcontour.Condition{
		Header: &projcontour.HeaderCondition{
			Name:     name,
			NotExact: value,
		},
	}
}

func headerPresentCondition(name string) projcontour.Condition {
	return projcontour.Condition{
		Header: &projcontour.HeaderCondition{
			Name:    name,
			Present: true,
		},
	}
}
