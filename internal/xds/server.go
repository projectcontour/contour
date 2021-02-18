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

package xds

import (
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
)

// NewServer If registry is non-nil gRPC server metrics will be automatically
// configured and enabled.
func NewServer(registry *prometheus.Registry, opts ...grpc.ServerOption) *grpc.Server {
	var metrics *grpc_prometheus.ServerMetrics

	// TODO: Decouple registry from this.
	if registry != nil {
		metrics = grpc_prometheus.NewServerMetrics()
		registry.MustRegister(metrics)

		opts = append(opts,
			grpc.StreamInterceptor(metrics.StreamServerInterceptor()),
			grpc.UnaryInterceptor(metrics.UnaryServerInterceptor()),
		)
	}

	g := grpc.NewServer(opts...)

	if metrics != nil {
		metrics.InitializeMetrics(g)
	}

	return g
}
