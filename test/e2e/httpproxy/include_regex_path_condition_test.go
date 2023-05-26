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
	"context"

	. "github.com/onsi/ginkgo/v2"
	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func testIncludeRegexCondition(namespace string) {
	Specify("HTTPProxy with included regex and prefix HTTPProxies", func() {
		var (
			t              = f.T()
			echo1Namespace = "echo-1"
			echo2Namespace = "echo-2"
		)

		for _, ns := range []string{echo1Namespace, echo2Namespace} {
			f.CreateNamespace(ns)
			defer f.DeleteNamespace(ns, false)
		}

		f.Fixtures.Echo.Deploy(echo1Namespace, "echo-1")
		f.Fixtures.Echo.Deploy(echo2Namespace, "echo-2")

		echo1Proxy := &contourv1.HTTPProxy{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: echo1Namespace,
				Name:      "echo-1",
			},
			Spec: contourv1.HTTPProxySpec{
				Routes: []contourv1.Route{
					{
						Services: []contourv1.Service{
							{
								Name: "echo-1",
								Port: 80,
							},
						},
						Conditions: []contourv1.MatchCondition{
							{
								Regex: "/us-west-3/.*",
							},
						},
					},
					{
						Services: []contourv1.Service{
							{
								Name: "echo-1",
								Port: 80,
							},
						},
						Conditions: []contourv1.MatchCondition{
							{
								Regex: "/us-west-1/.*",
							},
						},
					},
				},
			},
		}

		require.NoError(t, f.Client.Create(context.TODO(), echo1Proxy))

		echo2Proxy := &contourv1.HTTPProxy{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: echo2Namespace,
				Name:      "echo-2",
			},
			Spec: contourv1.HTTPProxySpec{
				Routes: []contourv1.Route{
					{
						Services: []contourv1.Service{
							{
								Name: "echo-2",
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
								Name: "echo-2",
								Port: 80,
							},
						},
						Conditions: []contourv1.MatchCondition{
							{
								Regex: "/(dev|staging)/.*",
							},
						},
					},
				},
			},
		}

		require.NoError(t, f.Client.Create(context.TODO(), echo2Proxy))

		rootProxy := &contourv1.HTTPProxy{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "echo",
			},
			Spec: contourv1.HTTPProxySpec{
				VirtualHost: &contourv1.VirtualHost{
					Fqdn: "includeregexmatch.projectcontour.io",
				},
				Includes: []contourv1.Include{
					{
						Name:      echo1Proxy.Name,
						Namespace: echo1Proxy.Namespace,
						Conditions: []contourv1.MatchCondition{
							{
								Prefix: "/echo1",
							},
						},
					},
					{
						Name:      echo2Proxy.Name,
						Namespace: echo2Proxy.Namespace,
						Conditions: []contourv1.MatchCondition{
							{
								Prefix: "/echo2",
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
					Fqdn: "regex-condition-invalid.projectcontour.io",
				},
				Includes: []contourv1.Include{
					{
						Name:      echo1Proxy.Name,
						Namespace: echo1Proxy.Namespace,
						Conditions: []contourv1.MatchCondition{
							{
								Regex: "/echo.*",
							},
						},
					},
				},
			},
		}
		f.CreateHTTPProxyAndWaitFor(rootProxy, e2e.HTTPProxyValid)

		f.CreateHTTPProxyAndWaitFor(invalidRootProxy, e2e.HTTPProxyInvalid)

		cases := map[string]string{
			"/echo2/":               "echo-2", // "Prefix: / with included Prefix: echo2/"
			"/echo1/us-west-1/test": "echo-1", // "Prefix: /echo1 with included Regex: /us-west-1/.*"
			"/echo1/us-west-3/test": "echo-1", // "Prefix: /echo1 with included Regex: /us-west-3/.*"
			"/echo2/dev/utils":      "echo-2", // "Prefix: /echo2 with included Regex: /(dev|staging)/.*"
		}

		for path, expectedService := range cases {
			t.Logf("Querying %q, expecting service %q", path, expectedService)

			res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
				Host:      rootProxy.Spec.VirtualHost.Fqdn,
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
