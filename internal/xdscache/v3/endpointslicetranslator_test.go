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
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	core_v1 "k8s.io/api/core/v1"
	discovery_v1 "k8s.io/api/discovery/v1"
	"k8s.io/utils/ptr"

	"github.com/projectcontour/contour/internal/dag"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/fixture"
	"github.com/projectcontour/contour/internal/protobuf"
)

func TestEndpointSliceTranslatorContents(t *testing.T) {
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
			endpointSliceTranslator := NewEndpointSliceTranslator(fixture.NewTestLogger(t))
			endpointSliceTranslator.entries = tc.contents
			got := endpointSliceTranslator.Contents()
			protobuf.ExpectEqual(t, tc.want, got)
		})
	}
}

func TestEndpointSliceTranslatorAddEndpoints(t *testing.T) {
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
		endpointSlice *discovery_v1.EndpointSlice
		want          []proto.Message
		wantUpdate    bool
	}{
		"simple": {
			endpointSlice: endpointSlice("default", "simple-eps-fs9du", "simple", discovery_v1.AddressTypeIPv4, []discovery_v1.Endpoint{
				{
					Addresses: []string{"192.168.183.24"},
				},
			}, []discovery_v1.EndpointPort{
				{
					Port:     ptr.To[int32](8080),
					Protocol: ptr.To[core_v1.Protocol]("TCP"),
				},
			},
			),
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
		"adding an endpoint slice not used by a cluster should not trigger a calculation": {
			endpointSlice: endpointSlice("default", "not-used-eps-sdf8s", "not-used-endpoint", discovery_v1.AddressTypeIPv4, []discovery_v1.Endpoint{
				{
					Addresses: []string{"192.168.183.24"},
				},
			}, []discovery_v1.EndpointPort{
				{
					Port:     ptr.To[int32](8080),
					Protocol: ptr.To[core_v1.Protocol]("TCP"),
				},
			},
			),
			want:       nil,
			wantUpdate: false,
		},
		"single slice, multiple addresses": {
			endpointSlice: endpointSlice("default", "simple-eps-fs9du", "simple", discovery_v1.AddressTypeIPv4, []discovery_v1.Endpoint{
				{
					Addresses: []string{
						"50.17.206.192",
					},
				},
			}, []discovery_v1.EndpointPort{
				{
					Port:     ptr.To[int32](80),
					Protocol: ptr.To[core_v1.Protocol]("TCP"),
				},
			},
			),
			want: []proto.Message{
				&envoy_config_endpoint_v3.ClusterLoadAssignment{ClusterName: "default/healthcheck-port"},
				&envoy_config_endpoint_v3.ClusterLoadAssignment{ClusterName: "default/httpbin-org/a"},
				&envoy_config_endpoint_v3.ClusterLoadAssignment{ClusterName: "default/httpbin-org/b"},
				&envoy_config_endpoint_v3.ClusterLoadAssignment{
					ClusterName: "default/simple",
					Endpoints: envoy_v3.WeightedEndpoints(1,
						envoy_v3.SocketAddress("50.17.206.192", 80),
					),
				},
			},
			wantUpdate: true,
		},
		"multiple slices": {
			endpointSlice: endpointSlice("default", "simple-eps-fs9du", "simple", discovery_v1.AddressTypeIPv4, []discovery_v1.Endpoint{
				{
					Addresses: []string{
						"50.17.206.192",
					},
				},
				{
					Addresses: []string{
						"23.23.247.89",
					},
				},
				{
					Addresses: []string{
						"50.17.192.147",
					},
				},
				{
					Addresses: []string{
						"50.19.99.160",
					},
				},
			}, []discovery_v1.EndpointPort{
				{
					Port:     ptr.To[int32](80),
					Protocol: ptr.To[core_v1.Protocol]("TCP"),
				},
			},
			),
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
			endpointSlice: endpointSlice("default", "httpbin-org-s9d8f", "httpbin-org", discovery_v1.AddressTypeIPv4, []discovery_v1.Endpoint{
				{
					Addresses: []string{
						"10.10.1.1",
					},
				},
			}, []discovery_v1.EndpointPort{
				{
					Name:     ptr.To[string]("a"),
					Port:     ptr.To[int32](8675),
					Protocol: ptr.To[core_v1.Protocol]("TCP"),
				},
				{
					Name:     ptr.To[string]("b"),
					Port:     ptr.To[int32](309),
					Protocol: ptr.To[core_v1.Protocol]("TCP"),
				},
			},
			),
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
			endpointSlice: endpointSlice("default", "httpbin-org-s9d8f", "httpbin-org", discovery_v1.AddressTypeIPv4, []discovery_v1.Endpoint{
				{
					Addresses: []string{
						"10.10.1.1",
					},
				},
				{
					Addresses: []string{
						"10.10.2.2",
					},
				},
			}, []discovery_v1.EndpointPort{
				{
					Name:     ptr.To[string]("a"),
					Port:     ptr.To[int32](8675),
					Protocol: ptr.To[core_v1.Protocol]("TCP"),
				},
				{
					Name:     ptr.To[string]("b"),
					Port:     ptr.To[int32](309),
					Protocol: ptr.To[core_v1.Protocol]("TCP"),
				},
			},
			),
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
			endpointSlice: endpointSlice("default", "httpbin-org-s9d8f", "httpbin-org", discovery_v1.AddressTypeIPv4, []discovery_v1.Endpoint{
				{
					Addresses: []string{
						"10.10.1.1",
					},
					Conditions: discovery_v1.EndpointConditions{
						Ready: ptr.To[bool](false),
					},
				},
				{
					Addresses: []string{
						"10.10.2.2",
					},
					Conditions: discovery_v1.EndpointConditions{
						Ready: ptr.To[bool](true),
					},
				},
			}, []discovery_v1.EndpointPort{
				{
					Name:     ptr.To[string]("a"),
					Port:     ptr.To[int32](8675),
					Protocol: ptr.To[core_v1.Protocol]("TCP"),
				},
				{
					Name:     ptr.To[string]("b"),
					Port:     ptr.To[int32](309),
					Protocol: ptr.To[core_v1.Protocol]("TCP"),
				},
			},
			),
			want: []proto.Message{
				&envoy_config_endpoint_v3.ClusterLoadAssignment{ClusterName: "default/healthcheck-port"},
				&envoy_config_endpoint_v3.ClusterLoadAssignment{
					ClusterName: "default/httpbin-org/a",
					Endpoints: envoy_v3.WeightedEndpoints(1,
						envoy_v3.SocketAddress("10.10.2.2", 8675),
					),
				},
				&envoy_config_endpoint_v3.ClusterLoadAssignment{
					ClusterName: "default/httpbin-org/b",
					Endpoints: envoy_v3.WeightedEndpoints(1,
						envoy_v3.SocketAddress("10.10.2.2", 309),
					),
				},
				&envoy_config_endpoint_v3.ClusterLoadAssignment{ClusterName: "default/simple"},
			},
			wantUpdate: true,
		},
		"health port": {
			endpointSlice: endpointSlice("default", "healthcheck-port-s9d8f", "healthcheck-port", discovery_v1.AddressTypeIPv4, []discovery_v1.Endpoint{
				{
					Addresses: []string{
						"10.10.1.1",
					},
				},
			}, []discovery_v1.EndpointPort{
				{
					Name:     ptr.To[string]("health"),
					Port:     ptr.To[int32](8998),
					Protocol: ptr.To[core_v1.Protocol]("TCP"),
				},
				{
					Name:     ptr.To[string]("a"),
					Port:     ptr.To[int32](309),
					Protocol: ptr.To[core_v1.Protocol]("TCP"),
				},
			},
			),
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
			endpointSliceTranslator := NewEndpointSliceTranslator(fixture.NewTestLogger(t))
			observer := &simpleObserver{}
			endpointSliceTranslator.Observer = observer

			require.NoError(t, endpointSliceTranslator.cache.SetClusters(clusters))
			endpointSliceTranslator.OnAdd(tc.endpointSlice, false)
			got := endpointSliceTranslator.Contents()
			protobuf.ExpectEqual(t, tc.want, got)
			require.Equal(t, tc.wantUpdate, observer.updated)
		})
	}
}

