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

	envoy_config_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	core_v1 "k8s.io/api/core/v1"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/featuretests"
	"github.com/projectcontour/contour/internal/fixture"
)

// test that adding and removing endpoints don't leave objects
// in the eds cache.
func TestAddRemoveEndpoints(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	rh.OnAdd(fixture.NewService("super-long-namespace-name-oh-boy/what-a-descriptive-service-name-you-must-be-so-proud").
		WithPorts(core_v1.ServicePort{Name: "https", Port: 8443},
			core_v1.ServicePort{Name: "http", Port: 8000}),
	)

	rh.OnAdd(fixture.NewProxy("super-long-namespace-name-oh-boy/proxy").
		WithFQDN("proxy.example.com").
		WithSpec(contour_v1.HTTPProxySpec{
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
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
	e1 := featuretests.Endpoints(
		"super-long-namespace-name-oh-boy",
		"what-a-descriptive-service-name-you-must-be-so-proud",
		core_v1.EndpointSubset{
			Addresses: featuretests.Addresses(
				"172.16.0.2",
				"172.16.0.1",
			),
			Ports: featuretests.Ports(
				featuretests.Port("https", 8443),
				featuretests.Port("http", 8000),
			),
		},
	)

	rh.OnAdd(e1)

	// check that it's been translated correctly.
	c.Request(endpointType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			&envoy_config_endpoint_v3.ClusterLoadAssignment{
				ClusterName: "super-long-namespace-name-oh-boy/what-a-descriptive-service-name-you-must-be-so-proud/http",
				Endpoints: envoy_v3.WeightedEndpoints(1,
					envoy_v3.SocketAddress("172.16.0.1", 8000), // endpoints and cluster names should be sorted
					envoy_v3.SocketAddress("172.16.0.2", 8000),
				),
			},
			&envoy_config_endpoint_v3.ClusterLoadAssignment{
				ClusterName: "super-long-namespace-name-oh-boy/what-a-descriptive-service-name-you-must-be-so-proud/https",
				Endpoints: envoy_v3.WeightedEndpoints(1,
					envoy_v3.SocketAddress("172.16.0.1", 8443),
					envoy_v3.SocketAddress("172.16.0.2", 8443),
				),
			},
		),
		TypeUrl: endpointType,
	})

	// remove e1 and check that the EDS cache is now empty.
	rh.OnDelete(e1)

	c.Request(endpointType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.ClusterLoadAssignment("super-long-namespace-name-oh-boy/what-a-descriptive-service-name-you-must-be-so-proud/http"),
			envoy_v3.ClusterLoadAssignment("super-long-namespace-name-oh-boy/what-a-descriptive-service-name-you-must-be-so-proud/https"),
		),
		TypeUrl: endpointType,
	})
}

func TestAddEndpointComplicated(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	rh.OnAdd(fixture.NewService("kuard").
		WithPorts(core_v1.ServicePort{Name: "foo", Port: 8080},
			core_v1.ServicePort{Name: "admin", Port: 9000}),
	)

	rh.OnAdd(fixture.NewProxy("kuard").
		WithFQDN("kuard.example.com").
		WithSpec(contour_v1.HTTPProxySpec{
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/foo",
				}},
				Services: []contour_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}, {
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/admin",
				}},
				Services: []contour_v1.Service{{
					Name: "kuard",
					Port: 9000,
				}},
			}},
		}),
	)

	e1 := featuretests.Endpoints(
		"default",
		"kuard",
		core_v1.EndpointSubset{
			Addresses: featuretests.Addresses(
				"10.48.1.78",
			),
			NotReadyAddresses: featuretests.Addresses(
				"10.48.1.77",
			),
			Ports: featuretests.Ports(
				featuretests.Port("foo", 8080),
			),
		},
		core_v1.EndpointSubset{
			Addresses: featuretests.Addresses(
				"10.48.1.78",
				"10.48.1.77",
			),
			Ports: featuretests.Ports(
				featuretests.Port("admin", 9000),
			),
		},
	)

	rh.OnAdd(e1)

	c.Request(endpointType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: endpointType,
		Resources: resources(t,
			&envoy_config_endpoint_v3.ClusterLoadAssignment{
				ClusterName: "default/kuard/admin",
				Endpoints: envoy_v3.WeightedEndpoints(1,
					envoy_v3.SocketAddress("10.48.1.77", 9000),
					envoy_v3.SocketAddress("10.48.1.78", 9000),
				),
			},
			&envoy_config_endpoint_v3.ClusterLoadAssignment{
				ClusterName: "default/kuard/foo",
				Endpoints: envoy_v3.WeightedEndpoints(1,
					envoy_v3.SocketAddress("10.48.1.78", 8080),
				),
			},
		),
	})
}

