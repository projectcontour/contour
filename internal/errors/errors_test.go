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
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

func TestParseFieldErrors(t *testing.T) {
	testCases := []struct {
		name     string
		given    field.ErrorList
		expected string
	}{
		{
			name:     "nil",
			given:    nil,
			expected: "",
		},
		{
			name: "unsupported field value",
			given: field.ErrorList{
				{
					Type:   field.ErrorTypeNotSupported,
					Field:  "foo",
					Detail: "supported values: bar",
				},
			},
			expected: "Unsupported value for foo; supported values: bar",
		},
		{
			name: "multiple unsupported field values",
			given: field.ErrorList{
				{
					Type:   field.ErrorTypeNotSupported,
					Field:  "foo",
					Detail: "supported values: bar",
				},
				{
					Type:   field.ErrorTypeNotSupported,
					Field:  "bar",
					Detail: "supported values: foo",
				},
			},
			expected: "Unsupported value for foo; supported values: bar. Unsupported value for bar; supported values: foo",
		},
		{
			name: "invalid field value",
			given: field.ErrorList{
				{
					Type:   field.ErrorTypeInvalid,
					Field:  "foo",
					Detail: "supported values: bar",
				},
			},
			expected: "Invalid value for foo; supported values: bar",
		},
		{
			name: "invalid field value",
			given: field.ErrorList{
				{
					Type:   field.ErrorTypeInternal,
					Field:  "baz",
					Detail: "baz is not good",
				},
			},
			expected: "Internal error for baz; baz is not good",
		},
	}

	for _, tc := range testCases {
		if got := ParseFieldErrors(tc.given); got != tc.expected {
			assert.Equal(t, tc.expected, got, tc.name)
		}
	}
}
