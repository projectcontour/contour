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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gatewayapi_v1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/projectcontour/contour/internal/gatewayapi"
	"github.com/projectcontour/contour/test/e2e"
)

func testResponseHeaderModifierBackendRef(namespace string, gateway types.NamespacedName) {
	Specify("response headers can be modified on backendref filters", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "echo-header-filter")
		f.Fixtures.Echo.Deploy(namespace, "echo-header-nofilter")

		route := &gatewayapi_v1.HTTPRoute{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "http-filter-1",
			},
			Spec: gatewayapi_v1.HTTPRouteSpec{
				Hostnames: []gatewayapi_v1.Hostname{"responseheadermodifierbackendref.gateway.projectcontour.io"},
				CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
					ParentRefs: []gatewayapi_v1.ParentReference{
						gatewayapi.GatewayParentRef(gateway.Namespace, gateway.Name),
					},
				},
				Rules: []gatewayapi_v1.HTTPRouteRule{
					{
						Matches: gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/filter"),
						BackendRefs: []gatewayapi_v1.HTTPBackendRef{
							{
								BackendRef: gatewayapi_v1.BackendRef{
									BackendObjectReference: gatewayapi.ServiceBackendObjectRef("echo-header-filter", 80),
								},
								Filters: []gatewayapi_v1.HTTPRouteFilter{
									{
										Type: gatewayapi_v1.HTTPRouteFilterResponseHeaderModifier,
										ResponseHeaderModifier: &gatewayapi_v1.HTTPHeaderFilter{
											Add: []gatewayapi_v1.HTTPHeader{
												{Name: gatewayapi_v1.HTTPHeaderName("My-Header"), Value: "Foo"},
											},
											Set: []gatewayapi_v1.HTTPHeader{
												{Name: gatewayapi_v1.HTTPHeaderName("Replace-Header"), Value: "Bar"},
											},
											Remove: []string{"Other-Header"},
										},
									},
								},
							},
							{
								BackendRef: gatewayapi_v1.BackendRef{
									BackendObjectReference: gatewayapi.ServiceBackendObjectRef("echo-header-nofilter", 80),
								},
							},
						},
					},
				},
			},
		}
		require.True(f.T(), f.CreateHTTPRouteAndWaitFor(route, e2e.HTTPRouteAccepted))

		// Check the route is available.
		res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      string(route.Spec.Hostnames[0]),
			Path:      "/filter",
			Condition: e2e.HasStatusCode(200),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)

		seenBackends := map[string]struct{}{}
		// Retry a bunch of times to make sure we get to both backends.
		for range 20 {
			res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
				Host: string(route.Spec.Hostnames[0]),
				Path: "/filter",
				RequestOpts: []func(*http.Request){
					e2e.OptSetHeaders(map[string]string{
						"X-Echo-Set-Header": "Other-Header:Remove,Replace-Header:Tobe-Replaced",
					}),
				},
				Condition: e2e.HasStatusCode(200),
			})
			require.NotNil(t, res, "request never succeeded")
			require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)
			body := f.GetEchoResponseBody(res.Body)

			seenBackends[body.Service] = struct{}{}
			switch body.Service {
			case "echo-header-filter":
				assert.Len(t, res.Headers["My-Header"], 1)
				assert.Equal(t, "Foo", res.Headers.Get("My-Header"))

				assert.Len(t, res.Headers["Replace-Header"], 1)
				assert.Equal(t, "Bar", res.Headers.Get("Replace-Header"))

				assert.Empty(t, res.Headers["Other-Header"])
			case "echo-header-nofilter":
				assert.Empty(t, res.Headers["My-Header"])

				assert.Len(t, res.Headers["Replace-Header"], 1)
				assert.Equal(t, "Tobe-Replaced", res.Headers.Get("Replace-Header"))

				assert.Len(t, res.Headers["Other-Header"], 1)
				assert.Equal(t, "Remove", res.Headers.Get("Other-Header"))
			}
		}
		assert.Contains(t, seenBackends, "echo-header-filter")
		assert.Contains(t, seenBackends, "echo-header-nofilter")
	})
}
