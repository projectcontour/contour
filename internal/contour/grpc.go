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
	"fmt"
	"strconv"
	"sync/atomic"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	v2 "github.com/envoyproxy/go-control-plane/api"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/heptio/contour/internal/envoy"
	"github.com/heptio/contour/internal/log"
)

// Resource types in xDS v2.
const (
	typePrefix   = "type.googleapis.com/envoy.api.v2."
	EndpointType = typePrefix + "ClusterLoadAssignment"
	ClusterType  = typePrefix + "Cluster"
	RouteType    = typePrefix + "RouteConfiguration"
	ListenerType = typePrefix + "Listener"
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

// VirtualHostCache holds a set of computed v2.VirtualHost resources.
type VirtualHostCache interface {
	// Values returns a copy of the contents of the cache.
	// The slice and its contents should be treated as read-only.
	Values() []*v2.VirtualHost

	// Register registers ch to receive a value when Notify is called.
	Register(chan int, int)
}

// NewGPRCAPI returns a *grpc.Server which responds to the Envoy v2 xDS gRPC API.
func NewGRPCAPI(l log.Logger, t *envoy.Translator) *grpc.Server {
	g := grpc.NewServer()
	s := newgrpcServer(l, t)
	v2.RegisterClusterDiscoveryServiceServer(g, s)
	v2.RegisterEndpointDiscoveryServiceServer(g, s)
	v2.RegisterListenerDiscoveryServiceServer(g, s)
	v2.RegisterRouteDiscoveryServiceServer(g, s)
	return g
}

type grpcServer struct {
	CDS
	EDS
	LDS
	RDS
}

func newgrpcServer(l log.Logger, t *envoy.Translator) *grpcServer {
	return &grpcServer{
		CDS: CDS{
			ClusterCache: &t.ClusterCache,
			Logger:       l.WithPrefix("CDS"),
		},
		EDS: EDS{
			ClusterLoadAssignmentCache: &t.ClusterLoadAssignmentCache,
			Logger: l.WithPrefix("EDS"),
		},
		LDS: LDS{
			ListenerCache: &t.ListenerCache,
			Logger:        l.WithPrefix("LDS"),
		},
		RDS: RDS{
			VirtualHostCache: &t.VirtualHostCache,
			Logger:           l.WithPrefix("RDS"),
		},
	}
}

// CDS implements the CDS v2 gRPC API.
type CDS struct {
	log.Logger
	ClusterCache
	count uint64
}

func (c *CDS) FetchClusters(_ context.Context, req *v2.DiscoveryRequest) (*v2.DiscoveryResponse, error) {
	return nil, grpc.Errorf(codes.Unimplemented, "FetchClusters Unimplemented")
}

func (c *CDS) StreamClusters(srv v2.ClusterDiscoveryService_StreamClustersServer) (err1 error) {
	log := c.Logger.WithPrefix(fmt.Sprintf("CDS(%06x)", atomic.AddUint64(&c.count, 1)))
	defer func() { log.Infof("stream terminated with error: %v", err1) }()
	ch := make(chan int, 1)
	last := 0
	ctx := srv.Context()
	nonce := 0
	for {
		log.Infof("waiting for notification, version: %d", last)
		c.Register(ch, last)
		select {
		case last = <-ch:
			log.Infof("notification received version: %d", last)
			v := c.Values()
			var resources []*any.Any
			for i := range v {
				data, err := proto.Marshal(v[i])
				if err != nil {
					return err
				}
				resources = append(resources, &any.Any{
					TypeUrl: ClusterType,
					Value:   data,
				})
			}
			nonce++
			out := v2.DiscoveryResponse{
				VersionInfo: strconv.FormatInt(int64(last), 10),
				Resources:   resources,
				TypeUrl:     ClusterType,
				Nonce:       strconv.FormatInt(int64(nonce), 10),
			}
			if err := srv.Send(&out); err != nil {
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// EDS implements the EDS v2 gRPC API.
type EDS struct {
	log.Logger
	ClusterLoadAssignmentCache
	count uint64
}

func (e *EDS) FetchEndpoints(_ context.Context, req *v2.DiscoveryRequest) (*v2.DiscoveryResponse, error) {
	return nil, grpc.Errorf(codes.Unimplemented, "FetchEndpoints Unimplemented")
}

func (e *EDS) StreamEndpoints(srv v2.EndpointDiscoveryService_StreamEndpointsServer) (err1 error) {
	log := e.Logger.WithPrefix(fmt.Sprintf("EDS(%06x)", atomic.AddUint64(&e.count, 1)))
	defer func() { log.Infof("stream terminated with error: %v", err1) }()
	ch := make(chan int, 1)
	last := 0

	ctx := srv.Context()
	nonce := 0
	for {
		log.Infof("waiting for notification, version: %d", last)
		e.Register(ch, last)

		select {
		case last = <-ch:
			log.Infof("notification received version: %d", last)
			v := e.Values()
			var resources []*any.Any
			for i := range v {
				data, err := proto.Marshal(v[i])
				if err != nil {
					return err
				}
				resources = append(resources, &any.Any{
					TypeUrl: EndpointType,
					Value:   data,
				})
			}
			nonce++
			out := v2.DiscoveryResponse{
				VersionInfo: strconv.FormatInt(int64(last), 10),
				Resources:   resources,
				TypeUrl:     EndpointType,
				Nonce:       strconv.FormatInt(int64(nonce), 10),
			}
			if err := srv.Send(&out); err != nil {
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (e *EDS) StreamLoadStats(srv v2.EndpointDiscoveryService_StreamLoadStatsServer) error {
	return grpc.Errorf(codes.Unimplemented, "FetchListeners Unimplemented")
}

// LDS implements the LDS v2 gRPC API.
type LDS struct {
	log.Logger
	ListenerCache
	count uint64
}

func (l *LDS) FetchListeners(context.Context, *v2.DiscoveryRequest) (*v2.DiscoveryResponse, error) {
	return nil, grpc.Errorf(codes.Unimplemented, "FetchListeners Unimplemented")
}

func (l *LDS) StreamListeners(srv v2.ListenerDiscoveryService_StreamListenersServer) (err1 error) {
	log := l.Logger.WithPrefix(fmt.Sprintf("LDS(%06x)", atomic.AddUint64(&l.count, 1)))
	defer func() { log.Infof("stream terminated with error: %v", err1) }()
	ch := make(chan int, 1)
	last := 0

	ctx := srv.Context()
	nonce := 0
	for {
		log.Infof("waiting for notification, version: %d", last)
		l.Register(ch, last)

		select {
		case last = <-ch:
			log.Infof("notification received version: %d", last)
			v := l.Values()
			var resources []*any.Any
			for i := range v {
				data, err := proto.Marshal(v[i])
				if err != nil {
					return err
				}
				resources = append(resources, &any.Any{
					TypeUrl: ListenerType,
					Value:   data,
				})
			}
			nonce++
			out := v2.DiscoveryResponse{
				VersionInfo: strconv.FormatInt(int64(last), 10),
				Resources:   resources,
				TypeUrl:     ListenerType,
				Nonce:       strconv.FormatInt(int64(nonce), 10),
			}
			if err := srv.Send(&out); err != nil {
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// RDS implements the RDS v2 gRPC API.
type RDS struct {
	log.Logger
	VirtualHostCache
	count uint64
}

func (r *RDS) FetchRoutes(context.Context, *v2.DiscoveryRequest) (*v2.DiscoveryResponse, error) {
	return nil, grpc.Errorf(codes.Unimplemented, "FetchRoutes Unimplemented")
}

func (r *RDS) StreamRoutes(srv v2.RouteDiscoveryService_StreamRoutesServer) (err1 error) {
	log := r.Logger.WithPrefix(fmt.Sprintf("RDS(%06x)", atomic.AddUint64(&r.count, 1)))
	defer func() { log.Infof("stream terminated with error: %v", err1) }()
	ch := make(chan int, 1)
	last := 0

	ctx := srv.Context()
	nonce := 0
	for {
		log.Infof("waiting for notification, version: %d", last)
		r.Register(ch, last)

		select {
		case last = <-ch:
			log.Infof("notification received version: %d", last)
			var resources []*any.Any
			rc := v2.RouteConfiguration{
				Name:         "ingress_http", // TODO(dfc) matches LDS configuration?
				VirtualHosts: r.Values(),
			}
			data, err := proto.Marshal(&rc)
			if err != nil {
				return err
			}
			resources = append(resources, &any.Any{
				TypeUrl: RouteType,
				Value:   data,
			})
			nonce++
			out := v2.DiscoveryResponse{
				VersionInfo: strconv.FormatInt(int64(last), 10),
				Resources:   resources,
				TypeUrl:     RouteType,
				Nonce:       strconv.FormatInt(int64(nonce), 10),
			}
			if err := srv.Send(&out); err != nil {
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
