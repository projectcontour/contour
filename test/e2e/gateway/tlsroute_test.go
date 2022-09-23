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
	"time"

	. "github.com/onsi/ginkgo/v2"
	"github.com/projectcontour/contour/internal/gatewayapi"
	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func testTLSRoutePassthrough(namespace string, gateway types.NamespacedName) {
	Specify("SNI matching can be used for routing", func() {
		t := f.T()

		f.Fixtures.EchoSecure.Deploy(namespace, "echo")
		f.Certs.CreateSelfSignedCert(namespace, "backend-server-cert", "backend-server-cert", "passthrough.tlsroute.gatewayapi.projectcontour.io")

		route := &gatewayapi_v1alpha2.TLSRoute{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "tls-route-1",
			},
			Spec: gatewayapi_v1alpha2.TLSRouteSpec{
				CommonRouteSpec: gatewayapi_v1alpha2.CommonRouteSpec{
					ParentRefs: []gatewayapi_v1alpha2.ParentReference{
						gatewayapi.GatewayParentRefV1Alpha2(gateway.Namespace, gateway.Name),
					},
				},
				Hostnames: []gatewayapi_v1alpha2.Hostname{"passthrough.tlsroute.gatewayapi.projectcontour.io"},
				Rules: []gatewayapi_v1alpha2.TLSRouteRule{{
					BackendRefs: gatewayapi.TLSRouteBackendRef("echo", 443, nil),
				}},
			},
		}
		f.CreateTLSRouteAndWaitFor(route, tlsRouteAccepted)

		// Ensure request routes to echo.
		res, ok := f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
			Host:      "passthrough.tlsroute.gatewayapi.projectcontour.io",
			Condition: e2e.HasStatusCode(200),
		})
		require.NotNil(t, res, "request never succeeded")
		assert.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)
		assert.Equal(t, "echo", f.GetEchoResponseBody(res.Body).Service)

		// Ensure request doesn't route when non-matching SNI is provided
		require.Never(f.T(), func() bool {
			_, err := f.HTTP.SecureRequest(&e2e.HTTPSRequestOpts{
				Host: "something.else.not.matching",
			})
			return err == nil
		}, time.Second*5, time.Millisecond*200)
	})
}
