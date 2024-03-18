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
	"errors"
	"fmt"
	"io"
	"testing"

	envoy_config_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/runtime/protoimpl"

	"github.com/projectcontour/contour/internal/fixture"
	"github.com/projectcontour/contour/internal/xds"
)

func TestXDSHandlerStream(t *testing.T) {
	log := fixture.NewDiscardLogger()
	tests := map[string]struct {
		xh     contourServer
		stream grpcStream
		want   error
	}{
		"recv returns error immediately": {
			xh: contourServer{FieldLogger: log},
			stream: &mockStream{
				context: context.Background,
				recv: func() (*envoy_service_discovery_v3.DiscoveryRequest, error) {
					return nil, io.EOF
				},
			},
			want: io.EOF,
		},
		"no registered typeURL": {
			xh: contourServer{FieldLogger: log},
			stream: &mockStream{
				context: context.Background,
				recv: func() (*envoy_service_discovery_v3.DiscoveryRequest, error) {
					return &envoy_service_discovery_v3.DiscoveryRequest{
						TypeUrl: "io.projectcontour.potato",
					}, nil
				},
			},
			want: fmt.Errorf("no resource registered for typeURL %q", "io.projectcontour.potato"),
		},
		"failed to convert values to any": {
			xh: contourServer{
				FieldLogger: log,
				resources: map[string]xds.Resource{
					"io.projectcontour.potato": &mockResource{
						register: func(ch chan int, i int) {
							ch <- i + 1
						},
						contents: func() []proto.Message {
							return []proto.Message{nil}
						},
						typeurl: func() string { return "io.projectcontour.potato" },
					},
				},
			},
			stream: &mockStream{
				context: context.Background,
				recv: func() (*envoy_service_discovery_v3.DiscoveryRequest, error) {
					return &envoy_service_discovery_v3.DiscoveryRequest{
						TypeUrl: "io.projectcontour.potato",
					}, nil
				},
			},
			want: protoimpl.X.NewError("invalid nil source message"),
		},
		"failed to send": {
			xh: contourServer{
				FieldLogger: log,
				resources: map[string]xds.Resource{
					"io.projectcontour.potato": &mockResource{
						register: func(ch chan int, i int) {
							ch <- i + 1
						},
						contents: func() []proto.Message {
							return []proto.Message{new(envoy_config_endpoint_v3.ClusterLoadAssignment)}
						},
						typeurl: func() string { return "io.projectcontour.potato" },
					},
				},
			},
			stream: &mockStream{
				context: context.Background,
				recv: func() (*envoy_service_discovery_v3.DiscoveryRequest, error) {
					return &envoy_service_discovery_v3.DiscoveryRequest{
						TypeUrl: "io.projectcontour.potato",
					}, nil
				},
				send: func(*envoy_service_discovery_v3.DiscoveryResponse) error {
					return io.EOF
				},
			},
			want: io.EOF,
		},
		"context canceled": {
			xh: contourServer{
				FieldLogger: log,
				resources: map[string]xds.Resource{
					"io.projectcontour.potato": &mockResource{
						register: func(chan int, int) {
							// do nothing
						},
						typeurl: func() string { return "io.projectcontour.potato" },
					},
				},
			},
			stream: &mockStream{
				context: func() context.Context {
					ctx := context.Background()
					ctx, cancel := context.WithCancel(ctx)
					cancel()
					return ctx
				},
				recv: func() (*envoy_service_discovery_v3.DiscoveryRequest, error) {
					return &envoy_service_discovery_v3.DiscoveryRequest{
						TypeUrl: "io.projectcontour.potato",
					}, nil
				},
				send: func(*envoy_service_discovery_v3.DiscoveryResponse) error {
					return io.EOF
				},
			},
			want: nil,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := tc.xh.stream(tc.stream)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestStreamLoggingConnectionClose(t *testing.T) {
	log, logHook := test.NewNullLogger()
	log.SetLevel(logrus.DebugLevel)

	tests := map[string]struct {
		closeErr     error
		wantErr      bool
		wantLogLevel logrus.Level
	}{
		"connection closed w/ context.Canceled": {
			closeErr:     context.Canceled,
			wantLogLevel: logrus.DebugLevel,
		},
		"connection closed w/ rpc error": {
			closeErr:     status.Error(codes.Canceled, "error canceled"),
			wantLogLevel: logrus.DebugLevel,
		},
		"connection closed w/ some other error": {
			closeErr:     errors.New("some other error"),
			wantLogLevel: logrus.ErrorLevel,
			wantErr:      true,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			server := contourServer{FieldLogger: log}
			stream := &mockStream{
				context: context.Background,
				recv: func() (*envoy_service_discovery_v3.DiscoveryRequest, error) {
					return nil, tc.closeErr
				},
			}
			err := server.stream(stream)

			assert.Len(t, logHook.AllEntries(), 1)
			entry := logHook.AllEntries()[0]
			assert.Equal(t, "stream terminated", entry.Message)
			assert.Equal(t, tc.wantLogLevel, entry.Level)

			if tc.wantErr {
				assert.Equal(t, tc.closeErr, err)
				assert.Equal(t, tc.closeErr, entry.Data["error"])
			} else {
				require.NoError(t, err)
			}

			logHook.Reset()
		})
	}
}

type mockStream struct {
	context func() context.Context
	send    func(*envoy_service_discovery_v3.DiscoveryResponse) error
	recv    func() (*envoy_service_discovery_v3.DiscoveryRequest, error)
}

func (m *mockStream) Context() context.Context { return m.context() }
func (m *mockStream) Send(resp *envoy_service_discovery_v3.DiscoveryResponse) error {
	return m.send(resp)
}
func (m *mockStream) Recv() (*envoy_service_discovery_v3.DiscoveryRequest, error) { return m.recv() }

type mockResource struct {
	contents func() []proto.Message
	query    func([]string) []proto.Message
	register func(chan int, int)
	typeurl  func() string
}

func (m *mockResource) Contents() []proto.Message                   { return m.contents() }
func (m *mockResource) Query(names []string) []proto.Message        { return m.query(names) }
func (m *mockResource) Register(ch chan int, last int, _ ...string) { m.register(ch, last) }
func (m *mockResource) TypeURL() string                             { return m.typeurl() }
