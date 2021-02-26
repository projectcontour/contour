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

package v3

import (
	"testing"

	envoy_accesslog_v3 "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_file_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/file/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	_struct "github.com/golang/protobuf/ptypes/struct"
	"github.com/projectcontour/contour/internal/protobuf"
	"github.com/projectcontour/contour/pkg/config"
)

func TestFileAccessLog(t *testing.T) {
	tests := map[string]struct {
		path string
		want []*envoy_accesslog_v3.AccessLog
	}{
		"stdout": {
			path: "/dev/stdout",
			want: []*envoy_accesslog_v3.AccessLog{{
				Name: wellknown.FileAccessLog,
				ConfigType: &envoy_accesslog_v3.AccessLog_TypedConfig{
					TypedConfig: protobuf.MustMarshalAny(&envoy_file_v3.FileAccessLog{
						Path: "/dev/stdout",
					}),
				},
			}},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := FileAccessLogEnvoy(tc.path)
			protobuf.ExpectEqual(t, tc.want, got)
		})
	}
}

func TestJSONFileAccessLog(t *testing.T) {
	tests := map[string]struct {
		path    string
		headers config.AccessLogFields
		want    []*envoy_accesslog_v3.AccessLog
	}{
		"only timestamp": {
			path:    "/dev/stdout",
			headers: config.AccessLogFields([]string{"@timestamp"}),
			want: []*envoy_accesslog_v3.AccessLog{{
				Name: wellknown.FileAccessLog,
				ConfigType: &envoy_accesslog_v3.AccessLog_TypedConfig{
					TypedConfig: protobuf.MustMarshalAny(&envoy_file_v3.FileAccessLog{
						Path: "/dev/stdout",
						AccessLogFormat: &envoy_file_v3.FileAccessLog_LogFormat{
							LogFormat: &envoy_config_core_v3.SubstitutionFormatString{
								Format: &envoy_config_core_v3.SubstitutionFormatString_JsonFormat{
									JsonFormat: &_struct.Struct{
										Fields: map[string]*_struct.Value{
											"@timestamp": sv("%START_TIME%"),
										},
									},
								},
							},
						},
					}),
				},
			},
			},
		},
		"custom fields should appear": {
			path: "/dev/stdout",
			headers: config.AccessLogFields([]string{
				"@timestamp",
				"method",
				"custom1=%REQ(X-CUSTOM-HEADER)%",
				"custom2=%DURATION%.0",
				"custom3=ST=%START_TIME(%s.%6f)%",
			}),
			want: []*envoy_accesslog_v3.AccessLog{{
				Name: wellknown.FileAccessLog,
				ConfigType: &envoy_accesslog_v3.AccessLog_TypedConfig{
					TypedConfig: protobuf.MustMarshalAny(&envoy_file_v3.FileAccessLog{
						Path: "/dev/stdout",
						AccessLogFormat: &envoy_file_v3.FileAccessLog_LogFormat{
							LogFormat: &envoy_config_core_v3.SubstitutionFormatString{
								Format: &envoy_config_core_v3.SubstitutionFormatString_JsonFormat{
									JsonFormat: &_struct.Struct{
										Fields: map[string]*_struct.Value{
											"@timestamp": sv("%START_TIME%"),
											"method":     sv("%REQ(:METHOD)%"),
											"custom1":    sv("%REQ(X-CUSTOM-HEADER)%"),
											"custom2":    sv("%DURATION%.0"),
											"custom3":    sv("ST=%START_TIME(%s.%6f)%"),
										},
									},
								},
							},
						},
					}),
				},
			},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := FileAccessLogJSON(tc.path, tc.headers)
			protobuf.ExpectEqual(t, tc.want, got)
		})
	}
}
