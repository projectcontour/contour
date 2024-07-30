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
	"crypto/tls"

	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/require"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	gatewayapi_v1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/projectcontour/contour/internal/gatewayapi"
	"github.com/projectcontour/contour/test/e2e"
)

func testTLSWildcardHost(namespace string, gateway types.NamespacedName) {
	Specify("wildcard hostname matching works with TLS", func() {
		t := f.T()
		hostSuffix := "wildcardhost.gateway.projectcontour.io"

		f.Fixtures.Echo.Deploy(namespace, "echo")

		route := &gatewayapi_v1.HTTPRoute{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "http-route-1",
			},
			Spec: gatewayapi_v1.HTTPRouteSpec{
				Hostnames: []gatewayapi_v1.Hostname{"*.wildcardhost.gateway.projectcontour.io"},
				CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
					ParentRefs: []gatewayapi_v1.ParentReference{
						{
							Namespace:   ptr.To(gatewayapi_v1.Namespace(gateway.Namespace)),
							Name:        gatewayapi_v1.ObjectName(gateway.Name),
							SectionName: ptr.To(gatewayapi_v1.SectionName("secure")),
						},
					},
				},
				Rules: []gatewayapi_v1.HTTPRouteRule{
					{
						Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
						BackendRefs: gatewayapi.HTTPBackendRef("echo", 80, 1),
					},
				},
			},
		}
		require.True(f.T(), f.CreateHTTPRouteAndWaitFor(route, e2e.HTTPRouteAccepted))

		cases := []struct {
			hostname   string
			sni        string
			wantStatus int
		}{
			{
				hostname:   "random1." + hostSuffix,
				sni:        "random1." + hostSuffix,
				wantStatus: 200,
			},
			{
				hostname:   "random2." + hostSuffix,
				sni:        "random2." + hostSuffix,
				wantStatus: 200,
			},
			{
				hostname:   "a.random3." + hostSuffix,
				sni:        "a.random3." + hostSuffix,
				wantStatus: 200,
			},
			{
				hostname:   "random4." + hostSuffix,
				sni:        "other-random4." + hostSuffix,
				wantStatus: 421,
			},
			{
				hostname:   "random5." + hostSuffix,
				sni:        "a.random5." + hostSuffix,
				wantStatus: 421,
			},
			{
				hostname:   "random6." + hostSuffix + ":9999",
				sni:        "random6." + hostSuffix,
				wantStatus: 200,
			},
		}

		for _, tc := range cases {
			t.Logf("Making request with hostname=%s, sni=%s", tc.hostname, tc.sni)

			res, ok := f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
				Host: tc.hostname,
				TLSConfigOpts: []func(*tls.Config){
					e2e.OptSetSNI(tc.sni),
				},
				Condition: e2e.HasStatusCode(tc.wantStatus),
			})
			require.NotNil(t, res, "request never succeeded")
			require.Truef(t, ok, "expected %d response code, got %d", tc.wantStatus, res.StatusCode)
		}
	})
}
