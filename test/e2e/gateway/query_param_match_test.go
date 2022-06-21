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
	. "github.com/onsi/ginkgo/v2"
	"github.com/projectcontour/contour/internal/gatewayapi"
	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func testGatewayQueryParamMatch(namespace string) {
	Specify("query param matching works", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "echo-1")
		f.Fixtures.Echo.Deploy(namespace, "echo-2")
		f.Fixtures.Echo.Deploy(namespace, "echo-3")
		f.Fixtures.Echo.Deploy(namespace, "echo-4")

		route := &gatewayapi_v1alpha2.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "httproute-1",
			},
			Spec: gatewayapi_v1alpha2.HTTPRouteSpec{
				Hostnames: []gatewayapi_v1alpha2.Hostname{"queryparams.gateway.projectcontour.io"},
				CommonRouteSpec: gatewayapi_v1alpha2.CommonRouteSpec{
					ParentRefs: []gatewayapi_v1alpha2.ParentReference{
						gatewayapi.GatewayParentRef("", "http"),
					},
				},
				Rules: []gatewayapi_v1alpha2.HTTPRouteRule{
					{
						Matches: []gatewayapi_v1alpha2.HTTPRouteMatch{
							{QueryParams: gatewayapi.HTTPQueryParamMatches(map[string]string{"animal": "whale"})},
						},
						BackendRefs: gatewayapi.HTTPBackendRef("echo-1", 80, 1),
					},
					{
						Matches: []gatewayapi_v1alpha2.HTTPRouteMatch{
							{QueryParams: gatewayapi.HTTPQueryParamMatches(map[string]string{"animal": "dolphin"})},
						},
						BackendRefs: gatewayapi.HTTPBackendRef("echo-2", 80, 1),
					},
					{
						Matches: []gatewayapi_v1alpha2.HTTPRouteMatch{
							{QueryParams: gatewayapi.HTTPQueryParamMatches(map[string]string{"animal": "dolphin", "color": "red"})},
						},
						BackendRefs: gatewayapi.HTTPBackendRef("echo-3", 80, 1),
					},
					{
						Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1alpha2.PathMatchPathPrefix, "/"),
						BackendRefs: gatewayapi.HTTPBackendRef("echo-4", 80, 1),
					},
				},
			},
		}
		f.CreateHTTPRouteAndWaitFor(route, httpRouteAccepted)

		cases := map[string]string{
			"/?animal=whale":                     "echo-1",
			"/?animal=whale&foo=bar":             "echo-1",
			"/?animal=dolphin":                   "echo-2",
			"/?animal=dolphin&color=blue":        "echo-2",
			"/?animal=dolphin&color=red":         "echo-3",
			"/?animal=dolphin&color=red&foo=bar": "echo-3",
			"/?animal=horse":                     "echo-4",
			"/?animal=whalesay":                  "echo-4",
			"/?animal=bluedolphin":               "echo-4",
			"/?color=blue":                       "echo-4",
			"/?nomatch=true":                     "echo-4",
		}

		for path, expectedService := range cases {
			t.Logf("Querying %q, expecting service %q", path, expectedService)

			res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
				Host:      string(route.Spec.Hostnames[0]),
				Path:      path,
				Condition: e2e.HasStatusCode(200),
			})
			if !assert.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode) {
				continue
			}

			body := f.GetEchoResponseBody(res.Body)
			assert.Equal(t, namespace, body.Namespace)
			assert.Equal(t, expectedService, body.Service)
		}
	})
}
