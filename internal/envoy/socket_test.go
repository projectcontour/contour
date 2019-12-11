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
	"testing"

	envoy_api_v2_auth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	envoy_api_v2_core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/projectcontour/contour/internal/assert"
)

func TestUpstreamTLSTransportSocket(t *testing.T) {
	tests := map[string]struct {
		ctxt *envoy_api_v2_auth.UpstreamTlsContext
		want *envoy_api_v2_core.TransportSocket
	}{
		"h2": {
			ctxt: UpstreamTLSContext(nil, "", "h2"),
			want: &envoy_api_v2_core.TransportSocket{
				Name: "envoy.transport_sockets.tls",
				ConfigType: &envoy_api_v2_core.TransportSocket_TypedConfig{
					TypedConfig: toAny(UpstreamTLSContext(nil, "", "h2")),
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := UpstreamTLSTransportSocket(tc.ctxt)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestDownstreamTLSTransportSocket(t *testing.T) {
	tests := map[string]struct {
		ctxt *envoy_api_v2_auth.DownstreamTlsContext
		want *envoy_api_v2_core.TransportSocket
	}{
		"default/tls": {
			ctxt: DownstreamTLSContext("default/tls", envoy_api_v2_auth.TlsParameters_TLSv1_1, "h2", "http/1.1"),
			want: &envoy_api_v2_core.TransportSocket{
				Name: "envoy.transport_sockets.tls",
				ConfigType: &envoy_api_v2_core.TransportSocket_TypedConfig{
					TypedConfig: toAny(DownstreamTLSContext("default/tls", envoy_api_v2_auth.TlsParameters_TLSv1_1, "h2", "http/1.1")),
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := DownstreamTLSTransportSocket(tc.ctxt)
			assert.Equal(t, tc.want, got)
		})
	}
}
