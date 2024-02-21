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

// Package metrics provides Prometheus metrics for Contour.
package metrics

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/projectcontour/contour/internal/build"
)

// Metrics provide Prometheus metrics for the app
type Metrics struct {
	buildInfoGauge *prometheus.GaugeVec

	proxyTotalGauge     *prometheus.GaugeVec
	proxyRootTotalGauge *prometheus.GaugeVec
	proxyInvalidGauge   *prometheus.GaugeVec
	proxyValidGauge     *prometheus.GaugeVec
	proxyOrphanedGauge  *prometheus.GaugeVec

	dagRebuildGauge             prometheus.Gauge
	dagCacheObjectGauge         *prometheus.GaugeVec
	dagRebuildTotal             prometheus.Counter
	DAGRebuildSeconds           prometheus.Summary
	CacheHandlerOnUpdateSummary prometheus.Summary
	EventHandlerOperations      *prometheus.CounterVec

	statusUpdateTotal           *prometheus.CounterVec
	statusUpdateSuccess         *prometheus.CounterVec
	statusUpdateFailed          *prometheus.CounterVec
	statusUpdateConflict        *prometheus.CounterVec
	statusUpdateNoop            *prometheus.CounterVec
	statusUpdateDurationSeconds *prometheus.SummaryVec

	// Keep a local cache of metrics for comparison on updates
	proxyMetricCache *RouteMetric
}

// RouteMetric stores various metrics for HTTPProxy objects
type RouteMetric struct {
	Total    map[Meta]int
	Valid    map[Meta]int
	Invalid  map[Meta]int
	Orphaned map[Meta]int
	Root     map[Meta]int
}

// Meta holds the vhost and namespace of a metric object
type Meta struct {
	VHost, Namespace string
}

const (
	BuildInfoGauge = "contour_build_info"

	HTTPProxyTotalGauge     = "contour_httpproxy"
	HTTPProxyRootTotalGauge = "contour_httpproxy_root"
	HTTPProxyInvalidGauge   = "contour_httpproxy_invalid"
	HTTPProxyValidGauge     = "contour_httpproxy_valid"
	HTTPProxyOrphanedGauge  = "contour_httpproxy_orphaned"

	DAGCacheObjectGauge         = "contour_dag_cache_object"
	DAGRebuildGauge             = "contour_dagrebuild_timestamp"
	DAGRebuildTotal             = "contour_dagrebuild_total"
	DAGRebuildSeconds           = "contour_dagrebuild_seconds"
	cacheHandlerOnUpdateSummary = "contour_cachehandler_onupdate_duration_seconds"
	eventHandlerOperations      = "contour_eventhandler_operation_total"

	statusUpdateTotal           = "contour_status_update_total"
	statusUpdateSuccess         = "contour_status_update_success_total"
	statusUpdateFailed          = "contour_status_update_failed_total"
	statusUpdateConflict        = "contour_status_update_conflict_total"
	statusUpdateNoop            = "contour_status_update_noop_total"
	statusUpdateDurationSeconds = "contour_status_update_duration_seconds"
)

