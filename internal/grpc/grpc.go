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
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	envoy_service_v2 "github.com/envoyproxy/go-control-plane/envoy/service/load_stats/v2"
	"github.com/sirupsen/logrus"

	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"github.com/heptio/contour/internal/contour"
)

// Resource types in xDS v2.
const (
	googleApis   = "type.googleapis.com/"
	typePrefix   = googleApis + "envoy.api.v2."
	endpointType = typePrefix + "ClusterLoadAssignment"
	clusterType  = typePrefix + "Cluster"
	routeType    = typePrefix + "RouteConfiguration"
	listenerType = typePrefix + "Listener"

	// somewhat arbitrary limit to handle many, many, EDS streams
	grpcMaxConcurrentStreams = 1 << 20
)

// ClusterCache holds a set of computed v2.Cluster resources.
type ClusterCache interface {
	// Values returns a copy of the contents of the cache.
	// The slice and its contents should be treated as read-only.
	Values() []*v2.Cluster

	// Register registers ch to receive a value when Notify is called.
	Register(chan int, int)
}

// ClusterLoadAssignmentCache holds a set of computed v2.ClusterLoadAssignment resources.
type ClusterLoadAssignmentCache interface {
	// Values returns a copy of the contents of the cache.
	// The slice and its contents should be treated as read-only.
	Values() []*v2.ClusterLoadAssignment

	// Register registers ch to receive a value when Notify is called.
	Register(chan int, int)
}

// ListenerCache holds a set of computed v2.Listener resources.
type ListenerCache interface {
	// Values returns a copy of the contents of the cache.
	// The slice and its contents should be treated as read-only.
	Values() []*v2.Listener

	// Register registers ch to receive a value when Notify is called.
	Register(chan int, int)
}

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
	count     uint64               // connection count, incremented atomically
	resources map[string]resourcer // registered resource types
}

func newgrpcServer(log logrus.FieldLogger, t *contour.Translator) *grpcServer {
	return &grpcServer{
		FieldLogger: log,
		resources: map[string]resourcer{
			clusterType: &CDS{
				ClusterCache: &t.ClusterCache,
			},
			endpointType: &EDS{
				ClusterLoadAssignmentCache: &t.ClusterLoadAssignmentCache,
			},
			listenerType: &LDS{
				ListenerCache: &t.ListenerCache,
			},
			routeType: &RDS{
				HTTP:  &t.VirtualHostCache.HTTP,
				HTTPS: &t.VirtualHostCache.HTTPS,
				Cond:  &t.VirtualHostCache.Cond,
			},
		},
	}
}

// A resourcer provides resources formatted as []types.Any.
type resourcer interface {
	Resources() ([]types.Any, error)
	TypeURL() string
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

func (s *grpcServer) fetch(req *v2.DiscoveryRequest) (*v2.DiscoveryResponse, error) {
	s.WithField("version_info", req.VersionInfo).WithField("resource_names", req.ResourceNames).WithField("type_url", req.TypeUrl).WithField("response_nonce", req.ResponseNonce).WithField("error_detail", req.ErrorDetail).Info("fetch")
	r, ok := s.resources[req.TypeUrl]
	if !ok {
		return nil, fmt.Errorf("no resourcer registered for typeURL %q", req.TypeUrl)
	}
	resources, err := r.Resources()
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
			return fmt.Errorf("no resourcer registered for typeURL %q", req.TypeUrl)
		}
		log.WithField("version_info", req.VersionInfo).WithField("resource_names", req.ResourceNames).WithField("type_url", req.TypeUrl).WithField("response_nonce", req.ResponseNonce).WithField("error_detail", req.ErrorDetail).Info("stream request")

		r.Register(ch, last)
		select {
		case last = <-ch:
			out, err := s.fetch(req)
			if err != nil {
				return err
			}
			if err := st.Send(out); err != nil {
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// CDS implements the CDS v2 gRPC API.
type CDS struct {
	ClusterCache
}

// Resources returns the contents of CDS"s cache as a []types.Any.
// TODO(dfc) cache the results of Resources in the ClusterCache so
// we can avoid the error handling.
func (c *CDS) Resources() ([]types.Any, error) {
	v := c.Values()
	resources := make([]types.Any, len(v))
	for i := range v {
		value, err := proto.Marshal(v[i])
		if err != nil {
			return nil, err
		}
		resources[i] = types.Any{TypeUrl: c.TypeURL(), Value: value}
	}
	return resources, nil
}

func (c *CDS) TypeURL() string { return clusterType }

// EDS implements the EDS v2 gRPC API.
type EDS struct {
	ClusterLoadAssignmentCache
}

// Resources returns the contents of EDS"s cache as a []types.Any.
// TODO(dfc) cache the results of Resources in the ClusterLoadAssignmentCache so
// we can avoid the error handling.
func (e *EDS) Resources() ([]types.Any, error) {
	v := e.Values()
	resources := make([]types.Any, len(v))
	for i := range v {
		value, err := proto.Marshal(v[i])
		if err != nil {
			return nil, err
		}
		resources[i] = types.Any{TypeUrl: e.TypeURL(), Value: value}
	}
	return resources, nil
}

func (e *EDS) TypeURL() string { return endpointType }

// LDS implements the LDS v2 gRPC API.
type LDS struct {
	ListenerCache
}

// Resources returns the contents of LDS"s cache as a []types.Any.
// TODO(dfc) cache the results of Resources in the ListenerCache so
// we can avoid the error handling.
func (l *LDS) Resources() ([]types.Any, error) {
	v := l.Values()
	resources := make([]types.Any, len(v))
	for i := range v {
		value, err := proto.Marshal(v[i])
		if err != nil {
			return nil, err
		}
		resources[i] = types.Any{TypeUrl: l.TypeURL(), Value: value}
	}
	return resources, nil
}

func (l *LDS) TypeURL() string { return listenerType }

// RDS implements the RDS v2 gRPC API.
type RDS struct {
	HTTP, HTTPS interface {
		// Values returns a copy of the contents of the cache.
		// The slice and its contents should be treated as read-only.
		Values() []route.VirtualHost
	}
	*contour.Cond
}

// Resources returns the contents of RDS"s cache as a []types.Any.
// TODO(dfc) cache the results of Resources in the VirtualHostCache so
// we can avoid the error handling.
func (r *RDS) Resources() ([]types.Any, error) {
	ingress_http, err := proto.Marshal(&v2.RouteConfiguration{
		Name:         "ingress_http", // TODO(dfc) matches LDS configuration?
		VirtualHosts: r.HTTP.Values(),
	})
	if err != nil {
		return nil, err
	}
	ingress_https, err := proto.Marshal(&v2.RouteConfiguration{

		Name:         "ingress_https", // TODO(dfc) matches LDS configuration?
		VirtualHosts: r.HTTPS.Values(),
	})
	if err != nil {
		return nil, err
	}
	return []types.Any{{
		TypeUrl: r.TypeURL(), Value: ingress_http,
	}, {
		TypeUrl: r.TypeURL(), Value: ingress_https,
	}}, nil
}

func (r *RDS) TypeURL() string { return routeType }

type grpcStream interface {
	Context() context.Context
	Send(*v2.DiscoveryResponse) error
	Recv() (*v2.DiscoveryRequest, error)
}

type notifier interface {
	resourcer
}
