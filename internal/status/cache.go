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
	"time"

	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayapi_v1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayapi_v1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/k8s"
)

// ConditionType is used to ensure we only use a limited set of possible values
// for DetailedCondition types. It's cast back to a string before sending off to
// HTTPProxy structs, as those use upstream types which we can't alias easily.
type ConditionType string

// ValidCondition is the ConditionType for Valid.
const ValidCondition ConditionType = "Valid"

// NewCache creates a new Cache for holding status updates.
func NewCache(gateway types.NamespacedName, gatewayController gatewayapi_v1.GatewayController) Cache {
	return Cache{
		gatewayRef:              gateway,
		gatewayController:       gatewayController,
		proxyUpdates:            make(map[types.NamespacedName]*ProxyUpdate),
		gatewayUpdates:          make(map[types.NamespacedName]*GatewayStatusUpdate),
		routeUpdates:            make(map[types.NamespacedName]*RouteStatusUpdate),
		backendTLSPolicyUpdates: make(map[types.NamespacedName]*BackendTLSPolicyStatusUpdate),
		entries:                 make(map[string]map[types.NamespacedName]CacheEntry),
	}
}

type CacheEntry interface {
	AsStatusUpdate() k8s.StatusUpdate
	ConditionFor(ConditionType) *contour_v1.DetailedCondition
}

// Cache holds status updates from the DAG back towards Kubernetes.
// It holds a per-Kind cache, and is intended to be accessed with a
// KindAccessor.
type Cache struct {
	gatewayRef        types.NamespacedName
	gatewayController gatewayapi_v1.GatewayController

	proxyUpdates            map[types.NamespacedName]*ProxyUpdate
	gatewayUpdates          map[types.NamespacedName]*GatewayStatusUpdate
	routeUpdates            map[types.NamespacedName]*RouteStatusUpdate
	backendTLSPolicyUpdates map[types.NamespacedName]*BackendTLSPolicyStatusUpdate

	// Map of cache entry maps, keyed on Kind.
	entries map[string]map[types.NamespacedName]CacheEntry
}

// Get returns a pointer to a the cache entry if it exists, nil
// otherwise. The return value is shared between all callers, who
// should take care to cooperate.
func (c *Cache) Get(obj meta_v1.Object) CacheEntry {
	kind := k8s.KindOf(obj)

	if _, ok := c.entries[kind]; !ok {
		c.entries[kind] = make(map[types.NamespacedName]CacheEntry)
	}

	return c.entries[kind][k8s.NamespacedNameOf(obj)]
}

