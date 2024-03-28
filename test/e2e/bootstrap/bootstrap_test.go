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

package bootstrap

import (
	"testing"

	contour_api_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/projectcontour/contour/pkg/config"
	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/require"
)

var (
	f = e2e.NewFramework(false)

	// Functions called after suite to clean up resources.
	cleanup []func()
)

func TestBootstrap(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Bootstrap tests")
}

var _ = BeforeSuite(func() {
	// add overload manager arguments to envoy bootstrap
	f.Deployment.ContourBootstrapExtraArgs = []string{
		"--overload-max-heap=987654321",
		"--overload-dowstream-max-conn=1000",
	}

	require.NoError(f.T(), f.Deployment.EnsureResourcesForLocalContour())
})

var _ = AfterSuite(func() {
	for _, c := range cleanup {
		c()
	}
	require.NoError(f.T(), f.Deployment.DeleteResourcesForLocalContour())
	gexec.CleanupBuildArtifacts()
})

var _ = Describe("Bootstrap", func() {
	var (
		contourCmd           *gexec.Session
		kubectlCmd           *gexec.Session
		contourConfig        *config.Parameters
		contourConfiguration *contour_api_v1alpha1.ContourConfiguration
		contourConfigFile    string
	)

	BeforeEach(func() {
		contourConfig = e2e.DefaultContourConfigFileParams()
		contourConfiguration = e2e.DefaultContourConfiguration()
	})

	JustBeforeEach(func() {
		var err error
		contourCmd, contourConfigFile, err = f.Deployment.StartLocalContour(contourConfig, contourConfiguration)
		require.NoError(f.T(), err)

		// Wait for Envoy to be healthy.
		require.NoError(f.T(), f.Deployment.WaitForEnvoyUpdated())

		kubectlCmd, err = f.Kubectl.StartKubectlPortForward(19001, 9001, "projectcontour", f.Deployment.EnvoyResourceAndName())
		require.NoError(f.T(), err)
	})

	AfterEach(func() {
		f.Kubectl.StopKubectlPortForward(kubectlCmd)
		require.NoError(f.T(), f.Deployment.StopLocalContour(contourCmd, contourConfigFile))
	})

	f.Test(func() {
		Specify("requests to default ready listener are served", func() {
			t := f.T()

			res, ok := f.HTTP.MetricsRequestUntil(&e2e.HTTPRequestOpts{
				Path:      "/ready",
				Condition: e2e.HasStatusCode(200),
			})
			require.NotNil(t, res, "request never succeeded")
			require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)
		})
	})
})
