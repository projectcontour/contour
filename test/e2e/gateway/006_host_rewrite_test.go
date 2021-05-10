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
	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

func testHostRewrite(fx *e2e.Framework) {
	t := fx.T()
	namespace := "gateway-006-host-rewrite"

	fx.CreateNamespace(namespace)
	defer fx.DeleteNamespace(namespace)

	fx.Fixtures.Echo.Deploy(namespace, "echo")

	route := &gatewayv1alpha1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "host-rewrite",
			Labels:    map[string]string{"app": "filter"},
		},
		Spec: gatewayv1alpha1.HTTPRouteSpec{
			Hostnames: []gatewayv1alpha1.Hostname{"hostrewrite.gateway.projectcontour.io"},
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
						},
					},
					Filters: []gatewayv1alpha1.HTTPRouteFilter{
						{
							Type: gatewayv1alpha1.HTTPRouteFilterRequestHeaderModifier,
							RequestHeaderModifier: &gatewayv1alpha1.HTTPRequestHeaderFilter{
								Add: map[string]string{
									"Host": "rewritten.com",
								},
							},
						},
					},
					ForwardTo: []gatewayv1alpha1.HTTPRouteForwardTo{
						{
							ServiceName: stringPtr("echo"),
							Port:        portNumPtr(80),
						},
					},
				},
			},
		},
	}
	fx.CreateHTTPRouteAndWaitFor(route, httpRouteAdmitted)

	res, ok := fx.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
		Host:      string(route.Spec.Hostnames[0]),
		Condition: e2e.HasStatusCode(200),
	})
	require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)
	body := fx.GetEchoResponseBody(res.Body)
	assert.Equal(t, "echo", body.Service)
	assert.Equal(t, "rewritten.com", body.Host)
}
