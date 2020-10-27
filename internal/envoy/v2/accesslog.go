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

package v2

import (
	accesslogv2 "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v2"
	accesslog "github.com/envoyproxy/go-control-plane/envoy/config/filter/accesslog/v2"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	_struct "github.com/golang/protobuf/ptypes/struct"
	"github.com/projectcontour/contour/internal/protobuf"
	"github.com/projectcontour/contour/pkg/config"
)

// FileAccessLogEnvoy returns a new file based access log filter
// that will output Envoy's default access logs.
func FileAccessLogEnvoy(path string) []*accesslog.AccessLog {
	return []*accesslog.AccessLog{{
		Name: wellknown.FileAccessLog,
		ConfigType: &accesslog.AccessLog_TypedConfig{
			TypedConfig: protobuf.MustMarshalAny(&accesslogv2.FileAccessLog{
				Path: path,
				// AccessLogFormat left blank to defer to Envoy's default log format.
			}),
		},
	}}
}

// FileAccessLogJSON returns a new file based access log filter
// that will log in JSON format
func FileAccessLogJSON(path string, fields config.AccessLogFields) []*accesslog.AccessLog {

	jsonformat := &_struct.Struct{
		Fields: make(map[string]*_struct.Value),
	}

	for k, v := range fields.AsFieldMap() {
		jsonformat.Fields[k] = sv(v)
	}

	return []*accesslog.AccessLog{{
		Name: wellknown.FileAccessLog,
		ConfigType: &accesslog.AccessLog_TypedConfig{
			TypedConfig: protobuf.MustMarshalAny(&accesslogv2.FileAccessLog{
				Path: path,
				AccessLogFormat: &accesslogv2.FileAccessLog_JsonFormat{
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