func TestEndpointSliceTranslatorRemoveEndpoints(t *testing.T) {
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
		setup         func(*EndpointSliceTranslator)
		endpointSlice *discovery_v1.EndpointSlice
		want          []proto.Message
		wantUpdate    bool
	}{
		"remove existing": {
			setup: func(endpointSliceTranslator *EndpointSliceTranslator) {
				endpointSliceTranslator.OnAdd(endpointSlice("default", "simple-eps-fs9du", "simple", discovery_v1.AddressTypeIPv4, []discovery_v1.Endpoint{
					{
						Addresses: []string{"192.168.183.24"},
					},
				}, []discovery_v1.EndpointPort{
					{
						Port:     ptr.To[int32](8080),
						Protocol: ptr.To[core_v1.Protocol]("TCP"),
					},
				},
				), false)
			},
			endpointSlice: endpointSlice("default", "simple-eps-fs9du", "simple", discovery_v1.AddressTypeIPv4, []discovery_v1.Endpoint{
				{
					Addresses: []string{"192.168.183.24"},
				},
			}, []discovery_v1.EndpointPort{
				{
					Port:     ptr.To[int32](8080),
					Protocol: ptr.To[core_v1.Protocol]("TCP"),
				},
			},
			),
			want: []proto.Message{
				envoy_v3.ClusterLoadAssignment("default/simple"),
				envoy_v3.ClusterLoadAssignment("super-long-namespace-name-oh-boy/what-a-descriptive-service-name-you-must-be-so-proud/http"),
				envoy_v3.ClusterLoadAssignment("super-long-namespace-name-oh-boy/what-a-descriptive-service-name-you-must-be-so-proud/https"),
			},
			wantUpdate: true,
		},
		"removing an Endpoints not used by a ServiceCluster should not trigger a recalculation": {
			setup: func(endpointSliceTranslator *EndpointSliceTranslator) {
				endpointSliceTranslator.OnAdd(endpointSlice("default", "simple-eps-fs9du", "simple", discovery_v1.AddressTypeIPv4, []discovery_v1.Endpoint{
					{
						Addresses: []string{"192.168.183.24"},
					},
				}, []discovery_v1.EndpointPort{
					{
						Port:     ptr.To[int32](8080),
						Protocol: ptr.To[core_v1.Protocol]("TCP"),
					},
				},
				), false)
			},
			endpointSlice: endpointSlice("default", "different-fs9du", "different", discovery_v1.AddressTypeIPv4, []discovery_v1.Endpoint{
				{
					Addresses: []string{"192.168.183.24"},
				},
			}, []discovery_v1.EndpointPort{
				{
					Port:     ptr.To[int32](8080),
					Protocol: ptr.To[core_v1.Protocol]("TCP"),
				},
			},
			),
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
			setup: func(*EndpointSliceTranslator) {},
			endpointSlice: endpointSlice("default", "simple-eps-fs9du", "simple", discovery_v1.AddressTypeIPv4, []discovery_v1.Endpoint{
				{
					Addresses: []string{"192.168.183.24"},
				},
			}, []discovery_v1.EndpointPort{
				{
					Port:     ptr.To[int32](8080),
					Protocol: ptr.To[core_v1.Protocol]("TCP"),
				},
			},
			),
			want: []proto.Message{
				envoy_v3.ClusterLoadAssignment("default/simple"),
				envoy_v3.ClusterLoadAssignment("super-long-namespace-name-oh-boy/what-a-descriptive-service-name-you-must-be-so-proud/http"),
				envoy_v3.ClusterLoadAssignment("super-long-namespace-name-oh-boy/what-a-descriptive-service-name-you-must-be-so-proud/https"),
			},
			wantUpdate: true,
		},
		"remove long name": {
			setup: func(endpointSliceTranslator *EndpointSliceTranslator) {
				e1 := endpointSlice(
					"super-long-namespace-name-oh-boy",
					"what-a-descriptive-service-name-you-must-be-so-proud-9d8f8",
					"what-a-descriptive-service-name-you-must-be-so-proud",
					discovery_v1.AddressTypeIPv4, []discovery_v1.Endpoint{
						{
							Addresses: []string{"172.16.0.2"},
						},
						{
							Addresses: []string{"172.16.0.1"},
						},
					}, []discovery_v1.EndpointPort{
						{
							Name:     ptr.To[string]("http"),
							Port:     ptr.To[int32](8080),
							Protocol: ptr.To[core_v1.Protocol]("TCP"),
						},
						{
							Name:     ptr.To[string]("https"),
							Port:     ptr.To[int32](8443),
							Protocol: ptr.To[core_v1.Protocol]("TCP"),
						},
					},
				)
				endpointSliceTranslator.OnAdd(e1, false)
			},
			endpointSlice: endpointSlice(
				"super-long-namespace-name-oh-boy",
				"what-a-descriptive-service-name-you-must-be-so-proud-9d8f8",
				"what-a-descriptive-service-name-you-must-be-so-proud",
				discovery_v1.AddressTypeIPv4, []discovery_v1.Endpoint{
					{
						Addresses: []string{"172.16.0.2"},
					},
					{
						Addresses: []string{"172.16.0.1"},
					},
				}, []discovery_v1.EndpointPort{
					{
						Name:     ptr.To[string]("http"),
						Port:     ptr.To[int32](8080),
						Protocol: ptr.To[core_v1.Protocol]("TCP"),
					},
					{
						Name:     ptr.To[string]("https"),
						Port:     ptr.To[int32](8443),
						Protocol: ptr.To[core_v1.Protocol]("TCP"),
					},
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
			endpointSliceTranslator := NewEndpointSliceTranslator(fixture.NewTestLogger(t))
			require.NoError(t, endpointSliceTranslator.cache.SetClusters(clusters))
			tc.setup(endpointSliceTranslator)

			// add the dummy observer after setting things up
			// so we only get notified if the deletion triggers
			// changes, not if the setup additions trigger changes.
			observer := &simpleObserver{}
			endpointSliceTranslator.Observer = observer

			endpointSliceTranslator.OnDelete(tc.endpointSlice)
			got := endpointSliceTranslator.Contents()
			protobuf.ExpectEqual(t, tc.want, got)
			require.Equal(t, tc.wantUpdate, observer.updated)
		})
	}
}

