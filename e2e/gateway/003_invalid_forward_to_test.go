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
	"context"
	"testing"

	"github.com/projectcontour/contour/e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

func testInvalidForwardTo(t *testing.T, fx *e2e.Framework) {
	namespace := "gateway-003-invalid-forward-to"

	fx.CreateNamespace(namespace)
	defer fx.DeleteNamespace(namespace)

	fx.CreateEchoWorkload(namespace, "echo-slash-default")

	// HTTPRoute
	route := &gatewayv1alpha1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "http-filter-1",
			Labels:    map[string]string{"app": "filter"},
		},
		Spec: gatewayv1alpha1.HTTPRouteSpec{
			Hostnames: []gatewayv1alpha1.Hostname{"invalidforwardto.projectcontour.io"},
			Rules: []gatewayv1alpha1.HTTPRouteRule{
				{
					Matches: []gatewayv1alpha1.HTTPRouteMatch{
						{
							Path: gatewayv1alpha1.HTTPPathMatch{
								Type:  gatewayv1alpha1.PathMatchPrefix,
								Value: "/invalidref",
							},
						},
					},
					ForwardTo: []gatewayv1alpha1.HTTPRouteForwardTo{
						{
							ServiceName: stringPtr("invalid"),
							Port:        portNumPtr(80),
						},
					},
				},

				{
					Matches: []gatewayv1alpha1.HTTPRouteMatch{
						{
							Path: gatewayv1alpha1.HTTPPathMatch{
								Type:  gatewayv1alpha1.PathMatchPrefix,
								Value: "/invalidport",
							},
						},
					},
					ForwardTo: []gatewayv1alpha1.HTTPRouteForwardTo{
						{
							ServiceName: stringPtr("echo-slash-default"),
						},
					},
				},

				{
					Matches: []gatewayv1alpha1.HTTPRouteMatch{
						{
							Path: gatewayv1alpha1.HTTPPathMatch{
								Type:  gatewayv1alpha1.PathMatchPrefix,
								Value: "/invalidservicename",
							},
						},
					},
					ForwardTo: []gatewayv1alpha1.HTTPRouteForwardTo{
						{
							ServiceName: stringPtr(""),
							Port:        portNumPtr(80),
						},
					},
				},

				{
					Matches: []gatewayv1alpha1.HTTPRouteMatch{
						{
							Path: gatewayv1alpha1.HTTPPathMatch{
								Type:  gatewayv1alpha1.PathMatchPrefix,
								Value: "/",
							},
						},
					},
					ForwardTo: []gatewayv1alpha1.HTTPRouteForwardTo{
						{
							ServiceName: stringPtr("echo-slash-default"),
							Port:        portNumPtr(80),
						},
					},
				},
			},
		},
	}
	require.NoError(t, fx.Client.Create(context.TODO(), route))

	// TODO should wait until HTTPRoute has a status of valid

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
			path:           "/invalidport",
			expectResponse: 503,
		},
		{
			path:           "/invalidservicename",
			expectResponse: 503,
		},
	}

	for _, tc := range cases {
		res, ok := fx.HTTPRequestUntil(e2e.HasStatusCode(tc.expectResponse), tc.path, string(route.Spec.Hostnames[0]))
		if !assert.Truef(t, ok, "did not get %d response", tc.expectResponse) {
			continue
		}
		if res.StatusCode != 200 {
			// If we expected something other than a 200,
			// then we don't need to check the body.
			continue
		}

		body := fx.GetEchoResponseBody(res.Body)
		assert.Equal(t, namespace, body.Namespace)
		assert.Equal(t, tc.expectService, body.Service)
	}
}
