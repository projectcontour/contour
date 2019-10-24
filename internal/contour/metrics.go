// Copyright © 2019 VMware
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
	ingressroutev1 "github.com/projectcontour/contour/apis/contour/v1beta1"
	projcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/metrics"
)

func calculateRouteMetric(statuses map[dag.Meta]dag.Status) (metrics.RouteMetric, metrics.RouteMetric) {
	irMetricTotal := make(map[metrics.Meta]int)
	irMetricValid := make(map[metrics.Meta]int)
	irMetricInvalid := make(map[metrics.Meta]int)
	irMetricOrphaned := make(map[metrics.Meta]int)
	irMetricRoots := make(map[metrics.Meta]int)

	proxyMetricTotal := make(map[metrics.Meta]int)
	proxyMetricValid := make(map[metrics.Meta]int)
	proxyMetricInvalid := make(map[metrics.Meta]int)
	proxyMetricOrphaned := make(map[metrics.Meta]int)
	proxyMetricRoots := make(map[metrics.Meta]int)

	for _, v := range statuses {
		switch o := v.Object.(type) {
		case *ingressroutev1.IngressRoute:
			calcMetrics(v, irMetricValid, irMetricInvalid, irMetricOrphaned, irMetricTotal)
			if o.Spec.VirtualHost != nil {
				irMetricRoots[metrics.Meta{Namespace: v.Object.GetObjectMeta().GetNamespace()}]++
			}
		case *projcontour.HTTPProxy:
			calcMetrics(v, proxyMetricValid, proxyMetricInvalid, proxyMetricOrphaned, proxyMetricTotal)
			if o.Spec.VirtualHost != nil {
				proxyMetricRoots[metrics.Meta{Namespace: v.Object.GetObjectMeta().GetNamespace()}]++
			}
		}
	}

	return metrics.RouteMetric{
			Invalid:  irMetricInvalid,
			Valid:    irMetricValid,
			Orphaned: irMetricOrphaned,
			Total:    irMetricTotal,
			Root:     irMetricRoots,
		},
		metrics.RouteMetric{
			Invalid:  proxyMetricInvalid,
			Valid:    proxyMetricValid,
			Orphaned: proxyMetricOrphaned,
			Total:    proxyMetricTotal,
			Root:     proxyMetricRoots,
		}
}

func calcMetrics(v dag.Status, metricValid map[metrics.Meta]int, metricInvalid map[metrics.Meta]int, metricOrphaned map[metrics.Meta]int, metricTotal map[metrics.Meta]int) {
	switch v.Status {
	case dag.StatusValid:
		metricValid[metrics.Meta{VHost: v.Vhost, Namespace: v.Object.GetObjectMeta().GetNamespace()}]++
	case dag.StatusInvalid:
		metricInvalid[metrics.Meta{VHost: v.Vhost, Namespace: v.Object.GetObjectMeta().GetNamespace()}]++
	case dag.StatusOrphaned:
		metricOrphaned[metrics.Meta{Namespace: v.Object.GetObjectMeta().GetNamespace()}]++
	}
	metricTotal[metrics.Meta{Namespace: v.Object.GetObjectMeta().GetNamespace()}]++
}
