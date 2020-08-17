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

package fixture

import (
	"github.com/projectcontour/contour/internal/k8s"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ObjectMeta cracks a Kubernetes object name string of the form
// "namespace/name" into a metav1.ObjectMeta struct. If the namespace
// portion is omitted, then the default namespace is filled in.
func ObjectMeta(nameStr string) metav1.ObjectMeta {
	name := k8s.NamespacedNameFrom(nameStr)
	return metav1.ObjectMeta{
		Name:        name.Name,
		Namespace:   name.Namespace,
		Annotations: map[string]string{},
	}
}
