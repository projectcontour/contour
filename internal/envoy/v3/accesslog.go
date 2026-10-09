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
	"time"

	envoy_config_accesslog_v3 "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_access_logger_file_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/file/v3"
	envoy_process_ratelimit_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/filters/process_ratelimit/v3"
	envoy_formatter_metadata_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/formatter/metadata/v3"
	envoy_formatter_req_without_query_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/formatter/req_without_query/v3"
	envoy_type_v3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/protobuf"
)

// FileAccessLogEnvoy returns a new file based access log filter
func FileAccessLogEnvoy(path, format string, extensions []string, level contour_v1alpha1.AccessLogLevel) []*envoy_config_accesslog_v3.AccessLog {
	if level == contour_v1alpha1.LogLevelDisabled {
		return nil
	}

	var filter *envoy_config_accesslog_v3.AccessLogFilter
	switch level {
	case contour_v1alpha1.LogLevelError:
		filter = filterOnlyErrors(300) // We want to log resp status >= 300
	case contour_v1alpha1.LogLevelCritical:
		filter = filterOnlyErrors(500) // We want to log resp status >= 500
	}
	// Nil by default to defer to Envoy's default log format.
	var logFormat *envoy_access_logger_file_v3.FileAccessLog_LogFormat

	if format != "" {
		logFormat = &envoy_access_logger_file_v3.FileAccessLog_LogFormat{
			LogFormat: &envoy_config_core_v3.SubstitutionFormatString{
				Format: &envoy_config_core_v3.SubstitutionFormatString_TextFormatSource{
					TextFormatSource: &envoy_config_core_v3.DataSource{
						Specifier: &envoy_config_core_v3.DataSource_InlineString{
							InlineString: format,
						},
					},
				},
				Formatters: extensionConfig(extensions),
			},
		}
	}

	return []*envoy_config_accesslog_v3.AccessLog{{
		Name: wellknown.FileAccessLog,
		ConfigType: &envoy_config_accesslog_v3.AccessLog_TypedConfig{
			TypedConfig: protobuf.MustMarshalAny(&envoy_access_logger_file_v3.FileAccessLog{
				Path:            path,
				AccessLogFormat: logFormat,
			}),
		},
		Filter: filter,
	}}
}

// FileAccessLogJSON returns a new file based access log filter
// that will log in JSON format
func FileAccessLogJSON(path string, fields contour_v1alpha1.AccessLogJSONFields, extensions []string, level contour_v1alpha1.AccessLogLevel) []*envoy_config_accesslog_v3.AccessLog {
	if level == contour_v1alpha1.LogLevelDisabled {
		return nil
	}

	var filter *envoy_config_accesslog_v3.AccessLogFilter
	switch level {
	case contour_v1alpha1.LogLevelError:
		filter = filterOnlyErrors(300) // We want to log resp status >= 300
	case contour_v1alpha1.LogLevelCritical:
		filter = filterOnlyErrors(500) // We want to log resp status >= 500
	}

	jsonformat := &structpb.Struct{
		Fields: make(map[string]*structpb.Value),
	}

	for k, v := range fields.AsFieldMap() {
		jsonformat.Fields[k] = sv(v)
	}

	return []*envoy_config_accesslog_v3.AccessLog{{
		Name: wellknown.FileAccessLog,
		ConfigType: &envoy_config_accesslog_v3.AccessLog_TypedConfig{
			TypedConfig: protobuf.MustMarshalAny(&envoy_access_logger_file_v3.FileAccessLog{
				Path: path,
				AccessLogFormat: &envoy_access_logger_file_v3.FileAccessLog_LogFormat{
					LogFormat: &envoy_config_core_v3.SubstitutionFormatString{
						Format: &envoy_config_core_v3.SubstitutionFormatString_JsonFormat{
							JsonFormat: jsonformat,
						},
						OmitEmptyValues: true,
						Formatters:      extensionConfig(extensions),
					},
				},
			}),
		},
		Filter: filter,
	}}
}

func sv(s string) *structpb.Value {
	return &structpb.Value{
		Kind: &structpb.Value_StringValue{
			StringValue: s,
		},
	}
}

