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
	envoy_service_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/service/cluster/v3"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	envoy_service_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/service/endpoint/v3"
	envoy_service_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/service/listener/v3"
	envoy_service_route_v3 "github.com/envoyproxy/go-control-plane/envoy/service/route/v3"
	envoy_service_runtime_v3 "github.com/envoyproxy/go-control-plane/envoy/service/runtime/v3"
	envoy_service_secret_v3 "github.com/envoyproxy/go-control-plane/envoy/service/secret/v3"
	"google.golang.org/grpc"
)

// Server is a collection of handlers for streaming discovery requests.
type Server interface {
	envoy_service_cluster_v3.ClusterDiscoveryServiceServer
	envoy_service_endpoint_v3.EndpointDiscoveryServiceServer
	envoy_service_listener_v3.ListenerDiscoveryServiceServer
	envoy_service_route_v3.RouteDiscoveryServiceServer
	envoy_service_discovery_v3.AggregatedDiscoveryServiceServer
	envoy_service_secret_v3.SecretDiscoveryServiceServer
	envoy_service_runtime_v3.RuntimeDiscoveryServiceServer
}

// RegisterServer registers the given xDS protocol Server with the gRPC
// runtime.
func RegisterServer(srv Server, g *grpc.Server) {
	// register services
	envoy_service_discovery_v3.RegisterAggregatedDiscoveryServiceServer(g, srv)
	envoy_service_secret_v3.RegisterSecretDiscoveryServiceServer(g, srv)
	envoy_service_cluster_v3.RegisterClusterDiscoveryServiceServer(g, srv)
	envoy_service_endpoint_v3.RegisterEndpointDiscoveryServiceServer(g, srv)
	envoy_service_listener_v3.RegisterListenerDiscoveryServiceServer(g, srv)
	envoy_service_route_v3.RegisterRouteDiscoveryServiceServer(g, srv)
	envoy_service_runtime_v3.RegisterRuntimeDiscoveryServiceServer(g, srv)
}
