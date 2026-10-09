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

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/test/e2e"
)

func testForwardProtoConfig(namespace string) {
	Specify("X-Forwarded-Proto follows the PROXY protocol destination port", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "echo")

		p := &contour_v1.HTTPProxy{
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
		require.True(t, f.CreateHTTPProxyAndWaitFor(p, e2e.HTTPProxyValid))

		envoyURL, err := url.Parse(f.HTTP.HTTPURLBase)
		require.NoError(t, err)

		// The connection to Envoy is plaintext in every case; only the
		// destination port claimed in the PROXY protocol header differs.
		cases := map[int]string{
			443:  "https", // in https-destination-ports
			80:   "http",  // in http-destination-ports
			8080: "http",  // in neither: Envoy falls back to the plaintext connection
		}
		for destinationPort, want := range cases {
			require.Eventuallyf(t, func() bool {
				status, headers, err := proxiedRequest(envoyURL.Host, destinationPort, p.Spec.VirtualHost.Fqdn)
				if err != nil || status != http.StatusOK {
					t.Logf("destination port %d: status %d, error %v", destinationPort, status, err)
					return false
				}
				if got := headers.Get("X-Forwarded-Proto"); got != want {
					t.Logf("destination port %d: got X-Forwarded-Proto %q, want %q", destinationPort, got, want)
					return false
				}
				return true
			}, f.RetryTimeout, f.RetryInterval, "destination port %d did not yield X-Forwarded-Proto %q", destinationPort, want)
		}
	})
}

// proxiedRequest opens a plaintext connection to Envoy, sends a PROXY protocol
// v1 header that claims destinationPort as the original destination port,
// then a GET for host, and returns the response status and the request
// headers the echo server received.
func proxiedRequest(envoyAddr string, destinationPort int, host string) (int, http.Header, error) {
	conn, err := net.DialTimeout("tcp", envoyAddr, 5*time.Second)
	if err != nil {
		return 0, nil, err
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return 0, nil, err
	}

	if _, err := fmt.Fprintf(conn, "PROXY TCP4 192.0.2.1 192.0.2.2 40000 %d\r\nGET / HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", destinationPort, host); err != nil {
		return 0, nil, err
	}

	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, nil, nil
	}

	return resp.StatusCode, f.GetEchoResponseBody(body).RequestHeaders, nil
}
