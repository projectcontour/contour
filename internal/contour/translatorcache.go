// Copyright © 2017 Heptio
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

package contour

import (
	"k8s.io/api/core/v1"
	_cache "k8s.io/client-go/tools/cache"
)

// A translatorCache holds cached values for use by the translator.
// It is modeled as a cache.ResourceEventHandler so it can be composed
// with ResourceEventHandlers easily.
type translatorCache struct {
	services map[metadata]*v1.Service
}

func (t *translatorCache) OnAdd(obj interface{}) {
	switch obj := obj.(type) {
	case *v1.Service:
		if t.services == nil {
			t.services = make(map[metadata]*v1.Service)
		}
		t.services[metadata{name: obj.Name, namespace: obj.Namespace}] = obj
	default:
		// ignore
	}
}

func (t *translatorCache) OnUpdate(oldObj, newObj interface{}) {
	t.OnAdd(newObj)
}

func (t *translatorCache) OnDelete(obj interface{}) {
	switch obj := obj.(type) {
	case *v1.Service:
		delete(t.services, metadata{name: obj.Name, namespace: obj.Namespace})
	case _cache.DeletedFinalStateUnknown:
		t.OnDelete(obj.Obj) // recurse into ourselves with the tombstoned value
	default:
		// ignore
	}
}
