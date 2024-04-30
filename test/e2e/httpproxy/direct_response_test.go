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

package httpproxy

import (
	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/test/e2e"
)

func testDirectResponseRule(namespace string) {
	Specify("direct response can be specified on route rule", func() {
		t := f.T()
		proxy := getDirectResponseHTTPProxy(namespace)
		doDirectTest(namespace, proxy, t)
	})
}

func doDirectTest(namespace string, proxy *contour_v1.HTTPProxy, t GinkgoTInterface) {
	f.Fixtures.Echo.Deploy(namespace, "echo")

	require.True(f.T(), f.CreateHTTPProxyAndWaitFor(proxy, e2e.HTTPProxyValid))

	assertDirectResponseRequest(t, proxy.Spec.VirtualHost.Fqdn, "/directresponse-nobody",
		"", 200)

	assertDirectResponseRequest(t, proxy.Spec.VirtualHost.Fqdn, "/directresponse",
		"directResponse success", 200)

	assertDirectResponseRequest(t, proxy.Spec.VirtualHost.Fqdn, "/directresponse-notfound",
		"not found", 404)
}

func assertDirectResponseRequest(t GinkgoTInterface, fqdn, path, expectedBody string, expectedStatusCode int) {
	res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
		Host:      fqdn,
		Path:      path,
		Condition: e2e.HasStatusCode(expectedStatusCode),
	})
	require.NotNil(t, res, "request never succeeded")
	require.Truef(t, ok, "expected %d response code, got %d", expectedStatusCode, res.StatusCode)
	assert.Equal(t, expectedBody, string(res.Body))
}

func getDirectResponseHTTPProxy(namespace string) *contour_v1.HTTPProxy {
	return &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "direct-response",
			Namespace: namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "directresponse.projectcontour.io",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/directresponse-nobody",
				}},
				DirectResponsePolicy: &contour_v1.HTTPDirectResponsePolicy{StatusCode: 200},
			}, {
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/directresponse",
				}},
				DirectResponsePolicy: &contour_v1.HTTPDirectResponsePolicy{
					StatusCode: 200,
					Body:       "directResponse success",
				},
			}, {
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/directresponse-notfound",
				}},
				DirectResponsePolicy: &contour_v1.HTTPDirectResponsePolicy{
					StatusCode: 404,
					Body:       "not found",
				},
			}},
		},
	}
}
