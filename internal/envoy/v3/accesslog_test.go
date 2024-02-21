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

	envoy_config_accesslog_v3 "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_access_logger_file_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/file/v3"
	envoy_formatter_req_without_query_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/formatter/req_without_query/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/types/known/structpb"

	contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/protobuf"
)

func TestFileAccessLog(t *testing.T) {
	tests := map[string]struct {
		path       string
		format     string
		extensions []string
		want       []*envoy_config_accesslog_v3.AccessLog
	}{
		"stdout": {
			path: "/dev/stdout",
			want: []*envoy_config_accesslog_v3.AccessLog{{
				Name: wellknown.FileAccessLog,
				ConfigType: &envoy_config_accesslog_v3.AccessLog_TypedConfig{
					TypedConfig: protobuf.MustMarshalAny(&envoy_access_logger_file_v3.FileAccessLog{
						Path: "/dev/stdout",
					}),
				},
			}},
		},
		"custom log format": {
			path:   "/dev/stdout",
			format: "%START_TIME%\n",
			want: []*envoy_config_accesslog_v3.AccessLog{{
				Name: wellknown.FileAccessLog,
				ConfigType: &envoy_config_accesslog_v3.AccessLog_TypedConfig{
					TypedConfig: protobuf.MustMarshalAny(&envoy_access_logger_file_v3.FileAccessLog{
						Path: "/dev/stdout",
						AccessLogFormat: &envoy_access_logger_file_v3.FileAccessLog_LogFormat{
							LogFormat: &envoy_config_core_v3.SubstitutionFormatString{
								Format: &envoy_config_core_v3.SubstitutionFormatString_TextFormatSource{
									TextFormatSource: &envoy_config_core_v3.DataSource{
										Specifier: &envoy_config_core_v3.DataSource_InlineString{
											InlineString: "%START_TIME%\n",
										},
									},
								},
							},
						},
					}),
				},
			}},
		},
		"custom log format with access log extension": {
			path:       "/dev/stdout",
			format:     "[%START_TIME%] \"%REQ_WITHOUT_QUERY(X-ENVOY-ORIGINAL-PATH?:PATH)%\"\n",
			extensions: []string{"envoy.formatter.req_without_query"},
			want: []*envoy_config_accesslog_v3.AccessLog{{
				Name: wellknown.FileAccessLog,
				ConfigType: &envoy_config_accesslog_v3.AccessLog_TypedConfig{
					TypedConfig: protobuf.MustMarshalAny(&envoy_access_logger_file_v3.FileAccessLog{
						Path: "/dev/stdout",
						AccessLogFormat: &envoy_access_logger_file_v3.FileAccessLog_LogFormat{
							LogFormat: &envoy_config_core_v3.SubstitutionFormatString{
								Format: &envoy_config_core_v3.SubstitutionFormatString_TextFormatSource{
									TextFormatSource: &envoy_config_core_v3.DataSource{
										Specifier: &envoy_config_core_v3.DataSource_InlineString{
											InlineString: "[%START_TIME%] \"%REQ_WITHOUT_QUERY(X-ENVOY-ORIGINAL-PATH?:PATH)%\"\n",
										},
									},
								},
								Formatters: []*envoy_config_core_v3.TypedExtensionConfig{{
									Name:        "envoy.formatter.req_without_query",
									TypedConfig: protobuf.MustMarshalAny(&envoy_formatter_req_without_query_v3.ReqWithoutQuery{ /* empty */ }),
								}},
							},
						},
					}),
				},
			}},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := FileAccessLogEnvoy(tc.path, tc.format, tc.extensions, contour_v1alpha1.LogLevelInfo)
			protobuf.ExpectEqual(t, tc.want, got)
		})
	}
}

