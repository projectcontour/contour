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

package grpc

import (
	"context"
	"net"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/heptio/contour/internal/contour"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestGRPCStreaming(t *testing.T) {
	var l net.Listener

	// tr is recreated before the start of each test.
	var tr *contour.Translator

	newClient := func(t *testing.T) *grpc.ClientConn {
		cc, err := grpc.Dial(l.Addr().String(), grpc.WithInsecure())
		check(t, err)
		return cc
	}

	tests := map[string]func(*testing.T){
		"StreamClusters": func(t *testing.T) {
			tr.OnAdd(&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "simple",
					Namespace: "default",
				},
				Spec: v1.ServiceSpec{
					Selector: map[string]string{
						"app": "simple",
					},
					Ports: []v1.ServicePort{{
						Protocol:   "TCP",
						Port:       80,
						TargetPort: intstr.FromInt(6502),
					}},
				},
			})

			cc := newClient(t)
			defer cc.Close()
			sds := v2.NewClusterDiscoveryServiceClient(cc)
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			stream, err := sds.StreamClusters(ctx)
			check(t, err)
			sendreq(t, stream, clusterType) // send initial notification
			checkrecv(t, stream)            // check we receive one notification
			checktimeout(t, stream)         // check that the second receive times out
		},
		"StreamEndpoints": func(t *testing.T) {
			// endpoints will be ignored unless there is a matching service.
			tr.OnAdd(&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kube-scheduler",
					Namespace: "kube-system",
				},
				Spec: v1.ServiceSpec{
					Ports: []v1.ServicePort{{
						Protocol:   "TCP",
						Port:       80,
						TargetPort: intstr.FromInt(6502),
					}},
				},
			})

			tr.OnAdd(&v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kube-scheduler",
					Namespace: "kube-system",
				},
				Subsets: []v1.EndpointSubset{{
					Addresses: []v1.EndpointAddress{{
						IP: "130.211.139.167",
					}},
					Ports: []v1.EndpointPort{{
						Port: 80,
					}, {
						Port: 443,
					}},
				}},
			})

			cc := newClient(t)
			defer cc.Close()
			eds := v2.NewEndpointDiscoveryServiceClient(cc)
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			stream, err := eds.StreamEndpoints(ctx)
			check(t, err)
			sendreq(t, stream, endpointType) // send initial notification
			checkrecv(t, stream)             // check we receive one notification
			checktimeout(t, stream)          // check that the second receive times out
		},
		"StreamListeners": func(t *testing.T) {
			cc := newClient(t)
			defer cc.Close()
			lds := v2.NewListenerDiscoveryServiceClient(cc)
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			stream, err := lds.StreamListeners(ctx)
			check(t, err)
			sendreq(t, stream, listenerType) // send initial notification
			checktimeout(t, stream)          // check that the first receive times out, there is no default listener

			// add an ingress, which will create a non tls listener
			tr.OnAdd(&v1beta1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "httpbin-org",
					Namespace: "default",
				},
				Spec: v1beta1.IngressSpec{
					Rules: []v1beta1.IngressRule{{
						Host: "httpbin.org",
						IngressRuleValue: v1beta1.IngressRuleValue{
							HTTP: &v1beta1.HTTPIngressRuleValue{
								Paths: []v1beta1.HTTPIngressPath{{
									Backend: v1beta1.IngressBackend{
										ServiceName: "httpbin-org",
										ServicePort: intstr.FromInt(80),
									},
								}},
							},
						},
					}},
				},
			})
			ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			stream, err = lds.StreamListeners(ctx)
			check(t, err)
			sendreq(t, stream, listenerType) // send initial notification
			checkrecv(t, stream)             // check we receive one notification
			checktimeout(t, stream)          // check that the second receive times out
		},
		"StreamRoutes": func(t *testing.T) {
			tr.OnAdd(&v1beta1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "httpbin-org",
					Namespace: "default",
				},
				Spec: v1beta1.IngressSpec{
					Rules: []v1beta1.IngressRule{{
						Host: "httpbin.org",
						IngressRuleValue: v1beta1.IngressRuleValue{
							HTTP: &v1beta1.HTTPIngressRuleValue{
								Paths: []v1beta1.HTTPIngressPath{{
									Backend: v1beta1.IngressBackend{
										ServiceName: "httpbin-org",
										ServicePort: intstr.FromInt(80),
									},
								}},
							},
						},
					}},
				},
			})

			cc := newClient(t)
			defer cc.Close()
			rds := v2.NewRouteDiscoveryServiceClient(cc)
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			stream, err := rds.StreamRoutes(ctx)
			check(t, err)
			sendreq(t, stream, routeType) // send initial notification
			checkrecv(t, stream)          // check we receive one notification
			checktimeout(t, stream)       // check that the second receive times out
		},
	}

	log := testLogger(t)
	for name, fn := range tests {
		t.Run(name, func(t *testing.T) {
			tr = &contour.Translator{
				FieldLogger: log,
			}
			srv := NewAPI(log, tr)
			var err error
			l, err = net.Listen("tcp", "127.0.0.1:0")
			check(t, err)
			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()
				srv.Serve(l)
			}()
			defer func() {
				srv.Stop()
				wg.Wait()
				l.Close()
			}()
			fn(t)
		})
	}
}

