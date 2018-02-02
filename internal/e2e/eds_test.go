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

package e2e

import (
	"context"
	"testing"
	"time"

	v2 "github.com/envoyproxy/go-control-plane/api"
	"github.com/gogo/protobuf/types"
	cgrpc "github.com/heptio/contour/internal/grpc"
	"google.golang.org/grpc"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// test that adding, updating, and removing endpoints don't leave turds
// in the eds cache.
func TestAddUpdateRemoveEndpoints(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	// e1 is a simple endpoint for two hosts, and two ports
	// it has a long name to check that it's clustername is _not_
	// hashed.
	e1 := endpoints(
		"super-long-namespace-name-oh-boy",
		"what-a-descriptive-service-name-you-must-be-so-proud",
		v1.EndpointSubset{
			Addresses: addresses(
				"172.16.0.1",
				"172.16.0.2",
			),
			Ports: ports(8000, 8443),
		},
	)

	rh.OnAdd(e1)

	// check that it's been translated correctly.
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []*types.Any{
			any(t, clusterloadassignment(
				"super-long-namespace-name-oh-boy/what-a-descriptive-service-name-you-must-be-so-proud/8000",
				lbendpoint("172.16.0.1", 8000),
				lbendpoint("172.16.0.2", 8000),
			)),
			any(t, clusterloadassignment(
				"super-long-namespace-name-oh-boy/what-a-descriptive-service-name-you-must-be-so-proud/8443",
				lbendpoint("172.16.0.1", 8443),
				lbendpoint("172.16.0.2", 8443),
			)),
		},
		TypeUrl: cgrpc.EndpointType,
		Nonce:   "0",
	}, fetchEDS(t, cc))

	// remove e1 and check that the EDS cache is now empty.
	rh.OnDelete(e1)

	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources:   []*types.Any{},
		TypeUrl:     cgrpc.EndpointType,
		Nonce:       "0",
	}, fetchEDS(t, cc))
}

func fetchEDS(t *testing.T, cc *grpc.ClientConn) *v2.DiscoveryResponse {
	t.Helper()
	rds := v2.NewEndpointDiscoveryServiceClient(cc)
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	resp, err := rds.FetchEndpoints(ctx, new(v2.DiscoveryRequest))
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func endpoints(ns, name string, subsets ...v1.EndpointSubset) *v1.Endpoints {
	return &v1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Subsets: subsets,
	}
}

func addresses(ips ...string) []v1.EndpointAddress {
	var addrs []v1.EndpointAddress
	for _, ip := range ips {
		addrs = append(addrs, v1.EndpointAddress{IP: ip})
	}
	return addrs
}

func ports(ps ...int32) []v1.EndpointPort {
	var ports []v1.EndpointPort
	for _, p := range ps {
		ports = append(ports, v1.EndpointPort{Port: p})
	}
	return ports
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

func lbendpoint(addr string, port uint32) *v2.LbEndpoint {
	return &v2.LbEndpoint{
		Endpoint: endpoint(addr, port),
	}
}

func endpoint(addr string, port uint32) *v2.Endpoint {
	return &v2.Endpoint{
		Address: &v2.Address{
			Address: &v2.Address_SocketAddress{
				SocketAddress: &v2.SocketAddress{
					Protocol: v2.SocketAddress_TCP,
					Address:  addr,
					PortSpecifier: &v2.SocketAddress_PortValue{
						PortValue: port,
					},
				},
			},
		},
	}
}

func lbendpoints(eps ...*v2.Endpoint) []*v2.LbEndpoint {
	var lbep []*v2.LbEndpoint
	for _, ep := range eps {
		lbep = append(lbep, &v2.LbEndpoint{
			Endpoint: ep,
		})
	}
	return lbep
}
