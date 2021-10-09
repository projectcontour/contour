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
// +build e2e

package contourconfiguration

import (
	"context"
	"os"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	contour_api_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var f = e2e.NewFramework(false)

func TestInfra(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ContourConfiguration tests")
}

var _ = BeforeSuite(func() {
	require.NoError(f.T(), f.Deployment.EnsureResourcesForLocalContour())
})

var _ = AfterSuite(func() {
	// Delete resources individually instead of deleting the entire contour
	// namespace as a performance optimization, because deleting non-empty
	// namespaces can take up to a couple of minutes to complete.
	require.NoError(f.T(), f.Deployment.DeleteResourcesForLocalContour())
	gexec.CleanupBuildArtifacts()
})

var (
	contourCmd            *gexec.Session
	contourConfigFile     string
	additionalContourArgs []string
)

var _ = Describe("ContourConfiguration Status", func() {

	AfterEach(func() {
		require.NoError(f.T(), f.Deployment.StopLocalContour(contourCmd, contourConfigFile))
	})

	f.Test(testValidContourConfigurationStatus)
})

func testValidContourConfigurationStatus() {

	Specify("leader election enabled", func() {
		if useContourConfiguration, variableFound := os.LookupEnv("USE_CONTOUR_CONFIGURATION_CRD"); variableFound && useContourConfiguration == "false" {
			Skip("configmap configuration not supported")
		}

		var err error

		contourConfiguration := e2e.DefaultContourConfiguration()

		// Enable LeaderElection since the test framework default is disabled.
		contourConfiguration.Spec.LeaderElection = contour_api_v1alpha1.LeaderElectionConfig{
			LeaseDuration: "10s",
			RenewDeadline: "5s",
			RetryPeriod:   "1s",
			Configmap: contour_api_v1alpha1.NamespacedName{
				Name:      "leader-elect",
				Namespace: "projectcontour",
			},
			DisableLeaderElection: false,
		}

		// Set the "config" to nil to disable running those tests since they don't apply.
		contourCmd, contourConfigFile, err = f.Deployment.StartLocalContour(nil, contourConfiguration, additionalContourArgs...)
		require.NoError(f.T(), err)

		// Verify Status on Contour
		require.True(f.T(), f.WaitForContourConfigurationStatus(contourConfiguration, contourConfigurationValid))

		// Cleanup since this case uses leader election whereas others do not.
		require.NoError(f.T(), f.Client.Delete(context.TODO(), &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "leader-elect",
				Namespace: "projectcontour",
			},
		}))
	})

	Specify("leader election disabled", func() {
		if useContourConfiguration, variableFound := os.LookupEnv("USE_CONTOUR_CONFIGURATION_CRD"); variableFound && useContourConfiguration == "false" {
			Skip("configmap configuration not supported")
		}
		var err error

		contourConfiguration := e2e.DefaultContourConfiguration()

		// Set the "config" to nil to disable running those tests since they don't apply.
		contourCmd, contourConfigFile, err = f.Deployment.StartLocalContour(nil, contourConfiguration, additionalContourArgs...)
		require.NoError(f.T(), err)

		// Verify Status on Contour
		require.True(f.T(), f.WaitForContourConfigurationStatus(contourConfiguration, contourConfigurationValid))
	})
}

// contourConfigurationValid returns true if the config has a .status.conditions
// entry of Valid: true".
func contourConfigurationValid(config *contour_api_v1alpha1.ContourConfiguration) bool {
	if config == nil {
		return false
	}

	for _, cond := range config.Status.Conditions {
		if cond.Type == "Valid" && cond.Status == metav1.ConditionTrue {
			return true
		}
	}
	return false
}
