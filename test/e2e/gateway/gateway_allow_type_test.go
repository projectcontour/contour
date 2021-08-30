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
	"context"

	. "github.com/onsi/ginkgo"
	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

func testGatewayAllowType(namespace string) {
	Specify("allowtype on route is respected", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "echo-blue")

		f.Fixtures.Echo.Deploy(namespace, "echo")

		// This route allows gateways from a list, and the actual gateway
		// is included in the list.
		gatewayInAllowedListRoute := &gatewayv1alpha1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "gateway-in-allowed-list",
				Labels:    map[string]string{"app": "filter"},
			},
			Spec: gatewayv1alpha1.HTTPRouteSpec{
				Hostnames: []gatewayv1alpha1.Hostname{"gatewayallowtype.gateway.projectcontour.io"},
				Gateways: &gatewayv1alpha1.RouteGateways{
					Allow: gatewayAllowTypePtr(gatewayv1alpha1.GatewayAllowFromList),
					GatewayRefs: []gatewayv1alpha1.GatewayReference{
						{
							Name:      "http",
							Namespace: namespace,
						},
					},
				},
				Rules: []gatewayv1alpha1.HTTPRouteRule{
					{
						Matches: []gatewayv1alpha1.HTTPRouteMatch{
							{
								Path: &gatewayv1alpha1.HTTPPathMatch{
									Type:  pathMatchTypePtr(gatewayv1alpha1.PathMatchExact),
									Value: stringPtr("/gateway-in-allowed-list"),
								},
							},
						},
						ForwardTo: []gatewayv1alpha1.HTTPRouteForwardTo{
							{
								ServiceName: stringPtr("echo-blue"),
								Port:        portNumPtr(80),
							},
						},
					},
				},
			},
		}
		f.CreateHTTPRouteAndWaitFor(gatewayInAllowedListRoute, httpRouteAdmitted)

		res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      string(gatewayInAllowedListRoute.Spec.Hostnames[0]),
			Path:      "/gateway-in-allowed-list",
			Condition: e2e.HasStatusCode(200),
		})
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)
		body := f.GetEchoResponseBody(res.Body)
		assert.Equal(t, "echo-blue", body.Service)

		// This route allows gateways from a list, and the actual gateway
		// is *NOT* included in the list.
		gatewayNotInAllowedListRoute := &gatewayv1alpha1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "gateway-not-in-allowed-list",
				Labels:    map[string]string{"app": "filter"},
			},
			Spec: gatewayv1alpha1.HTTPRouteSpec{
				Hostnames: []gatewayv1alpha1.Hostname{"gatewayallowtype.gateway.projectcontour.io"},
				Gateways: &gatewayv1alpha1.RouteGateways{
					Allow: gatewayAllowTypePtr(gatewayv1alpha1.GatewayAllowFromList),
					GatewayRefs: []gatewayv1alpha1.GatewayReference{
						{
							Name:      "invalid-name",
							Namespace: "invalid-ns",
						},
					},
				},
				Rules: []gatewayv1alpha1.HTTPRouteRule{
					{
						Matches: []gatewayv1alpha1.HTTPRouteMatch{
							{
								Path: &gatewayv1alpha1.HTTPPathMatch{
									Type:  pathMatchTypePtr(gatewayv1alpha1.PathMatchExact),
									Value: stringPtr("/gateway-not-in-allowed-list"),
								},
							},
						},
						ForwardTo: []gatewayv1alpha1.HTTPRouteForwardTo{
							{
								ServiceName: stringPtr("echo-blue"),
								Port:        portNumPtr(80),
							},
						},
					},
				},
			},
		}
		// can't wait for admitted because it'll be invalid
		require.NoError(t, f.Client.Create(context.TODO(), gatewayNotInAllowedListRoute))

		res, ok = f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      string(gatewayNotInAllowedListRoute.Spec.Hostnames[0]),
			Path:      "/gateway-not-in-allowed-list",
			Condition: e2e.HasStatusCode(404),
		})
		require.Truef(t, ok, "expected 404 response code, got %d", res.StatusCode)

		// This route allows gateways in the same namespace, and the actual
		// gateway is in the same namespace.
		gatewayInSameNamespaceRoute := &gatewayv1alpha1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "gateway-in-same-namespace",
				Labels:    map[string]string{"app": "filter"},
			},
			Spec: gatewayv1alpha1.HTTPRouteSpec{
				Hostnames: []gatewayv1alpha1.Hostname{"gatewayallowtype.gateway.projectcontour.io"},
				Gateways: &gatewayv1alpha1.RouteGateways{
					Allow: gatewayAllowTypePtr(gatewayv1alpha1.GatewayAllowSameNamespace),
				},
				Rules: []gatewayv1alpha1.HTTPRouteRule{
					{
						Matches: []gatewayv1alpha1.HTTPRouteMatch{
							{
								Path: &gatewayv1alpha1.HTTPPathMatch{
									Type:  pathMatchTypePtr(gatewayv1alpha1.PathMatchExact),
									Value: stringPtr("/gateway-in-same-namespace"),
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
		f.CreateHTTPRouteAndWaitFor(gatewayInSameNamespaceRoute, httpRouteAdmitted)

		res, ok = f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      string(gatewayInSameNamespaceRoute.Spec.Hostnames[0]),
			Path:      "/gateway-in-same-namespace",
			Condition: e2e.HasStatusCode(200),
		})
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)

		// This route allows gateways in the same namespace, and the actual
		// gateway is *NOT* in the same namespace.
		f.CreateNamespace("gateway-allow-type-invalid")
		defer f.DeleteNamespace("gateway-allow-type-invalid", false)
		gatewayNotInSameNamespaceRoute := &gatewayv1alpha1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "gateway-allow-type-invalid",
				Name:      "gateway-not-in-same-namespace",
				Labels:    map[string]string{"app": "filter"},
			},
			Spec: gatewayv1alpha1.HTTPRouteSpec{
				Hostnames: []gatewayv1alpha1.Hostname{"gatewayallowtype.gateway.projectcontour.io"},
				Gateways: &gatewayv1alpha1.RouteGateways{
					Allow: gatewayAllowTypePtr(gatewayv1alpha1.GatewayAllowSameNamespace),
				},
				Rules: []gatewayv1alpha1.HTTPRouteRule{
					{
						Matches: []gatewayv1alpha1.HTTPRouteMatch{
							{
								Path: &gatewayv1alpha1.HTTPPathMatch{
									Type:  pathMatchTypePtr(gatewayv1alpha1.PathMatchExact),
									Value: stringPtr("/gateway-not-in-same-namespace"),
								},
							},
						},
						ForwardTo: []gatewayv1alpha1.HTTPRouteForwardTo{
							{
								ServiceName: stringPtr("echo-blue"),
								Port:        portNumPtr(80),
							},
						},
					},
				},
			},
		}
		// can't wait for admitted because it'll be invalid
		require.NoError(t, f.Client.Create(context.TODO(), gatewayNotInSameNamespaceRoute))

		res, ok = f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      string(gatewayNotInSameNamespaceRoute.Spec.Hostnames[0]),
			Path:      "/gateway-not-in-same-namespace",
			Condition: e2e.HasStatusCode(404),
		})
		require.Truef(t, ok, "expected 404 response code, got %d", res.StatusCode)
	})
}
