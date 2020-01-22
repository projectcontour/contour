// Copyright © 2020 VMware
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
	"io"

	"github.com/prometheus/common/expfmt"
)

// ParseOpenConnections returns the sum of open connections from a Prometheus HTTP request
func ParseOpenConnections(stats io.Reader, prometheusStat string, prometheusValues []string) (int, error) {
	var parser expfmt.TextParser
	var openConnections = 0

	if stats == nil {
		return -1, fmt.Errorf("stats input was nil")
	}

	// Parse Prometheus http response
	metricFamilies, err := parser.TextToMetricFamilies(stats)
	if err != nil {
		return -1, fmt.Errorf("parsing prometheus text format failed: %v", err)
	}

	// Validate stat exists in output
	if _, ok := metricFamilies[prometheusStat]; !ok {
		return -1, fmt.Errorf("prometheus stat [%s] not found in request result", prometheusStat)
	}

	// Look up open connections value
	for _, metrics := range metricFamilies[prometheusStat].Metric {
		for _, labels := range metrics.Label {
			for _, item := range prometheusValues {
				if item == *labels.Value {
					openConnections += int(*metrics.Gauge.Value)
				}
			}
		}
	}
	return openConnections, nil
}
