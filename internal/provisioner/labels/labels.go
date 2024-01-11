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

package labels

type LabeledObject interface {
	GetLabels() map[string]string
}

// AnyExist returns true if obj contains at least one of the provided labels.
func AnyExist(obj LabeledObject, labels map[string]string) bool {
	objLabels := obj.GetLabels()

	if len(objLabels) == 0 {
		return false
	}

	for k, v := range labels {
		if val, ok := objLabels[k]; ok && val == v {
			return true
		}
	}

	return false
}