// Put returns an entry to the cache.
func (c *Cache) Put(obj meta_v1.Object, e CacheEntry) {
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

	for fullname, backendTLSPolicyUpdate := range c.backendTLSPolicyUpdates {
		update := k8s.StatusUpdate{
			NamespacedName: fullname,
			Resource:       &gatewayapi_v1alpha3.BackendTLSPolicy{},
			Mutator:        backendTLSPolicyUpdate,
		}

		flattened = append(flattened, update)
	}

	for fullname, pu := range c.proxyUpdates {
		update := k8s.StatusUpdate{
			NamespacedName: fullname,
			Resource:       &contour_v1.HTTPProxy{},
			Mutator:        pu,
		}

		flattened = append(flattened, update)
	}

	for fullname, routeUpdate := range c.routeUpdates {
		update := k8s.StatusUpdate{
			NamespacedName: fullname,
			Resource:       routeUpdate.Resource,
			Mutator:        routeUpdate,
		}

		flattened = append(flattened, update)
	}

	for fullname, gwUpdate := range c.gatewayUpdates {
		update := k8s.StatusUpdate{
			NamespacedName: fullname,
			Resource:       &gatewayapi_v1.Gateway{},
			Mutator:        gwUpdate,
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

// GetGatewayUpdates gets the underlying GatewayStatusUpdate objects from the cache.
func (c *Cache) GetGatewayUpdates() []*GatewayStatusUpdate {
	var allUpdates []*GatewayStatusUpdate
	for _, conditionsUpdate := range c.gatewayUpdates {
		allUpdates = append(allUpdates, conditionsUpdate)
	}
	return allUpdates
}

// GetRouteUpdates gets the underlying RouteConditionsUpdate objects from the cache.
func (c *Cache) GetRouteUpdates() []*RouteStatusUpdate {
	var allUpdates []*RouteStatusUpdate
	for _, conditionsUpdate := range c.routeUpdates {
		allUpdates = append(allUpdates, conditionsUpdate)
	}
	return allUpdates
}

// GetBackendTLSPolicyUpdates gets the underlying BackendTLSPolicyConditionsUpdate objects from the cache.
func (c *Cache) GetBackendTLSPolicyUpdates() []*BackendTLSPolicyStatusUpdate {
	var allUpdates []*BackendTLSPolicyStatusUpdate
	for _, conditionsUpdate := range c.backendTLSPolicyUpdates {
		allUpdates = append(allUpdates, conditionsUpdate)
	}
	return allUpdates
}

// GatewayStatusAccessor returns a GatewayStatusUpdate that allows a client to build up a list of
// status changes as well as a function to commit the change back to the cache when everything
// is done. The commit function pattern is used so that the GatewayStatusUpdate does not need
// to know anything the cache internals.
func (c *Cache) GatewayStatusAccessor(nsName types.NamespacedName, generation int64, gs *gatewayapi_v1.GatewayStatus) (*GatewayStatusUpdate, func()) {
	gu := &GatewayStatusUpdate{
		FullName:           nsName,
		Conditions:         make(map[gatewayapi_v1.GatewayConditionType]meta_v1.Condition),
		ExistingConditions: getGatewayConditions(gs),
		Generation:         generation,
		TransitionTime:     meta_v1.NewTime(time.Now()),
	}

	return gu, func() {
		if len(gu.Conditions) == 0 && len(gu.ListenerStatus) == 0 {
			return
		}
		c.gatewayUpdates[gu.FullName] = gu
	}
}

// ProxyAccessor returns a ProxyUpdate that allows a client to build up a list of
// errors and warnings to go onto the proxy as conditions, and a function to commit the change
// back to the cache when everything is done.
// The commit function pattern is used so that the ProxyUpdate does not need to know anything
// the cache internals.
func (c *Cache) ProxyAccessor(proxy *contour_v1.HTTPProxy) (*ProxyUpdate, func()) {
	pu := &ProxyUpdate{
		Fullname:       k8s.NamespacedNameOf(proxy),
		Generation:     proxy.Generation,
		TransitionTime: meta_v1.NewTime(time.Now()),
		Conditions:     make(map[ConditionType]*contour_v1.DetailedCondition),
	}

	return pu, func() {
		if len(pu.Conditions) == 0 {
			return
		}

		_, ok := c.proxyUpdates[pu.Fullname]
		if ok {
			// When we're committing, if we already have a Valid Condition with an error, and we're trying to
			// set the object back to Valid, skip the commit, as we've visited too far down.
			// If this is removed, the status reporting for when a parent delegates to a child that delegates to itself
			// will not work. Yes, I know, problems everywhere. I'm sorry.
			// TODO(youngnick)#2968: This issue has more details.
			if c.proxyUpdates[pu.Fullname].Conditions[ValidCondition].Status == contour_v1.ConditionFalse {
				if pu.Conditions[ValidCondition].Status == contour_v1.ConditionTrue {
					return
				}
			}
		}
		c.proxyUpdates[pu.Fullname] = pu
	}
}

// RouteConditionsAccessor returns a RouteStatusUpdate that allows a client to build up a list of
// meta_v1.Conditions as well as a function to commit the change back to the cache when everything
// is done. The commit function pattern is used so that the RouteStatusUpdate does not need
// to know anything the cache internals.
func (c *Cache) RouteConditionsAccessor(nsName types.NamespacedName, generation int64, resource client.Object) (*RouteStatusUpdate, func()) {
	pu := &RouteStatusUpdate{
		FullName:          nsName,
		GatewayRef:        c.gatewayRef,
		GatewayController: c.gatewayController,
		Generation:        generation,
		TransitionTime:    meta_v1.NewTime(time.Now()),
		Resource:          resource,
	}

	return pu, func() {
		if len(pu.RouteParentStatuses) == 0 {
			return
		}
		c.routeUpdates[pu.FullName] = pu
	}
}

// BackendTLSPolicyConditionsAccessor returns a BackendTLSPolicyStatusUpdate that allows a client
// to build up a list of metav1.Conditions as well as a function to commit the change back to the
// cache when everything is done. The commit function pattern is used so that the
// BackendTLSPolicyStatusUpdate does not need to know anything the cache internals.
func (c *Cache) BackendTLSPolicyConditionsAccessor(nsName types.NamespacedName, generation int64) (*BackendTLSPolicyStatusUpdate, func()) {
	pu := &BackendTLSPolicyStatusUpdate{
		FullName:          nsName,
		GatewayRef:        c.gatewayRef,
		GatewayController: c.gatewayController,
		Generation:        generation,
		TransitionTime:    meta_v1.NewTime(time.Now()),
	}

	return pu, func() {
		if len(pu.PolicyAncestorStatuses) == 0 {
			return
		}
		c.backendTLSPolicyUpdates[pu.FullName] = pu
	}
}
