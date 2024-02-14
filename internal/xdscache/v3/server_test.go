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
	"net"
	"testing"
	"time"

	envoy_service_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/service/cluster/v3"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	envoy_service_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/service/endpoint/v3"
	envoy_service_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/service/listener/v3"
	envoy_service_route_v3 "github.com/envoyproxy/go-control-plane/envoy/service/route/v3"
	envoy_service_runtime_v3 "github.com/envoyproxy/go-control-plane/envoy/service/runtime/v3"
	envoy_service_secret_v3 "github.com/envoyproxy/go-control-plane/envoy/service/secret/v3"
	resource "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	core_v1 "k8s.io/api/core/v1"
	discovery_v1 "k8s.io/api/discovery/v1"
	networking_v1 "k8s.io/api/networking/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	"github.com/projectcontour/contour/internal/contour"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/fixture"
	"github.com/projectcontour/contour/internal/xds"
	contour_xds_v3 "github.com/projectcontour/contour/internal/xds/v3"
	"github.com/projectcontour/contour/internal/xdscache"
)

func TestGRPC(t *testing.T) {
	// tr and et is recreated before the start of each test.
	var et *EndpointsTranslator
	var eh *contour.EventHandler
	var est *EndpointSliceTranslator

	tests := map[string]func(*testing.T, *grpc.ClientConn){
		"StreamClusters": func(t *testing.T, cc *grpc.ClientConn) {
			eh.OnAdd(&core_v1.Service{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "simple",
					Namespace: "default",
				},
				Spec: core_v1.ServiceSpec{
					Selector: map[string]string{
						"app": "simple",
					},
					Ports: []core_v1.ServicePort{{
						Protocol:   "TCP",
						Port:       80,
						TargetPort: intstr.FromInt(6502),
					}},
				},
			}, false)

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
			et.OnAdd(&core_v1.Endpoints{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "kube-scheduler",
					Namespace: "kube-system",
				},
				Subsets: []core_v1.EndpointSubset{{
					Addresses: []core_v1.EndpointAddress{{
						IP: "130.211.139.167",
					}},
					Ports: []core_v1.EndpointPort{{
						Port: 80,
					}, {
						Port: 443,
					}},
				}},
			}, false)

			eds := envoy_service_endpoint_v3.NewEndpointDiscoveryServiceClient(cc)
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			stream, err := eds.StreamEndpoints(ctx)
			require.NoError(t, err)
			sendreq(t, stream, resource.EndpointType) // send initial notification
			checkrecv(t, stream)                      // check we receive one notification
			checktimeout(t, stream)                   // check that the second receive times out
		},
		"StreamEndpointSlices": func(t *testing.T, cc *grpc.ClientConn) {
			et.OnAdd(&discovery_v1.EndpointSlice{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "kube-scheduler",
					Namespace: "kube-system",
				},
				AddressType: discovery_v1.AddressTypeIPv4,
				Endpoints: []discovery_v1.Endpoint{
					{
						Addresses: []string{
							"130.211.139.167",
						},
					},
				},
				Ports: []discovery_v1.EndpointPort{
					{
						Port: ptr.To[int32](80),
					},
					{
						Port: ptr.To[int32](80),
					},
				},
			}, false)

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
				ObjectMeta: meta_v1.ObjectMeta{
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
			}, false)

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
				ObjectMeta: meta_v1.ObjectMeta{
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
			}, false)

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
			eh.OnAdd(&core_v1.Secret{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					core_v1.TLSCertKey:       []byte("certificate"),
					core_v1.TLSPrivateKeyKey: []byte("key"),
				},
			}, false)

			sds := envoy_service_secret_v3.NewSecretDiscoveryServiceClient(cc)
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			stream, err := sds.StreamSecrets(ctx)
			require.NoError(t, err)
			sendreq(t, stream, resource.SecretType) // send initial notification
			checkrecv(t, stream)                    // check we receive one notification
			checktimeout(t, stream)                 // check that the second receive times out
		},
		"StreamRuntime": func(t *testing.T, cc *grpc.ClientConn) {
			rtds := envoy_service_runtime_v3.NewRuntimeDiscoveryServiceClient(cc)
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			stream, err := rtds.StreamRuntime(ctx)
			require.NoError(t, err)
			sendreq(t, stream, resource.RuntimeType) // send initial notification
			checkrecv(t, stream)                     // check we receive one notification
			checktimeout(t, stream)                  // check that the second receive times out
		},
	}

	log := fixture.NewDiscardLogger()
	for name, fn := range tests {
		t.Run(name, func(t *testing.T) {
			et = NewEndpointsTranslator(fixture.NewTestLogger(t))
			est = NewEndpointSliceTranslator(fixture.NewTestLogger(t))

			resources := []xdscache.ResourceCache{
				&ListenerCache{},
				&SecretCache{},
				&RouteCache{},
				&ClusterCache{},
				est,
				et,
				NewRuntimeCache(ConfigurableRuntimeSettings{}),
			}

			eh = contour.NewEventHandler(contour.EventHandlerConfig{
				Logger:   log,
				Builder:  new(dag.Builder),
				Observer: dag.ComposeObservers(xdscache.ObserversOf(resources)...),
			}, func() bool { return true })

			srv := xds.NewServer(nil)
			contour_xds_v3.RegisterServer(contour_xds_v3.NewContourServer(log, xdscache.ResourcesOf(resources)...), srv)
			l, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			done := make(chan error, 1)
			ctx, cancel := context.WithCancel(context.Background())
			go eh.Start(ctx) // nolint:errcheck
			go func() {
				done <- srv.Serve(l)
			}()
			defer func() {
				srv.GracefulStop()
				cancel()
				<-done
			}()
			cc, err := grpc.Dial(l.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
			require.NoError(t, err)
			defer cc.Close()
			fn(t, cc)
		})
	}
}

func sendreq(t *testing.T, stream interface {
	Send(*envoy_service_discovery_v3.DiscoveryRequest) error
}, typeurl string,
) {
	t.Helper()
	err := stream.Send(&envoy_service_discovery_v3.DiscoveryRequest{
		TypeUrl: typeurl,
	})
	require.NoError(t, err)
}

func checkrecv(t *testing.T, stream interface {
	Recv() (*envoy_service_discovery_v3.DiscoveryResponse, error)
},
) {
	t.Helper()
	_, err := stream.Recv()
	require.NoError(t, err)
}

func checktimeout(t *testing.T, stream interface {
	Recv() (*envoy_service_discovery_v3.DiscoveryResponse, error)
},
) {
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
