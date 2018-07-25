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
	"github.com/prometheus/client_golang/prometheus"
)

// Metrics provide Prometheus metrics for the app
type Metrics struct {
	ingressRouteTotalGauge     *prometheus.GaugeVec
	ingressRouteRootTotalGauge *prometheus.GaugeVec
	ingressRouteInvalidGauge   *prometheus.GaugeVec
	ingressRouteValidGauge     *prometheus.GaugeVec
	ingressRouteOrphanedGauge  *prometheus.GaugeVec

	CacheHandlerOnUpdateSummary prometheus.Summary
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
	IngressRouteTotalGauge     = "contour_ingressroute_total"
	IngressRouteRootTotalGauge = "contour_ingressroute_root_total"
	IngressRouteInvalidGauge   = "contour_ingressroute_invalid_total"
	IngressRouteValidGauge     = "contour_ingressroute_valid_total"
	IngressRouteOrphanedGauge  = "contour_ingressroute_orphaned_total"

	cacheHandlerOnUpdateSummary = "contour_cachehandler_onupdate_duration_seconds"
)

// NewMetrics creates a new set of metrics and registers them with
// the supplied registry.
func NewMetrics(registry *prometheus.Registry) *Metrics {
	m := Metrics{
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
		CacheHandlerOnUpdateSummary: prometheus.NewSummary(prometheus.SummaryOpts{
			Name:       cacheHandlerOnUpdateSummary,
			Help:       "Histogram for the runtime of xDS cache regeneration",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		}),
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
		m.CacheHandlerOnUpdateSummary,
	)
}

// SetIngressRouteMetric takes
func (m *Metrics) SetIngressRouteMetric(metrics IngressRouteMetric) {
	for meta, value := range metrics.Total {
		m.ingressRouteTotalGauge.WithLabelValues(meta.Namespace).Set(float64(value))
	}
	for meta, value := range metrics.Invalid {
		m.ingressRouteInvalidGauge.WithLabelValues(meta.Namespace, meta.VHost).Set(float64(value))
	}
	for meta, value := range metrics.Orphaned {
		m.ingressRouteOrphanedGauge.WithLabelValues(meta.Namespace).Set(float64(value))
	}
	for meta, value := range metrics.Valid {
		m.ingressRouteValidGauge.WithLabelValues(meta.Namespace, meta.VHost).Set(float64(value))
	}
	for meta, value := range metrics.Root {
		m.ingressRouteRootTotalGauge.WithLabelValues(meta.Namespace).Set(float64(value))
	}
}
