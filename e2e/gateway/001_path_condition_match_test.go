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

func testGatewayPathConditionMatch(t *testing.T, fx *e2e.Framework) {
	namespace := "gateway-001-path-condition-match"

	fx.CreateNamespace(namespace)
	defer fx.DeleteNamespace(namespace)

	fx.CreateEchoWorkload(namespace, "echo-slash-prefix")
	fx.CreateEchoWorkload(namespace, "echo-slash-noprefix")
	fx.CreateEchoWorkload(namespace, "echo-slash-default")
	fx.CreateEchoWorkload(namespace, "echo-slash-exact")

	// HTTPRoute
	route := &gatewayv1alpha1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "http-filter-1",
			Labels:    map[string]string{"app": "filter"},
		},
		Spec: gatewayv1alpha1.HTTPRouteSpec{
			Hostnames: []gatewayv1alpha1.Hostname{"gatewaypathconditions.projectcontour.io"},
			Rules: []gatewayv1alpha1.HTTPRouteRule{
				{
					Matches: []gatewayv1alpha1.HTTPRouteMatch{
						{
							Path: gatewayv1alpha1.HTTPPathMatch{
								Type:  gatewayv1alpha1.PathMatchPrefix,
								Value: "/path/prefix/",
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
							Path: gatewayv1alpha1.HTTPPathMatch{
								Type:  gatewayv1alpha1.PathMatchPrefix,
								Value: "/path/prefix",
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
							Path: gatewayv1alpha1.HTTPPathMatch{
								Type:  gatewayv1alpha1.PathMatchExact,
								Value: "/path/exact",
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

		res, ok := fx.HTTPRequestUntil(e2e.IsOK, path, string(route.Spec.Hostnames[0]))
		if !assert.True(t, ok, "did not get 200 response") {
			continue
		}

		body := fx.GetEchoResponseBody(res.Body)
		assert.Equal(t, namespace, body.Namespace)
		assert.Equal(t, expectedService, body.Service)
	}
}
