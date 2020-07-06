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

package e2e

import (
	"context"
	"testing"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/projectcontour/contour/internal/assert"
	"github.com/projectcontour/contour/internal/envoy"
	"google.golang.org/grpc"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// test that adding and removing endpoints don't leave turds
// in the eds cache.
func TestAddRemoveEndpoints(t *testing.T) {
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
				"172.16.0.2",
				"172.16.0.1",
			),
			Ports: ports(
				port("https", 8443),
				port("http", 8000),
			),
		},
	)

	rh.OnAdd(e1)

	// check that it's been translated correctly.
	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "2",
		Resources: resources(t,
			envoy.ClusterLoadAssignment(
				"super-long-namespace-name-oh-boy/what-a-descriptive-service-name-you-must-be-so-proud/http",
				envoy.SocketAddress("172.16.0.1", 8000), // endpoints and cluster names should be sorted
				envoy.SocketAddress("172.16.0.2", 8000),
			),
			envoy.ClusterLoadAssignment(
				"super-long-namespace-name-oh-boy/what-a-descriptive-service-name-you-must-be-so-proud/https",
				envoy.SocketAddress("172.16.0.1", 8443),
				envoy.SocketAddress("172.16.0.2", 8443),
			),
		),
		TypeUrl: endpointType,
		Nonce:   "2",
	}, streamEDS(t, cc))

	// remove e1 and check that the EDS cache is now empty.
	rh.OnDelete(e1)

	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "4",
		Resources:   resources(t),
		TypeUrl:     endpointType,
		Nonce:       "4",
	}, streamEDS(t, cc))
}

func TestAddEndpointComplicated(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	e1 := endpoints(
		"default",
		"kuard",
		v1.EndpointSubset{
			Addresses: addresses(
				"10.48.1.78",
			),
			NotReadyAddresses: addresses(
				"10.48.1.77",
			),
			Ports: ports(
				port("foo", 8080),
			),
		},
		v1.EndpointSubset{
			Addresses: addresses(
				"10.48.1.78",
				"10.48.1.77",
			),
			Ports: ports(
				port("admin", 9000),
			),
		},
	)

	rh.OnAdd(e1)

	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "2",
		Resources: resources(t,
			envoy.ClusterLoadAssignment(
				"default/kuard/admin",
				envoy.SocketAddress("10.48.1.77", 9000),
				envoy.SocketAddress("10.48.1.78", 9000),
			),
			envoy.ClusterLoadAssignment(
				"default/kuard/foo",
				envoy.SocketAddress("10.48.1.78", 8080),
			),
		),
		TypeUrl: endpointType,
		Nonce:   "2",
	}, streamEDS(t, cc))
}

func TestEndpointFilter(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	// a single endpoint that represents several
	// cluster load assignments.
	e1 := endpoints(
		"default",
		"kuard",
		v1.EndpointSubset{
			Addresses: addresses(
				"10.48.1.78",
			),
			NotReadyAddresses: addresses(
				"10.48.1.77",
			),
			Ports: ports(
				port("foo", 8080),
			),
		},
		v1.EndpointSubset{
			Addresses: addresses(
				"10.48.1.77",
				"10.48.1.78",
			),
			Ports: ports(
				port("admin", 9000),
			),
		},
	)

	rh.OnAdd(e1)

	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "2",
		Resources: resources(t,
			envoy.ClusterLoadAssignment(
				"default/kuard/foo",
				envoy.SocketAddress("10.48.1.78", 8080),
			),
		),
		TypeUrl: endpointType,
		Nonce:   "2",
	}, streamEDS(t, cc, "default/kuard/foo"))

	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "2",
		TypeUrl:     endpointType,
		Resources: resources(t,
			envoy.ClusterLoadAssignment("default/kuard/bar"),
		),
		Nonce: "2",
	}, streamEDS(t, cc, "default/kuard/bar"))

}

// issue 602, test that an update from N endpoints
// to zero endpoints is handled correctly.
func TestIssue602(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	e1 := endpoints("default", "simple", v1.EndpointSubset{
		Addresses: addresses("192.168.183.24"),
		Ports: ports(
			port("", 8080),
		),
	})
	rh.OnAdd(e1)

	// Assert endpoint was added
	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "1",
		Resources: resources(t,
			envoy.ClusterLoadAssignment("default/simple", envoy.SocketAddress("192.168.183.24", 8080)),
		),
		TypeUrl: endpointType,
		Nonce:   "1",
	}, streamEDS(t, cc))

	// e2 is the same as e1, but without endpoint subsets
	e2 := endpoints("default", "simple")
	rh.OnUpdate(e1, e2)

	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "2",
		Resources:   resources(t),
		TypeUrl:     endpointType,
		Nonce:       "2",
	}, streamEDS(t, cc))
}

func streamEDS(t *testing.T, cc *grpc.ClientConn, rn ...string) *v2.DiscoveryResponse {
	t.Helper()
	rds := v2.NewEndpointDiscoveryServiceClient(cc)
	st, err := rds.StreamEndpoints(context.TODO())
	check(t, err)
	return stream(t, st, &v2.DiscoveryRequest{
		TypeUrl:       endpointType,
		ResourceNames: rn,
	})
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

func ports(eps ...v1.EndpointPort) []v1.EndpointPort {
	return eps
}

func port(name string, port int32) v1.EndpointPort {
	return v1.EndpointPort{
		Name:     name,
		Port:     port,
		Protocol: "TCP",
	}
}

func addresses(ips ...string) []v1.EndpointAddress {
	var addrs []v1.EndpointAddress
	for _, ip := range ips {
		addrs = append(addrs, v1.EndpointAddress{IP: ip})
	}
	return addrs
}
