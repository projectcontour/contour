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

import (
	"strconv"

	v2 "github.com/envoyproxy/go-control-plane/api"
	"k8s.io/api/core/v1"
)

// ClusterLoadAssignmentCache manage the contents of the gRPC EDS cache.
type ClusterLoadAssignmentCache struct {
	clusterLoadAssignmentCache
	Cond
}

// recomputeClusterLoadAssignment recomputes the EDS cache for newep.
func (cc *ClusterLoadAssignmentCache) recomputeClusterLoadAssignment(oldep, newep *v1.Endpoints) {
	if newep != nil && len(newep.Subsets) < 1 {
		// if there are no endpoints in this object, ignore it
		// to avoid sending a noop notification to watchers.
		return
	}
	defer cc.Notify()
	if newep == nil {
		for _, s := range oldep.Subsets {
			for _, p := range s.Ports {
				cc.Remove(servicename(oldep.ObjectMeta, strconv.Itoa(int(p.Port))))
			}
		}
		return
	}
	for _, s := range newep.Subsets {
		// skip any subsets that don't have ready addresses or ports
		if len(s.Addresses) == 0 || len(s.Ports) == 0 {
			continue
		}

		for _, p := range s.Ports {
			// ClusterName must match Cluster.ServiceName
			// TODO(dfc) an endpoint document may list multiple sets of ports, only some of which may
			// correspond to the specific cluster we're talking about.
			cla := clusterloadassignment(servicename(newep.ObjectMeta, strconv.Itoa(int(p.Port))))
			for _, a := range s.Addresses {
				cla.Endpoints[0].LbEndpoints = append(cla.Endpoints[0].LbEndpoints, &v2.LbEndpoint{
					Endpoint: endpoint(a.IP, p.Port),
				})
			}
			cc.Add(cla)
		}
	}
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

func portname(p v1.EndpointPort) string {
	if p.Name != "" {
		return p.Name
	}
	return strconv.Itoa(int(p.Port))
}
