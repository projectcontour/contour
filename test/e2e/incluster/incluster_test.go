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
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/require"
)

var f = e2e.NewFramework(true)

func TestIncluster(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "In-cluster tests")
}

var _ = BeforeSuite(func() {
	require.NoError(f.T(), f.Deployment.EnsureResourcesForInclusterContour(false))
})

var _ = AfterSuite(func() {
	// Delete resources individually instead of deleting the entire contour
	// namespace as a performance optimization, because deleting non-empty
	// namespaces can take up to a couple minutes to complete.
	require.NoError(f.T(), f.Deployment.DeleteResourcesForInclusterContour())
})

var _ = Describe("Incluster", func() {
	JustBeforeEach(func() {
		// Create contour deployment here so we can modify or do other
		// actions in BeforeEach.
		require.NoError(f.T(), f.Deployment.EnsureContourDeployment())
		require.NoError(f.T(), f.Deployment.WaitForContourDeploymentUpdated())
		require.NoError(f.T(), f.Deployment.WaitForEnvoyDaemonSetUpdated())
	})

	AfterEach(func() {
		// Clean out contour after each test.
		require.NoError(f.T(), f.Deployment.EnsureDeleted(f.Deployment.ContourDeployment))
	})

	f.NamespacedTest("smoke-test", testSimpleSmoke)

	f.NamespacedTest("leader-election", testLeaderElection)

	f.NamespacedTest("projectcontour-resource-rbac", testProjectcontourResourcesRBAC)

	f.NamespacedTest("ingress-resource-rbac", testIngressResourceRBAC)
})
