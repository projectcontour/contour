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
	"github.com/projectcontour/contour/internal/k8s"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	gatewayapi_v1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

// ConditionType is used to ensure we only use a limited set of possible values
// for DetailedCondition types. It's cast back to a string before sending off to
// HTTPProxy structs, as those use upstream types which we can't alias easily.
type ConditionType string

// ValidCondition is the ConditionType for Valid.
const ValidCondition ConditionType = "Valid"

// NewCache creates a new Cache for holding status updates.
func NewCache(gateway types.NamespacedName) Cache {
	return Cache{
		proxyUpdates:   make(map[types.NamespacedName]*ProxyUpdate),
		gatewayRef:     gateway,
		gatewayUpdates: make(map[types.NamespacedName]*GatewayConditionsUpdate),
		routeUpdates:   make(map[types.NamespacedName]*RouteConditionsUpdate),
		entries:        make(map[string]map[types.NamespacedName]CacheEntry),
	}
}

type CacheEntry interface {
	AsStatusUpdate() k8s.StatusUpdate
	ConditionFor(ConditionType) *contour_api_v1.DetailedCondition
}

// Cache holds status updates from the DAG back towards Kubernetes.
// It holds a per-Kind cache, and is intended to be accessed with a
// KindAccessor.
type Cache struct {
	proxyUpdates map[types.NamespacedName]*ProxyUpdate

	gatewayRef     types.NamespacedName
	gatewayUpdates map[types.NamespacedName]*GatewayConditionsUpdate
	routeUpdates   map[types.NamespacedName]*RouteConditionsUpdate

	// Map of cache entry maps, keyed on Kind.
	entries map[string]map[types.NamespacedName]CacheEntry
}

// Get returns a pointer to a the cache entry if it exists, nil
// otherwise. The return value is shared between all callers, who
// should take care to cooperate.
func (c *Cache) Get(obj metav1.Object) CacheEntry {
	kind := k8s.KindOf(obj)

	if _, ok := c.entries[kind]; !ok {
		c.entries[kind] = make(map[types.NamespacedName]CacheEntry)
	}

	return c.entries[kind][k8s.NamespacedNameOf(obj)]
}

// Put returns an entry to the cache.
func (c *Cache) Put(obj metav1.Object, e CacheEntry) {
	kind := k8s.KindOf(obj)

	if _, ok := c.entries[kind]; !ok {
		c.entries[kind] = make(map[types.NamespacedName]CacheEntry)
	}

	c.entries[kind][k8s.NamespacedNameOf(obj)] = e
}

// GetStatusUpdates returns a slice of StatusUpdates, ready to be sent off
// to the StatusUpdater by the event handler.
// As more kinds are handled by Cache, we'll update this method.
func (c *Cache) GetStatusUpdates() []k8s.StatusUpdate {
	var flattened []k8s.StatusUpdate

	for fullname, pu := range c.proxyUpdates {
		update := k8s.StatusUpdate{
			NamespacedName: fullname,
			Resource:       contour_api_v1.HTTPProxyGVR,
			Mutator:        pu,
		}

		flattened = append(flattened, update)
	}

	for fullname, routeUpdate := range c.routeUpdates {
		update := k8s.StatusUpdate{
			NamespacedName: fullname,
			Resource: schema.GroupVersionResource{
				Group:    gatewayapi_v1alpha1.GroupVersion.Group,
				Version:  gatewayapi_v1alpha1.GroupVersion.Version,
				Resource: routeUpdate.Resource,
			},
			Mutator: routeUpdate,
		}

		flattened = append(flattened, update)
	}

	for fullname, gwUpdate := range c.gatewayUpdates {
		update := k8s.StatusUpdate{
			NamespacedName: fullname,
			Resource: schema.GroupVersionResource{
				Group:    gatewayapi_v1alpha1.GroupVersion.Group,
				Version:  gatewayapi_v1alpha1.GroupVersion.Version,
				Resource: gwUpdate.Resource,
			},
			Mutator: gwUpdate,
		}

		flattened = append(flattened, update)
	}

	for _, byKind := range c.entries {
		for _, e := range byKind {
			flattened = append(flattened, e.AsStatusUpdate())
		}
	}

	return flattened
}

// GetProxyUpdates gets the underlying ProxyUpdate objects
// from the cache, used by various things (`internal/contour/metrics.go` and `internal/dag/status_test.go`)
// to retrieve info they need.
// TODO(youngnick)#2969: This could conceivably be replaced with a Walk pattern.
func (c *Cache) GetProxyUpdates() []*ProxyUpdate {
	var allUpdates []*ProxyUpdate
	for _, pu := range c.proxyUpdates {
		allUpdates = append(allUpdates, pu)
	}
	return allUpdates
}

// GetGatewayUpdates gets the underlying GatewayConditionsUpdate objects from the cache.
func (c *Cache) GetGatewayUpdates() []*GatewayConditionsUpdate {
	var allUpdates []*GatewayConditionsUpdate
	for _, conditionsUpdate := range c.gatewayUpdates {
		allUpdates = append(allUpdates, conditionsUpdate)
	}
	return allUpdates
}

// GetRouteUpdates gets the underlying RouteConditionsUpdate objects from the cache.
func (c *Cache) GetRouteUpdates() []*RouteConditionsUpdate {
	var allUpdates []*RouteConditionsUpdate
	for _, conditionsUpdate := range c.routeUpdates {
		allUpdates = append(allUpdates, conditionsUpdate)
	}
	return allUpdates
}
