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

package errors

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation/field"
)

// ParseFieldErrors parses Field and Detail from fieldErrs, returning
// a comma separated string of the field errors.
func ParseFieldErrors(fieldErrs field.ErrorList) string {
	if fieldErrs == nil {
		return ""
	}

	var errs []string
	for _, err := range fieldErrs {
		errs = append(errs, fmt.Sprintf("%v for %s; %s", err.Type, err.Field, err.Detail))
	}
	if len(errs) > 0 {
		return strings.Join(errs, ". ")
	}

	return ""
}
