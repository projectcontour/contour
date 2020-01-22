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
	"strings"
	"testing"

	"github.com/projectcontour/contour/internal/assert"
)

func TestParseOpenConnections(t *testing.T) {
	type testcase struct {
		stats            io.Reader
		prometheusStat   string
		prometheusValues []string
		wantConnections  int
		wantError        error
	}

	run := func(t *testing.T, name string, tc testcase) {
		t.Helper()

		t.Run(name, func(t *testing.T) {
			t.Helper()

			gotConnections, gotError := ParseOpenConnections(tc.stats, tc.prometheusStat, tc.prometheusValues)
			assert.Equal(t, tc.wantError, gotError)
			assert.Equal(t, tc.wantConnections, gotConnections)
		})
	}

	run(t, "nil stats", testcase{
		stats:            nil,
		prometheusStat:   "envoy_http_downstream_cx_active",
		prometheusValues: []string{"ingress_http", "ingress_https"},
		wantConnections:  -1,
		wantError:        fmt.Errorf("stats input was nil"),
	})

	run(t, "basic http only", testcase{
		stats:            strings.NewReader(VALIDHTTP),
		prometheusStat:   "envoy_http_downstream_cx_active",
		prometheusValues: []string{"ingress_http", "ingress_https"},
		wantConnections:  4,
		wantError:        nil,
	})

	run(t, "basic https only", testcase{
		stats:            strings.NewReader(VALIDHTTPS),
		prometheusStat:   "envoy_http_downstream_cx_active",
		prometheusValues: []string{"ingress_http", "ingress_https"},
		wantConnections:  4,
		wantError:        nil,
	})

	run(t, "basic both protocols", testcase{
		stats:            strings.NewReader(VALIDBOTH),
		prometheusStat:   "envoy_http_downstream_cx_active",
		prometheusValues: []string{"ingress_http", "ingress_https"},
		wantConnections:  8,
		wantError:        nil,
	})

	run(t, "missing values", testcase{
		stats:            strings.NewReader(MISSING_STATS),
		prometheusStat:   "envoy_http_downstream_cx_active",
		prometheusValues: []string{"ingress_http", "ingress_https"},
		wantConnections:  -1,
		wantError:        fmt.Errorf("prometheus stat [envoy_http_downstream_cx_active] not found in request result"),
	})

	run(t, "invalid stats", testcase{
		stats:            strings.NewReader("!!##$$##!!"),
		prometheusStat:   "envoy_http_downstream_cx_active",
		prometheusValues: []string{"ingress_http", "ingress_https"},
		wantConnections:  -1,
		wantError:        fmt.Errorf("parsing prometheus text format failed: text format parsing error in line 1: invalid metric name"),
	})
}

