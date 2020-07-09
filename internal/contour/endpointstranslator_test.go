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

package contour

import (
	"testing"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/golang/protobuf/proto"
	"github.com/projectcontour/contour/internal/assert"
	"github.com/projectcontour/contour/internal/envoy"
	v1 "k8s.io/api/core/v1"
)

func TestEndpointsTranslatorContents(t *testing.T) {
	tests := map[string]struct {
		contents map[string]*v2.ClusterLoadAssignment
		want     []proto.Message
	}{
		"empty": {
			contents: nil,
			want:     nil,
		},
		"simple": {
			contents: clusterloadassignments(
				envoy.ClusterLoadAssignment("default/httpbin-org",
					envoy.SocketAddress("10.10.10.10", 80),
				),
			),
			want: []proto.Message{
				envoy.ClusterLoadAssignment("default/httpbin-org",
					envoy.SocketAddress("10.10.10.10", 80),
				),
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var et EndpointsTranslator
			et.entries = tc.contents
			got := et.Contents()
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestEndpointCacheQuery(t *testing.T) {
	tests := map[string]struct {
		contents map[string]*v2.ClusterLoadAssignment
		query    []string
		want     []proto.Message
	}{
		"exact match": {
			contents: clusterloadassignments(
				envoy.ClusterLoadAssignment("default/httpbin-org",
					envoy.SocketAddress("10.10.10.10", 80),
				),
			),
			query: []string{"default/httpbin-org"},
			want: []proto.Message{
				envoy.ClusterLoadAssignment("default/httpbin-org",
					envoy.SocketAddress("10.10.10.10", 80),
				),
			},
		},
		"partial match": {
			contents: clusterloadassignments(
				envoy.ClusterLoadAssignment("default/httpbin-org",
					envoy.SocketAddress("10.10.10.10", 80),
				),
			),
			query: []string{"default/kuard/8080", "default/httpbin-org"},
			want: []proto.Message{
				envoy.ClusterLoadAssignment("default/httpbin-org",
					envoy.SocketAddress("10.10.10.10", 80),
				),
				envoy.ClusterLoadAssignment("default/kuard/8080"),
			},
		},
		"no match": {
			contents: clusterloadassignments(
				envoy.ClusterLoadAssignment("default/httpbin-org",
					envoy.SocketAddress("10.10.10.10", 80),
				),
			),
			query: []string{"default/kuard/8080"},
			want: []proto.Message{
				envoy.ClusterLoadAssignment("default/kuard/8080"),
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var et EndpointsTranslator
			et.entries = tc.contents
			got := et.Query(tc.query)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestEndpointsTranslatorAddEndpoints(t *testing.T) {
	tests := map[string]struct {
		ep   *v1.Endpoints
		want []proto.Message
	}{
		"simple": {
			ep: endpoints("default", "simple", v1.EndpointSubset{
				Addresses: addresses("192.168.183.24"),
				Ports: ports(
					port("", 8080),
				),
			}),
			want: []proto.Message{
				envoy.ClusterLoadAssignment("default/simple", envoy.SocketAddress("192.168.183.24", 8080)),
			},
		},
		"multiple addresses": {
			ep: endpoints("default", "httpbin-org", v1.EndpointSubset{
				Addresses: addresses(
					"50.17.192.147",
					"50.17.206.192",
					"50.19.99.160",
					"23.23.247.89",
				),
				Ports: ports(
					port("", 80),
				),
			}),
			want: []proto.Message{
				envoy.ClusterLoadAssignment("default/httpbin-org",
					envoy.SocketAddress("23.23.247.89", 80), // addresses should be sorted
					envoy.SocketAddress("50.17.192.147", 80),
					envoy.SocketAddress("50.17.206.192", 80),
					envoy.SocketAddress("50.19.99.160", 80),
				),
			},
		},
		"multiple ports": {
			ep: endpoints("default", "httpbin-org", v1.EndpointSubset{
				Addresses: addresses(
					"10.10.1.1",
				),
				Ports: ports(
					port("b", 309),
					port("a", 8675),
				),
			}),
			want: []proto.Message{
				envoy.ClusterLoadAssignment("default/httpbin-org/a", // cluster names should be sorted
					envoy.SocketAddress("10.10.1.1", 8675),
				),
				envoy.ClusterLoadAssignment("default/httpbin-org/b",
					envoy.SocketAddress("10.10.1.1", 309),
				),
			},
		},
		"cartesian product": {
			ep: endpoints("default", "httpbin-org", v1.EndpointSubset{
				Addresses: addresses(
					"10.10.2.2",
					"10.10.1.1",
				),
				Ports: ports(
					port("b", 309),
					port("a", 8675),
				),
			}),
			want: []proto.Message{
				envoy.ClusterLoadAssignment("default/httpbin-org/a",
					envoy.SocketAddress("10.10.1.1", 8675), // addresses should be sorted
					envoy.SocketAddress("10.10.2.2", 8675),
				),
				envoy.ClusterLoadAssignment("default/httpbin-org/b",
					envoy.SocketAddress("10.10.1.1", 309),
					envoy.SocketAddress("10.10.2.2", 309),
				),
			},
		},
		"not ready": {
			ep: endpoints("default", "httpbin-org", v1.EndpointSubset{
				Addresses: addresses(
					"10.10.1.1",
				),
				NotReadyAddresses: addresses(
					"10.10.2.2",
				),
				Ports: ports(
					port("a", 8675),
				),
			}, v1.EndpointSubset{
				Addresses: addresses(
					"10.10.2.2",
					"10.10.1.1",
				),
				Ports: ports(
					port("b", 309),
				),
			}),
			want: []proto.Message{
				envoy.ClusterLoadAssignment("default/httpbin-org/a",
					envoy.SocketAddress("10.10.1.1", 8675),
				),
				envoy.ClusterLoadAssignment("default/httpbin-org/b",
					envoy.SocketAddress("10.10.1.1", 309),
					envoy.SocketAddress("10.10.2.2", 309),
				),
			},
		},
	}

	log := testLogger(t)
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			et := &EndpointsTranslator{
				FieldLogger: log,
			}
			et.OnAdd(tc.ep)
			got := et.Contents()
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestEndpointsTranslatorRemoveEndpoints(t *testing.T) {
	tests := map[string]struct {
		setup func(*EndpointsTranslator)
		ep    *v1.Endpoints
		want  []proto.Message
	}{
		"remove existing": {
			setup: func(et *EndpointsTranslator) {
				et.OnAdd(endpoints("default", "simple", v1.EndpointSubset{
					Addresses: addresses("192.168.183.24"),
					Ports: ports(
						port("", 8080),
					),
				}))
			},
			ep: endpoints("default", "simple", v1.EndpointSubset{
				Addresses: addresses("192.168.183.24"),
				Ports: ports(
					port("", 8080),
				),
			}),
			want: nil,
		},
		"remove different": {
			setup: func(et *EndpointsTranslator) {
				et.OnAdd(endpoints("default", "simple", v1.EndpointSubset{
					Addresses: addresses("192.168.183.24"),
					Ports: ports(
						port("", 8080),
					),
				}))
			},
			ep: endpoints("default", "different", v1.EndpointSubset{
				Addresses: addresses("192.168.183.24"),
				Ports: ports(
					port("", 8080),
				),
			}),
			want: []proto.Message{
				envoy.ClusterLoadAssignment("default/simple", envoy.SocketAddress("192.168.183.24", 8080)),
			},
		},
		"remove non existent": {
			setup: func(*EndpointsTranslator) {},
			ep: endpoints("default", "simple", v1.EndpointSubset{
				Addresses: addresses("192.168.183.24"),
				Ports: ports(
					port("", 8080),
				),
			}),
			want: nil,
		},
		"remove long name": {
			setup: func(et *EndpointsTranslator) {
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
							port("http", 8080),
						),
					},
				)
				et.OnAdd(e1)
			},
			ep: endpoints(
				"super-long-namespace-name-oh-boy",
				"what-a-descriptive-service-name-you-must-be-so-proud",
				v1.EndpointSubset{
					Addresses: addresses(
						"172.16.0.2",
						"172.16.0.1",
					),
					Ports: ports(
						port("https", 8443),
						port("http", 8080),
					),
				},
			),
			want: nil,
		},
	}

	log := testLogger(t)
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			et := &EndpointsTranslator{
				FieldLogger: log,
			}
			tc.setup(et)
			et.OnDelete(tc.ep)
			got := et.Contents()
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestEndpointsTranslatorRecomputeClusterLoadAssignment(t *testing.T) {
	tests := map[string]struct {
		oldep, newep *v1.Endpoints
		want         []proto.Message
	}{
		"simple": {
			newep: endpoints("default", "simple", v1.EndpointSubset{
				Addresses: addresses("192.168.183.24"),
				Ports: ports(
					port("", 8080),
				),
			}),
			want: []proto.Message{
				envoy.ClusterLoadAssignment("default/simple", envoy.SocketAddress("192.168.183.24", 8080)),
			},
		},
		"multiple addresses": {
			newep: endpoints("default", "httpbin-org", v1.EndpointSubset{
				Addresses: addresses(
					"50.17.192.147",
					"23.23.247.89",
					"50.17.206.192",
					"50.19.99.160",
				),
				Ports: ports(
					port("", 80),
				),
			}),
			want: []proto.Message{
				envoy.ClusterLoadAssignment("default/httpbin-org",
					envoy.SocketAddress("23.23.247.89", 80),
					envoy.SocketAddress("50.17.192.147", 80),
					envoy.SocketAddress("50.17.206.192", 80),
					envoy.SocketAddress("50.19.99.160", 80),
				),
			},
		},
		"named container port": {
			newep: endpoints("default", "secure", v1.EndpointSubset{
				Addresses: addresses("192.168.183.24"),
				Ports: ports(
					port("https", 8443),
				),
			}),
			want: []proto.Message{
				envoy.ClusterLoadAssignment("default/secure/https", envoy.SocketAddress("192.168.183.24", 8443)),
			},
		},
		"remove existing": {
			oldep: endpoints("default", "simple", v1.EndpointSubset{
				Addresses: addresses("192.168.183.24"),
				Ports: ports(
					port("", 8080),
				),
			}),
			want: nil,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var et EndpointsTranslator
			et.recomputeClusterLoadAssignment(tc.oldep, tc.newep)
			got := et.Contents()
			assert.Equal(t, tc.want, got)
		})
	}
}

// See #602
func TestEndpointsTranslatorScaleToZeroEndpoints(t *testing.T) {
	var et EndpointsTranslator
	e1 := endpoints("default", "simple", v1.EndpointSubset{
		Addresses: addresses("192.168.183.24"),
		Ports: ports(
			port("", 8080),
		),
	})
	et.OnAdd(e1)

	// Assert endpoint was added
	want := []proto.Message{
		envoy.ClusterLoadAssignment("default/simple", envoy.SocketAddress("192.168.183.24", 8080)),
	}
	got := et.Contents()

	assert.Equal(t, want, got)

	// e2 is the same as e1, but without endpoint subsets
	e2 := endpoints("default", "simple")
	et.OnUpdate(e1, e2)

	// Assert endpoints are removed
	want = nil
	got = et.Contents()

	assert.Equal(t, want, got)
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

func clusterloadassignments(clas ...*v2.ClusterLoadAssignment) map[string]*v2.ClusterLoadAssignment {
	m := make(map[string]*v2.ClusterLoadAssignment)
	for _, cla := range clas {
		m[cla.ClusterName] = cla
	}
	return m
}
