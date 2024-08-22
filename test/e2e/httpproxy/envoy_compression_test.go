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
	"fmt"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/require"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/test/e2e"
)

func testEnvoyDisableCompression(namespace string, enabled bool) {
	testSpec := "responses compressed with default settings"
	if enabled {
		testSpec = "responses are plaintext when compression disabled"
	}
	FSpecify(testSpec, func() {
		resp := "minimum_text_to_enable_gzipminimum_text_to_enable_gzipminimum_text_to_enable_gzipminimum_text_to_enable_gzipminimum_text_to_enable_gzipminimum_text_to_enable_gzip"
		p := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      "direct-response",
				Namespace: namespace,
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: fmt.Sprintf("%s-fqdn.projectcontour.io", namespace),
				}, Routes: []contour_v1.Route{
					{
						Conditions: []contour_v1.MatchCondition{{
							Prefix: "/directresponse",
						}},
						DirectResponsePolicy: &contour_v1.HTTPDirectResponsePolicy{
							StatusCode: 200,
							Body:       resp,
						},
					},
				},
			},
		}
		require.True(f.T(), f.CreateHTTPProxyAndWaitFor(p, e2e.HTTPProxyValid))

		// Send HTTP request, we will check backend connection was over HTTPS.
		res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Path: "/directresponse",
			Host: p.Spec.VirtualHost.Fqdn,
			RequestOpts: []func(*http.Request){
				e2e.OptSetHeaders(map[string]string{
					"Accept-Encoding": "gzip, deflate",
				}),
			},
			Condition: e2e.HasStatusCode(200),
		})
		require.NotNil(f.T(), res, "request never succeeded")
		require.Truef(f.T(), ok, "expected 200 response code, got %d", res.StatusCode)
		fmt.Printf("response: %+v\n", res.Headers)
		if enabled {
			require.NotContains(f.T(), res.Headers["Content-Encoding"], "gzip", "expected plain text")
			return
		}
		require.Contains(f.T(), res.Headers["Content-Encoding"], "gzip", "expected plain text")
	})
}
