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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/test/e2e"
)

func testResponseOverridePolicy(namespace string) {
	Specify("response overrides can be configured", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "echo")

		// Create a simple HTTPProxy with ResponseOverridePolicy
		proxy := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "response-override",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "response-override.projectcontour.io",
				},
				Routes: []contour_v1.Route{
					{
						Conditions: []contour_v1.MatchCondition{
							{
								Prefix: "/",
							},
						},
						Services: []contour_v1.Service{
							{
								Name: "echo",
								Port: 80,
							},
						},
						ResponseOverridePolicy: []contour_v1.HTTPResponseOverridePolicy{
							{
								// Match 404 responses
								Match: contour_v1.ResponseOverrideMatch{
									StatusCodes: []contour_v1.StatusCodeMatch{
										{
											Type:  "Value",
											Value: 404,
										},
									},
								},
								Response: contour_v1.ResponseOverrideResponse{
									ContentType: "text/html",
									Body: contour_v1.ResponseBodyConfig{
										Type:   "Inline",
										Inline: "<html><body><h1>Custom 404 Page</h1></body></html>",
									},
								},
							},
							{
								// Match 5xx responses
								Match: contour_v1.ResponseOverrideMatch{
									StatusCodes: []contour_v1.StatusCodeMatch{
										{
											Type: "Range",
											Range: &contour_v1.StatusCodeRange{
												Start: 500,
												End:   599,
											},
										},
									},
								},
								Response: contour_v1.ResponseOverrideResponse{
									ContentType: "application/json",
									Body: contour_v1.ResponseBodyConfig{
										Type:   "Inline",
										Inline: `{"error":"Server Error","code":500}`,
									},
								},
							},
						},
					},
				},
			},
		}

		require.True(t, f.CreateHTTPProxyAndWaitFor(proxy, e2e.HTTPProxyValid))

		// Test that a request to non-existent path gets the custom 404 response
		res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      proxy.Spec.VirtualHost.Fqdn,
			Path:      "/non-existent",
			Condition: e2e.HasStatusCode(404),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 404 response code, got %d", res.StatusCode)
		assert.Equal(t, "<html><body><h1>Custom 404 Page</h1></body></html>", string(res.Body))
		assert.Equal(t, "text/html", res.Headers.Get("Content-Type"))

		// Update the proxy to include a path that will return 500 error
		require.NoError(t, retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			err := f.Client.Get(context.TODO(), client.ObjectKeyFromObject(proxy), proxy)
			if err != nil {
				return err
			}

			proxy.Spec.Routes = append(proxy.Spec.Routes, contour_v1.Route{
				Conditions: []contour_v1.MatchCondition{
					{
						Prefix: "/error",
					},
				},
				DirectResponsePolicy: &contour_v1.HTTPDirectResponsePolicy{
					StatusCode: 500,
				},
			})

			return f.Client.Update(context.TODO(), proxy)
		}))

		// Wait for the update to be processed
		require.True(t, f.CreateHTTPProxyAndWaitFor(proxy, e2e.HTTPProxyValid))

		// Test that a request to /error gets the custom 500 response
		res, ok = f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      proxy.Spec.VirtualHost.Fqdn,
			Path:      "/error",
			Condition: e2e.HasStatusCode(500),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 500 response code, got %d", res.StatusCode)
		assert.Equal(t, `{"error":"Server Error","code":500}`, string(res.Body))
		assert.Equal(t, "application/json", res.Headers.Get("Content-Type"))
	})
}

func init() {
	f.NamespacedTest("httpproxy-response-override-policy", testResponseOverridePolicy)
}
