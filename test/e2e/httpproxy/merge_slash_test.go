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
	"github.com/stretchr/testify/require"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/test/e2e"
)

func testDisableMergeSlashes(disableMergeSlashes bool) e2e.NamespacedTestBody {
	var testName string
	if disableMergeSlashes {
		testName = "when disable merge slashes is true, consecutive slashes in requests are not merged"
	} else {
		testName = "when disable merge slashes is false, consecutive slashes in requests are merged"
	}
	return func(namespace string) {
		Specify(testName, func() {
			t := f.T()

			f.Fixtures.Echo.Deploy(namespace, "echo-1")
			f.Fixtures.Echo.Deploy(namespace, "echo-2")

			var fqdn string
			if disableMergeSlashes {
				fqdn = "disable.mergeslashes.projectcontour.io"
			} else {
				fqdn = "enable.mergeslashes.projectcontour.io"
			}

			p := &contour_v1.HTTPProxy{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: namespace,
					Name:      "echo",
				},
				Spec: contour_v1.HTTPProxySpec{
					VirtualHost: &contour_v1.VirtualHost{
						Fqdn: fqdn,
					},
					Routes: []contour_v1.Route{
						{
							Services: []contour_v1.Service{
								{
									Name: "echo-1",
									Port: 80,
								},
							},
							Conditions: []contour_v1.MatchCondition{
								{
									Prefix: "/foo",
								},
							},
						},
						{
							Services: []contour_v1.Service{
								{
									Name: "echo-2",
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
			require.True(f.T(), f.CreateHTTPProxyAndWaitFor(p, e2e.HTTPProxyValid))

			var testCases map[string]string
			if disableMergeSlashes {
				testCases = map[string]string{
					"/foo":  "echo-1",
					"//foo": "echo-2", // since the slashes aren't merged, this request won't match the first route, so will default to the second
				}
			} else {
				testCases = map[string]string{
					"/foo":  "echo-1",
					"//foo": "echo-1", // since the slashes *are* merged, this request will match the first route
				}
			}

			for path, svc := range testCases {
				res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
					Host: p.Spec.VirtualHost.Fqdn,
					Path: path,
					Condition: func(res *e2e.HTTPResponse) bool {
						if !e2e.HasStatusCode(200)(res) {
							t.Logf("Got response code %d", res.StatusCode)
							return false
						}

						responseBody := f.GetEchoResponseBody(res.Body)

						if responseBody.Service != svc {
							t.Logf("Got service %s", responseBody.Service)
							return false
						}

						return true
					},
				})
				require.NotNil(t, res, "request never succeeded")
				require.True(t, ok)
			}
		})
	}
}
