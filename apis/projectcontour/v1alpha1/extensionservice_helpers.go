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

package v1alpha1

import (
	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
)

// GetConditionFor returns the a pointer to the condition for a given type,
// or nil if there are none currently present.
func (status *ExtensionServiceStatus) GetConditionFor(condType string) *contour_v1.DetailedCondition {
	for i, cond := range status.Conditions {
		if cond.Type == condType {
			return &status.Conditions[i]
		}
	}

	return nil
}
