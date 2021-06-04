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

// +build e2e

package ingress

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/projectcontour/contour/pkg/config"
	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/require"
	networkingv1 "k8s.io/api/networking/v1"
)

var f = e2e.NewFramework(false)

func TestIngress(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Ingress tests")
}

var _ = BeforeSuite(func() {
	require.NoError(f.T(), f.Deployment.EnsureResourcesForLocalContour())
})

var _ = AfterSuite(func() {
	f.DeleteNamespace(f.Deployment.Namespace.Name, true)
	gexec.CleanupBuildArtifacts()
})

var _ = Describe("Ingress", func() {
	var (
		contourCmd            *gexec.Session
		contourConfig         *config.Parameters
		contourConfigFile     string
		additionalContourArgs []string
	)

	BeforeEach(func() {
		// Contour config file contents, can be modified in nested
		// BeforeEach.
		contourConfig = &config.Parameters{}

		// Default contour serve command line arguments can be appended to in
		// nested BeforeEach.
		additionalContourArgs = []string{}
	})

	JustBeforeEach(func() {
		var err error
		contourCmd, contourConfigFile, err = f.Deployment.StartLocalContour(contourConfig, additionalContourArgs...)
		require.NoError(f.T(), err)

		// Wait for Envoy to be healthy.
		require.NoError(f.T(), f.Deployment.WaitForEnvoyDaemonSetUpdated())
	})

	AfterEach(func() {
		require.NoError(f.T(), f.Deployment.StopLocalContour(contourCmd, contourConfigFile))
	})

	f.NamespacedTest("001-tls-wildcard-host", testTLSWildcardHost)

	f.NamespacedTest("002-ingress-ensure-v1beta1", testEnsureV1Beta1)
})

func pathTypePtr(val networkingv1.PathType) *networkingv1.PathType {
	return &val
}
