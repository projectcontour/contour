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

import envoy_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"

var defaultConfigSource = &envoy_core_v3.ConfigSource{
	ResourceApiVersion: envoy_core_v3.ApiVersion_V3,
	ConfigSourceSpecifier: &envoy_core_v3.ConfigSource_ApiConfigSource{
		ApiConfigSource: &envoy_core_v3.ApiConfigSource{
			ApiType:             envoy_core_v3.ApiConfigSource_GRPC,
			TransportApiVersion: envoy_core_v3.ApiVersion_V3,
			GrpcServices: []*envoy_core_v3.GrpcService{
				{
					TargetSpecifier: &envoy_core_v3.GrpcService_EnvoyGrpc_{
						EnvoyGrpc: &envoy_core_v3.GrpcService_EnvoyGrpc{
							ClusterName: "contour",
							Authority:   "contour",
						},
					},
				},
			},
		},
	},
}

// // httpConnectionManager creates a new HTTP Connection Manager filter
// // for the supplied route, access log, and client request timeout.
// func httpConnectionManager(routename string, accesslogger []*accesslog_v3.AccessLog, requestTimeout time.Duration) *envoy_listener_v3.Filter {
// 	cg := NewConfigGenerator()
// 	return cg.HTTPConnectionManagerBuilder().
// 		RouteConfigName(routename).
// 		MetricsPrefix(routename).
// 		AccessLoggers(accesslogger).
// 		RequestTimeout(timeout.DurationSetting(requestTimeout)).
// 		DefaultFilters().
// 		Get()
// }
