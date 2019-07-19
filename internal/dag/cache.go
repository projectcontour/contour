// Copyright © 2018 Heptio
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

// Package dag provides a data model, in the form of a directed acyclic graph,
// of the relationship between Kubernetes Ingress, Service, and Secret objects.
package dag

import (
	"sync"

	v1 "k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/client-go/tools/cache"

	ingressroutev1 "github.com/heptio/contour/apis/contour/v1beta1"
)

// A KubernetesCache holds Kubernetes objects and associated configuration and produces
// DAG values.
type KubernetesCache struct {
	// IngressRouteRootNamespaces specifies the namespaces where root
	// IngressRoutes can be defined. If empty, roots can be defined in any
	// namespace.
	IngressRouteRootNamespaces []string

	mu sync.RWMutex

	ingresses     map[Meta]*v1beta1.Ingress
	ingressroutes map[Meta]*ingressroutev1.IngressRoute
	secrets       map[Meta]*v1.Secret
	delegations   map[Meta]*ingressroutev1.TLSCertificateDelegation
	services      map[Meta]*v1.Service
}

// Meta holds the name and namespace of a Kubernetes object.
type Meta struct {
	name, namespace string
}

// Insert inserts obj into the KubernetesCache.
// If an object with a matching type, name, and namespace exists, it will be overwritten.
func (kc *KubernetesCache) Insert(obj interface{}) {
	kc.mu.Lock()
	defer kc.mu.Unlock()
	switch obj := obj.(type) {
	case *v1.Secret:
		m := Meta{name: obj.Name, namespace: obj.Namespace}
		if kc.secrets == nil {
			kc.secrets = make(map[Meta]*v1.Secret)
		}
		kc.secrets[m] = obj
	case *v1.Service:
		m := Meta{name: obj.Name, namespace: obj.Namespace}
		if kc.services == nil {
			kc.services = make(map[Meta]*v1.Service)
		}
		kc.services[m] = obj
	case *v1beta1.Ingress:
		m := Meta{name: obj.Name, namespace: obj.Namespace}
		if kc.ingresses == nil {
			kc.ingresses = make(map[Meta]*v1beta1.Ingress)
		}
		kc.ingresses[m] = obj
	case *ingressroutev1.IngressRoute:
		m := Meta{name: obj.Name, namespace: obj.Namespace}
		if kc.ingressroutes == nil {
			kc.ingressroutes = make(map[Meta]*ingressroutev1.IngressRoute)
		}
		kc.ingressroutes[m] = obj
	case *ingressroutev1.TLSCertificateDelegation:
		m := Meta{name: obj.Name, namespace: obj.Namespace}
		if kc.delegations == nil {
			kc.delegations = make(map[Meta]*ingressroutev1.TLSCertificateDelegation)
		}
		kc.delegations[m] = obj

	default:
		// not an interesting object
	}
}

// Remove removes obj from the KubernetesCache.
// If no object with a matching type, name, and namespace exists in the DAG, no action is taken.
func (kc *KubernetesCache) Remove(obj interface{}) {
	switch obj := obj.(type) {
	default:
		kc.remove(obj)
	case cache.DeletedFinalStateUnknown:
		kc.Remove(obj.Obj) // recurse into ourselves with the tombstoned value
	}
}

func (kc *KubernetesCache) remove(obj interface{}) {
	kc.mu.Lock()
	defer kc.mu.Unlock()
	switch obj := obj.(type) {
	case *v1.Secret:
		m := Meta{name: obj.Name, namespace: obj.Namespace}
		delete(kc.secrets, m)
	case *v1.Service:
		m := Meta{name: obj.Name, namespace: obj.Namespace}
		delete(kc.services, m)
	case *v1beta1.Ingress:
		m := Meta{name: obj.Name, namespace: obj.Namespace}
		delete(kc.ingresses, m)
	case *ingressroutev1.IngressRoute:
		m := Meta{name: obj.Name, namespace: obj.Namespace}
		delete(kc.ingressroutes, m)
	case *ingressroutev1.TLSCertificateDelegation:
		m := Meta{name: obj.Name, namespace: obj.Namespace}
		delete(kc.delegations, m)
	default:
		// not interesting
	}
}
