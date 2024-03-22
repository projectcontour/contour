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
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"

	"github.com/projectcontour/contour/internal/envoy"
)

type SocketOptions struct {
	options []*envoy_config_core_v3.SocketOption
}

func NewSocketOptions() *SocketOptions {
	return &SocketOptions{}
}

func (so *SocketOptions) TCPKeepalive() *SocketOptions {
	so.options = append(so.options,
		// Enable TCP keep-alive.
		&envoy_config_core_v3.SocketOption{
			Description: "Enable TCP keep-alive",
			Level:       envoy.SOL_SOCKET,
			Name:        envoy.SO_KEEPALIVE,
			Value:       &envoy_config_core_v3.SocketOption_IntValue{IntValue: 1},
			State:       envoy_config_core_v3.SocketOption_STATE_LISTENING,
		},
		// The time (in seconds) the connection needs to remain idle
		// before TCP starts sending keepalive probes.
		&envoy_config_core_v3.SocketOption{
			Description: "TCP keep-alive initial idle time",
			Level:       envoy.IPPROTO_TCP,
			Name:        envoy.TCP_KEEPIDLE,
			Value:       &envoy_config_core_v3.SocketOption_IntValue{IntValue: 45},
			State:       envoy_config_core_v3.SocketOption_STATE_LISTENING,
		},
		// The time (in seconds) between individual keepalive probes.
		&envoy_config_core_v3.SocketOption{
			Description: "TCP keep-alive time between probes",
			Level:       envoy.IPPROTO_TCP,
			Name:        envoy.TCP_KEEPINTVL,
			Value:       &envoy_config_core_v3.SocketOption_IntValue{IntValue: 5},
			State:       envoy_config_core_v3.SocketOption_STATE_LISTENING,
		},
		// The maximum number of TCP keep-alive probes to send before
		// giving up and killing the connection if no response is
		// obtained from the other end.
		&envoy_config_core_v3.SocketOption{
			Description: "TCP keep-alive probe count",
			Level:       envoy.IPPROTO_TCP,
			Name:        envoy.TCP_KEEPCNT,
			Value:       &envoy_config_core_v3.SocketOption_IntValue{IntValue: 9},
			State:       envoy_config_core_v3.SocketOption_STATE_LISTENING,
		},
	)

	return so
}

// TOS sets the IP_TOS socket option for IPv4 socket.
func (so *SocketOptions) TOS(value int32) *SocketOptions {
	if value != 0 {
		so.options = append(so.options,
			&envoy_config_core_v3.SocketOption{
				Description: "Set IPv4 TOS field",
				Level:       envoy.IPPROTO_IP,
				Name:        envoy.IP_TOS,
				State:       envoy_config_core_v3.SocketOption_STATE_LISTENING,
				Value:       &envoy_config_core_v3.SocketOption_IntValue{IntValue: int64(value)},
			})
	}
	return so
}

// TrafficClass sets the IPV6_TCLASS socket option for IPv4 socket.
func (so *SocketOptions) TrafficClass(value int32) *SocketOptions {
	if value != 0 {
		so.options = append(so.options,
			&envoy_config_core_v3.SocketOption{
				Description: "Set IPv6 Traffic Class field",
				Level:       envoy.IPPROTO_IPV6,
				Name:        envoy.IPV6_TCLASS,
				State:       envoy_config_core_v3.SocketOption_STATE_LISTENING,
				Value:       &envoy_config_core_v3.SocketOption_IntValue{IntValue: int64(value)},
			})
	}
	return so
}

func (so *SocketOptions) Build() []*envoy_config_core_v3.SocketOption {
	return so.options
}
