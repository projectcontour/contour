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
	"fmt"
	"net/http"
	"time"

	"github.com/projectcontour/contour/internal/build"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics provide Prometheus metrics for the app
type Metrics struct {
	buildInfoGauge *prometheus.GaugeVec

	deprecatedProxyTotalGauge     *prometheus.GaugeVec
	deprecatedProxyRootTotalGauge *prometheus.GaugeVec
	deprecatedProxyInvalidGauge   *prometheus.GaugeVec
	deprecatedProxyValidGauge     *prometheus.GaugeVec
	deprecatedProxyOrphanedGauge  *prometheus.GaugeVec

	proxyTotalGauge     *prometheus.GaugeVec
	proxyRootTotalGauge *prometheus.GaugeVec
	proxyInvalidGauge   *prometheus.GaugeVec
	proxyValidGauge     *prometheus.GaugeVec
	proxyOrphanedGauge  *prometheus.GaugeVec

	dagRebuildGauge             *prometheus.GaugeVec
	dagRebuildTotal             prometheus.Counter
	CacheHandlerOnUpdateSummary prometheus.Summary
	EventHandlerOperations      *prometheus.CounterVec

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

	DeprecatedHTTPProxyTotalGauge     = "contour_httpproxy_total"
	DeprecatedHTTPProxyRootTotalGauge = "contour_httpproxy_root_total"
	DeprecatedHTTPProxyInvalidGauge   = "contour_httpproxy_invalid_total"
	DeprecatedHTTPProxyValidGauge     = "contour_httpproxy_valid_total"
	DeprecatedHTTPProxyOrphanedGauge  = "contour_httpproxy_orphaned_total"

	HTTPProxyTotalGauge     = "contour_httpproxy"
	HTTPProxyRootTotalGauge = "contour_httpproxy_root"
	HTTPProxyInvalidGauge   = "contour_httpproxy_invalid"
	HTTPProxyValidGauge     = "contour_httpproxy_valid"
	HTTPProxyOrphanedGauge  = "contour_httpproxy_orphaned"

	DAGRebuildGauge             = "contour_dagrebuild_timestamp"
	DAGRebuildTotal             = "contour_dagrebuild_total"
	cacheHandlerOnUpdateSummary = "contour_cachehandler_onupdate_duration_seconds"
	eventHandlerOperations      = "contour_eventhandler_operation_total"
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
		deprecatedProxyTotalGauge: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: DeprecatedHTTPProxyTotalGauge,
				Help: fmt.Sprintf(
					"(Deprecated): Total number of HTTPProxies that exist regardless of status. Use %s instead",
					HTTPProxyTotalGauge),
			},
			[]string{"namespace"},
		),
		deprecatedProxyRootTotalGauge: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: DeprecatedHTTPProxyRootTotalGauge,
				Help: fmt.Sprintf(
					"(Deprecated): Total number of root HTTPProxies. Note there will only be a single root HTTPProxy per vhost. Use %s instead",
					HTTPProxyRootTotalGauge),
			},
			[]string{"namespace"},
		),
		deprecatedProxyInvalidGauge: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: DeprecatedHTTPProxyInvalidGauge,
				Help: fmt.Sprintf(
					"(Deprecated): Total number of invalid HTTPProxies. Use %s instead.",
					HTTPProxyInvalidGauge),
			},
			[]string{"namespace", "vhost"},
		),
		deprecatedProxyValidGauge: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: DeprecatedHTTPProxyValidGauge,
				Help: fmt.Sprintf(
					"(Deprecated): Total number of valid HTTPProxies. Use %s instead",
					HTTPProxyValidGauge),
			},
			[]string{"namespace", "vhost"},
		),
		deprecatedProxyOrphanedGauge: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: DeprecatedHTTPProxyOrphanedGauge,
				Help: fmt.Sprintf(
					"(Deprecated): Total number of orphaned HTTPProxies which have no root delegating to them. Use %s instead",
					HTTPProxyOrphanedGauge),
			},
			[]string{"namespace"},
		),
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
		dagRebuildGauge: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: DAGRebuildGauge,
				Help: "Timestamp of the last DAG rebuild.",
			},
			[]string{},
		),
		dagRebuildTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: DAGRebuildTotal,
				Help: "Total number of times DAG has been rebuilt since startup",
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
		m.deprecatedProxyTotalGauge,
		m.proxyRootTotalGauge,
		m.deprecatedProxyRootTotalGauge,
		m.proxyInvalidGauge,
		m.deprecatedProxyInvalidGauge,
		m.proxyValidGauge,
		m.deprecatedProxyValidGauge,
		m.proxyOrphanedGauge,
		m.deprecatedProxyOrphanedGauge,
		m.dagRebuildGauge,
		m.dagRebuildTotal,
		m.CacheHandlerOnUpdateSummary,
		m.EventHandlerOperations,
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

	prometheus.NewTimer(m.CacheHandlerOnUpdateSummary).ObserveDuration()
}

