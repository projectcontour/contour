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
	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/test/e2e"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func testOmitRouteSorting(namespace string) {
	Specify("Path matching works", func() {
		var (
			t                = f.T()
			serviceNamespace = namespace
		)
		f.Fixtures.Echo.Deploy(serviceNamespace, "echo-1")
		f.Fixtures.Echo.Deploy(serviceNamespace, "echo-2")
		f.Fixtures.Echo.Deploy(serviceNamespace, "echo-3")
		serviceProxy := &contour_api_v1.HTTPProxy{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: serviceNamespace,
				Name:      "no-sorting",
			},
			Spec: contour_api_v1.HTTPProxySpec{
				VirtualHost: &contour_api_v1.VirtualHost{
					Fqdn: "sorting.projectcontour.io",
				},
				Routes: []contour_api_v1.Route{
					{
						Services: []contour_api_v1.Service{
							{
								Name: "echo-1",
								Port: 80,
							},
						},
						Conditions: []contour_api_v1.MatchCondition{
							{
								Header: &contour_api_v1.HeaderMatchCondition{
									Name:  "x-experiment",
									Exact: "bar",
								},
							},
						},
					},
					{
						Services: []contour_api_v1.Service{
							{
								Name: "echo-2",
								Port: 80,
							},
						},
						Conditions: []contour_api_v1.MatchCondition{
							{
								Exact: "/bar",
							},
						},
					},
					{
						Services: []contour_api_v1.Service{
							{
								Name: "echo-3",
								Port: 80,
							},
						},
						Conditions: []contour_api_v1.MatchCondition{
							{
								Prefix: "/",
							},
						},
					},
				},
			},
		}
		f.CreateHTTPProxyAndWaitFor(serviceProxy, e2e.HTTPProxyValid)
		tests := []struct {
			Path    string
			Headers map[string]string
			wantSvc string
		}{
			{
				Path:    "/bar",
				Headers: map[string]string{"x-experiment": "bar"},
				wantSvc: "echo-1",
			},
			{
				Path:    "/bar",
				wantSvc: "echo-2",
			},
			{
				Path:    "/",
				wantSvc: "echo-3",
			},
		}
		for _, tt := range tests {
			t.Logf("Querying path: %q, expecting service to be called: %q", tt.Path, tt.wantSvc)

			res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
				Host:      serviceProxy.Spec.VirtualHost.Fqdn,
				Path:      tt.Path,
				Condition: e2e.HasStatusCode(200),
				RequestOpts: []func(*http.Request){
					e2e.OptSetHeaders(tt.Headers),
				},
			})
			if !assert.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode) {
				continue
			}

			assert.Equal(t, tt.wantSvc, f.GetEchoResponseBody(res.Body).Service)
		}
	})
}
