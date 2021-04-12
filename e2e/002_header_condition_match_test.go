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

// +build e2e

package e2e

import (
	"net/http"
	"testing"

	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestHeaderConditionMatch(t *testing.T) {
	t.Parallel()

	var (
		fx        = NewFramework(t)
		namespace = "002-header-condition-match"
	)

	fx.CreateNamespace(namespace)
	defer fx.DeleteNamespace(namespace)

	fx.CreateEchoWorkload(namespace, "echo-header-present")
	fx.CreateEchoWorkload(namespace, "echo-header-contains")
	fx.CreateEchoWorkload(namespace, "echo-header-notcontains")
	fx.CreateEchoWorkload(namespace, "echo-header-exact")
	fx.CreateEchoWorkload(namespace, "echo-header-notexact")

	p := &contourv1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "header-conditions",
		},
		Spec: contourv1.HTTPProxySpec{
			VirtualHost: &contourv1.VirtualHost{
				Fqdn: "headerconditions.projectcontour.io",
			},
			Routes: []contourv1.Route{
				{
					Services: []contourv1.Service{
						{
							Name: "echo-header-present",
							Port: 80,
						},
					},
					Conditions: []contourv1.MatchCondition{
						{
							Header: &contourv1.HeaderMatchCondition{
								Name:    "Target-Present",
								Present: true,
							},
						},
					},
				},
				{
					Services: []contourv1.Service{
						{
							Name: "echo-header-contains",
							Port: 80,
						},
					},
					Conditions: []contourv1.MatchCondition{
						{
							Header: &contourv1.HeaderMatchCondition{
								Name:     "Target-Contains",
								Contains: "ContainsValue",
							},
						},
					},
				},
				{
					Services: []contourv1.Service{
						{
							Name: "echo-header-notcontains",
							Port: 80,
						},
					},
					Conditions: []contourv1.MatchCondition{
						{
							Header: &contourv1.HeaderMatchCondition{
								Name:        "Target-NotContains",
								NotContains: "ContainsValue",
							},
						},
					},
				},
				{
					Services: []contourv1.Service{
						{
							Name: "echo-header-exact",
							Port: 80,
						},
					},
					Conditions: []contourv1.MatchCondition{
						{
							Header: &contourv1.HeaderMatchCondition{
								Name:  "Target-Exact",
								Exact: "ExactValue",
							},
						},
					},
				},
				{
					Services: []contourv1.Service{
						{
							Name: "echo-header-notexact",
							Port: 80,
						},
					},
					Conditions: []contourv1.MatchCondition{
						{
							Header: &contourv1.HeaderMatchCondition{
								Name:     "Target-NotExact",
								NotExact: "ExactValue",
							},
						},
					},
				},
			},
		},
	}
	fx.CreateHTTPProxyAndWaitFor(p, HTTPProxyValid)

	type scenario struct {
		headers        map[string]string
		expectResponse int
		expectService  string
	}

	cases := []scenario{
		{
			headers:        map[string]string{"Target-Present": "random"},
			expectResponse: 200,
			expectService:  "echo-header-present",
		},
		{
			headers:        map[string]string{"Target-Contains": "random"},
			expectResponse: 404,
		},
		{
			headers:        map[string]string{"Target-Contains": "ContainsValue"},
			expectResponse: 200,
			expectService:  "echo-header-contains",
		},
		{
			headers:        map[string]string{"Target-Contains": "xxx ContainsValue xxx"},
			expectResponse: 200,
			expectService:  "echo-header-contains",
		},
		{
			headers:        map[string]string{"Target-NotContains": "ContainsValue"},
			expectResponse: 404,
		},
		{
			headers:        map[string]string{"Target-NotContains": "xxx ContainsValue xxx"},
			expectResponse: 404,
		},
		{
			headers:        map[string]string{"Target-NotContains": "random"},
			expectResponse: 200,
			expectService:  "echo-header-notcontains",
		},
		{
			headers:        map[string]string{"Target-Exact": "random"},
			expectResponse: 404,
		},
		{
			headers:        map[string]string{"Target-Exact": "NotExactValue"},
			expectResponse: 404,
		},
		{
			headers:        map[string]string{"Target-Exact": "ExactValue"},
			expectResponse: 200,
			expectService:  "echo-header-exact",
		},
		{
			headers:        map[string]string{"Target-NotExact": "random"},
			expectResponse: 200,
			expectService:  "echo-header-notexact",
		},
		{
			headers:        map[string]string{"Target-NotExact": "NotExactValue"},
			expectResponse: 200,
			expectService:  "echo-header-notexact",
		},
		{
			headers:        map[string]string{"Target-NotExact": "ExactValue"},
			expectResponse: 404,
		},
	}

	for _, tc := range cases {
		setHeader := func(r *http.Request) {
			for k, v := range tc.headers {
				r.Header.Set(k, v)
			}
		}

		res, ok := fx.HTTPRequestUntil(HasStatusCode(tc.expectResponse), "/header-condition-match", p.Spec.VirtualHost.Fqdn, setHeader)
		if !assert.Truef(t, ok, "did not get %d response", tc.expectResponse) {
			continue
		}
		if res.StatusCode != 200 {
			// If we expected something other than a 200,
			// then we don't need to check the body.
			continue
		}

		body := fx.GetEchoResponseBody(res.Body)
		assert.Equal(t, namespace, body.Namespace)
		assert.Equal(t, tc.expectService, body.Service)
	}
}
