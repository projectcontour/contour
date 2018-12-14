// Copyright © 2017 Heptio
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

package envoy

import (
	"bytes"
	"testing"
)

func TestConfigWriter_WriteYAML(t *testing.T) {
	tests := map[string]struct {
		ConfigWriter
		want string
	}{
		"default configuration": {
			ConfigWriter: ConfigWriter{},
			want: `dynamic_resources:
  lds_config:
    api_config_source:
      api_type: GRPC
      grpc_services:
      - envoy_grpc:
          cluster_name: contour
  cds_config:
    api_config_source:
      api_type: GRPC
      grpc_services:
      - envoy_grpc:
          cluster_name: contour
static_resources:
  clusters:
  - name: contour
    connect_timeout: { seconds: 5 }
    type: STRICT_DNS
    hosts:
    - socket_address:
        address: 127.0.0.1
        port_value: 8001
    lb_policy: ROUND_ROBIN
    http2_protocol_options: {}
    circuit_breakers:
      thresholds:
        - priority: high
          max_connections: 100000
          max_pending_requests: 100000
          max_requests: 60000000
          max_retries: 50
        - priority: default
          max_connections: 100000
          max_pending_requests: 100000
          max_requests: 60000000
          max_retries: 50
  - name: service_stats
    connect_timeout: 0.250s
    type: LOGICAL_DNS
    lb_policy: ROUND_ROBIN
    hosts:
      - socket_address:
          protocol: TCP
          address: 127.0.0.1
          port_value: 9001
  listeners:
    - address:
        socket_address:
          protocol: TCP
          address: 0.0.0.0
          port_value: 8002
      filter_chains:
        - filters:
            - name: envoy.http_connection_manager
              config:
                codec_type: AUTO
                stat_prefix: stats
                route_config:
                  virtual_hosts:
                    - name: backend
                      domains:
                        - "*"
                      routes:
                        - match:
                            prefix: /stats
                          route:
                            cluster: service_stats
                http_filters:
                  - name: envoy.health_check
                    config:
                      pass_through_mode: false
                      headers:
                      - name: ":path"
                        exact_match: "/healthz"
                  - name: envoy.router
                    config:
admin:
  access_log_path: /dev/null
  address:
    socket_address:
      address: 127.0.0.1
      port_value: 9001
`,
		},
		"stats address and port defined": {
			ConfigWriter: ConfigWriter{
				StatsdEnabled: true,
				StatsAddress:  "1.2.3.4",
				StatsPort:     1234,
			},
			want: `dynamic_resources:
  lds_config:
    api_config_source:
      api_type: GRPC
      grpc_services:
      - envoy_grpc:
          cluster_name: contour
  cds_config:
    api_config_source:
      api_type: GRPC
      grpc_services:
      - envoy_grpc:
          cluster_name: contour
static_resources:
  clusters:
  - name: contour
    connect_timeout: { seconds: 5 }
    type: STRICT_DNS
    hosts:
    - socket_address:
        address: 127.0.0.1
        port_value: 8001
    lb_policy: ROUND_ROBIN
    http2_protocol_options: {}
    circuit_breakers:
      thresholds:
        - priority: high
          max_connections: 100000
          max_pending_requests: 100000
          max_requests: 60000000
          max_retries: 50
        - priority: default
          max_connections: 100000
          max_pending_requests: 100000
          max_requests: 60000000
          max_retries: 50
  - name: service_stats
    connect_timeout: 0.250s
    type: LOGICAL_DNS
    lb_policy: ROUND_ROBIN
    hosts:
      - socket_address:
          protocol: TCP
          address: 127.0.0.1
          port_value: 9001
  listeners:
    - address:
        socket_address:
          protocol: TCP
          address: 1.2.3.4
          port_value: 1234
      filter_chains:
        - filters:
            - name: envoy.http_connection_manager
              config:
                codec_type: AUTO
                stat_prefix: stats
                route_config:
                  virtual_hosts:
                    - name: backend
                      domains:
                        - "*"
                      routes:
                        - match:
                            prefix: /stats
                          route:
                            cluster: service_stats
                http_filters:
                  - name: envoy.health_check
                    config:
                      pass_through_mode: false
                      headers:
                      - name: ":path"
                        exact_match: "/healthz"
                  - name: envoy.router
                    config:
stats_sinks:
  - name: envoy.statsd
    config:
      address:
        socket_address:
          protocol: UDP
          address: 127.0.0.1
          port_value: 9125
admin:
  access_log_path: /dev/null
  address:
    socket_address:
      address: 127.0.0.1
      port_value: 9001
`,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			err := tc.ConfigWriter.WriteYAML(&buf)
			checkErr(t, err)
			got := buf.String()
			if tc.want != got {
				t.Errorf("%#v: want: %s\n, got: %s", tc.ConfigWriter, tc.want, got)
			}
		})
	}
}

func checkErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
