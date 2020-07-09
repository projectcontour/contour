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

package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/projectcontour/contour/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// Collect all the label names for this metric and return them as
// a comma-deparated string.
func labels(mf *dto.MetricFamily) string {
	var l []string

	for _, m := range mf.GetMetric() {
		for _, pair := range m.GetLabel() {
			l = append(l, pair.GetName())
		}
	}

	return strings.Join(l, ", ")
}

// Generate a string name for the metric type, linking to the
// Prometheus docs if we know there is a suitable target.
func typeof(mf *dto.MetricFamily) string {
	switch t := mf.GetType(); t {
	case dto.MetricType_COUNTER, dto.MetricType_GAUGE,
		dto.MetricType_SUMMARY, dto.MetricType_HISTOGRAM:
		return fmt.Sprintf(
			"[%s](https://prometheus.io/docs/concepts/metric_types/#%s)",
			t.String(), strings.ToLower(t.String()))
	default:
		return t.String()
	}
}

func main() {
	registry := prometheus.NewRegistry()
	m := metrics.NewMetrics(registry)

	m.Zero()

	family, err := registry.Gather()
	if err != nil {
		log.Fatalf("%s", err)
	}

	for _, mf := range family {
		f, err := os.OpenFile(fmt.Sprintf("%s.md", mf.GetName()), os.O_CREATE|os.O_RDWR, 0644)
		if err != nil {
			log.Fatalf("%s", err)
		}

		fmt.Fprintf(f, "---\n")
		fmt.Fprintf(f, "name: '%s'\n", mf.GetName())
		fmt.Fprintf(f, "type: '%s'\n", typeof(mf))
		fmt.Fprintf(f, "labels: '%s'\n", labels(mf))
		fmt.Fprintf(f, "---\n")
		fmt.Fprintf(f, "\n%s\n", mf.GetHelp())

		f.Close()
	}
}
