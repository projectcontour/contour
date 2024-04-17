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
	"github.com/stretchr/testify/require"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gatewayapi_v1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/projectcontour/contour/internal/gatewayapi"
	"github.com/projectcontour/contour/test/e2e"
)

func testHTTPRouteConflictMatch(namespace string, gateway types.NamespacedName) {
	Specify("Creates two http routes, second one has conflict match against the first one, report Accepted: false", func() {
		By("create httproute-1 first")
		route1 := &gatewayapi_v1.HTTPRoute{
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
		require.True(f.T(), f.CreateHTTPRouteAndWaitFor(route1, e2e.HTTPRouteAccepted))

		By("create httproute-2 with conflicted matches")
		route2 := &gatewayapi_v1.HTTPRoute{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "httproute-2",
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
				},
			},
		}
		require.True(f.T(), f.CreateHTTPRouteAndWaitFor(route2, e2e.HTTPRouteNotAcceptedDueToConflict))
	})
}
