// Copyright © 2018 Heptio
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
	"reflect"
	"testing"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/gogo/protobuf/proto"
	"github.com/sirupsen/logrus"
)

func TestXDSHandlerFetch(t *testing.T) {
	log := logrus.New()
	log.SetOutput(ioutil.Discard)
	tests := map[string]struct {
		xh   xdsHandler
		req  *v2.DiscoveryRequest
		want error
	}{
		"no registered typeURL": {
			xh:   xdsHandler{FieldLogger: log},
			req:  &v2.DiscoveryRequest{TypeUrl: "com.heptio.potato"},
			want: fmt.Errorf("no resource registered for typeURL %q", "com.heptio.potato"),
		},
		"failed to convert values to any": {
			xh: xdsHandler{
				FieldLogger: log,
				resources: map[string]resource{
					"com.heptio.potato": &mockResource{
						values: func(fn func(string) bool) []proto.Message {
							return []proto.Message{nil}
						},
						typeurl: func() string { return "com.heptio.potato" },
					},
				},
			},
			req:  &v2.DiscoveryRequest{TypeUrl: "com.heptio.potato"},
			want: fmt.Errorf("proto: Marshal called with nil"),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			_, got := tc.xh.fetch(tc.req)
			if !reflect.DeepEqual(tc.want, got) {
				t.Fatalf("expected: %v, got: %v", tc.want, got)
			}
		})
	}
}

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
				resources: map[string]resource{
					"com.heptio.potato": &mockResource{
						register: func(ch chan int, i int) {
							ch <- i + 1
						},
						values: func(fn func(string) bool) []proto.Message {
							return []proto.Message{nil}
						},
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
				resources: map[string]resource{
					"com.heptio.potato": &mockResource{
						register: func(ch chan int, i int) {
							ch <- i + 1
						},
						values: func(fn func(string) bool) []proto.Message {
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
				resources: map[string]resource{
					"com.heptio.potato": &mockResource{
						register: func(ch chan int, i int) {
							// do nothing
						},
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
			if !reflect.DeepEqual(tc.want, got) {
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
	values   func(func(string) bool) []proto.Message
	register func(chan int, int)
	typeurl  func() string
}

func (m *mockResource) Values(fn func(string) bool) []proto.Message { return m.values(fn) }
func (m *mockResource) Register(ch chan int, last int)              { m.register(ch, last) }
func (m *mockResource) TypeURL() string                             { return m.typeurl() }

func TestToFilter(t *testing.T) {
	tests := map[string]struct {
		names []string
		input []string
		want  []string
	}{
		"empty names": {
			names: nil,
			input: []string{"a", "b", "c"},
			want:  []string{"a", "b", "c"},
		},
		"empty input": {
			names: []string{"a", "b", "c"},
			input: nil,
			want:  []string{},
		},
		"fully matching filter": {
			names: []string{"a", "b", "c"},
			input: []string{"a", "b", "c"},
			want:  []string{"a", "b", "c"},
		},
		"non matching filter": {
			names: []string{"d", "e"},
			input: []string{"a", "b", "c"},
			want:  []string{},
		},
		"partially matching filter": {
			names: []string{"c", "e"},
			input: []string{"a", "b", "c", "d"},
			want:  []string{"c"},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := []string{}
			filter := toFilter(tc.names)
			for _, i := range tc.input {
				if filter(i) {
					got = append(got, i)
				}
			}
			if !reflect.DeepEqual(tc.want, got) {
				t.Fatalf("expected: %v, got: %v", tc.want, got)
			}
		})
	}
}

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
