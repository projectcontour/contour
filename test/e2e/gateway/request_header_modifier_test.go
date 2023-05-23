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
	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gatewayapi_v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

func testRequestHeaderModifierBackendRef(namespace string, gateway types.NamespacedName) {
	Specify("request headers can be modified on backendref filters", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "echo-header-filter")
		f.Fixtures.Echo.Deploy(namespace, "echo-header-nofilter")

		route := &gatewayapi_v1beta1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "http-filter-1",
			},
			Spec: gatewayapi_v1beta1.HTTPRouteSpec{
				Hostnames: []gatewayapi_v1beta1.Hostname{"requestheadermodifierbackendref.gateway.projectcontour.io"},
				CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
					ParentRefs: []gatewayapi_v1beta1.ParentReference{
						gatewayapi.GatewayParentRef(gateway.Namespace, gateway.Name),
					},
				},
				Rules: []gatewayapi_v1beta1.HTTPRouteRule{
					{
						Matches: gatewayapi.HTTPRouteMatch(gatewayapi_v1beta1.PathMatchPathPrefix, "/filter"),
						BackendRefs: []gatewayapi_v1beta1.HTTPBackendRef{
							{
								BackendRef: gatewayapi_v1beta1.BackendRef{
									BackendObjectReference: gatewayapi.ServiceBackendObjectRef("echo-header-filter", 80),
								},
								Filters: []gatewayapi_v1beta1.HTTPRouteFilter{
									{
										Type: gatewayapi_v1beta1.HTTPRouteFilterRequestHeaderModifier,
										RequestHeaderModifier: &gatewayapi_v1beta1.HTTPHeaderFilter{
											Add: []gatewayapi_v1beta1.HTTPHeader{
												{Name: gatewayapi_v1beta1.HTTPHeaderName("My-Header"), Value: "Foo"},
											},
											Set: []gatewayapi_v1beta1.HTTPHeader{
												{Name: gatewayapi_v1beta1.HTTPHeaderName("Replace-Header"), Value: "Bar"},
											},
											Remove: []string{"Other-Header"},
										},
									},
								},
							},
						},
					},
					{
						Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1beta1.PathMatchPathPrefix, "/nofilter"),
						BackendRefs: gatewayapi.HTTPBackendRef("echo-header-nofilter", 80, 1),
					},
				},
			},
		}
		f.CreateHTTPRouteAndWaitFor(route, e2e.HTTPRouteAccepted)

		// Check the route with the RequestHeaderModifier filter.
		res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host: string(route.Spec.Hostnames[0]),
			Path: "/filter",
			RequestOpts: []func(*http.Request){
				e2e.OptSetHeaders(map[string]string{
					"Other-Header":   "Remove",
					"Replace-Header": "Tobe-Replaced",
				}),
			},
			Condition: e2e.HasStatusCode(200),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)
		body := f.GetEchoResponseBody(res.Body)
		assert.Equal(t, "echo-header-filter", body.Service)

		assert.Equal(t, "Foo", body.RequestHeaders.Get("My-Header"))
		assert.Equal(t, "Bar", body.RequestHeaders.Get("Replace-Header"))

		_, found := body.RequestHeaders["Other-Header"]
		assert.False(t, found, "Other-Header was found on the response")

		// Check the route without any filters.
		res, ok = f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host: string(route.Spec.Hostnames[0]),
			Path: "/nofilter",
			RequestOpts: []func(*http.Request){
				e2e.OptSetHeaders(map[string]string{
					"Other-Header": "Exist",
				}),
			},
			Condition: e2e.HasStatusCode(200),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)
		body = f.GetEchoResponseBody(res.Body)
		assert.Equal(t, "echo-header-nofilter", body.Service)

		assert.Equal(t, "Exist", body.RequestHeaders.Get("Other-Header"))

		_, found = body.RequestHeaders["My-Header"]
		assert.False(t, found, "My-Header was found on the response")
	})
}
