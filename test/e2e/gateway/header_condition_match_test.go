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

// +build e2e

package gateway

import (
	"net/http"

	. "github.com/onsi/ginkgo"
	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

func testGatewayHeaderConditionMatch(namespace string) {
	Specify("header match routing works", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "echo-header-exact")

		route := &gatewayv1alpha1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "http-filter-1",
				Labels:    map[string]string{"app": "filter"},
			},
			Spec: gatewayv1alpha1.HTTPRouteSpec{
				Hostnames: []gatewayv1alpha1.Hostname{"gatewayheaderconditions.projectcontour.io"},
				Gateways: &gatewayv1alpha1.RouteGateways{
					Allow: gatewayAllowTypePtr(gatewayv1alpha1.GatewayAllowAll),
				},
				Rules: []gatewayv1alpha1.HTTPRouteRule{
					{
						Matches: []gatewayv1alpha1.HTTPRouteMatch{
							{
								Path: &gatewayv1alpha1.HTTPPathMatch{
									Type:  pathMatchTypePtr(gatewayv1alpha1.PathMatchPrefix),
									Value: stringPtr("/"),
								},
								Headers: &gatewayv1alpha1.HTTPHeaderMatch{
									Type: headerMatchTypePtr(gatewayv1alpha1.HeaderMatchExact),
									Values: map[string]string{
										"My-Header": "Foo",
									},
								},
							},
						},
						ForwardTo: []gatewayv1alpha1.HTTPRouteForwardTo{
							{
								ServiceName: stringPtr("echo-header-exact"),
								Port:        portNumPtr(80),
							},
						},
					},
				},
			},
		}
		f.CreateHTTPRouteAndWaitFor(route, httpRouteAdmitted)

		type scenario struct {
			headers        map[string]string
			expectResponse int
			expectService  string
		}

		cases := []scenario{
			{
				headers:        map[string]string{"My-Header": "Foo"},
				expectResponse: 200,
				expectService:  "echo-header-exact",
			},
			{
				headers:        map[string]string{"My-Header": "NotFoo"},
				expectResponse: 404,
			},
			{
				headers:        map[string]string{"Other-Header": "Foo"},
				expectResponse: 404,
			},
		}

		for _, tc := range cases {
			res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
				Host: string(route.Spec.Hostnames[0]),
				RequestOpts: []func(*http.Request){
					e2e.OptSetHeaders(tc.headers),
				},
				Condition: e2e.HasStatusCode(tc.expectResponse),
			})
			if !assert.Truef(t, ok, "expected %d response code, got %d", tc.expectResponse, res.StatusCode) {
				continue
			}
			if res.StatusCode != 200 {
				// If we expected something other than a 200,
				// then we don't need to check the body.
				continue
			}

			body := f.GetEchoResponseBody(res.Body)
			assert.Equal(t, namespace, body.Namespace)
			assert.Equal(t, tc.expectService, body.Service)
		}
	})
}
