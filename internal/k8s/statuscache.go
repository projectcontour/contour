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

package k8s

import (
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
)

// StatusUpdateCacher takes status updates and applies them to a cache, to be used for testing.
type StatusUpdateCacher struct {
	objectCache map[string]client.Object
}

// IsCacheable returns whether this type of object can be stored in
// the status cache.
func (suc *StatusUpdateCacher) IsCacheable(obj any) bool {
	switch obj.(type) {
	case *contour_v1.HTTPProxy:
		return true
	default:
		return false
	}
}

// OnDelete removes an object from the status cache.
func (suc *StatusUpdateCacher) OnDelete(obj any) {
	if suc.objectCache != nil {
		switch o := obj.(type) {
		case *contour_v1.HTTPProxy:
			delete(suc.objectCache, suc.objKey(fmt.Sprintf("%T", o), o.Name, o.Namespace))
		default:
			panic(fmt.Sprintf("status caching not supported for object type %T", obj))
		}
	}
}

// OnAdd adds an object to the status cache.
func (suc *StatusUpdateCacher) OnAdd(obj any) {
	if suc.objectCache == nil {
		suc.objectCache = make(map[string]client.Object)
	}

	switch o := obj.(type) {
	case *contour_v1.HTTPProxy:
		suc.objectCache[suc.objKey(fmt.Sprintf("%T", o), o.Name, o.Namespace)] = o
	default:
		panic(fmt.Sprintf("status caching not supported for object type %T", obj))
	}
}

// Get allows retrieval of objects from the cache.
func (suc *StatusUpdateCacher) Get(objType, name, namespace string) any {
	if suc.objectCache == nil {
		suc.objectCache = make(map[string]client.Object)
	}

	obj, ok := suc.objectCache[suc.objKey(objType, name, namespace)]
	if ok {
		return obj
	}
	return nil
}

func (suc *StatusUpdateCacher) Add(name, namespace string, obj client.Object) bool {
	if suc.objectCache == nil {
		suc.objectCache = make(map[string]client.Object)
	}

	prefix := suc.objKey(fmt.Sprintf("%T", obj), name, namespace)
	_, ok := suc.objectCache[prefix]
	if ok {
		return false
	}

	suc.objectCache[prefix] = obj

	return true
}

func (suc *StatusUpdateCacher) GetStatus(obj any) (*contour_v1.HTTPProxyStatus, error) {
	switch o := obj.(type) {
	case *contour_v1.HTTPProxy:
		objectKey := suc.objKey(fmt.Sprintf("%T", o), o.Name, o.Namespace)
		cachedObj, ok := suc.objectCache[objectKey]
		if ok {
			if c, ok := cachedObj.(*contour_v1.HTTPProxy); ok {
				return &c.Status, nil
			}
		}
		return nil, fmt.Errorf("no status for key '%s'", objectKey)
	default:
		panic(fmt.Sprintf("status caching not supported for object type %T", obj))
	}
}

func (suc *StatusUpdateCacher) objKey(objType, name, namespace string) string {
	return fmt.Sprintf("%s/%s/%s", objType, namespace, name)
}

func (suc *StatusUpdateCacher) Send(su StatusUpdate) {
	if suc.objectCache == nil {
		suc.objectCache = make(map[string]client.Object)
	}

	objKey := suc.objKey(fmt.Sprintf("%T", su.Resource), su.NamespacedName.Name, su.NamespacedName.Namespace)
	obj, ok := suc.objectCache[objKey]
	if ok {
		suc.objectCache[objKey] = su.Mutator.Mutate(obj)
	}
}
