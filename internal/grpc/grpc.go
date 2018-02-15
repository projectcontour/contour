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
	"strconv"
	"sync/atomic"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	envoy_service_v2 "github.com/envoyproxy/go-control-plane/envoy/service/load_stats/v2"

	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"github.com/heptio/contour/internal/contour"
	"github.com/heptio/contour/internal/log"
)

// Resource types in xDS v2.
const (
	googleApis   = "type.googleapis.com/"
	typePrefix   = googleApis + "envoy.api.v2."
	endpointType = typePrefix + "ClusterLoadAssignment"
	clusterType  = typePrefix + "Cluster"
	routeType    = typePrefix + "RouteConfiguration"
	listenerType = typePrefix + "Listener"
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
func NewAPI(l log.Logger, t *contour.Translator) *grpc.Server {
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

func newgrpcServer(l log.Logger, t *contour.Translator) *grpcServer {
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
			HTTP:   &t.VirtualHostCache.HTTP,
			HTTPS:  &t.VirtualHostCache.HTTPS,
			Cond:   &t.VirtualHostCache.Cond,
			Logger: l.WithPrefix("RDS"),
		},
	}
}

// A resourcer provides resources formatted as []types.Any.
type resourcer interface {
	Resources() ([]types.Any, error)
	TypeURL() string
}

// CDS implements the CDS v2 gRPC API.
type CDS struct {
	log.Logger
	ClusterCache
	count uint64
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

func (c *CDS) FetchClusters(context.Context, *v2.DiscoveryRequest) (*v2.DiscoveryResponse, error) {
	return fetch(c, 0, 0)
}

func (c *CDS) StreamClusters(srv v2.ClusterDiscoveryService_StreamClustersServer) (err1 error) {
	log := c.Logger.WithPrefix(fmt.Sprintf("CDS(%06x)", atomic.AddUint64(&c.count, 1)))
	defer func() { log.Infof("stream terminated with error: %v", err1) }()
	return stream(srv, c, log)
}

// EDS implements the EDS v2 gRPC API.
type EDS struct {
	log.Logger
	ClusterLoadAssignmentCache
	count uint64
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

func (e *EDS) FetchEndpoints(context.Context, *v2.DiscoveryRequest) (*v2.DiscoveryResponse, error) {
	return fetch(e, 0, 0)
}

func (e *EDS) StreamEndpoints(srv v2.EndpointDiscoveryService_StreamEndpointsServer) (err1 error) {
	log := e.Logger.WithPrefix(fmt.Sprintf("EDS(%06x)", atomic.AddUint64(&e.count, 1)))
	defer func() { log.Infof("stream terminated with error: %v", err1) }()
	return stream(srv, e, log)
}

func (e *EDS) StreamLoadStats(srv envoy_service_v2.LoadReportingService_StreamLoadStatsServer) error {
	return grpc.Errorf(codes.Unimplemented, "FetchListeners Unimplemented")
}

// LDS implements the LDS v2 gRPC API.
type LDS struct {
	log.Logger
	ListenerCache
	count uint64
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

func (l *LDS) FetchListeners(ctx context.Context, req *v2.DiscoveryRequest) (*v2.DiscoveryResponse, error) {
	return fetch(l, 0, 0)
}

func (l *LDS) StreamListeners(srv v2.ListenerDiscoveryService_StreamListenersServer) (err1 error) {
	log := l.Logger.WithPrefix(fmt.Sprintf("LDS(%06x)", atomic.AddUint64(&l.count, 1)))
	defer func() { log.Infof("stream terminated with error: %v", err1) }()
	return stream(srv, l, log)
}

// RDS implements the RDS v2 gRPC API.
type RDS struct {
	log.Logger
	HTTP, HTTPS interface {
		// Values returns a copy of the contents of the cache.
		// The slice and its contents should be treated as read-only.
		Values() []route.VirtualHost
	}
	*contour.Cond
	count uint64
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

func (r *RDS) FetchRoutes(context.Context, *v2.DiscoveryRequest) (*v2.DiscoveryResponse, error) {
	return fetch(r, 0, 0)
}

func (r *RDS) StreamRoutes(srv v2.RouteDiscoveryService_StreamRoutesServer) (err1 error) {
	log := r.Logger.WithPrefix(fmt.Sprintf("RDS(%06x)", atomic.AddUint64(&r.count, 1)))
	defer func() { log.Infof("stream terminated with error: %v", err1) }()
	return stream(srv, r, log)
}

// fetch returns a *v2.DiscoveryResponse for the current resourcer, typeurl, version and nonce.
func fetch(r resourcer, version, nonce int) (*v2.DiscoveryResponse, error) {
	resources, err := r.Resources()
	return &v2.DiscoveryResponse{
		VersionInfo: strconv.FormatInt(int64(version), 10),
		Resources:   resources,
		TypeUrl:     r.TypeURL(),
		Nonce:       strconv.FormatInt(int64(nonce), 10),
	}, err
}

type grpcStream interface {
	Context() context.Context
	Send(*v2.DiscoveryResponse) error
	Recv() (*v2.DiscoveryRequest, error)
}

type notifier interface {
	resourcer
	Register(chan int, int)
}

// stream streams a *v2.DiscoveryResponses to the receiver.
func stream(st grpcStream, n notifier, log log.Logger) error {
	ch := make(chan int, 1)
	last := 0
	nonce := 0
	ctx := st.Context()
	for {
		log.Infof("waiting for notification, version: %d", last)
		n.Register(ch, last)
		select {
		case last = <-ch:
			log.Infof("notification received version: %d", last)
			out, err := fetch(n, last, nonce)
			if err != nil {
				return err
			}
			if err := st.Send(out); err != nil {
				return err
			}
			nonce++
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
