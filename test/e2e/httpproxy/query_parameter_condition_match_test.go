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
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/test/e2e"
)

func testQueryParameterConditionMatch(namespace string) {
	Specify("query parameter match routing works", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "echo-query-parameter-exact")
		f.Fixtures.Echo.Deploy(namespace, "echo-query-parameter-exact-ignorecase")
		f.Fixtures.Echo.Deploy(namespace, "echo-query-parameter-prefix")
		f.Fixtures.Echo.Deploy(namespace, "echo-query-parameter-prefix-ignorecase")
		f.Fixtures.Echo.Deploy(namespace, "echo-query-parameter-suffix")
		f.Fixtures.Echo.Deploy(namespace, "echo-query-parameter-suffix-ignorecase")
		f.Fixtures.Echo.Deploy(namespace, "echo-query-parameter-regex")
		f.Fixtures.Echo.Deploy(namespace, "echo-query-parameter-present")
		f.Fixtures.Echo.Deploy(namespace, "echo-query-parameter-contains")
		f.Fixtures.Echo.Deploy(namespace, "echo-query-parameter-contains-ignorecase")

		p := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "query-parameter-conditions",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "queryparam.projectcontour.io",
				},
				Routes: []contour_v1.Route{
					{
						Services: []contour_v1.Service{
							{
								Name: "echo-query-parameter-exact",
								Port: 80,
							},
						},
						Conditions: []contour_v1.MatchCondition{
							{
								QueryParameter: &contour_v1.QueryParameterMatchCondition{
									Name:  "targetExact",
									Exact: "ExactValue",
								},
							},
						},
					},
					{
						Services: []contour_v1.Service{
							{
								Name: "echo-query-parameter-exact-ignorecase",
								Port: 80,
							},
						},
						Conditions: []contour_v1.MatchCondition{
							{
								QueryParameter: &contour_v1.QueryParameterMatchCondition{
									Name:       "targetExactIgnoreCase",
									Exact:      "exactvalueIgnorecase",
									IgnoreCase: true,
								},
							},
						},
					},
					{
						Services: []contour_v1.Service{
							{
								Name: "echo-query-parameter-prefix",
								Port: 80,
							},
						},
						Conditions: []contour_v1.MatchCondition{
							{
								QueryParameter: &contour_v1.QueryParameterMatchCondition{
									Name:   "targetPrefix",
									Prefix: "Prefix",
								},
							},
						},
					},
					{
						Services: []contour_v1.Service{
							{
								Name: "echo-query-parameter-prefix-ignorecase",
								Port: 80,
							},
						},
						Conditions: []contour_v1.MatchCondition{
							{
								QueryParameter: &contour_v1.QueryParameterMatchCondition{
									Name:       "targetPrefixIgnoreCase",
									Prefix:     "prefixval",
									IgnoreCase: true,
								},
							},
						},
					},
					{
						Services: []contour_v1.Service{
							{
								Name: "echo-query-parameter-suffix",
								Port: 80,
							},
						},
						Conditions: []contour_v1.MatchCondition{
							{
								QueryParameter: &contour_v1.QueryParameterMatchCondition{
									Name:   "targetSuffix",
									Suffix: "ffixValue",
								},
							},
						},
					},
					{
						Services: []contour_v1.Service{
							{
								Name: "echo-query-parameter-suffix-ignorecase",
								Port: 80,
							},
						},
						Conditions: []contour_v1.MatchCondition{
							{
								QueryParameter: &contour_v1.QueryParameterMatchCondition{
									Name:       "targetSuffixIgnoreCase",
									Suffix:     "ffixvalueignorecase",
									IgnoreCase: true,
								},
							},
						},
					},
					{
						Services: []contour_v1.Service{
							{
								Name: "echo-query-parameter-regex",
								Port: 80,
							},
						},
						Conditions: []contour_v1.MatchCondition{
							{
								QueryParameter: &contour_v1.QueryParameterMatchCondition{
									Name:  "targetRegex",
									Regex: "^RegexV.*",
								},
							},
						},
					},
					{
						Services: []contour_v1.Service{
							{
								Name: "echo-query-parameter-contains",
								Port: 80,
							},
						},
						Conditions: []contour_v1.MatchCondition{
							{
								QueryParameter: &contour_v1.QueryParameterMatchCondition{
									Name:     "targetContains",
									Contains: "nsVal",
								},
							},
						},
					},
					{
						Services: []contour_v1.Service{
							{
								Name: "echo-query-parameter-contains-ignorecase",
								Port: 80,
							},
						},
						Conditions: []contour_v1.MatchCondition{
							{
								QueryParameter: &contour_v1.QueryParameterMatchCondition{
									Name:       "targetContainsIgnoreCase",
									Contains:   "svalueIgnorec",
									IgnoreCase: true,
								},
							},
						},
					},
					{
						Services: []contour_v1.Service{
							{
								Name: "echo-query-parameter-present",
								Port: 80,
							},
						},
						Conditions: []contour_v1.MatchCondition{
							{
								QueryParameter: &contour_v1.QueryParameterMatchCondition{
									Name:    "targetPresent",
									Present: true,
								},
							},
						},
					},
				},
			},
		}
		require.True(f.T(), f.CreateHTTPProxyAndWaitFor(p, e2e.HTTPProxyValid))

		type scenario struct {
			queryParams    map[string]string
			expectResponse int
			expectService  string
		}

		cases := []scenario{
			{
				queryParams:    map[string]string{"targetExact": "random"},
				expectResponse: 404,
			},
			{
				queryParams:    map[string]string{"targetExact": "ExactValue"},
				expectResponse: 200,
				expectService:  "echo-query-parameter-exact",
			},
			{
				queryParams:    map[string]string{"targetExact": "exactvalue"},
				expectResponse: 404,
				expectService:  "echo-query-parameter-exact",
			},
			{
				queryParams:    map[string]string{"targetExactIgnoreCase": "ExactValueIgnoreCase"},
				expectResponse: 200,
				expectService:  "echo-query-parameter-exact-ignorecase",
			},
			{
				queryParams:    map[string]string{"targetPrefix": "random"},
				expectResponse: 404,
			},
			{
				queryParams:    map[string]string{"targetPrefix": "PrefixValue"},
				expectResponse: 200,
				expectService:  "echo-query-parameter-prefix",
			},
			{
				queryParams:    map[string]string{"targetPrefix": "prefixvalue"},
				expectResponse: 404,
				expectService:  "echo-query-parameter-prefix",
			},
			{
				queryParams:    map[string]string{"targetPrefixIgnoreCase": "PrefixValueIgnoreCase"},
				expectResponse: 200,
				expectService:  "echo-query-parameter-prefix-ignorecase",
			},
			{
				queryParams:    map[string]string{"targetSuffix": "random"},
				expectResponse: 404,
			},
			{
				queryParams:    map[string]string{"targetSuffix": "SuffixValue"},
				expectResponse: 200,
				expectService:  "echo-query-parameter-suffix",
			},
			{
				queryParams:    map[string]string{"targetSuffix": "suffixvalue"},
				expectResponse: 404,
				expectService:  "echo-query-parameter-suffix",
			},
			{
				queryParams:    map[string]string{"targetSuffixIgnoreCase": "SuffixValueIgnoreCase"},
				expectResponse: 200,
				expectService:  "echo-query-parameter-suffix-ignorecase",
			},
			{
				queryParams:    map[string]string{"targetRegex": "random"},
				expectResponse: 404,
			},
			{
				queryParams:    map[string]string{"targetRegex": "RegexValue"},
				expectResponse: 200,
				expectService:  "echo-query-parameter-regex",
			},
			{
				queryParams:    map[string]string{"targetContains": "random"},
				expectResponse: 404,
			},
			{
				queryParams:    map[string]string{"targetContains": "ContainsValue"},
				expectResponse: 200,
				expectService:  "echo-query-parameter-contains",
			},
			{
				queryParams:    map[string]string{"targetContains": "xxx ContainsValue xxx"},
				expectResponse: 200,
				expectService:  "echo-query-parameter-contains",
			},
			{
				queryParams:    map[string]string{"targetContains": "containsvalueignorecase"},
				expectResponse: 404,
				expectService:  "echo-query-parameter-contains",
			},
			{
				queryParams:    map[string]string{"targetContainsIgnoreCase": "ContainsValueIgnoreCase"},
				expectResponse: 200,
				expectService:  "echo-query-parameter-contains-ignorecase",
			},
			{
				queryParams:    map[string]string{"targetPresent": "random"},
				expectResponse: 200,
				expectService:  "echo-query-parameter-present",
			},
		}

		for _, tc := range cases {
			res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
				Host: p.Spec.VirtualHost.Fqdn,
				RequestOpts: []func(*http.Request){
					e2e.OptSetQueryParams(tc.queryParams),
				},
				Condition: e2e.HasStatusCode(tc.expectResponse),
			})
			if !assert.Truef(t, ok, "expected %d response code, got %d with query parameters %v", tc.expectResponse, res.StatusCode, tc.queryParams) {
				continue
			}
			if res.StatusCode != 200 {
				// If we expected something other than a 200,
				// then we don't need to check the body.
				continue
			}

			body := f.GetEchoResponseBody(res.Body)
			assert.Equal(t, namespace, body.Namespace)
			assert.Equal(t, tc.expectService, body.Service)
		}
	})
}

