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
	"github.com/projectcontour/contour/internal/gatewayapi"
	"github.com/projectcontour/contour/internal/ref"
	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayapi_v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

func testTCPRoute(namespace string, gateway types.NamespacedName) {
	Specify("A TCPRoute does L4 TCP proxying of traffic for its Listener port", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "echo")

		route := &gatewayapi_v1alpha2.TCPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "tcproute-1",
			},
			Spec: gatewayapi_v1alpha2.TCPRouteSpec{
				CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
					ParentRefs: []gatewayapi_v1alpha2.ParentReference{
						{
							Namespace: ref.To(gatewayapi_v1beta1.Namespace(gateway.Namespace)),
							Name:      gatewayapi_v1beta1.ObjectName(gateway.Name),
						},
					},
				},
				Rules: []gatewayapi_v1alpha2.TCPRouteRule{
					{
						BackendRefs: gatewayapi.TLSRouteBackendRef("echo", 80, ref.To(int32(1))),
					},
				},
			},
		}
		route, ok := f.CreateTCPRouteAndWaitFor(route, e2e.TCPRouteAccepted)
		require.True(t, ok)
		require.NotNil(t, route)

		res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Condition: e2e.HasStatusCode(200),
		})
		assert.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)
		assert.Equal(t, "echo", f.GetEchoResponseBody(res.Body).Service)

		// Envoy is expected to add the "server: envoy" and
		// "x-envoy-upstream-service-time" HTTP headers when
		// proxying HTTP; this ensures we are proxying TCP only.
		assert.Equal(t, "", res.Headers.Get("server"))
		assert.Equal(t, "", res.Headers.Get("x-envoy-upstream-service-time"))
	})
}
