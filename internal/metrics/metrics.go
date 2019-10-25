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

// Package metrics provides Prometheus metrics for Contour.
package metrics

import (
	"fmt"
	"net/http"
	"time"

	"k8s.io/client-go/kubernetes"

	"github.com/projectcontour/contour/internal/httpsvc"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics provide Prometheus metrics for the app
type Metrics struct {
	ingressRouteTotalGauge     *prometheus.GaugeVec
	ingressRouteRootTotalGauge *prometheus.GaugeVec
	ingressRouteInvalidGauge   *prometheus.GaugeVec
	ingressRouteValidGauge     *prometheus.GaugeVec
	ingressRouteOrphanedGauge  *prometheus.GaugeVec

	proxyTotalGauge     *prometheus.GaugeVec
	proxyRootTotalGauge *prometheus.GaugeVec
	proxyInvalidGauge   *prometheus.GaugeVec
	proxyValidGauge     *prometheus.GaugeVec
	proxyOrphanedGauge  *prometheus.GaugeVec

	dagRebuildGauge             *prometheus.GaugeVec
	CacheHandlerOnUpdateSummary prometheus.Summary
	ResourceEventHandlerSummary *prometheus.SummaryVec

	// Keep a local cache of metrics for comparison on updates
	ingressRouteMetricCache *RouteMetric
	proxyMetricCache        *RouteMetric
}

// RouteMetric stores various metrics for IngressRoute objects
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
	IngressRouteTotalGauge     = "contour_ingressroute_total"
	IngressRouteRootTotalGauge = "contour_ingressroute_root_total"
	IngressRouteInvalidGauge   = "contour_ingressroute_invalid_total"
	IngressRouteValidGauge     = "contour_ingressroute_valid_total"
	IngressRouteOrphanedGauge  = "contour_ingressroute_orphaned_total"

	HTTPProxyTotalGauge     = "contour_httpproxy_total"
	HTTPProxyRootTotalGauge = "contour_httpproxy_root_total"
	HTTPProxyInvalidGauge   = "contour_httpproxy_invalid_total"
	HTTPProxyValidGauge     = "contour_httpproxy_valid_total"
	HTTPProxyOrphanedGauge  = "contour_httpproxy_orphaned_total"

	DAGRebuildGauge             = "contour_dagrebuild_timestamp"
	cacheHandlerOnUpdateSummary = "contour_cachehandler_onupdate_duration_seconds"
	resourceEventHandlerSummary = "contour_resourceeventhandler_duration_seconds"
)

// NewMetrics creates a new set of metrics and registers them with
// the supplied registry.
//
// NOTE: when adding new metrics, update Zero() and run
// `./hack/generate-metrics-doc.go` using `make metrics-docs`
// to regenerate the metrics documentation.
func NewMetrics(registry *prometheus.Registry) *Metrics {
	m := Metrics{
		ingressRouteMetricCache: &RouteMetric{},
		proxyMetricCache:        &RouteMetric{},
		ingressRouteTotalGauge: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: IngressRouteTotalGauge,
				Help: "Total number of IngressRoutes that exist regardless of status",
			},
			[]string{"namespace"},
		),
		ingressRouteRootTotalGauge: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: IngressRouteRootTotalGauge,
				Help: "Total number of root IngressRoutes. Note there will only be a single root IngressRoute per vhost.",
			},
			[]string{"namespace"},
		),
		ingressRouteInvalidGauge: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: IngressRouteInvalidGauge,
				Help: "Total number of invalid IngressRoutes.",
			},
			[]string{"namespace", "vhost"},
		),
		ingressRouteValidGauge: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: IngressRouteValidGauge,
				Help: "Total number of valid IngressRoutes.",
			},
			[]string{"namespace", "vhost"},
		),
		ingressRouteOrphanedGauge: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: IngressRouteOrphanedGauge,
				Help: "Total number of orphaned IngressRoutes which have no root delegating to them.",
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
		CacheHandlerOnUpdateSummary: prometheus.NewSummary(prometheus.SummaryOpts{
			Name:       cacheHandlerOnUpdateSummary,
			Help:       "Histogram for the runtime of xDS cache regeneration.",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		}),
		ResourceEventHandlerSummary: prometheus.NewSummaryVec(prometheus.SummaryOpts{
			Name:       resourceEventHandlerSummary,
			Help:       "Histogram for the runtime of k8s watcher events.",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		},
			[]string{"op"},
		),
	}
	m.register(registry)
	return &m
}

