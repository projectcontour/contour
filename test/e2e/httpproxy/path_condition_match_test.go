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

func testPathConditionMatch(namespace string) {
	Specify("path match routing works", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "echo-slash-prefix")
		f.Fixtures.Echo.Deploy(namespace, "echo-slash-noprefix")
		f.Fixtures.Echo.Deploy(namespace, "echo-slash-default")

		p := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "path-conditions",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "pathconditions.projectcontour.io",
				},
				Routes: []contour_v1.Route{
					{
						Services: []contour_v1.Service{
							{
								Name: "echo-slash-prefix",
								Port: 80,
							},
						},
						Conditions: []contour_v1.MatchCondition{
							{
								Prefix: "/path/prefix/",
							},
						},
					},
					{
						Services: []contour_v1.Service{
							{
								Name: "echo-slash-noprefix",
								Port: 80,
							},
						},
						Conditions: []contour_v1.MatchCondition{
							{
								Prefix: "/path/prefix",
							},
						},
					},
					{
						Services: []contour_v1.Service{
							{
								Name: "echo-slash-default",
								Port: 80,
							},
						},
					},
				},
			},
		}
		require.True(f.T(), f.CreateHTTPProxyAndWaitFor(p, e2e.HTTPProxyValid))

		cases := map[string]string{
			"/":                "echo-slash-default",
			"/foo":             "echo-slash-default",
			"/path/prefix":     "echo-slash-noprefix",
			"/path/prefixfoo":  "echo-slash-noprefix",
			"/path/prefix/":    "echo-slash-prefix",
			"/path/prefix/foo": "echo-slash-prefix",
		}

		for path, expectedService := range cases {
			t.Logf("Querying %q, expecting service %q", path, expectedService)

			res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
				Host:      p.Spec.VirtualHost.Fqdn,
				Path:      path,
				Condition: e2e.HasStatusCode(200),
			})
			if !assert.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode) {
				continue
			}

			body := f.GetEchoResponseBody(res.Body)
			assert.Equal(t, namespace, body.Namespace)
			assert.Equal(t, expectedService, body.Service)
		}
	})
}