// SetDAGLastRebuilt records the last time the DAG was rebuilt.
func (m *Metrics) SetDAGLastRebuilt(ts time.Time) {
	m.dagRebuildGauge.WithLabelValues().Set(float64(ts.Unix()))
}

// SetDAGRebuiltTotal records the total number of times DAG was rebuilt
func (m *Metrics) SetDAGRebuiltTotal() {
	m.dagRebuildTotal.Inc()
}

// SetHTTPProxyMetric sets metric values for a set of HTTPProxies
func (m *Metrics) SetHTTPProxyMetric(metrics RouteMetric) {
	// Process metrics
	for meta, value := range metrics.Total {
		m.proxyTotalGauge.WithLabelValues(meta.Namespace).Set(float64(value))
		m.deprecatedProxyTotalGauge.WithLabelValues(meta.Namespace).Set(float64(value))
		delete(m.proxyMetricCache.Total, meta)
	}
	for meta, value := range metrics.Invalid {
		m.proxyInvalidGauge.WithLabelValues(meta.Namespace, meta.VHost).Set(float64(value))
		m.deprecatedProxyInvalidGauge.WithLabelValues(meta.Namespace, meta.VHost).Set(float64(value))
		delete(m.proxyMetricCache.Invalid, meta)
	}
	for meta, value := range metrics.Orphaned {
		m.proxyOrphanedGauge.WithLabelValues(meta.Namespace).Set(float64(value))
		m.deprecatedProxyOrphanedGauge.WithLabelValues(meta.Namespace).Set(float64(value))
		delete(m.proxyMetricCache.Orphaned, meta)
	}
	for meta, value := range metrics.Valid {
		m.proxyValidGauge.WithLabelValues(meta.Namespace, meta.VHost).Set(float64(value))
		m.deprecatedProxyValidGauge.WithLabelValues(meta.Namespace, meta.VHost).Set(float64(value))
		delete(m.proxyMetricCache.Valid, meta)
	}
	for meta, value := range metrics.Root {
		m.proxyRootTotalGauge.WithLabelValues(meta.Namespace).Set(float64(value))
		m.deprecatedProxyRootTotalGauge.WithLabelValues(meta.Namespace).Set(float64(value))
		delete(m.proxyMetricCache.Root, meta)
	}

	// All metrics processed, now remove what's left as they are not needed
	for meta := range m.proxyMetricCache.Total {
		m.proxyTotalGauge.DeleteLabelValues(meta.Namespace)
		m.deprecatedProxyTotalGauge.DeleteLabelValues(meta.Namespace)
	}
	for meta := range m.proxyMetricCache.Invalid {
		m.proxyInvalidGauge.DeleteLabelValues(meta.Namespace, meta.VHost)
		m.deprecatedProxyInvalidGauge.DeleteLabelValues(meta.Namespace, meta.VHost)
	}
	for meta := range m.proxyMetricCache.Orphaned {
		m.proxyOrphanedGauge.DeleteLabelValues(meta.Namespace)
		m.deprecatedProxyOrphanedGauge.DeleteLabelValues(meta.Namespace)
	}
	for meta := range m.proxyMetricCache.Valid {
		m.proxyValidGauge.DeleteLabelValues(meta.Namespace, meta.VHost)
		m.deprecatedProxyValidGauge.DeleteLabelValues(meta.Namespace, meta.VHost)
	}
	for meta := range m.proxyMetricCache.Root {
		m.proxyRootTotalGauge.DeleteLabelValues(meta.Namespace)
		m.deprecatedProxyRootTotalGauge.DeleteLabelValues(meta.Namespace)
	}

	m.proxyMetricCache = &RouteMetric{
		Total:    metrics.Total,
		Invalid:  metrics.Invalid,
		Valid:    metrics.Valid,
		Orphaned: metrics.Orphaned,
		Root:     metrics.Root,
	}
}

// Handler returns a http Handler for a metrics endpoint.
func Handler(registry *prometheus.Registry) http.Handler {
	return promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
}
