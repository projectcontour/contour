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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gatewayapi_v1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/projectcontour/contour/internal/gatewayapi"
	"github.com/projectcontour/contour/test/e2e"
)

func testGatewayMultipleQueryParamMatch(namespace string, gateway types.NamespacedName) {
	Specify("first of multiple query params in a request is used for routing", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "echo-1")
		f.Fixtures.Echo.Deploy(namespace, "echo-2")
		f.Fixtures.Echo.Deploy(namespace, "echo-3")
		f.Fixtures.Echo.Deploy(namespace, "echo-4")

		route := &gatewayapi_v1.HTTPRoute{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "httproute-1",
			},
			Spec: gatewayapi_v1.HTTPRouteSpec{
				Hostnames: []gatewayapi_v1.Hostname{"queryparams.gateway.projectcontour.io"},
				CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
					ParentRefs: []gatewayapi_v1.ParentReference{
						gatewayapi.GatewayParentRef(gateway.Namespace, gateway.Name),
					},
				},
				Rules: []gatewayapi_v1.HTTPRouteRule{
					{
						Matches: []gatewayapi_v1.HTTPRouteMatch{
							{QueryParams: gatewayapi.HTTPQueryParamMatches(map[string]string{"animal": "whale"})},
						},
						BackendRefs: gatewayapi.HTTPBackendRef("echo-1", 80, 1),
					},
					{
						Matches: []gatewayapi_v1.HTTPRouteMatch{
							{QueryParams: gatewayapi.HTTPQueryParamMatches(map[string]string{"animal": "dolphin"})},
						},
						BackendRefs: gatewayapi.HTTPBackendRef("echo-2", 80, 1),
					},
				},
			},
		}
		require.True(f.T(), f.CreateHTTPRouteAndWaitFor(route, e2e.HTTPRouteAccepted))

		cases := map[string]string{
			// These test cases are for documenting Envoy's behavior when
			// handling requests with multiple query params with the same key:
			// The first occurrence is used for matching.
			"/?animal=whale&animal=dolphin": "echo-1",
			"/?animal=dolphin&animal=whale": "echo-2",
		}

		for path, expectedService := range cases {
			t.Logf("Querying %q, expecting service %q", path, expectedService)

			res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
				Host:      string(route.Spec.Hostnames[0]),
				Path:      path,
				Condition: e2e.HasStatusCode(200),
			})
			require.NotNil(t, res)
			if !assert.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode) {
				continue
			}

			body := f.GetEchoResponseBody(res.Body)
			assert.Equal(t, namespace, body.Namespace)
			assert.Equal(t, expectedService, body.Service)
		}
	})
}
