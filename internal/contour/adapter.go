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

// Package contour contains the translation business logic that listens
// to Kubernetes ResourceEventHandler events and translates those into
// additions/deletions in caches connected to the Envoy xDS gRPC API server.
package contour

import (
	"fmt"

	"github.com/heptio/contour/internal/dag"
	"k8s.io/api/extensions/v1beta1"
)

const DEFAULT_INGRESS_CLASS = "contour"

// DAGAdapter wraps a dag.ResourceEventHandler to hook post update cache
// generation.
type DAGAdapter struct {
	// Contour's IngressClass.
	// If not set, defaults to DEFAULT_INGRESS_CLASS.
	IngressClass string

	dag.ResourceEventHandler // provides a Visit method
	ListenerCache
	RouteCache
	ClusterCache
}

func (d *DAGAdapter) OnAdd(obj interface{}) {
	if !d.validIngressClass(obj) {
		return
	}
	d.setIngressRouteStatus(d.ResourceEventHandler.OnAdd(obj))
	d.updateListeners()
	d.updateRoutes()
	d.updateClusters()
}

func (d *DAGAdapter) OnUpdate(oldObj, newObj interface{}) {
	oldValid, newValid := d.validIngressClass(oldObj), d.validIngressClass(newObj)
	switch {
	case !oldValid && !newValid:
		// the old object did not match the ingress class, nor does
		// the new object, nothing to do
	case oldValid && !newValid:
		// if the old object was valid, and the replacement is not, then we need
		// to remove the old object and _not_ insert the new object.
		d.OnDelete(oldObj)
	default:
		d.setIngressRouteStatus(d.ResourceEventHandler.OnUpdate(oldObj, newObj))
		d.updateListeners()
		d.updateRoutes()
		d.updateClusters()
	}
}

func (d *DAGAdapter) OnDelete(obj interface{}) {
	// no need to check ingress class here
	d.setIngressRouteStatus(d.ResourceEventHandler.OnDelete(obj))
	d.updateListeners()
	d.updateRoutes()
	d.updateClusters()
}

func (d *DAGAdapter) setIngressRouteStatus(statuses dag.IngressrouteStatus) {
	for _, s := range statuses.GetStatuses() {
		fmt.Println(fmt.Sprintf("DAGVer: %d IR: %s Namespace: %s Status: %s Msg: %s", statuses.GetVersion(), s.GetIngressRouteName(), s.GetIngressRouteNamespace(), s.GetStatus(), s.GetMsg()))
	}
}

// validIngressClass returns true iff:
//
// 1. obj is not of type *v1beta1.Ingress.
// 2. obj has no ingress.class annotation.
// 2. obj's ingress.class annotation matches d.IngressClass.
func (d *DAGAdapter) validIngressClass(obj interface{}) bool {
	i, ok := obj.(*v1beta1.Ingress)
	if !ok {
		return true
	}
	class, ok := i.Annotations["kubernetes.io/ingress.class"]
	return !ok || class == d.ingressClass()
}

// ingressClass returns the IngressClass
// or DEFAULT_INGRESS_CLASS if not configured.
func (d *DAGAdapter) ingressClass() string {
	if d.IngressClass != "" {
		return d.IngressClass
	}
	return DEFAULT_INGRESS_CLASS
}

func (d *DAGAdapter) updateListeners() {
	v := listenerVisitor{
		ListenerCache: &d.ListenerCache,
		DAG:           &d.DAG,
	}
	d.ListenerCache.Update(v.Visit())
}

func (d *DAGAdapter) updateRoutes() {
	v := routeVisitor{
		RouteCache: &d.RouteCache,
		DAG:        &d.DAG,
	}
	routes := v.Visit()
	d.RouteCache.Update(routes)
}

func (d *DAGAdapter) updateClusters() {
	v := clusterVisitor{
		ClusterCache: &d.ClusterCache,
		DAG:          &d.DAG,
	}
	d.clusterCache.Update(v.Visit())
}
