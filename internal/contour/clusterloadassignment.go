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

// recomputeClusterLoadAssignment recomputes the EDS cache taking into account old and new endpoints.
func (cc *ClusterLoadAssignmentCache) recomputeClusterLoadAssignment(oldep, newep *v1.Endpoints) {
	// skip computation if either old and new services or endpoints are equal (thus also handling nil)
	if oldep == newep {
		return
	}

	defer cc.Notify()

	if oldep == nil {
		oldep = &v1.Endpoints{
			ObjectMeta: newep.ObjectMeta,
		}
	}

	if newep == nil {
		newep = &v1.Endpoints{
			ObjectMeta: oldep.ObjectMeta,
		}
	}

	clas := make(map[string]*v2.ClusterLoadAssignment)
	// add or update endpoints
	for _, s := range newep.Subsets {
		// skip any subsets that don't have ready addresses
		if len(s.Addresses) == 0 {
			continue
		}

		for _, p := range s.Ports {
			// TODO(dfc) check protocol, don't add UDP enties by mistake

			// if this endpoint's service's port has a name, then the endpoint
			// controller will apply the name here. The name may appear once per subset.
			name := p.Name
			if name == "" {
				// if the port's name is not set then the service's port is unnamed
				// and there is only one port, and it only deploys to an unnamed
				// container port, therefore the name generated for the service in CDS
				// will be the port number.
				name = strconv.Itoa(int(p.Port))
			}
			cla, ok := clas[name]
			if !ok {
				cla = clusterloadassignment(servicename(newep.ObjectMeta, name))
				clas[name] = cla
			}
			for _, a := range s.Addresses {
				cla.Endpoints[0].LbEndpoints = append(cla.Endpoints[0].LbEndpoints, &v2.LbEndpoint{
					Endpoint: endpoint(a.IP, p.Port),
				})
			}
		}
	}

	// iterate all the defined clusters and add or update them.
	for _, c := range clas {
		cc.Add(c)
	}

	// iterate over the ports in the old spec, remove any that are not
	// mentioned in clas
	for _, s := range oldep.Subsets {
		if len(s.Addresses) == 0 {
			continue
		}
		for _, p := range s.Ports {
			// if this endpoint's service's port has a name, then the endpoint
			// controller will apply the name here. The name may appear once per subset.
			name := p.Name
			if name == "" {
				// if the port's name is not set then the service's port is unnamed
				// and there is only one port, and it only deploys to an unnamed
				// container port, therefore the name generated for the service in CDS
				// will be the port number.
				name = strconv.Itoa(int(p.Port))
			}
			if _, ok := clas[name]; !ok {
				// port is not present in the list added / updated, so remove it
				cc.Remove(servicename(oldep.ObjectMeta, name))
			}
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
