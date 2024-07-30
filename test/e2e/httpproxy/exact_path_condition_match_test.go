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

package httpproxy

import (
	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/test/e2e"
)

func testExactPathCondition(namespace string) {
	Specify("Exact path match routing works and has precedence over prefix match", func() {
		var (
			t                = f.T()
			serviceNamespace = namespace
		)

		f.Fixtures.Echo.Deploy(serviceNamespace, "echo-blue")
		f.Fixtures.Echo.Deploy(serviceNamespace, "echo-green")
		f.Fixtures.Echo.Deploy(serviceNamespace, "echo-default")

		serviceProxy := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: serviceNamespace,
				Name:      "echo-exact",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "exactpathcondition.projectcontour.io",
				},
				Routes: []contour_v1.Route{
					{
						Services: []contour_v1.Service{
							{
								Name: "echo-blue",
								Port: 80,
							},
						},
						Conditions: []contour_v1.MatchCondition{
							{
								Prefix: "/blue",
							},
						},
					},
					{
						Services: []contour_v1.Service{
							{
								Name: "echo-blue",
								Port: 80,
							},
						},
						Conditions: []contour_v1.MatchCondition{
							{
								Exact: "/common/exact-blue",
							},
						},
					},
					{
						Services: []contour_v1.Service{
							{
								Name: "echo-green",
								Port: 80,
							},
						},
						Conditions: []contour_v1.MatchCondition{
							{
								Prefix: "/common/exact-blue/",
							},
						},
					},
					{
						Services: []contour_v1.Service{
							{
								Name: "echo-green",
								Port: 80,
							},
						},
						Conditions: []contour_v1.MatchCondition{
							{
								Exact: "/blue-exact-green",
							},
						},
					},
					{
						Services: []contour_v1.Service{
							{
								Name: "echo-green",
								Port: 80,
							},
						},
						Conditions: []contour_v1.MatchCondition{
							{
								Exact: "/blue/exact-green",
							},
						},
					},
					{
						Services: []contour_v1.Service{
							{
								Name: "echo-default",
								Port: 80,
							},
						},
						Conditions: []contour_v1.MatchCondition{
							{
								Prefix: "/",
							},
						},
					},
				},
			},
		}

		require.True(f.T(), f.CreateHTTPProxyAndWaitFor(serviceProxy, e2e.HTTPProxyValid))

		cases := map[string]string{
			"/":                        "echo-default", // Condition matched: "Prefix: /"
			"/blue":                    "echo-blue",    // Condition matched: "Prefix: /blue"
			"/blue/exact-green":        "echo-green",   // Condition matched: "Exact:  /blue/exact" (exact takes precedence over "Prefix: /blue")
			"/common/exact-blue":       "echo-blue",    // Condition matched: "Exact:  /common/exact-blue"
			"/common/exact-blue/green": "echo-green",   // Condition matched: "Prefix: /common/exact-blue/"
			"/blue-exact-green":        "echo-green",   // Condition matched: "Exact:  /blue-exact-green"
			"/blue-exact-green/extra":  "echo-blue",    // Condition matched: "Prefix: /blue"
			"/app":                     "echo-default", // Condition matched: "Prefix: /"
		}

		for path, expectedService := range cases {
			t.Logf("Querying %q, expecting service %q", path, expectedService)

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
