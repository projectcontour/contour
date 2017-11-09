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
	"sort"
	"sync"

	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"

	"github.com/heptio/contour/internal/log"
)

// DataSource provides Service, Ingress, and Endpoints caches.
type DataSource struct {
	log.Logger
	ServiceCache
	EndpointsCache
	IngressCache
}

func (ds *DataSource) OnAdd(obj interface{}) {
	switch obj := obj.(type) {
	case *v1.Service:
		ds.AddService(obj)
	case *v1.Endpoints:
		ds.AddEndpoints(obj)
	case *v1beta1.Ingress:
		ds.AddIngress(obj)
	default:
		ds.Errorf("OnAdd unexpected type %T: %#v", obj, obj)
	}
}

func (ds *DataSource) OnUpdate(_, newObj interface{}) {
	ds.OnAdd(newObj)
}

func (ds *DataSource) OnDelete(obj interface{}) {
	switch obj := obj.(type) {
	case *v1.Service:
		ds.RemoveService(obj)
	case *v1.Endpoints:
		ds.RemoveEndpoints(obj)
	case *v1beta1.Ingress:
		ds.RemoveIngress(obj)
	case cache.DeletedFinalStateUnknown:
		ds.OnDelete(obj.Obj) // recurse into ourselves with the tombstoned value
	default:
		ds.Errorf("OnDelete unexpected type %T: %#v", obj, obj)
	}
}

// ServiceCache is a goroutine safe cache of v1.Service objects.
type ServiceCache struct {
	mu       sync.Mutex                // protects following fields
	services map[types.UID]*v1.Service // map of Service.Meta.UID to Service
}

// AddService adds the Service to the ServiceCache.
// If the Service is already present in the ServiceCache
// it is replaced unconditionally.
func (sc *ServiceCache) AddService(s *v1.Service) {
	sc.apply(func(services map[types.UID]*v1.Service) {
		services[s.ObjectMeta.UID] = s
	})
}

// RemoveService removes the Service from the ServiceCache.
func (sc *ServiceCache) RemoveService(s *v1.Service) {
	sc.apply(func(services map[types.UID]*v1.Service) {
		delete(services, s.UID)
	})
}

// Each calls fn for every v1.Service in the cache in lexical order
// of the services' UID
func (sc *ServiceCache) Each(fn func(*v1.Service)) {
	sc.apply(func(services map[types.UID]*v1.Service) {
		// sort keys to ensure a stable iteration order
		keys := make([]types.UID, 0, len(services))
		for k := range services {
			keys = append(keys, k)
		}
		sort.SliceStable(keys, func(i, j int) bool {
			return keys[i] < keys[j]
		})
		for _, k := range keys {
			fn(services[k])
		}
	})
}

func (sc *ServiceCache) apply(fn func(map[types.UID]*v1.Service)) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	if sc.services == nil {
		sc.services = make(map[types.UID]*v1.Service)
	}
	fn(sc.services)
}

// EndpointsCache is a goroutine safe cache of v1.Endpoints objects.
type EndpointsCache struct {
	mu        sync.Mutex                  // protects following fields
	endpoints map[types.UID]*v1.Endpoints // map of Endpoints.Meta.UID to Endpoints
}

// AddEndpoints adds the Endpoints to the EndpointsCache.
// If the Endpoints is already present in the EndpointsCache
// it is replaced unconditionally.
func (ec *EndpointsCache) AddEndpoints(e *v1.Endpoints) {
	ec.apply(func(endpoints map[types.UID]*v1.Endpoints) {
		endpoints[e.ObjectMeta.UID] = e
	})
}

// RemoveEndpoints removes the Endpoints from the EndpointsCache.
func (ec *EndpointsCache) RemoveEndpoints(e *v1.Endpoints) {
	ec.apply(func(endpoints map[types.UID]*v1.Endpoints) {
		delete(endpoints, e.UID)
	})
}

// Each calls fn for every v1.Endpoints in the cache. The iteration order is not stable.
func (ec *EndpointsCache) Each(fn func(*v1.Endpoints)) {
	ec.apply(func(endpoints map[types.UID]*v1.Endpoints) {
		for _, ep := range endpoints {
			fn(ep)
		}
	})
}

func (ec *EndpointsCache) apply(fn func(map[types.UID]*v1.Endpoints)) {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	if ec.endpoints == nil {
		ec.endpoints = make(map[types.UID]*v1.Endpoints)
	}
	fn(ec.endpoints)
}

// IngressCacche is a goroutine safe cache of extentions.Ingress objects.
type IngressCache struct {
	mu      sync.Mutex                     // protects following fields
	ingress map[types.UID]*v1beta1.Ingress // map of Ingress.Meta.UID to Ingress
}

// AddIngress adds the Ingress to the IngressCache.
// If the Ingress is already present in the IngressCache
// it is replaced unconditionally.
func (ic *IngressCache) AddIngress(i *v1beta1.Ingress) {
	ic.apply(func(ingress map[types.UID]*v1beta1.Ingress) {
		ingress[i.ObjectMeta.UID] = i
	})
}

// RemoveIngress removes the Ingress from the IngressCache..
func (ic *IngressCache) RemoveIngress(i *v1beta1.Ingress) {
	ic.apply(func(ingress map[types.UID]*v1beta1.Ingress) {
		delete(ingress, i.UID)
	})
}

// Each calls fn for every Ingress in the cache. The iteration order is not stable.
func (ic *IngressCache) Each(fn func(*v1beta1.Ingress)) {
	ic.apply(func(ingress map[types.UID]*v1beta1.Ingress) {
		for _, i := range ingress {
			fn(i)
		}
	})
}

func (ic *IngressCache) apply(fn func(map[types.UID]*v1beta1.Ingress)) {
	ic.mu.Lock()
	defer ic.mu.Unlock()
	if ic.ingress == nil {
		ic.ingress = make(map[types.UID]*v1beta1.Ingress)
	}
	fn(ic.ingress)
}
