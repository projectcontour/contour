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
	projcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/k8s"
	"github.com/projectcontour/contour/internal/metrics"
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

func calculateRouteMetric(statuses map[k8s.FullName]dag.Status) metrics.RouteMetric {
	proxyMetricTotal := make(map[metrics.Meta]int)
	proxyMetricValid := make(map[metrics.Meta]int)
	proxyMetricInvalid := make(map[metrics.Meta]int)
	proxyMetricOrphaned := make(map[metrics.Meta]int)
	proxyMetricRoots := make(map[metrics.Meta]int)

	for _, v := range statuses {
		switch o := v.Object.(type) {
		case *projcontour.HTTPProxy:
			calcMetrics(v, proxyMetricValid, proxyMetricInvalid, proxyMetricOrphaned, proxyMetricTotal)
			if o.Spec.VirtualHost != nil {
				proxyMetricRoots[metrics.Meta{Namespace: v.Object.GetObjectMeta().GetNamespace()}]++
			}
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

func calcMetrics(v dag.Status, metricValid map[metrics.Meta]int, metricInvalid map[metrics.Meta]int, metricOrphaned map[metrics.Meta]int, metricTotal map[metrics.Meta]int) {
	switch v.Status {
	case k8s.StatusValid:
		metricValid[metrics.Meta{VHost: v.Vhost, Namespace: v.Object.GetObjectMeta().GetNamespace()}]++
	case k8s.StatusInvalid:
		metricInvalid[metrics.Meta{VHost: v.Vhost, Namespace: v.Object.GetObjectMeta().GetNamespace()}]++
	case k8s.StatusOrphaned:
		metricOrphaned[metrics.Meta{Namespace: v.Object.GetObjectMeta().GetNamespace()}]++
	}
	metricTotal[metrics.Meta{Namespace: v.Object.GetObjectMeta().GetNamespace()}]++
}
