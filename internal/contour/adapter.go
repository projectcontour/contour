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
	"github.com/heptio/contour/internal/cluster"
	"github.com/heptio/contour/internal/dag"
	"github.com/heptio/contour/internal/listener"
	"github.com/heptio/contour/internal/route"
)

// DAGAdapter wraps a dag.ResourceEventHandler to hook post update cache
// generation.
type DAGAdapter struct {
	dag.ResourceEventHandler // provides a Visit method
	listener.ListenerCache
	route.RouteCache
	cluster.ClusterCache
}

func (d *DAGAdapter) OnAdd(obj interface{}) {
	d.ResourceEventHandler.OnAdd(obj)
	d.updateListeners()
	d.updateRoutes()
	d.updateClusters()
}

func (d *DAGAdapter) OnUpdate(oldObj, newObj interface{}) {
	d.ResourceEventHandler.OnUpdate(oldObj, newObj)
	d.updateListeners()
	d.updateRoutes()
	d.updateClusters()
}

func (d *DAGAdapter) OnDelete(obj interface{}) {
	d.ResourceEventHandler.OnDelete(obj)
	d.updateListeners()
	d.updateRoutes()
	d.updateClusters()
}

func (d *DAGAdapter) updateListeners() {
	v := listener.Visitor{
		ListenerCache: &d.ListenerCache,
		DAG:           &d.DAG,
	}
	d.ListenerCache.Update(v.Visit())
}

func (d *DAGAdapter) updateRoutes() {
	v := route.Visitor{
		RouteCache: &d.RouteCache,
		DAG:        &d.DAG,
	}
	routes := v.Visit()
	d.RouteCache.Update(routes)
}

func (d *DAGAdapter) updateClusters() {
	v := cluster.Visitor{
		ClusterCache: &d.ClusterCache,
		DAG:          &d.DAG,
	}
	d.ClusterCache.Update(v.Visit())
}
