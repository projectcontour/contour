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
	"testing"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"github.com/stretchr/testify/assert"

	"github.com/projectcontour/contour/internal/envoy"
)

func TestSocketOptions(t *testing.T) {
	// No options shall be set when value 0 is set.
	so := NewSocketOptions().TOS(0).TrafficClass(0)
	assert.Empty(t, so.options)

	so.TOS(64)
	assert.Equal(t,
		[]*envoy_config_core_v3.SocketOption{
			{
				Description: "Set IPv4 TOS field",
				Level:       envoy.IPPROTO_IP,
				Name:        envoy.IP_TOS,
				Value:       &envoy_config_core_v3.SocketOption_IntValue{IntValue: 64},
				State:       envoy_config_core_v3.SocketOption_STATE_LISTENING,
			},
		},
		so.Build(),
	)

	so.TrafficClass(64)
	assert.Equal(t,
		[]*envoy_config_core_v3.SocketOption{
			{
				Description: "Set IPv4 TOS field",
				Level:       envoy.IPPROTO_IP,
				Name:        envoy.IP_TOS,
				Value:       &envoy_config_core_v3.SocketOption_IntValue{IntValue: 64},
				State:       envoy_config_core_v3.SocketOption_STATE_LISTENING,
			},
			{
				Description: "Set IPv6 Traffic Class field",
				Level:       envoy.IPPROTO_IPV6,
				Name:        envoy.IPV6_TCLASS,
				Value:       &envoy_config_core_v3.SocketOption_IntValue{IntValue: 64},
				State:       envoy_config_core_v3.SocketOption_STATE_LISTENING,
			},
		},
		so.Build(),
	)
}
