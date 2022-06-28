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
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	"github.com/projectcontour/contour/internal/gatewayapi"
	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	gatewayapi_v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

func testRequestRedirectRule(namespace string) {
	Specify("redirects can be specified on route rule", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "echo")

		route := &gatewayapi_v1beta1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "httproute-redirect",
			},
			Spec: gatewayapi_v1beta1.HTTPRouteSpec{
				Hostnames: []gatewayapi_v1beta1.Hostname{"requestredirectrule.gateway.projectcontour.io"},
				CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
					ParentRefs: []gatewayapi_v1beta1.ParentReference{
						gatewayapi.GatewayParentRef("", "http"), // TODO need a better way to inform the test case of the Gateway it should use
					},
				},
				Rules: []gatewayapi_v1beta1.HTTPRouteRule{
					{
						Matches: gatewayapi.HTTPRouteMatch(gatewayapi_v1beta1.PathMatchPathPrefix, "/basic-redirect"),
						Filters: []gatewayapi_v1beta1.HTTPRouteFilter{
							{
								Type: gatewayapi_v1beta1.HTTPRouteFilterRequestRedirect,
								RequestRedirect: &gatewayapi_v1beta1.HTTPRequestRedirectFilter{
									Hostname: gatewayapi.PreciseHostname("projectcontour.io"),
								},
							},
						},
						BackendRefs: gatewayapi.HTTPBackendRef("echo", 80, 1),
					},
					{
						Matches: gatewayapi.HTTPRouteMatch(gatewayapi_v1beta1.PathMatchPathPrefix, "/complex-redirect"),
						Filters: []gatewayapi_v1beta1.HTTPRouteFilter{
							{
								Type: gatewayapi_v1beta1.HTTPRouteFilterRequestRedirect,
								RequestRedirect: &gatewayapi_v1beta1.HTTPRequestRedirectFilter{
									Hostname:   gatewayapi.PreciseHostname("envoyproxy.io"),
									StatusCode: pointer.Int(301),
									Scheme:     pointer.String("https"),
									Port:       gatewayapi.PortNumPtr(8080),
								},
							},
						},
						BackendRefs: gatewayapi.HTTPBackendRef("echo", 80, 1),
					},
				},
			},
		}
		f.CreateHTTPRouteAndWaitFor(route, httpRouteAccepted)

		// /basic-redirect only specifies a host name to
		// redirect to.
		res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host: string(route.Spec.Hostnames[0]),
			Path: "/basic-redirect",
			ClientOpts: []func(*http.Client){
				e2e.OptDontFollowRedirects,
			},
			Condition: e2e.HasStatusCode(302),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 302 response code, got %d", res.StatusCode)
		assert.Equal(t, "http://projectcontour.io/basic-redirect", res.Headers.Get("Location"))

		// /complex-redirect specifies a host name,
		// scheme, port and response code for the
		// redirect.
		res, ok = f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host: string(route.Spec.Hostnames[0]),
			Path: "/complex-redirect",
			ClientOpts: []func(*http.Client){
				e2e.OptDontFollowRedirects,
			},
			Condition: e2e.HasStatusCode(301),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 301 response code, got %d", res.StatusCode)
		assert.Equal(t, "https://envoyproxy.io:8080/complex-redirect", res.Headers.Get("Location"))
	})
}
