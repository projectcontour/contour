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

package selector

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestMatchesSelector(t *testing.T) {
	tests := map[string]struct {
		selector metav1.LabelSelector
		labels   map[string]string
		want     bool
	}{
		// Comparing against MatchLabels only should match the
		// provided labels.
		"matches labels": {
			selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "contour",
				},
			},
			labels: map[string]string{
				"app": "contour",
			},
			want: true,
		},
		// Comparing against MatchLabels only which don't match the labels.
		"labels do not match": {
			selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "contour",
				},
			},
			labels: map[string]string{
				"something": "else",
			},
			want: false,
		},
		"simple matches In expression": {
			selector: metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{{
					Key:      "app",
					Operator: "In",
					Values:   []string{"contour"},
				}},
			},
			labels: map[string]string{
				"app": "contour",
			},
			want: true,
		},
		"simple matches NotIn expression": {
			selector: metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{{
					Key:      "app",
					Operator: "NotIn",
					Values:   []string{"contour"},
				}},
			},
			labels: map[string]string{
				"app": "doesntmatch",
			},
			want: true,
		},
		"simple matches Exists expression": {
			selector: metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{{
					Key:      "app",
					Operator: "Exists",
				}},
			},
			labels: map[string]string{
				"app": "contour",
			},
			want: true,
		},
		"simple matches NotExists expression": {
			selector: metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{{
					Key:      "app",
					Operator: "DoesNotExist",
				}},
			},
			labels: map[string]string{
				"somekey": "contour",
			},
			want: true,
		},
		"combo expression": {
			selector: metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{{
					Key:      "donotbethere",
					Operator: "DoesNotExist",
				}, {
					Key:      "alsodonotbethere",
					Operator: "DoesNotExist",
				}, {
					Key:      "app",
					Operator: "Exists",
				}, {
					Key:      "somekey",
					Operator: "Exists",
				}, {
					Key:      "somekey",
					Operator: "NotIn",
					Values:   []string{"contour"},
				}, {
					Key:      "app",
					Operator: "In",
					Values:   []string{"contour", "someothervalue"},
				}},
			},
			labels: map[string]string{
				"somekey": "somevalue",
				"app":     "contour",
			},
			want: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {

			got := MatchesLabelSelector(tc.selector, tc.labels)
			assert.Equal(t, tc.want, got)
		})
	}
}
