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
	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

		appProxy := &contourv1.HTTPProxy{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: appNamespace,
				Name:      "echo-app",
			},
			Spec: contourv1.HTTPProxySpec{
				Routes: []contourv1.Route{
					{
						Services: []contourv1.Service{
							{
								Name: "echo-app",
								Port: 80,
							},
						},
						Conditions: []contourv1.MatchCondition{
							{
								Exact: "/foo",
							},
						},
					},
					{
						Services: []contourv1.Service{
							{
								Name: "echo-app",
								Port: 80,
							},
						},
						Conditions: []contourv1.MatchCondition{
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

		adminProxy := &contourv1.HTTPProxy{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: adminNamespace,
				Name:      "echo-admin",
			},
			Spec: contourv1.HTTPProxySpec{
				Routes: []contourv1.Route{
					{
						Services: []contourv1.Service{
							{
								Name: "echo-admin",
								Port: 80,
							},
						},
						Conditions: []contourv1.MatchCondition{
							{
								Prefix: "/",
							},
						},
					},
					{
						Services: []contourv1.Service{
							{
								Name: "echo-admin",
								Port: 80,
							},
						},
						Conditions: []contourv1.MatchCondition{
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

		baseProxy := &contourv1.HTTPProxy{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "echo",
			},
			Spec: contourv1.HTTPProxySpec{
				VirtualHost: &contourv1.VirtualHost{
					Fqdn: "includeexactcondition.projectcontour.io",
				},
				Includes: []contourv1.Include{
					{
						Name:      appProxy.Name,
						Namespace: appProxy.Namespace,
						Conditions: []contourv1.MatchCondition{
							{
								Prefix: "/app",
							},
						},
					},
					{
						Name:      adminProxy.Name,
						Namespace: adminProxy.Namespace,
						Conditions: []contourv1.MatchCondition{
							{
								Prefix: "/app/",
							},
						},
					},
					{
						Name:      adminProxy.Name,
						Namespace: adminProxy.Namespace,
						Conditions: []contourv1.MatchCondition{
							{
								Prefix: "/admin",
							},
						},
					},
				},
			},
		}
		invalidRootProxy := &contourv1.HTTPProxy{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "echo-invalid",
			},
			Spec: contourv1.HTTPProxySpec{
				VirtualHost: &contourv1.VirtualHost{
					Fqdn: "includeexactcondition-invalid.projectcontour.io",
				},
				Includes: []contourv1.Include{
					{
						Name:      appProxy.Name,
						Namespace: appProxy.Namespace,
						Conditions: []contourv1.MatchCondition{
							{
								Exact: "/app",
							},
						},
					},
				},
			},
		}
		f.CreateHTTPProxyAndWaitFor(baseProxy, e2e.HTTPProxyValid)
		f.CreateHTTPProxyAndWaitFor(invalidRootProxy, e2e.HTTPProxyInvalid)

		cases := map[string]string{
			"/app/foo":      "echo-app",
			"/app/admin":    "echo-admin",
			"/app/bar":      "echo-admin",
			"/app/v1":       "echo-app",
			"/app/v1/page":  "echo-app",
			"/admin/portal": "echo-admin",
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
