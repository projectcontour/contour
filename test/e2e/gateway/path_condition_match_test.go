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
	gatewayapi_v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

func testGatewayPathConditionMatch(namespace string) {
	Specify("path match routing works", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "echo-slash-prefix")
		f.Fixtures.Echo.Deploy(namespace, "echo-slash-default")
		f.Fixtures.Echo.Deploy(namespace, "echo-slash-exact")

		route := &gatewayapi_v1beta1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "http-filter-1",
			},
			Spec: gatewayapi_v1beta1.HTTPRouteSpec{
				Hostnames: []gatewayapi_v1beta1.Hostname{"gatewaypathconditions.projectcontour.io"},
				CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
					ParentRefs: []gatewayapi_v1beta1.ParentReference{
						gatewayapi.GatewayParentRef("", "http"), // TODO need a better way to inform the test case of the Gateway it should use
					},
				},
				Rules: []gatewayapi_v1beta1.HTTPRouteRule{
					{
						Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1beta1.PathMatchPathPrefix, "/path/prefix"),
						BackendRefs: gatewayapi.HTTPBackendRef("echo-slash-prefix", 80, 1),
					},

					{
						Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1beta1.PathMatchExact, "/path/exact"),
						BackendRefs: gatewayapi.HTTPBackendRef("echo-slash-exact", 80, 1),
					},

					{
						Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1beta1.PathMatchPathPrefix, "/"),
						BackendRefs: gatewayapi.HTTPBackendRef("echo-slash-default", 80, 1),
					},
				},
			},
		}
		f.CreateHTTPRouteAndWaitFor(route, httpRouteAccepted)

		cases := map[string]string{
			"/":    "echo-slash-default",
			"/foo": "echo-slash-default",

			"/path/prefix":         "echo-slash-prefix",
			"/path/prefix/":        "echo-slash-prefix",
			"/path/prefix/foo":     "echo-slash-prefix",
			"/path/prefix/foo/bar": "echo-slash-prefix",
			"/path/prefixfoo":      "echo-slash-default", // not a segment prefix match
			"/foo/path/prefix":     "echo-slash-default",

			"/path/exact":     "echo-slash-exact",
			"/path/exactfoo":  "echo-slash-default",
			"/path/exact/":    "echo-slash-default",
			"/path/exact/foo": "echo-slash-default",
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
