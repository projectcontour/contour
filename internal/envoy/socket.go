// Copyright © 2019 VMware
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
	envoy_api_v2_auth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	envoy_api_v2_core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
)

// UpstreamTLSTransportSocket returns a custom transport socket using the UpstreamTlsContext provided.
func UpstreamTLSTransportSocket(tls *envoy_api_v2_auth.UpstreamTlsContext) *envoy_api_v2_core.TransportSocket {
	return &envoy_api_v2_core.TransportSocket{
		Name: "tls",
		ConfigType: &envoy_api_v2_core.TransportSocket_TypedConfig{
			TypedConfig: toAny(tls),
		},
	}
}

// DownstreamTLSTransportSocket returns a custom transport socket using the DownstreamTlsContext provided.
func DownstreamTLSTransportSocket(tls *envoy_api_v2_auth.DownstreamTlsContext) *envoy_api_v2_core.TransportSocket {
	return &envoy_api_v2_core.TransportSocket{
		Name: "tls",
		ConfigType: &envoy_api_v2_core.TransportSocket_TypedConfig{
			TypedConfig: toAny(tls),
		},
	}
}
