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

// Package contour contains the translation business logic that listens
// to Kubernetes ResourceEventHandler events and translates those into
// additions/deletions in caches connected to the Envoy xDS gRPC API server.
package contour

import (
	"time"

	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/k8s"
	"github.com/projectcontour/contour/internal/metrics"
	"github.com/projectcontour/contour/internal/status"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/client-go/tools/cache"
)

// EventRecorder records the count and kind of events forwarded
// to another ResourceEventHandler.
type EventRecorder struct {
	Next    cache.ResourceEventHandler
	Counter *prometheus.CounterVec
}

func (e *EventRecorder) OnAdd(obj interface{}) {
	e.recordOperation("add", obj)
	e.Next.OnAdd(obj)
}

func (e *EventRecorder) OnUpdate(oldObj, newObj interface{}) {
	e.recordOperation("update", newObj) // the api server guarantees that an object's kind cannot be updated
	e.Next.OnUpdate(oldObj, newObj)
}

func (e *EventRecorder) OnDelete(obj interface{}) {
	e.recordOperation("delete", obj)
	e.Next.OnDelete(obj)
}

func (e *EventRecorder) recordOperation(op string, obj interface{}) {
	kind := k8s.KindOf(obj)
	if kind == "" {
		kind = "unknown"
	}
	e.Counter.WithLabelValues(op, kind).Inc()
}

// RebuildMetricsObserver is a dag.Observer that emits metrics for DAG rebuilds.
type RebuildMetricsObserver struct {
	// Metrics to emit.
	Metrics *metrics.Metrics

	// IsLeader will become ready to read when this EventHandler becomes
	// the leader. If IsLeader is not readable, or nil, status events will
	// be suppressed.
	IsLeader chan struct{}

	// NextObserver contains the stack of dag.Observers that act on DAG rebuilds.
	NextObserver dag.Observer
}

func (m *RebuildMetricsObserver) OnChange(d *dag.DAG) {
	m.Metrics.SetDAGLastRebuilt(time.Now())

	timer := prometheus.NewTimer(m.Metrics.CacheHandlerOnUpdateSummary)
	m.NextObserver.OnChange(d)
	timer.ObserveDuration()

	select {
	// If we are leader, the IsLeader channel is closed.
	case <-m.IsLeader:
		m.Metrics.SetHTTPProxyMetric(calculateRouteMetric(d.StatusCache.GetProxyStatusMetrics()))
	default:
	}
}

func calculateRouteMetric(statuses []status.Metric) metrics.RouteMetric {
	proxyMetricTotal := make(map[metrics.Meta]int)
	proxyMetricValid := make(map[metrics.Meta]int)
	proxyMetricInvalid := make(map[metrics.Meta]int)
	proxyMetricOrphaned := make(map[metrics.Meta]int)
	proxyMetricRoots := make(map[metrics.Meta]int)

	for _, s := range statuses {
		calcMetrics(s, proxyMetricValid, proxyMetricInvalid, proxyMetricOrphaned, proxyMetricTotal)
		if s.Vhost != "" {
			proxyMetricRoots[metrics.Meta{Namespace: s.Namespace}]++
		}
	}

	return metrics.RouteMetric{
		Invalid:  proxyMetricInvalid,
		Valid:    proxyMetricValid,
		Orphaned: proxyMetricOrphaned,
		Total:    proxyMetricTotal,
		Root:     proxyMetricRoots,
	}
}

func calcMetrics(s status.Metric, metricValid map[metrics.Meta]int, metricInvalid map[metrics.Meta]int, metricOrphaned map[metrics.Meta]int, metricTotal map[metrics.Meta]int) {
	switch s.Status {
	case k8s.StatusValid:
		metricValid[metrics.Meta{VHost: s.Vhost, Namespace: s.Namespace}]++
	case k8s.StatusInvalid:
		metricInvalid[metrics.Meta{VHost: s.Vhost, Namespace: s.Namespace}]++
	case k8s.StatusOrphaned:
		metricOrphaned[metrics.Meta{Namespace: s.Namespace}]++
	}
	metricTotal[metrics.Meta{Namespace: s.Namespace}]++
}
