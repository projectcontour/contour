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

package contour

import (
	"reflect"
	"sort"
	"testing"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/gogo/protobuf/proto"
	"k8s.io/api/core/v1"
)

func TestEndpointsTranslatorAddEndpoints(t *testing.T) {
	tests := []struct {
		name string
		ep   *v1.Endpoints
		want []proto.Message
	}{{
		name: "simple",
		ep: endpoints("default", "simple", v1.EndpointSubset{
			Addresses: addresses("192.168.183.24"),
			Ports:     ports(8080),
		}),
		want: []proto.Message{
			clusterloadassignment("default/simple", lbendpoint("192.168.183.24", 8080)),
		},
	}, {
		name: "multiple addresses",
		ep: endpoints("default", "httpbin-org", v1.EndpointSubset{
			Addresses: addresses(
				"23.23.247.89",
				"50.17.192.147",
				"50.17.206.192",
				"50.19.99.160",
			),
			Ports: ports(80),
		}),
		want: []proto.Message{
			clusterloadassignment("default/httpbin-org",
				lbendpoint("23.23.247.89", 80),
				lbendpoint("50.17.192.147", 80),
				lbendpoint("50.17.206.192", 80),
				lbendpoint("50.19.99.160", 80),
			),
		},
	}}

	log := testLogger(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			et := &EndpointsTranslator{
				FieldLogger: log,
			}
			et.OnAdd(tc.ep)
			got := contents(et)
			sort.Stable(clusterLoadAssignmentsByName(got))
			if !reflect.DeepEqual(tc.want, got) {
				t.Fatalf("got: %v, want: %v", got, tc.want)
			}
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
					Ports:     ports(8080),
				}))
			},
			ep: endpoints("default", "simple", v1.EndpointSubset{
				Addresses: addresses("192.168.183.24"),
				Ports:     ports(8080),
			}),
			want: []proto.Message{},
		},
		"remove different": {
			setup: func(et *EndpointsTranslator) {
				et.OnAdd(endpoints("default", "simple", v1.EndpointSubset{
					Addresses: addresses("192.168.183.24"),
					Ports:     ports(8080),
				}))
			},
			ep: endpoints("default", "different", v1.EndpointSubset{
				Addresses: addresses("192.168.183.24"),
				Ports:     ports(8080),
			}),
			want: []proto.Message{
				clusterloadassignment("default/simple", lbendpoint("192.168.183.24", 8080)),
			},
		},
		"remove non existent": {
			setup: func(*EndpointsTranslator) {},
			ep: endpoints("default", "simple", v1.EndpointSubset{
				Addresses: addresses("192.168.183.24"),
				Ports:     ports(8080),
			}),
			want: []proto.Message{},
		},
		"remove long name": {
			setup: func(et *EndpointsTranslator) {
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
				et.OnAdd(e1)
			},
			ep: endpoints(
				"super-long-namespace-name-oh-boy",
				"what-a-descriptive-service-name-you-must-be-so-proud",
				v1.EndpointSubset{
					Addresses: addresses(
						"172.16.0.1",
						"172.16.0.2",
					),
					Ports: ports(8000, 8443),
				},
			),
			want: []proto.Message{},
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
			got := contents(et)
			sort.Stable(clusterLoadAssignmentsByName(got))
			if !reflect.DeepEqual(tc.want, got) {
				t.Fatalf("\nwant: %v\n got: %v", tc.want, got)
			}
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
				Ports:     ports(8080),
			}),
			want: []proto.Message{
				clusterloadassignment("default/simple", lbendpoint("192.168.183.24", 8080)),
			},
		},
		"multiple addresses": {
			newep: endpoints("default", "httpbin-org", v1.EndpointSubset{
				Addresses: addresses(
					"23.23.247.89",
					"50.17.192.147",
					"50.17.206.192",
					"50.19.99.160",
				),
				Ports: ports(80),
			}),
			want: []proto.Message{
				clusterloadassignment("default/httpbin-org",
					lbendpoint("23.23.247.89", 80),
					lbendpoint("50.17.192.147", 80),
					lbendpoint("50.17.206.192", 80),
					lbendpoint("50.19.99.160", 80),
				),
			},
		},
		"named container port": {
			newep: endpoints("default", "secure", v1.EndpointSubset{
				Addresses: addresses("192.168.183.24"),
				Ports: []v1.EndpointPort{{
					Name: "https",
					Port: 8443,
				}},
			}),
			want: []proto.Message{
				clusterloadassignment("default/secure/https", lbendpoint("192.168.183.24", 8443)),
			},
		},
		"remove existing": {
			oldep: endpoints("default", "simple", v1.EndpointSubset{
				Addresses: addresses("192.168.183.24"),
				Ports:     ports(8080),
			}),
			want: []proto.Message{},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var et EndpointsTranslator
			et.recomputeClusterLoadAssignment(tc.oldep, tc.newep)
			got := contents(&et)
			sort.Stable(clusterLoadAssignmentsByName(got))
			if !reflect.DeepEqual(tc.want, got) {
				t.Fatalf("expected:\n%v\ngot:\n%v", tc.want, got)
			}
		})
	}
}

// See #602
func TestEndpointsTranslatorScaleToZeroEndpoints(t *testing.T) {
	var et EndpointsTranslator
	e1 := endpoints("default", "simple", v1.EndpointSubset{
		Addresses: addresses("192.168.183.24"),
		Ports:     ports(8080),
	})
	et.OnAdd(e1)

	// Assert endpoint was added
	want := []proto.Message{
		clusterloadassignment("default/simple", lbendpoint("192.168.183.24", 8080)),
	}
	got := contents(&et)
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("expected:\n%v\ngot:\n%v\n", want, got)
	}

	// e2 is the same as e1, but without endpoint subsets
	e2 := endpoints("default", "simple")
	et.OnUpdate(e1, e2)

	// Assert endpoints are removed
	want = []proto.Message{}
	got = contents(&et)
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("expected:\n%v\ngot:\n%v\n", want, got)
	}
}

type clusterLoadAssignmentsByName []proto.Message

func (c clusterLoadAssignmentsByName) Len() int      { return len(c) }
func (c clusterLoadAssignmentsByName) Swap(i, j int) { c[i], c[j] = c[j], c[i] }
func (c clusterLoadAssignmentsByName) Less(i, j int) bool {
	return c[i].(*v2.ClusterLoadAssignment).ClusterName < c[j].(*v2.ClusterLoadAssignment).ClusterName
}
