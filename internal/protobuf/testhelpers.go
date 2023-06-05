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

// Package protobuf provides helpers for working with golang/protobuf types.
package protobuf

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"
)

// ExpectEqual will test that want == got for protobufs, call t.Error if it does not,
// and return a bool to indicate the result. This mimics the behavior of the testify `assert`
// functions.
func ExpectEqual(t *testing.T, want, got any) bool {
	t.Helper()

	diff := cmp.Diff(want, got, protocmp.Transform())
	if diff != "" {
		t.Error(diff)
		return false
	}

	return true
}

// RequireEqual will test that want == got for protobufs, call t.fatal if it does not,
// This mimics the behavior of the testify `require` functions.
func RequireEqual(t *testing.T, want, got any) {
	t.Helper()

	diff := cmp.Diff(want, got, protocmp.Transform())
	if diff != "" {
		t.Fatal(diff)
	}
}
