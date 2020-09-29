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
	v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
)

// DetailedConditionBuilder is a builder object to make creating HTTPProxy fixtures more succinct.
type DetailedConditionBuilder v1.DetailedCondition

// NewProxy creates a new ProxyBuilder with the specified object name.
func NewValidCondition() *DetailedConditionBuilder {
	b := &DetailedConditionBuilder{
		Condition: v1.Condition{
			Type: v1.ValidConditionType,
		},
	}

	return b
}

func (dcb *DetailedConditionBuilder) WithGeneration(gen int64) *DetailedConditionBuilder {
	dcb.ObservedGeneration = gen
	return dcb
}

func (dcb *DetailedConditionBuilder) Valid() v1.DetailedCondition {

	dc := (*v1.DetailedCondition)(dcb)
	dc.Reason = "valid"
	dc.Message = "valid HTTPProxy"

	return *dc
}

func (dcb *DetailedConditionBuilder) WithError(errorType, reason, message string) v1.DetailedCondition {

	dc := (*v1.DetailedCondition)(dcb)
	dc.AddError(errorType, reason, message)

	return *dc

}

func (dcb *DetailedConditionBuilder) WithWarning(errorType, reason, message string) v1.DetailedCondition {

	dc := (*v1.DetailedCondition)(dcb)
	dc.AddWarning(errorType, reason, message)

	return *dc

}
