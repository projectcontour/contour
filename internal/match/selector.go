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

package match

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// LabelSelector returns true if the selector set matches the values in the labels.
func LabelSelector(selector metav1.LabelSelector, matchLabels map[string]string) bool {

	matchesLabels := false
	matchesExpression := false

	// If MatchLabels aren't defined then skip the validation check.
	if len(selector.MatchLabels) == 0 {
		matchesLabels = true
	} else {
		matches := 0
		for k, v := range selector.MatchLabels {
			if matchLabels[k] == v {
				matches++
				break
			}
		}
		if matches == len(selector.MatchLabels) {
			matchesLabels = true
		}
	}

	// If MatchExpressions aren't defined then skip the validation check.
	if len(selector.MatchExpressions) == 0 {
		matchesExpression = true
	} else {
		matches := 0
		for _, expression := range selector.MatchExpressions {
			switch expression.Operator {
			case metav1.LabelSelectorOpIn:
				// Must find key with match in the list of values.
				for _, val := range expression.Values {
					if value, ok := matchLabels[expression.Key]; ok {
						if val == value {
							matches++
							break
						}
					}
				}
			case metav1.LabelSelectorOpNotIn:
				// Must not find key in the list of values.
				found := false
				for _, val := range expression.Values {
					if value, ok := matchLabels[expression.Key]; ok {
						if val == value {
							found = true
							break
						}
					}
				}
				if !found {
					matches++
				}
			case metav1.LabelSelectorOpExists:
				// Must find key in the list of values.
				if _, ok := matchLabels[expression.Key]; ok {
					matches++
				}
			case metav1.LabelSelectorOpDoesNotExist:
				// Must not find key in the list of values.
				if _, ok := matchLabels[expression.Key]; !ok {
					matches++
				}
			}
		}
		if matches == len(selector.MatchExpressions) {
			matchesExpression = true
		}
	}

	return matchesLabels && matchesExpression
}
