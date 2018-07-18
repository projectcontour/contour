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
	"reflect"
	"testing"

	"github.com/prometheus/client_model/go"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
)

func TestWriteMetric(t *testing.T) {
	tests := map[string]struct {
		irStatus map[string]int
		want     []*io_prometheus_client.Metric
	}{
		"simple": {
			irStatus: map[string]int{
				"bar.com|default": 2,
				"foo.com|default": 5,
			},
			want: []*io_prometheus_client.Metric{
				{
					Label: []*io_prometheus_client.LabelPair{{
						Name:  func() *string { i := "namespace"; return &i }(),
						Value: func() *string { i := "default"; return &i }(),
					}, {
						Name:  func() *string { i := "vhost"; return &i }(),
						Value: func() *string { i := "bar.com"; return &i }(),
					}},
					Gauge: &io_prometheus_client.Gauge{
						Value: func() *float64 { i := float64(2); return &i }(),
					},
				},
				{
					Label: []*io_prometheus_client.LabelPair{{
						Name:  func() *string { i := "namespace"; return &i }(),
						Value: func() *string { i := "default"; return &i }(),
					}, {
						Name:  func() *string { i := "vhost"; return &i }(),
						Value: func() *string { i := "foo.com"; return &i }(),
					}},
					Gauge: &io_prometheus_client.Gauge{
						Value: func() *float64 { i := float64(5); return &i }(),
					},
				},
			},
		},
		"bad key": {
			irStatus: map[string]int{
				"bar.com": 2,
			},
			want: []*io_prometheus_client.Metric{},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			m := NewMetrics(logrus.New())
			m.RegisterPrometheus(false)
			m.SetIngressRouteMetric(tc.irStatus)

			gatherers := prometheus.Gatherers{
				m.Registry,
				prometheus.DefaultGatherer,
			}

			gathering, err := gatherers.Gather()
			if err != nil {
				t.Fatal(err)
			}

			got := []*io_prometheus_client.Metric{}
			for _, mf := range gathering {
				if mf.GetName() == IngressRouteTotalGauge {
					got = mf.Metric
				}
			}

			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("write metric failed, want: %v got: %v", tc.want, got)
			}
		})
	}
}
