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

// Package status holds pieces for handling status updates propagated from
// the DAG back to Kubernetes
package status

import (
	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	contour_api_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/k8s"
	"k8s.io/apimachinery/pkg/types"
)

// NewCache creates a new Cache for holding status updates.
func NewCache() Cache {
	return Cache{
		updates: make(map[string]map[types.NamespacedName]k8s.StatusUpdate),
	}
}

// Cache holds status updates from the DAG back towards Kubernetes.
// It holds a per-Kind cache, and is intended to be accessed with a
// KindAccessor.
type Cache struct {
	updates map[string]map[types.NamespacedName]k8s.StatusUpdate
}

func (c *Cache) accessorFor(obj k8s.Object) (string, *Accessor) {
	kind := k8s.KindOf(obj)

	if _, ok := c.updates[kind]; !ok {
		c.updates[kind] = make(map[types.NamespacedName]k8s.StatusUpdate)
	}

	return kind, NewAccessor(obj)
}

// ProxyAccessor returns a ProxyUpdate that allows a client to build up a list of
// errors and warnings to go onto the proxy as conditions, and a function to commit the change
// back to the cache when everything is done.
// The commit function pattern is used so that the ProxyUpdate does not need to know anything
// the cache internals.
func (c *Cache) ProxyAccessor(proxy *contour_api_v1.HTTPProxy) (*Accessor, func()) {
	kind, a := c.accessorFor(proxy)

	return a, func() {
		if len(a.Conditions) > 0 {
			c.updates[kind][a.Name] = k8s.StatusUpdate{
				NamespacedName: a.Name,
				Resource:       contour_api_v1.HTTPProxyGVR,
				Mutator:        ProxyStatusMutator(a),
			}
		}
	}
}

func (c *Cache) ExtensionAccessor(ext *contour_api_v1alpha1.ExtensionService) (*Accessor, func()) {
	kind, a := c.accessorFor(ext)

	return a, func() {
		if len(a.Conditions) > 0 {
			c.updates[kind][a.Name] = k8s.StatusUpdate{
				NamespacedName: a.Name,
				Resource:       contour_api_v1alpha1.ExtensionServiceGVR,
				Mutator:        ExtensionStatusMutator(a),
			}
		}
	}
}

// GetStatusUpdates returns a slice of StatusUpdates, ready to be sent off
// to the StatusUpdater by the event handler.
func (c *Cache) GetStatusUpdates() []k8s.StatusUpdate {
	var flattened []k8s.StatusUpdate

	for _, byKind := range c.updates {
		for _, u := range byKind {
			flattened = append(flattened, u)
		}
	}

	return flattened
}
