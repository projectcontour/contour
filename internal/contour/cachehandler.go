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
)

// CacheHandler manages the state of xDS caches.
type CacheHandler struct {
	ListenerCache
	RouteCache
	ClusterCache

	IngressRouteStatus *k8s.IngressRouteStatus
	logrus.FieldLogger
	metrics.Metrics
}

type statusable interface {
	Statuses() []dag.Status
}

func (ch *CacheHandler) update(b *dag.Builder) {
	dag := b.Compute()
	ch.setIngressRouteStatus(dag)
	ch.updateListeners(dag)
	ch.updateRoutes(dag)
	ch.updateClusters(dag)
	ch.updateIngressRouteMetric(dag)
}

func (ch *CacheHandler) setIngressRouteStatus(st statusable) {
	for _, s := range st.Statuses() {
		err := ch.IngressRouteStatus.SetStatus(s.Status, s.Description, s.Object)
		if err != nil {
			ch.Errorf("Error Setting Status of IngressRoute: ", err)
		}
	}
}

func (ch *CacheHandler) updateListeners(v dag.Visitable) {
	lv := listenerVisitor{
		ListenerCache: &ch.ListenerCache,
		Visitable:     v,
	}
	ch.ListenerCache.Update(lv.Visit())
}

func (ch *CacheHandler) updateRoutes(v dag.Visitable) {
	rv := routeVisitor{
		RouteCache: &ch.RouteCache,
		Visitable:  v,
	}
	routes := rv.Visit()
	ch.RouteCache.Update(routes)
}

func (ch *CacheHandler) updateClusters(v dag.Visitable) {
	cv := clusterVisitor{
		ClusterCache: &ch.ClusterCache,
		Visitable:    v,
	}
	ch.clusterCache.Update(cv.Visit())
}

func (ch *CacheHandler) updateIngressRouteMetric(v dag.Visitable) {
	metrics := calculateIngressRouteMetric(v)
	ch.Metrics.SetIngressRouteMetric(metrics)
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
