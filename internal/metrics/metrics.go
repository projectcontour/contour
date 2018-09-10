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

package metrics

import (
	"fmt"
	"net/http"

	"github.com/heptio/contour/internal/httpsvc"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics provide Prometheus metrics for the app
type Metrics struct {
	ingressRouteTotalGauge      *prometheus.GaugeVec
	ingressRouteRootTotalGauge  *prometheus.GaugeVec
	ingressRouteInvalidGauge    *prometheus.GaugeVec
	ingressRouteValidGauge      *prometheus.GaugeVec
	ingressRouteOrphanedGauge   *prometheus.GaugeVec
	ingressRouteDAGRebuildGauge *prometheus.GaugeVec

	CacheHandlerOnUpdateSummary prometheus.Summary
	ResourceEventHandlerSummary *prometheus.SummaryVec

	// Keep a local cache of metrics for comparison on updates
	metricCache *IngressRouteMetric
}

// IngressRouteMetric stores various metrics for IngressRoute objects
type IngressRouteMetric struct {
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
	IngressRouteTotalGauge      = "contour_ingressroute_total"
	IngressRouteRootTotalGauge  = "contour_ingressroute_root_total"
	IngressRouteInvalidGauge    = "contour_ingressroute_invalid_total"
	IngressRouteValidGauge      = "contour_ingressroute_valid_total"
	IngressRouteOrphanedGauge   = "contour_ingressroute_orphaned_total"
	IngressRouteDAGRebuildGauge = "contour_ingressroute_dagrebuild_timestamp"

	cacheHandlerOnUpdateSummary = "contour_cachehandler_onupdate_duration_seconds"
	resourceEventHandlerSummary = "contour_resourceeventhandler_duration_seconds"
)

// NewMetrics creates a new set of metrics and registers them with
// the supplied registry.
func NewMetrics(registry *prometheus.Registry) *Metrics {
	m := Metrics{
		metricCache: &IngressRouteMetric{},
		ingressRouteTotalGauge: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: IngressRouteTotalGauge,
				Help: "Total number of IngressRoutes",
			},
			[]string{"namespace"},
		),
		ingressRouteRootTotalGauge: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: IngressRouteRootTotalGauge,
				Help: "Total number of root IngressRoutes",
			},
			[]string{"namespace"},
		),
		ingressRouteInvalidGauge: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: IngressRouteInvalidGauge,
				Help: "Total number of invalid IngressRoutes",
			},
			[]string{"namespace", "vhost"},
		),
		ingressRouteValidGauge: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: IngressRouteValidGauge,
				Help: "Total number of valid IngressRoutes",
			},
			[]string{"namespace", "vhost"},
		),
		ingressRouteOrphanedGauge: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: IngressRouteOrphanedGauge,
				Help: "Total number of orphaned IngressRoutes",
			},
			[]string{"namespace"},
		),
		ingressRouteDAGRebuildGauge: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: IngressRouteDAGRebuildGauge,
				Help: "Timestamp of the last DAG rebuild",
			},
			[]string{},
		),
		CacheHandlerOnUpdateSummary: prometheus.NewSummary(prometheus.SummaryOpts{
			Name:       cacheHandlerOnUpdateSummary,
			Help:       "Histogram for the runtime of xDS cache regeneration",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		}),
		ResourceEventHandlerSummary: prometheus.NewSummaryVec(prometheus.SummaryOpts{
			Name:       resourceEventHandlerSummary,
			Help:       "Histogram for the runtime of k8s watcher events",
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
		m.ingressRouteDAGRebuildGauge,
		m.CacheHandlerOnUpdateSummary,
		m.ResourceEventHandlerSummary,
	)
}

// SetDAGRebuiltMetric records the last time the DAG was rebuilt
func (m *Metrics) SetDAGRebuiltMetric(timestamp int64) {
	m.ingressRouteDAGRebuildGauge.WithLabelValues().Set(float64(timestamp))
}

// SetIngressRouteMetric sets metric values for a set of IngressRoutes
func (m *Metrics) SetIngressRouteMetric(metrics IngressRouteMetric) {
	// Process metrics
	for meta, value := range metrics.Total {
		m.ingressRouteTotalGauge.WithLabelValues(meta.Namespace).Set(float64(value))
		if _, ok := m.metricCache.Total[meta]; ok {
			delete(m.metricCache.Total, meta)
		}
	}
	for meta, value := range metrics.Invalid {
		m.ingressRouteInvalidGauge.WithLabelValues(meta.Namespace, meta.VHost).Set(float64(value))
		if _, ok := m.metricCache.Invalid[meta]; ok {
			delete(m.metricCache.Invalid, meta)
		}
	}
	for meta, value := range metrics.Orphaned {
		m.ingressRouteOrphanedGauge.WithLabelValues(meta.Namespace).Set(float64(value))
		if _, ok := m.metricCache.Orphaned[meta]; ok {
			delete(m.metricCache.Orphaned, meta)
		}
	}
	for meta, value := range metrics.Valid {
		m.ingressRouteValidGauge.WithLabelValues(meta.Namespace, meta.VHost).Set(float64(value))
		if _, ok := m.metricCache.Valid[meta]; ok {
			delete(m.metricCache.Valid, meta)
		}
	}
	for meta, value := range metrics.Root {
		m.ingressRouteRootTotalGauge.WithLabelValues(meta.Namespace).Set(float64(value))
		if _, ok := m.metricCache.Root[meta]; ok {
			delete(m.metricCache.Root, meta)
		}
	}

	// All metrics processed, now remove what's left as they are not needed
	for meta := range m.metricCache.Total {
		m.ingressRouteTotalGauge.DeleteLabelValues(meta.Namespace)
	}
	for meta := range m.metricCache.Invalid {
		m.ingressRouteInvalidGauge.DeleteLabelValues(meta.Namespace, meta.VHost)
	}
	for meta := range m.metricCache.Orphaned {
		m.ingressRouteOrphanedGauge.DeleteLabelValues(meta.Namespace)
	}
	for meta := range m.metricCache.Valid {
		m.ingressRouteValidGauge.DeleteLabelValues(meta.Namespace, meta.VHost)
	}
	for meta := range m.metricCache.Root {
		m.ingressRouteRootTotalGauge.DeleteLabelValues(meta.Namespace)
	}

	// copier.Copy(&m.metricCache, metrics)
	m.metricCache = &IngressRouteMetric{
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
}

// Start fulfills the g.Start contract.
// When stop is closed the http server will shutdown.
func (svc *Service) Start(stop <-chan struct{}) error {
	registerHealthCheck(&svc.ServeMux)
	registerMetrics(&svc.ServeMux, svc.Registry)

	return svc.Service.Start(stop)
}

func registerHealthCheck(mux *http.ServeMux) {
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "OK")
	})
}

func registerMetrics(mux *http.ServeMux, registry *prometheus.Registry) {
	mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
}
