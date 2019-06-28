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

// Package grpc provides a gRPC implementation of the Envoy v2 xDS API.
package grpc

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2"
	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v2"
	loadstats "github.com/envoyproxy/go-control-plane/envoy/service/load_stats/v2"
	"github.com/sirupsen/logrus"
)

const (
	// somewhat arbitrary limit to handle many, many, EDS streams
	grpcMaxConcurrentStreams = 1 << 20
)

// NewAPI returns a *grpc.Server which responds to the Envoy v2 xDS gRPC API.
func NewAPI(log logrus.FieldLogger, resources map[string]Resource) *grpc.Server {
	opts := []grpc.ServerOption{
		// By default the Go grpc library defaults to a value of ~100 streams per
		// connection. This number is likely derived from the HTTP/2 spec:
		// https://http2.github.io/http2-spec/#SettingValues
		// We need to raise this value because Envoy will open one EDS stream per
		// CDS entry. There doesn't seem to be a penalty for increasing this value,
		// so set it the limit similar to envoyproxy/go-control-plane#70.
		grpc.MaxConcurrentStreams(grpcMaxConcurrentStreams),
	}
	g := grpc.NewServer(opts...)
	s := &grpcServer{
		xdsHandler{
			FieldLogger: log,
			resources:   resources,
		},
	}

	v2.RegisterClusterDiscoveryServiceServer(g, s)
	v2.RegisterEndpointDiscoveryServiceServer(g, s)
	v2.RegisterListenerDiscoveryServiceServer(g, s)
	v2.RegisterRouteDiscoveryServiceServer(g, s)
	discovery.RegisterSecretDiscoveryServiceServer(g, s)
	return g
}

// grpcServer implements the LDS, RDS, CDS, and EDS, gRPC endpoints.
type grpcServer struct {
	xdsHandler
}

func (s *grpcServer) FetchClusters(_ context.Context, req *v2.DiscoveryRequest) (*v2.DiscoveryResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "FetchClusters unimplemented")
}

func (s *grpcServer) FetchEndpoints(_ context.Context, req *v2.DiscoveryRequest) (*v2.DiscoveryResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "FetchEndpoints unimplemented")
}

func (s *grpcServer) DeltaEndpoints(v2.EndpointDiscoveryService_DeltaEndpointsServer) error {
	return status.Errorf(codes.Unimplemented, "DeltaEndpoints unimplemented")
}

func (s *grpcServer) FetchListeners(_ context.Context, req *v2.DiscoveryRequest) (*v2.DiscoveryResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "FetchListeners unimplemented")
}

func (s *grpcServer) DeltaListeners(v2.ListenerDiscoveryService_DeltaListenersServer) error {
	return status.Errorf(codes.Unimplemented, "DeltaListeners unimplemented")
}

func (s *grpcServer) FetchRoutes(_ context.Context, req *v2.DiscoveryRequest) (*v2.DiscoveryResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "FetchRoutes unimplemented")
}

func (s *grpcServer) FetchSecrets(_ context.Context, req *v2.DiscoveryRequest) (*v2.DiscoveryResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "FetchSecrets unimplemented")
}

func (s *grpcServer) DeltaSecrets(discovery.SecretDiscoveryService_DeltaSecretsServer) error {
	return status.Errorf(codes.Unimplemented, "DeltaSecrets unimplemented")
}

func (s *grpcServer) StreamClusters(srv v2.ClusterDiscoveryService_StreamClustersServer) error {
	return s.stream(srv)
}

func (s *grpcServer) StreamEndpoints(srv v2.EndpointDiscoveryService_StreamEndpointsServer) error {
	return s.stream(srv)
}

func (s *grpcServer) StreamLoadStats(srv loadstats.LoadReportingService_StreamLoadStatsServer) error {
	return status.Errorf(codes.Unimplemented, "StreamLoadStats unimplemented")
}

func (s *grpcServer) DeltaClusters(v2.ClusterDiscoveryService_DeltaClustersServer) error {
	return status.Errorf(codes.Unimplemented, "IncrementalClusters unimplemented")
}

func (s *grpcServer) DeltaRoutes(v2.RouteDiscoveryService_DeltaRoutesServer) error {
	return status.Errorf(codes.Unimplemented, "IncrementalRoutes unimplemented")
}

func (s *grpcServer) StreamListeners(srv v2.ListenerDiscoveryService_StreamListenersServer) error {
	return s.stream(srv)
}

func (s *grpcServer) StreamRoutes(srv v2.RouteDiscoveryService_StreamRoutesServer) error {
	return s.stream(srv)
}

func (s *grpcServer) StreamSecrets(srv discovery.SecretDiscoveryService_StreamSecretsServer) error {
	return s.stream(srv)
}