func TestGRPCFetching(t *testing.T) {
	var l net.Listener

	newClient := func(t *testing.T) *grpc.ClientConn {
		cc, err := grpc.Dial(l.Addr().String(), grpc.WithInsecure())
		check(t, err)
		return cc
	}

	tests := map[string]func(*testing.T){
		"FetchClusters": func(t *testing.T) {
			cc := newClient(t)
			defer cc.Close()
			sds := v2.NewClusterDiscoveryServiceClient(cc)
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			req := &v2.DiscoveryRequest{
				TypeUrl: clusterType,
			}
			_, err := sds.FetchClusters(ctx, req)
			check(t, err)
		},
		"FetchEndpoints": func(t *testing.T) {
			cc := newClient(t)
			defer cc.Close()
			eds := v2.NewEndpointDiscoveryServiceClient(cc)
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			req := &v2.DiscoveryRequest{
				TypeUrl: endpointType,
			}
			_, err := eds.FetchEndpoints(ctx, req)
			check(t, err)
		},
		"FetchListeners": func(t *testing.T) {
			cc := newClient(t)
			defer cc.Close()
			lds := v2.NewListenerDiscoveryServiceClient(cc)
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			req := &v2.DiscoveryRequest{
				TypeUrl: listenerType,
			}
			_, err := lds.FetchListeners(ctx, req)
			check(t, err)
		},
		"FetchRoutes": func(t *testing.T) {
			cc := newClient(t)
			defer cc.Close()
			rds := v2.NewRouteDiscoveryServiceClient(cc)
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			req := &v2.DiscoveryRequest{
				TypeUrl: routeType,
			}
			_, err := rds.FetchRoutes(ctx, req)
			check(t, err)
		},
	}

	log := logrus.New()
	log.Out = &testWriter{t}
	for name, fn := range tests {
		t.Run(name, func(t *testing.T) {
			tr := &contour.Translator{
				FieldLogger: log,
			}
			srv := NewAPI(log, tr)
			var err error
			l, err = net.Listen("tcp", "127.0.0.1:0")
			check(t, err)
			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()
				srv.Serve(l)
			}()
			defer func() {
				srv.Stop()
				wg.Wait()
				l.Close()
			}()
			fn(t)
		})
	}
}

func check(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func sendreq(t *testing.T, stream interface {
	Send(*v2.DiscoveryRequest) error
}, typeurl string) {
	t.Helper()
	err := stream.Send(&v2.DiscoveryRequest{
		TypeUrl: typeurl,
	})
	check(t, err)
}

func checkrecv(t *testing.T, stream interface {
	Recv() (*v2.DiscoveryResponse, error)
}) {
	t.Helper()
	_, err := stream.Recv()
	check(t, err)
}

func checktimeout(t *testing.T, stream interface {
	Recv() (*v2.DiscoveryResponse, error)
}) {
	t.Helper()
	_, err := stream.Recv()
	if err == nil {
		t.Fatal("expected timeout")
	}
	s, ok := status.FromError(err)
	if !ok {
		t.Fatal(err)
	}
	if s.Code() != codes.DeadlineExceeded {
		t.Fatalf("expected %q, got %q", codes.DeadlineExceeded, s.Code())
	}
}

func testLogger(t *testing.T) logrus.FieldLogger {
	log := logrus.New()
	log.Out = &testWriter{t}
	return log
}

type testWriter struct {
	*testing.T
}

func (t *testWriter) Write(buf []byte) (int, error) {
	t.Logf("%s", buf)
	return len(buf), nil
}

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
