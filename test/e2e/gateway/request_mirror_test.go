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
	"context"
	"regexp"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	. "github.com/onsi/ginkgo/v2"
	"github.com/projectcontour/contour/internal/gatewayapi"
	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gatewayapi_v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

func testRequestMirrorRule(namespace string, gateway types.NamespacedName) {
	// Flake tracking issue: https://github.com/projectcontour/contour/issues/4650
	Specify("mirrors can be specified on route rule", FlakeAttempts(3), func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "echo-primary")
		f.Fixtures.Echo.DeployN(namespace, "echo-shadow", 2)

		route := &gatewayapi_v1beta1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "httproute-mirror",
			},
			Spec: gatewayapi_v1beta1.HTTPRouteSpec{
				Hostnames: []gatewayapi_v1beta1.Hostname{"requestmirrorrule.gateway.projectcontour.io"},
				CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
					ParentRefs: []gatewayapi_v1beta1.ParentReference{
						gatewayapi.GatewayParentRef(gateway.Namespace, gateway.Name),
					},
				},
				Rules: []gatewayapi_v1beta1.HTTPRouteRule{
					{
						Matches: gatewayapi.HTTPRouteMatch(gatewayapi_v1beta1.PathMatchPathPrefix, "/mirror"),
						Filters: []gatewayapi_v1beta1.HTTPRouteFilter{
							{
								Type: gatewayapi_v1beta1.HTTPRouteFilterRequestMirror,
								RequestMirror: &gatewayapi_v1beta1.HTTPRequestMirrorFilter{
									BackendRef: gatewayapi.ServiceBackendObjectRef("echo-shadow", 80),
								},
							},
						},
						BackendRefs: gatewayapi.HTTPBackendRef("echo-primary", 80, 1),
					},
				},
			},
		}
		f.CreateHTTPRouteAndWaitFor(route, e2e.HTTPRouteAccepted)

		// Wait for "echo-shadow" deployment to be available
		require.Eventually(f.T(), func() bool {
			d := &appsv1.Deployment{}
			if err := f.Client.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: "echo-shadow"}, d); err != nil {
				return false
			}
			for _, c := range d.Status.Conditions {
				return c.Type == appsv1.DeploymentAvailable && c.Status == corev1.ConditionTrue
			}
			return false
		}, f.RetryTimeout, f.RetryInterval)

		res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      string(route.Spec.Hostnames[0]),
			Path:      "/mirror",
			Condition: e2e.HasStatusCode(200),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)

		// Ensure the request was mirrored to one of "echo-shadow" pods via logs
		require.Eventually(t, func() bool {
			var mirrored bool
			mirrorLogRegexp := regexp.MustCompile(`Echoing back request made to \/mirror to client`)

			logs, err := f.Fixtures.Echo.DumpEchoLogs(namespace, "echo-shadow")
			if err != nil {
				return false
			}

			for _, log := range logs {
				if mirrorLogRegexp.MatchString(string(log)) {
					mirrored = true
				}
			}
			return mirrored

		}, f.RetryTimeout, f.RetryInterval)
	})
}
