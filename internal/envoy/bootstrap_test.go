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

package envoy

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidAdminAddress(t *testing.T) {
	tests := []struct {
		name    string
		address string
		want    error
	}{
		{name: "valid socket name", address: "/admin/admin.sock", want: nil},
		{name: "valid socket name", address: "admin.sock", want: nil},
		{name: "ip address invalid", address: "127.0.0.1", want: fmt.Errorf("invalid value %q, cannot be `localhost` or an ip address", "127.0.0.1")},
		{name: "localhost invalid", address: "localhost", want: fmt.Errorf("invalid value %q, cannot be `localhost` or an ip address", "localhost")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ValidAdminAddress(tc.address)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestValidConnectionLimit(t *testing.T) {
	tests := []struct {
		name      string
		connLimit int64
		want      error
	}{
		{name: "valid connection limit", connLimit: 10, want: nil},
		{name: "valid connection limit", connLimit: 0, want: nil},
		{name: "invalid connection limit", connLimit: -10, want: errors.New("invalid value -10, cannot be < 0")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ValidConnectionLimit(tc.connLimit)
			assert.Equal(t, tc.want, got)
		})
	}
}
