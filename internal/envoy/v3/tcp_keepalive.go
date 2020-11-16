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
	envoy_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"github.com/projectcontour/contour/internal/envoy"
)

func TCPKeepaliveSocketOptions() []*envoy_core_v3.SocketOption {

	// Note: TCP_KEEPIDLE + (TCP_KEEPINTVL * TCP_KEEPCNT) must be greater than
	// the grpc.KeepaliveParams time + timeout (currently 60 + 20 = 80 seconds)
	// otherwise TestGRPC/StreamClusters fails.
	return []*envoy_core_v3.SocketOption{
		// Enable TCP keep-alive.
		{
			Description: "Enable TCP keep-alive",
			Level:       envoy.SOL_SOCKET,
			Name:        envoy.SO_KEEPALIVE,
			Value:       &envoy_core_v3.SocketOption_IntValue{IntValue: 1},
			State:       envoy_core_v3.SocketOption_STATE_LISTENING,
		},
		// The time (in seconds) the connection needs to remain idle
		// before TCP starts sending keepalive probes.
		{
			Description: "TCP keep-alive initial idle time",
			Level:       envoy.IPPROTO_TCP,
			Name:        envoy.TCP_KEEPIDLE,
			Value:       &envoy_core_v3.SocketOption_IntValue{IntValue: 45},
			State:       envoy_core_v3.SocketOption_STATE_LISTENING,
		},
		// The time (in seconds) between individual keepalive probes.
		{
			Description: "TCP keep-alive time between probes",
			Level:       envoy.IPPROTO_TCP,
			Name:        envoy.TCP_KEEPINTVL,
			Value:       &envoy_core_v3.SocketOption_IntValue{IntValue: 5},
			State:       envoy_core_v3.SocketOption_STATE_LISTENING,
		},
		// The maximum number of TCP keep-alive probes to send before
		// giving up and killing the connection if no response is
		// obtained from the other end.
		{
			Description: "TCP keep-alive probe count",
			Level:       envoy.IPPROTO_TCP,
			Name:        envoy.TCP_KEEPCNT,
			Value:       &envoy_core_v3.SocketOption_IntValue{IntValue: 9},
			State:       envoy_core_v3.SocketOption_STATE_LISTENING,
		},
	}
}
