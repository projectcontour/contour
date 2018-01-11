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

func TestConfigWriter_WriteJSON(t *testing.T) {
	tests := []struct {
		name string
		ConfigWriter
		want string
	}{{
		name: "default configuration",
		ConfigWriter: ConfigWriter{
			AdminAccessLogPath: "/tmp/admin_access.log",
			AdminAddress:       "0.0.0.0",
		},
		want: `{
  "listeners": [],
  "lds": {
    "cluster": "rds",
    "refresh_delay_ms": 1000
  },
  "admin": {
    "access_log_path": "/tmp/admin_access.log",
    "address": "tcp://0.0.0.0:9001"
  },
  "cluster_manager": {
    "clusters": [
      {
        "name": "rds",
        "type": "strict_dns",
        "connect_timeout_ms": 250,
        "lb_type": "round_robin",
        "hosts": [
          {
            "url": "tcp://127.0.0.1:8000"
          }
        ]
      }
    ],
    "sds": {
      "cluster": {
        "name": "sds",
        "type": "strict_dns",
        "connect_timeout_ms": 250,
        "lb_type": "round_robin",
        "hosts": [
          {
            "url": "tcp://127.0.0.1:8000"
          }
        ]
      },
      "refresh_delay_ms": 1000
    },
    "cds": {
      "cluster": {
        "name": "cds",
        "type": "strict_dns",
        "connect_timeout_ms": 250,
        "lb_type": "round_robin",
        "hosts": [
          {
            "url": "tcp://127.0.0.1:8000"
          }
        ]
      },
      "refresh_delay_ms": 1000
    }
  }
}
`,
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := tc.ConfigWriter.WriteJSON(&buf)
			checkErr(t, err)
			got := buf.String()
			if tc.want != got {
				t.Errorf("%#v: want: %s\n, got: %s", tc.ConfigWriter, tc.want, got)
			}
		})
	}
}

func TestConfigWriter_WriteYAML(t *testing.T) {
	tests := []struct {
		name string
		ConfigWriter
		want string
	}{{
		name:         "default configuration",
		ConfigWriter: ConfigWriter{},
		want: `dynamic_resources:
  lds_config:
    api_config_source:
      api_type: GRPC
      cluster_names: [xds_cluster]
      grpc_services:
      - envoy_grpc:
          cluster_name: xds_cluster
  cds_config:
    api_config_source:
      api_type: GRPC
      cluster_names: [xds_cluster]
      grpc_services:
      - envoy_grpc:
          cluster_name: xds_cluster
static_resources:
  clusters:
  - name: xds_cluster
    connect_timeout: { seconds: 5 }
    type: STATIC
    hosts:
    - socket_address:
        address: 127.0.0.1
        port_value: 8001
    lb_policy: ROUND_ROBIN
    http2_protocol_options: {}
admin:
  access_log_path: /dev/null
  address:
    socket_address:
      address: 127.0.0.1
      port_value: 9001
`,
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
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
