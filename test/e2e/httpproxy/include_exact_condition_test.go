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
	"context"

	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/test/e2e"
)

func testIncludeExactCondition(namespace string) {
	Specify("HTTPProxy include exacts can cross namespaces", func() {
		var (
			t              = f.T()
			appNamespace   = "httpproxy-include-exact-condition-app"
			adminNamespace = "httpproxy-include-exact-condition-admin"
		)

		for _, ns := range []string{appNamespace, adminNamespace} {
			f.CreateNamespace(ns)
			defer f.DeleteNamespace(ns, false)
		}

		f.Fixtures.Echo.Deploy(appNamespace, "echo-app")
		f.Fixtures.Echo.Deploy(adminNamespace, "echo-admin")

		appProxy := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: appNamespace,
				Name:      "echo-app",
			},
			Spec: contour_v1.HTTPProxySpec{
				Routes: []contour_v1.Route{
					{
						Services: []contour_v1.Service{
							{
								Name: "echo-app",
								Port: 80,
							},
						},
						Conditions: []contour_v1.MatchCondition{
							{
								Exact: "/foo",
							},
						},
					},
					{
						Services: []contour_v1.Service{
							{
								Name: "echo-app",
								Port: 80,
							},
						},
						Conditions: []contour_v1.MatchCondition{
							{
								Prefix: "/v1",
							},
						},
					},
				},
			},
		}
		// appProxy will be orphaned when created so can't wait for
		// it to be valid.
		require.NoError(t, f.Client.Create(context.TODO(), appProxy))

		adminProxy := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: adminNamespace,
				Name:      "echo-admin",
			},
			Spec: contour_v1.HTTPProxySpec{
				Routes: []contour_v1.Route{
					{
						Services: []contour_v1.Service{
							{
								Name: "echo-admin",
								Port: 80,
							},
						},
						Conditions: []contour_v1.MatchCondition{
							{
								Prefix: "/",
							},
						},
					},
					{
						Services: []contour_v1.Service{
							{
								Name: "echo-admin",
								Port: 80,
							},
						},
						Conditions: []contour_v1.MatchCondition{
							{
								Exact: "/portal",
							},
						},
					},
				},
			},
		}
		// adminProxy will be orphaned when created so can't wait for
		// it to be valid.
		require.NoError(t, f.Client.Create(context.TODO(), adminProxy))

		baseProxy := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "echo",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "includeexactcondition.projectcontour.io",
				},
				Includes: []contour_v1.Include{
					{
						Name:      appProxy.Name,
						Namespace: appProxy.Namespace,
						Conditions: []contour_v1.MatchCondition{
							{
								Prefix: "/app",
							},
						},
					},
					{
						Name:      adminProxy.Name,
						Namespace: adminProxy.Namespace,
						Conditions: []contour_v1.MatchCondition{
							{
								Prefix: "/app/",
							},
						},
					},
					{
						Name:      adminProxy.Name,
						Namespace: adminProxy.Namespace,
						Conditions: []contour_v1.MatchCondition{
							{
								Prefix: "/admin",
							},
						},
					},
				},
			},
		}
		invalidRootProxy := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "echo-invalid",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "includeexactcondition-invalid.projectcontour.io",
				},
				Includes: []contour_v1.Include{
					{
						Name:      appProxy.Name,
						Namespace: appProxy.Namespace,
						Conditions: []contour_v1.MatchCondition{
							{
								Exact: "/app",
							},
						},
					},
				},
			},
		}
		require.True(f.T(), f.CreateHTTPProxyAndWaitFor(baseProxy, e2e.HTTPProxyValid))
		require.True(f.T(), f.CreateHTTPProxyAndWaitFor(invalidRootProxy, e2e.HTTPProxyInvalid))

		cases := map[string]string{
			"/app/foo":      "echo-app",   // Condition matched: "Prefix: /app"   +  "Exact:  /foo"    = "Exact:  /app/foo"
			"/app/admin":    "echo-admin", // Condition matched: "Prefix: /app/"  +  "Prefix: /"       = "Prefix: /app"
			"/app/bar":      "echo-admin", // Condition matched: "Prefix: /app/"  +  "Prefix: /"       = "Prefix: /app"
			"/app/v1":       "echo-app",   // Condition matched: "Prefix: /app"   +  "Prefix: /v1"     = "Prefix: /app/v1"
			"/app/v1/page":  "echo-app",   // Condition matched: "Prefix: /app"   +  "Prefix: /v1"     = "Prefix: /app/v1"
			"/admin/portal": "echo-admin", // Condition matched: "Prefix: /admin" +  "Exact:  /portal" = "Exact:  /app/v1"
		}

		for path, expectedService := range cases {
			t.Logf("Querying %q, expecting service %q", path, expectedService)

			res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
				Host:      baseProxy.Spec.VirtualHost.Fqdn,
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
