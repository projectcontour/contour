// Copyright © 2018 Heptio
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

	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
	"github.com/google/go-cmp/cmp"
)

func TestLBEndpoint(t *testing.T) {
	const (
		addr = "foo.example.com"
		port = 8123
	)

	got := LBEndpoint(addr, port)
	want := endpoint.LbEndpoint{
		HostIdentifier: &endpoint.LbEndpoint_Endpoint{
			Endpoint: &endpoint.Endpoint{
				Address: &core.Address{
					Address: &core.Address_SocketAddress{
						SocketAddress: &core.SocketAddress{
							Protocol: core.TCP,
							Address:  addr,
							PortSpecifier: &core.SocketAddress_PortValue{
								PortValue: port,
							},
						},
					},
				},
			},
		},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatal(diff)
	}
}
