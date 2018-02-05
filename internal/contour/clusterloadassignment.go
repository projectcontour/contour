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

// recomputeClusterLoadAssignment recomputes the EDS cache taking into account old and new services and endpoints.
func (cc *ClusterLoadAssignmentCache) recomputeClusterLoadAssignment(oldsvc, newsvc *v1.Service, oldep, newep *v1.Endpoints) {
	// skip computation if either old and new services or endpoints are equal (thus also handling nil)
	if oldsvc == newsvc || oldep == newep {
		return
	}

	defer cc.Notify()

	// normalise all the paramters
	if oldsvc == nil {
		// if oldsvc is nil, replace it with a blank spec so entries
		// are conditionally added.
		oldsvc = &v1.Service{
			ObjectMeta: newsvc.ObjectMeta,
		}
	}

	if newsvc == nil {
		// if newsvc is nil, replace it with a blank spec so entries
		// are conditionally deleted.
		newsvc = &v1.Service{
			ObjectMeta: oldsvc.ObjectMeta,
		}
	}

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

	// sanity check
	if newep.Name != newsvc.Name || newep.Namespace != newsvc.Namespace {
		panic("service and endpoint document mismatch: service: " + newsvc.Namespace + "/" +
			newsvc.Name + ", endpoint: " + newep.Namespace + "/" + newep.Name)
	}

	services := make(map[string]bool)
	// add or update endpoints
	for _, s := range newep.Subsets {
		// skip any subsets that don't have ready addresses or ports
		if len(s.Addresses) == 0 || len(s.Ports) == 0 {
			continue
		}

		for _, p := range s.Ports {
			for _, sp := range newsvc.Spec.Ports {
				// TODO(dfc) ignore non tcp ports

				// we have the generate a service name that matches the
				// targetPort value in the matching service.
				name := sp.TargetPort.String()
				if p.Name != name {
					if strconv.Itoa(int(p.Port)) != name {
						// this sevice doesn't match and port in the endpoint
						// skip it. We'll only match one svc and port pair.
						continue
					}
				}
				cla := clusterloadassignment(servicename(newep.ObjectMeta, name))
				for _, a := range s.Addresses {
					cla.Endpoints[0].LbEndpoints = append(cla.Endpoints[0].LbEndpoints, &v2.LbEndpoint{
						Endpoint: endpoint(a.IP, p.Port),
					})
				}
				cc.Add(cla)
				services[name] = true
			}
		}
	}

	// remove endpoints that lack a matching endpoint
	for _, p := range oldsvc.Spec.Ports {
		switch p.Protocol {
		case "TCP":
			if !services[p.TargetPort.String()] {
				cc.Remove(servicename(oldsvc.ObjectMeta, p.TargetPort.String()))
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
