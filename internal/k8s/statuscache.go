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

	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// StatusUpdateCacher takes status updates and applies them to a cache, to be used for testing.
type StatusUpdateCacher struct {
	objectCache map[string]interface{}
}

// IsCacheable returns whether this type of object can be stored in
// the status cache.
func (suc *StatusUpdateCacher) IsCacheable(obj interface{}) bool {
	switch obj.(type) {
	case *contour_api_v1.HTTPProxy:
		return true
	default:
		return false
	}
}

// OnDelete removes an object from the status cache.
func (suc *StatusUpdateCacher) OnDelete(obj interface{}) {
	if suc.objectCache != nil {
		switch o := obj.(type) {
		case *contour_api_v1.HTTPProxy:
			delete(suc.objectCache, suc.objKey(o.Name, o.Namespace, contour_api_v1.HTTPProxyGVR))
		default:
			panic(fmt.Sprintf("status caching not supported for object type %T", obj))
		}

	}
}

// OnAdd adds an object to the status cache.
func (suc *StatusUpdateCacher) OnAdd(obj interface{}) {
	if suc.objectCache == nil {
		suc.objectCache = make(map[string]interface{})
	}

	switch o := obj.(type) {
	case *contour_api_v1.HTTPProxy:
		suc.objectCache[suc.objKey(o.Name, o.Namespace, contour_api_v1.HTTPProxyGVR)] = obj
	default:
		panic(fmt.Sprintf("status caching not supported for object type %T", obj))
	}

}

// Get allows retrieval of objects from the cache.
func (suc *StatusUpdateCacher) Get(name, namespace string, gvr schema.GroupVersionResource) interface{} {

	if suc.objectCache == nil {
		suc.objectCache = make(map[string]interface{})
	}

	obj, ok := suc.objectCache[suc.objKey(name, namespace, gvr)]
	if ok {
		return obj
	}
	return nil

}

func (suc *StatusUpdateCacher) Add(name, namespace string, gvr schema.GroupVersionResource, obj interface{}) bool {

	if suc.objectCache == nil {
		suc.objectCache = make(map[string]interface{})
	}

	prefix := suc.objKey(name, namespace, gvr)
	_, ok := suc.objectCache[prefix]
	if ok {
		return false
	}

	suc.objectCache[prefix] = obj

	return true

}

func (suc *StatusUpdateCacher) GetStatus(obj interface{}) (*contour_api_v1.HTTPProxyStatus, error) {
	switch o := obj.(type) {
	case *contour_api_v1.HTTPProxy:
		objectKey := suc.objKey(o.Name, o.Namespace, contour_api_v1.HTTPProxyGVR)
		cachedObj, ok := suc.objectCache[objectKey]
		if ok {
			switch c := cachedObj.(type) {
			case *contour_api_v1.HTTPProxy:
				return &c.Status, nil
			}
		}
		return nil, fmt.Errorf("no status for key '%s'", objectKey)
	default:
		panic(fmt.Sprintf("status caching not supported for object type %T", obj))
	}
}

func (suc *StatusUpdateCacher) objKey(name, namespace string, gvr schema.GroupVersionResource) string {

	return fmt.Sprintf("%s/%s/%s/%s", gvr.Group, gvr.Resource, namespace, name)
}

func (suc *StatusUpdateCacher) Send(su StatusUpdate) {
	if suc.objectCache == nil {
		suc.objectCache = make(map[string]interface{})
	}
	objKey := suc.objKey(su.NamespacedName.Name, su.NamespacedName.Namespace, su.Resource)
	obj, ok := suc.objectCache[objKey]
	if ok {
		suc.objectCache[objKey] = su.Mutator.Mutate(obj)
	}

}
