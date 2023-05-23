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

package gateway

import (
	. "github.com/onsi/ginkgo/v2"
	"github.com/projectcontour/contour/internal/gatewayapi"
	"github.com/projectcontour/contour/internal/ref"
	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gatewayapi_v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

func testTLSGateway(namespace string, gateway types.NamespacedName) {
	Specify("routes bound to port 443 listener are HTTPS and routes bound to port 80 listener are HTTP", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "echo-insecure")
		f.Fixtures.Echo.Deploy(namespace, "echo-secure")

		route := &gatewayapi_v1beta1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "http-route-1",
			},
			Spec: gatewayapi_v1beta1.HTTPRouteSpec{
				Hostnames: []gatewayapi_v1beta1.Hostname{"tls-gateway.projectcontour.io"},
				CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
					ParentRefs: []gatewayapi_v1beta1.ParentReference{
						{
							Namespace:   ref.To(gatewayapi_v1beta1.Namespace(gateway.Namespace)),
							Name:        gatewayapi_v1beta1.ObjectName(gateway.Name),
							SectionName: ref.To(gatewayapi_v1beta1.SectionName("insecure")),
						},
					},
				},
				Rules: []gatewayapi_v1beta1.HTTPRouteRule{
					{

						Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1beta1.PathMatchPathPrefix, "/"),
						BackendRefs: gatewayapi.HTTPBackendRef("echo-insecure", 80, 1),
					},
				},
			},
		}
		f.CreateHTTPRouteAndWaitFor(route, e2e.HTTPRouteAccepted)

		route = &gatewayapi_v1beta1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "http-route-2",
			},
			Spec: gatewayapi_v1beta1.HTTPRouteSpec{
				Hostnames: []gatewayapi_v1beta1.Hostname{"tls-gateway.projectcontour.io"},
				CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
					ParentRefs: []gatewayapi_v1beta1.ParentReference{
						{
							Namespace:   ref.To(gatewayapi_v1beta1.Namespace(gateway.Namespace)),
							Name:        gatewayapi_v1beta1.ObjectName(gateway.Name),
							SectionName: ref.To(gatewayapi_v1beta1.SectionName("secure")),
						},
					},
				},
				Rules: []gatewayapi_v1beta1.HTTPRouteRule{
					{
						Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1beta1.PathMatchPathPrefix, "/"),
						BackendRefs: gatewayapi.HTTPBackendRef("echo-secure", 80, 1),
					},
				},
			},
		}
		f.CreateHTTPRouteAndWaitFor(route, e2e.HTTPRouteAccepted)

		// Ensure http (insecure) request routes to echo-insecure.
		res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      "tls-gateway.projectcontour.io",
			Condition: e2e.HasStatusCode(200),
		})
		assert.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)
		assert.Equal(t, "echo-insecure", f.GetEchoResponseBody(res.Body).Service)

		// Ensure https (secure) request routes to echo-secure.
		res, ok = f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
			Host:      "tls-gateway.projectcontour.io",
			Condition: e2e.HasStatusCode(200),
		})
		assert.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)
		assert.Equal(t, "echo-secure", f.GetEchoResponseBody(res.Body).Service)
	})
}
