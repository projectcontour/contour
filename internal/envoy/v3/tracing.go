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
	"strings"

	envoy_config_trace_v3 "github.com/envoyproxy/go-control-plane/envoy/config/trace/v3"
	envoy_filter_network_http_connection_manager_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoy_trace_v3 "github.com/envoyproxy/go-control-plane/envoy/type/tracing/v3"
	envoy_type_v3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"k8s.io/apimachinery/pkg/types"

	contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/protobuf"
	"github.com/projectcontour/contour/internal/timeout"
)

// TracingConfig returns a tracing config,
// or nil if config is nil.
func TracingConfig(tracing *EnvoyTracingConfig) *envoy_filter_network_http_connection_manager_v3.HttpConnectionManager_Tracing {
	if tracing == nil {
		return nil
	}

	var customTags []*envoy_trace_v3.CustomTag
	for _, tag := range tracing.CustomTags {
		if traceCustomTag := customTag(tag); traceCustomTag != nil {
			customTags = append(customTags, traceCustomTag)
		}
	}

	connManagerTracing := &envoy_filter_network_http_connection_manager_v3.HttpConnectionManager_Tracing{
		OverallSampling: &envoy_type_v3.Percent{
			Value: tracing.OverallSampling,
		},
		MaxPathTagLength:  wrapperspb.UInt32(tracing.MaxPathTagLength),
		CustomTags:        customTags,
		SpawnUpstreamSpan: wrapperspb.Bool(true),
	}

	switch tracing.System {
	case contour_v1alpha1.TracingSystemOpenTelemetry:
		connManagerTracing.Provider = &envoy_config_trace_v3.Tracing_Http{
			Name: "envoy.tracers.opentelemetry",
			ConfigType: &envoy_config_trace_v3.Tracing_Http_TypedConfig{
				TypedConfig: protobuf.MustMarshalAny(&envoy_config_trace_v3.OpenTelemetryConfig{
					GrpcService: GrpcService(dag.ExtensionClusterName(tracing.ExtensionService), tracing.SNI, tracing.Timeout),
					ServiceName: tracing.ServiceName,
				}),
			},
		}
	case contour_v1alpha1.TracingSystemZipkin:
		connManagerTracing.Provider = &envoy_config_trace_v3.Tracing_Http{
			Name: "envoy.tracers.zipkin",
			ConfigType: &envoy_config_trace_v3.Tracing_Http_TypedConfig{
				TypedConfig: protobuf.MustMarshalAny(&envoy_config_trace_v3.ZipkinConfig{
					CollectorCluster:         dag.ExtensionClusterName(tracing.ExtensionService),
					CollectorHostname:        strings.ReplaceAll(dag.ExtensionClusterName(tracing.ExtensionService), "/", "."),
					CollectorEndpoint:        "/api/v2/spans",
					SharedSpanContext:        wrapperspb.Bool(false),
					CollectorEndpointVersion: envoy_config_trace_v3.ZipkinConfig_HTTP_JSON,
				}),
			},
		}
	}

	return connManagerTracing
}

func customTag(tag *CustomTag) *envoy_trace_v3.CustomTag {
	if tag == nil {
		return nil
	}
	if tag.Literal != "" {
		return &envoy_trace_v3.CustomTag{
			Tag: tag.TagName,
			Type: &envoy_trace_v3.CustomTag_Literal_{
				Literal: &envoy_trace_v3.CustomTag_Literal{
					Value: tag.Literal,
				},
			},
		}
	}
	if tag.EnvironmentName != "" {
		return &envoy_trace_v3.CustomTag{
			Tag: tag.TagName,
			Type: &envoy_trace_v3.CustomTag_Environment_{
				Environment: &envoy_trace_v3.CustomTag_Environment{
					Name: tag.EnvironmentName,
				},
			},
		}
	}
	if tag.RequestHeaderName != "" {
		return &envoy_trace_v3.CustomTag{
			Tag: tag.TagName,
			Type: &envoy_trace_v3.CustomTag_RequestHeader{
				RequestHeader: &envoy_trace_v3.CustomTag_Header{
					Name: tag.RequestHeaderName,
				},
			},
		}
	}
	return nil
}

type EnvoyTracingConfig struct {
	ExtensionService types.NamespacedName
	ServiceName      string
	SNI              string
	Timeout          timeout.Setting
	OverallSampling  float64
	MaxPathTagLength uint32
	CustomTags       []*CustomTag
	System           contour_v1alpha1.TracingSystem
}

type CustomTag struct {
	TagName           string
	Literal           string
	EnvironmentName   string
	RequestHeaderName string
}