func TestJSONFileAccessLog(t *testing.T) {
	tests := map[string]struct {
		path    string
		headers contour_v1alpha1.AccessLogJSONFields
		want    []*envoy_config_accesslog_v3.AccessLog
	}{
		"only timestamp": {
			path:    "/dev/stdout",
			headers: contour_v1alpha1.AccessLogJSONFields([]string{"@timestamp"}),
			want: []*envoy_config_accesslog_v3.AccessLog{
				{
					Name: wellknown.FileAccessLog,
					ConfigType: &envoy_config_accesslog_v3.AccessLog_TypedConfig{
						TypedConfig: protobuf.MustMarshalAny(&envoy_access_logger_file_v3.FileAccessLog{
							Path: "/dev/stdout",
							AccessLogFormat: &envoy_access_logger_file_v3.FileAccessLog_LogFormat{
								LogFormat: &envoy_config_core_v3.SubstitutionFormatString{
									OmitEmptyValues: true,
									Format: &envoy_config_core_v3.SubstitutionFormatString_JsonFormat{
										JsonFormat: &structpb.Struct{
											Fields: map[string]*structpb.Value{
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
			headers: contour_v1alpha1.AccessLogJSONFields([]string{
				"@timestamp",
				"method",
				"custom1=%REQ(X-CUSTOM-HEADER)%",
				"custom2=%DURATION%.0",
				"custom3=ST=%START_TIME(%s.%6f)%",
			}),
			want: []*envoy_config_accesslog_v3.AccessLog{
				{
					Name: wellknown.FileAccessLog,
					ConfigType: &envoy_config_accesslog_v3.AccessLog_TypedConfig{
						TypedConfig: protobuf.MustMarshalAny(&envoy_access_logger_file_v3.FileAccessLog{
							Path: "/dev/stdout",
							AccessLogFormat: &envoy_access_logger_file_v3.FileAccessLog_LogFormat{
								LogFormat: &envoy_config_core_v3.SubstitutionFormatString{
									OmitEmptyValues: true,
									Format: &envoy_config_core_v3.SubstitutionFormatString_JsonFormat{
										JsonFormat: &structpb.Struct{
											Fields: map[string]*structpb.Value{
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
			got := FileAccessLogJSON(tc.path, tc.headers, nil, contour_v1alpha1.LogLevelInfo)
			protobuf.ExpectEqual(t, tc.want, got)
		})
	}
}

func TestAccessLogLevel(t *testing.T) {
	tests := map[string]struct {
		level          contour_v1alpha1.AccessLogLevel
		wantRespStatus uint32
	}{
		"Error Logs": {
			level:          contour_v1alpha1.LogLevelError,
			wantRespStatus: 300,
		},
		"Server Error Logs": {
			level:          contour_v1alpha1.LogLevelCritical,
			wantRespStatus: 500,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := FileAccessLogEnvoy("/dev/stdout", "", nil, tc.level)
			want := []*envoy_config_accesslog_v3.AccessLog{{
				Name: wellknown.FileAccessLog,
				ConfigType: &envoy_config_accesslog_v3.AccessLog_TypedConfig{
					TypedConfig: protobuf.MustMarshalAny(&envoy_access_logger_file_v3.FileAccessLog{
						Path: "/dev/stdout",
					}),
				},
				Filter: &envoy_config_accesslog_v3.AccessLogFilter{
					FilterSpecifier: &envoy_config_accesslog_v3.AccessLogFilter_OrFilter{
						OrFilter: &envoy_config_accesslog_v3.OrFilter{
							Filters: []*envoy_config_accesslog_v3.AccessLogFilter{
								{
									FilterSpecifier: &envoy_config_accesslog_v3.AccessLogFilter_StatusCodeFilter{
										StatusCodeFilter: &envoy_config_accesslog_v3.StatusCodeFilter{
											Comparison: &envoy_config_accesslog_v3.ComparisonFilter{
												Op: envoy_config_accesslog_v3.ComparisonFilter_GE,
												Value: &envoy_config_core_v3.RuntimeUInt32{
													DefaultValue: tc.wantRespStatus,
													RuntimeKey:   "contour.accesslog.filter.status_code",
												},
											},
										},
									},
								},
								{
									FilterSpecifier: &envoy_config_accesslog_v3.AccessLogFilter_ResponseFlagFilter{},
								},
							},
						},
					},
				},
			}}
			protobuf.ExpectEqual(t, want, got)
		})
	}

	// Log level disabled should return nil.
	assert.Nil(t, FileAccessLogEnvoy("/dev/stdout", "", nil, contour_v1alpha1.LogLevelDisabled))

	got := FileAccessLogJSON("/dev/stdout", nil, nil, contour_v1alpha1.LogLevelError)
	want := []*envoy_config_accesslog_v3.AccessLog{{
		Name: wellknown.FileAccessLog,
		ConfigType: &envoy_config_accesslog_v3.AccessLog_TypedConfig{
			TypedConfig: protobuf.MustMarshalAny(&envoy_access_logger_file_v3.FileAccessLog{
				Path: "/dev/stdout",
				AccessLogFormat: &envoy_access_logger_file_v3.FileAccessLog_LogFormat{
					LogFormat: &envoy_config_core_v3.SubstitutionFormatString{
						OmitEmptyValues: true,
						Format: &envoy_config_core_v3.SubstitutionFormatString_JsonFormat{
							JsonFormat: &structpb.Struct{
								Fields: map[string]*structpb.Value{},
							},
						},
					},
				},
			}),
		},
		Filter: &envoy_config_accesslog_v3.AccessLogFilter{
			FilterSpecifier: &envoy_config_accesslog_v3.AccessLogFilter_OrFilter{
				OrFilter: &envoy_config_accesslog_v3.OrFilter{
					Filters: []*envoy_config_accesslog_v3.AccessLogFilter{
						{
							FilterSpecifier: &envoy_config_accesslog_v3.AccessLogFilter_StatusCodeFilter{
								StatusCodeFilter: &envoy_config_accesslog_v3.StatusCodeFilter{
									Comparison: &envoy_config_accesslog_v3.ComparisonFilter{
										Op: envoy_config_accesslog_v3.ComparisonFilter_GE,
										Value: &envoy_config_core_v3.RuntimeUInt32{
											DefaultValue: 300,
											RuntimeKey:   "contour.accesslog.filter.status_code",
										},
									},
								},
							},
						},
						{
							FilterSpecifier: &envoy_config_accesslog_v3.AccessLogFilter_ResponseFlagFilter{},
						},
					},
				},
			},
		},
	}}
	protobuf.ExpectEqual(t, want, got)

	// Log level disabled should return nil.
	assert.Nil(t, FileAccessLogJSON("/dev/stdout", nil, nil, contour_v1alpha1.LogLevelDisabled))
}
