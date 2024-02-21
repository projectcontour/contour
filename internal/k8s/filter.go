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
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

type namespaceFilter struct {
	next  cache.ResourceEventHandler
	index map[string]struct{}
}

// NewNamespaceFilter returns a cache.ResourceEventHandler that accepts
// only objects whose namespaces are included in the given slice of
// namespaces. Objects with matching namespaces are passed to the next
// handler.
func NewNamespaceFilter(
	namespaces []string,
	next cache.ResourceEventHandler,
) cache.ResourceEventHandler {
	e := &namespaceFilter{
		next:  next,
		index: make(map[string]struct{}),
	}

	for _, ns := range namespaces {
		e.index[ns] = struct{}{}
	}

	return e
}

func (e *namespaceFilter) allowed(obj any) bool {
	if obj, ok := obj.(meta_v1.Object); ok {
		_, ok := e.index[obj.GetNamespace()]
		return ok
	}

	return true
}

func (e *namespaceFilter) OnAdd(obj any, isInInitialList bool) {
	if e.allowed(obj) {
		e.next.OnAdd(obj, isInInitialList)
	}
}

func (e *namespaceFilter) OnUpdate(oldObj, newObj any) {
	if e.allowed(oldObj) {
		e.next.OnUpdate(oldObj, newObj)
	}
}

func (e *namespaceFilter) OnDelete(obj any) {
	if e.allowed(obj) {
		e.next.OnDelete(obj)
	}
}
