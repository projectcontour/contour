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

// Package envoy contains a configuration writer for v2 YAML config.
// To avoid a dependncy on a YAML library, we generate the YAML using
// the text/template package.
package envoy

import (
	"io"
	"text/template"
)

// A ConfigWriter knows how to write a bootstap Envoy configuration in YAML format.
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

	// StatsAddress is the address that Envoy will listen on which serves the /stats path
	// Defaults to 0.0.0.0
	StatsAddress string

	// StatsPort is the port that Envoy will listen on which serves the /stats path
	// Defaults to 8002
	StatsPort int

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

const yamlConfig = `dynamic_resources:
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
        address: {{ if .XDSAddress }}{{ .XDSAddress }}{{ else }}127.0.0.1{{ end }}
        port_value: {{ if .XDSGRPCPort }}{{ .XDSGRPCPort }}{{ else }}8001{{ end }}
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
  - name: service-stats
    connect_timeout: 0.250s
    type: LOGICAL_DNS
    lb_policy: ROUND_ROBIN
    hosts:
      - socket_address:
          protocol: TCP
          address: 127.0.0.1
          port_value: {{ if .AdminPort }}{{ .AdminPort }}{{ else }}9001{{ end }}
  listeners:
    - address:
        socket_address:
          protocol: TCP
          address: {{ if .StatsAddress }}{{ .StatsAddress }}{{ else }}0.0.0.0{{ end }}
          port_value: {{ if .StatsPort }}{{ .StatsPort }}{{ else }}8002{{ end }}
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
                            cluster: service-stats
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
