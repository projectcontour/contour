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
	envoy_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
)

// LBEndpoint creates a new LbEndpoint.
func LBEndpoint(addr *envoy_core_v3.Address) *envoy_endpoint_v3.LbEndpoint {
	return &envoy_endpoint_v3.LbEndpoint{
		HostIdentifier: &envoy_endpoint_v3.LbEndpoint_Endpoint{
			Endpoint: &envoy_endpoint_v3.Endpoint{
				Address: addr,
			},
		},
	}
}

// Endpoints returns a slice of LocalityLbEndpoints.
// The slice contains one entry, with one LbEndpoint per
// *envoy_core_v3.Address supplied.
func Endpoints(addrs ...*envoy_core_v3.Address) []*envoy_endpoint_v3.LocalityLbEndpoints {
	lbendpoints := make([]*envoy_endpoint_v3.LbEndpoint, 0, len(addrs))
	for _, addr := range addrs {
		lbendpoints = append(lbendpoints, LBEndpoint(addr))
	}
	return []*envoy_endpoint_v3.LocalityLbEndpoints{{
		LbEndpoints: lbendpoints,
	}}
}
