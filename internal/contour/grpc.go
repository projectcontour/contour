// Copyright © 2017 Heptio
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
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	v2 "github.com/envoyproxy/go-control-plane/api"
	"github.com/heptio/contour/internal/log"
)

// NewGPRCAPI returns a *grpc.Server which responds to the Envoy v2 xDS gRPC API.
func NewGRPCAPI(l log.Logger) *grpc.Server {
	a := &grpcAPI{
		Logger: l,
	}
	s := grpc.NewServer()
	v2.RegisterClusterDiscoveryServiceServer(s, a)
	v2.RegisterEndpointDiscoveryServiceServer(s, a)
	v2.RegisterListenerDiscoveryServiceServer(s, a)
	v2.RegisterRouteDiscoveryServiceServer(s, a)
	return s
}

type grpcAPI struct {
	log.Logger
}

func (g *grpcAPI) FetchClusters(_ context.Context, req *v2.DiscoveryRequest) (*v2.DiscoveryResponse, error) {
	return nil, grpc.Errorf(codes.Unimplemented, "FetchClusters Unimplemented")
}

func (g *grpcAPI) StreamClusters(v2.ClusterDiscoveryService_StreamClustersServer) error {
	return grpc.Errorf(codes.Unimplemented, "StreamClusters Unimplemented")
}

func (g *grpcAPI) FetchEndpoints(_ context.Context, req *v2.DiscoveryRequest) (*v2.DiscoveryResponse, error) {
	return nil, grpc.Errorf(codes.Unimplemented, "FetchEndpoints Unimplemented")
}

func (g *grpcAPI) StreamEndpoints(v2.EndpointDiscoveryService_StreamEndpointsServer) error {
	return grpc.Errorf(codes.Unimplemented, "StreamEndpoints Unimplemented")
}

func (g *grpcAPI) StreamLoadStats(v2.EndpointDiscoveryService_StreamLoadStatsServer) error {
	return grpc.Errorf(codes.Unimplemented, "StreamLoadStats Unimplemented")
}

func (g *grpcAPI) FetchListeners(context.Context, *v2.DiscoveryRequest) (*v2.DiscoveryResponse, error) {
	return nil, grpc.Errorf(codes.Unimplemented, "FetchListeners Unimplemented")
}

func (g *grpcAPI) StreamListeners(v2.ListenerDiscoveryService_StreamListenersServer) error {
	return grpc.Errorf(codes.Unimplemented, "StreamListeners Unimplemented")
}

func (g *grpcAPI) FetchRoutes(context.Context, *v2.DiscoveryRequest) (*v2.DiscoveryResponse, error) {
	return nil, grpc.Errorf(codes.Unimplemented, "FetchRoutes Unimplemented")
}

func (g *grpcAPI) StreamRoutes(v2.RouteDiscoveryService_StreamRoutesServer) error {
	return grpc.Errorf(codes.Unimplemented, "StreamRoutes Unimplemented")
}
