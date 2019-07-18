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
	"time"

	"github.com/heptio/contour/internal/dag"
	"github.com/heptio/contour/internal/k8s"
	"github.com/heptio/contour/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
)

// CacheHandler manages the state of xDS caches.
type CacheHandler struct {
	ListenerVisitorConfig
	ListenerCache
	RouteCache
	ClusterCache
	SecretCache

	IngressRouteStatus *k8s.IngressRouteStatus
	logrus.FieldLogger
	*metrics.Metrics
}

type statusable interface {
	Statuses() map[dag.Meta]dag.Status
}

func (ch *CacheHandler) OnChange(kc *dag.KubernetesCache) {
	timer := prometheus.NewTimer(ch.CacheHandlerOnUpdateSummary)
	defer timer.ObserveDuration()
	dag := dag.BuildDAG(kc)
	ch.setIngressRouteStatus(dag)
	ch.updateSecrets(dag)
	ch.updateListeners(dag)
	ch.updateRoutes(dag)
	ch.updateClusters(dag)
	ch.updateIngressRouteMetric(dag)
	ch.SetDAGLastRebuilt(time.Now())
}

func (ch *CacheHandler) setIngressRouteStatus(st statusable) {
	for _, s := range st.Statuses() {
		err := ch.IngressRouteStatus.SetStatus(s.Status, s.Description, s.Object)
		if err != nil {
			ch.Errorf("Error Setting Status of IngressRoute: %v", err)
		}
	}
}

func (ch *CacheHandler) updateSecrets(root dag.Visitable) {
	secrets := visitSecrets(root)
	ch.SecretCache.Update(secrets)
}

func (ch *CacheHandler) updateListeners(root dag.Visitable) {
	listeners := visitListeners(root, &ch.ListenerVisitorConfig)
	ch.ListenerCache.Update(listeners)
}

func (ch *CacheHandler) updateRoutes(root dag.Visitable) {
	routes := visitRoutes(root)
	ch.RouteCache.Update(routes)
}

func (ch *CacheHandler) updateClusters(root dag.Visitable) {
	clusters := visitClusters(root)
	ch.ClusterCache.Update(clusters)
}

func (ch *CacheHandler) updateIngressRouteMetric(st statusable) {
	metrics := calculateIngressRouteMetric(st)
	ch.Metrics.SetIngressRouteMetric(metrics)
}

func calculateIngressRouteMetric(st statusable) metrics.IngressRouteMetric {
	metricTotal := make(map[metrics.Meta]int)
	metricValid := make(map[metrics.Meta]int)
	metricInvalid := make(map[metrics.Meta]int)
	metricOrphaned := make(map[metrics.Meta]int)
	metricRoots := make(map[metrics.Meta]int)

	for _, v := range st.Statuses() {
		switch v.Status {
		case dag.StatusValid:
			metricValid[metrics.Meta{VHost: v.Vhost, Namespace: v.Object.GetNamespace()}]++
		case dag.StatusInvalid:
			metricInvalid[metrics.Meta{VHost: v.Vhost, Namespace: v.Object.GetNamespace()}]++
		case dag.StatusOrphaned:
			metricOrphaned[metrics.Meta{Namespace: v.Object.GetNamespace()}]++
		}
		metricTotal[metrics.Meta{Namespace: v.Object.GetNamespace()}]++

		if v.Object.Spec.VirtualHost != nil {
			metricRoots[metrics.Meta{Namespace: v.Object.GetNamespace()}]++
		}
	}

	return metrics.IngressRouteMetric{
		Invalid:  metricInvalid,
		Valid:    metricValid,
		Orphaned: metricOrphaned,
		Total:    metricTotal,
		Root:     metricRoots,
	}
}
