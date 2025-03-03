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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/test/e2e"
)

func testEnvoyDisableCompression(namespace, acceptEncoding, contentEncoding string, disabled bool) {
	testSpec := fmt.Sprintf("responses compressed with accept-encoding %s expecting content-encoding %s", acceptEncoding, contentEncoding)
	if disabled {
		testSpec = "responses are plaintext when compression disabled"
	}

	Specify(testSpec, func() {
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

		require.EventuallyWithT(f.T(), func(c *assert.CollectT) {
			res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
				Path: "/directresponse",
				Host: p.Spec.VirtualHost.Fqdn,
				RequestOpts: []func(*http.Request){
					e2e.OptSetHeaders(map[string]string{
						"Accept-Encoding": fmt.Sprintf("%s, deflate", acceptEncoding),
					}),
				},
				Condition: e2e.HasStatusCode(200),
			})
			assert.NotNil(c, res, "request never succeeded")
			assert.Truef(c, ok, "expected 200 response code, got %d", res.StatusCode)
			contentEncodingHeaderValue := res.Headers.Get("Content-Encoding")
			if disabled {
				assert.NotEqual(c, contentEncodingHeaderValue, contentEncoding, "expected plain text")
				return
			}
			assert.Equal(c, contentEncoding, contentEncodingHeaderValue, "expected plain text")
		}, f.RetryTimeout, f.RetryInterval)
	})
}
