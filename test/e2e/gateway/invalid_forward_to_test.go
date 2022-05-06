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
	. "github.com/onsi/ginkgo/v2"
	"github.com/projectcontour/contour/internal/gatewayapi"
	"github.com/projectcontour/contour/internal/status"
	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func testInvalidForwardTo(namespace string) {
	Specify("invalid forward to returns 503 status code", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "echo-slash-default")

		route := &gatewayapi_v1alpha2.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "http-filter-1",
			},
			Spec: gatewayapi_v1alpha2.HTTPRouteSpec{
				Hostnames: []gatewayapi_v1alpha2.Hostname{"invalidforwardto.projectcontour.io"},
				CommonRouteSpec: gatewayapi_v1alpha2.CommonRouteSpec{
					ParentRefs: []gatewayapi_v1alpha2.ParentRef{
						gatewayapi.GatewayParentRef("", "http"), // TODO need a better way to inform the test case of the Gateway it should use
					},
				},
				Rules: []gatewayapi_v1alpha2.HTTPRouteRule{
					{
						Matches: []gatewayapi_v1alpha2.HTTPRouteMatch{
							{
								Path: &gatewayapi_v1alpha2.HTTPPathMatch{
									Type:  gatewayapi.PathMatchTypePtr(gatewayapi_v1alpha2.PathMatchPathPrefix),
									Value: pointer.StringPtr("/invalidref"),
								},
							},
						},
						BackendRefs: []gatewayapi_v1alpha2.HTTPBackendRef{
							{
								BackendRef: gatewayapi_v1alpha2.BackendRef{
									BackendObjectReference: gatewayapi.ServiceBackendObjectRef("invalid", 80),
								},
							},
						},
					},
					{
						Matches: []gatewayapi_v1alpha2.HTTPRouteMatch{
							{
								Path: &gatewayapi_v1alpha2.HTTPPathMatch{
									Type:  gatewayapi.PathMatchTypePtr(gatewayapi_v1alpha2.PathMatchPathPrefix),
									Value: pointer.StringPtr("/invalidservicename"),
								},
							},
						},
						BackendRefs: []gatewayapi_v1alpha2.HTTPBackendRef{
							{
								BackendRef: gatewayapi_v1alpha2.BackendRef{
									BackendObjectReference: gatewayapi_v1alpha2.BackendObjectReference{
										Kind: gatewayapi.KindPtr("Service"),
										Name: "non-existent-service",
										Port: gatewayapi.PortNumPtr(80),
									},
								},
							},
						},
					},
					{
						Matches: []gatewayapi_v1alpha2.HTTPRouteMatch{
							{
								Path: &gatewayapi_v1alpha2.HTTPPathMatch{
									Type:  gatewayapi.PathMatchTypePtr(gatewayapi_v1alpha2.PathMatchPathPrefix),
									Value: pointer.StringPtr("/"),
								},
							},
						},
						BackendRefs: []gatewayapi_v1alpha2.HTTPBackendRef{
							{
								BackendRef: gatewayapi_v1alpha2.BackendRef{
									BackendObjectReference: gatewayapi_v1alpha2.BackendObjectReference{
										Kind: gatewayapi.KindPtr("Service"),
										Name: "echo-slash-default",
										Port: gatewayapi.PortNumPtr(80),
									},
								},
							},
						},
					},
				},
			},
		}

		f.CreateHTTPRouteAndWaitFor(route, func(route *gatewayapi_v1alpha2.HTTPRoute) bool {
			if len(route.Status.Parents) != 1 {
				return false
			}

			if len(route.Status.Parents[0].Conditions) != 2 {
				return false
			}

			var hasAccepted, hasResolvedRefs bool
			for _, cond := range route.Status.Parents[0].Conditions {
				if cond.Type == string(gatewayapi_v1alpha2.ConditionRouteAccepted) && cond.Status == metav1.ConditionFalse {
					hasAccepted = true
				}
				if cond.Type == string(status.ConditionResolvedRefs) && cond.Status == metav1.ConditionFalse {
					hasResolvedRefs = true
				}
			}

			return hasAccepted && hasResolvedRefs
		})

		type scenario struct {
			path           string
			expectResponse int
			expectService  string
		}

		cases := []scenario{
			{
				path:           "/",
				expectResponse: 200,
				expectService:  "echo-slash-default",
			},
			{
				path:           "/invalidref",
				expectResponse: 503,
			},
			{
				path:           "/invalidservicename",
				expectResponse: 503,
			},
		}

		for _, tc := range cases {
			res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
				Host:      string(route.Spec.Hostnames[0]),
				Path:      tc.path,
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
