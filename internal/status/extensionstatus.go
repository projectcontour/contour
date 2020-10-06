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

package status

import (
	"fmt"
	"time"

	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	contour_api_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/k8s"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// ConditionCache holds all the DetailedConditions to add to the object
// keyed by the Type (since that's what the API server will end up doing).
type ConditionCache struct {
	Conditions map[ConditionType]*contour_api_v1.DetailedCondition
}

// ConditionFor returns the cached DetailedCondition of the given
// type. If no such condition exists, a new one is created.
func (c *ConditionCache) ConditionFor(condType ConditionType) *contour_api_v1.DetailedCondition {
	if c.Conditions == nil {
		c.Conditions = make(map[ConditionType]*contour_api_v1.DetailedCondition)
	}

	if cond, ok := c.Conditions[condType]; ok {
		return cond
	}

	cond := &contour_api_v1.DetailedCondition{
		Condition: contour_api_v1.Condition{
			Type:   string(condType),
			Status: contour_api_v1.ConditionUnknown,
		},
	}

	c.Conditions[condType] = cond
	return cond
}

// ExtensionCacheEntry holds status updates for a particular ExtensionService
type ExtensionCacheEntry struct {
	ConditionCache

	Name           types.NamespacedName
	Generation     int64
	TransitionTime v1.Time
}

var _ CacheEntry = &ExtensionCacheEntry{}

func (e *ExtensionCacheEntry) AsStatusUpdate() k8s.StatusUpdate {
	m := k8s.StatusMutatorFunc(func(obj interface{}) interface{} {
		o, ok := obj.(*contour_api_v1alpha1.ExtensionService)
		if !ok {
			panic(fmt.Sprintf("unsupported %T object %q in status mutator", obj, e.Name))
		}

		ext := o.DeepCopy()

		for condType, cond := range e.Conditions {
			cond.ObservedGeneration = e.Generation
			cond.LastTransitionTime = e.TransitionTime

			currCond := ext.Status.GetConditionFor(string(condType))
			if currCond == nil {
				ext.Status.Conditions = append(ext.Status.Conditions, *cond)
				continue
			}

			cond.DeepCopyInto(currCond)
		}

		return ext
	})

	return k8s.StatusUpdate{
		NamespacedName: e.Name,
		Resource:       contour_api_v1alpha1.ExtensionServiceGVR,
		Mutator:        m,
	}
}

// ExtensionAccessor returns a pointer to a shared status cache entry
// for the given ExtensionStatus object. If no such entry exists, a
// new entry is added. When the caller finishes with the cache entry,
// it must call the returned function to release the entry back to the
// cache.
func ExtensionAccessor(c *Cache, ext *contour_api_v1alpha1.ExtensionService) (*ExtensionCacheEntry, func()) {
	entry := c.Get(ext)
	if entry == nil {
		entry = &ExtensionCacheEntry{
			Name:           k8s.NamespacedNameOf(ext),
			Generation:     ext.GetGeneration(),
			TransitionTime: v1.NewTime(time.Now()),
		}

		// Populate the cache with the new entry
		c.Put(ext, entry)
	}

	entry = c.Get(ext)
	return entry.(*ExtensionCacheEntry), func() {
		c.Put(ext, entry)
	}
}
