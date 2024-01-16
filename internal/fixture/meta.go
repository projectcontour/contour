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
	"strconv"
	"strings"
	"sync/atomic"

	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var generation int64

// ObjectMeta cracks a Kubernetes object name string of the form
// "namespace/name" into a meta_v1.ObjectMeta struct. If the namespace
// portion is omitted, then the default namespace is filled in.
func ObjectMeta(nameStr string) meta_v1.ObjectMeta {
	// NOTE: We don't use k8s.NamespacedNameFrom here, because that
	// would generate an import cycle.

	// NOTE: Not all objects have a generation field.
	// In this helper function we do not know the type of the object where the return value gets used

	v := strings.SplitN(nameStr, "/", 2)
	switch len(v) {
	case 1:
		// No '/' separator.
		return *UpdateObjectVersion(&meta_v1.ObjectMeta{
			Name:        v[0],
			Namespace:   meta_v1.NamespaceDefault,
			Annotations: map[string]string{},
		})
	default:
		return *UpdateObjectVersion(&meta_v1.ObjectMeta{
			Name:        v[1],
			Namespace:   v[0],
			Annotations: map[string]string{},
		})
	}
}

// ObjectMetaWithAnnotations returns an ObjectMeta with the given annotations.
func ObjectMetaWithAnnotations(nameStr string, annotations map[string]string) meta_v1.ObjectMeta {
	meta := ObjectMeta(nameStr)
	meta.Annotations = annotations
	return meta
}

func UpdateObjectVersion(meta *meta_v1.ObjectMeta) *meta_v1.ObjectMeta {
	meta.Generation = nextGeneration()
	meta.ResourceVersion = strconv.FormatInt(meta.Generation, 10)
	return meta
}

// nextGeneration returns the next generation number.
func nextGeneration() int64 {
	return atomic.AddInt64(&generation, 1)
}
