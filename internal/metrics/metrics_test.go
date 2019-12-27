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

package metrics

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	io_prometheus_client "github.com/prometheus/client_model/go"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

type testMetric struct {
	metric string
	want   []*io_prometheus_client.Metric
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
				want: []*io_prometheus_client.Metric{
					{
						Gauge: &io_prometheus_client.Gauge{
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

			gotTimestamp := []*io_prometheus_client.Metric{}
			for _, mf := range gathering {
				if mf.GetName() == tc.timestampMetric.metric {
					gotTimestamp = mf.Metric
				}
			}

			if !reflect.DeepEqual(gotTimestamp, tc.timestampMetric.want) {
				t.Fatalf("write metric timestamp metric failed, want: %v got: %v", tc.timestampMetric.want, gotTimestamp)
			}
		})
	}
}

func TestWriteIngressRouteMetric(t *testing.T) {
	tests := map[string]struct {
		irMetrics RouteMetric
		total     testMetric
		valid     testMetric
		invalid   testMetric
		orphaned  testMetric
		root      testMetric
	}{
		"simple": {
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
			total: testMetric{
				metric: IngressRouteTotalGauge,
				want: []*io_prometheus_client.Metric{
					{
						Label: []*io_prometheus_client.LabelPair{{
							Name:  func() *string { i := "namespace"; return &i }(),
							Value: func() *string { i := "foons"; return &i }(),
						}},
						Gauge: &io_prometheus_client.Gauge{
							Value: func() *float64 { i := float64(3); return &i }(),
						},
					},
					{
						Label: []*io_prometheus_client.LabelPair{{
							Name:  func() *string { i := "namespace"; return &i }(),
							Value: func() *string { i := "testns"; return &i }(),
						}},
						Gauge: &io_prometheus_client.Gauge{
							Value: func() *float64 { i := float64(6); return &i }(),
						},
					},
				},
			},
			orphaned: testMetric{
				metric: IngressRouteOrphanedGauge,
				want: []*io_prometheus_client.Metric{
					{
						Label: []*io_prometheus_client.LabelPair{{
							Name:  func() *string { i := "namespace"; return &i }(),
							Value: func() *string { i := "testns"; return &i }(),
						}},
						Gauge: &io_prometheus_client.Gauge{
							Value: func() *float64 { i := float64(1); return &i }(),
						},
					},
				},
			},
			valid: testMetric{
				metric: IngressRouteValidGauge,
				want: []*io_prometheus_client.Metric{
					{
						Label: []*io_prometheus_client.LabelPair{{
							Name:  func() *string { i := "namespace"; return &i }(),
							Value: func() *string { i := "testns"; return &i }(),
						}, {
							Name:  func() *string { i := "vhost"; return &i }(),
							Value: func() *string { i := "foo.com"; return &i }(),
						}},
						Gauge: &io_prometheus_client.Gauge{
							Value: func() *float64 { i := float64(3); return &i }(),
						},
					},
				},
			},
			invalid: testMetric{
				metric: IngressRouteInvalidGauge,
				want: []*io_prometheus_client.Metric{
					{
						Label: []*io_prometheus_client.LabelPair{{
							Name:  func() *string { i := "namespace"; return &i }(),
							Value: func() *string { i := "testns"; return &i }(),
						}, {
							Name:  func() *string { i := "vhost"; return &i }(),
							Value: func() *string { i := "foo.com"; return &i }(),
						}},
						Gauge: &io_prometheus_client.Gauge{
							Value: func() *float64 { i := float64(2); return &i }(),
						},
					},
				},
			},
			root: testMetric{
				metric: IngressRouteRootTotalGauge,
				want: []*io_prometheus_client.Metric{
					{
						Label: []*io_prometheus_client.LabelPair{{
							Name:  func() *string { i := "namespace"; return &i }(),
							Value: func() *string { i := "testns"; return &i }(),
						}},
						Gauge: &io_prometheus_client.Gauge{
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
			m.SetIngressRouteMetric(tc.irMetrics)

			gatherers := prometheus.Gatherers{
				r,
				prometheus.DefaultGatherer,
			}

			gathering, err := gatherers.Gather()
			if err != nil {
				t.Fatal(err)
			}

			gotTotal := []*io_prometheus_client.Metric{}
			gotValid := []*io_prometheus_client.Metric{}
			gotInvalid := []*io_prometheus_client.Metric{}
			gotOrphaned := []*io_prometheus_client.Metric{}
			gotRoot := []*io_prometheus_client.Metric{}
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

			if !reflect.DeepEqual(gotTotal, tc.total.want) {
				t.Fatalf("write metric total metric failed, want: %v got: %v", tc.total.want, gotTotal)
			}
			if !reflect.DeepEqual(gotValid, tc.valid.want) {
				t.Fatalf("write metric valid metric failed, want: %v got: %v", tc.valid.want, gotValid)
			}
			if !reflect.DeepEqual(gotInvalid, tc.invalid.want) {
				t.Fatalf("write metric invalid metric failed, want: %v got: %v", tc.invalid.want, gotInvalid)
			}
			if !reflect.DeepEqual(gotOrphaned, tc.orphaned.want) {
				t.Fatalf("write metric orphaned metric failed, want: %v got: %v", tc.orphaned.want, gotOrphaned)
			}
			if !reflect.DeepEqual(gotRoot, tc.root.want) {
				t.Fatalf("write metric orphaned metric failed, want: %v got: %v", tc.root.want, gotRoot)
			}
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
				want: []*io_prometheus_client.Metric{
					{
						Label: []*io_prometheus_client.LabelPair{{
							Name:  func() *string { i := "namespace"; return &i }(),
							Value: func() *string { i := "foons"; return &i }(),
						}},
						Gauge: &io_prometheus_client.Gauge{
							Value: func() *float64 { i := float64(3); return &i }(),
						},
					},
					{
						Label: []*io_prometheus_client.LabelPair{{
							Name:  func() *string { i := "namespace"; return &i }(),
							Value: func() *string { i := "testns"; return &i }(),
						}},
						Gauge: &io_prometheus_client.Gauge{
							Value: func() *float64 { i := float64(6); return &i }(),
						},
					},
				},
			},
			orphaned: testMetric{
				metric: HTTPProxyOrphanedGauge,
				want: []*io_prometheus_client.Metric{
					{
						Label: []*io_prometheus_client.LabelPair{{
							Name:  func() *string { i := "namespace"; return &i }(),
							Value: func() *string { i := "testns"; return &i }(),
						}},
						Gauge: &io_prometheus_client.Gauge{
							Value: func() *float64 { i := float64(1); return &i }(),
						},
					},
				},
			},
			valid: testMetric{
				metric: HTTPProxyValidGauge,
				want: []*io_prometheus_client.Metric{
					{
						Label: []*io_prometheus_client.LabelPair{{
							Name:  func() *string { i := "namespace"; return &i }(),
							Value: func() *string { i := "testns"; return &i }(),
						}, {
							Name:  func() *string { i := "vhost"; return &i }(),
							Value: func() *string { i := "foo.com"; return &i }(),
						}},
						Gauge: &io_prometheus_client.Gauge{
							Value: func() *float64 { i := float64(3); return &i }(),
						},
					},
				},
			},
			invalid: testMetric{
				metric: HTTPProxyInvalidGauge,
				want: []*io_prometheus_client.Metric{
					{
						Label: []*io_prometheus_client.LabelPair{{
							Name:  func() *string { i := "namespace"; return &i }(),
							Value: func() *string { i := "testns"; return &i }(),
						}, {
							Name:  func() *string { i := "vhost"; return &i }(),
							Value: func() *string { i := "foo.com"; return &i }(),
						}},
						Gauge: &io_prometheus_client.Gauge{
							Value: func() *float64 { i := float64(2); return &i }(),
						},
					},
				},
			},
			root: testMetric{
				metric: HTTPProxyRootTotalGauge,
				want: []*io_prometheus_client.Metric{
					{
						Label: []*io_prometheus_client.LabelPair{{
							Name:  func() *string { i := "namespace"; return &i }(),
							Value: func() *string { i := "testns"; return &i }(),
						}},
						Gauge: &io_prometheus_client.Gauge{
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

			gotTotal := []*io_prometheus_client.Metric{}
			gotValid := []*io_prometheus_client.Metric{}
			gotInvalid := []*io_prometheus_client.Metric{}
			gotOrphaned := []*io_prometheus_client.Metric{}
			gotRoot := []*io_prometheus_client.Metric{}
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

			if !reflect.DeepEqual(gotTotal, tc.total.want) {
				t.Fatalf("write metric total metric failed, want: %v got: %v", tc.total.want, gotTotal)
			}
			if !reflect.DeepEqual(gotValid, tc.valid.want) {
				t.Fatalf("write metric valid metric failed, want: %v got: %v", tc.valid.want, gotValid)
			}
			if !reflect.DeepEqual(gotInvalid, tc.invalid.want) {
				t.Fatalf("write metric invalid metric failed, want: %v got: %v", tc.invalid.want, gotInvalid)
			}
			if !reflect.DeepEqual(gotOrphaned, tc.orphaned.want) {
				t.Fatalf("write metric orphaned metric failed, want: %v got: %v", tc.orphaned.want, gotOrphaned)
			}
			if !reflect.DeepEqual(gotRoot, tc.root.want) {
				t.Fatalf("write metric orphaned metric failed, want: %v got: %v", tc.root.want, gotRoot)
			}
		})
	}
}

func TestRemoveMetric(t *testing.T) {
	total := testMetric{
		metric: IngressRouteTotalGauge,
		want: []*io_prometheus_client.Metric{
			{
				Label: []*io_prometheus_client.LabelPair{{
					Name:  func() *string { i := "namespace"; return &i }(),
					Value: func() *string { i := "foons"; return &i }(),
				}},
				Gauge: &io_prometheus_client.Gauge{
					Value: func() *float64 { i := float64(3); return &i }(),
				},
			},
			{
				Label: []*io_prometheus_client.LabelPair{{
					Name:  func() *string { i := "namespace"; return &i }(),
					Value: func() *string { i := "testns"; return &i }(),
				}},
				Gauge: &io_prometheus_client.Gauge{
					Value: func() *float64 { i := float64(6); return &i }(),
				},
			},
		},
	}

	orphaned := testMetric{
		metric: IngressRouteOrphanedGauge,
		want: []*io_prometheus_client.Metric{
			{
				Label: []*io_prometheus_client.LabelPair{{
					Name:  func() *string { i := "namespace"; return &i }(),
					Value: func() *string { i := "testns"; return &i }(),
				}},
				Gauge: &io_prometheus_client.Gauge{
					Value: func() *float64 { i := float64(1); return &i }(),
				},
			},
		},
	}

	valid := testMetric{
		metric: IngressRouteValidGauge,
		want: []*io_prometheus_client.Metric{
			{
				Label: []*io_prometheus_client.LabelPair{{
					Name:  func() *string { i := "namespace"; return &i }(),
					Value: func() *string { i := "testns"; return &i }(),
				}, {
					Name:  func() *string { i := "vhost"; return &i }(),
					Value: func() *string { i := "foo.com"; return &i }(),
				}},
				Gauge: &io_prometheus_client.Gauge{
					Value: func() *float64 { i := float64(3); return &i }(),
				},
			},
		},
	}

	invalid := testMetric{
		metric: IngressRouteInvalidGauge,
		want: []*io_prometheus_client.Metric{
			{
				Label: []*io_prometheus_client.LabelPair{{
					Name:  func() *string { i := "namespace"; return &i }(),
					Value: func() *string { i := "testns"; return &i }(),
				}, {
					Name:  func() *string { i := "vhost"; return &i }(),
					Value: func() *string { i := "foo.com"; return &i }(),
				}},
				Gauge: &io_prometheus_client.Gauge{
					Value: func() *float64 { i := float64(2); return &i }(),
				},
			},
		},
	}

	root := testMetric{
		metric: IngressRouteRootTotalGauge,
		want: []*io_prometheus_client.Metric{
			{
				Label: []*io_prometheus_client.LabelPair{{
					Name:  func() *string { i := "namespace"; return &i }(),
					Value: func() *string { i := "testns"; return &i }(),
				}},
				Gauge: &io_prometheus_client.Gauge{
					Value: func() *float64 { i := float64(4); return &i }(),
				},
			},
		},
	}

	tests := map[string]struct {
		irMetrics        RouteMetric
		irMetricsUpdated RouteMetric
		totalWant        []*io_prometheus_client.Metric
		validWant        []*io_prometheus_client.Metric
		invalidWant      []*io_prometheus_client.Metric
		orphanedWant     []*io_prometheus_client.Metric
		rootWant         []*io_prometheus_client.Metric
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
			totalWant: []*io_prometheus_client.Metric{
				{
					Label: []*io_prometheus_client.LabelPair{{
						Name:  func() *string { i := "namespace"; return &i }(),
						Value: func() *string { i := "foons"; return &i }(),
					}},
					Gauge: &io_prometheus_client.Gauge{
						Value: func() *float64 { i := float64(3); return &i }(),
					},
				},
				{
					Label: []*io_prometheus_client.LabelPair{{
						Name:  func() *string { i := "namespace"; return &i }(),
						Value: func() *string { i := "testns"; return &i }(),
					}},
					Gauge: &io_prometheus_client.Gauge{
						Value: func() *float64 { i := float64(6); return &i }(),
					},
				},
			},
			orphanedWant: []*io_prometheus_client.Metric{},
			validWant: []*io_prometheus_client.Metric{
				{
					Label: []*io_prometheus_client.LabelPair{{
						Name:  func() *string { i := "namespace"; return &i }(),
						Value: func() *string { i := "testns"; return &i }(),
					}, {
						Name:  func() *string { i := "vhost"; return &i }(),
						Value: func() *string { i := "foo.com"; return &i }(),
					}},
					Gauge: &io_prometheus_client.Gauge{
						Value: func() *float64 { i := float64(3); return &i }(),
					},
				},
			},
			invalidWant: []*io_prometheus_client.Metric{
				{
					Label: []*io_prometheus_client.LabelPair{{
						Name:  func() *string { i := "namespace"; return &i }(),
						Value: func() *string { i := "testns"; return &i }(),
					}, {
						Name:  func() *string { i := "vhost"; return &i }(),
						Value: func() *string { i := "foo.com"; return &i }(),
					}},
					Gauge: &io_prometheus_client.Gauge{
						Value: func() *float64 { i := float64(2); return &i }(),
					},
				},
			},
			rootWant: []*io_prometheus_client.Metric{
				{
					Label: []*io_prometheus_client.LabelPair{{
						Name:  func() *string { i := "namespace"; return &i }(),
						Value: func() *string { i := "testns"; return &i }(),
					}},
					Gauge: &io_prometheus_client.Gauge{
						Value: func() *float64 { i := float64(4); return &i }(),
					},
				},
			},
		},
		"root IngressRoute is deleted": {
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
			totalWant: []*io_prometheus_client.Metric{
				{
					Label: []*io_prometheus_client.LabelPair{{
						Name:  func() *string { i := "namespace"; return &i }(),
						Value: func() *string { i := "testns"; return &i }(),
					}},
					Gauge: &io_prometheus_client.Gauge{
						Value: func() *float64 { i := float64(6); return &i }(),
					},
				},
			},
			orphanedWant: []*io_prometheus_client.Metric{},
			validWant: []*io_prometheus_client.Metric{
				{
					Label: []*io_prometheus_client.LabelPair{{
						Name:  func() *string { i := "namespace"; return &i }(),
						Value: func() *string { i := "testns"; return &i }(),
					}, {
						Name:  func() *string { i := "vhost"; return &i }(),
						Value: func() *string { i := "foo.com"; return &i }(),
					}},
					Gauge: &io_prometheus_client.Gauge{
						Value: func() *float64 { i := float64(3); return &i }(),
					},
				},
			},
			invalidWant: []*io_prometheus_client.Metric{
				{
					Label: []*io_prometheus_client.LabelPair{{
						Name:  func() *string { i := "namespace"; return &i }(),
						Value: func() *string { i := "testns"; return &i }(),
					}, {
						Name:  func() *string { i := "vhost"; return &i }(),
						Value: func() *string { i := "foo.com"; return &i }(),
					}},
					Gauge: &io_prometheus_client.Gauge{
						Value: func() *float64 { i := float64(2); return &i }(),
					},
				},
			},
			rootWant: []*io_prometheus_client.Metric{
				{
					Label: []*io_prometheus_client.LabelPair{{
						Name:  func() *string { i := "namespace"; return &i }(),
						Value: func() *string { i := "testns"; return &i }(),
					}},
					Gauge: &io_prometheus_client.Gauge{
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
			totalWant: []*io_prometheus_client.Metric{
				{
					Label: []*io_prometheus_client.LabelPair{{
						Name:  func() *string { i := "namespace"; return &i }(),
						Value: func() *string { i := "testns"; return &i }(),
					}},
					Gauge: &io_prometheus_client.Gauge{
						Value: func() *float64 { i := float64(6); return &i }(),
					},
				},
			},
			orphanedWant: []*io_prometheus_client.Metric{},
			validWant: []*io_prometheus_client.Metric{
				{
					Label: []*io_prometheus_client.LabelPair{{
						Name:  func() *string { i := "namespace"; return &i }(),
						Value: func() *string { i := "testns"; return &i }(),
					}, {
						Name:  func() *string { i := "vhost"; return &i }(),
						Value: func() *string { i := "foo.com"; return &i }(),
					}},
					Gauge: &io_prometheus_client.Gauge{
						Value: func() *float64 { i := float64(3); return &i }(),
					},
				},
			},
			invalidWant: []*io_prometheus_client.Metric{
				{
					Label: []*io_prometheus_client.LabelPair{{
						Name:  func() *string { i := "namespace"; return &i }(),
						Value: func() *string { i := "testns"; return &i }(),
					}, {
						Name:  func() *string { i := "vhost"; return &i }(),
						Value: func() *string { i := "foo.com"; return &i }(),
					}},
					Gauge: &io_prometheus_client.Gauge{
						Value: func() *float64 { i := float64(2); return &i }(),
					},
				},
			},
			rootWant: []*io_prometheus_client.Metric{
				{
					Label: []*io_prometheus_client.LabelPair{{
						Name:  func() *string { i := "namespace"; return &i }(),
						Value: func() *string { i := "testns"; return &i }(),
					}},
					Gauge: &io_prometheus_client.Gauge{
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
			m.SetIngressRouteMetric(tc.irMetrics)

			gatherers := prometheus.Gatherers{
				r,
				prometheus.DefaultGatherer,
			}

			gathering, err := gatherers.Gather()
			if err != nil {
				t.Fatal(err)
			}

			gotTotal := []*io_prometheus_client.Metric{}
			gotValid := []*io_prometheus_client.Metric{}
			gotInvalid := []*io_prometheus_client.Metric{}
			gotOrphaned := []*io_prometheus_client.Metric{}
			gotRoot := []*io_prometheus_client.Metric{}
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

			if !reflect.DeepEqual(gotTotal, total.want) {
				t.Fatalf("write metric total metric failed, want: %v got: %v", total.want, gotTotal)
			}
			if !reflect.DeepEqual(gotValid, valid.want) {
				t.Fatalf("write metric valid metric failed, want: %v got: %v", valid.want, gotValid)
			}
			if !reflect.DeepEqual(gotInvalid, invalid.want) {
				t.Fatalf("write metric invalid metric failed, want: %v got: %v", invalid.want, gotInvalid)
			}
			if !reflect.DeepEqual(gotOrphaned, orphaned.want) {
				t.Fatalf("write metric orphaned metric failed, want: %v got: %v", orphaned.want, gotOrphaned)
			}
			if !reflect.DeepEqual(gotRoot, root.want) {
				t.Fatalf("write metric orphaned metric failed, want: %v got: %v", root.want, gotRoot)
			}

			m.SetIngressRouteMetric(tc.irMetricsUpdated)

			// Now validate that metrics got removed
			gatherers = prometheus.Gatherers{
				r,
				prometheus.DefaultGatherer,
			}

			gathering, err = gatherers.Gather()
			if err != nil {
				t.Fatal(err)
			}

			gotTotal = []*io_prometheus_client.Metric{}
			gotValid = []*io_prometheus_client.Metric{}
			gotInvalid = []*io_prometheus_client.Metric{}
			gotOrphaned = []*io_prometheus_client.Metric{}
			gotRoot = []*io_prometheus_client.Metric{}
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

			if !reflect.DeepEqual(gotTotal, tc.totalWant) {
				t.Fatalf("write metric total metric failed, want: %v got: %v", tc.totalWant, gotTotal)
			}
			if !reflect.DeepEqual(gotValid, tc.validWant) {
				t.Fatalf("write metric valid metric failed, want: %v got: %v", tc.validWant, gotValid)
			}
			if !reflect.DeepEqual(gotInvalid, tc.invalidWant) {
				t.Fatalf("write metric invalid metric failed, want: %v got: %v", tc.invalidWant, gotInvalid)
			}
			if !reflect.DeepEqual(gotOrphaned, tc.orphanedWant) {
				t.Fatalf("write metric orphaned metric failed, want: %v got: %v", tc.orphanedWant, gotOrphaned)
			}
			if !reflect.DeepEqual(gotRoot, tc.rootWant) {
				t.Fatalf("write metric orphaned metric failed, want: %v got: %v", tc.rootWant, gotRoot)
			}
		})
	}
}

func TestRemoveProxyMetric(t *testing.T) {
	total := testMetric{
		metric: HTTPProxyTotalGauge,
		want: []*io_prometheus_client.Metric{
			{
				Label: []*io_prometheus_client.LabelPair{{
					Name:  func() *string { i := "namespace"; return &i }(),
					Value: func() *string { i := "foons"; return &i }(),
				}},
				Gauge: &io_prometheus_client.Gauge{
					Value: func() *float64 { i := float64(3); return &i }(),
				},
			},
			{
				Label: []*io_prometheus_client.LabelPair{{
					Name:  func() *string { i := "namespace"; return &i }(),
					Value: func() *string { i := "testns"; return &i }(),
				}},
				Gauge: &io_prometheus_client.Gauge{
					Value: func() *float64 { i := float64(6); return &i }(),
				},
			},
		},
	}

	orphaned := testMetric{
		metric: HTTPProxyOrphanedGauge,
		want: []*io_prometheus_client.Metric{
			{
				Label: []*io_prometheus_client.LabelPair{{
					Name:  func() *string { i := "namespace"; return &i }(),
					Value: func() *string { i := "testns"; return &i }(),
				}},
				Gauge: &io_prometheus_client.Gauge{
					Value: func() *float64 { i := float64(1); return &i }(),
				},
			},
		},
	}

	valid := testMetric{
		metric: HTTPProxyValidGauge,
		want: []*io_prometheus_client.Metric{
			{
				Label: []*io_prometheus_client.LabelPair{{
					Name:  func() *string { i := "namespace"; return &i }(),
					Value: func() *string { i := "testns"; return &i }(),
				}, {
					Name:  func() *string { i := "vhost"; return &i }(),
					Value: func() *string { i := "foo.com"; return &i }(),
				}},
				Gauge: &io_prometheus_client.Gauge{
					Value: func() *float64 { i := float64(3); return &i }(),
				},
			},
		},
	}

	invalid := testMetric{
		metric: HTTPProxyInvalidGauge,
		want: []*io_prometheus_client.Metric{
			{
				Label: []*io_prometheus_client.LabelPair{{
					Name:  func() *string { i := "namespace"; return &i }(),
					Value: func() *string { i := "testns"; return &i }(),
				}, {
					Name:  func() *string { i := "vhost"; return &i }(),
					Value: func() *string { i := "foo.com"; return &i }(),
				}},
				Gauge: &io_prometheus_client.Gauge{
					Value: func() *float64 { i := float64(2); return &i }(),
				},
			},
		},
	}

	root := testMetric{
		metric: HTTPProxyRootTotalGauge,
		want: []*io_prometheus_client.Metric{
			{
				Label: []*io_prometheus_client.LabelPair{{
					Name:  func() *string { i := "namespace"; return &i }(),
					Value: func() *string { i := "testns"; return &i }(),
				}},
				Gauge: &io_prometheus_client.Gauge{
					Value: func() *float64 { i := float64(4); return &i }(),
				},
			},
		},
	}

	tests := map[string]struct {
		irMetrics        RouteMetric
		irMetricsUpdated RouteMetric
		totalWant        []*io_prometheus_client.Metric
		validWant        []*io_prometheus_client.Metric
		invalidWant      []*io_prometheus_client.Metric
		orphanedWant     []*io_prometheus_client.Metric
		rootWant         []*io_prometheus_client.Metric
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
			totalWant: []*io_prometheus_client.Metric{
				{
					Label: []*io_prometheus_client.LabelPair{{
						Name:  func() *string { i := "namespace"; return &i }(),
						Value: func() *string { i := "foons"; return &i }(),
					}},
					Gauge: &io_prometheus_client.Gauge{
						Value: func() *float64 { i := float64(3); return &i }(),
					},
				},
				{
					Label: []*io_prometheus_client.LabelPair{{
						Name:  func() *string { i := "namespace"; return &i }(),
						Value: func() *string { i := "testns"; return &i }(),
					}},
					Gauge: &io_prometheus_client.Gauge{
						Value: func() *float64 { i := float64(6); return &i }(),
					},
				},
			},
			orphanedWant: []*io_prometheus_client.Metric{},
			validWant: []*io_prometheus_client.Metric{
				{
					Label: []*io_prometheus_client.LabelPair{{
						Name:  func() *string { i := "namespace"; return &i }(),
						Value: func() *string { i := "testns"; return &i }(),
					}, {
						Name:  func() *string { i := "vhost"; return &i }(),
						Value: func() *string { i := "foo.com"; return &i }(),
					}},
					Gauge: &io_prometheus_client.Gauge{
						Value: func() *float64 { i := float64(3); return &i }(),
					},
				},
			},
			invalidWant: []*io_prometheus_client.Metric{
				{
					Label: []*io_prometheus_client.LabelPair{{
						Name:  func() *string { i := "namespace"; return &i }(),
						Value: func() *string { i := "testns"; return &i }(),
					}, {
						Name:  func() *string { i := "vhost"; return &i }(),
						Value: func() *string { i := "foo.com"; return &i }(),
					}},
					Gauge: &io_prometheus_client.Gauge{
						Value: func() *float64 { i := float64(2); return &i }(),
					},
				},
			},
			rootWant: []*io_prometheus_client.Metric{
				{
					Label: []*io_prometheus_client.LabelPair{{
						Name:  func() *string { i := "namespace"; return &i }(),
						Value: func() *string { i := "testns"; return &i }(),
					}},
					Gauge: &io_prometheus_client.Gauge{
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
			totalWant: []*io_prometheus_client.Metric{
				{
					Label: []*io_prometheus_client.LabelPair{{
						Name:  func() *string { i := "namespace"; return &i }(),
						Value: func() *string { i := "testns"; return &i }(),
					}},
					Gauge: &io_prometheus_client.Gauge{
						Value: func() *float64 { i := float64(6); return &i }(),
					},
				},
			},
			orphanedWant: []*io_prometheus_client.Metric{},
			validWant: []*io_prometheus_client.Metric{
				{
					Label: []*io_prometheus_client.LabelPair{{
						Name:  func() *string { i := "namespace"; return &i }(),
						Value: func() *string { i := "testns"; return &i }(),
					}, {
						Name:  func() *string { i := "vhost"; return &i }(),
						Value: func() *string { i := "foo.com"; return &i }(),
					}},
					Gauge: &io_prometheus_client.Gauge{
						Value: func() *float64 { i := float64(3); return &i }(),
					},
				},
			},
			invalidWant: []*io_prometheus_client.Metric{
				{
					Label: []*io_prometheus_client.LabelPair{{
						Name:  func() *string { i := "namespace"; return &i }(),
						Value: func() *string { i := "testns"; return &i }(),
					}, {
						Name:  func() *string { i := "vhost"; return &i }(),
						Value: func() *string { i := "foo.com"; return &i }(),
					}},
					Gauge: &io_prometheus_client.Gauge{
						Value: func() *float64 { i := float64(2); return &i }(),
					},
				},
			},
			rootWant: []*io_prometheus_client.Metric{
				{
					Label: []*io_prometheus_client.LabelPair{{
						Name:  func() *string { i := "namespace"; return &i }(),
						Value: func() *string { i := "testns"; return &i }(),
					}},
					Gauge: &io_prometheus_client.Gauge{
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
			totalWant: []*io_prometheus_client.Metric{
				{
					Label: []*io_prometheus_client.LabelPair{{
						Name:  func() *string { i := "namespace"; return &i }(),
						Value: func() *string { i := "testns"; return &i }(),
					}},
					Gauge: &io_prometheus_client.Gauge{
						Value: func() *float64 { i := float64(6); return &i }(),
					},
				},
			},
			orphanedWant: []*io_prometheus_client.Metric{},
			validWant: []*io_prometheus_client.Metric{
				{
					Label: []*io_prometheus_client.LabelPair{{
						Name:  func() *string { i := "namespace"; return &i }(),
						Value: func() *string { i := "testns"; return &i }(),
					}, {
						Name:  func() *string { i := "vhost"; return &i }(),
						Value: func() *string { i := "foo.com"; return &i }(),
					}},
					Gauge: &io_prometheus_client.Gauge{
						Value: func() *float64 { i := float64(3); return &i }(),
					},
				},
			},
			invalidWant: []*io_prometheus_client.Metric{
				{
					Label: []*io_prometheus_client.LabelPair{{
						Name:  func() *string { i := "namespace"; return &i }(),
						Value: func() *string { i := "testns"; return &i }(),
					}, {
						Name:  func() *string { i := "vhost"; return &i }(),
						Value: func() *string { i := "foo.com"; return &i }(),
					}},
					Gauge: &io_prometheus_client.Gauge{
						Value: func() *float64 { i := float64(2); return &i }(),
					},
				},
			},
			rootWant: []*io_prometheus_client.Metric{
				{
					Label: []*io_prometheus_client.LabelPair{{
						Name:  func() *string { i := "namespace"; return &i }(),
						Value: func() *string { i := "testns"; return &i }(),
					}},
					Gauge: &io_prometheus_client.Gauge{
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

			gotTotal := []*io_prometheus_client.Metric{}
			gotValid := []*io_prometheus_client.Metric{}
			gotInvalid := []*io_prometheus_client.Metric{}
			gotOrphaned := []*io_prometheus_client.Metric{}
			gotRoot := []*io_prometheus_client.Metric{}
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

			if !reflect.DeepEqual(gotTotal, total.want) {
				t.Fatalf("write metric total metric failed, want: %v got: %v", total.want, gotTotal)
			}
			if !reflect.DeepEqual(gotValid, valid.want) {
				t.Fatalf("write metric valid metric failed, want: %v got: %v", valid.want, gotValid)
			}
			if !reflect.DeepEqual(gotInvalid, invalid.want) {
				t.Fatalf("write metric invalid metric failed, want: %v got: %v", invalid.want, gotInvalid)
			}
			if !reflect.DeepEqual(gotOrphaned, orphaned.want) {
				t.Fatalf("write metric orphaned metric failed, want: %v got: %v", orphaned.want, gotOrphaned)
			}
			if !reflect.DeepEqual(gotRoot, root.want) {
				t.Fatalf("write metric orphaned metric failed, want: %v got: %v", root.want, gotRoot)
			}

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

			gotTotal = []*io_prometheus_client.Metric{}
			gotValid = []*io_prometheus_client.Metric{}
			gotInvalid = []*io_prometheus_client.Metric{}
			gotOrphaned = []*io_prometheus_client.Metric{}
			gotRoot = []*io_prometheus_client.Metric{}
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

			if !reflect.DeepEqual(gotTotal, tc.totalWant) {
				t.Fatalf("write metric total metric failed, want: %v got: %v", tc.totalWant, gotTotal)
			}
			if !reflect.DeepEqual(gotValid, tc.validWant) {
				t.Fatalf("write metric valid metric failed, want: %v got: %v", tc.validWant, gotValid)
			}
			if !reflect.DeepEqual(gotInvalid, tc.invalidWant) {
				t.Fatalf("write metric invalid metric failed, want: %v got: %v", tc.invalidWant, gotInvalid)
			}
			if !reflect.DeepEqual(gotOrphaned, tc.orphanedWant) {
				t.Fatalf("write metric orphaned metric failed, want: %v got: %v", tc.orphanedWant, gotOrphaned)
			}
			if !reflect.DeepEqual(gotRoot, tc.rootWant) {
				t.Fatalf("write metric orphaned metric failed, want: %v got: %v", tc.rootWant, gotRoot)
			}
		})
	}
}

func TestHealthzMetrics(t *testing.T) {
	val0 := func() *float64 { i := float64(0); return &i }
	val1 := func() *float64 { i := float64(1); return &i }

	tests := map[string]struct {
		client       func() kubernetes.Interface
		statusCode   int
		total        testMetric
		successful   testMetric
		unsuccessful testMetric
	}{
		// TODO: implement unsuccessful: I don't think we really want to implement the entire
		// kubernetes.Clientset interface but I don't know another way to do this (at the moment).
		"successful": {
			client: func() kubernetes.Interface {
				return fake.NewSimpleClientset()
			},
			statusCode: http.StatusOK,
			total: testMetric{
				metric: healthzHitsGauge,
				want: []*io_prometheus_client.Metric{
					{
						Label: []*io_prometheus_client.LabelPair{},
						Gauge: &io_prometheus_client.Gauge{
							Value: val1(),
						},
					},
				},
			},
			successful: testMetric{
				metric: healthzSuccessfulGauge,
				want: []*io_prometheus_client.Metric{
					{
						Label: []*io_prometheus_client.LabelPair{},
						Gauge: &io_prometheus_client.Gauge{
							Value: val1(),
						},
					},
				},
			},
			unsuccessful: testMetric{
				metric: healthzUnsuccessfulGauge,
				want: []*io_prometheus_client.Metric{
					{
						Label: []*io_prometheus_client.LabelPair{},
						Gauge: &io_prometheus_client.Gauge{
							Value: val0(),
						},
					},
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			r := prometheus.NewRegistry()
			mux := http.NewServeMux()

			registerHealthCheck(mux, tc.client(), r)

			ts := httptest.NewServer(mux)
			defer ts.Close()

			// health and healthz use the same handler so we really only need to hit the one.
			res, err := http.Get(ts.URL + "/healthz")
			if err != nil {
				t.Fatal(err)
			}

			if res.StatusCode != tc.statusCode {
				t.Fatalf("expected status code, want: %d, got: %d", tc.statusCode, res.StatusCode)
			}

			gatherers := prometheus.Gatherers{
				r,
				prometheus.DefaultGatherer,
			}

			gathering, err := gatherers.Gather()
			if err != nil {
				t.Fatal(err)
			}

			total := []*io_prometheus_client.Metric{}
			successful := []*io_prometheus_client.Metric{}
			unsuccessful := []*io_prometheus_client.Metric{}
			for _, mf := range gathering {
				switch mf.GetName() {
				case tc.total.metric:
					total = mf.Metric
				case tc.successful.metric:
					successful = mf.Metric
				case tc.unsuccessful.metric:
					unsuccessful = mf.Metric
				}
			}

			if !reflect.DeepEqual(total, tc.total.want) {
				t.Fatalf("healthz metrics total metric failed, want: %v got: %v", tc.total.want, total)
			}
			if !reflect.DeepEqual(successful, tc.successful.want) {
				t.Fatalf("healthz metrics successful failed, want: %v got: %v", tc.successful.want, successful)
			}
			if !reflect.DeepEqual(unsuccessful, tc.unsuccessful.want) {
				t.Fatalf("healthz metrics unsuccessful failed, want %v got: %v", tc.unsuccessful.want, unsuccessful)
			}
		})
	}
}
