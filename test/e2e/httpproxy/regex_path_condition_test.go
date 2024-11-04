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

func testRegexPathCondition(namespace string) {
	Specify("Regex path matching works", func() {
		var (
			t                = f.T()
			serviceNamespace = namespace
		)

		f.Fixtures.Echo.Deploy(serviceNamespace, "echo-1")
		f.Fixtures.Echo.Deploy(serviceNamespace, "echo-2")
		f.Fixtures.Echo.Deploy(serviceNamespace, "echo-3")

		serviceProxy := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: serviceNamespace,
				Name:      "regex",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "regexpath.projectcontour.io",
				},
				Routes: []contour_v1.Route{
					{
						Services: []contour_v1.Service{
							{
								Name: "echo-2",
								Port: 80,
							},
						},
						Conditions: []contour_v1.MatchCondition{
							{
								Regex: "/apiv1/prod-.+/",
							},
						},
					},
					{
						Services: []contour_v1.Service{
							{
								Name: "echo-3",
								Port: 80,
							},
						},
						Conditions: []contour_v1.MatchCondition{
							{
								Regex: "/(local|global)/.*/",
							},
						},
					},
					{
						Services: []contour_v1.Service{
							{
								Name: "echo-1",
								Port: 80,
							},
						},
						Conditions: []contour_v1.MatchCondition{
							{
								Regex: "/[a-zA-Z]+",
							},
						},
					},
					{
						Services: []contour_v1.Service{
							{
								Name: "echo-3",
								Port: 80,
							},
						},
						Conditions: []contour_v1.MatchCondition{
							{
								Regex: "/[\\d]+/.+/",
							},
						},
					},
					{
						Services: []contour_v1.Service{
							{
								Name: "echo-1",
								Port: 80,
							},
						},
						Conditions: []contour_v1.MatchCondition{
							{
								Regex: "/base/.*",
							},
						},
					},
					{
						Services: []contour_v1.Service{
							{
								Name: "echo-1",
								Port: 80,
							},
						},
						Conditions: []contour_v1.MatchCondition{
							{
								Regex: "/",
							},
						},
					},
				},
			},
		}

		require.True(f.T(), f.CreateHTTPProxyAndWaitFor(serviceProxy, e2e.HTTPProxyValid))

		cases := map[string]string{
			"/":                      "echo-1", // Regex Pattern /
			"/apiv1/prod-v2/echo-2/": "echo-2", // Regex Pattern apiv1/prod-.+/
			"/local/echo-3/":         "echo-3", // Regex Pattern /local|global/.*
			"/echo":                  "echo-1", // Regex Pattern /[a-zA-Z]+
			"/3/echo/":               "echo-3", // Regex Pattern /[\d]+/.+/
			"/base/root":             "echo-1", // Regex Pattern /base/.*
		}

		for path, expectedService := range cases {
			t.Logf("Querying path: %q, expecting service to be called: %q", path, expectedService)

			res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
				Host:      serviceProxy.Spec.VirtualHost.Fqdn,
				Path:      path,
				Condition: e2e.HasStatusCode(200),
			})
			if !assert.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode) {
				continue
			}

			assert.Equal(t, expectedService, f.GetEchoResponseBody(res.Body).Service)
		}
	})
}
