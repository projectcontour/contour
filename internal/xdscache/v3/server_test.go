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
	"io/ioutil"
	"net"
	"testing"
	"time"

	envoy_service_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/service/cluster/v3"
	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	envoy_service_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/service/endpoint/v3"
	envoy_service_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/service/listener/v3"
	envoy_service_route_v3 "github.com/envoyproxy/go-control-plane/envoy/service/route/v3"
	envoy_service_secret_v3 "github.com/envoyproxy/go-control-plane/envoy/service/secret/v3"
	resource "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/projectcontour/contour/internal/contour"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/fixture"
	"github.com/projectcontour/contour/internal/xds"
	contour_xds_v3 "github.com/projectcontour/contour/internal/xds/v3"
	"github.com/projectcontour/contour/internal/xdscache"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	v1 "k8s.io/api/core/v1"
	networking_v1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestGRPC(t *testing.T) {
	// tr and et is recreated before the start of each test.
	var et *EndpointsTranslator
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

			sds := envoy_service_cluster_v3.NewClusterDiscoveryServiceClient(cc)
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			stream, err := sds.StreamClusters(ctx)
			require.NoError(t, err)
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

			eds := envoy_service_endpoint_v3.NewEndpointDiscoveryServiceClient(cc)
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			stream, err := eds.StreamEndpoints(ctx)
			require.NoError(t, err)
			sendreq(t, stream, resource.EndpointType) // send initial notification
			checkrecv(t, stream)                      // check we receive one notification
			checktimeout(t, stream)                   // check that the second receive times out
		},
		"StreamListeners": func(t *testing.T, cc *grpc.ClientConn) {
			// add an ingress, which will create a non tls listener
			eh.OnAdd(&networking_v1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "httpbin-org",
					Namespace: "default",
				},
				Spec: networking_v1.IngressSpec{
					Rules: []networking_v1.IngressRule{{
						Host: "httpbin.org",
						IngressRuleValue: networking_v1.IngressRuleValue{
							HTTP: &networking_v1.HTTPIngressRuleValue{
								Paths: []networking_v1.HTTPIngressPath{{
									Backend: *backend("httpbin-org", 80),
								}},
							},
						},
					}},
				},
			})

			lds := envoy_service_listener_v3.NewListenerDiscoveryServiceClient(cc)
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			stream, err := lds.StreamListeners(ctx)
			require.NoError(t, err)
			sendreq(t, stream, resource.ListenerType) // send initial notification
			checkrecv(t, stream)                      // check we receive one notification
			checktimeout(t, stream)                   // check that the second receive times out
		},
		"StreamRoutes": func(t *testing.T, cc *grpc.ClientConn) {
			eh.OnAdd(&networking_v1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "httpbin-org",
					Namespace: "default",
				},
				Spec: networking_v1.IngressSpec{
					Rules: []networking_v1.IngressRule{{
						Host: "httpbin.org",
						IngressRuleValue: networking_v1.IngressRuleValue{
							HTTP: &networking_v1.HTTPIngressRuleValue{
								Paths: []networking_v1.HTTPIngressPath{{
									Backend: *backend("httpbin-org", 80),
								}},
							},
						},
					}},
				},
			})

			rds := envoy_service_route_v3.NewRouteDiscoveryServiceClient(cc)
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			stream, err := rds.StreamRoutes(ctx)
			require.NoError(t, err)
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

			sds := envoy_service_secret_v3.NewSecretDiscoveryServiceClient(cc)
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			stream, err := sds.StreamSecrets(ctx)
			require.NoError(t, err)
			sendreq(t, stream, resource.SecretType) // send initial notification
			checkrecv(t, stream)                    // check we receive one notification
			checktimeout(t, stream)                 // check that the second receive times out
		},
	}

	log := logrus.New()
	log.SetOutput(ioutil.Discard)
	for name, fn := range tests {
		t.Run(name, func(t *testing.T) {
			et = NewEndpointsTranslator(fixture.NewTestLogger(t))

			resources := []xdscache.ResourceCache{
				NewListenerCache(ListenerConfig{}, "", 0, 0),
				&SecretCache{},
				&RouteCache{},
				&ClusterCache{},
				et,
			}

			eh = &contour.EventHandler{
				Observer:    dag.ComposeObservers(xdscache.ObserversOf(resources)...),
				FieldLogger: log,
			}

			srv := xds.NewServer(nil)
			contour_xds_v3.RegisterServer(contour_xds_v3.NewContourServer(log, xdscache.ResourcesOf(resources)...), srv)
			l, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			done := make(chan error, 1)
			stop := make(chan struct{})
			run := eh.Start()
			go run(stop) // nolint:errcheck
			go func() {
				done <- srv.Serve(l)
			}()
			defer func() {
				srv.GracefulStop()
				close(stop)
				<-done
			}()
			cc, err := grpc.Dial(l.Addr().String(), grpc.WithInsecure())
			require.NoError(t, err)
			defer cc.Close()
			fn(t, cc)
		})
	}
}

func sendreq(t *testing.T, stream interface {
	Send(*discovery.DiscoveryRequest) error
}, typeurl string) {
	t.Helper()
	err := stream.Send(&discovery.DiscoveryRequest{
		TypeUrl: typeurl,
	})
	require.NoError(t, err)
}

func checkrecv(t *testing.T, stream interface {
	Recv() (*discovery.DiscoveryResponse, error)
}) {
	t.Helper()
	_, err := stream.Recv()
	require.NoError(t, err)
}

func checktimeout(t *testing.T, stream interface {
	Recv() (*discovery.DiscoveryResponse, error)
}) {
	t.Helper()
	_, err := stream.Recv()
	require.Errorf(t, err, "expected timeout")
	s, ok := status.FromError(err)
	require.Truef(t, ok, "Error wasn't what was expected: %T %v", err, err)

	// Work around grpc/grpc-go#1645 which sometimes seems to
	// set the status code to Unknown, even when the message is derived from context.DeadlineExceeded.
	if s.Code() != codes.DeadlineExceeded && s.Message() != context.DeadlineExceeded.Error() {
		t.Fatalf("expected %q, got %q %T %v", codes.DeadlineExceeded, s.Code(), err, err)
	}
}
