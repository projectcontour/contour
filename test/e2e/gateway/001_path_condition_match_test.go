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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

func testGatewayPathConditionMatch(fx *e2e.Framework) {
	t := fx.T()
	namespace := "gateway-001-path-condition-match"

	fx.CreateNamespace(namespace)
	defer fx.DeleteNamespace(namespace)

	fx.Fixtures.Echo.Deploy(namespace, "echo-slash-prefix")
	fx.Fixtures.Echo.Deploy(namespace, "echo-slash-noprefix")
	fx.Fixtures.Echo.Deploy(namespace, "echo-slash-default")
	fx.Fixtures.Echo.Deploy(namespace, "echo-slash-exact")

	route := &gatewayv1alpha1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "http-filter-1",
			Labels:    map[string]string{"app": "filter"},
		},
		Spec: gatewayv1alpha1.HTTPRouteSpec{
			Hostnames: []gatewayv1alpha1.Hostname{"gatewaypathconditions.projectcontour.io"},
			Gateways: &gatewayv1alpha1.RouteGateways{
				Allow: gatewayAllowTypePtr(gatewayv1alpha1.GatewayAllowAll),
			},
			Rules: []gatewayv1alpha1.HTTPRouteRule{
				{
					Matches: []gatewayv1alpha1.HTTPRouteMatch{
						{
							Path: &gatewayv1alpha1.HTTPPathMatch{
								Type:  pathMatchTypePtr(gatewayv1alpha1.PathMatchPrefix),
								Value: stringPtr("/path/prefix/"),
							},
						},
					},
					ForwardTo: []gatewayv1alpha1.HTTPRouteForwardTo{
						{
							ServiceName: stringPtr("echo-slash-prefix"),
							Port:        portNumPtr(80),
						},
					},
				},

				{
					Matches: []gatewayv1alpha1.HTTPRouteMatch{
						{
							Path: &gatewayv1alpha1.HTTPPathMatch{
								Type:  pathMatchTypePtr(gatewayv1alpha1.PathMatchPrefix),
								Value: stringPtr("/path/prefix"),
							},
						},
					},
					ForwardTo: []gatewayv1alpha1.HTTPRouteForwardTo{
						{
							ServiceName: stringPtr("echo-slash-noprefix"),
							Port:        portNumPtr(80),
						},
					},
				},

				{
					Matches: []gatewayv1alpha1.HTTPRouteMatch{
						{
							Path: &gatewayv1alpha1.HTTPPathMatch{
								Type:  pathMatchTypePtr(gatewayv1alpha1.PathMatchExact),
								Value: stringPtr("/path/exact"),
							},
						},
					},
					ForwardTo: []gatewayv1alpha1.HTTPRouteForwardTo{
						{
							ServiceName: stringPtr("echo-slash-exact"),
							Port:        portNumPtr(80),
						},
					},
				},

				{
					Matches: []gatewayv1alpha1.HTTPRouteMatch{
						{
							Path: &gatewayv1alpha1.HTTPPathMatch{
								Type:  pathMatchTypePtr(gatewayv1alpha1.PathMatchPrefix),
								Value: stringPtr("/"),
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
	fx.CreateHTTPRouteAndWaitFor(route, httpRouteAdmitted)

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

		res, ok := fx.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      string(route.Spec.Hostnames[0]),
			Path:      path,
			Condition: e2e.HasStatusCode(200),
		})
		if !assert.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode) {
			continue
		}

		body := fx.GetEchoResponseBody(res.Body)
		assert.Equal(t, namespace, body.Namespace)
		assert.Equal(t, expectedService, body.Service)
	}
}
