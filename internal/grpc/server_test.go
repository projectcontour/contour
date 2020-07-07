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
	"io/ioutil"
	"net"
	"testing"
	"time"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v2"
	resource "github.com/envoyproxy/go-control-plane/pkg/resource/v2"
	"github.com/projectcontour/contour/internal/contour"
	"github.com/projectcontour/contour/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	v1 "k8s.io/api/core/v1"
	"k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestGRPC(t *testing.T) {
	// tr and et is recreated before the start of each test.
	var et *contour.EndpointsTranslator
	var eh *contour.EventHandler

	tests := map[string]func(*testing.T, *grpc.ClientConn){
		"StreamClusters": func(t *testing.T, cc *grpc.ClientConn) {
			eh.OnAdd(&v1.Service{
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

			sds := v2.NewClusterDiscoveryServiceClient(cc)
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			stream, err := sds.StreamClusters(ctx)
			check(t, err)
			sendreq(t, stream, resource.ClusterType) // send initial notification
			checkrecv(t, stream)                     // check we receive one notification
			checktimeout(t, stream)                  // check that the second receive times out
		},
		"StreamEndpoints": func(t *testing.T, cc *grpc.ClientConn) {
			et.OnAdd(&v1.Endpoints{
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

			eds := v2.NewEndpointDiscoveryServiceClient(cc)
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			stream, err := eds.StreamEndpoints(ctx)
			check(t, err)
			sendreq(t, stream, resource.EndpointType) // send initial notification
			checkrecv(t, stream)                      // check we receive one notification
			checktimeout(t, stream)                   // check that the second receive times out
		},
		"StreamListeners": func(t *testing.T, cc *grpc.ClientConn) {
			// add an ingress, which will create a non tls listener
			eh.OnAdd(&v1beta1.Ingress{
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

			lds := v2.NewListenerDiscoveryServiceClient(cc)
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			stream, err := lds.StreamListeners(ctx)
			check(t, err)
			sendreq(t, stream, resource.ListenerType) // send initial notification
			checkrecv(t, stream)                      // check we receive one notification
			checktimeout(t, stream)                   // check that the second receive times out
		},
		"StreamRoutes": func(t *testing.T, cc *grpc.ClientConn) {
			eh.OnAdd(&v1beta1.Ingress{
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

			rds := v2.NewRouteDiscoveryServiceClient(cc)
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			stream, err := rds.StreamRoutes(ctx)
			check(t, err)
			sendreq(t, stream, resource.RouteType) // send initial notification
			checkrecv(t, stream)                   // check we receive one notification
			checktimeout(t, stream)                // check that the second receive times out
		},
		"StreamSecrets": func(t *testing.T, cc *grpc.ClientConn) {
			eh.OnAdd(&v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					v1.TLSCertKey:       []byte("certificate"),
					v1.TLSPrivateKeyKey: []byte("key"),
				},
			})

			sds := discovery.NewSecretDiscoveryServiceClient(cc)
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			stream, err := sds.StreamSecrets(ctx)
			check(t, err)
			sendreq(t, stream, resource.SecretType) // send initial notification
			checkrecv(t, stream)                    // check we receive one notification
			checktimeout(t, stream)                 // check that the second receive times out
		},
	}

	log := logrus.New()
	log.SetOutput(ioutil.Discard)
	for name, fn := range tests {
		t.Run(name, func(t *testing.T) {
			et = &contour.EndpointsTranslator{
				FieldLogger: log,
			}
			ch := contour.CacheHandler{
				Metrics: metrics.NewMetrics(prometheus.NewRegistry()),
			}

			eh = &contour.EventHandler{
				CacheHandler: &ch,
				FieldLogger:  log,
			}
			r := prometheus.NewRegistry()
			srv := NewAPI(log, map[string]Resource{
				ch.ClusterCache.TypeURL():  &ch.ClusterCache,
				ch.RouteCache.TypeURL():    &ch.RouteCache,
				ch.ListenerCache.TypeURL(): &ch.ListenerCache,
				ch.SecretCache.TypeURL():   &ch.SecretCache,
				et.TypeURL():               et,
			}, r)
			l, err := net.Listen("tcp", "127.0.0.1:0")
			check(t, err)
			done := make(chan error, 1)
			stop := make(chan struct{})
			run := eh.Start()
			go run(stop) // nolint:errcheck
			go func() {
				done <- srv.Serve(l)
			}()
			defer func() {
				srv.Stop()
				close(stop)
				<-done
			}()
			cc, err := grpc.Dial(l.Addr().String(), grpc.WithInsecure())
			check(t, err)
			defer cc.Close()
			fn(t, cc)
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
		t.Fatalf("%T %v", err, err)
	}

	// Work around grpc/grpc-go#1645 which sometimes seems to
	// set the status code to Unknown, even when the message is derived from context.DeadlineExceeded.
	if s.Code() != codes.DeadlineExceeded && s.Message() != context.DeadlineExceeded.Error() {
		t.Fatalf("expected %q, got %q %T %v", codes.DeadlineExceeded, s.Code(), err, err)
	}
}
