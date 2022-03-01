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

package objects

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// NewUnprivilegedPodSecurity makes a a non-root PodSecurityContext object
// using 65534 as the user and group ID.
func NewUnprivilegedPodSecurity() *corev1.PodSecurityContext {
	user := int64(65534)
	group := int64(65534)
	nonRoot := true
	return &corev1.PodSecurityContext{
		RunAsUser:    &user,
		RunAsGroup:   &group,
		RunAsNonRoot: &nonRoot,
	}
}

// TagFromImage returns the tag from the provided image or an
// empty string if the image does not contain a tag.
func TagFromImage(image string) string {
	if strings.Contains(image, ":") {
		parsed := strings.Split(image, ":")
		return parsed[1]
	}
	return ""
}
