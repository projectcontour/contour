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

// mergePathMatchConditions merges the given slice of prefix MatchConditions into a single
// prefix Condition.
// pathMatchConditionsValid guarantees that if a prefix is present, it will start with a
// / character, so we can simply concatenate.
func mergePathMatchConditions(conds []contour_api_v1.MatchCondition) MatchCondition {
	prefix := ""
	for _, cond := range conds {
		prefix = prefix + cond.Prefix
	}

	re := regexp.MustCompile(`//+`)
	prefix = re.ReplaceAllString(prefix, `/`)

	// After the merge operation is done, if the string is still empty, then
	// we need to set the prefix to /.
	// Remember that this step is done AFTER all the includes have happened.
	// Setting this to / allows us to pass this prefix to Envoy, as there must
	// be at least one path, prefix, or regex set on each Envoy route.
	if prefix == "" {
		prefix = `/`
	}

	return &PrefixMatchCondition{
		Prefix: prefix,
	}
}

// pathMatchConditionsValid validates a slice of MatchConditions can be correctly merged.
// It encodes the business rules about what is allowed for prefix MatchConditions.
func pathMatchConditionsValid(conds []contour_api_v1.MatchCondition) error {
	prefixCount := 0

	for _, cond := range conds {
		if cond.Prefix != "" {
			prefixCount++
			if cond.Prefix[0] != '/' {
				return fmt.Errorf("prefix conditions must start with /, %s was supplied", cond.Prefix)
			}
		}
		if prefixCount > 1 {
			return errors.New("more than one prefix is not allowed in a condition block")
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

// headerMatchConditionsValid validates that the header conditions within a
// slice of MatchConditions are valid. Specifically, it returns an error for
// any of the following scenarios:
//	- more than 1 'exact' condition for the same header
//	- a 'present' and a 'notpresent' condition for the same header
//	- an 'exact' and a 'notexact' condition for the same header, with the same values
//	- a 'contains' and a 'notcontains' condition for the same header, with the same values
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

// ValidateRegex returns an error if the supplied
// RE2 regex syntax is invalid.
func ValidateRegex(regex string) error {
	_, err := regexp.Compile(regex)
	return err
}
