// Copyright © 2017 Heptio
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

package contour

import v2 "github.com/envoyproxy/go-control-plane/api"

// ClusterLoadAssignmentCache manage the contents of the gRPC EDS cache.
type ClusterLoadAssignmentCache struct {
	clusterLoadAssignmentCache
	Cond
}

func clusterloadassignment(name string, lbendpoints ...*v2.LbEndpoint) *v2.ClusterLoadAssignment {
	return &v2.ClusterLoadAssignment{
		ClusterName: name,
		Endpoints: []*v2.LocalityLbEndpoints{{
			Locality: &v2.Locality{
				Region:  "ap-southeast-2", // totally a guess
				Zone:    "2b",
				SubZone: "banana", // yeah, need to think of better values here
			},
			LbEndpoints: lbendpoints,
		}},
		Policy: &v2.ClusterLoadAssignment_Policy{
			DropOverload: 0.0,
		},
	}
}

func lbendpoint(addr string, port int32) *v2.LbEndpoint {
	return &v2.LbEndpoint{
		Endpoint: endpoint(addr, port),
	}
}

func endpoint(addr string, port int32) *v2.Endpoint {
	return &v2.Endpoint{
		Address: &v2.Address{
			Address: &v2.Address_SocketAddress{
				SocketAddress: &v2.SocketAddress{
					Protocol: v2.SocketAddress_TCP,
					Address:  addr,
					PortSpecifier: &v2.SocketAddress_PortValue{
						PortValue: uint32(port),
					},
				},
			},
		},
	}
}
