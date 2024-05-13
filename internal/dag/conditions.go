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

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
)

// mergePathMatchConditions merges the given slice of prefix, regex or exact MatchConditions into a single
// prefix/exact/regex Condition.
// In the case no regex is present then the leaf condition of the include tree
// decides whether the merged condition will be prefix or exact.
// In case there is a regex condition present the entire condition becomes a regex condition.
// pathMatchConditionsValid guarantees that if a prefix is present, it will start with a
// / character, so we can simply concatenate.
func mergePathMatchConditions(conds []contour_v1.MatchCondition) MatchCondition {
	mergedPath := ""
	isRegex := false

	for _, cond := range conds {
		switch {
		case cond.Prefix != "":
			mergedPath += cond.Prefix
		case cond.Exact != "":
			mergedPath += cond.Exact
		case cond.Regex != "":
			mergedPath += cond.Regex
			isRegex = true
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
	case isRegex:
		return &RegexMatchCondition{
			Regex: mergedPath,
		}
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
func pathMatchConditionsValid(conds []contour_v1.MatchCondition) error {
	prefixCount := 0
	exactCount := 0
	regexCount := 0

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
		if cond.Regex != "" {
			regexCount++
			if cond.Regex[0] != '/' {
				return fmt.Errorf("regex conditions must start with /, %s was supplied", cond.Regex)
			}
			if err := ValidateRegex(cond.Regex); err != nil {
				return fmt.Errorf("supplied regex: %s invalid. error: %s", cond.Regex, err)
			}
		}
	}

	if prefixCount > 1 || exactCount > 1 || regexCount > 1 || prefixCount+exactCount+regexCount > 1 {
		return errors.New("more than one prefix, exact or regex is not allowed in a condition block")
	}

	return nil
}

// includeMatchConditionsValid validates the MatchConditions supplied in the includes
func includeMatchConditionsValid(conds []contour_v1.MatchCondition) error {
	for _, cond := range conds {
		if cond.Exact != "" {
			return fmt.Errorf("exact conditions are not allowed in includes block")
		}
		if cond.Regex != "" {
			return fmt.Errorf("regex conditions are not allowed in includes block")
		}
	}

	return nil
}

func mergeHeaderMatchConditions(conds []contour_v1.MatchCondition) []HeaderMatchCondition {
	var headerConditions []contour_v1.HeaderMatchCondition
	for _, cond := range conds {
		if cond.Header != nil {
			headerConditions = append(headerConditions, *cond.Header)
		}
	}

	return headerMatchConditions(headerConditions)
}

func mergeQueryParamMatchConditions(conds []contour_v1.MatchCondition) []QueryParamMatchCondition {
	var queryParameterConditions []contour_v1.QueryParameterMatchCondition
	for _, cond := range conds {
		if cond.QueryParameter != nil {
			queryParameterConditions = append(queryParameterConditions, *cond.QueryParameter)
		}
	}

	return queryParameterMatchConditions(queryParameterConditions)
}

func headerMatchConditions(conditions []contour_v1.HeaderMatchCondition) []HeaderMatchCondition {
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
				Name:       cond.Name,
				Value:      cond.Contains,
				MatchType:  HeaderMatchTypeContains,
				IgnoreCase: cond.IgnoreCase,
			})
		case cond.NotContains != "":
			hc = append(hc, HeaderMatchCondition{
				Name:                cond.Name,
				Value:               cond.NotContains,
				MatchType:           HeaderMatchTypeContains,
				Invert:              true,
				IgnoreCase:          cond.IgnoreCase,
				TreatMissingAsEmpty: cond.TreatMissingAsEmpty,
			})
		case cond.Exact != "":
			hc = append(hc, HeaderMatchCondition{
				Name:       cond.Name,
				Value:      cond.Exact,
				MatchType:  HeaderMatchTypeExact,
				IgnoreCase: cond.IgnoreCase,
			})
		case cond.NotExact != "":
			hc = append(hc, HeaderMatchCondition{
				Name:                cond.Name,
				Value:               cond.NotExact,
				MatchType:           HeaderMatchTypeExact,
				Invert:              true,
				IgnoreCase:          cond.IgnoreCase,
				TreatMissingAsEmpty: cond.TreatMissingAsEmpty,
			})
		case cond.Regex != "":
			hc = append(hc, HeaderMatchCondition{
				Name:      cond.Name,
				Value:     cond.Regex,
				MatchType: HeaderMatchTypeRegex,
			})
		}
	}
	return hc
}

