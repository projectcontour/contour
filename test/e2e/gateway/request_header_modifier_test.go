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

	. "github.com/onsi/ginkgo"
	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

func testRequestHeaderModifierForwardTo(namespace string) {
	Specify("request headers can be modified on forward to", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "echo-header-filter")
		f.Fixtures.Echo.Deploy(namespace, "echo-header-nofilter")

		route := &gatewayv1alpha1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "http-filter-1",
				Labels:    map[string]string{"app": "filter"},
			},
			Spec: gatewayv1alpha1.HTTPRouteSpec{
				Hostnames: []gatewayv1alpha1.Hostname{"requestheadermodifierforwardto.gateway.projectcontour.io"},
				Gateways: &gatewayv1alpha1.RouteGateways{
					Allow: gatewayAllowTypePtr(gatewayv1alpha1.GatewayAllowAll),
				},
				Rules: []gatewayv1alpha1.HTTPRouteRule{
					{
						Matches: []gatewayv1alpha1.HTTPRouteMatch{
							{
								Path: &gatewayv1alpha1.HTTPPathMatch{
									Type:  pathMatchTypePtr(gatewayv1alpha1.PathMatchPrefix),
									Value: stringPtr("/filter"),
								},
							},
						},
						ForwardTo: []gatewayv1alpha1.HTTPRouteForwardTo{
							{
								ServiceName: stringPtr("echo-header-filter"),
								Port:        portNumPtr(80),
								Filters: []gatewayv1alpha1.HTTPRouteFilter{
									{
										Type: gatewayv1alpha1.HTTPRouteFilterRequestHeaderModifier,
										RequestHeaderModifier: &gatewayv1alpha1.HTTPRequestHeaderFilter{
											Add: map[string]string{
												"My-Header": "Foo",
											},
											Set: map[string]string{
												"Replace-Header": "Bar",
											},
											Remove: []string{"Other-Header"},
										},
									},
								},
							},
						},
					},
					{
						Matches: []gatewayv1alpha1.HTTPRouteMatch{
							{
								Path: &gatewayv1alpha1.HTTPPathMatch{
									Type:  pathMatchTypePtr(gatewayv1alpha1.PathMatchPrefix),
									Value: stringPtr("/nofilter"),
								},
							},
						},
						ForwardTo: []gatewayv1alpha1.HTTPRouteForwardTo{
							{
								ServiceName: stringPtr("echo-header-nofilter"),
								Port:        portNumPtr(80),
							},
						},
					},
				},
			},
		}
		f.CreateHTTPRouteAndWaitFor(route, httpRouteAdmitted)

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

func testRequestHeaderModifierRule(namespace string) {
	Specify("request headers can be modified on route rule", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "echo-header-filter")
		f.Fixtures.Echo.Deploy(namespace, "echo-header-nofilter")

		route := &gatewayv1alpha1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "http-filter-1",
				Labels:    map[string]string{"app": "filter"},
			},
			Spec: gatewayv1alpha1.HTTPRouteSpec{
				Hostnames: []gatewayv1alpha1.Hostname{"requestheadermodifierrule.gateway.projectcontour.io"},
				Gateways: &gatewayv1alpha1.RouteGateways{
					Allow: gatewayAllowTypePtr(gatewayv1alpha1.GatewayAllowAll),
				},
				Rules: []gatewayv1alpha1.HTTPRouteRule{
					{
						Matches: []gatewayv1alpha1.HTTPRouteMatch{
							{
								Path: &gatewayv1alpha1.HTTPPathMatch{
									Type:  pathMatchTypePtr(gatewayv1alpha1.PathMatchPrefix),
									Value: stringPtr("/filter"),
								},
							},
						},
						Filters: []gatewayv1alpha1.HTTPRouteFilter{
							{
								Type: gatewayv1alpha1.HTTPRouteFilterRequestHeaderModifier,
								RequestHeaderModifier: &gatewayv1alpha1.HTTPRequestHeaderFilter{
									Add: map[string]string{
										"My-Header": "Foo",
									},
									Set: map[string]string{
										"Replace-Header": "Bar",
									},
									Remove: []string{"Other-Header"},
								},
							},
						},
						ForwardTo: []gatewayv1alpha1.HTTPRouteForwardTo{
							{
								ServiceName: stringPtr("echo-header-filter"),
								Port:        portNumPtr(80),
							},
						},
					},
					{
						Matches: []gatewayv1alpha1.HTTPRouteMatch{
							{
								Path: &gatewayv1alpha1.HTTPPathMatch{
									Type:  pathMatchTypePtr(gatewayv1alpha1.PathMatchPrefix),
									Value: stringPtr("/nofilter"),
								},
							},
						},
						ForwardTo: []gatewayv1alpha1.HTTPRouteForwardTo{
							{
								ServiceName: stringPtr("echo-header-nofilter"),
								Port:        portNumPtr(80),
							},
						},
					},
				},
			},
		}
		f.CreateHTTPRouteAndWaitFor(route, httpRouteAdmitted)

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
