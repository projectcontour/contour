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
	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_api_v2_core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoy_api_v2_endpoint "github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
	"github.com/projectcontour/contour/internal/protobuf"
)

// LBEndpoint creates a new LbEndpoint.
func LBEndpoint(addr *envoy_api_v2_core.Address) *envoy_api_v2_endpoint.LbEndpoint {
	return &envoy_api_v2_endpoint.LbEndpoint{
		HostIdentifier: &envoy_api_v2_endpoint.LbEndpoint_Endpoint{
			Endpoint: &envoy_api_v2_endpoint.Endpoint{
				Address: addr,
			},
		},
	}
}

// Endpoints returns a slice of LocalityLbEndpoints.
// The slice contains one entry, with one LbEndpoint per
// *envoy_api_v2_core.Address supplied.
func Endpoints(addrs ...*envoy_api_v2_core.Address) []*envoy_api_v2_endpoint.LocalityLbEndpoints {
	lbendpoints := make([]*envoy_api_v2_endpoint.LbEndpoint, 0, len(addrs))
	for _, addr := range addrs {
		lbendpoints = append(lbendpoints, LBEndpoint(addr))
	}
	return []*envoy_api_v2_endpoint.LocalityLbEndpoints{{
		LbEndpoints: lbendpoints,
	}}
}

func WeightedEndpoints(weight uint32, addrs ...*envoy_api_v2_core.Address) []*envoy_api_v2_endpoint.LocalityLbEndpoints {
	lbendpoints := Endpoints(addrs...)
	lbendpoints[0].LoadBalancingWeight = protobuf.UInt32(weight)
	return lbendpoints
}

// ClusterLoadAssignment returns a *v2.ClusterLoadAssignment with a single
// LocalityLbEndpoints of the supplied addresses.
func ClusterLoadAssignment(name string, addrs ...*envoy_api_v2_core.Address) *v2.ClusterLoadAssignment {
	if len(addrs) == 0 {
		return &v2.ClusterLoadAssignment{ClusterName: name}
	}
	return &v2.ClusterLoadAssignment{
		ClusterName: name,
		Endpoints:   Endpoints(addrs...),
	}
}