// register registers the Metrics with the supplied registry.
func (m *Metrics) register(registry *prometheus.Registry) {
	registry.MustRegister(
		m.ingressRouteTotalGauge,
		m.ingressRouteRootTotalGauge,
		m.ingressRouteInvalidGauge,
		m.ingressRouteValidGauge,
		m.ingressRouteOrphanedGauge,
		m.proxyTotalGauge,
		m.proxyRootTotalGauge,
		m.proxyInvalidGauge,
		m.proxyValidGauge,
		m.proxyOrphanedGauge,
		m.dagRebuildGauge,
		m.CacheHandlerOnUpdateSummary,
		m.ResourceEventHandlerSummary,
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
	m.SetIngressRouteMetric(zeroes)
	m.SetHTTPProxyMetric(zeroes)

	defer prometheus.NewTimer(m.CacheHandlerOnUpdateSummary).ObserveDuration()

	// TODO(jpeach) add ResourceEventHandlerSummary when it gets used
}

// SetDAGLastRebuilt records the last time the DAG was rebuilt.
func (m *Metrics) SetDAGLastRebuilt(ts time.Time) {
	m.dagRebuildGauge.WithLabelValues().Set(float64(ts.Unix()))
}

// SetIngressRouteMetric sets metric values for a set of IngressRoutes
func (m *Metrics) SetIngressRouteMetric(metrics RouteMetric) {
	// Process metrics
	for meta, value := range metrics.Total {
		m.ingressRouteTotalGauge.WithLabelValues(meta.Namespace).Set(float64(value))
		delete(m.ingressRouteMetricCache.Total, meta)
	}
	for meta, value := range metrics.Invalid {
		m.ingressRouteInvalidGauge.WithLabelValues(meta.Namespace, meta.VHost).Set(float64(value))
		delete(m.ingressRouteMetricCache.Invalid, meta)
	}
	for meta, value := range metrics.Orphaned {
		m.ingressRouteOrphanedGauge.WithLabelValues(meta.Namespace).Set(float64(value))
		delete(m.ingressRouteMetricCache.Orphaned, meta)
	}
	for meta, value := range metrics.Valid {
		m.ingressRouteValidGauge.WithLabelValues(meta.Namespace, meta.VHost).Set(float64(value))
		delete(m.ingressRouteMetricCache.Valid, meta)
	}
	for meta, value := range metrics.Root {
		m.ingressRouteRootTotalGauge.WithLabelValues(meta.Namespace).Set(float64(value))
		delete(m.ingressRouteMetricCache.Root, meta)
	}

	// All metrics processed, now remove what's left as they are not needed
	for meta := range m.ingressRouteMetricCache.Total {
		m.ingressRouteTotalGauge.DeleteLabelValues(meta.Namespace)
	}
	for meta := range m.ingressRouteMetricCache.Invalid {
		m.ingressRouteInvalidGauge.DeleteLabelValues(meta.Namespace, meta.VHost)
	}
	for meta := range m.ingressRouteMetricCache.Orphaned {
		m.ingressRouteOrphanedGauge.DeleteLabelValues(meta.Namespace)
	}
	for meta := range m.ingressRouteMetricCache.Valid {
		m.ingressRouteValidGauge.DeleteLabelValues(meta.Namespace, meta.VHost)
	}
	for meta := range m.ingressRouteMetricCache.Root {
		m.ingressRouteRootTotalGauge.DeleteLabelValues(meta.Namespace)
	}

	m.ingressRouteMetricCache = &RouteMetric{
		Total:    metrics.Total,
		Invalid:  metrics.Invalid,
		Valid:    metrics.Valid,
		Orphaned: metrics.Orphaned,
		Root:     metrics.Root,
	}
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

// Service serves various metric and health checking endpoints
type Service struct {
	httpsvc.Service
	*prometheus.Registry
	Client *kubernetes.Clientset
}

// Start fulfills the g.Start contract.
// When stop is closed the http server will shutdown.
func (svc *Service) Start(stop <-chan struct{}) error {

	registerHealthCheck(&svc.ServeMux, svc.Client)
	registerMetrics(&svc.ServeMux, svc.Registry)

	return svc.Service.Start(stop)
}

func registerHealthCheck(mux *http.ServeMux, client *kubernetes.Clientset) {
	healthCheckHandler := func(w http.ResponseWriter, r *http.Request) {
		// Try and lookup Kubernetes server version as a quick and dirty check
		_, err := client.ServerVersion()
		if err != nil {
			msg := fmt.Sprintf("Failed Kubernetes Check: %v", err)
			http.Error(w, msg, http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "OK")
	}
	mux.HandleFunc("/health", healthCheckHandler)
	mux.HandleFunc("/healthz", healthCheckHandler)
}

func registerMetrics(mux *http.ServeMux, registry *prometheus.Registry) {
	mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
}
