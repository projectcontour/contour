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
	. "github.com/onsi/ginkgo"
	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func testTLSGateway(namespace string) {
	Specify("routes bound to port 443 listener are HTTPS and routes bound to port 80 listener are HTTP", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "echo-insecure")
		f.Fixtures.Echo.Deploy(namespace, "echo-secure")

		route := &gatewayapi_v1alpha2.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "http-route-1",
				Labels:    map[string]string{"type": "insecure"},
			},
			Spec: gatewayapi_v1alpha2.HTTPRouteSpec{
				Hostnames: []gatewayapi_v1alpha2.Hostname{"tls-gateway.projectcontour.io"},
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
							},
						},
						ForwardTo: []gatewayapi_v1alpha2.HTTPRouteForwardTo{
							{
								ServiceName: stringPtr("echo-insecure"),
								Port:        portNumPtr(80),
							},
						},
					},
				},
			},
		}
		f.CreateHTTPRouteAndWaitFor(route, httpRouteAdmitted)

		route = &gatewayapi_v1alpha2.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "http-route-2",
				Labels:    map[string]string{"type": "secure"},
			},
			Spec: gatewayapi_v1alpha2.HTTPRouteSpec{
				Hostnames: []gatewayapi_v1alpha2.Hostname{"tls-gateway.projectcontour.io"},
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
							},
						},
						ForwardTo: []gatewayapi_v1alpha2.HTTPRouteForwardTo{
							{
								ServiceName: stringPtr("echo-secure"),
								Port:        portNumPtr(80),
							},
						},
					},
				},
			},
		}
		f.CreateHTTPRouteAndWaitFor(route, httpRouteAdmitted)

		// Ensure http (insecure) request routes to echo-insecure.
		res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      "tls-gateway.projectcontour.io",
			Condition: e2e.HasStatusCode(200),
		})
		assert.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)
		assert.Equal(t, "echo-insecure", f.GetEchoResponseBody(res.Body).Service)

		// Ensure https (secure) request routes to echo-secure.
		res, ok = f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
			Host:      "tls-gateway.projectcontour.io",
			Condition: e2e.HasStatusCode(200),
		})
		assert.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)
		assert.Equal(t, "echo-secure", f.GetEchoResponseBody(res.Body).Service)
	})
}
