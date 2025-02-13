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
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"k8s.io/apimachinery/pkg/types"

	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/xds"
)

// LBEndpoint creates a new LbEndpoint.
func LBEndpoint(addr *envoy_config_core_v3.Address) *envoy_config_endpoint_v3.LbEndpoint {
	return &envoy_config_endpoint_v3.LbEndpoint{
		HostIdentifier: &envoy_config_endpoint_v3.LbEndpoint_Endpoint{
			Endpoint: &envoy_config_endpoint_v3.Endpoint{
				Address: addr,
			},
		},
	}
}

// HealthCheckConfig returns an *envoy_config_endpoint_v3.Endpoint_HealthCheckConfig with a single
func HealthCheckConfig(healthCheckPort int32) *envoy_config_endpoint_v3.Endpoint_HealthCheckConfig {
	if healthCheckPort == 0 {
		return nil
	}
	return &envoy_config_endpoint_v3.Endpoint_HealthCheckConfig{
		PortValue: uint32(healthCheckPort), //nolint:gosec // disable G115
	}
}

// Endpoints returns a slice of LocalityLbEndpoints.
// The slice contains one entry, with one LbEndpoint per
// *envoy_config_core_v3.Address supplied.
func Endpoints(addrs ...*envoy_config_core_v3.Address) []*envoy_config_endpoint_v3.LocalityLbEndpoints {
	lbendpoints := make([]*envoy_config_endpoint_v3.LbEndpoint, 0, len(addrs))
	for _, addr := range addrs {
		lbendpoints = append(lbendpoints, LBEndpoint(addr))
	}
	return []*envoy_config_endpoint_v3.LocalityLbEndpoints{{
		LbEndpoints: lbendpoints,
	}}
}

func WeightedEndpoints(weight uint32, addrs ...*envoy_config_core_v3.Address) []*envoy_config_endpoint_v3.LocalityLbEndpoints {
	lbendpoints := Endpoints(addrs...)
	lbendpoints[0].LoadBalancingWeight = wrapperspb.UInt32(weight)
	return lbendpoints
}

// ClusterLoadAssignment returns a *envoy_config_endpoint_v3.ClusterLoadAssignment with a single
// LocalityLbEndpoints of the supplied addresses.
func ClusterLoadAssignment(name string, addrs ...*envoy_config_core_v3.Address) *envoy_config_endpoint_v3.ClusterLoadAssignment {
	if len(addrs) == 0 {
		return &envoy_config_endpoint_v3.ClusterLoadAssignment{ClusterName: name}
	}
	return &envoy_config_endpoint_v3.ClusterLoadAssignment{
		ClusterName: name,
		Endpoints:   Endpoints(addrs...),
	}
}

// externalNameClusterLoadAssignment creates a *envoy_config_endpoint_v3.ClusterLoadAssignment pointing to service's ExternalName DNS address.
func externalNameClusterLoadAssignment(service *dag.Service) *envoy_config_endpoint_v3.ClusterLoadAssignment {
	cla := ClusterLoadAssignment(
		xds.ClusterLoadAssignmentName(
			types.NamespacedName{Name: service.Weighted.ServiceName, Namespace: service.Weighted.ServiceNamespace},
			service.Weighted.ServicePort.Name,
		),
		SocketAddress(service.ExternalName, int(service.Weighted.ServicePort.Port)),
	)
	if service.Weighted.ServicePort.Port != service.Weighted.HealthPort.Port {
		cla.Endpoints[0].LbEndpoints[0].GetEndpoint().HealthCheckConfig = HealthCheckConfig(service.Weighted.HealthPort.Port)
	}
	return cla
}
