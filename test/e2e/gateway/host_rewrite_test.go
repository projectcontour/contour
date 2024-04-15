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
	"k8s.io/utils/ptr"
	gatewayapi_v1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/projectcontour/contour/internal/gatewayapi"
	"github.com/projectcontour/contour/test/e2e"
)

func testHostRewrite(namespace string, gateway types.NamespacedName) {
	Specify("host can be rewritten in route filter", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "echo")

		route := &gatewayapi_v1.HTTPRoute{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "host-rewrite",
			},
			Spec: gatewayapi_v1.HTTPRouteSpec{
				Hostnames: []gatewayapi_v1.Hostname{"hostrewrite.gateway.projectcontour.io"},
				CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
					ParentRefs: []gatewayapi_v1.ParentReference{
						gatewayapi.GatewayParentRef(gateway.Namespace, gateway.Name),
					},
				},
				Rules: []gatewayapi_v1.HTTPRouteRule{
					{
						Matches: []gatewayapi_v1.HTTPRouteMatch{
							{
								Path: &gatewayapi_v1.HTTPPathMatch{
									Type:  ptr.To(gatewayapi_v1.PathMatchPathPrefix),
									Value: ptr.To("/"),
								},
							},
						},
						Filters: []gatewayapi_v1.HTTPRouteFilter{
							{
								Type: gatewayapi_v1.HTTPRouteFilterRequestHeaderModifier,
								RequestHeaderModifier: &gatewayapi_v1.HTTPHeaderFilter{
									Add: []gatewayapi_v1.HTTPHeader{
										{Name: gatewayapi_v1.HTTPHeaderName("Host"), Value: "rewritten.com"},
									},
								},
							},
						},
						BackendRefs: gatewayapi.HTTPBackendRef("echo", 80, 1),
					},
				},
			},
		}
		require.True(f.T(), f.CreateHTTPRouteAndWaitFor(route, e2e.HTTPRouteAccepted))

		res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      string(route.Spec.Hostnames[0]),
			Condition: e2e.HasStatusCode(200),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)
		body := f.GetEchoResponseBody(res.Body)
		assert.Equal(t, "echo", body.Service)
		assert.Equal(t, "rewritten.com", body.Host)
	})
}
