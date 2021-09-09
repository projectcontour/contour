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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func testGatewayHeaderConditionMatch(namespace string) {
	Specify("header match routing works", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "echo-header-exact")

		route := &gatewayapi_v1alpha2.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "http-filter-1",
				Labels:    map[string]string{"app": "filter"},
			},
			Spec: gatewayapi_v1alpha2.HTTPRouteSpec{
				Hostnames: []gatewayapi_v1alpha2.Hostname{"gatewayheaderconditions.projectcontour.io"},
				Gateways: &gatewayapi_v1alpha2.RouteGateways{
					Allow: gatewayAllowTypePtr(gatewayapi_v1alpha2.GatewayAllowAll),
				},
				Rules: []gatewayapi_v1alpha2.HTTPRouteRule{
					{
						Matches: []gatewayapi_v1alpha2.HTTPRouteMatch{
							{
								Path: &gatewayapi_v1alpha2.HTTPPathMatch{
									Type:  pathMatchTypePtr(gatewayapi_v1alpha2.PathMatchPrefix),
									Value: stringPtr("/"),
								},
								Headers: &gatewayapi_v1alpha2.HTTPHeaderMatch{
									Type: headerMatchTypePtr(gatewayapi_v1alpha2.HeaderMatchExact),
									Values: map[string]string{
										"My-Header": "Foo",
									},
								},
							},
						},
						ForwardTo: []gatewayapi_v1alpha2.HTTPRouteForwardTo{
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
