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

package fixture

import (
	projectcontour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
)

// DetailedConditionBuilder is a builder object to make creating HTTPProxy fixtures more succinct.
type DetailedConditionBuilder projectcontour_v1.DetailedCondition

// NewValidCondition creates a new DetailedConditionBuilder.
func NewValidCondition() *DetailedConditionBuilder {
	b := &DetailedConditionBuilder{
		Condition: projectcontour_v1.Condition{
			Type: projectcontour_v1.ValidConditionType,
		},
	}

	return b
}

func (dcb *DetailedConditionBuilder) WithGeneration(gen int64) *DetailedConditionBuilder {
	dcb.ObservedGeneration = gen
	return dcb
}

func (dcb *DetailedConditionBuilder) Valid() projectcontour_v1.DetailedCondition {

	dc := (*projectcontour_v1.DetailedCondition)(dcb)
	dc.Status = projectcontour_v1.ConditionTrue
	dc.Reason = "Valid"
	dc.Message = "Valid HTTPProxy"

	return *dc
}

func (dcb *DetailedConditionBuilder) Orphaned() projectcontour_v1.DetailedCondition {

	dc := (*projectcontour_v1.DetailedCondition)(dcb)
	dc.AddError(projectcontour_v1.ConditionTypeOrphanedError, "Orphaned", "this HTTPProxy is not part of a delegation chain from a root HTTPProxy")

	return *dc
}

func (dcb *DetailedConditionBuilder) WithError(errorType string, reason, message string) projectcontour_v1.DetailedCondition {

	dc := (*projectcontour_v1.DetailedCondition)(dcb)
	dc.AddError(errorType, reason, message)

	return *dc

}

func (dcb *DetailedConditionBuilder) WithErrorf(errorType string, reason, formatmsg string, args ...interface{}) projectcontour_v1.DetailedCondition {

	dc := (*projectcontour_v1.DetailedCondition)(dcb)
	dc.AddErrorf(errorType, reason, formatmsg, args...)

	return *dc

}

func (dcb *DetailedConditionBuilder) WithWarning(errorType, reason, message string) projectcontour_v1.DetailedCondition {

	dc := (*projectcontour_v1.DetailedCondition)(dcb)
	dc.AddWarning(errorType, reason, message)

	return *dc

}

func (dcb *DetailedConditionBuilder) WithWarningf(warnType, reason, formatmsg string, args ...interface{}) projectcontour_v1.DetailedCondition {

	dc := (*projectcontour_v1.DetailedCondition)(dcb)
	dc.AddWarningf(warnType, reason, formatmsg, args...)

	return *dc

}
