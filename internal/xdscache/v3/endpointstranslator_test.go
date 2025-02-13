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

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
	core_v1 "k8s.io/api/core/v1"

	"github.com/projectcontour/contour/internal/dag"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/fixture"
	"github.com/projectcontour/contour/internal/protobuf"
)

func TestEndpointsTranslatorContents(t *testing.T) {
	tests := map[string]struct {
		contents map[string]*envoy_config_endpoint_v3.ClusterLoadAssignment
		want     []proto.Message
	}{
		"empty": {
			contents: nil,
			want:     nil,
		},
		"simple": {
			contents: clusterloadassignments(
				envoy_v3.ClusterLoadAssignment("default/httpbin-org",
					envoy_v3.SocketAddress("10.10.10.10", 80),
				),
			),
			want: []proto.Message{
				envoy_v3.ClusterLoadAssignment("default/httpbin-org",
					envoy_v3.SocketAddress("10.10.10.10", 80),
				),
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			et := NewEndpointsTranslator(fixture.NewTestLogger(t))
			et.entries = tc.contents
			got := et.Contents()
			protobuf.ExpectEqual(t, tc.want, got)
		})
	}
}

func TestEndpointsTranslatorAddEndpoints(t *testing.T) {
	clusters := []*dag.ServiceCluster{
		{
			ClusterName: "default/httpbin-org/a",
			Services: []dag.WeightedService{
				{
					Weight:           1,
					ServiceName:      "httpbin-org",
					ServiceNamespace: "default",
					ServicePort:      core_v1.ServicePort{Name: "a"},
				},
			},
		},
		{
			ClusterName: "default/httpbin-org/b",
			Services: []dag.WeightedService{
				{
					Weight:           1,
					ServiceName:      "httpbin-org",
					ServiceNamespace: "default",
					ServicePort:      core_v1.ServicePort{Name: "b"},
				},
			},
		},
		{
			ClusterName: "default/simple",
			Services: []dag.WeightedService{
				{
					Weight:           1,
					ServiceName:      "simple",
					ServiceNamespace: "default",
					ServicePort:      core_v1.ServicePort{},
				},
			},
		},
		{
			ClusterName: "default/healthcheck-port",
			Services: []dag.WeightedService{
				{
					Weight:           1,
					ServiceName:      "healthcheck-port",
					ServiceNamespace: "default",
					ServicePort:      core_v1.ServicePort{Name: "a"},
					HealthPort:       core_v1.ServicePort{Name: "health", Port: 8998},
				},
			},
		},
	}

	tests := map[string]struct {
		ep         *core_v1.Endpoints
		want       []proto.Message
		wantUpdate bool
	}{
		"simple": {
			ep: endpoints("default", "simple", core_v1.EndpointSubset{
				Addresses: addresses("192.168.183.24"),
				Ports: ports(
					port("", 8080),
				),
			}),
			want: []proto.Message{
				&envoy_config_endpoint_v3.ClusterLoadAssignment{ClusterName: "default/healthcheck-port"},
				&envoy_config_endpoint_v3.ClusterLoadAssignment{ClusterName: "default/httpbin-org/a"},
				&envoy_config_endpoint_v3.ClusterLoadAssignment{ClusterName: "default/httpbin-org/b"},
				&envoy_config_endpoint_v3.ClusterLoadAssignment{
					ClusterName: "default/simple",
					Endpoints:   envoy_v3.WeightedEndpoints(1, envoy_v3.SocketAddress("192.168.183.24", 8080)),
				},
			},
			wantUpdate: true,
		},
		"adding an Endpoints not used by a ServiceCluster should not trigger a recalculation": {
			ep: endpoints("default", "not-used-endpoint", core_v1.EndpointSubset{
				Addresses: addresses("192.168.183.24"),
				Ports: ports(
					port("", 8080),
				),
			}),
			want:       nil,
			wantUpdate: false,
		},
		"multiple addresses": {
			ep: endpoints("default", "simple", core_v1.EndpointSubset{
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
				&envoy_config_endpoint_v3.ClusterLoadAssignment{ClusterName: "default/healthcheck-port"},
				&envoy_config_endpoint_v3.ClusterLoadAssignment{ClusterName: "default/httpbin-org/a"},
				&envoy_config_endpoint_v3.ClusterLoadAssignment{ClusterName: "default/httpbin-org/b"},
				&envoy_config_endpoint_v3.ClusterLoadAssignment{
					ClusterName: "default/simple",
					Endpoints: envoy_v3.WeightedEndpoints(1,
						envoy_v3.SocketAddress("23.23.247.89", 80), // addresses should be sorted
						envoy_v3.SocketAddress("50.17.192.147", 80),
						envoy_v3.SocketAddress("50.17.206.192", 80),
						envoy_v3.SocketAddress("50.19.99.160", 80),
					),
				},
			},
			wantUpdate: true,
		},
		"multiple ports": {
			ep: endpoints("default", "httpbin-org", core_v1.EndpointSubset{
				Addresses: addresses(
					"10.10.1.1",
				),
				Ports: ports(
					port("b", 309),
					port("a", 8675),
				),
			}),
			want: []proto.Message{
				&envoy_config_endpoint_v3.ClusterLoadAssignment{ClusterName: "default/healthcheck-port"},
				// Results should be sorted by cluster name.
				&envoy_config_endpoint_v3.ClusterLoadAssignment{
					ClusterName: "default/httpbin-org/a",
					Endpoints:   envoy_v3.WeightedEndpoints(1, envoy_v3.SocketAddress("10.10.1.1", 8675)),
				},
				&envoy_config_endpoint_v3.ClusterLoadAssignment{
					ClusterName: "default/httpbin-org/b",
					Endpoints:   envoy_v3.WeightedEndpoints(1, envoy_v3.SocketAddress("10.10.1.1", 309)),
				},
				&envoy_config_endpoint_v3.ClusterLoadAssignment{ClusterName: "default/simple"},
			},
			wantUpdate: true,
		},
		"cartesian product": {
			ep: endpoints("default", "httpbin-org", core_v1.EndpointSubset{
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
				&envoy_config_endpoint_v3.ClusterLoadAssignment{ClusterName: "default/healthcheck-port"},
				&envoy_config_endpoint_v3.ClusterLoadAssignment{
					ClusterName: "default/httpbin-org/a",
					Endpoints: envoy_v3.WeightedEndpoints(1,
						envoy_v3.SocketAddress("10.10.1.1", 8675), // addresses should be sorted
						envoy_v3.SocketAddress("10.10.2.2", 8675),
					),
				},
				&envoy_config_endpoint_v3.ClusterLoadAssignment{
					ClusterName: "default/httpbin-org/b",
					Endpoints: envoy_v3.WeightedEndpoints(1,
						envoy_v3.SocketAddress("10.10.1.1", 309),
						envoy_v3.SocketAddress("10.10.2.2", 309),
					),
				},
				&envoy_config_endpoint_v3.ClusterLoadAssignment{ClusterName: "default/simple"},
			},
			wantUpdate: true,
		},
		"not ready": {
			ep: endpoints("default", "httpbin-org", core_v1.EndpointSubset{
				Addresses: addresses(
					"10.10.1.1",
				),
				NotReadyAddresses: addresses(
					"10.10.2.2",
				),
				Ports: ports(
					port("a", 8675),
				),
			}, core_v1.EndpointSubset{
				Addresses: addresses(
					"10.10.2.2",
					"10.10.1.1",
				),
				Ports: ports(
					port("b", 309),
				),
			}),
			want: []proto.Message{
				&envoy_config_endpoint_v3.ClusterLoadAssignment{ClusterName: "default/healthcheck-port"},
				&envoy_config_endpoint_v3.ClusterLoadAssignment{
					ClusterName: "default/httpbin-org/a",
					Endpoints: envoy_v3.WeightedEndpoints(1,
						envoy_v3.SocketAddress("10.10.1.1", 8675),
					),
				},
				&envoy_config_endpoint_v3.ClusterLoadAssignment{
					ClusterName: "default/httpbin-org/b",
					Endpoints: envoy_v3.WeightedEndpoints(1,
						envoy_v3.SocketAddress("10.10.1.1", 309),
						envoy_v3.SocketAddress("10.10.2.2", 309),
					),
				},
				&envoy_config_endpoint_v3.ClusterLoadAssignment{ClusterName: "default/simple"},
			},
			wantUpdate: true,
		},
		"health port": {
			ep: endpoints("default", "healthcheck-port", core_v1.EndpointSubset{
				Addresses: addresses("10.10.1.1"),
				Ports: ports(
					port("a", 309),
					port("health", 8998),
				),
			}),
			want: []proto.Message{
				&envoy_config_endpoint_v3.ClusterLoadAssignment{
					ClusterName: "default/healthcheck-port",
					Endpoints: weightedHealthcheckEndpoints(1, 8998,
						envoy_v3.SocketAddress("10.10.1.1", 309),
					),
				},
				&envoy_config_endpoint_v3.ClusterLoadAssignment{ClusterName: "default/httpbin-org/a"},
				&envoy_config_endpoint_v3.ClusterLoadAssignment{ClusterName: "default/httpbin-org/b"},
				&envoy_config_endpoint_v3.ClusterLoadAssignment{ClusterName: "default/simple"},
			},
			wantUpdate: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			et := NewEndpointsTranslator(fixture.NewTestLogger(t))
			observer := &simpleObserver{}
			et.Observer = observer

			require.NoError(t, et.cache.SetClusters(clusters))
			et.OnAdd(tc.ep, false)
			got := et.Contents()
			protobuf.ExpectEqual(t, tc.want, got)
			require.Equal(t, tc.wantUpdate, observer.updated)
		})
	}
}

type simpleObserver struct {
	updated bool
}

func (s *simpleObserver) Refresh() {
	s.updated = true
}

func TestEndpointsTranslatorRemoveEndpoints(t *testing.T) {
	clusters := []*dag.ServiceCluster{
		{
			ClusterName: "default/simple",
			Services: []dag.WeightedService{
				{
					Weight:           1,
					ServiceName:      "simple",
					ServiceNamespace: "default",
					ServicePort:      core_v1.ServicePort{},
				},
			},
		},
		{
			ClusterName: "super-long-namespace-name-oh-boy/what-a-descriptive-service-name-you-must-be-so-proud/http",
			Services: []dag.WeightedService{
				{
					Weight:           1,
					ServiceName:      "what-a-descriptive-service-name-you-must-be-so-proud",
					ServiceNamespace: "super-long-namespace-name-oh-boy",
					ServicePort:      core_v1.ServicePort{Name: "http"},
				},
			},
		},
		{
			ClusterName: "super-long-namespace-name-oh-boy/what-a-descriptive-service-name-you-must-be-so-proud/https",
			Services: []dag.WeightedService{
				{
					Weight:           1,
					ServiceName:      "what-a-descriptive-service-name-you-must-be-so-proud",
					ServiceNamespace: "super-long-namespace-name-oh-boy",
					ServicePort:      core_v1.ServicePort{Name: "https"},
				},
			},
		},
	}

	tests := map[string]struct {
		setup      func(*EndpointsTranslator)
		ep         *core_v1.Endpoints
		want       []proto.Message
		wantUpdate bool
	}{
		"remove existing": {
			setup: func(et *EndpointsTranslator) {
				et.OnAdd(endpoints("default", "simple", core_v1.EndpointSubset{
					Addresses: addresses("192.168.183.24"),
					Ports: ports(
						port("", 8080),
					),
				}), false)
			},
			ep: endpoints("default", "simple", core_v1.EndpointSubset{
				Addresses: addresses("192.168.183.24"),
				Ports: ports(
					port("", 8080),
				),
			}),
			want: []proto.Message{
				envoy_v3.ClusterLoadAssignment("default/simple"),
				envoy_v3.ClusterLoadAssignment("super-long-namespace-name-oh-boy/what-a-descriptive-service-name-you-must-be-so-proud/http"),
				envoy_v3.ClusterLoadAssignment("super-long-namespace-name-oh-boy/what-a-descriptive-service-name-you-must-be-so-proud/https"),
			},
			wantUpdate: true,
		},
		"removing an Endpoints not used by a ServiceCluster should not trigger a recalculation": {
			setup: func(et *EndpointsTranslator) {
				et.OnAdd(endpoints("default", "simple", core_v1.EndpointSubset{
					Addresses: addresses("192.168.183.24"),
					Ports: ports(
						port("", 8080),
					),
				}), false)
			},
			ep: endpoints("default", "different", core_v1.EndpointSubset{
				Addresses: addresses("192.168.183.24"),
				Ports: ports(
					port("", 8080),
				),
			}),
			want: []proto.Message{
				&envoy_config_endpoint_v3.ClusterLoadAssignment{
					ClusterName: "default/simple",
					Endpoints:   envoy_v3.WeightedEndpoints(1, envoy_v3.SocketAddress("192.168.183.24", 8080)),
				},
				envoy_v3.ClusterLoadAssignment("super-long-namespace-name-oh-boy/what-a-descriptive-service-name-you-must-be-so-proud/http"),
				envoy_v3.ClusterLoadAssignment("super-long-namespace-name-oh-boy/what-a-descriptive-service-name-you-must-be-so-proud/https"),
			},
			wantUpdate: false,
		},
		"remove non existent": {
			setup: func(*EndpointsTranslator) {},
			ep: endpoints("default", "simple", core_v1.EndpointSubset{
				Addresses: addresses("192.168.183.24"),
				Ports: ports(
					port("", 8080),
				),
			}),
			want: []proto.Message{
				envoy_v3.ClusterLoadAssignment("default/simple"),
				envoy_v3.ClusterLoadAssignment("super-long-namespace-name-oh-boy/what-a-descriptive-service-name-you-must-be-so-proud/http"),
				envoy_v3.ClusterLoadAssignment("super-long-namespace-name-oh-boy/what-a-descriptive-service-name-you-must-be-so-proud/https"),
			},
			wantUpdate: true,
		},
		"remove long name": {
			setup: func(et *EndpointsTranslator) {
				e1 := endpoints(
					"super-long-namespace-name-oh-boy",
					"what-a-descriptive-service-name-you-must-be-so-proud",
					core_v1.EndpointSubset{
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
				et.OnAdd(e1, false)
			},
			ep: endpoints(
				"super-long-namespace-name-oh-boy",
				"what-a-descriptive-service-name-you-must-be-so-proud",
				core_v1.EndpointSubset{
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
			want: []proto.Message{
				envoy_v3.ClusterLoadAssignment("default/simple"),
				envoy_v3.ClusterLoadAssignment("super-long-namespace-name-oh-boy/what-a-descriptive-service-name-you-must-be-so-proud/http"),
				envoy_v3.ClusterLoadAssignment("super-long-namespace-name-oh-boy/what-a-descriptive-service-name-you-must-be-so-proud/https"),
			},
			wantUpdate: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			et := NewEndpointsTranslator(fixture.NewTestLogger(t))
			require.NoError(t, et.cache.SetClusters(clusters))
			tc.setup(et)

			// add the dummy observer after setting things up
			// so we only get notified if the deletion triggers
			// changes, not if the setup additions trigger changes.
			observer := &simpleObserver{}
			et.Observer = observer

			// TODO(jpeach): this doesn't actually test
			// that deleting endpoints works. We ought to
			// ensure the cache is populated first and
			// only after that, verify that deletion gives
			// the expected result.
			et.OnDelete(tc.ep)
			got := et.Contents()
			protobuf.ExpectEqual(t, tc.want, got)
			require.Equal(t, tc.wantUpdate, observer.updated)
		})
	}
}

func TestEndpointsTranslatorUpdateEndpoints(t *testing.T) {
	clusters := []*dag.ServiceCluster{
		{
			ClusterName: "default/simple",
			Services: []dag.WeightedService{
				{
					Weight:           1,
					ServiceName:      "simple",
					ServiceNamespace: "default",
					ServicePort:      core_v1.ServicePort{},
				},
			},
		},
	}

	tests := map[string]struct {
		setup      func(*EndpointsTranslator)
		old, new   *core_v1.Endpoints
		want       []proto.Message
		wantUpdate bool
	}{
		"update existing": {
			setup: func(et *EndpointsTranslator) {
				et.OnAdd(endpoints("default", "simple", core_v1.EndpointSubset{
					Addresses: addresses("192.168.183.24"),
					Ports: ports(
						port("", 8080),
					),
				}), false)
			},
			old: endpoints("default", "simple", core_v1.EndpointSubset{
				Addresses: addresses("192.168.183.24"),
				Ports: ports(
					port("", 8080),
				),
			}),
			new: endpoints("default", "simple", core_v1.EndpointSubset{
				Addresses: addresses("192.168.183.25"),
				Ports: ports(
					port("", 8081),
				),
			}),
			want: []proto.Message{
				&envoy_config_endpoint_v3.ClusterLoadAssignment{
					ClusterName: "default/simple",
					Endpoints:   envoy_v3.WeightedEndpoints(1, envoy_v3.SocketAddress("192.168.183.25", 8081)),
				},
			},
			wantUpdate: true,
		},
		"getting an update for an Endpoints not used by a ServiceCluster should not trigger a recalculation": {
			setup: func(et *EndpointsTranslator) {
				et.OnAdd(endpoints("default", "simple", core_v1.EndpointSubset{
					Addresses: addresses("192.168.183.24"),
					Ports: ports(
						port("", 8080),
					),
				}), false)
			},
			old: endpoints("default", "different", core_v1.EndpointSubset{
				Addresses: addresses("192.168.183.24"),
				Ports: ports(
					port("", 8080),
				),
			}),
			new: endpoints("default", "different", core_v1.EndpointSubset{
				Addresses: addresses("192.168.183.25"),
				Ports: ports(
					port("", 8081),
				),
			}),
			want: []proto.Message{
				&envoy_config_endpoint_v3.ClusterLoadAssignment{
					ClusterName: "default/simple",
					Endpoints:   envoy_v3.WeightedEndpoints(1, envoy_v3.SocketAddress("192.168.183.24", 8080)),
				},
			},
			wantUpdate: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			et := NewEndpointsTranslator(fixture.NewTestLogger(t))
			require.NoError(t, et.cache.SetClusters(clusters))
			tc.setup(et)

			// add the dummy observer after setting things up
			// so we only get notified if the update triggers
			// changes, not if the setup additions trigger changes.
			observer := &simpleObserver{}
			et.Observer = observer

			et.OnUpdate(tc.old, tc.new)
			got := et.Contents()
			protobuf.ExpectEqual(t, tc.want, got)
			require.Equal(t, tc.wantUpdate, observer.updated)
		})
	}
}

func TestEndpointsTranslatorRecomputeClusterLoadAssignment(t *testing.T) {
	tests := map[string]struct {
		cluster dag.ServiceCluster
		ep      *core_v1.Endpoints
		want    []proto.Message
	}{
		"simple": {
			cluster: dag.ServiceCluster{
				ClusterName: "default/simple",
				Services: []dag.WeightedService{{
					Weight:           1,
					ServiceName:      "simple",
					ServiceNamespace: "default",
				}},
			},
			ep: endpoints("default", "simple", core_v1.EndpointSubset{
				Addresses: addresses("192.168.183.24"),
				Ports: ports(
					port("", 8080),
				),
			}),
			want: []proto.Message{
				&envoy_config_endpoint_v3.ClusterLoadAssignment{
					ClusterName: "default/simple",
					Endpoints: envoy_v3.WeightedEndpoints(1,
						envoy_v3.SocketAddress("192.168.183.24", 8080)),
				},
			},
		},
		"multiple addresses": {
			cluster: dag.ServiceCluster{
				ClusterName: "default/httpbin-org",
				Services: []dag.WeightedService{{
					Weight:           1,
					ServiceName:      "httpbin-org",
					ServiceNamespace: "default",
				}},
			},
			ep: endpoints("default", "httpbin-org", core_v1.EndpointSubset{
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
				&envoy_config_endpoint_v3.ClusterLoadAssignment{
					ClusterName: "default/httpbin-org",
					Endpoints: envoy_v3.WeightedEndpoints(1,
						envoy_v3.SocketAddress("23.23.247.89", 80),
						envoy_v3.SocketAddress("50.17.192.147", 80),
						envoy_v3.SocketAddress("50.17.206.192", 80),
						envoy_v3.SocketAddress("50.19.99.160", 80),
					),
				},
			},
		},
		"named container port": {
			cluster: dag.ServiceCluster{
				ClusterName: "default/secure/https",
				Services: []dag.WeightedService{{
					Weight:           1,
					ServiceName:      "secure",
					ServiceNamespace: "default",
					ServicePort:      core_v1.ServicePort{Name: "https"},
				}},
			},
			ep: endpoints("default", "secure", core_v1.EndpointSubset{
				Addresses: addresses("192.168.183.24"),
				Ports: ports(
					port("https", 8443),
				),
			}),
			want: []proto.Message{
				&envoy_config_endpoint_v3.ClusterLoadAssignment{
					ClusterName: "default/secure/https",
					Endpoints: envoy_v3.WeightedEndpoints(1,
						envoy_v3.SocketAddress("192.168.183.24", 8443)),
				},
			},
		},
		"multiple addresses and healthcheck port": {
			cluster: dag.ServiceCluster{
				ClusterName: "default/httpbin-org",
				Services: []dag.WeightedService{{
					Weight:           1,
					ServiceName:      "httpbin-org",
					ServiceNamespace: "default",
					HealthPort:       core_v1.ServicePort{Name: "health", Port: 8998},
					ServicePort:      core_v1.ServicePort{Name: "a", Port: 80},
				}},
			},
			ep: endpoints("default", "httpbin-org", core_v1.EndpointSubset{
				Addresses: addresses(
					"50.17.192.147",
					"23.23.247.89",
					"50.17.206.192",
					"50.19.99.160",
				),
				Ports: ports(
					port("a", 80),
					port("health", 8998),
				),
			}),
			want: []proto.Message{
				&envoy_config_endpoint_v3.ClusterLoadAssignment{
					ClusterName: "default/httpbin-org",
					Endpoints: weightedHealthcheckEndpoints(1, 8998,
						envoy_v3.SocketAddress("23.23.247.89", 80),
						envoy_v3.SocketAddress("50.17.192.147", 80),
						envoy_v3.SocketAddress("50.17.206.192", 80),
						envoy_v3.SocketAddress("50.19.99.160", 80),
					),
				},
			},
		},
		"health port is the same as service port": {
			cluster: dag.ServiceCluster{
				ClusterName: "default/httpbin-org",
				Services: []dag.WeightedService{{
					Weight:           1,
					ServiceName:      "httpbin-org",
					ServiceNamespace: "default",
					HealthPort:       core_v1.ServicePort{Name: "a", Port: 80},
					ServicePort:      core_v1.ServicePort{Name: "a", Port: 80},
				}},
			},
			ep: endpoints("default", "httpbin-org", core_v1.EndpointSubset{
				Addresses: addresses(
					"50.17.192.147",
					"23.23.247.89",
					"50.17.206.192",
					"50.19.99.160",
				),
				Ports: ports(
					port("a", 80),
					port("health", 8998),
				),
			}),
			want: []proto.Message{
				&envoy_config_endpoint_v3.ClusterLoadAssignment{
					ClusterName: "default/httpbin-org",
					Endpoints: envoy_v3.WeightedEndpoints(1,
						envoy_v3.SocketAddress("23.23.247.89", 80),
						envoy_v3.SocketAddress("50.17.192.147", 80),
						envoy_v3.SocketAddress("50.17.206.192", 80),
						envoy_v3.SocketAddress("50.19.99.160", 80),
					),
				},
			},
		},
	}

	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			et := NewEndpointsTranslator(fixture.NewTestLogger(t))
			// nolint:gosec
			require.NoError(t, et.cache.SetClusters([]*dag.ServiceCluster{&tc.cluster}))
			et.OnAdd(tc.ep, false)
			got := et.Contents()
			protobuf.ExpectEqual(t, tc.want, got)
		})
	}
}

// See #602
func TestEndpointsTranslatorScaleToZeroEndpoints(t *testing.T) {
	et := NewEndpointsTranslator(fixture.NewTestLogger(t))

	require.NoError(t, et.cache.SetClusters([]*dag.ServiceCluster{
		{
			ClusterName: "default/simple",
			Services: []dag.WeightedService{{
				Weight:           1,
				ServiceName:      "simple",
				ServiceNamespace: "default",
				ServicePort:      core_v1.ServicePort{},
			}},
		},
	}))

	e1 := endpoints("default", "simple", core_v1.EndpointSubset{
		Addresses: addresses("192.168.183.24"),
		Ports: ports(
			port("", 8080),
		),
	})
	et.OnAdd(e1, false)

	// Assert endpoint was added
	want := []proto.Message{
		&envoy_config_endpoint_v3.ClusterLoadAssignment{
			ClusterName: "default/simple",
			Endpoints:   envoy_v3.WeightedEndpoints(1, envoy_v3.SocketAddress("192.168.183.24", 8080)),
		},
	}

	protobuf.RequireEqual(t, want, et.Contents())

	// e2 is the same as e1, but without endpoint subsets
	e2 := endpoints("default", "simple")
	et.OnUpdate(e1, e2)

	// Assert endpoints are removed
	want = []proto.Message{
		&envoy_config_endpoint_v3.ClusterLoadAssignment{ClusterName: "default/simple"},
	}

	protobuf.RequireEqual(t, want, et.Contents())
}

// Test that a cluster with weighted services propagates the weights.
func TestEndpointsTranslatorWeightedService(t *testing.T) {
	et := NewEndpointsTranslator(fixture.NewTestLogger(t))
	clusters := []*dag.ServiceCluster{
		{
			ClusterName: "default/weighted",
			Services: []dag.WeightedService{
				{
					Weight:           0,
					ServiceName:      "weight0",
					ServiceNamespace: "default",
					ServicePort:      core_v1.ServicePort{},
				},
				{
					Weight:           1,
					ServiceName:      "weight1",
					ServiceNamespace: "default",
					ServicePort:      core_v1.ServicePort{},
				},
				{
					Weight:           2,
					ServiceName:      "weight2",
					ServiceNamespace: "default",
					ServicePort:      core_v1.ServicePort{},
				},
			},
		},
	}

	require.NoError(t, et.cache.SetClusters(clusters))

	epSubset := core_v1.EndpointSubset{
		Addresses: addresses("192.168.183.24"),
		Ports:     ports(port("", 8080)),
	}

	et.OnAdd(endpoints("default", "weight0", epSubset), false)
	et.OnAdd(endpoints("default", "weight1", epSubset), false)
	et.OnAdd(endpoints("default", "weight2", epSubset), false)

	// Each helper builds a `LocalityLbEndpoints` with one
	// entry, so we can compose the final result by reaching
	// in an taking the first element of each slice.
	w0 := envoy_v3.Endpoints(envoy_v3.SocketAddress("192.168.183.24", 8080))
	w1 := envoy_v3.WeightedEndpoints(1, envoy_v3.SocketAddress("192.168.183.24", 8080))
	w2 := envoy_v3.WeightedEndpoints(2, envoy_v3.SocketAddress("192.168.183.24", 8080))

	want := []proto.Message{
		&envoy_config_endpoint_v3.ClusterLoadAssignment{
			ClusterName: "default/weighted",
			Endpoints: []*envoy_config_endpoint_v3.LocalityLbEndpoints{
				w0[0], w1[0], w2[0],
			},
		},
	}

	protobuf.ExpectEqual(t, want, et.Contents())
}

// Test that a cluster with weighted services that all leave the
// weights unspecified defaults to equally weighed and propagates the
// weights.
func TestEndpointsTranslatorDefaultWeightedService(t *testing.T) {
	et := NewEndpointsTranslator(fixture.NewTestLogger(t))
	clusters := []*dag.ServiceCluster{
		{
			ClusterName: "default/weighted",
			Services: []dag.WeightedService{
				{
					ServiceName:      "weight0",
					ServiceNamespace: "default",
					ServicePort:      core_v1.ServicePort{},
				},
				{
					ServiceName:      "weight1",
					ServiceNamespace: "default",
					ServicePort:      core_v1.ServicePort{},
				},
				{
					ServiceName:      "weight2",
					ServiceNamespace: "default",
					ServicePort:      core_v1.ServicePort{},
				},
			},
		},
	}

	require.NoError(t, et.cache.SetClusters(clusters))

	epSubset := core_v1.EndpointSubset{
		Addresses: addresses("192.168.183.24"),
		Ports:     ports(port("", 8080)),
	}

	et.OnAdd(endpoints("default", "weight0", epSubset), false)
	et.OnAdd(endpoints("default", "weight1", epSubset), false)
	et.OnAdd(endpoints("default", "weight2", epSubset), false)

	// Each helper builds a `LocalityLbEndpoints` with one
	// entry, so we can compose the final result by reaching
	// in an taking the first element of each slice.
	w0 := envoy_v3.WeightedEndpoints(1, envoy_v3.SocketAddress("192.168.183.24", 8080))
	w1 := envoy_v3.WeightedEndpoints(1, envoy_v3.SocketAddress("192.168.183.24", 8080))
	w2 := envoy_v3.WeightedEndpoints(1, envoy_v3.SocketAddress("192.168.183.24", 8080))

	want := []proto.Message{
		&envoy_config_endpoint_v3.ClusterLoadAssignment{
			ClusterName: "default/weighted",
			Endpoints: []*envoy_config_endpoint_v3.LocalityLbEndpoints{
				w0[0], w1[0], w2[0],
			},
		},
	}

	protobuf.ExpectEqual(t, want, et.Contents())
}

func TestEqual(t *testing.T) {
	tests := map[string]struct {
		a, b map[string]*envoy_config_endpoint_v3.ClusterLoadAssignment
		want bool
	}{
		"both nil": {
			a:    nil,
			b:    nil,
			want: true,
		},
		"one nil, one empty": {
			a:    map[string]*envoy_config_endpoint_v3.ClusterLoadAssignment{},
			b:    nil,
			want: true,
		},
		"both empty": {
			a:    map[string]*envoy_config_endpoint_v3.ClusterLoadAssignment{},
			b:    map[string]*envoy_config_endpoint_v3.ClusterLoadAssignment{},
			want: true,
		},
		"a is an incomplete subset of b": {
			a: map[string]*envoy_config_endpoint_v3.ClusterLoadAssignment{
				"a": {ClusterName: "a"},
				"b": {ClusterName: "b"},
			},
			b: map[string]*envoy_config_endpoint_v3.ClusterLoadAssignment{
				"a": {ClusterName: "a"},
				"b": {ClusterName: "b"},
				"c": {ClusterName: "c"},
			},
			want: false,
		},
		"b is an incomplete subset of a": {
			a: map[string]*envoy_config_endpoint_v3.ClusterLoadAssignment{
				"a": {ClusterName: "a"},
				"b": {ClusterName: "b"},
				"c": {ClusterName: "c"},
			},
			b: map[string]*envoy_config_endpoint_v3.ClusterLoadAssignment{
				"a": {ClusterName: "a"},
				"b": {ClusterName: "b"},
			},
			want: false,
		},
		"a and b have the same keys, different values": {
			a: map[string]*envoy_config_endpoint_v3.ClusterLoadAssignment{
				"a": {ClusterName: "a"},
				"b": {ClusterName: "b"},
				"c": {ClusterName: "c"},
			},
			b: map[string]*envoy_config_endpoint_v3.ClusterLoadAssignment{
				"a": {ClusterName: "a"},
				"b": {ClusterName: "b"},
				"c": {ClusterName: "different"},
			},
			want: false,
		},
		"a and b have the same values, different keys": {
			a: map[string]*envoy_config_endpoint_v3.ClusterLoadAssignment{
				"a": {ClusterName: "a"},
				"b": {ClusterName: "b"},
				"c": {ClusterName: "c"},
			},
			b: map[string]*envoy_config_endpoint_v3.ClusterLoadAssignment{
				"d": {ClusterName: "a"},
				"e": {ClusterName: "b"},
				"f": {ClusterName: "c"},
			},
			want: false,
		},
		"a and b have the same keys, same values": {
			a: map[string]*envoy_config_endpoint_v3.ClusterLoadAssignment{
				"a": {ClusterName: "a"},
				"b": {ClusterName: "b"},
				"c": {ClusterName: "c"},
			},
			b: map[string]*envoy_config_endpoint_v3.ClusterLoadAssignment{
				"a": {ClusterName: "a"},
				"b": {ClusterName: "b"},
				"c": {ClusterName: "c"},
			},
			want: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, equal(tc.a, tc.b))
		})
	}
}

