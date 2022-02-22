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

package incluster

import (
	"context"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	contour_api_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var f = e2e.NewFramework(true)

func TestIncluster(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "In-cluster tests")
}

var _ = BeforeSuite(func() {
	// Default to using ContourConfiguration CRD and debug logging.
	originalArgs := f.Deployment.ContourDeployment.Spec.Template.Spec.Containers[0].Args
	var newArgs []string
	for _, arg := range originalArgs {
		if !strings.Contains(arg, "--config-path") {
			newArgs = append(newArgs, arg)
		}
	}
	newArgs = append(newArgs, "--contour-config-name=incluster-tests", "--debug")
	f.Deployment.ContourDeployment.Spec.Template.Spec.Containers[0].Args = newArgs

	require.NoError(f.T(), f.Deployment.EnsureResourcesForInclusterContour(false))
})

var _ = AfterSuite(func() {
	// Delete resources individually instead of deleting the entire contour
	// namespace as a performance optimization, because deleting non-empty
	// namespaces can take up to a couple minutes to complete.
	require.NoError(f.T(), f.Deployment.DeleteResourcesForInclusterContour())
})

var _ = Describe("Incluster", func() {
	var contourConfig *contour_api_v1alpha1.ContourConfiguration

	BeforeEach(func() {
		contourConfig = e2e.DefaultContourConfiguration()
		contourConfig.Name = "incluster-tests"
	})

	JustBeforeEach(func() {
		// Create contour deployment and config here so we can modify or do other
		// actions in BeforeEach.
		require.NoError(f.T(), f.Client.Create(context.TODO(), contourConfig))
		require.NoError(f.T(), f.Deployment.EnsureContourDeployment())
		require.NoError(f.T(), f.Deployment.WaitForContourDeploymentUpdated())
		require.NoError(f.T(), f.Deployment.WaitForEnvoyUpdated())
	})

	AfterEach(func() {
		require.NoError(f.T(), f.Deployment.DumpContourLogs())

		// Clean out contour after each test.
		require.NoError(f.T(), f.Deployment.EnsureDeleted(f.Deployment.ContourDeployment))
		require.NoError(f.T(), f.Deployment.EnsureDeleted(contourConfig))
		require.Eventually(f.T(), func() bool {
			pods := new(v1.PodList)
			podListOptions := &client.ListOptions{
				LabelSelector: labels.SelectorFromSet(f.Deployment.ContourDeployment.Spec.Selector.MatchLabels),
				Namespace:     f.Deployment.ContourDeployment.Namespace,
			}
			if err := f.Client.List(context.TODO(), pods, podListOptions); err != nil {
				return false
			}
			return len(pods.Items) == 0
		}, time.Minute, time.Millisecond*50)
	})

	f.NamespacedTest("smoke-test", testSimpleSmoke)

	f.NamespacedTest("leader-election", testLeaderElection)

	f.NamespacedTest("projectcontour-resource-rbac", testProjectcontourResourcesRBAC)

	f.NamespacedTest("ingress-resource-rbac", testIngressResourceRBAC)
})
