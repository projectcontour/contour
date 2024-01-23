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

package util

import (
	"testing"
)

func Test_DiscoveryPoolCIDR(t *testing.T) {
	tests := []struct {
		name         string
		expectedName string
	}{
		{
			name:         "test",
			expectedName: "contour-resource-test",
		},
		{
			name:         "dev",
			expectedName: "contour-resource-dev",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := ContourResourceName(tt.name)
			if n != tt.expectedName {
				t.Errorf("function generated name %s for %s doesn't match %s.", n, tt.name, tt.expectedName)
			}
		})
	}
}
