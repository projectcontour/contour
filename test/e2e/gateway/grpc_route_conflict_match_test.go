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

package gateway

import (
	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/require"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gatewayapi_v1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/projectcontour/contour/internal/gatewayapi"
	"github.com/projectcontour/contour/test/e2e"
)

func testGRPCRouteConflictMatch(namespace string, gateway types.NamespacedName) {
	Specify("Creates two GRPCRoutes, second one has conflict match against the first one, report Accepted: false", func() {
		cleanup := f.Fixtures.GRPC.Deploy(namespace, "grpc-echo")

		By("create grpcroute-1 first")
		route1 := &gatewayapi_v1.GRPCRoute{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "grpcroute-1",
			},
			Spec: gatewayapi_v1.GRPCRouteSpec{
				Hostnames: []gatewayapi_v1.Hostname{"queryparams.gateway.projectcontour.io"},
				CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
					ParentRefs: []gatewayapi_v1.ParentReference{
						gatewayapi.GatewayParentRef(gateway.Namespace, gateway.Name),
					},
				},
				Rules: []gatewayapi_v1.GRPCRouteRule{{
					Matches: []gatewayapi_v1.GRPCRouteMatch{
						{
							Method: gatewayapi.GRPCMethodMatch(gatewayapi_v1.GRPCMethodMatchExact, "com.example.service", "Login"),
						},
						{
							Method: gatewayapi.GRPCMethodMatch(gatewayapi_v1.GRPCMethodMatchExact, "foo.com.example.service", "Login"),
						},
					},
					BackendRefs: gatewayapi.GRPCRouteBackendRef("grpc-echo", 9000, 1),
				}},
			},
		}
		ok := f.CreateGRPCRouteAndWaitFor(route1, e2e.GRPCRouteAccepted)
		require.True(f.T(), ok)

		By("create grpcroute-2 with conflicted matches")
		route2 := &gatewayapi_v1.GRPCRoute{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "grpcroute-2",
			},
			Spec: gatewayapi_v1.GRPCRouteSpec{
				Hostnames: []gatewayapi_v1.Hostname{"queryparams.gateway.projectcontour.io"},
				CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
					ParentRefs: []gatewayapi_v1.ParentReference{
						gatewayapi.GatewayParentRef(gateway.Namespace, gateway.Name),
					},
				},
				Rules: []gatewayapi_v1.GRPCRouteRule{
					{
						Matches: []gatewayapi_v1.GRPCRouteMatch{
							{
								Method: gatewayapi.GRPCMethodMatch(gatewayapi_v1.GRPCMethodMatchExact, "com.example.service", "Login"),
							},
							{
								Method: gatewayapi.GRPCMethodMatch(gatewayapi_v1.GRPCMethodMatchExact, "bar.com.example.service", "Login"),
							},
						},
						BackendRefs: gatewayapi.GRPCRouteBackendRef("grpc-echo", 9000, 1),
					},
				},
			},
		}
		ok = f.CreateGRPCRouteAndWaitFor(route2, e2e.GRPCRouteNotAcceptedDueToConflict)
		require.True(f.T(), ok)

		cleanup()
	})
}
