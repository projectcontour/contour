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

package metrics

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	client_model "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
)

type testMetric struct {
	metric string
	want   []*client_model.Metric
}

func TestSetDAGLastRebuilt(t *testing.T) {
	tests := map[string]struct {
		timestampMetric testMetric
		value           time.Time
	}{
		"simple": {
			value: time.Date(2009, 11, 17, 20, 34, 58, 651387237, time.UTC),
			timestampMetric: testMetric{
				metric: DAGRebuildGauge,
				want: []*client_model.Metric{
					{
						Gauge: &client_model.Gauge{
							Value: func() *float64 { i := float64(1.258490098e+09); return &i }(),
						},
					},
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			r := prometheus.NewRegistry()
			m := NewMetrics(r)
			m.SetDAGLastRebuilt(tc.value)

			gatherers := prometheus.Gatherers{
				r,
				prometheus.DefaultGatherer,
			}

			gathering, err := gatherers.Gather()
			if err != nil {
				t.Fatal(err)
			}

			gotTimestamp := []*client_model.Metric{}
			for _, mf := range gathering {
				if mf.GetName() == tc.timestampMetric.metric {
					gotTimestamp = mf.Metric
				}
			}

			assert.Equal(t, tc.timestampMetric.want, gotTimestamp)
		})
	}
}

func TestWriteProxyMetric(t *testing.T) {
	tests := map[string]struct {
		proxyMetrics RouteMetric
		total        testMetric
		valid        testMetric
		invalid      testMetric
		orphaned     testMetric
		root         testMetric
	}{
		"simple": {
			proxyMetrics: RouteMetric{
				Total: map[Meta]int{
					{Namespace: "testns"}: 6,
					{Namespace: "foons"}:  3,
				},
				Valid: map[Meta]int{
					{Namespace: "testns", VHost: "foo.com"}: 3,
				},
				Invalid: map[Meta]int{
					{Namespace: "testns", VHost: "foo.com"}: 2,
				},
				Orphaned: map[Meta]int{
					{Namespace: "testns"}: 1,
				},
				Root: map[Meta]int{
					{Namespace: "testns"}: 4,
				},
			},
			total: testMetric{
				metric: HTTPProxyTotalGauge,
				want: []*client_model.Metric{
					{
						Label: []*client_model.LabelPair{{
							Name:  func() *string { i := "namespace"; return &i }(),
							Value: func() *string { i := "foons"; return &i }(),
						}},
						Gauge: &client_model.Gauge{
							Value: func() *float64 { i := float64(3); return &i }(),
						},
					},
					{
						Label: []*client_model.LabelPair{{
							Name:  func() *string { i := "namespace"; return &i }(),
							Value: func() *string { i := "testns"; return &i }(),
						}},
						Gauge: &client_model.Gauge{
							Value: func() *float64 { i := float64(6); return &i }(),
						},
					},
				},
			},
			orphaned: testMetric{
				metric: HTTPProxyOrphanedGauge,
				want: []*client_model.Metric{
					{
						Label: []*client_model.LabelPair{{
							Name:  func() *string { i := "namespace"; return &i }(),
							Value: func() *string { i := "testns"; return &i }(),
						}},
						Gauge: &client_model.Gauge{
							Value: func() *float64 { i := float64(1); return &i }(),
						},
					},
				},
			},
			valid: testMetric{
				metric: HTTPProxyValidGauge,
				want: []*client_model.Metric{
					{
						Label: []*client_model.LabelPair{{
							Name:  func() *string { i := "namespace"; return &i }(),
							Value: func() *string { i := "testns"; return &i }(),
						}, {
							Name:  func() *string { i := "vhost"; return &i }(),
							Value: func() *string { i := "foo.com"; return &i }(),
						}},
						Gauge: &client_model.Gauge{
							Value: func() *float64 { i := float64(3); return &i }(),
						},
					},
				},
			},
			invalid: testMetric{
				metric: HTTPProxyInvalidGauge,
				want: []*client_model.Metric{
					{
						Label: []*client_model.LabelPair{{
							Name:  func() *string { i := "namespace"; return &i }(),
							Value: func() *string { i := "testns"; return &i }(),
						}, {
							Name:  func() *string { i := "vhost"; return &i }(),
							Value: func() *string { i := "foo.com"; return &i }(),
						}},
						Gauge: &client_model.Gauge{
							Value: func() *float64 { i := float64(2); return &i }(),
						},
					},
				},
			},
			root: testMetric{
				metric: HTTPProxyRootTotalGauge,
				want: []*client_model.Metric{
					{
						Label: []*client_model.LabelPair{{
							Name:  func() *string { i := "namespace"; return &i }(),
							Value: func() *string { i := "testns"; return &i }(),
						}},
						Gauge: &client_model.Gauge{
							Value: func() *float64 { i := float64(4); return &i }(),
						},
					},
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			r := prometheus.NewRegistry()
			m := NewMetrics(r)
			m.SetHTTPProxyMetric(tc.proxyMetrics)

			gatherers := prometheus.Gatherers{
				r,
				prometheus.DefaultGatherer,
			}

			gathering, err := gatherers.Gather()
			if err != nil {
				t.Fatal(err)
			}

			gotTotal := []*client_model.Metric{}
			gotValid := []*client_model.Metric{}
			gotInvalid := []*client_model.Metric{}
			gotOrphaned := []*client_model.Metric{}
			gotRoot := []*client_model.Metric{}
			for _, mf := range gathering {
				switch mf.GetName() {
				case tc.total.metric:
					gotTotal = mf.Metric
				case tc.valid.metric:
					gotValid = mf.Metric
				case tc.invalid.metric:
					gotInvalid = mf.Metric
				case tc.orphaned.metric:
					gotOrphaned = mf.Metric
				case tc.root.metric:
					gotRoot = mf.Metric
				}
			}

			assert.Equal(t, tc.total.want, gotTotal)
			assert.Equal(t, tc.valid.want, gotValid)
			assert.Equal(t, tc.invalid.want, gotInvalid)
			assert.Equal(t, tc.orphaned.want, gotOrphaned)
			assert.Equal(t, tc.root.want, gotRoot)
		})
	}
}

func TestRemoveProxyMetric(t *testing.T) {
	total := testMetric{
		metric: HTTPProxyTotalGauge,
		want: []*client_model.Metric{
			{
				Label: []*client_model.LabelPair{{
					Name:  func() *string { i := "namespace"; return &i }(),
					Value: func() *string { i := "foons"; return &i }(),
				}},
				Gauge: &client_model.Gauge{
					Value: func() *float64 { i := float64(3); return &i }(),
				},
			},
			{
				Label: []*client_model.LabelPair{{
					Name:  func() *string { i := "namespace"; return &i }(),
					Value: func() *string { i := "testns"; return &i }(),
				}},
				Gauge: &client_model.Gauge{
					Value: func() *float64 { i := float64(6); return &i }(),
				},
			},
		},
	}

	orphaned := testMetric{
		metric: HTTPProxyOrphanedGauge,
		want: []*client_model.Metric{
			{
				Label: []*client_model.LabelPair{{
					Name:  func() *string { i := "namespace"; return &i }(),
					Value: func() *string { i := "testns"; return &i }(),
				}},
				Gauge: &client_model.Gauge{
					Value: func() *float64 { i := float64(1); return &i }(),
				},
			},
		},
	}

	valid := testMetric{
		metric: HTTPProxyValidGauge,
		want: []*client_model.Metric{
			{
				Label: []*client_model.LabelPair{{
					Name:  func() *string { i := "namespace"; return &i }(),
					Value: func() *string { i := "testns"; return &i }(),
				}, {
					Name:  func() *string { i := "vhost"; return &i }(),
					Value: func() *string { i := "foo.com"; return &i }(),
				}},
				Gauge: &client_model.Gauge{
					Value: func() *float64 { i := float64(3); return &i }(),
				},
			},
		},
	}

	invalid := testMetric{
		metric: HTTPProxyInvalidGauge,
		want: []*client_model.Metric{
			{
				Label: []*client_model.LabelPair{{
					Name:  func() *string { i := "namespace"; return &i }(),
					Value: func() *string { i := "testns"; return &i }(),
				}, {
					Name:  func() *string { i := "vhost"; return &i }(),
					Value: func() *string { i := "foo.com"; return &i }(),
				}},
				Gauge: &client_model.Gauge{
					Value: func() *float64 { i := float64(2); return &i }(),
				},
			},
		},
	}

	root := testMetric{
		metric: HTTPProxyRootTotalGauge,
		want: []*client_model.Metric{
			{
				Label: []*client_model.LabelPair{{
					Name:  func() *string { i := "namespace"; return &i }(),
					Value: func() *string { i := "testns"; return &i }(),
				}},
				Gauge: &client_model.Gauge{
					Value: func() *float64 { i := float64(4); return &i }(),
				},
			},
		},
	}

	tests := map[string]struct {
		irMetrics        RouteMetric
		irMetricsUpdated RouteMetric
		totalWant        []*client_model.Metric
		validWant        []*client_model.Metric
		invalidWant      []*client_model.Metric
		orphanedWant     []*client_model.Metric
		rootWant         []*client_model.Metric
	}{
		"orphan is resolved": {
			irMetrics: RouteMetric{
				Total: map[Meta]int{
					{Namespace: "testns"}: 6,
					{Namespace: "foons"}:  3,
				},
				Valid: map[Meta]int{
					{Namespace: "testns", VHost: "foo.com"}: 3,
				},
				Invalid: map[Meta]int{
					{Namespace: "testns", VHost: "foo.com"}: 2,
				},
				Orphaned: map[Meta]int{
					{Namespace: "testns"}: 1,
				},
				Root: map[Meta]int{
					{Namespace: "testns"}: 4,
				},
			},
			irMetricsUpdated: RouteMetric{
				Total: map[Meta]int{
					{Namespace: "testns"}: 6,
					{Namespace: "foons"}:  3,
				},
				Valid: map[Meta]int{
					{Namespace: "testns", VHost: "foo.com"}: 3,
				},
				Invalid: map[Meta]int{
					{Namespace: "testns", VHost: "foo.com"}: 2,
				},
				Orphaned: map[Meta]int{},
				Root: map[Meta]int{
					{Namespace: "testns"}: 4,
				},
			},
			totalWant: []*client_model.Metric{
				{
					Label: []*client_model.LabelPair{{
						Name:  func() *string { i := "namespace"; return &i }(),
						Value: func() *string { i := "foons"; return &i }(),
					}},
					Gauge: &client_model.Gauge{
						Value: func() *float64 { i := float64(3); return &i }(),
					},
				},
				{
					Label: []*client_model.LabelPair{{
						Name:  func() *string { i := "namespace"; return &i }(),
						Value: func() *string { i := "testns"; return &i }(),
					}},
					Gauge: &client_model.Gauge{
						Value: func() *float64 { i := float64(6); return &i }(),
					},
				},
			},
			orphanedWant: []*client_model.Metric{},
			validWant: []*client_model.Metric{
				{
					Label: []*client_model.LabelPair{{
						Name:  func() *string { i := "namespace"; return &i }(),
						Value: func() *string { i := "testns"; return &i }(),
					}, {
						Name:  func() *string { i := "vhost"; return &i }(),
						Value: func() *string { i := "foo.com"; return &i }(),
					}},
					Gauge: &client_model.Gauge{
						Value: func() *float64 { i := float64(3); return &i }(),
					},
				},
			},
			invalidWant: []*client_model.Metric{
				{
					Label: []*client_model.LabelPair{{
						Name:  func() *string { i := "namespace"; return &i }(),
						Value: func() *string { i := "testns"; return &i }(),
					}, {
						Name:  func() *string { i := "vhost"; return &i }(),
						Value: func() *string { i := "foo.com"; return &i }(),
					}},
					Gauge: &client_model.Gauge{
						Value: func() *float64 { i := float64(2); return &i }(),
					},
				},
			},
			rootWant: []*client_model.Metric{
				{
					Label: []*client_model.LabelPair{{
						Name:  func() *string { i := "namespace"; return &i }(),
						Value: func() *string { i := "testns"; return &i }(),
					}},
					Gauge: &client_model.Gauge{
						Value: func() *float64 { i := float64(4); return &i }(),
					},
				},
			},
		},
		"root HTTPProxy is deleted": {
			irMetrics: RouteMetric{
				Total: map[Meta]int{
					{Namespace: "testns"}: 6,
					{Namespace: "foons"}:  3,
				},
				Valid: map[Meta]int{
					{Namespace: "testns", VHost: "foo.com"}: 3,
				},
				Invalid: map[Meta]int{
					{Namespace: "testns", VHost: "foo.com"}: 2,
				},
				Orphaned: map[Meta]int{
					{Namespace: "testns"}: 1,
				},
				Root: map[Meta]int{
					{Namespace: "testns"}: 4,
				},
			},
			irMetricsUpdated: RouteMetric{
				Total: map[Meta]int{
					{Namespace: "testns"}: 6,
				},
				Valid: map[Meta]int{
					{Namespace: "testns", VHost: "foo.com"}: 3,
				},
				Invalid: map[Meta]int{
					{Namespace: "testns", VHost: "foo.com"}: 2,
				},
				Orphaned: map[Meta]int{},
				Root: map[Meta]int{
					{Namespace: "testns"}: 4,
				},
			},
			totalWant: []*client_model.Metric{
				{
					Label: []*client_model.LabelPair{{
						Name:  func() *string { i := "namespace"; return &i }(),
						Value: func() *string { i := "testns"; return &i }(),
					}},
					Gauge: &client_model.Gauge{
						Value: func() *float64 { i := float64(6); return &i }(),
					},
				},
			},
			orphanedWant: []*client_model.Metric{},
			validWant: []*client_model.Metric{
				{
					Label: []*client_model.LabelPair{{
						Name:  func() *string { i := "namespace"; return &i }(),
						Value: func() *string { i := "testns"; return &i }(),
					}, {
						Name:  func() *string { i := "vhost"; return &i }(),
						Value: func() *string { i := "foo.com"; return &i }(),
					}},
					Gauge: &client_model.Gauge{
						Value: func() *float64 { i := float64(3); return &i }(),
					},
				},
			},
			invalidWant: []*client_model.Metric{
				{
					Label: []*client_model.LabelPair{{
						Name:  func() *string { i := "namespace"; return &i }(),
						Value: func() *string { i := "testns"; return &i }(),
					}, {
						Name:  func() *string { i := "vhost"; return &i }(),
						Value: func() *string { i := "foo.com"; return &i }(),
					}},
					Gauge: &client_model.Gauge{
						Value: func() *float64 { i := float64(2); return &i }(),
					},
				},
			},
			rootWant: []*client_model.Metric{
				{
					Label: []*client_model.LabelPair{{
						Name:  func() *string { i := "namespace"; return &i }(),
						Value: func() *string { i := "testns"; return &i }(),
					}},
					Gauge: &client_model.Gauge{
						Value: func() *float64 { i := float64(4); return &i }(),
					},
				},
			},
		},
		"valid is deleted from namespace": {
			irMetrics: RouteMetric{
				Total: map[Meta]int{
					{Namespace: "testns"}: 6,
					{Namespace: "foons"}:  3,
				},
				Valid: map[Meta]int{
					{Namespace: "testns", VHost: "foo.com"}: 3,
				},
				Invalid: map[Meta]int{
					{Namespace: "testns", VHost: "foo.com"}: 2,
				},
				Orphaned: map[Meta]int{
					{Namespace: "testns"}: 1,
				},
				Root: map[Meta]int{
					{Namespace: "testns"}: 4,
				},
			},
			irMetricsUpdated: RouteMetric{
				Total: map[Meta]int{
					{Namespace: "testns"}: 6,
				},
				Valid: map[Meta]int{
					{Namespace: "testns", VHost: "foo.com"}: 3,
				},
				Invalid: map[Meta]int{
					{Namespace: "testns", VHost: "foo.com"}: 2,
				},
				Orphaned: map[Meta]int{},
				Root: map[Meta]int{
					{Namespace: "testns"}: 4,
				},
			},
			totalWant: []*client_model.Metric{
				{
					Label: []*client_model.LabelPair{{
						Name:  func() *string { i := "namespace"; return &i }(),
						Value: func() *string { i := "testns"; return &i }(),
					}},
					Gauge: &client_model.Gauge{
						Value: func() *float64 { i := float64(6); return &i }(),
					},
				},
			},
			orphanedWant: []*client_model.Metric{},
			validWant: []*client_model.Metric{
				{
					Label: []*client_model.LabelPair{{
						Name:  func() *string { i := "namespace"; return &i }(),
						Value: func() *string { i := "testns"; return &i }(),
					}, {
						Name:  func() *string { i := "vhost"; return &i }(),
						Value: func() *string { i := "foo.com"; return &i }(),
					}},
					Gauge: &client_model.Gauge{
						Value: func() *float64 { i := float64(3); return &i }(),
					},
				},
			},
			invalidWant: []*client_model.Metric{
				{
					Label: []*client_model.LabelPair{{
						Name:  func() *string { i := "namespace"; return &i }(),
						Value: func() *string { i := "testns"; return &i }(),
					}, {
						Name:  func() *string { i := "vhost"; return &i }(),
						Value: func() *string { i := "foo.com"; return &i }(),
					}},
					Gauge: &client_model.Gauge{
						Value: func() *float64 { i := float64(2); return &i }(),
					},
				},
			},
			rootWant: []*client_model.Metric{
				{
					Label: []*client_model.LabelPair{{
						Name:  func() *string { i := "namespace"; return &i }(),
						Value: func() *string { i := "testns"; return &i }(),
					}},
					Gauge: &client_model.Gauge{
						Value: func() *float64 { i := float64(4); return &i }(),
					},
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			r := prometheus.NewRegistry()
			m := NewMetrics(r)
			m.SetHTTPProxyMetric(tc.irMetrics)

			gatherers := prometheus.Gatherers{
				r,
				prometheus.DefaultGatherer,
			}

			gathering, err := gatherers.Gather()
			if err != nil {
				t.Fatal(err)
			}

			gotTotal := []*client_model.Metric{}
			gotValid := []*client_model.Metric{}
			gotInvalid := []*client_model.Metric{}
			gotOrphaned := []*client_model.Metric{}
			gotRoot := []*client_model.Metric{}
			for _, mf := range gathering {
				switch mf.GetName() {
				case total.metric:
					gotTotal = mf.Metric
				case valid.metric:
					gotValid = mf.Metric
				case invalid.metric:
					gotInvalid = mf.Metric
				case orphaned.metric:
					gotOrphaned = mf.Metric
				case root.metric:
					gotRoot = mf.Metric
				}
			}

			assert.Equal(t, total.want, gotTotal)
			assert.Equal(t, valid.want, gotValid)
			assert.Equal(t, invalid.want, gotInvalid)
			assert.Equal(t, orphaned.want, gotOrphaned)
			assert.Equal(t, root.want, gotRoot)

			m.SetHTTPProxyMetric(tc.irMetricsUpdated)

			// Now validate that metrics got removed
			gatherers = prometheus.Gatherers{
				r,
				prometheus.DefaultGatherer,
			}

			gathering, err = gatherers.Gather()
			if err != nil {
				t.Fatal(err)
			}

			gotTotal = []*client_model.Metric{}
			gotValid = []*client_model.Metric{}
			gotInvalid = []*client_model.Metric{}
			gotOrphaned = []*client_model.Metric{}
			gotRoot = []*client_model.Metric{}
			for _, mf := range gathering {
				switch mf.GetName() {
				case total.metric:
					gotTotal = mf.Metric
				case valid.metric:
					gotValid = mf.Metric
				case invalid.metric:
					gotInvalid = mf.Metric
				case orphaned.metric:
					gotOrphaned = mf.Metric
				case root.metric:
					gotRoot = mf.Metric
				}
			}

			assert.Equal(t, tc.totalWant, gotTotal)
			assert.Equal(t, tc.validWant, gotValid)
			assert.Equal(t, tc.invalidWant, gotInvalid)
			assert.Equal(t, tc.orphanedWant, gotOrphaned)
			assert.Equal(t, tc.rootWant, gotRoot)
		})
	}
}
