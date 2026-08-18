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
	"bytes"
	"net/http"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/test/e2e"
)

// requestBodySizeLimitBytes is the value the specs below expect to be
// configured as the listener level max request body size.
const requestBodySizeLimitBytes = 1024

// testRequestBodySizeLimit verifies the Envoy body size limit filter that is
// configured globally by the listener level maxRequestBodyBytes setting.
//
// The filter is added to both the insecure filter chain and the per virtual
// host HTTPS filter chains, so both are exercised here.
//
// If limitConfigured is false, no limit is expected to be in effect and
// requests of any size should be proxied to the backend.
func testRequestBodySizeLimit(limitConfigured bool) e2e.NamespacedTestBody {
	testName := "requests are proxied regardless of their body size"
	fqdn := "nolimit.requestbodysize.projectcontour.io"

	if limitConfigured {
		testName = "requests with a body larger than the configured limit are rejected"
		fqdn = "limit.requestbodysize.projectcontour.io"
	}

	return func(namespace string) {
		Specify(testName, func() {
			t := f.T()

			f.Fixtures.Echo.Deploy(namespace, "echo")
			f.Certs.CreateSelfSignedCert(namespace, "echo-cert", fqdn)

			p := &contour_v1.HTTPProxy{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: namespace,
					Name:      "echo",
				},
				Spec: contour_v1.HTTPProxySpec{
					VirtualHost: &contour_v1.VirtualHost{
						Fqdn: fqdn,
						TLS: &contour_v1.TLS{
							SecretName: "echo-cert",
						},
					},
					Routes: []contour_v1.Route{
						{
							Services: []contour_v1.Service{
								{
									Name: "echo",
									Port: 80,
								},
							},
							// The body size limit filter is added to both the
							// insecure filter chain and the per virtual host
							// HTTPS filter chains, so serve the route over
							// plain HTTP as well instead of redirecting to
							// HTTPS, to be able to exercise both.
							PermitInsecure: true,
						},
					},
				},
			}
			require.True(t, f.CreateHTTPProxyAndWaitFor(p, e2e.HTTPProxyValid))

			// The requests carrying a body below cannot be retried, since
			// the request body can only be read once. Wait until both
			// listeners serve the route before making them.
			//
			// Redirects are not followed, so that a missing permitInsecure
			// on the route above surfaces as an unexpected 301 rather than
			// as a failure to resolve the virtual host name.
			res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
				Host:       fqdn,
				ClientOpts: []func(*http.Client){e2e.OptDontFollowRedirects},
				Condition:  e2e.HasStatusCode(200),
			})
			require.NotNil(t, res, "request never succeeded")
			require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)

			res, ok = f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
				Host:      fqdn,
				Condition: e2e.HasStatusCode(200),
			})
			require.NotNil(t, res, "request never succeeded")
			require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)

			// A request with a body over the limit is rejected with
			// 413 (Payload Too Large) only if a limit is configured.
			// Note that the filter rejects bodies that exceed the limit,
			// so a body of exactly the limit is still proxied.
			overLimitStatusCode := 200
			if limitConfigured {
				overLimitStatusCode = 413
			}

			// The body sizes are deliberately kept small so that the
			// request body always fits into the socket buffers. Envoy
			// rejects the request based on the Content-Length header,
			// i.e. before the body is sent, and a larger body would
			// make the client fail writing it instead of receiving the
			// 413 response.
			cases := []struct {
				description        string
				bodySize           int
				expectedStatusCode int
			}{
				{
					description:        "body under the limit",
					bodySize:           requestBodySizeLimitBytes / 2,
					expectedStatusCode: 200,
				},
				{
					description:        "body at the limit",
					bodySize:           requestBodySizeLimitBytes,
					expectedStatusCode: 200,
				},
				{
					description:        "body over the limit",
					bodySize:           requestBodySizeLimitBytes * 2,
					expectedStatusCode: overLimitStatusCode,
				},
			}

			for _, tc := range cases {
				res, err := f.HTTP.Request(&e2e.HTTPRequestOpts{
					Host:        fqdn,
					Body:        requestBody(tc.bodySize),
					RequestOpts: []func(*http.Request){optPostMethod},
					ClientOpts:  []func(*http.Client){e2e.OptDontFollowRedirects},
				})
				require.NoError(t, err)
				assert.Equalf(t, tc.expectedStatusCode, res.StatusCode,
					"unexpected status code for HTTP request with %s (%d bytes)", tc.description, tc.bodySize)

				res, err = f.HTTP.SecureRequest(&e2e.HTTPSRequestOpts{
					Host:        fqdn,
					Body:        requestBody(tc.bodySize),
					RequestOpts: []func(*http.Request){optPostMethod},
				})
				require.NoError(t, err)
				assert.Equalf(t, tc.expectedStatusCode, res.StatusCode,
					"unexpected status code for HTTPS request with %s (%d bytes)", tc.description, tc.bodySize)
			}
		})
	}
}

// requestBody returns a request body of the given size. Using a
// *bytes.Reader ensures the request is sent with a Content-Length header.
func requestBody(size int) *bytes.Reader {
	return bytes.NewReader([]byte(strings.Repeat("x", size)))
}

func optPostMethod(r *http.Request) {
	r.Method = http.MethodPost
}