func TestEndpointSliceTranslatorUpdateEndpoints(t *testing.T) {
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
		setup      func(*EndpointSliceTranslator)
		old, new   *discovery_v1.EndpointSlice
		want       []proto.Message
		wantUpdate bool
	}{
		"update existing": {
			setup: func(endpointSliceTranslator *EndpointSliceTranslator) {
				endpointSliceTranslator.OnAdd(endpointSlice("default", "simple-sdf8s", "simple", discovery_v1.AddressTypeIPv4, []discovery_v1.Endpoint{
					{
						Addresses: []string{"192.168.183.24"},
					},
				}, []discovery_v1.EndpointPort{
					{
						Port:     ptr.To[int32](8080),
						Protocol: ptr.To[core_v1.Protocol]("TCP"),
					},
				},
				), false)
			},
			old: endpointSlice("default", "simple-sdf8s", "simple", discovery_v1.AddressTypeIPv4, []discovery_v1.Endpoint{
				{
					Addresses: []string{"192.168.183.24"},
				},
			}, []discovery_v1.EndpointPort{
				{
					Port:     ptr.To[int32](8080),
					Protocol: ptr.To[core_v1.Protocol]("TCP"),
				},
			},
			),
			new: endpointSlice("default", "simple-sdf8s", "simple", discovery_v1.AddressTypeIPv4, []discovery_v1.Endpoint{
				{
					Addresses: []string{"192.168.183.25"},
				},
			}, []discovery_v1.EndpointPort{
				{
					Port:     ptr.To[int32](8081),
					Protocol: ptr.To[core_v1.Protocol]("TCP"),
				},
			},
			),
			want: []proto.Message{
				&envoy_config_endpoint_v3.ClusterLoadAssignment{
					ClusterName: "default/simple",
					Endpoints:   envoy_v3.WeightedEndpoints(1, envoy_v3.SocketAddress("192.168.183.25", 8081)),
				},
			},
			wantUpdate: true,
		},
		"getting an update for an Endpoints not used by a ServiceCluster should not trigger a recalculation": {
			setup: func(endpointSliceTranslator *EndpointSliceTranslator) {
				endpointSliceTranslator.OnAdd(endpointSlice("default", "simple-sdf8s", "simple", discovery_v1.AddressTypeIPv4, []discovery_v1.Endpoint{
					{
						Addresses: []string{"192.168.183.24"},
					},
				}, []discovery_v1.EndpointPort{
					{
						Port:     ptr.To[int32](8080),
						Protocol: ptr.To[core_v1.Protocol]("TCP"),
					},
				},
				), false)
			},
			old: endpointSlice("default", "different-eps-fs9du", "different", discovery_v1.AddressTypeIPv4, []discovery_v1.Endpoint{
				{
					Addresses: []string{"192.168.183.24"},
				},
			}, []discovery_v1.EndpointPort{
				{
					Port:     ptr.To[int32](8080),
					Protocol: ptr.To[core_v1.Protocol]("TCP"),
				},
			},
			),
			new: endpointSlice("default", "different-eps-fs9du", "different", discovery_v1.AddressTypeIPv4, []discovery_v1.Endpoint{
				{
					Addresses: []string{"192.168.183.25"},
				},
			}, []discovery_v1.EndpointPort{
				{
					Port:     ptr.To[int32](8081),
					Protocol: ptr.To[core_v1.Protocol]("TCP"),
				},
			},
			),
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
			endpointSliceTranslator := NewEndpointSliceTranslator(fixture.NewTestLogger(t))
			require.NoError(t, endpointSliceTranslator.cache.SetClusters(clusters))
			tc.setup(endpointSliceTranslator)

			// add the dummy observer after setting things up
			// so we only get notified if the update triggers
			// changes, not if the setup additions trigger changes.
			observer := &simpleObserver{}
			endpointSliceTranslator.Observer = observer

			endpointSliceTranslator.OnUpdate(tc.old, tc.new)
			got := endpointSliceTranslator.Contents()
			protobuf.ExpectEqual(t, tc.want, got)
			require.Equal(t, tc.wantUpdate, observer.updated)
		})
	}
}

