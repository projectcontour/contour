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
	"io"
	"text/template"
)

// Configuration writers for v1 JSON config and v2 YAML config.
// To avoid a dependncy on a YAML library, we generate the YAML using
// the text/template package. For consistency we do the same for JSON.

// A ConfigWriter knows how to write a bootstap Envoy configuration in both JSON
// and YAML formats.
type ConfigWriter struct {
	// AdminAccessLogPath is the path to write the access log for the administration server.
	// Defaults to /dev/null.
	AdminAccessLogPath string

	// AdminAddress is the TCP address that the administration server will listen on.
	// Defaults to 127.0.0.1.
	AdminAddress string

	// AdminPort is the port that the administration server will listen on.
	// Defaults to 9001.
	AdminPort int

	// XDSAddress is the TCP address of the XDS management server. For JSON configurations
	// this is the address of the v1 REST API server. For YAML configurations this is the
	// address of the v2 gRPC management server.
	// Defaults to 127.0.0.1.
	XDSAddress string

	// XDSRESTPort is the management server port that provides the v1 REST API.
	// Defaults to 8000.
	XDSRESTPort int

	// XDSGRPCPort is the management server port that provides the v2 gRPC API.
	// Defaults to 8001.
	XDSGRPCPort int
}

const jsonConfig = `{
  "listeners": [],
  "lds": {
    "cluster": "rds",
    "refresh_delay_ms": 1000
  },
  "admin": {
    "access_log_path": "{{ if .AdminAccessLogPath }}{{ .AdminAccessLogPath }}{{ else }}/dev/null{{ end }}",
    "address": "tcp://{{ if .AdminAddress }}{{ .AdminAddress }}{{ else }}127.0.0.1{{ end }}:{{ if .AdminPort }}{{ .AdminPort }}{{ else }}9001{{ end }}"
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
            "url": "tcp://{{ if .XDSAddress }}{{ .XDSAddress }}{{ else }}127.0.0.1{{ end }}:{{ if .XDSRESTPort }}{{ .XDSRESTPort }}{{ else }}8000{{ end }}"
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
            "url": "tcp://{{ if .XDSAddress }}{{ .XDSAddress }}{{ else }}127.0.0.1{{ end }}:{{ if .XDSRESTPort }}{{ .XDSRESTPort }}{{ else }}8000{{ end }}"
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
            "url": "tcp://{{ if .XDSAddress }}{{ .XDSAddress }}{{ else }}127.0.0.1{{ end }}:{{ if .XDSRESTPort }}{{ .XDSRESTPort }}{{ else }}8000{{ end }}"
          }
        ]
      },
      "refresh_delay_ms": 1000
    }
  }
}
`

// WriteJSON writes the configuration to the supplied writer in JSON v1 format.
// If the supplied io.Writer is a file, it should end with a .json extension.
func (c *ConfigWriter) WriteJSON(w io.Writer) error {
	t, err := template.New("config").Parse(jsonConfig)
	if err != nil {
		return err
	}
	return t.Execute(w, c)
}

const yamlConfig = `dynamic_resources:
  lds_config:
    api_config_source:
      api_type: GRPC
      cluster_name: [xds_cluster]
  cds_config:
    api_config_source:
      api_type: GRPC
      cluster_name: [xds_cluster]
static_resources:
  clusters:
  - name: xds_cluster
    connect_timeout: { seconds: 5 }
    type: STATIC
    hosts:
    - socket_address:
        address: {{ if .XDSAddress }}{{ .XDSAddress }}{{ else }}127.0.0.1{{ end }}
        port_value: {{ if .XDSGRPCPort }}{{ .XDSGRPCPOrt }}{{ else }}8001{{ end }}
    lb_policy: ROUND_ROBIN
    http2_protocol_options: {}
admin:
  access_log_path: {{ if .AdminAccessLogPath }}{{ .AdminAccessLogPath }}{{ else }}/dev/null{{ end }}
  address:
    socket_address:
      address: {{ if .AdminAddress }}{{ .AdminAddress }}{{ else }}127.0.0.1{{ end }}
      port_value: {{ if .AdminPort }}{{ .AdminPort }}{{ else }}9001{{ end }}
`

// WriteYAML writes the configuration to the supplied writer in YAML v2 format.
// If the supplied io.Writer is a file, it should end with a .yaml extension.
func (c *ConfigWriter) WriteYAML(w io.Writer) error {
	t, err := template.New("config").Parse(yamlConfig)
	if err != nil {
		return err
	}
	return t.Execute(w, c)
}
