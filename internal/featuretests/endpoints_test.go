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

package featuretests

import (
	"testing"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	projcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/envoy"
	"github.com/projectcontour/contour/internal/fixture"
	v1 "k8s.io/api/core/v1"
)

// test that adding and removing endpoints don't leave objects
// in the eds cache.
func TestAddRemoveEndpoints(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	rh.OnAdd(fixture.NewService("super-long-namespace-name-oh-boy/what-a-descriptive-service-name-you-must-be-so-proud").
		WithPorts(v1.ServicePort{Name: "https", Port: 8443},
			v1.ServicePort{Name: "http", Port: 8000}),
	)

	rh.OnAdd(fixture.NewProxy("super-long-namespace-name-oh-boy/proxy").
		WithFQDN("proxy.example.com").
		WithSpec(projcontour.HTTPProxySpec{
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: "what-a-descriptive-service-name-you-must-be-so-proud",
					Port: 8000,
				}, {
					Name: "what-a-descriptive-service-name-you-must-be-so-proud",
					Port: 8443,
				}},
			}},
		}),
	)

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
	c.Request(endpointType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			&v2.ClusterLoadAssignment{
				ClusterName: "super-long-namespace-name-oh-boy/what-a-descriptive-service-name-you-must-be-so-proud/http",
				Endpoints: envoy.WeightedEndpoints(1,
					envoy.SocketAddress("172.16.0.1", 8000), // endpoints and cluster names should be sorted
					envoy.SocketAddress("172.16.0.2", 8000),
				),
			},
			&v2.ClusterLoadAssignment{
				ClusterName: "super-long-namespace-name-oh-boy/what-a-descriptive-service-name-you-must-be-so-proud/https",
				Endpoints: envoy.WeightedEndpoints(1,
					envoy.SocketAddress("172.16.0.1", 8443),
					envoy.SocketAddress("172.16.0.2", 8443),
				),
			},
		),
		TypeUrl: endpointType,
	})

	// remove e1 and check that the EDS cache is now empty.
	rh.OnDelete(e1)

	c.Request(endpointType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.ClusterLoadAssignment("super-long-namespace-name-oh-boy/what-a-descriptive-service-name-you-must-be-so-proud/http"),
			envoy.ClusterLoadAssignment("super-long-namespace-name-oh-boy/what-a-descriptive-service-name-you-must-be-so-proud/https"),
		),
		TypeUrl: endpointType,
	})
}

func TestAddEndpointComplicated(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	rh.OnAdd(fixture.NewService("kuard").
		WithPorts(v1.ServicePort{Name: "foo", Port: 8080},
			v1.ServicePort{Name: "admin", Port: 9000}),
	)

	rh.OnAdd(fixture.NewProxy("kuard").
		WithFQDN("kuard.example.com").
		WithSpec(projcontour.HTTPProxySpec{
			Routes: []projcontour.Route{{
				Conditions: []projcontour.MatchCondition{{
					Prefix: "/foo",
				}},
				Services: []projcontour.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}, {
				Conditions: []projcontour.MatchCondition{{
					Prefix: "/admin",
				}},
				Services: []projcontour.Service{{
					Name: "kuard",
					Port: 9000,
				}},
			}},
		}),
	)

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

	c.Request(endpointType).Equals(&v2.DiscoveryResponse{
		TypeUrl: endpointType,
		Resources: resources(t,
			&v2.ClusterLoadAssignment{
				ClusterName: "default/kuard/admin",
				Endpoints: envoy.WeightedEndpoints(1,
					envoy.SocketAddress("10.48.1.77", 9000),
					envoy.SocketAddress("10.48.1.78", 9000),
				),
			},
			&v2.ClusterLoadAssignment{
				ClusterName: "default/kuard/foo",
				Endpoints: envoy.WeightedEndpoints(1,
					envoy.SocketAddress("10.48.1.78", 8080),
				),
			},
		),
	})
}

func TestEndpointFilter(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	rh.OnAdd(fixture.NewService("default/kuard").WithPorts(
		v1.ServicePort{Name: "foo", Port: 8080},
		v1.ServicePort{Name: "admin", Port: 9000},
	))

	rh.OnAdd(fixture.NewProxy("default/kuard").
		WithFQDN("kuard.example.com").
		WithSpec(projcontour.HTTPProxySpec{
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		}),
	)

	// a single endpoint that represents several
	// cluster load assignments.
	rh.OnAdd(endpoints(
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
	))

	c.Request(endpointType, "default/kuard/foo").Equals(&v2.DiscoveryResponse{
		TypeUrl: endpointType,
		Resources: resources(t,
			&v2.ClusterLoadAssignment{
				ClusterName: "default/kuard/foo",
				Endpoints:   envoy.WeightedEndpoints(1, envoy.SocketAddress("10.48.1.78", 8080)),
			},
		),
	})

	c.Request(endpointType, "default/kuard/bar").Equals(&v2.DiscoveryResponse{
		TypeUrl: endpointType,
		Resources: resources(t,
			envoy.ClusterLoadAssignment("default/kuard/bar"),
		),
	})
}

// issue 602, test that an update from N endpoints
// to zero endpoints is handled correctly.
func TestIssue602(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	rh.OnAdd(fixture.NewService("simple").WithPorts(
		v1.ServicePort{Port: 8080},
	))

	rh.OnAdd(fixture.NewProxy("simple").
		WithFQDN("simple.example.com").
		WithSpec(projcontour.HTTPProxySpec{
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: "simple",
					Port: 8080,
				}},
			}},
		}),
	)

	e1 := endpoints("default", "simple", v1.EndpointSubset{
		Addresses: addresses("192.168.183.24"),
		Ports: ports(
			port("", 8080),
		),
	})
	rh.OnAdd(e1)

	// Assert endpoint was added
	c.Request(endpointType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			&v2.ClusterLoadAssignment{
				ClusterName: "default/simple",
				Endpoints:   envoy.WeightedEndpoints(1, envoy.SocketAddress("192.168.183.24", 8080)),
			},
		),
		TypeUrl: endpointType,
	})

	// e2 is the same as e1, but without endpoint subsets
	e2 := endpoints("default", "simple")
	rh.OnUpdate(e1, e2)

	c.Request(endpointType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t, envoy.ClusterLoadAssignment("default/simple")),
		TypeUrl:   endpointType,
	})
}