func TestEndpointSliceTranslatorRecomputeClusterLoadAssignment(t *testing.T) {
	tests := map[string]struct {
		cluster       dag.ServiceCluster
		endpointSlice *discovery_v1.EndpointSlice
		want          []proto.Message
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
			endpointSlice: endpointSlice("default", "simple-eps-fs9du", "simple", discovery_v1.AddressTypeIPv4, []discovery_v1.Endpoint{
				{
					Addresses: []string{"192.168.183.24"},
				},
			}, []discovery_v1.EndpointPort{
				{
					Port:     ptr.To[int32](8080),
					Protocol: ptr.To[core_v1.Protocol]("TCP"),
				},
			},
			),
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
			endpointSlice: endpointSlice("default", "httpbin-org-fs9du", "httpbin-org", discovery_v1.AddressTypeIPv4, []discovery_v1.Endpoint{
				{
					Addresses: []string{"50.17.192.147"},
				},
				{
					Addresses: []string{"23.23.247.89"},
				},
				{
					Addresses: []string{"50.17.206.192"},
				},
				{
					Addresses: []string{"50.19.99.160"},
				},
			}, []discovery_v1.EndpointPort{
				{
					Port:     ptr.To[int32](80),
					Protocol: ptr.To[core_v1.Protocol]("TCP"),
				},
			},
			),
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
			endpointSlice: endpointSlice("default", "secure-fs9du", "secure", discovery_v1.AddressTypeIPv4, []discovery_v1.Endpoint{
				{
					Addresses: []string{"192.168.183.24"},
				},
			}, []discovery_v1.EndpointPort{
				{
					Port:     ptr.To[int32](8443),
					Protocol: ptr.To[core_v1.Protocol]("TCP"),
					Name:     ptr.To[string]("https"),
				},
			},
			),
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
			endpointSlice: endpointSlice("default", "httpbin-org-fs9du", "httpbin-org", discovery_v1.AddressTypeIPv4, []discovery_v1.Endpoint{
				{
					Addresses: []string{"50.17.192.147"},
				},
				{
					Addresses: []string{"23.23.247.89"},
				},
				{
					Addresses: []string{"50.17.206.192"},
				},
				{
					Addresses: []string{"50.19.99.160"},
				},
			}, []discovery_v1.EndpointPort{
				{
					Port:     ptr.To[int32](80),
					Protocol: ptr.To[core_v1.Protocol]("TCP"),
					Name:     ptr.To[string]("a"),
				},
				{
					Port:     ptr.To[int32](8998),
					Protocol: ptr.To[core_v1.Protocol]("TCP"),
					Name:     ptr.To[string]("health"),
				},
			},
			),
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
			endpointSlice: endpointSlice("default", "httpbin-org-fs9du", "httpbin-org", discovery_v1.AddressTypeIPv4, []discovery_v1.Endpoint{
				{
					Addresses: []string{"50.17.192.147"},
				},
				{
					Addresses: []string{"23.23.247.89"},
				},
				{
					Addresses: []string{"50.17.206.192"},
				},
				{
					Addresses: []string{"50.19.99.160"},
				},
			}, []discovery_v1.EndpointPort{
				{
					Port:     ptr.To[int32](80),
					Protocol: ptr.To[core_v1.Protocol]("TCP"),
					Name:     ptr.To[string]("a"),
				},
				{
					Port:     ptr.To[int32](8998),
					Protocol: ptr.To[core_v1.Protocol]("TCP"),
					Name:     ptr.To[string]("health"),
				},
			},
			),
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
			endpointSliceTranslator := NewEndpointSliceTranslator(fixture.NewTestLogger(t))
			require.NoError(t, endpointSliceTranslator.cache.SetClusters([]*dag.ServiceCluster{&tc.cluster}))
			endpointSliceTranslator.OnAdd(tc.endpointSlice, false)
			got := endpointSliceTranslator.Contents()
			protobuf.ExpectEqual(t, tc.want, got)
		})
	}
}

