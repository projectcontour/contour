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

package secret

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTagFromImage(t *testing.T) {
	tests := map[string]struct {
		image string
		want  string
	}{
		"tag": {
			image: "ghcr.io/projectcontour/contour:v1.33.0",
			want:  "v1.33.0",
		},
		"no tag": {
			image: "ghcr.io/projectcontour/contour",
			want:  "",
		},
		"registry with port and tag": {
			image: "registry.example.com:5000/projectcontour/contour:v1.33.0",
			want:  "v1.33.0",
		},
		"registry with port and no tag": {
			image: "registry.example.com:5000/projectcontour/contour",
			want:  "",
		},
		"tag and digest": {
			image: "ghcr.io/projectcontour/contour:v1.33.0@sha256:0123456789abcdef",
			want:  "v1.33.0",
		},
		"digest only": {
			image: "ghcr.io/projectcontour/contour@sha256:0123456789abcdef",
			want:  "sha256:0123456789abcdef",
		},
		"registry with port and digest only": {
			image: "registry.example.com:5000/projectcontour/contour@sha256:0123456789abcdef",
			want:  "sha256:0123456789abcdef",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, tagFromImage(tc.image))
		})
	}
}
