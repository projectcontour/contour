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
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	"github.com/projectcontour/contour/internal/gatewayapi"
	"github.com/projectcontour/contour/internal/ref"
	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gatewayapi_v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

func testRequestRedirectRule(namespace string, gateway types.NamespacedName) {
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
						gatewayapi.GatewayParentRef(gateway.Namespace, gateway.Name),
					},
				},
				Rules: []gatewayapi_v1beta1.HTTPRouteRule{
					{
						Matches: gatewayapi.HTTPRouteMatch(gatewayapi_v1beta1.PathMatchPathPrefix, "/complex-redirect"),
						Filters: []gatewayapi_v1beta1.HTTPRouteFilter{
							{
								Type: gatewayapi_v1beta1.HTTPRouteFilterRequestRedirect,
								RequestRedirect: &gatewayapi_v1beta1.HTTPRequestRedirectFilter{
									Hostname:   ref.To(gatewayapi_v1beta1.PreciseHostname("envoyproxy.io")),
									StatusCode: ref.To(301),
									Scheme:     ref.To("https"),
									Port:       ref.To(gatewayapi_v1beta1.PortNumber(8080)),
								},
							},
						},
						BackendRefs: gatewayapi.HTTPBackendRef("echo", 80, 1),
					},
				},
			},
		}
		f.CreateHTTPRouteAndWaitFor(route, e2e.HTTPRouteAccepted)

		// /complex-redirect specifies a host name,
		// scheme, port and response code for the
		// redirect.
		res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
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
