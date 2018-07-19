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
	"os"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
)

// Metrics provide Prometheus metrics for the app
type Metrics struct {
	Registry *prometheus.Registry
	Metrics  map[string]prometheus.Collector
	logrus.FieldLogger
}

const (
	IngressRouteTotalGauge = "contour_ingressroute_total"
)

// NewMetrics returns a map of Prometheus metrics
func NewMetrics(logger logrus.FieldLogger) Metrics {
	return Metrics{
		Registry:    prometheus.NewRegistry(),
		FieldLogger: logger,
		Metrics: map[string]prometheus.Collector{
			IngressRouteTotalGauge: prometheus.NewGaugeVec(
				prometheus.GaugeOpts{
					Name: IngressRouteTotalGauge,
					Help: "Total number of IngressRoutes",
				},
				[]string{"namespace", "vhost"},
			),
		},
	}
}

// RegisterPrometheus registers the Metrics
func (m *Metrics) RegisterPrometheus(registerDefault bool) {

	if registerDefault {
		// Register detault process / go collectors
		m.Registry.MustRegister(prometheus.NewProcessCollector(os.Getpid(), ""))
		m.Registry.MustRegister(prometheus.NewGoCollector())
	}

	// Register with Prometheus's default registry
	for _, v := range m.Metrics {
		m.Registry.MustRegister(v)
	}
}

func (m *Metrics) SetIngressRouteMetric(ingressRouteMetric map[string]int) {
	for k, v := range ingressRouteMetric {
		values := strings.Split(k, "|")

		// Check for valid
		if len(values) != 2 {
			m.FieldLogger.Errorf("Expected proper key for IngressRouteMetric. Got: %s", k)
			continue
		}

		m, ok := m.Metrics[IngressRouteTotalGauge].(*prometheus.GaugeVec)
		if ok {
			m.WithLabelValues(values[1], values[0]).Set(float64(v))
		}
	}
}