func TestEndpointFilter(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	rh.OnAdd(fixture.NewService("default/kuard").WithPorts(
		core_v1.ServicePort{Name: "foo", Port: 8080},
		core_v1.ServicePort{Name: "admin", Port: 9000},
	))

	rh.OnAdd(fixture.NewProxy("default/kuard").
		WithFQDN("kuard.example.com").
		WithSpec(contour_v1.HTTPProxySpec{
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		}),
	)

	// a single endpoint that represents several
	// cluster load assignments.
	rh.OnAdd(featuretests.Endpoints(
		"default",
		"kuard",
		core_v1.EndpointSubset{
			Addresses: featuretests.Addresses(
				"10.48.1.78",
			),
			NotReadyAddresses: featuretests.Addresses(
				"10.48.1.77",
			),
			Ports: featuretests.Ports(
				featuretests.Port("foo", 8080),
			),
		},
		core_v1.EndpointSubset{
			Addresses: featuretests.Addresses(
				"10.48.1.77",
				"10.48.1.78",
			),
			Ports: featuretests.Ports(
				featuretests.Port("admin", 9000),
			),
		},
	))

	c.Request(endpointType, "default/kuard/foo").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: endpointType,
		Resources: resources(t,
			&envoy_config_endpoint_v3.ClusterLoadAssignment{
				ClusterName: "default/kuard/foo",
				Endpoints:   envoy_v3.WeightedEndpoints(1, envoy_v3.SocketAddress("10.48.1.78", 8080)),
			},
		),
	})

	// Nonexistent endpoint shouldn't return anything.
	c.Request(endpointType, "default/kuard/bar").Equals(&envoy_service_discovery_v3.DiscoveryResponse{})
}

// issue 602, test that an update from N endpoints
// to zero endpoints is handled correctly.
func TestIssue602(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	rh.OnAdd(fixture.NewService("simple").WithPorts(
		core_v1.ServicePort{Port: 8080},
	))

	rh.OnAdd(fixture.NewProxy("simple").
		WithFQDN("simple.example.com").
		WithSpec(contour_v1.HTTPProxySpec{
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: "simple",
					Port: 8080,
				}},
			}},
		}),
	)

	e1 := featuretests.Endpoints("default", "simple", core_v1.EndpointSubset{
		Addresses: featuretests.Addresses("192.168.183.24"),
		Ports: featuretests.Ports(
			featuretests.Port("", 8080),
		),
	})
	rh.OnAdd(e1)

	// Assert endpoint was added
	c.Request(endpointType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			&envoy_config_endpoint_v3.ClusterLoadAssignment{
				ClusterName: "default/simple",
				Endpoints:   envoy_v3.WeightedEndpoints(1, envoy_v3.SocketAddress("192.168.183.24", 8080)),
			},
		),
		TypeUrl: endpointType,
	})

	// e2 is the same as e1, but without endpoint subsets
	e2 := featuretests.Endpoints("default", "simple")
	rh.OnUpdate(e1, e2)

	c.Request(endpointType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t, envoy_v3.ClusterLoadAssignment("default/simple")),
		TypeUrl:   endpointType,
	})
}
