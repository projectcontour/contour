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

// HTTPProxy helpers

import (
	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
)

func matchconditions(first contour_v1.MatchCondition, rest ...contour_v1.MatchCondition) []contour_v1.MatchCondition {
	return append([]contour_v1.MatchCondition{first}, rest...)
}

func prefixMatchCondition(prefix string) contour_v1.MatchCondition {
	return contour_v1.MatchCondition{
		Prefix: prefix,
	}
}

func headerContainsMatchCondition(name, value string, ignoreCase bool) contour_v1.MatchCondition {
	return contour_v1.MatchCondition{
		Header: &contour_v1.HeaderMatchCondition{
			Name:       name,
			Contains:   value,
			IgnoreCase: ignoreCase,
		},
	}
}

func headerNotContainsMatchCondition(name, value string, ignoreCase, treatMissingAsEmpty bool) contour_v1.MatchCondition {
	return contour_v1.MatchCondition{
		Header: &contour_v1.HeaderMatchCondition{
			Name:                name,
			NotContains:         value,
			IgnoreCase:          ignoreCase,
			TreatMissingAsEmpty: treatMissingAsEmpty,
		},
	}
}

func headerExactMatchCondition(name, value string, ignoreCase bool) contour_v1.MatchCondition {
	return contour_v1.MatchCondition{
		Header: &contour_v1.HeaderMatchCondition{
			Name:       name,
			Exact:      value,
			IgnoreCase: ignoreCase,
		},
	}
}

func headerNotExactMatchCondition(name, value string, ignoreCase, treatMissingAsEmpty bool) contour_v1.MatchCondition {
	return contour_v1.MatchCondition{
		Header: &contour_v1.HeaderMatchCondition{
			Name:                name,
			NotExact:            value,
			IgnoreCase:          ignoreCase,
			TreatMissingAsEmpty: treatMissingAsEmpty,
		},
	}
}

func headerPresentMatchCondition(name string) contour_v1.MatchCondition {
	return contour_v1.MatchCondition{
		Header: &contour_v1.HeaderMatchCondition{
			Name:    name,
			Present: true,
		},
	}
}

func headerRegexMatchCondition(name, value string) contour_v1.MatchCondition {
	return contour_v1.MatchCondition{
		Header: &contour_v1.HeaderMatchCondition{
			Name:  name,
			Regex: value,
		},
	}
}

func queryParameterExactMatchCondition(name, value string, ignoreCase bool) contour_v1.MatchCondition {
	return contour_v1.MatchCondition{
		QueryParameter: &contour_v1.QueryParameterMatchCondition{
			Name:       name,
			Exact:      value,
			IgnoreCase: ignoreCase,
		},
	}
}

func queryParameterPrefixMatchCondition(name, value string, ignoreCase bool) contour_v1.MatchCondition {
	return contour_v1.MatchCondition{
		QueryParameter: &contour_v1.QueryParameterMatchCondition{
			Name:       name,
			Prefix:     value,
			IgnoreCase: ignoreCase,
		},
	}
}

func queryParameterSuffixMatchCondition(name, value string, ignoreCase bool) contour_v1.MatchCondition {
	return contour_v1.MatchCondition{
		QueryParameter: &contour_v1.QueryParameterMatchCondition{
			Name:       name,
			Suffix:     value,
			IgnoreCase: ignoreCase,
		},
	}
}

func queryParameterRegexMatchCondition(name, value string) contour_v1.MatchCondition {
	return contour_v1.MatchCondition{
		QueryParameter: &contour_v1.QueryParameterMatchCondition{
			Name:  name,
			Regex: value,
		},
	}
}

func queryParameterContainsMatchCondition(name, value string, ignoreCase bool) contour_v1.MatchCondition {
	return contour_v1.MatchCondition{
		QueryParameter: &contour_v1.QueryParameterMatchCondition{
			Name:       name,
			Contains:   value,
			IgnoreCase: ignoreCase,
		},
	}
}

func queryParameterPresentMatchCondition(name string) contour_v1.MatchCondition {
	return contour_v1.MatchCondition{
		QueryParameter: &contour_v1.QueryParameterMatchCondition{
			Name:    name,
			Present: true,
		},
	}
}
