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
      cluster_names: [contour]
      grpc_services:
      - envoy_grpc:
          cluster_name: contour
  cds_config:
    api_config_source:
      api_type: GRPC
      cluster_names: [contour]
      grpc_services:
      - envoy_grpc:
          cluster_name: contour
static_resources:
  clusters:
  - name: contour
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
