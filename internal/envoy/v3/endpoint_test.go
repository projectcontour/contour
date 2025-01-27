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
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/protobuf"
)

func TestLBEndpoint(t *testing.T) {
	got := LBEndpoint(SocketAddress("microsoft.com", 81))
	want := &envoy_config_endpoint_v3.LbEndpoint{
		HostIdentifier: &envoy_config_endpoint_v3.LbEndpoint_Endpoint{
			Endpoint: &envoy_config_endpoint_v3.Endpoint{
				Address: SocketAddress("microsoft.com", 81),
			},
		},
	}
	protobuf.ExpectEqual(t, want, got)
}

func TestHealthCheckConfig(t *testing.T) {
	got := HealthCheckConfig(8998)
	want := &envoy_config_endpoint_v3.Endpoint_HealthCheckConfig{
		PortValue: uint32(8998),
	}
	protobuf.ExpectEqual(t, want, got)

	require.Nil(t, HealthCheckConfig(0))
}

func TestEndpoints(t *testing.T) {
	got := Endpoints(
		SocketAddress("github.com", 443),
		SocketAddress("microsoft.com", 80),
	)
	want := []*envoy_config_endpoint_v3.LocalityLbEndpoints{{
		LbEndpoints: []*envoy_config_endpoint_v3.LbEndpoint{{
			HostIdentifier: &envoy_config_endpoint_v3.LbEndpoint_Endpoint{
				Endpoint: &envoy_config_endpoint_v3.Endpoint{
					Address: SocketAddress("github.com", 443),
				},
			},
		}, {
			HostIdentifier: &envoy_config_endpoint_v3.LbEndpoint_Endpoint{
				Endpoint: &envoy_config_endpoint_v3.Endpoint{
					Address: SocketAddress("microsoft.com", 80),
				},
			},
		}},
	}}
	protobuf.ExpectEqual(t, want, got)
}

func TestClusterLoadAssignment(t *testing.T) {
	got := ClusterLoadAssignment("empty")
	want := &envoy_config_endpoint_v3.ClusterLoadAssignment{
		ClusterName: "empty",
	}

	protobuf.RequireEqual(t, want, got)

	got = ClusterLoadAssignment("one addr", SocketAddress("microsoft.com", 81))
	want = &envoy_config_endpoint_v3.ClusterLoadAssignment{
		ClusterName: "one addr",
		Endpoints:   Endpoints(SocketAddress("microsoft.com", 81)),
	}

	protobuf.RequireEqual(t, want, got)

	got = ClusterLoadAssignment("two addrs",
		SocketAddress("microsoft.com", 81),
		SocketAddress("github.com", 443),
	)
	want = &envoy_config_endpoint_v3.ClusterLoadAssignment{
		ClusterName: "two addrs",
		Endpoints: Endpoints(
			SocketAddress("microsoft.com", 81),
			SocketAddress("github.com", 443),
		),
	}

	protobuf.RequireEqual(t, want, got)
}

func TestExternalNameClusterLoadAssignment(t *testing.T) {
	s1 := &dag.Service{
		Weighted: dag.WeightedService{
			Weight:           1,
			ServiceName:      "kuard",
			ServiceNamespace: "default",
			ServicePort: core_v1.ServicePort{
				Name:       "http",
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt32(8080),
			},
			HealthPort: core_v1.ServicePort{
				Name:       "http",
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt32(8080),
			},
		},
		ExternalName: "foo.io",
	}

	s2 := &dag.Service{
		Weighted: dag.WeightedService{
			Weight:           1,
			ServiceName:      "kuard",
			ServiceNamespace: "default",
			ServicePort: core_v1.ServicePort{
				Name:       "http",
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt32(8080),
			},
			HealthPort: core_v1.ServicePort{
				Name:       "http",
				Protocol:   "TCP",
				Port:       8998,
				TargetPort: intstr.FromInt32(8998),
			},
		},
		ExternalName: "foo.io",
	}

	got := externalNameClusterLoadAssignment(s1)
	want := &envoy_config_endpoint_v3.ClusterLoadAssignment{
		ClusterName: "default/kuard/http",
		Endpoints: Endpoints(
			SocketAddress("foo.io", 80),
		),
	}
	protobuf.RequireEqual(t, want, got)

	got = externalNameClusterLoadAssignment(s2)
	want = &envoy_config_endpoint_v3.ClusterLoadAssignment{
		ClusterName: "default/kuard/http",
		Endpoints: []*envoy_config_endpoint_v3.LocalityLbEndpoints{
			{
				LbEndpoints: []*envoy_config_endpoint_v3.LbEndpoint{
					{
						HostIdentifier: &envoy_config_endpoint_v3.LbEndpoint_Endpoint{
							Endpoint: &envoy_config_endpoint_v3.Endpoint{
								Address:           SocketAddress("foo.io", 80),
								HealthCheckConfig: HealthCheckConfig(8998),
							},
						},
					},
				},
			},
		},
	}
	protobuf.RequireEqual(t, want, got)
}
