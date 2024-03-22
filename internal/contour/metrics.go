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

	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/client-go/tools/cache"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/k8s"
	"github.com/projectcontour/contour/internal/metrics"
	"github.com/projectcontour/contour/internal/status"
)

// EventRecorder records the count and kind of events forwarded
// to another ResourceEventHandler.
type EventRecorder struct {
	Next    cache.ResourceEventHandler
	Counter *prometheus.CounterVec
}

func (e *EventRecorder) OnAdd(obj any, isInInitialList bool) {
	e.recordOperation("add", obj)
	e.Next.OnAdd(obj, isInInitialList)
}

func (e *EventRecorder) OnUpdate(oldObj, newObj any) {
	e.recordOperation("update", newObj) // the api server guarantees that an object's kind cannot be updated
	e.Next.OnUpdate(oldObj, newObj)
}

func (e *EventRecorder) OnDelete(obj any) {
	e.recordOperation("delete", obj)
	e.Next.OnDelete(obj)
}

func (e *EventRecorder) recordOperation(op string, obj any) {
	kind := k8s.KindOf(obj)
	if kind == "" {
		kind = "unknown"
	}
	e.Counter.WithLabelValues(op, kind).Inc()
}

// RebuildMetricsObserver is a dag.Observer that emits metrics for DAG rebuilds.
type RebuildMetricsObserver struct {
	// Metrics to emit.
	metrics *metrics.Metrics

	// httpProxyMetricsEnabled will become ready to read when this EventHandler becomes
	// the leader. If httpProxyMetricsEnabled is not readable, or nil, status events will
	// be suppressed.
	httpProxyMetricsEnabled chan struct{}

	// NextObserver contains the stack of dag.Observers that act on DAG rebuilds.
	nextObserver dag.Observer
}

func NewRebuildMetricsObserver(metrics *metrics.Metrics, nextObserver dag.Observer) *RebuildMetricsObserver {
	return &RebuildMetricsObserver{
		metrics:                 metrics,
		nextObserver:            nextObserver,
		httpProxyMetricsEnabled: make(chan struct{}),
	}
}

func (m *RebuildMetricsObserver) OnElectedLeader() {
	close(m.httpProxyMetricsEnabled)
}

func (m *RebuildMetricsObserver) OnChange(d *dag.DAG) {
	m.metrics.SetDAGLastRebuilt(time.Now())
	m.metrics.SetDAGRebuiltTotal()

	timer := prometheus.NewTimer(m.metrics.CacheHandlerOnUpdateSummary)
	m.nextObserver.OnChange(d)
	timer.ObserveDuration()

	select {
	case <-m.httpProxyMetricsEnabled:
		m.metrics.SetHTTPProxyMetric(calculateRouteMetric(d.StatusCache.GetProxyUpdates()))
	default:
	}
}

func calculateRouteMetric(updates []*status.ProxyUpdate) metrics.RouteMetric {
	proxyMetricTotal := make(map[metrics.Meta]int)
	proxyMetricValid := make(map[metrics.Meta]int)
	proxyMetricInvalid := make(map[metrics.Meta]int)
	proxyMetricOrphaned := make(map[metrics.Meta]int)
	proxyMetricRoots := make(map[metrics.Meta]int)

	for _, u := range updates {
		calcMetrics(u, proxyMetricValid, proxyMetricInvalid, proxyMetricOrphaned, proxyMetricTotal)
		if u.Vhost != "" {
			proxyMetricRoots[metrics.Meta{VHost: u.Vhost, Namespace: u.Fullname.Namespace}]++
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

func calcMetrics(u *status.ProxyUpdate, metricValid, metricInvalid, metricOrphaned, metricTotal map[metrics.Meta]int) {
	validCond := u.ConditionFor(status.ValidCondition)
	switch validCond.Status {
	case contour_v1.ConditionTrue:
		metricValid[metrics.Meta{VHost: u.Vhost, Namespace: u.Fullname.Namespace}]++
	case contour_v1.ConditionFalse:
		if _, ok := validCond.GetError(contour_v1.ConditionTypeOrphanedError); ok {
			metricOrphaned[metrics.Meta{Namespace: u.Fullname.Namespace}]++
		} else {
			metricInvalid[metrics.Meta{VHost: u.Vhost, Namespace: u.Fullname.Namespace}]++
		}
	}
	metricTotal[metrics.Meta{Namespace: u.Fullname.Namespace}]++
}
