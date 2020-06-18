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

// Package grpc provides a gRPC implementation of the Envoy v2 xDS API.
package grpc

import (
	"context"

	api "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v2"
	"github.com/envoyproxy/go-control-plane/pkg/cache/v2"
	xds "github.com/envoyproxy/go-control-plane/pkg/server/v2"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
)

// NewAPI returns a *grpc.Server which responds to the Envoy v2 xDS gRPC API.
func NewAPI(registry *prometheus.Registry, snapshotCache cache.SnapshotCache, opts ...grpc.ServerOption) *grpc.Server {
	metrics := grpc_prometheus.NewServerMetrics()

	registry.MustRegister(metrics)
	opts = append(opts, grpc.StreamInterceptor(metrics.StreamServerInterceptor()),
		grpc.UnaryInterceptor(metrics.UnaryServerInterceptor()))
	g := grpc.NewServer(opts...)

	xdsServer := xds.NewServer(context.Background(), snapshotCache, nil)

	discovery.RegisterAggregatedDiscoveryServiceServer(g, xdsServer)
	discovery.RegisterSecretDiscoveryServiceServer(g, xdsServer)
	api.RegisterEndpointDiscoveryServiceServer(g, xdsServer)
	api.RegisterClusterDiscoveryServiceServer(g, xdsServer)
	api.RegisterRouteDiscoveryServiceServer(g, xdsServer)
	api.RegisterListenerDiscoveryServiceServer(g, xdsServer)

	metrics.InitializeMetrics(g)
	return g
}
