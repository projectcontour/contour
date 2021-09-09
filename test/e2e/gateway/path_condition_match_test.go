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

func testGatewayPathConditionMatch(namespace string) {
	Specify("path match routing works", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "echo-slash-prefix")
		f.Fixtures.Echo.Deploy(namespace, "echo-slash-noprefix")
		f.Fixtures.Echo.Deploy(namespace, "echo-slash-default")
		f.Fixtures.Echo.Deploy(namespace, "echo-slash-exact")

		route := &gatewayapi_v1alpha2.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "http-filter-1",
				Labels:    map[string]string{"app": "filter"},
			},
			Spec: gatewayapi_v1alpha2.HTTPRouteSpec{
				Hostnames: []gatewayapi_v1alpha2.Hostname{"gatewaypathconditions.projectcontour.io"},
				Gateways: &gatewayapi_v1alpha2.RouteGateways{
					Allow: gatewayAllowTypePtr(gatewayapi_v1alpha2.GatewayAllowAll),
				},
				Rules: []gatewayapi_v1alpha2.HTTPRouteRule{
					{
						Matches: []gatewayapi_v1alpha2.HTTPRouteMatch{
							{
								Path: &gatewayapi_v1alpha2.HTTPPathMatch{
									Type:  pathMatchTypePtr(gatewayapi_v1alpha2.PathMatchPrefix),
									Value: stringPtr("/path/prefix/"),
								},
							},
						},
						ForwardTo: []gatewayapi_v1alpha2.HTTPRouteForwardTo{
							{
								ServiceName: stringPtr("echo-slash-prefix"),
								Port:        portNumPtr(80),
							},
						},
					},

					{
						Matches: []gatewayapi_v1alpha2.HTTPRouteMatch{
							{
								Path: &gatewayapi_v1alpha2.HTTPPathMatch{
									Type:  pathMatchTypePtr(gatewayapi_v1alpha2.PathMatchPrefix),
									Value: stringPtr("/path/prefix"),
								},
							},
						},
						ForwardTo: []gatewayapi_v1alpha2.HTTPRouteForwardTo{
							{
								ServiceName: stringPtr("echo-slash-noprefix"),
								Port:        portNumPtr(80),
							},
						},
					},

					{
						Matches: []gatewayapi_v1alpha2.HTTPRouteMatch{
							{
								Path: &gatewayapi_v1alpha2.HTTPPathMatch{
									Type:  pathMatchTypePtr(gatewayapi_v1alpha2.PathMatchExact),
									Value: stringPtr("/path/exact"),
								},
							},
						},
						ForwardTo: []gatewayapi_v1alpha2.HTTPRouteForwardTo{
							{
								ServiceName: stringPtr("echo-slash-exact"),
								Port:        portNumPtr(80),
							},
						},
					},

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
								ServiceName: stringPtr("echo-slash-default"),
								Port:        portNumPtr(80),
							},
						},
					},
				},
			},
		}
		f.CreateHTTPRouteAndWaitFor(route, httpRouteAdmitted)

		cases := map[string]string{
			"/":                "echo-slash-default",
			"/foo":             "echo-slash-default",
			"/path/prefix":     "echo-slash-noprefix",
			"/path/prefixfoo":  "echo-slash-noprefix",
			"/path/prefix/":    "echo-slash-prefix",
			"/path/prefix/foo": "echo-slash-prefix",
			"/path/exact":      "echo-slash-exact",
			"/path/exactfoo":   "echo-slash-default",
			"/path/exact/":     "echo-slash-default",
			"/path/exact/foo":  "echo-slash-default",
		}

		for path, expectedService := range cases {
			t.Logf("Querying %q, expecting service %q", path, expectedService)

			res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
				Host:      string(route.Spec.Hostnames[0]),
				Path:      path,
				Condition: e2e.HasStatusCode(200),
			})
			if !assert.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode) {
				continue
			}

			body := f.GetEchoResponseBody(res.Body)
			assert.Equal(t, namespace, body.Namespace)
			assert.Equal(t, expectedService, body.Service)
		}
	})
}
