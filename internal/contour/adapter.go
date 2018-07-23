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

	ingressroutev1 "github.com/heptio/contour/apis/contour/v1beta1"
	"github.com/heptio/contour/internal/dag"
	"github.com/heptio/contour/internal/k8s"
	"github.com/heptio/contour/internal/metrics"
	"github.com/sirupsen/logrus"
	"k8s.io/api/extensions/v1beta1"
)

const DEFAULT_INGRESS_CLASS = "contour"

// DAGAdapter wraps a dag.DAG to hook post update cache generation.
type DAGAdapter struct {
	// Contour's IngressClass.
	// If not set, defaults to DEFAULT_INGRESS_CLASS.
	IngressClass string

	dag.Builder
	ListenerCache
	RouteCache
	ClusterCache

	IngressRouteStatus *k8s.IngressRouteStatus
	logrus.FieldLogger
	metrics.Metrics
}

func (d *DAGAdapter) OnAdd(obj interface{}) {
	if !d.validIngressClass(obj) {
		return
	}
	d.Insert(obj)
	d.update()
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
		d.Remove(oldObj)
		d.Insert(newObj)
		d.update()
	}
}

func (d *DAGAdapter) OnDelete(obj interface{}) {
	// no need to check ingress class here
	d.Remove(obj)
	d.update()
}

type statusable interface {
	Statuses() []dag.Status
}

func (d *DAGAdapter) update() {
	dag := d.Compute()
	d.setIngressRouteStatus(dag)
	d.updateListeners(dag)
	d.updateRoutes(dag)
	d.updateClusters(dag)
	d.updateIngressRouteMetric(dag)
}

func (d *DAGAdapter) setIngressRouteStatus(st statusable) {
	for _, s := range st.Statuses() {
		err := d.IngressRouteStatus.SetStatus(s.Status, s.Description, s.Object)
		if err != nil {
			d.FieldLogger.Errorf("Error Setting Status of IngressRoute: ", err)
		}
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

func (d *DAGAdapter) updateListeners(v dag.Visitable) {
	lv := listenerVisitor{
		ListenerCache: &d.ListenerCache,
		Visitable:     v,
	}
	d.ListenerCache.Update(lv.Visit())
}

func (d *DAGAdapter) updateRoutes(v dag.Visitable) {
	rv := routeVisitor{
		RouteCache: &d.RouteCache,
		Visitable:  v,
	}
	routes := rv.Visit()
	d.RouteCache.Update(routes)
}

func (d *DAGAdapter) updateClusters(v dag.Visitable) {
	cv := clusterVisitor{
		ClusterCache: &d.ClusterCache,
		Visitable:    v,
	}
	d.clusterCache.Update(cv.Visit())
}

func (d *DAGAdapter) updateIngressRouteMetric(v dag.Visitable) {
	metrics := calculateIngressRouteMetric(v)
	d.Metrics.SetIngressRouteMetric(metrics)
}

func calculateIngressRouteMetric(v dag.Visitable) map[string]int {
	ingressRouteMetric := make(map[string]int)

	v.Visit(func(v dag.Vertex) {
		switch vh := v.(type) {
		case *dag.VirtualHost:
			hostname := vh.FQDN()
			vh.Visit(func(v dag.Vertex) {
				switch r := v.(type) {
				case *dag.Route:
					switch rt := r.Object.(type) {
					case *ingressroutev1.IngressRoute:
						ingressRouteMetric[fmt.Sprintf("%s|%s", hostname, rt.ObjectMeta.Namespace)]++
					}
				}
			})
		}
	})
	return ingressRouteMetric
}
