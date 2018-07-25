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
	metrics map[string]prometheus.Collector
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
)

// NewMetrics returns a new Metrics value.
func NewMetrics() Metrics {
	return Metrics{
		metrics: map[string]prometheus.Collector{
			IngressRouteTotalGauge: prometheus.NewGaugeVec(
				prometheus.GaugeOpts{
					Name: IngressRouteTotalGauge,
					Help: "Total number of IngressRoutes",
				},
				[]string{"namespace"},
			),
			IngressRouteRootTotalGauge: prometheus.NewGaugeVec(
				prometheus.GaugeOpts{
					Name: IngressRouteRootTotalGauge,
					Help: "Total number of root IngressRoutes",
				},
				[]string{"namespace"},
			),
			IngressRouteInvalidGauge: prometheus.NewGaugeVec(
				prometheus.GaugeOpts{
					Name: IngressRouteInvalidGauge,
					Help: "Total number of invalid IngressRoutes",
				},
				[]string{"namespace", "vhost"},
			),
			IngressRouteValidGauge: prometheus.NewGaugeVec(
				prometheus.GaugeOpts{
					Name: IngressRouteValidGauge,
					Help: "Total number of valid IngressRoutes",
				},
				[]string{"namespace", "vhost"},
			),
			IngressRouteOrphanedGauge: prometheus.NewGaugeVec(
				prometheus.GaugeOpts{
					Name: IngressRouteOrphanedGauge,
					Help: "Total number of orphaned IngressRoutes",
				},
				[]string{"namespace"},
			),
		},
	}
}

// RegisterPrometheus registers the Metrics
func (m *Metrics) RegisterPrometheus(registry *prometheus.Registry) {
	for _, v := range m.metrics {
		registry.MustRegister(v)
	}
}

// SetIngressRouteMetric takes
func (m *Metrics) SetIngressRouteMetric(metrics IngressRouteMetric) {

	for meta, value := range metrics.Total {
		m, ok := m.metrics[IngressRouteTotalGauge].(*prometheus.GaugeVec)
		if ok {
			m.WithLabelValues(meta.Namespace).Set(float64(value))
		}
	}
	for meta, value := range metrics.Invalid {
		m, ok := m.metrics[IngressRouteInvalidGauge].(*prometheus.GaugeVec)
		if ok {
			m.WithLabelValues(meta.Namespace, meta.VHost).Set(float64(value))
		}
	}
	for meta, value := range metrics.Orphaned {
		m, ok := m.metrics[IngressRouteOrphanedGauge].(*prometheus.GaugeVec)
		if ok {
			m.WithLabelValues(meta.Namespace).Set(float64(value))
		}
	}
	for meta, value := range metrics.Valid {
		m, ok := m.metrics[IngressRouteValidGauge].(*prometheus.GaugeVec)
		if ok {
			m.WithLabelValues(meta.Namespace, meta.VHost).Set(float64(value))
		}
	}
	for meta, value := range metrics.Root {
		m, ok := m.metrics[IngressRouteRootTotalGauge].(*prometheus.GaugeVec)
		if ok {
			m.WithLabelValues(meta.Namespace).Set(float64(value))
		}
	}
}
