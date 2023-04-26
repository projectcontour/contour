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

package dag

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
)

// mergePathMatchConditions merges the given slice of prefix or exact MatchConditions into a single
// prefix/exact Condition. The leaf condition of the include tree decides whether the merged condition
// will be prefix or exact
// pathMatchConditionsValid guarantees that if a prefix is present, it will start with a
// / character, so we can simply concatenate.
func mergePathMatchConditions(conds []contour_api_v1.MatchCondition) MatchCondition {
	mergedPath := ""

	for _, cond := range conds {
		switch {
		case cond.Prefix != "":
			mergedPath += cond.Prefix
		case cond.Exact != "":
			mergedPath += cond.Exact
		}
	}

	re := regexp.MustCompile(`//+`)
	mergedPath = re.ReplaceAllString(mergedPath, `/`)

	// After the merge operation is done, if the string is still empty, then
	// we need to set the prefix to /.
	// Remember that this step is done AFTER all the includes have happened.
	// Setting this to / allows us to pass this prefix to Envoy, as there must
	// be at least one path, prefix, or regex set on each Envoy route.
	if mergedPath == "" || len(conds) == 0 {
		mergedPath = `/`
		return &PrefixMatchCondition{
			Prefix: mergedPath,
		}
	}

	// If mergedPath is not empty then choose match type using the last rule of the delegation chain
	lastCondition := conds[len(conds)-1]
	switch {
	case lastCondition.Prefix != "":
		return &PrefixMatchCondition{
			Prefix: mergedPath,
		}
	case lastCondition.Exact != "":
		return &ExactMatchCondition{
			Path: mergedPath,
		}
	default:
		return &PrefixMatchCondition{
			Prefix: mergedPath,
		}
	}
}

// pathMatchConditionsValid validates a slice of MatchConditions can be correctly merged.
// It encodes the business rules about what is allowed for MatchConditions.
func pathMatchConditionsValid(conds []contour_api_v1.MatchCondition) error {
	prefixCount := 0
	exactCount := 0

	for _, cond := range conds {
		if cond.Prefix != "" {
			prefixCount++
			if cond.Prefix[0] != '/' {
				return fmt.Errorf("prefix conditions must start with /, %s was supplied", cond.Prefix)
			}
		}
		if cond.Exact != "" {
			exactCount++
			if cond.Exact[0] != '/' {
				return fmt.Errorf("exact conditions must start with /, %s was supplied", cond.Exact)
			}
		}
		if prefixCount > 1 || exactCount > 1 || prefixCount+exactCount > 1 {
			return errors.New("more than one prefix or exact is not allowed in a condition block")
		}
	}

	return nil
}

// includeMatchConditionsValid validates the MatchConditions supplied in the includes
func includeMatchConditionsValid(conds []contour_api_v1.MatchCondition) error {
	for _, cond := range conds {
		if cond.Exact != "" {
			return fmt.Errorf("exact conditions are not allowed in includes block")
		}
	}

	return nil
}

func mergeHeaderMatchConditions(conds []contour_api_v1.MatchCondition) []HeaderMatchCondition {
	var headerConditions []contour_api_v1.HeaderMatchCondition
	for _, cond := range conds {
		if cond.Header != nil {
			headerConditions = append(headerConditions, *cond.Header)
		}
	}

	return headerMatchConditions(headerConditions)
}

func mergeQueryParamMatchConditions(conds []contour_api_v1.MatchCondition) []QueryParamMatchCondition {
	var queryParameterConditions []contour_api_v1.QueryParameterMatchCondition
	for _, cond := range conds {
		if cond.QueryParameter != nil {
			queryParameterConditions = append(queryParameterConditions, *cond.QueryParameter)
		}
	}

	return queryParameterMatchConditions(queryParameterConditions)
}

