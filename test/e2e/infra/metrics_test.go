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

	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/stretchr/testify/require"

	"github.com/projectcontour/contour/test/e2e"
)

func testMetrics() {
	Specify("requests to default metrics listener are served", func() {
		res, ok := f.HTTP.MetricsRequestUntil(&e2e.HTTPRequestOpts{
			Path:      "/stats",
			Condition: e2e.HasStatusCode(200),
		})
		require.NotNil(f.T(), res, "request never succeeded")
		require.Truef(f.T(), ok, "expected 200 response code, got %d", res.StatusCode)
	})
}

func testReady() {
	Specify("requests to default ready listener are served", func() {
		res, ok := f.HTTP.MetricsRequestUntil(&e2e.HTTPRequestOpts{
			Path:      "/ready",
			Condition: e2e.HasStatusCode(200),
		})
		require.NotNil(f.T(), res, "request never succeeded")
		require.Truef(f.T(), ok, "expected 200 response code, got %d", res.StatusCode)
	})
}

func testEnvoyMetricsOverHTTPS() {
	// Flake tracking issue: https://github.com/projectcontour/contour/issues/5932
	Specify("requests to metrics listener are served", FlakeAttempts(3), func() {
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

		var kubectlCmd *gexec.Session
		defer func() {
			if kubectlCmd != nil {
				f.Kubectl.StopKubectlPortForward(kubectlCmd)
			}
		}()

		gomega.Eventually(func() int {
			var err error
			if kubectlCmd == nil {
				kubectlCmd, err = f.Kubectl.StartKubectlPortForward(18003, 8003, "projectcontour", f.Deployment.EnvoyResourceAndName())
				if err != nil {
					GinkgoWriter.Println("failed to start port-forward:", err)
					return 0
				}
			}

			resp, err := client.Get("https://localhost:18003/stats")
			if err != nil {
				GinkgoWriter.Println("request failed, restarting port-forward:", err)
				f.Kubectl.StopKubectlPortForward(kubectlCmd)
				kubectlCmd = nil
				return 0
			}
			defer resp.Body.Close()
			return resp.StatusCode
		}, "30s", "1s").Should(gomega.Equal(http.StatusOK))
	})
}
