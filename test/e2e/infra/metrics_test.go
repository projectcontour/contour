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

package infra

import (
	"crypto/tls"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/require"
)

func testMetrics() {
	Specify("requests to default metrics listener are served", func() {
		t := f.T()

		res, ok := f.HTTP.MetricsRequestUntil(&e2e.HTTPRequestOpts{
			Path:      "/stats",
			Condition: e2e.HasStatusCode(200),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)
	})
}

func testReady() {
	Specify("requests to default ready listener are served", func() {
		t := f.T()

		res, ok := f.HTTP.MetricsRequestUntil(&e2e.HTTPRequestOpts{
			Path:      "/ready",
			Condition: e2e.HasStatusCode(200),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)
	})
}

func testEnvoyMetricsOverHTTPS() {
	Specify("requests to metrics listener are served", func() {
		t := f.T()

		// Port-forward seems to be flaky. Following sequence happens:
		//
		// 1. Envoy becomes ready.
		// 2. Port-forward is started.
		// 3. HTTPS request is sent but the connection times out with errors
		//     "error creating error stream for port 18003 -> 8003: Timeout occurred",
		//     "error creating forwarding stream for port 18003 -> 8003: Timeout occurred"
		// 4. Meanwhile the metrics listener gets added.
		// 5. Sometimes (one out of ~1-50 runs) port-forward gets stuck and packets are not forwarded
		//    even after listener is up and connection attempts are still regularly retried.
		//
		// When the problem occurs, Wireshark does not show any traffic on the container side.
		// The problem could be e.g. undiscovered race condition with Kubernetes port-forward.
		//
		// Following workarounds seem to work:
		//
		// a) Add a fixed delay before port-forwarding.
		// b) Wait for Envoy to have listener by observing Envoy logs before port-forwarding.
		// c) Restart port-forwarding when connection attempts fail.
		//
		// Executing port-forward started in BeforeEach(), JustBeforeEach() or combining metrics
		// port with the admin port-forward command (127.0.0.1:19001 -> 9001) did not help.
		//
		// The simplest workaround (a) is taken here.
		time.Sleep(5 * time.Second)

		// Port-forward for metrics over HTTPS
		kubectlCmd, err := f.Kubectl.StartKubectlPortForward(18003, 8003, "projectcontour", f.Deployment.EnvoyResourceAndName())
		require.NoError(t, err)
		defer f.Kubectl.StopKubectlPortForward(kubectlCmd)

		clientCert, caBundle := f.Certs.GetTLSCertificate("projectcontour", "metrics-client")
		client := http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					MinVersion:   tls.VersionTLS13,
					Certificates: []tls.Certificate{clientCert},
					RootCAs:      caBundle,
				},
			},
		}

		gomega.Eventually(func() int {
			resp, err := client.Get("https://localhost:18003/stats")
			if err != nil {
				GinkgoWriter.Println(err)
				return 0
			}
			return resp.StatusCode
		}, "10s", "1s").Should(gomega.Equal(http.StatusOK))
	})
}