func headerMatchConditions(conditions []contour_api_v1.HeaderMatchCondition) []HeaderMatchCondition {
	var hc []HeaderMatchCondition

	for _, cond := range conditions {
		switch {
		case cond.Present:
			hc = append(hc, HeaderMatchCondition{
				Name:      cond.Name,
				MatchType: HeaderMatchTypePresent,
			})
		case cond.NotPresent:
			hc = append(hc, HeaderMatchCondition{
				Name:      cond.Name,
				MatchType: HeaderMatchTypePresent,
				Invert:    true,
			})
		case cond.Contains != "":
			hc = append(hc, HeaderMatchCondition{
				Name:      cond.Name,
				Value:     cond.Contains,
				MatchType: HeaderMatchTypeContains,
			})
		case cond.NotContains != "":
			hc = append(hc, HeaderMatchCondition{
				Name:      cond.Name,
				Value:     cond.NotContains,
				MatchType: HeaderMatchTypeContains,
				Invert:    true,
			})
		case cond.Exact != "":
			hc = append(hc, HeaderMatchCondition{
				Name:      cond.Name,
				Value:     cond.Exact,
				MatchType: HeaderMatchTypeExact,
			})
		case cond.NotExact != "":
			hc = append(hc, HeaderMatchCondition{
				Name:      cond.Name,
				Value:     cond.NotExact,
				MatchType: HeaderMatchTypeExact,
				Invert:    true,
			})
		}
	}
	return hc
}

func queryParameterMatchConditions(conditions []contour_api_v1.QueryParameterMatchCondition) []QueryParamMatchCondition {
	var qpc []QueryParamMatchCondition

	for _, cond := range conditions {
		switch {
		case cond.Exact != "":
			qpc = append(qpc, QueryParamMatchCondition{
				Name:       cond.Name,
				Value:      cond.Exact,
				MatchType:  QueryParamMatchTypeExact,
				IgnoreCase: cond.IgnoreCase,
			})
		case cond.Prefix != "":
			qpc = append(qpc, QueryParamMatchCondition{
				Name:       cond.Name,
				Value:      cond.Prefix,
				MatchType:  QueryParamMatchTypePrefix,
				IgnoreCase: cond.IgnoreCase,
			})
		case cond.Suffix != "":
			qpc = append(qpc, QueryParamMatchCondition{
				Name:       cond.Name,
				Value:      cond.Suffix,
				MatchType:  QueryParamMatchTypeSuffix,
				IgnoreCase: cond.IgnoreCase,
			})
		case cond.Regex != "":
			qpc = append(qpc, QueryParamMatchCondition{
				Name:      cond.Name,
				Value:     cond.Regex,
				MatchType: QueryParamMatchTypeRegex,
			})
		case cond.Contains != "":
			qpc = append(qpc, QueryParamMatchCondition{
				Name:       cond.Name,
				Value:      cond.Contains,
				MatchType:  QueryParamMatchTypeContains,
				IgnoreCase: cond.IgnoreCase,
			})
		case cond.Present:
			qpc = append(qpc, QueryParamMatchCondition{
				Name:      cond.Name,
				MatchType: QueryParamMatchTypePresent,
			})

		}
	}
	return qpc
}