func testQueryParameterConditionMultiple(namespace string) {
	Specify("first of multiple query params in a request is used for routing", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "echo-1")
		f.Fixtures.Echo.Deploy(namespace, "echo-2")

		p := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "query-parameter-multiple",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "queryparam-multiple.projectcontour.io",
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
								QueryParameter: &contour_v1.QueryParameterMatchCondition{
									Name:  "animal",
									Exact: "whale",
								},
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
								QueryParameter: &contour_v1.QueryParameterMatchCondition{
									Name:  "animal",
									Exact: "dolphin",
								},
							},
						},
					},
				},
			},
		}
		require.True(f.T(), f.CreateHTTPProxyAndWaitFor(p, e2e.HTTPProxyValid))

		type scenario struct {
			path           string
			expectResponse int
			expectService  string
		}

		cases := []scenario{
			{
				path:           "/?animal=whale&animal=dolphin",
				expectResponse: 200,
				expectService:  "echo-1",
			},
			{
				path:           "/?animal=dolphin&animal=whale",
				expectResponse: 200,
				expectService:  "echo-2",
			},
		}

		for _, tc := range cases {
			res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
				Host:      p.Spec.VirtualHost.Fqdn,
				Path:      tc.path,
				Condition: e2e.HasStatusCode(tc.expectResponse),
			})
			if !assert.Truef(t, ok, "expected %d response code, got %d with path %v", tc.expectResponse, res.StatusCode, tc.path) {
				continue
			}
			if res.StatusCode != 200 {
				// If we expected something other than a 200,
				// then we don't need to check the body.
				continue
			}

			body := f.GetEchoResponseBody(res.Body)
			assert.Equal(t, namespace, body.Namespace)
			assert.Equal(t, tc.expectService, body.Service)
		}
	})
}
