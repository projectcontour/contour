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
// +build e2e

package gateway

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	"github.com/projectcontour/contour/internal/gatewayapi"
	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayapi_v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

func testRouteParentRefs(namespace string) {
	Specify("route parentRefs are respected", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "echo-blue")

		f.Fixtures.Echo.Deploy(namespace, "echo")

		// This route has a parentRef to the gateway.
		gatewayInParentRefsRoute := &gatewayapi_v1beta1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "gateway-in-parent-refs",
			},
			Spec: gatewayapi_v1beta1.HTTPRouteSpec{
				Hostnames: []gatewayapi_v1beta1.Hostname{"routeparentrefs.gateway.projectcontour.io"},
				CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
					ParentRefs: []gatewayapi_v1beta1.ParentReference{
						gatewayapi.GatewayParentRef("", "http"), // TODO need a better way to inform the test case of the Gateway it should use
					},
				},
				Rules: []gatewayapi_v1beta1.HTTPRouteRule{
					{
						Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1beta1.PathMatchExact, "/gateway-in-parent-refs"),
						BackendRefs: gatewayapi.HTTPBackendRef("echo-blue", 80, 1),
					},
				},
			},
		}
		f.CreateHTTPRouteAndWaitFor(gatewayInParentRefsRoute, httpRouteAccepted)

		res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      string(gatewayInParentRefsRoute.Spec.Hostnames[0]),
			Path:      "/gateway-in-parent-refs",
			Condition: e2e.HasStatusCode(200),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)
		body := f.GetEchoResponseBody(res.Body)
		assert.Equal(t, "echo-blue", body.Service)

		// This route does not have a parentRef to the gateway.
		gatewayNotInParentRefsRoute := &gatewayapi_v1beta1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "gateway-not-in-parent-refs",
			},
			Spec: gatewayapi_v1beta1.HTTPRouteSpec{
				Hostnames: []gatewayapi_v1beta1.Hostname{"routeparentrefs.gateway.projectcontour.io"},
				CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
					ParentRefs: []gatewayapi_v1beta1.ParentReference{
						gatewayapi.GatewayParentRef("", "invalid-name"),
					},
				},
				Rules: []gatewayapi_v1beta1.HTTPRouteRule{
					{
						Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1beta1.PathMatchExact, "/gateway-not-in-parent-refs"),
						BackendRefs: gatewayapi.HTTPBackendRef("echo-blue", 80, 1),
					},
				},
			},
		}
		// can't wait for accepted because it'll be invalid
		require.NoError(t, f.Client.Create(context.TODO(), gatewayNotInParentRefsRoute))

		res, ok = f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      string(gatewayNotInParentRefsRoute.Spec.Hostnames[0]),
			Path:      "/gateway-not-in-parent-refs",
			Condition: e2e.HasStatusCode(404),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 404 response code, got %d", res.StatusCode)
	})
}
