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
	"context"
	"fmt"
	"strconv"

	envoy_service_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/service/cluster/v3"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	envoy_service_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/service/endpoint/v3"
	envoy_service_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/service/listener/v3"
	envoy_service_route_v3 "github.com/envoyproxy/go-control-plane/envoy/service/route/v3"
	envoy_service_secret_v3 "github.com/envoyproxy/go-control-plane/envoy/service/secret/v3"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/projectcontour/contour/internal/xds"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/types/known/anypb"
)

type grpcStream interface {
	Context() context.Context
	Send(*envoy_service_discovery_v3.DiscoveryResponse) error
	Recv() (*envoy_service_discovery_v3.DiscoveryRequest, error)
}

// NewContourServer creates an internally implemented Server that streams the
// provided set of Resource objects. The returned Server implements the xDS
// State of the World (SotW) variant.
func NewContourServer(log logrus.FieldLogger, resources ...xds.Resource) Server {
	c := contourServer{
		FieldLogger: log,
		resources:   map[string]xds.Resource{},
	}

	for i, r := range resources {
		c.resources[r.TypeURL()] = resources[i]
	}

	return &c
}

type contourServer struct {
	// Since we only implement the streaming state of the world
	// protocol, embed the default null implementations to handle
	// the unimplemented gRPC endpoints.
	envoy_service_discovery_v3.UnimplementedAggregatedDiscoveryServiceServer
	envoy_service_secret_v3.UnimplementedSecretDiscoveryServiceServer
	envoy_service_route_v3.UnimplementedRouteDiscoveryServiceServer
	envoy_service_endpoint_v3.UnimplementedEndpointDiscoveryServiceServer
	envoy_service_cluster_v3.UnimplementedClusterDiscoveryServiceServer
	envoy_service_listener_v3.UnimplementedListenerDiscoveryServiceServer

	logrus.FieldLogger
	resources   map[string]xds.Resource
	connections xds.Counter
}

// stream processes a stream of DiscoveryRequests.
func (s *contourServer) stream(st grpcStream) error {
	// Bump connection counter and set it as a field on the logger.
	log := s.WithField("connection", s.connections.Next())

	// Notify whether the stream terminated on error.
	done := func(log logrus.FieldLogger, err error) error {
		if err != nil {
			log.WithError(err).Error("stream terminated")
		} else {
			log.Info("stream terminated")
		}

		return err
	}

	ch := make(chan int, 1)

	// internally all registration values start at zero so sending
	// a last that is less than zero will guarantee that each stream
	// will generate a response immediately, then wait.
	last := -1
	ctx := st.Context()

	// now stick in this loop until the client disconnects.
	for {
		// first we wait for the request from Envoy, this is part of
		// the xDS protocol.
		req, err := st.Recv()
		if err != nil {
			return done(log, err)
		}

		// Note: redeclare log in this scope so the next time around the loop all is forgotten.
		log := logDiscoveryRequestDetails(log, req)

		// From the request we derive the resource to stream which have
		// been registered according to the typeURL.
		r, ok := s.resources[req.GetTypeUrl()]
		if !ok {
			return done(log, fmt.Errorf("no resource registered for typeURL %q", req.GetTypeUrl()))
		}

		// now we wait for a notification, if this is the first request received on this
		// connection last will be less than zero and that will trigger a response immediately.
		r.Register(ch, last, req.ResourceNames...)
		select {
		case last = <-ch:
			// boom, something in the cache has changed.
			// TODO(dfc) the thing that has changed may not be in the scope of the filter
			// so we're going to be sending an update that is a no-op. See #426

			var resources []proto.Message
			switch len(req.ResourceNames) {
			case 0:
				// no resource hints supplied, return the full
				// contents of the resource
				resources = r.Contents()
			default:
				// resource hints supplied, return exactly those
				resources = r.Query(req.ResourceNames)
			}

			any := make([]*any.Any, 0, len(resources))
			for _, r := range resources {
				a, err := anypb.New(proto.MessageV2(r))
				if err != nil {
					return done(log, err)
				}
				any = append(any, a)
			}

			resp := &envoy_service_discovery_v3.DiscoveryResponse{
				VersionInfo: strconv.Itoa(last),
				Resources:   any,
				TypeUrl:     req.GetTypeUrl(),
				Nonce:       strconv.Itoa(last),
			}

			if err := st.Send(resp); err != nil {
				return done(log, err)
			}

		case <-ctx.Done():
			return done(log, ctx.Err())
		}
	}
}

func (s *contourServer) StreamClusters(srv envoy_service_cluster_v3.ClusterDiscoveryService_StreamClustersServer) error {
	return s.stream(srv)
}

func (s *contourServer) StreamEndpoints(srv envoy_service_endpoint_v3.EndpointDiscoveryService_StreamEndpointsServer) error {
	return s.stream(srv)
}

func (s *contourServer) StreamListeners(srv envoy_service_listener_v3.ListenerDiscoveryService_StreamListenersServer) error {
	return s.stream(srv)
}

func (s *contourServer) StreamRoutes(srv envoy_service_route_v3.RouteDiscoveryService_StreamRoutesServer) error {
	return s.stream(srv)
}

func (s *contourServer) StreamSecrets(srv envoy_service_secret_v3.SecretDiscoveryService_StreamSecretsServer) error {
	return s.stream(srv)
}