// headerMatchConditionsValid validates that the header conditions within a
// slice of MatchConditions are valid. Specifically, it returns an error for
// any of the following scenarios:
//   - more than 1 'exact' condition for the same header
//   - a 'present' and a 'notpresent' condition for the same header
//   - an 'exact' and a 'notexact' condition for the same header, with the same values
//   - a 'contains' and a 'notcontains' condition for the same header, with the same values
//
// Note that there are additional, more complex scenarios that we could check for here. For
// example, "exact: foo" and "notcontains: <any substring of foo>" are contradictory.
func headerMatchConditionsValid(conditions []contour_api_v1.MatchCondition) error {
	seenMatchConditions := map[contour_api_v1.HeaderMatchCondition]bool{}
	headersWithExactMatch := map[string]bool{}

	for _, v := range conditions {
		if v.Header == nil {
			continue
		}

		headerName := strings.ToLower(v.Header.Name)
		switch {
		case v.Header.Present:
			if seenMatchConditions[contour_api_v1.HeaderMatchCondition{
				Name:       headerName,
				NotPresent: true,
			}] {
				return errors.New("cannot specify contradictory 'present' and 'notpresent' conditions for the same route and header")
			}
		case v.Header.NotPresent:
			if seenMatchConditions[contour_api_v1.HeaderMatchCondition{
				Name:    headerName,
				Present: true,
			}] {
				return errors.New("cannot specify contradictory 'present' and 'notpresent' conditions for the same route and header")
			}
		case v.Header.Exact != "":
			// Look for duplicate "exact match" headers on conditions
			if headersWithExactMatch[headerName] {
				return errors.New("cannot specify duplicate header 'exact match' conditions in the same route")
			}
			headersWithExactMatch[headerName] = true

			// look for a NotExact condition on the same header with the same value
			if seenMatchConditions[contour_api_v1.HeaderMatchCondition{
				Name:     headerName,
				NotExact: v.Header.Exact,
			}] {
				return errors.New("cannot specify contradictory 'exact' and 'notexact' conditions for the same route and header")
			}
		case v.Header.NotExact != "":
			// look for an Exact condition on the same header with the same value
			if seenMatchConditions[contour_api_v1.HeaderMatchCondition{
				Name:  headerName,
				Exact: v.Header.NotExact,
			}] {
				return errors.New("cannot specify contradictory 'exact' and 'notexact' conditions for the same route and header")
			}
		case v.Header.Contains != "":
			// look for a NotContains condition on the same header with the same value
			if seenMatchConditions[contour_api_v1.HeaderMatchCondition{
				Name:        headerName,
				NotContains: v.Header.Contains,
			}] {
				return errors.New("cannot specify contradictory 'contains' and 'notcontains' conditions for the same route and header")
			}
		case v.Header.NotContains != "":
			// look for a Contains condition on the same header with the same value
			if seenMatchConditions[contour_api_v1.HeaderMatchCondition{
				Name:     headerName,
				Contains: v.Header.NotContains,
			}] {
				return errors.New("cannot specify contradictory 'contains' and 'notcontains' conditions for the same route and header")
			}
		}

		key := *v.Header
		// use the lower-cased header name so comparisons are case-insensitive
		key.Name = headerName
		seenMatchConditions[key] = true
	}

	return nil
}

// queryParameterMatchConditionsValid validates that the query parameter conditions within a
// slice of MatchConditions are valid. Specifically, it returns an error for
// any of the following scenarios:
//   - no conditions are set
//   - more than one condition is set in the same match condition branch
//   - more than 1 'exact' condition for the same query parameter
//   - invalid regular expression is specified for the Regex condition
func queryParameterMatchConditionsValid(conditions []contour_api_v1.MatchCondition) error {
	queryParametersWithExactMatch := map[string]bool{}

	for _, v := range conditions {
		if v.QueryParameter == nil {
			continue
		}

		matches := []string{v.QueryParameter.Exact, v.QueryParameter.Prefix, v.QueryParameter.Suffix, v.QueryParameter.Regex, v.QueryParameter.Contains}
		if v.QueryParameter.Present {
			matches = append(matches, "true")
		}

		if strings.Join(matches, "") == "" {
			return errors.New("must specify at least one query parameter condition")
		}

		for i, match := range matches {
			if match == "" {
				continue
			}
			excluding := matches
			excluding[i] = ""
			if strings.Join(excluding, "") != "" {
				return errors.New("cannot specify more than one condition in the same match condition branch")
			}
		}

		queryParameterName := strings.ToLower(v.QueryParameter.Name)
		if v.QueryParameter.Exact != "" {
			if queryParametersWithExactMatch[queryParameterName] {
				return errors.New("cannot specify duplicate query parameter 'exact match' conditions in the same route")
			}
			queryParametersWithExactMatch[queryParameterName] = true
		}

		if v.QueryParameter.Regex != "" {
			if err := ValidateRegex(v.QueryParameter.Regex); err != nil {
				return errors.New("invalid regular expression specified for 'regex' condition")
			}
		}
	}

	return nil
}

// ValidateRegex returns an error if the supplied
// RE2 regex syntax is invalid.
func ValidateRegex(regex string) error {
	_, err := regexp.Compile(regex)
	return err
}