// NewMetrics creates a new set of metrics and registers them with
// the supplied registry.
//
// NOTE: when adding new metrics, update Zero() and run
// `./hack/generate-metrics-doc.go` using `make generate-metrics-docs`
// to regenerate the metrics documentation.
func NewMetrics(registry *prometheus.Registry) *Metrics {
	m := Metrics{
		buildInfoGauge: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: BuildInfoGauge,
				Help: "Build information for Contour. Labels include the branch and git SHA that Contour was built from, and the Contour version.",
			},
			[]string{"branch", "revision", "version"},
		),
		proxyMetricCache: &RouteMetric{},
		proxyTotalGauge: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: HTTPProxyTotalGauge,
				Help: "Total number of HTTPProxies that exist regardless of status.",
			},
			[]string{"namespace"},
		),
		proxyRootTotalGauge: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: HTTPProxyRootTotalGauge,
				Help: "Total number of root HTTPProxies. Note there will only be a single root HTTPProxy per vhost.",
			},
			[]string{"namespace"},
		),
		proxyInvalidGauge: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: HTTPProxyInvalidGauge,
				Help: "Total number of invalid HTTPProxies.",
			},
			[]string{"namespace", "vhost"},
		),
		proxyValidGauge: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: HTTPProxyValidGauge,
				Help: "Total number of valid HTTPProxies.",
			},
			[]string{"namespace", "vhost"},
		),
		proxyOrphanedGauge: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: HTTPProxyOrphanedGauge,
				Help: "Total number of orphaned HTTPProxies which have no root delegating to them.",
			},
			[]string{"namespace"},
		),
		dagRebuildGauge: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: DAGRebuildGauge,
				Help: "Timestamp of the last DAG rebuild.",
			},
		),
		dagCacheObjectGauge: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: DAGCacheObjectGauge,
				Help: "Total number of items that are currently in the DAG cache.",
			},
			[]string{"kind"},
		),
		dagRebuildTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: DAGRebuildTotal,
				Help: "Total number of times DAG has been rebuilt since startup",
			},
		),
		DAGRebuildSeconds: prometheus.NewSummary(
			prometheus.SummaryOpts{
				Name: DAGRebuildSeconds,
				Help: "Duration in seconds of DAG rebuilds",
				Objectives: map[float64]float64{
					0.00: 0.01,
					0.25: 0.01,
					0.50: 0.01,
					0.75: 0.01,
					0.90: 0.01,
					0.95: 0.005,
					0.99: 0.001,
					1.00: 0.001,
				},
			},
		),
		CacheHandlerOnUpdateSummary: prometheus.NewSummary(prometheus.SummaryOpts{
			Name:       cacheHandlerOnUpdateSummary,
			Help:       "Histogram for the runtime of xDS cache regeneration.",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		}),
		EventHandlerOperations: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: eventHandlerOperations,
				Help: "Total number of Kubernetes object changes Contour has received by operation and object kind.",
			},
			[]string{"op", "kind"},
		),
		statusUpdateTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: statusUpdateTotal,
				Help: "Total number of status updates by object kind.",
			},
			[]string{"kind"},
		),
		statusUpdateSuccess: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: statusUpdateSuccess,
				Help: "Number of status updates that succeeded by object kind.",
			},
			[]string{"kind"},
		),
		statusUpdateNoop: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: statusUpdateNoop,
				Help: "Number of status updates that are no-ops by object kind. This is a subset of successful status updates.",
			},
			[]string{"kind"},
		),
		statusUpdateFailed: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: statusUpdateFailed,
				Help: "Number of status updates that failed by object kind.",
			},
			[]string{"kind"},
		),
		statusUpdateConflict: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: statusUpdateConflict,
				Help: "Number of status update conflicts encountered by object kind.",
			},
			[]string{"kind"},
		),
		statusUpdateDurationSeconds: prometheus.NewSummaryVec(
			prometheus.SummaryOpts{
				Name: statusUpdateDurationSeconds,
				Help: "How long a status update takes to finish.",
				Objectives: map[float64]float64{
					0.00: 0.01,
					0.25: 0.01,
					0.50: 0.01,
					0.75: 0.01,
					0.90: 0.01,
					0.95: 0.005,
					0.99: 0.001,
					1.00: 0.001,
				},
			},
			[]string{"kind", "error"},
		),
	}
	m.buildInfoGauge.WithLabelValues(build.Branch, build.Sha, build.Version).Set(1)
	m.register(registry)
	return &m
}

// register registers the Metrics with the supplied registry.
func (m *Metrics) register(registry *prometheus.Registry) {
	registry.MustRegister(
		m.buildInfoGauge,
		m.proxyTotalGauge,
		m.proxyRootTotalGauge,
		m.proxyInvalidGauge,
		m.proxyValidGauge,
		m.proxyOrphanedGauge,
		m.dagRebuildGauge,
		m.dagRebuildTotal,
		m.dagCacheObjectGauge,
		m.DAGRebuildSeconds,
		m.CacheHandlerOnUpdateSummary,
		m.EventHandlerOperations,
		m.statusUpdateTotal,
		m.statusUpdateSuccess,
		m.statusUpdateFailed,
		m.statusUpdateConflict,
		m.statusUpdateNoop,
		m.statusUpdateDurationSeconds,
	)
}

// Zero sets zero values for all the registered metrics. This is needed
// for generating metrics documentation. The prometheus.Registry()
// won't emit the metric metadata until the metric has been set.
func (m *Metrics) Zero() {
	meta := Meta{
		VHost:     "",
		Namespace: "",
	}

	zeroes := RouteMetric{
		Total:    map[Meta]int{meta: 0},
		Valid:    map[Meta]int{meta: 0},
		Invalid:  map[Meta]int{meta: 0},
		Orphaned: map[Meta]int{meta: 0},
		Root:     map[Meta]int{meta: 0},
	}

	m.SetDAGLastRebuilt(time.Now())
	m.SetHTTPProxyMetric(zeroes)
	m.EventHandlerOperations.WithLabelValues("add", "Secret").Inc()
	m.SetDAGCacheObjectMetric("kind", 1)
	m.SetStatusUpdateTotal("kind")
	m.SetStatusUpdateSuccess("kind")
	m.SetStatusUpdateNoop("kind")
	m.SetStatusUpdateFailed("kind")
	m.SetStatusUpdateConflict("kind")
	m.SetStatusUpdateDuration(time.Nanosecond, "kind", false)

	m.CacheHandlerOnUpdateSummary.Observe(0)
	m.DAGRebuildSeconds.Observe(0)
}

