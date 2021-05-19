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

	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

func testTLSRoute(fx *e2e.Framework) {
	t := fx.T()
	namespace := "gateway-008-tlsroute"

	fx.CreateNamespace(namespace)
	defer fx.DeleteNamespace(namespace)

	fx.Fixtures.EchoSecure.Deploy(namespace, "echo")
	fx.Certs.CreateSelfSignedCert(namespace, "backend-server-cert", "backend-server-cert", "tlsroute.gatewayapi.projectcontour.io")

	route := &gatewayv1alpha1.TLSRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "tls-route-1",
		},
		Spec: gatewayv1alpha1.TLSRouteSpec{
			Rules: []gatewayv1alpha1.TLSRouteRule{
				{
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
				},
			},
		},
	}
	fx.CreateTLSRouteAndWaitFor(route, tlsRouteAdmitted)

	// Ensure request routes to echo.
	res, ok := fx.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
		Host:      "tlsroute.gatewayapi.projectcontour.io",
		Condition: e2e.HasStatusCode(200),
	})
	assert.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)
	assert.Equal(t, "echo", fx.GetEchoResponseBody(res.Body).Service)

	require.NoError(t, retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if err := fx.Client.Get(context.TODO(), client.ObjectKeyFromObject(route), route); err != nil {
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
