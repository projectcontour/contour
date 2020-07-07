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

package grpc

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"testing"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/golang/protobuf/proto"
	"github.com/sirupsen/logrus"
)

func TestXDSHandlerStream(t *testing.T) {
	log := logrus.New()
	log.SetOutput(ioutil.Discard)
	tests := map[string]struct {
		xh     xdsHandler
		stream grpcStream
		want   error
	}{
		"recv returns error immediately": {
			xh: xdsHandler{FieldLogger: log},
			stream: &mockStream{
				context: context.Background,
				recv: func() (*v2.DiscoveryRequest, error) {
					return nil, io.EOF
				},
			},
			want: io.EOF,
		},
		"no registered typeURL": {
			xh: xdsHandler{FieldLogger: log},
			stream: &mockStream{
				context: context.Background,
				recv: func() (*v2.DiscoveryRequest, error) {
					return &v2.DiscoveryRequest{
						TypeUrl: "com.heptio.potato",
					}, nil
				},
			},
			want: fmt.Errorf("no resource registered for typeURL %q", "com.heptio.potato"),
		},
		"failed to convert values to any": {
			xh: xdsHandler{
				FieldLogger: log,
				resources: map[string]Resource{
					"com.heptio.potato": &mockResource{
						register: func(ch chan int, i int) {
							ch <- i + 1
						},
						contents: func() []proto.Message {
							return []proto.Message{nil}
						},
						typeurl: func() string { return "com.heptio.potato" },
					},
				},
			},
			stream: &mockStream{
				context: context.Background,
				recv: func() (*v2.DiscoveryRequest, error) {
					return &v2.DiscoveryRequest{
						TypeUrl: "com.heptio.potato",
					}, nil
				},
			},
			want: fmt.Errorf("proto: Marshal called with nil"),
		},
		"failed to send": {
			xh: xdsHandler{
				FieldLogger: log,
				resources: map[string]Resource{
					"com.heptio.potato": &mockResource{
						register: func(ch chan int, i int) {
							ch <- i + 1
						},
						contents: func() []proto.Message {
							return []proto.Message{new(v2.ClusterLoadAssignment)}
						},
						typeurl: func() string { return "com.heptio.potato" },
					},
				},
			},
			stream: &mockStream{
				context: context.Background,
				recv: func() (*v2.DiscoveryRequest, error) {
					return &v2.DiscoveryRequest{
						TypeUrl: "com.heptio.potato",
					}, nil
				},
				send: func(resp *v2.DiscoveryResponse) error {
					return io.EOF
				},
			},
			want: io.EOF,
		},
		"context canceled": {
			xh: xdsHandler{
				FieldLogger: log,
				resources: map[string]Resource{
					"com.heptio.potato": &mockResource{
						register: func(ch chan int, i int) {
							// do nothing
						},
						typeurl: func() string { return "com.heptio.potato" },
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
				recv: func() (*v2.DiscoveryRequest, error) {
					return &v2.DiscoveryRequest{
						TypeUrl: "com.heptio.potato",
					}, nil
				},
				send: func(resp *v2.DiscoveryResponse) error {
					return io.EOF
				},
			},
			want: context.Canceled,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := tc.xh.stream(tc.stream)
			if !equalError(tc.want, got) {
				t.Fatalf("expected: %v, got: %v", tc.want, got)
			}
		})
	}
}

type mockStream struct {
	context func() context.Context
	send    func(*v2.DiscoveryResponse) error
	recv    func() (*v2.DiscoveryRequest, error)
}

func (m *mockStream) Context() context.Context              { return m.context() }
func (m *mockStream) Send(resp *v2.DiscoveryResponse) error { return m.send(resp) }
func (m *mockStream) Recv() (*v2.DiscoveryRequest, error)   { return m.recv() }

type mockResource struct {
	contents func() []proto.Message
	query    func([]string) []proto.Message
	register func(chan int, int)
	typeurl  func() string
}

func (m *mockResource) Contents() []proto.Message                       { return m.contents() }
func (m *mockResource) Query(names []string) []proto.Message            { return m.query(names) }
func (m *mockResource) Register(ch chan int, last int, hints ...string) { m.register(ch, last) }
func (m *mockResource) TypeURL() string                                 { return m.typeurl() }

func TestCounterNext(t *testing.T) {
	var c counter
	// not a map this time as we want tests to execute
	// in sequence.
	tests := []struct {
		fn   func() uint64
		want uint64
	}{{
		fn:   c.next,
		want: 1,
	}, {
		fn:   c.next,
		want: 2,
	}, {
		fn:   c.next,
		want: 3,
	}}

	for _, tc := range tests {
		got := tc.fn()
		if tc.want != got {
			t.Fatalf("expected %d, got %d", tc.want, got)
		}
	}
}

func equalError(a, b error) bool {
	if a == nil {
		return b == nil
	}
	if b == nil {
		return a == nil
	}
	return a.Error() == b.Error()
}
