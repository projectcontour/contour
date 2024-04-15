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

func testPathPrefixRewrite(namespace string) {
	Specify("path prefix rewrite works", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "no-rewrite")
		f.Fixtures.Echo.Deploy(namespace, "prefix-rewrite")
		f.Fixtures.Echo.Deploy(namespace, "prefix-rewrite-to-root")

		p := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "prefix-rewrite",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "prefixrewrite.projectcontour.io",
				},
				Routes: []contour_v1.Route{
					{
						Services: []contour_v1.Service{
							{
								Name: "no-rewrite",
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
								Name: "prefix-rewrite",
								Port: 80,
							},
						},
						Conditions: []contour_v1.MatchCondition{
							{
								Prefix: "/someprefix1",
							},
						},
						PathRewritePolicy: &contour_v1.PathRewritePolicy{
							ReplacePrefix: []contour_v1.ReplacePrefix{
								{
									Prefix:      "/someprefix1",
									Replacement: "/someotherprefix",
								},
							},
						},
					},
					{
						Services: []contour_v1.Service{
							{
								Name: "prefix-rewrite-to-root",
								Port: 80,
							},
						},
						Conditions: []contour_v1.MatchCondition{
							{
								Prefix: "/someprefix2",
							},
						},
						PathRewritePolicy: &contour_v1.PathRewritePolicy{
							ReplacePrefix: []contour_v1.ReplacePrefix{
								{
									Prefix:      "/someprefix2",
									Replacement: "/",
								},
							},
						},
					},
				},
			},
		}
		require.True(f.T(), f.CreateHTTPProxyAndWaitFor(p, e2e.HTTPProxyValid))

		cases := []struct {
			path            string
			expectedService string
			expectedPath    string
		}{
			{path: "/", expectedService: "no-rewrite", expectedPath: "/"},
			{path: "/foo", expectedService: "no-rewrite", expectedPath: "/foo"},
			{path: "/someprefix1", expectedService: "prefix-rewrite", expectedPath: "/someotherprefix"},
			{path: "/someprefix1foobar", expectedService: "prefix-rewrite", expectedPath: "/someotherprefixfoobar"},
			{path: "/someprefix1/segment", expectedService: "prefix-rewrite", expectedPath: "/someotherprefix/segment"},
			{path: "/someprefix2", expectedService: "prefix-rewrite-to-root", expectedPath: "/"},
			{path: "/someprefix2foobar", expectedService: "prefix-rewrite-to-root", expectedPath: "/foobar"},
			{path: "/someprefix2/segment", expectedService: "prefix-rewrite-to-root", expectedPath: "/segment"},
		}

		for _, tc := range cases {
			t.Logf("Querying %q, expecting service %q and path %q", tc.path, tc.expectedService, tc.expectedPath)

			res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
				Host:      p.Spec.VirtualHost.Fqdn,
				Path:      tc.path,
				Condition: e2e.HasStatusCode(200),
			})
			if !assert.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode) {
				continue
			}

			body := f.GetEchoResponseBody(res.Body)
			assert.Equal(t, tc.expectedService, body.Service)
			assert.Equal(t, tc.expectedPath, body.Path)
		}
	})
}