// WithProcessRateLimit wraps access log entries with a ProcessRateLimitFilter
// that rate limits log emission using an inline token bucket.
func WithProcessRateLimit(accessLogs []*envoy_config_accesslog_v3.AccessLog, maxTokens uint32, tokensPerFill uint32, fillInterval time.Duration) []*envoy_config_accesslog_v3.AccessLog {
	rateLimitFilter := &envoy_config_accesslog_v3.AccessLogFilter{
		FilterSpecifier: &envoy_config_accesslog_v3.AccessLogFilter_ExtensionFilter{
			ExtensionFilter: &envoy_config_accesslog_v3.ExtensionFilter{
				Name: "envoy.access_loggers.extension_filters.process_ratelimit",
				ConfigType: &envoy_config_accesslog_v3.ExtensionFilter_TypedConfig{
					TypedConfig: protobuf.MustMarshalAny(&envoy_process_ratelimit_v3.ProcessRateLimitFilter{
						TokenBucket: &envoy_type_v3.TokenBucket{
							MaxTokens:     maxTokens,
							TokensPerFill: wrapperspb.UInt32(tokensPerFill),
							FillInterval:  durationpb.New(fillInterval),
						},
					}),
				},
			},
		},
	}

	for i, al := range accessLogs {
		if al.Filter == nil {
			accessLogs[i].Filter = rateLimitFilter
		} else {
			accessLogs[i].Filter = &envoy_config_accesslog_v3.AccessLogFilter{
				FilterSpecifier: &envoy_config_accesslog_v3.AccessLogFilter_AndFilter{
					AndFilter: &envoy_config_accesslog_v3.AndFilter{
						Filters: []*envoy_config_accesslog_v3.AccessLogFilter{
							al.Filter,
							rateLimitFilter,
						},
					},
				},
			}
		}
	}

	return accessLogs
}

// extensionConfig returns a list of extension configs required by the access log format.
//
// Note: When adding support for new formatter, update the list of extensions here and
// add the corresponding extension in pkg/config/parameters.go AccessLogFormatterExtensions().
// Currently only one extension exist in Envoy.
func extensionConfig(extensions []string) []*envoy_config_core_v3.TypedExtensionConfig {
	var config []*envoy_config_core_v3.TypedExtensionConfig

	for _, e := range extensions {
		switch e {
		case "envoy.formatter.req_without_query":
			config = append(config, &envoy_config_core_v3.TypedExtensionConfig{
				Name:        "envoy.formatter.req_without_query",
				TypedConfig: protobuf.MustMarshalAny(&envoy_formatter_req_without_query_v3.ReqWithoutQuery{ /* empty */ }),
			})
		case "envoy.formatter.metadata":
			config = append(config, &envoy_config_core_v3.TypedExtensionConfig{
				Name:        "envoy.formatter.metadata",
				TypedConfig: protobuf.MustMarshalAny(&envoy_formatter_metadata_v3.Metadata{}),
			})
		}
	}

	return config
}

func filterOnlyErrors(respCodeMin uint32) *envoy_config_accesslog_v3.AccessLogFilter {
	return &envoy_config_accesslog_v3.AccessLogFilter{
		FilterSpecifier: &envoy_config_accesslog_v3.AccessLogFilter_OrFilter{
			OrFilter: &envoy_config_accesslog_v3.OrFilter{
				Filters: []*envoy_config_accesslog_v3.AccessLogFilter{
					{
						FilterSpecifier: &envoy_config_accesslog_v3.AccessLogFilter_StatusCodeFilter{
							StatusCodeFilter: &envoy_config_accesslog_v3.StatusCodeFilter{
								Comparison: &envoy_config_accesslog_v3.ComparisonFilter{
									Op: envoy_config_accesslog_v3.ComparisonFilter_GE,
									Value: &envoy_config_core_v3.RuntimeUInt32{
										DefaultValue: respCodeMin,
										RuntimeKey:   "contour.accesslog.filter.status_code",
									},
								},
							},
						},
					},
					{
						FilterSpecifier: &envoy_config_accesslog_v3.AccessLogFilter_ResponseFlagFilter{
							ResponseFlagFilter: &envoy_config_accesslog_v3.ResponseFlagFilter{
								// Left empty to match all response flags, they all represent errors.
							},
						},
					},
				},
			},
		},
	}
}
