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
	"regexp"
	"time"

	. "github.com/onsi/ginkgo/v2"
	"github.com/projectcontour/contour/internal/gatewayapi"
	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func testRequestMirrorRule(namespace string) {
	Specify("mirrors can be specified on route rule", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "echo-primary")
		f.Fixtures.Echo.DeployN(namespace, "echo-shadow", 2)

		route := &gatewayapi_v1alpha2.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "httproute-mirror",
			},
			Spec: gatewayapi_v1alpha2.HTTPRouteSpec{
				Hostnames: []gatewayapi_v1alpha2.Hostname{"requestmirrorrule.gateway.projectcontour.io"},
				CommonRouteSpec: gatewayapi_v1alpha2.CommonRouteSpec{
					ParentRefs: []gatewayapi_v1alpha2.ParentRef{
						gatewayapi.GatewayParentRef("", "http"), // TODO need a better way to inform the test case of the Gateway it should use
					},
				},
				Rules: []gatewayapi_v1alpha2.HTTPRouteRule{
					{
						Matches: gatewayapi.HTTPRouteMatch(gatewayapi_v1alpha2.PathMatchPathPrefix, "/mirror"),
						Filters: []gatewayapi_v1alpha2.HTTPRouteFilter{
							{
								Type: gatewayapi_v1alpha2.HTTPRouteFilterRequestMirror,
								RequestMirror: &gatewayapi_v1alpha2.HTTPRequestMirrorFilter{
									BackendRef: gatewayapi.ServiceBackendObjectRef("echo-shadow", 80),
								},
							},
						},
						BackendRefs: gatewayapi.HTTPBackendRef("echo-primary", 80, 1),
					},
				},
			},
		}
		f.CreateHTTPRouteAndWaitFor(route, httpRouteAccepted)

		res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      string(route.Spec.Hostnames[0]),
			Path:      "/mirror",
			Condition: e2e.HasStatusCode(200),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)

		// Wait for echo logs successfully applied to the Kubernetes API Server
		// If this test fails, sufficiently increase the delay and try again
		time.Sleep(1 * time.Second)

		// Ensure the request was mirrored to one of "echo-shadow" pods via logs
		var mirrored bool
		mirrorLogRegexp := regexp.MustCompile(`Echoing back request made to \/mirror to client`)

		logs, err := f.Fixtures.Echo.DumpEchoLogs(namespace, "echo-shadow")
		require.NoError(f.T(), err)
		for _, log := range logs {
			if mirrorLogRegexp.MatchString(string(log)) {
				mirrored = true
			}
		}
		require.True(t, mirrored, "expected the request to be mirrored")
	})
}
