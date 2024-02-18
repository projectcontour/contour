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

package filewatcher

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/require"
	apps_v1 "k8s.io/api/apps/v1"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/projectcontour/contour/test/e2e"
)

var f = e2e.NewFramework(true)

func TestFileWatcher(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "File Watcher tests")
}

var _ = BeforeSuite(func() {
	// just need 1 contour pod
	f.Deployment.ContourDeployment.Spec.Replicas = ptr.To(int32(1))
	require.NoError(f.T(), f.Deployment.EnsureResourcesForInclusterContour(true))
})

var _ = Describe("Contour file watcher", func() {
	Context("Update configMap of contour", func() {
		Specify("Contour should restart after configMap is restarted", func() {
			By("Get Contour pod's restart time")
			cd := &apps_v1.Deployment{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      f.Deployment.ContourDeployment.Name,
					Namespace: f.Deployment.ContourDeployment.Namespace,
				},
			}
			require.NoError(f.T(), f.Client.Get(context.TODO(), client.ObjectKeyFromObject(cd), cd))

			labelSelector := cd.Spec.Selector
			require.Len(f.T(), labelSelector.MatchLabels, 1)

			var podList core_v1.PodList
			require.NoError(f.T(), f.Client.List(context.TODO(), &podList, client.MatchingLabels(labelSelector.MatchLabels)))
			require.NotNil(f.T(), podList)
			require.Len(f.T(), podList.Items, 1)
			require.Len(f.T(), podList.Items[0].Status.ContainerStatuses, 1)

			previousRestartCount := podList.Items[0].Status.ContainerStatuses[0].RestartCount

			By("Get ConfigMap")
			cm := &core_v1.ConfigMap{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      f.Deployment.ContourConfigMap.Name,
					Namespace: f.Deployment.ContourConfigMap.Namespace,
				},
			}
			require.NoError(f.T(), f.Client.Get(context.TODO(), client.ObjectKeyFromObject(cm), cm))

			By("Update configmap's content")
			newConfigMapContent := `
accesslog-format: envoy`
			cm.Data["contour.yaml"] = newConfigMapContent

			require.NoError(f.T(), f.Client.Update(context.TODO(), cm))

			By("Contour pod's restartCount should get larger")
			require.Eventually(f.T(), func() bool {
				var podList core_v1.PodList
				require.NoError(f.T(), f.Client.List(context.TODO(), &podList, client.MatchingLabels(labelSelector.MatchLabels)))
				require.NotNil(f.T(), podList)
				require.Len(f.T(), podList.Items, 1)
				require.Len(f.T(), podList.Items[0].Status.ContainerStatuses, 1)
				return podList.Items[0].Status.ContainerStatuses[0].RestartCount > previousRestartCount
			}, time.Minute*2, time.Second)
		})
	})
})

var _ = AfterSuite(func() {
	require.NoError(f.T(), f.Deployment.DeleteResourcesForInclusterContour())
})
