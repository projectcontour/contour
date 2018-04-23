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

// Package grpc provides a gRPC implementation of the Envoy v2 xDS API.
package grpc

import (
	"context"
	"fmt"
	"sync/atomic"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_service_v2 "github.com/envoyproxy/go-control-plane/envoy/service/load_stats/v2"
	"github.com/sirupsen/logrus"

	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"github.com/heptio/contour/internal/contour"
)

const (
	// somewhat arbitrary limit to handle many, many, EDS streams
	grpcMaxConcurrentStreams = 1 << 20
)

// NewAPI returns a *grpc.Server which responds to the Envoy v2 xDS gRPC API.
func NewAPI(log logrus.FieldLogger, t *contour.Translator) *grpc.Server {
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
	s := newgrpcServer(log, t)
	v2.RegisterClusterDiscoveryServiceServer(g, s)
	v2.RegisterEndpointDiscoveryServiceServer(g, s)
	v2.RegisterListenerDiscoveryServiceServer(g, s)
	v2.RegisterRouteDiscoveryServiceServer(g, s)
	return g
}

type grpcServer struct {
	logrus.FieldLogger
	count     uint64              // connection count, incremented atomically
	resources map[string]resource // registered resource types
}

func newgrpcServer(log logrus.FieldLogger, t *contour.Translator) *grpcServer {
	return &grpcServer{
		FieldLogger: log,
		resources: map[string]resource{
			clusterType: &CDS{
				cache: &t.ClusterCache,
			},
			endpointType: &EDS{
				cache: &t.ClusterLoadAssignmentCache,
			},
			listenerType: &LDS{
				cache: &t.ListenerCache,
			},
			routeType: &RDS{
				HTTP:  &t.VirtualHostCache.HTTP,
				HTTPS: &t.VirtualHostCache.HTTPS,
				Cond:  &t.VirtualHostCache.Cond,
			},
		},
	}
}

// A resource provides resources formatted as []types.Any.
type resource interface {
	// Values returns a slice of proto.Message implementations that match
	// the provided filter.
	Values(func(string) bool) []proto.Message

	// TypeURL returns the typeURL of messages returned from Values.
	TypeURL() string

	// Register registers the channel for change notifications.
	Register(chan int, int)
}

func (s *grpcServer) FetchClusters(_ context.Context, req *v2.DiscoveryRequest) (*v2.DiscoveryResponse, error) {
	return s.fetch(req)
}

func (s *grpcServer) FetchEndpoints(_ context.Context, req *v2.DiscoveryRequest) (*v2.DiscoveryResponse, error) {
	return s.fetch(req)
}

func (s *grpcServer) FetchListeners(_ context.Context, req *v2.DiscoveryRequest) (*v2.DiscoveryResponse, error) {
	return s.fetch(req)
}

func (s *grpcServer) FetchRoutes(_ context.Context, req *v2.DiscoveryRequest) (*v2.DiscoveryResponse, error) {
	return s.fetch(req)
}

// fetch handles a single DiscoveryRequest.
func (s *grpcServer) fetch(req *v2.DiscoveryRequest) (*v2.DiscoveryResponse, error) {
	s.WithField("connection", atomic.AddUint64(&s.count, 1)).WithField("version_info", req.VersionInfo).WithField("resource_names", req.ResourceNames).WithField("type_url", req.TypeUrl).WithField("response_nonce", req.ResponseNonce).WithField("error_detail", req.ErrorDetail).Info("fetch")
	r, ok := s.resources[req.TypeUrl]
	if !ok {
		return nil, fmt.Errorf("no resource registered for typeURL %q", req.TypeUrl)
	}
	filter := func(string) bool { return true }
	resources, err := toAny(r, filter)
	return &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources:   resources,
		TypeUrl:     r.TypeURL(),
		Nonce:       "0",
	}, err
}

func (s *grpcServer) StreamClusters(srv v2.ClusterDiscoveryService_StreamClustersServer) error {
	return s.stream(srv)
}

func (s *grpcServer) StreamEndpoints(srv v2.EndpointDiscoveryService_StreamEndpointsServer) error {
	return s.stream(srv)
}

func (s *grpcServer) StreamLoadStats(srv envoy_service_v2.LoadReportingService_StreamLoadStatsServer) error {
	return grpc.Errorf(codes.Unimplemented, "FetchListeners Unimplemented")
}

func (s *grpcServer) StreamListeners(srv v2.ListenerDiscoveryService_StreamListenersServer) error {
	return s.stream(srv)
}

func (s *grpcServer) StreamRoutes(srv v2.RouteDiscoveryService_StreamRoutesServer) error {
	return s.stream(srv)
}

type grpcStream interface {
	Context() context.Context
	Send(*v2.DiscoveryResponse) error
	Recv() (*v2.DiscoveryRequest, error)
}

// stream processes a stream of DiscoveryRequests.
func (s *grpcServer) stream(st grpcStream) (err error) {
	log := s.WithField("connection", atomic.AddUint64(&s.count, 1))
	defer func() {
		if err != nil {
			log.WithError(err).Error("stream terminated")
		} else {
			log.Info("stream terminated")
		}
	}()

	ch := make(chan int, 1)
	last := 0
	ctx := st.Context()
	for {
		req, err := st.Recv()
		if err != nil {
			return err
		}
		r, ok := s.resources[req.TypeUrl]
		if !ok {
			return fmt.Errorf("no resource registered for typeURL %q", req.TypeUrl)
		}
		log.WithField("version_info", req.VersionInfo).WithField("resource_names", req.ResourceNames).WithField("type_url", req.TypeUrl).WithField("response_nonce", req.ResponseNonce).WithField("error_detail", req.ErrorDetail).Info("stream_wait")

		r.Register(ch, last)
		select {
		case last = <-ch:
			filter := func(string) bool { return true }
			resources, err := toAny(r, filter)
			if err != nil {
				return err
			}
			resp := &v2.DiscoveryResponse{
				VersionInfo: "0",
				Resources:   resources,
				TypeUrl:     r.TypeURL(),
				Nonce:       "0",
			}
			if err := st.Send(resp); err != nil {
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// toAny converts the contens of a resourcer's Values to the
// respective slice of types.Any.
func toAny(res resource, filter func(string) bool) ([]types.Any, error) {
	v := res.Values(filter)
	resources := make([]types.Any, len(v))
	for i := range v {
		value, err := proto.Marshal(v[i])
		if err != nil {
			return nil, err
		}
		resources[i] = types.Any{TypeUrl: res.TypeURL(), Value: value}
	}
	return resources, nil
}
