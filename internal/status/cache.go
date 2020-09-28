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

	projcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/k8s"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// NewCache creates a new Cache for holding status updates.
func NewCache() Cache {
	return Cache{
		proxyUpdates: make(map[types.NamespacedName]*ProxyUpdate),
	}
}

// Cache holds status updates from the DAG back towards Kubernetes.
// It holds a per-Kind cache, and is intended to be accessed with a
// KindAccessor.
type Cache struct {
	proxyUpdates map[types.NamespacedName]*ProxyUpdate
}

// ProxyAccessor returns a ProxyUpdate that allows a client to build up a list of
// errors and warnings to go onto the proxy as conditions, and a function to commit the change
// back to the cache when everything is done.
// The commit function pattern is used so that the ProxyUpdate does not need to know anything
// the cache internals.
func (c Cache) ProxyAccessor(proxy *projcontour.HTTPProxy) (*ProxyUpdate, func()) {
	pu := &ProxyUpdate{
		Fullname:       k8s.NamespacedNameOf(proxy),
		Generation:     proxy.Generation,
		TransitionTime: v1.NewTime(time.Now()),
		Conditions:     make(map[ConditionType]*projcontour.DetailedCondition),
	}

	return pu, func() {
		c.commitProxy(pu)
	}
}

func (c Cache) commitProxy(pu *ProxyUpdate) {
	if len(pu.Conditions) == 0 {
		return
	}

	c.proxyUpdates[pu.Fullname] = pu
}

// GetStatusUpdates returns a slice of StatusUpdates, ready to be sent off
// to the StatusUpdater by the event handler.
// As more kinds are handled by Cache, we'll update this method.
func (c Cache) GetStatusUpdates() []k8s.StatusUpdate {
	return c.getProxyStatusUpdates()
}

func (c Cache) getProxyStatusUpdates() []k8s.StatusUpdate {
	var psu []k8s.StatusUpdate

	for fullname, pu := range c.proxyUpdates {

		update := k8s.StatusUpdate{
			NamespacedName: fullname,
			Resource:       projcontour.HTTPProxyGVR,
			Mutator:        pu,
		}

		psu = append(psu, update)
	}
	return psu

}
