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
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/require"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/test/e2e"
)

func testForwardProtoConfig(namespace string) {
	Specify("X-Forwarded-Proto follows the PROXY protocol destination port", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "echo")

		// A plain virtual host: the backend sees the derived header.
		echoProxy := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "forward-proto",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "forwardproto.projectcontour.io",
				},
				Routes: []contour_v1.Route{
					{
						Services: []contour_v1.Service{
							{
								Name: "echo",
								Port: 80,
							},
						},
					},
				},
			},
		}
		require.True(t, f.CreateHTTPProxyAndWaitFor(echoProxy, e2e.HTTPProxyValid))

		// A virtual host that redirects plaintext requests to HTTPS by matching
		// the derived header. Without forward proto config every request would
		// carry X-Forwarded-Proto: http and be redirected, including the ones
		// that already arrived over HTTPS at the load balancer.
		redirectProxy := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "forward-proto-redirect",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "forwardproto-redirect.projectcontour.io",
				},
				Routes: []contour_v1.Route{
					{
						Conditions: []contour_v1.MatchCondition{
							{
								Header: &contour_v1.HeaderMatchCondition{
									Name:  "X-Forwarded-Proto",
									Exact: "http",
								},
							},
						},
						RequestRedirectPolicy: &contour_v1.HTTPRequestRedirectPolicy{
							Scheme:     ptr.To("https"),
							StatusCode: ptr.To(contour_v1.RedirectResponseCode(301)),
						},
					},
					{
						Services: []contour_v1.Service{
							{
								Name: "echo",
								Port: 80,
							},
						},
					},
				},
			},
		}
		require.True(t, f.CreateHTTPProxyAndWaitFor(redirectProxy, e2e.HTTPProxyValid))

		envoyURL, err := url.Parse(f.HTTP.HTTPURLBase)
		require.NoError(t, err)

		// The connection to Envoy is plaintext in every case; only the
		// destination port claimed in the PROXY protocol header differs.
		cases := []struct {
			name            string
			fqdn            string
			destinationPort int
			wantStatus      int
			wantProto       string // X-Forwarded-Proto seen by the backend, when served
			wantLocation    string // Location header, when redirected
		}{
			{"https destination port", echoProxy.Spec.VirtualHost.Fqdn, 443, http.StatusOK, "https", ""},
			{"http destination port", echoProxy.Spec.VirtualHost.Fqdn, 80, http.StatusOK, "http", ""},
			// In neither list: Envoy falls back to the plaintext connection.
			{"unlisted destination port", echoProxy.Spec.VirtualHost.Fqdn, 8080, http.StatusOK, "http", ""},
			{"https destination port is served, not redirected", redirectProxy.Spec.VirtualHost.Fqdn, 443, http.StatusOK, "https", ""},
			{"http destination port is redirected to https", redirectProxy.Spec.VirtualHost.Fqdn, 80, http.StatusMovedPermanently, "", "https://" + redirectProxy.Spec.VirtualHost.Fqdn + "/"},
		}
		for _, tc := range cases {
			require.Eventuallyf(t, func() bool {
				res, err := proxiedRequest(envoyURL.Host, tc.destinationPort, tc.fqdn)
				if err != nil || res.status != tc.wantStatus {
					t.Logf("%s: status %d, error %v", tc.name, res.status, err)
					return false
				}
				if tc.wantProto != "" {
					if got := res.echoHeaders.Get("X-Forwarded-Proto"); got != tc.wantProto {
						t.Logf("%s: got X-Forwarded-Proto %q, want %q", tc.name, got, tc.wantProto)
						return false
					}
				}
				if tc.wantLocation != "" {
					if got := res.headers.Get("Location"); got != tc.wantLocation {
						t.Logf("%s: got Location %q, want %q", tc.name, got, tc.wantLocation)
						return false
					}
				}
				return true
			}, f.RetryTimeout, f.RetryInterval, "%s: did not get status %d with X-Forwarded-Proto %q / Location %q", tc.name, tc.wantStatus, tc.wantProto, tc.wantLocation)
		}
	})
}

// proxiedResponse is what a PROXY protocol request to Envoy came back with:
// the response status and headers, and the request headers the echo server
// received when the request was served.
type proxiedResponse struct {
	status      int
	headers     http.Header
	echoHeaders http.Header
}

// proxiedRequest opens a plaintext connection to Envoy, sends a PROXY protocol
// v1 header that claims destinationPort as the original destination port,
// then a GET for host.
func proxiedRequest(envoyAddr string, destinationPort int, host string) (proxiedResponse, error) {
	conn, err := net.DialTimeout("tcp", envoyAddr, 5*time.Second)
	if err != nil {
		return proxiedResponse{}, err
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return proxiedResponse{}, err
	}

	if _, err := fmt.Fprintf(conn, "PROXY TCP4 192.0.2.1 192.0.2.2 40000 %d\r\nGET / HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", destinationPort, host); err != nil {
		return proxiedResponse{}, err
	}

	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		return proxiedResponse{}, err
	}
	defer resp.Body.Close()

	res := proxiedResponse{status: resp.StatusCode, headers: resp.Header}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return res, err
	}
	if resp.StatusCode == http.StatusOK {
		res.echoHeaders = f.GetEchoResponseBody(body).RequestHeaders
	}

	return res, nil
}
