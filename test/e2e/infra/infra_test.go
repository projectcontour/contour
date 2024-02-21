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
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/stretchr/testify/require"
	core_v1 "k8s.io/api/core/v1"

	contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/pkg/config"
	"github.com/projectcontour/contour/test/e2e"
)

var (
	f = e2e.NewFramework(false)

	// Functions called after suite to clean up resources.
	cleanup []func()
)

func TestInfra(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Infra tests")
}

var _ = BeforeSuite(func() {
	// Add volume mount for the Envoy deployment for certificate and key,
	// used only for testing metrics over HTTPS.
	f.Deployment.EnvoyExtraVolumeMounts = []core_v1.VolumeMount{{
		Name:      "metrics-certs",
		MountPath: "/metrics-certs",
	}}
	f.Deployment.EnvoyExtraVolumes = []core_v1.Volume{{
		Name: "metrics-certs",
		VolumeSource: core_v1.VolumeSource{
			Secret: &core_v1.SecretVolumeSource{
				SecretName: "metrics-server",
			},
		},
	}}

	require.NoError(f.T(), f.Deployment.EnsureResourcesForLocalContour())

	// Create certificate and key for metrics over HTTPS.
	cleanup = append(cleanup,
		f.Certs.CreateCA("projectcontour", "metrics-ca"),
		f.Certs.CreateCert("projectcontour", "metrics-server", "metrics-ca", "localhost"),
		f.Certs.CreateCert("projectcontour", "metrics-client", "metrics-ca"),
	)
})

var _ = AfterSuite(func() {
	// Delete resources individually instead of deleting the entire contour
	// namespace as a performance optimization, because deleting non-empty
	// namespaces can take up to a couple of minutes to complete.
	for _, c := range cleanup {
		c()
	}
	require.NoError(f.T(), f.Deployment.DeleteResourcesForLocalContour())
	gexec.CleanupBuildArtifacts()
})

var _ = Describe("Infra", func() {
	var (
		contourCmd            *gexec.Session
		kubectlCmd            *gexec.Session
		contourConfig         *config.Parameters
		contourConfiguration  *contour_v1alpha1.ContourConfiguration
		contourConfigFile     string
		additionalContourArgs []string
	)

	BeforeEach(func() {
		// Contour config file contents, can be modified in nested
		// BeforeEach.
		contourConfig = e2e.DefaultContourConfigFileParams()

		// Contour configuration crd, can be modified in nested
		// BeforeEach.
		contourConfiguration = e2e.DefaultContourConfiguration()

		// Default contour serve command line arguments can be appended to in
		// nested BeforeEach.
		additionalContourArgs = []string{}
	})

	// JustBeforeEach is called after each of the nested BeforeEach are
	// called, so it is a final setup step before running a test.
	// A nested BeforeEach may have modified Contour config, so we wait
	// until here to start Contour.
	JustBeforeEach(func() {
		var err error
		contourCmd, contourConfigFile, err = f.Deployment.StartLocalContour(contourConfig, contourConfiguration, additionalContourArgs...)
		require.NoError(f.T(), err)

		// Wait for Envoy to be healthy.
		require.NoError(f.T(), f.Deployment.WaitForEnvoyUpdated())

		kubectlCmd, err = f.Kubectl.StartKubectlPortForward(19001, 9001, "projectcontour", f.Deployment.EnvoyResourceAndName(), additionalContourArgs...)
		require.NoError(f.T(), err)
	})

	AfterEach(func() {
		f.Kubectl.StopKubectlPortForward(kubectlCmd)
		require.NoError(f.T(), f.Deployment.StopLocalContour(contourCmd, contourConfigFile))
	})

	f.Test(testMetrics)
	f.Test(testReady)

	Context("when serving metrics over HTTPS", func() {
		BeforeEach(func() {
			contourConfig.Metrics.Envoy = config.MetricsServerParameters{
				Address:    "0.0.0.0",
				Port:       8003,
				ServerCert: "/metrics-certs/tls.crt",
				ServerKey:  "/metrics-certs/tls.key",
				CABundle:   "/metrics-certs/ca.crt",
			}

			contourConfiguration.Spec.Envoy.Metrics = &contour_v1alpha1.MetricsConfig{
				Address: "0.0.0.0",
				Port:    8003,
				TLS: &contour_v1alpha1.MetricsTLS{
					CertFile: "/metrics-certs/tls.crt",
					KeyFile:  "/metrics-certs/tls.key",
					CAFile:   "/metrics-certs/ca.crt",
				},
			}
		})
		f.Test(testEnvoyMetricsOverHTTPS)
	})

	f.Test(testAdminInterface)

	Context("contour with endpoint slices", func() {
		withEndpointSlicesEnabled := func(body e2e.NamespacedTestBody) e2e.NamespacedTestBody {
			return func(namespace string) {
				Context("with endpoint slice enabled", func() {
					BeforeEach(func() {
						contourConfig.FeatureFlags = []string{
							"useEndpointSlices",
						}
					})

					body(namespace)
				})
			}
		}

		f.NamespacedTest("simple-endpoint-slice", withEndpointSlicesEnabled(testSimpleEndpointSlice))
	})
})
