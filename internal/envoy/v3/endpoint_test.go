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

	envoy_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"github.com/projectcontour/contour/internal/protobuf"
)

func TestLBEndpoint(t *testing.T) {
	got := LBEndpoint(SocketAddress("microsoft.com", 81))
	want := &envoy_endpoint_v3.LbEndpoint{
		HostIdentifier: &envoy_endpoint_v3.LbEndpoint_Endpoint{
			Endpoint: &envoy_endpoint_v3.Endpoint{
				Address: SocketAddress("microsoft.com", 81),
			},
		},
	}
	protobuf.ExpectEqual(t, want, got)
}