// SetDAGLastRebuilt records the last time the DAG was rebuilt.
func (m *Metrics) SetDAGLastRebuilt(ts time.Time) {
	m.dagRebuildGauge.Set(float64(ts.Unix()))
}

// SetDAGRebuiltTotal records the total number of times DAG was rebuilt
func (m *Metrics) SetDAGRebuiltTotal() {
	m.dagRebuildTotal.Inc()
}

// SetDAGCacheObjectMetric records the total number of items that are currently in the DAG cache.
func (m *Metrics) SetDAGCacheObjectMetric(kind string, count int) {
	if m == nil {
		return
	}
	m.dagCacheObjectGauge.WithLabelValues(kind).Set(float64(count))
}

// SetHTTPProxyMetric sets metric values for a set of HTTPProxies
func (m *Metrics) SetHTTPProxyMetric(metrics RouteMetric) {
	// Process metrics
	for meta, value := range metrics.Total {
		m.proxyTotalGauge.WithLabelValues(meta.Namespace).Set(float64(value))
		delete(m.proxyMetricCache.Total, meta)
	}
	for meta, value := range metrics.Invalid {
		m.proxyInvalidGauge.WithLabelValues(meta.Namespace, meta.VHost).Set(float64(value))
		delete(m.proxyMetricCache.Invalid, meta)
	}
	for meta, value := range metrics.Orphaned {
		m.proxyOrphanedGauge.WithLabelValues(meta.Namespace).Set(float64(value))
		delete(m.proxyMetricCache.Orphaned, meta)
	}
	for meta, value := range metrics.Valid {
		m.proxyValidGauge.WithLabelValues(meta.Namespace, meta.VHost).Set(float64(value))
		delete(m.proxyMetricCache.Valid, meta)
	}
	for meta, value := range metrics.Root {
		m.proxyRootTotalGauge.WithLabelValues(meta.Namespace).Set(float64(value))
		delete(m.proxyMetricCache.Root, meta)
	}

	// All metrics processed, now remove what's left as they are not needed
	for meta := range m.proxyMetricCache.Total {
		m.proxyTotalGauge.DeleteLabelValues(meta.Namespace)
	}
	for meta := range m.proxyMetricCache.Invalid {
		m.proxyInvalidGauge.DeleteLabelValues(meta.Namespace, meta.VHost)
	}
	for meta := range m.proxyMetricCache.Orphaned {
		m.proxyOrphanedGauge.DeleteLabelValues(meta.Namespace)
	}
	for meta := range m.proxyMetricCache.Valid {
		m.proxyValidGauge.DeleteLabelValues(meta.Namespace, meta.VHost)
	}
	for meta := range m.proxyMetricCache.Root {
		m.proxyRootTotalGauge.DeleteLabelValues(meta.Namespace)
	}

	m.proxyMetricCache = &RouteMetric{
		Total:    metrics.Total,
		Invalid:  metrics.Invalid,
		Valid:    metrics.Valid,
		Orphaned: metrics.Orphaned,
		Root:     metrics.Root,
	}
}

func (m *Metrics) SetStatusUpdateTotal(kind string) {
	m.statusUpdateTotal.With(prometheus.Labels{"kind": kind}).Inc()
}

func (m *Metrics) SetStatusUpdateSuccess(kind string) {
	m.statusUpdateSuccess.With(prometheus.Labels{"kind": kind}).Inc()
}

func (m *Metrics) SetStatusUpdateNoop(kind string) {
	m.statusUpdateNoop.With(prometheus.Labels{"kind": kind}).Inc()
}

func (m *Metrics) SetStatusUpdateFailed(kind string) {
	m.statusUpdateFailed.With(prometheus.Labels{"kind": kind}).Inc()
}

func (m *Metrics) SetStatusUpdateConflict(kind string) {
	m.statusUpdateConflict.With(prometheus.Labels{"kind": kind}).Inc()
}

func (m *Metrics) SetStatusUpdateDuration(duration time.Duration, kind string, onError bool) {
	labels := prometheus.Labels{"kind": kind, "error": "false"}
	if onError {
		labels["error"] = "true"
	}
	m.statusUpdateDurationSeconds.With(labels).Observe(duration.Seconds())
}

// Handler returns a http Handler for a metrics endpoint.
func Handler(registry *prometheus.Registry) http.Handler {
	return promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
}
