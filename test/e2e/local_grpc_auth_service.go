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
	"sync/atomic"

	envoy_service_auth_v3 "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
	envoy_type_v3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/require"
	rpc_status "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	core_v1 "k8s.io/api/core/v1"
	discovery_v1 "k8s.io/api/discovery/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// LocalGRPCAuthServer is a test helper that runs an in-process gRPC ext_authz
// server whose behavior can be changed on the fly.
type LocalGRPCAuthServer struct {
	envoy_service_auth_v3.UnimplementedAuthorizationServer
	handler atomic.Pointer[func(*envoy_service_auth_v3.CheckRequest) *envoy_service_auth_v3.CheckResponse]
}

// Check implements the ext_authz Check function.
func (s *LocalGRPCAuthServer) Check(_ context.Context, req *envoy_service_auth_v3.CheckRequest) (*envoy_service_auth_v3.CheckResponse, error) {
	return (*s.handler.Load())(req), nil
}

// Deny sets the server to deny every request.
func (s *LocalGRPCAuthServer) Deny(httpStatusCode int32) {
	s.RespondWith(DenyResponse(httpStatusCode))
}

// Allow sets the server to allow every request.
func (s *LocalGRPCAuthServer) Allow() {
	s.RespondWith(AllowResponse().Build())
}

// RespondWith sets a static response for all requests.
func (s *LocalGRPCAuthServer) RespondWith(resp *envoy_service_auth_v3.CheckResponse) {
	h := func(_ *envoy_service_auth_v3.CheckRequest) *envoy_service_auth_v3.CheckResponse { return resp }
	s.handler.Store(&h)
}

// Handle sets a custom handler that is called for every Check RPC.
func (s *LocalGRPCAuthServer) Handle(h func(*envoy_service_auth_v3.CheckRequest) *envoy_service_auth_v3.CheckResponse) {
	s.handler.Store(&h)
}

type CheckResponseBuilder struct {
	resp *envoy_service_auth_v3.CheckResponse
}

// AllowResponse starts building a CheckResponse that allows the request.
func AllowResponse() *CheckResponseBuilder {
	return &CheckResponseBuilder{
		resp: &envoy_service_auth_v3.CheckResponse{
			Status: &rpc_status.Status{Code: int32(codes.OK)},
			HttpResponse: &envoy_service_auth_v3.CheckResponse_OkResponse{
				OkResponse: &envoy_service_auth_v3.OkHttpResponse{},
			},
		},
	}
}

// DenyResponse starts building a CheckResponse that denies the request with
// the given HTTP status code.
func DenyResponse(httpStatusCode int32) *envoy_service_auth_v3.CheckResponse {
	return &envoy_service_auth_v3.CheckResponse{
		Status: &rpc_status.Status{Code: int32(codes.PermissionDenied)},
		HttpResponse: &envoy_service_auth_v3.CheckResponse_DeniedResponse{
			DeniedResponse: &envoy_service_auth_v3.DeniedHttpResponse{
				Status: &envoy_type_v3.HttpStatus{Code: envoy_type_v3.StatusCode(httpStatusCode)},
			},
		},
	}
}

// Build returns the finished CheckResponse.
func (b *CheckResponseBuilder) Build() *envoy_service_auth_v3.CheckResponse {
	return b.resp
}

// StartLocalGRPCAuthService starts a cleartext gRPC Authorization server on
// CONTOUR_E2E_LOCAL_HOST:<random-port> and registers it in-cluster as a headless
// Service and EndpointSlice.
//
// CONTOUR_E2E_LOCAL_HOST must be set to a host IP reachable from within the
// cluster. The test is skipped if the variable is absent.
func StartLocalGRPCAuthService(t ginkgo.GinkgoTInterface, c client.Client, ns, name string) *LocalGRPCAuthServer {
	hostIP := os.Getenv("CONTOUR_E2E_LOCAL_HOST")
	if hostIP == "" {
		ginkgo.Skip("CONTOUR_E2E_LOCAL_HOST must be set to a host IP reachable from within the cluster")
	}

	srv := &LocalGRPCAuthServer{}

	grpcServer := grpc.NewServer()
	envoy_service_auth_v3.RegisterAuthorizationServer(grpcServer, srv)

	listener, err := net.Listen("tcp", net.JoinHostPort(hostIP, "0"))
	require.NoError(t, err)

	go func() {
		_ = grpcServer.Serve(listener)
	}()
	t.Cleanup(func() { grpcServer.GracefulStop() })

	port := listener.Addr().(*net.TCPAddr).Port
	createHeadlessService(t, c, ns, name, hostIP, port)

	return srv
}

// createHeadlessService creates a headless Service and EndpointSlice pointing
// at the given host:port, so in-cluster workloads can reach the local server.
func createHeadlessService(t ginkgo.GinkgoTInterface, c client.Client, ns, name, hostIP string, port int) {
	require.NoError(t, c.Create(context.TODO(), &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{Name: name, Namespace: ns},
		Spec: core_v1.ServiceSpec{
			Ports: []core_v1.ServicePort{{Name: "grpc", Protocol: core_v1.ProtocolTCP, Port: 9443}},
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
