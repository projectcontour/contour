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

package parse

import (
	"testing"
)

func TestImage(t *testing.T) {
	testCases := []struct {
		description string
		image       string
		expected    bool
	}{
		{
			description: "image name",
			image:       "repo/org/project",
			expected:    true,
		},
		{
			description: "image with tag",
			image:       "repo/org/project:tag",
			expected:    true,
		},
		{
			description: "image name, tag and sha256 digest",
			image:       "repo/org/project:tag@sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae",
			expected:    true,
		},
		{
			description: "image name, tag and incorrect digest reference",
			image:       "repo/org/project:tag@2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae",
			expected:    false,
		},
		{
			description: "image name, tag and invalid sha256 digest length",
			image:       "repo/org/project@sha256:9ce7a12",
			expected:    false,
		},
		{
			description: "image name, tag and sha512 digest",
			image: "repo/org/project:tag@sha512:54cdb8ee95fa7264b7eca84766ecccde7fd9e3e00c8b8bf518e9fcff52ad0" +
				"61ad28cae49ec3a09144ee8f342666462743718b5a73215bee373ed6f3120d30351",
			expected: true,
		},
		{
			description: "image name, tag and a sha512 digest with a sha256 length",
			image:       "repo/org/project:tag@sha512:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae",
			expected:    false,
		},
	}

	for _, tc := range testCases {
		err := Image(tc.image)
		switch {
		case err != nil && tc.expected:
			t.Fatalf("%q: %v", tc.description, err)
		case err == nil && !tc.expected:
			t.Fatalf("%q: expected an error but received nil", tc.description)
		}
	}
}
