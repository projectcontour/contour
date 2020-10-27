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

package v2

import (
	api "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v2"
	"google.golang.org/grpc"
)

// Server is a collection of handlers for streaming discovery requests.
type Server interface {
	api.ClusterDiscoveryServiceServer
	api.EndpointDiscoveryServiceServer
	api.ListenerDiscoveryServiceServer
	api.RouteDiscoveryServiceServer
	discovery.AggregatedDiscoveryServiceServer
	discovery.SecretDiscoveryServiceServer
}

// RegisterServer registers the given xDS protocol Server with the gRPC
// runtime. If registry is non-nil gRPC server metrics will be automatically
// configured and enabled.
func RegisterServer(srv Server, g *grpc.Server) {
	// register services
	discovery.RegisterAggregatedDiscoveryServiceServer(g, srv)
	discovery.RegisterSecretDiscoveryServiceServer(g, srv)
	api.RegisterClusterDiscoveryServiceServer(g, srv)
	api.RegisterEndpointDiscoveryServiceServer(g, srv)
	api.RegisterListenerDiscoveryServiceServer(g, srv)
	api.RegisterRouteDiscoveryServiceServer(g, srv)
}
