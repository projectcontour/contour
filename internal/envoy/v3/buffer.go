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
	envoy_filter_http_buffer_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/buffer/v3"
	envoy_filter_network_http_connection_manager_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/projectcontour/contour/internal/protobuf"
)

// BufferFilter returns a configured HTTP buffer filter,
// or nil if maxRequestBytes = 0.
func BufferFilter(maxRequestBytes uint32) *envoy_filter_network_http_connection_manager_v3.HttpFilter {
	if maxRequestBytes == 0 {
		return nil
	}

	return &envoy_filter_network_http_connection_manager_v3.HttpFilter{
		Name: wellknown.Buffer,
		ConfigType: &envoy_filter_network_http_connection_manager_v3.HttpFilter_TypedConfig{
			TypedConfig: protobuf.MustMarshalAny(&envoy_filter_http_buffer_v3.Buffer{
				MaxRequestBytes: wrapperspb.UInt32(maxRequestBytes),
			}),
		},
	}
}
