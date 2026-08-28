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

//go:build e2e

package e2e

import (
	"context"
	"net"
	"os"

	"github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	core_v1 "k8s.io/api/core/v1"
	discovery_v1 "k8s.io/api/discovery/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// grpcEchoServiceDesc is a hand-written gRPC service descriptor for
// contourecho.Echo with a single unary method "Ping". This avoids any
// dependency on protoc-generated code.
var grpcEchoServiceDesc = grpc.ServiceDesc{
	ServiceName: "contourecho.Echo",
	HandlerType: (*any)(nil),
	Methods: []grpc.MethodDesc{{
		MethodName: "Ping",
		Handler: func(_ any, _ context.Context, dec func(any) error, _ grpc.UnaryServerInterceptor) (any, error) {
			req := new(emptypb.Empty)
			if err := dec(req); err != nil {
				return nil, err
			}
			return &wrapperspb.StringValue{Value: "pong"}, nil
		},
	}},
}

// StartLocalGRPCEchoService starts a cleartext gRPC server on
// CONTOUR_E2E_LOCAL_HOST:<random-port> that implements
// contourecho.Echo/Ping (returns StringValue "pong").
//
// It registers the server in-cluster as a headless Service and
// EndpointSlice so Envoy can route to it. The server is stopped
// automatically when the test completes.
//
// CONTOUR_E2E_LOCAL_HOST must be set to a host IP reachable from within
// the cluster. The test is skipped if the variable is absent.
func StartLocalGRPCEchoService(t ginkgo.GinkgoTInterface, c client.Client, ns, name string) {
	hostIP := os.Getenv("CONTOUR_E2E_LOCAL_HOST")
	if hostIP == "" {
		ginkgo.Skip("CONTOUR_E2E_LOCAL_HOST must be set to a host IP reachable from within the cluster")
	}

	grpcServer := grpc.NewServer()
	grpcServer.RegisterService(&grpcEchoServiceDesc, &struct{}{})

	listener, err := net.Listen("tcp", net.JoinHostPort(hostIP, "0"))
	require.NoError(t, err)

	go func() {
		_ = grpcServer.Serve(listener)
	}()
	t.Cleanup(func() { grpcServer.GracefulStop() })

	port := listener.Addr().(*net.TCPAddr).Port

	require.NoError(t, c.Create(context.TODO(), &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{Name: name, Namespace: ns},
		Spec: core_v1.ServiceSpec{
			Ports: []core_v1.ServicePort{{Name: "grpc", Protocol: core_v1.ProtocolTCP, Port: 9000}},
		},
	}))
	require.NoError(t, c.Create(context.TODO(), &discovery_v1.EndpointSlice{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: name, Namespace: ns,
			Labels: map[string]string{discovery_v1.LabelServiceName: name},
		},
		AddressType: discovery_v1.AddressTypeIPv4,
		Endpoints: []discovery_v1.Endpoint{
			{Addresses: []string{hostIP}, Conditions: discovery_v1.EndpointConditions{Ready: ptr.To(true)}},
		},
		Ports: []discovery_v1.EndpointPort{
			{Name: ptr.To("grpc"), Port: ptr.To(int32(port)), Protocol: ptr.To(core_v1.ProtocolTCP)}, //nolint:gosec
		},
	}))
}
