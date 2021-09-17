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
	. "github.com/onsi/ginkgo"
	"github.com/projectcontour/contour/internal/gatewayapi"
	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func testHostRewrite(namespace string) {
	Specify("host can be rewritten in route filter", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "echo")

		route := &gatewayapi_v1alpha2.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "host-rewrite",
			},
			Spec: gatewayapi_v1alpha2.HTTPRouteSpec{
				Hostnames: []gatewayapi_v1alpha2.Hostname{"hostrewrite.gateway.projectcontour.io"},
				CommonRouteSpec: gatewayapi_v1alpha2.CommonRouteSpec{
					ParentRefs: []gatewayapi_v1alpha2.ParentRef{
						gatewayapi.GatewayParentRef("", "http"), // TODO need a better way to inform the test case of the Gateway it should use
					},
				},
				Rules: []gatewayapi_v1alpha2.HTTPRouteRule{
					{
						Matches: []gatewayapi_v1alpha2.HTTPRouteMatch{
							{
								Path: &gatewayapi_v1alpha2.HTTPPathMatch{
									Type:  gatewayapi.PathMatchTypePtr(gatewayapi_v1alpha2.PathMatchPrefix),
									Value: pointer.StringPtr("/"),
								},
							},
						},
						Filters: []gatewayapi_v1alpha2.HTTPRouteFilter{
							{
								Type: gatewayapi_v1alpha2.HTTPRouteFilterRequestHeaderModifier,
								RequestHeaderModifier: &gatewayapi_v1alpha2.HTTPRequestHeaderFilter{
									Add: []gatewayapi_v1alpha2.HTTPHeader{
										{Name: gatewayapi_v1alpha2.HTTPHeaderName("Host"), Value: "rewritten.com"},
									},
								},
							},
						},
						BackendRefs: []gatewayapi_v1alpha2.HTTPBackendRef{
							{
								BackendRef: gatewayapi_v1alpha2.BackendRef{
									BackendObjectReference: gatewayapi.ServiceBackendObjectRef("echo", 80),
								},
							},
						},
					},
				},
			},
		}
		f.CreateHTTPRouteAndWaitFor(route, httpRouteAdmitted)

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