func ports(eps ...core_v1.EndpointPort) []core_v1.EndpointPort {
	return eps
}

func port(name string, port int32) core_v1.EndpointPort {
	return core_v1.EndpointPort{
		Name:     name,
		Port:     port,
		Protocol: "TCP",
	}
}

func clusterloadassignments(clas ...*envoy_config_endpoint_v3.ClusterLoadAssignment) map[string]*envoy_config_endpoint_v3.ClusterLoadAssignment {
	m := make(map[string]*envoy_config_endpoint_v3.ClusterLoadAssignment)
	for _, cla := range clas {
		m[cla.ClusterName] = cla
	}
	return m
}

func weightedHealthcheckEndpoints(weight, healthcheckPort uint32, addrs ...*envoy_config_core_v3.Address) []*envoy_config_endpoint_v3.LocalityLbEndpoints {
	lbendpoints := healthcheckEndpoints(healthcheckPort, addrs...)
	lbendpoints[0].LoadBalancingWeight = wrapperspb.UInt32(weight)
	return lbendpoints
}

func healthcheckEndpoints(healthcheckPort uint32, addrs ...*envoy_config_core_v3.Address) []*envoy_config_endpoint_v3.LocalityLbEndpoints {
	lbendpoints := make([]*envoy_config_endpoint_v3.LbEndpoint, 0, len(addrs))
	for _, addr := range addrs {
		lbendpoints = append(lbendpoints, healthCheckLBEndpoint(addr, healthcheckPort))
	}
	return []*envoy_config_endpoint_v3.LocalityLbEndpoints{{
		LbEndpoints: lbendpoints,
	}}
}

// healthCheckLBEndpoint creates a new LbEndpoint include healthCheckConfig
func healthCheckLBEndpoint(addr *envoy_config_core_v3.Address, healthCheckPort uint32) *envoy_config_endpoint_v3.LbEndpoint {
	var hc *envoy_config_endpoint_v3.Endpoint_HealthCheckConfig
	if healthCheckPort != 0 {
		hc = &envoy_config_endpoint_v3.Endpoint_HealthCheckConfig{
			PortValue: healthCheckPort,
		}
	}
	return &envoy_config_endpoint_v3.LbEndpoint{
		HostIdentifier: &envoy_config_endpoint_v3.LbEndpoint_Endpoint{
			Endpoint: &envoy_config_endpoint_v3.Endpoint{
				Address:           addr,
				HealthCheckConfig: hc,
			},
		},
	}
}
