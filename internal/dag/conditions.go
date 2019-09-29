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

package dag

import (
	"path"

	projcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
)

func pathCondition(conds []projcontour.Condition) Condition {
	prefix := "/"
	for _, cond := range conds {
		prefix = path.Join(prefix, cond.Prefix)
	}
	return &PrefixCondition{
		Prefix: prefix,
	}
}

func headerConditions(conds []projcontour.Condition) []HeaderCondition {
	var hc []HeaderCondition
	for _, cond := range conds {
		switch {
		case cond.Header == nil:
			// skip it
		case cond.Header.Present:
			hc = append(hc, HeaderCondition{
				Name:      cond.Header.Name,
				MatchType: "present",
			})
		case cond.Header.Contains != "":
			hc = append(hc, HeaderCondition{
				Name:      cond.Header.Name,
				Value:     cond.Header.Contains,
				MatchType: "contains",
			})
		case cond.Header.NotContains != "":
			hc = append(hc, HeaderCondition{
				Name:      cond.Header.Name,
				Value:     cond.Header.NotContains,
				MatchType: "contains",
				Invert:    true,
			})
		case cond.Header.Exact != "":
			hc = append(hc, HeaderCondition{
				Name:      cond.Header.Name,
				Value:     cond.Header.Exact,
				MatchType: "exact",
			})
		case cond.Header.NotExact != "":
			hc = append(hc, HeaderCondition{
				Name:      cond.Header.Name,
				Value:     cond.Header.NotExact,
				MatchType: "exact",
				Invert:    true,
			})
		}
	}
	return hc
}
