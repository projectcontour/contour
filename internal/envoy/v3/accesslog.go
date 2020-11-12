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
	envoy_accesslog_v3 "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v3"
	envoy_file_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/file/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	_struct "github.com/golang/protobuf/ptypes/struct"
	"github.com/projectcontour/contour/internal/protobuf"
	"github.com/projectcontour/contour/pkg/config"
)

// FileAccessLogEnvoy returns a new file based access log filter
// that will output Envoy's default access logs.
func FileAccessLogEnvoy(path string) []*envoy_accesslog_v3.AccessLog {
	return []*envoy_accesslog_v3.AccessLog{{
		Name: wellknown.FileAccessLog,
		ConfigType: &envoy_accesslog_v3.AccessLog_TypedConfig{
			TypedConfig: protobuf.MustMarshalAny(&envoy_file_v3.FileAccessLog{
				Path: path,
				// AccessLogFormat left blank to defer to Envoy's default log format.
			}),
		},
	}}
}

// FileAccessLogJSON returns a new file based access log filter
// that will log in JSON format
func FileAccessLogJSON(path string, fields config.AccessLogFields) []*envoy_accesslog_v3.AccessLog {

	jsonformat := &_struct.Struct{
		Fields: make(map[string]*_struct.Value),
	}

	for k, v := range fields.AsFieldMap() {
		jsonformat.Fields[k] = sv(v)
	}

	return []*envoy_accesslog_v3.AccessLog{{
		Name: wellknown.FileAccessLog,
		ConfigType: &envoy_accesslog_v3.AccessLog_TypedConfig{
			TypedConfig: protobuf.MustMarshalAny(&envoy_file_v3.FileAccessLog{
				Path: path,
				AccessLogFormat: &envoy_file_v3.FileAccessLog_JsonFormat{
					JsonFormat: jsonformat,
				},
			}),
		},
	}}
}

func sv(s string) *_struct.Value {
	return &_struct.Value{
		Kind: &_struct.Value_StringValue{
			StringValue: s,
		},
	}
}