func queryParameterMatchConditions(conditions []contour_v1.QueryParameterMatchCondition) []QueryParamMatchCondition {
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
//   - invalid regular expression is specified for the Regex condition
//
// Note that there are additional, more complex scenarios that we could check for here. For
// example, "exact: foo" and "notcontains: <any substring of foo>" are contradictory.
func headerMatchConditionsValid(conditions []contour_v1.MatchCondition) error {
	seenMatchConditions := map[contour_v1.HeaderMatchCondition]bool{}
	headersWithExactMatch := map[string]bool{}

	for _, v := range conditions {
		if v.Header == nil {
			continue
		}

		headerName := strings.ToLower(v.Header.Name)
		switch {
		case v.Header.Present:
			if seenMatchConditions[contour_v1.HeaderMatchCondition{
				Name:       headerName,
				NotPresent: true,
			}] {
				return errors.New("cannot specify contradictory 'present' and 'notpresent' conditions for the same route and header")
			}
		case v.Header.NotPresent:
			if seenMatchConditions[contour_v1.HeaderMatchCondition{
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
			if seenMatchConditions[contour_v1.HeaderMatchCondition{
				Name:     headerName,
				NotExact: v.Header.Exact,
			}] {
				return errors.New("cannot specify contradictory 'exact' and 'notexact' conditions for the same route and header")
			}
		case v.Header.NotExact != "":
			// look for an Exact condition on the same header with the same value
			if seenMatchConditions[contour_v1.HeaderMatchCondition{
				Name:  headerName,
				Exact: v.Header.NotExact,
			}] {
				return errors.New("cannot specify contradictory 'exact' and 'notexact' conditions for the same route and header")
			}
		case v.Header.Contains != "":
			// look for a NotContains condition on the same header with the same value
			if seenMatchConditions[contour_v1.HeaderMatchCondition{
				Name:        headerName,
				NotContains: v.Header.Contains,
			}] {
				return errors.New("cannot specify contradictory 'contains' and 'notcontains' conditions for the same route and header")
			}
		case v.Header.NotContains != "":
			// look for a Contains condition on the same header with the same value
			if seenMatchConditions[contour_v1.HeaderMatchCondition{
				Name:     headerName,
				Contains: v.Header.NotContains,
			}] {
				return errors.New("cannot specify contradictory 'contains' and 'notcontains' conditions for the same route and header")
			}
		case v.Header.Regex != "":
			if err := ValidateRegex(v.Header.Regex); err != nil {
				return errors.New("invalid regular expression specified for 'regex' condition")
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
func queryParameterMatchConditionsValid(conditions []contour_v1.MatchCondition) error {
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

// ExternalAuthAllowedHeadersValid validates that the allowed header conditions within a
// slice of HttpAuthorizationServerAllowedHeaders are valid. Specifically, it returns an error for
// any of the following scenarios:
//   - no conditions are set
//   - more than one condition is set in the same allowed header condition branch
//   - invalid regular expression is specified for the Regex condition
func ExternalAuthAllowedHeadersValid(allowedHeaders []contour_v1.HTTPAuthorizationServerAllowedHeaders) error {
	for _, allowedHeader := range allowedHeaders {
		sum := 0

		// To streamline user experience and mitigate potential issues, we do not support regex.
		// Additionally, it's essential to ensure that any regex patterns adhere to the configured runtime key, re2.max_program_size.error_level
		// by verifying that the program size is smaller than the specified value.
		// This necessitates thorough validation of user input.
		//
		// if allowedHeader.Regex != "" {
		// 	if err := ValidateRegex(allowedHeader.Regex); err != nil {
		// 		return errors.New("the RE2 regex syntax is invalid")
		// 	}
		// 	sum++
		// }

		if allowedHeader.Exact != "" {
			sum++
		}

		if allowedHeader.Prefix != "" {
			sum++
		}

		if allowedHeader.Suffix != "" {
			sum++
		}

		if allowedHeader.Contains != "" {
			sum++
		}

		if sum == 0 {
			return errors.New("one of prefix, suffix, exact or contains is required for each allowedHeader")
		}

		if sum > 1 {
			return errors.New("more than one prefix, suffix, exact or contains is not allowed in an allowedHeader")
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
