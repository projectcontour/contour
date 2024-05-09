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
	"context"

	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	gatewayapi_v1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/projectcontour/contour/internal/gatewayapi"
	"github.com/projectcontour/contour/test/e2e"
)

func testTCPRoute(namespace string, gateway types.NamespacedName) {
	Specify("A TCPRoute does L4 TCP proxying of traffic for its Listener port", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "echo-tcproute-backend")

		route := &gatewayapi_v1alpha2.TCPRoute{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "tcproute-1",
			},
			Spec: gatewayapi_v1alpha2.TCPRouteSpec{
				CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
					ParentRefs: []gatewayapi_v1.ParentReference{
						{
							Namespace: ptr.To(gatewayapi_v1.Namespace(gateway.Namespace)),
							Name:      gatewayapi_v1.ObjectName(gateway.Name),
						},
					},
				},
				Rules: []gatewayapi_v1alpha2.TCPRouteRule{
					{
						BackendRefs: gatewayapi.TLSRouteBackendRef("echo-tcproute-backend", 80, ptr.To(int32(1))),
					},
				},
			},
		}
		require.True(f.T(), f.CreateTCPRouteAndWaitFor(route, e2e.TCPRouteAccepted))

		res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Condition: e2e.HasStatusCode(200),
		})
		assert.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)
		assert.Equal(t, "echo-tcproute-backend", f.GetEchoResponseBody(res.Body).Service)

		// Envoy is expected to add the "server: envoy" and
		// "x-envoy-upstream-service-time" HTTP headers when
		// proxying HTTP; this ensures we are proxying TCP only.
		assert.Equal(t, "", res.Headers.Get("server"))
		assert.Equal(t, "", res.Headers.Get("x-envoy-upstream-service-time"))

		// Delete route and wait for config to no longer be present so this
		// test doesn't pollute others. This route effectively matches all
		// hostnames so it can affect other tests.
		require.NoError(t, f.Client.Delete(context.Background(), route))
		require.Eventually(t, func() bool {
			_, err := f.HTTP.Request(&e2e.HTTPRequestOpts{})
			return err != nil
		}, f.RetryTimeout, f.RetryInterval, "expected request to eventually fail")
		require.Never(t, func() bool {
			_, err := f.HTTP.Request(&e2e.HTTPRequestOpts{})
			return err == nil
		}, f.RetryTimeout, f.RetryInterval, "expected request to never succeed after failing")
	})
}
