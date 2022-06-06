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

package retryableerror

import (
	"errors"
	"testing"
	"time"

	utilerrors "k8s.io/apimachinery/pkg/util/errors"
)

func TestRetryableError(t *testing.T) {
	tests := map[string]struct {
		errors          []error
		expectRetryable bool
		expectAggregate bool
		expectAfter     time.Duration
	}{
		"empty list": {},
		"nil error": {
			errors: []error{nil},
		},
		"non-retryable errors": {
			errors:          []error{errors.New("foo"), errors.New("bar")},
			expectAggregate: true,
		},
		"mix of retryable and non-retryable errors": {
			errors: []error{
				errors.New("foo"),
				errors.New("bar"),
				New(errors.New("baz"), time.Second*15),
				New(errors.New("quux"), time.Minute),
			},
			expectAggregate: true,
		},
		"only retryable errors": {
			errors: []error{
				New(errors.New("baz"), time.Second*15),
				New(errors.New("quux"), time.Minute),
				nil,
			},
			expectRetryable: true,
			expectAfter:     time.Second * 15,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			err := NewMaybeRetryableAggregate(test.errors)
			if retryable, gotRetryable := err.(Error); gotRetryable != test.expectRetryable {
				t.Errorf("expected retryable %T, got %T: %v", test.expectRetryable, gotRetryable, err)
			} else if gotRetryable && retryable.After() != test.expectAfter {
				t.Errorf("expected after %v, got %v: %v", test.expectAfter, retryable.After(), err)
			}
			if _, gotAggregate := err.(utilerrors.Aggregate); gotAggregate != test.expectAggregate {
				t.Errorf("expected aggregate %T, got %T: %v", test.expectAggregate, gotAggregate, err)
			}
		})
	}
}