func TestEndpointSliceTranslatorScaleToZeroEndpoints(t *testing.T) {
	endpointSliceTranslator := NewEndpointSliceTranslator(fixture.NewTestLogger(t))

	require.NoError(t, endpointSliceTranslator.cache.SetClusters([]*dag.ServiceCluster{
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

	e1 := endpointSlice("default", "simple-eps-fs9du", "simple", discovery_v1.AddressTypeIPv4, []discovery_v1.Endpoint{
		{
			Addresses: []string{"192.168.183.24"},
		},
	}, []discovery_v1.EndpointPort{
		{
			Port:     ptr.To[int32](8080),
			Protocol: ptr.To[core_v1.Protocol]("TCP"),
		},
	},
	)
	endpointSliceTranslator.OnAdd(e1, false)

	// Assert endpoint was added
	want := []proto.Message{
		&envoy_config_endpoint_v3.ClusterLoadAssignment{
			ClusterName: "default/simple",
			Endpoints:   envoy_v3.WeightedEndpoints(1, envoy_v3.SocketAddress("192.168.183.24", 8080)),
		},
	}

	protobuf.RequireEqual(t, want, endpointSliceTranslator.Contents())

	// e2 is the same as e1, but without endpoint subsets
	e2 := endpointSlice("default", "simple-eps-fs9du", "simple", discovery_v1.AddressTypeIPv4, nil, nil)
	endpointSliceTranslator.OnUpdate(e1, e2)

	// Assert endpoints are removed
	want = []proto.Message{
		&envoy_config_endpoint_v3.ClusterLoadAssignment{ClusterName: "default/simple"},
	}

	protobuf.RequireEqual(t, want, endpointSliceTranslator.Contents())
}

func TestEndpointSliceTranslatorWeightedService(t *testing.T) {
	endpointSliceTranslator := NewEndpointSliceTranslator(fixture.NewTestLogger(t))
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

	require.NoError(t, endpointSliceTranslator.cache.SetClusters(clusters))

	endpoints := []discovery_v1.Endpoint{
		{
			Addresses: []string{"192.168.183.24"},
		},
	}

	ports := []discovery_v1.EndpointPort{
		{
			Port:     ptr.To[int32](8080),
			Protocol: ptr.To[core_v1.Protocol]("TCP"),
		},
	}

	endpointSliceTranslator.OnAdd(endpointSlice("default", "weight0-eps-fs23r", "weight0", discovery_v1.AddressTypeIPv4, endpoints, ports), false)
	endpointSliceTranslator.OnAdd(endpointSlice("default", "weight0-eps-sdf9f", "weight1", discovery_v1.AddressTypeIPv4, endpoints, ports), false)
	endpointSliceTranslator.OnAdd(endpointSlice("default", "weight0-eps-v9drg", "weight2", discovery_v1.AddressTypeIPv4, endpoints, ports), false)

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

	protobuf.ExpectEqual(t, want, endpointSliceTranslator.Contents())
}

func TestEndpointSliceTranslatorDefaultWeightedService(t *testing.T) {
	endpointSliceTranslator := NewEndpointSliceTranslator(fixture.NewTestLogger(t))
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

	require.NoError(t, endpointSliceTranslator.cache.SetClusters(clusters))

	endpoints := []discovery_v1.Endpoint{
		{
			Addresses: []string{"192.168.183.24"},
		},
	}

	ports := []discovery_v1.EndpointPort{
		{
			Port:     ptr.To[int32](8080),
			Protocol: ptr.To[core_v1.Protocol]("TCP"),
		},
	}

	endpointSliceTranslator.OnAdd(endpointSlice("default", "weight0-eps-fs23r", "weight0", discovery_v1.AddressTypeIPv4, endpoints, ports), false)
	endpointSliceTranslator.OnAdd(endpointSlice("default", "weight0-eps-sdf9f", "weight1", discovery_v1.AddressTypeIPv4, endpoints, ports), false)
	endpointSliceTranslator.OnAdd(endpointSlice("default", "weight0-eps-v9drg", "weight2", discovery_v1.AddressTypeIPv4, endpoints, ports), false)

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

	protobuf.ExpectEqual(t, want, endpointSliceTranslator.Contents())
}
