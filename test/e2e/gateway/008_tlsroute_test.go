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
	"crypto/tls"

	. "github.com/onsi/ginkgo"
	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

func testTLSRoutePassthrough(namespace string) {
	Specify("SNI matching can be used for routing", func() {
		t := f.T()

		f.Fixtures.EchoSecure.Deploy(namespace, "echo")
		f.Certs.CreateSelfSignedCert(namespace, "backend-server-cert", "backend-server-cert", "tlsroute.gatewayapi.projectcontour.io")

		// TLSRoute that doesn't define the termination type.
		route := &gatewayv1alpha1.TLSRoute{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "tls-route-1",
			},
			Spec: gatewayv1alpha1.TLSRouteSpec{
				Gateways: &gatewayv1alpha1.RouteGateways{
					Allow: gatewayAllowTypePtr(gatewayv1alpha1.GatewayAllowAll),
				},
				Rules: []gatewayv1alpha1.TLSRouteRule{{
					Matches: []gatewayv1alpha1.TLSRouteMatch{
						{
							SNIs: []gatewayv1alpha1.Hostname{
								gatewayv1alpha1.Hostname("tlsroute.gatewayapi.projectcontour.io"),
							},
						},
					},
					ForwardTo: []gatewayv1alpha1.RouteForwardTo{
						{
							ServiceName: stringPtr("echo"),
							Port:        portNumPtr(443),
						},
					},
				}},
			},
		}
		f.CreateTLSRouteAndWaitFor(route, tlsRouteAdmitted)

		// Ensure request routes to echo.
		res, ok := f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
			Host:      "tlsroute.gatewayapi.projectcontour.io",
			Condition: e2e.HasStatusCode(200),
		})
		assert.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)
		assert.Equal(t, "echo", f.GetEchoResponseBody(res.Body).Service)

		require.NoError(t, retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			if err := f.Client.Get(context.TODO(), client.ObjectKeyFromObject(route), route); err != nil {
				return err
			}

			route.Spec.Rules = []gatewayv1alpha1.TLSRouteRule{
				{
					ForwardTo: []gatewayv1alpha1.RouteForwardTo{
						{
							ServiceName: stringPtr("echo"),
							Port:        portNumPtr(443),
						},
					},
				},
			}

			return f.Client.Update(context.TODO(), route)
		}))

		// Ensure request routes to echo.
		res, ok = f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
			Host:      "anything.should.work.now",
			Condition: e2e.HasStatusCode(200),
		})
		assert.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)
		assert.Equal(t, "echo", f.GetEchoResponseBody(res.Body).Service)
	})
}

func testTLSRouteTerminate(fx *e2e.Framework, namespace string) {
	t := fx.T()

	fx.CreateNamespace(namespace)
	defer fx.DeleteNamespace(namespace)

	fx.Fixtures.Echo.Deploy(namespace, "echo")

	route := &gatewayv1alpha1.TLSRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "tls-route-1",
		},
		Spec: gatewayv1alpha1.TLSRouteSpec{
			Gateways: &gatewayv1alpha1.RouteGateways{
				Allow: gatewayAllowTypePtr(gatewayv1alpha1.GatewayAllowAll),
			},
			Rules: []gatewayv1alpha1.TLSRouteRule{{
				Matches: []gatewayv1alpha1.TLSRouteMatch{{
					SNIs: []gatewayv1alpha1.Hostname{
						gatewayv1alpha1.Hostname("tlsroute.gatewayapi.projectcontour.io"),
					},
				}},
				ForwardTo: []gatewayv1alpha1.RouteForwardTo{{
					ServiceName: stringPtr("echo"),
					Port:        portNumPtr(80),
				}},
			}},
		},
	}
	fx.CreateTLSRouteAndWaitFor(route, tlsRouteAdmitted)

	// Ensure request routes to echo matching SNI: tlsroute.gatewayapi.projectcontour.io
	res, ok := fx.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
		Host:      "tlsroute.gatewayapi.projectcontour.io",
		Condition: e2e.HasStatusCode(200),
		TLSConfigOpts: []func(*tls.Config){
			e2e.OptSetSNI("tlsroute.gatewayapi.projectcontour.io"),
		},
	})
	assert.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)
	assert.Equal(t, "echo", fx.GetEchoResponseBody(res.Body).Service)

	// Ensure request doesn't route to non-matching SNI: tlsroute.gatewayapi.projectcontour.io
	res, _ = fx.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
		Host: "something.else.not.matching",
		TLSConfigOpts: []func(*tls.Config){
			e2e.OptSetSNI("something.else.not.matching"),
		},
	})

	// Since SNI doesn't match, Envoy won't respond.
	assert.Nil(t, res, "expected no response but got a response.")

	// Update the TLSRoute to remove the Matches section which will allow it to match any SNI.
	require.NoError(t, retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if err := fx.Client.Get(context.TODO(), client.ObjectKeyFromObject(route), route); err != nil {
			return err
		}

		route.Spec.Rules = []gatewayv1alpha1.TLSRouteRule{
			{
				ForwardTo: []gatewayv1alpha1.RouteForwardTo{
					{
						ServiceName: stringPtr("echo"),
						Port:        portNumPtr(80),
					},
				},
			},
		}

		return fx.Client.Update(context.TODO(), route)
	}))

	// Ensure request routes to echo.
	res, ok = fx.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
		Host:      "anything.should.work.now",
		Condition: e2e.HasStatusCode(200),
	})
	assert.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)
	assert.Equal(t, "echo", fx.GetEchoResponseBody(res.Body).Service)
}