const (
	VALIDHTTP = `envoy_cluster_circuit_breakers_default_cx_pool_open{envoy_cluster_name="projectcontour_service-stats_9001"} 0
envoy_cluster_max_host_weight{envoy_cluster_name="projectcontour_service-stats_9001"} 0
envoy_cluster_upstream_rq_pending_active{envoy_cluster_name="projectcontour_service-stats_9001"} 0
envoy_cluster_circuit_breakers_high_rq_retry_open{envoy_cluster_name="projectcontour_service-stats_9001"} 0
envoy_cluster_circuit_breakers_high_cx_pool_open{envoy_cluster_name="projectcontour_service-stats_9001"} 0
envoy_cluster_upstream_cx_tx_bytes_buffered{envoy_cluster_name="projectcontour_service-stats_9001"} 0
envoy_cluster_version{envoy_cluster_name="projectcontour_service-stats_9001"} 0
envoy_cluster_circuit_breakers_default_cx_open{envoy_cluster_name="projectcontour_service-stats_9001"} 0
# TYPE envoy_http_downstream_cx_ssl_active gauge
envoy_http_downstream_cx_ssl_active{envoy_http_conn_manager_prefix="admin"} 0
# TYPE envoy_server_total_connections gauge
envoy_server_total_connections{} 1
# TYPE envoy_runtime_num_layers gauge
envoy_runtime_num_layers{} 2
# TYPE envoy_server_parent_connections gauge
envoy_server_parent_connections{} 0
# TYPE envoy_server_stats_recent_lookups gauge
envoy_server_stats_recent_lookups{} 0
# TYPE envoy_cluster_manager_warming_clusters gauge
envoy_cluster_manager_warming_clusters{} 0
# TYPE envoy_server_days_until_first_cert_expiring gauge
envoy_server_days_until_first_cert_expiring{} 82
# TYPE envoy_server_hot_restart_epoch gauge
envoy_server_hot_restart_epoch{} 0
# TYPE envoy_http_downstream_cx_active gauge
envoy_http_downstream_cx_active{envoy_http_conn_manager_prefix="ingress_http"} 4
`
	VALIDHTTPS = `envoy_cluster_circuit_breakers_default_cx_pool_open{envoy_cluster_name="projectcontour_service-stats_9001"} 0
envoy_cluster_max_host_weight{envoy_cluster_name="projectcontour_service-stats_9001"} 0
envoy_cluster_upstream_rq_pending_active{envoy_cluster_name="projectcontour_service-stats_9001"} 0
envoy_cluster_circuit_breakers_high_rq_retry_open{envoy_cluster_name="projectcontour_service-stats_9001"} 0
envoy_cluster_circuit_breakers_high_cx_pool_open{envoy_cluster_name="projectcontour_service-stats_9001"} 0
envoy_cluster_upstream_cx_tx_bytes_buffered{envoy_cluster_name="projectcontour_service-stats_9001"} 0
envoy_cluster_version{envoy_cluster_name="projectcontour_service-stats_9001"} 0
envoy_cluster_circuit_breakers_default_cx_open{envoy_cluster_name="projectcontour_service-stats_9001"} 0
# TYPE envoy_http_downstream_cx_ssl_active gauge
envoy_http_downstream_cx_ssl_active{envoy_http_conn_manager_prefix="admin"} 0
# TYPE envoy_server_total_connections gauge
envoy_server_total_connections{} 1
# TYPE envoy_runtime_num_layers gauge
envoy_runtime_num_layers{} 2
# TYPE envoy_server_parent_connections gauge
envoy_server_parent_connections{} 0
# TYPE envoy_server_stats_recent_lookups gauge
envoy_server_stats_recent_lookups{} 0
# TYPE envoy_cluster_manager_warming_clusters gauge
envoy_cluster_manager_warming_clusters{} 0
# TYPE envoy_server_days_until_first_cert_expiring gauge
envoy_server_days_until_first_cert_expiring{} 82
# TYPE envoy_server_hot_restart_epoch gauge
envoy_server_hot_restart_epoch{} 0
# TYPE envoy_http_downstream_cx_active gauge
envoy_http_downstream_cx_active{envoy_http_conn_manager_prefix="ingress_http"} 4
`
	VALIDBOTH = `envoy_cluster_circuit_breakers_default_cx_pool_open{envoy_cluster_name="projectcontour_service-stats_9001"} 0
envoy_cluster_max_host_weight{envoy_cluster_name="projectcontour_service-stats_9001"} 0
envoy_cluster_upstream_rq_pending_active{envoy_cluster_name="projectcontour_service-stats_9001"} 0
envoy_cluster_circuit_breakers_high_rq_retry_open{envoy_cluster_name="projectcontour_service-stats_9001"} 0
envoy_cluster_circuit_breakers_high_cx_pool_open{envoy_cluster_name="projectcontour_service-stats_9001"} 0
envoy_cluster_upstream_cx_tx_bytes_buffered{envoy_cluster_name="projectcontour_service-stats_9001"} 0
envoy_cluster_version{envoy_cluster_name="projectcontour_service-stats_9001"} 0
envoy_cluster_circuit_breakers_default_cx_open{envoy_cluster_name="projectcontour_service-stats_9001"} 0
# TYPE envoy_http_downstream_cx_ssl_active gauge
envoy_http_downstream_cx_ssl_active{envoy_http_conn_manager_prefix="admin"} 0
# TYPE envoy_server_total_connections gauge
envoy_server_total_connections{} 1
# TYPE envoy_runtime_num_layers gauge
envoy_runtime_num_layers{} 2
# TYPE envoy_server_parent_connections gauge
envoy_server_parent_connections{} 0
# TYPE envoy_server_stats_recent_lookups gauge
envoy_server_stats_recent_lookups{} 0
# TYPE envoy_cluster_manager_warming_clusters gauge
envoy_cluster_manager_warming_clusters{} 0
# TYPE envoy_server_days_until_first_cert_expiring gauge
envoy_server_days_until_first_cert_expiring{} 82
# TYPE envoy_server_hot_restart_epoch gauge
envoy_server_hot_restart_epoch{} 0
# TYPE envoy_http_downstream_cx_active gauge
envoy_http_downstream_cx_active{envoy_http_conn_manager_prefix="ingress_http"} 4
envoy_http_downstream_cx_active{envoy_http_conn_manager_prefix="ingress_https"} 4
`

	MISSING_STATS = `envoy_cluster_circuit_breakers_default_cx_pool_open{envoy_cluster_name="projectcontour_service-stats_9001"} 0
envoy_cluster_max_host_weight{envoy_cluster_name="projectcontour_service-stats_9001"} 0
envoy_cluster_upstream_rq_pending_active{envoy_cluster_name="projectcontour_service-stats_9001"} 0
envoy_cluster_circuit_breakers_high_rq_retry_open{envoy_cluster_name="projectcontour_service-stats_9001"} 0
envoy_cluster_circuit_breakers_high_cx_pool_open{envoy_cluster_name="projectcontour_service-stats_9001"} 0
envoy_cluster_upstream_cx_tx_bytes_buffered{envoy_cluster_name="projectcontour_service-stats_9001"} 0
envoy_cluster_version{envoy_cluster_name="projectcontour_service-stats_9001"} 0
envoy_cluster_circuit_breakers_default_cx_open{envoy_cluster_name="projectcontour_service-stats_9001"} 0
# TYPE envoy_http_downstream_cx_ssl_active gauge
envoy_http_downstream_cx_ssl_active{envoy_http_conn_manager_prefix="admin"} 0
# TYPE envoy_server_total_connections gauge
envoy_server_total_connections{} 1
# TYPE envoy_runtime_num_layers gauge
envoy_runtime_num_layers{} 2
# TYPE envoy_server_parent_connections gauge
envoy_server_parent_connections{} 0
# TYPE envoy_server_stats_recent_lookups gauge
envoy_server_stats_recent_lookups{} 0
# TYPE envoy_cluster_manager_warming_clusters gauge
envoy_cluster_manager_warming_clusters{} 0
# TYPE envoy_server_days_until_first_cert_expiring gauge
envoy_server_days_until_first_cert_expiring{} 82
# TYPE envoy_server_hot_restart_epoch gauge
envoy_server_hot_restart_epoch{} 0
`
)
