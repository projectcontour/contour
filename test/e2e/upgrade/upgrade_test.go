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

package upgrade

import (
	"context"
	"os"
	"testing"

	. "github.com/onsi/ginkgo"
	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	networking_v1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	f *e2e.Framework

	// Contour container image to upgrade deployment to.
	// If running against a kind cluster, this image should be loaded into
	// the cluster prior to running this test suite.
	contourUpgradeToImage string

	// Contour version we are upgrading from.
	contourUpgradeFromVersion string
)

func TestUpgrade(t *testing.T) {
	RunSpecs(t, "Upgrade Suite")
}

var _ = BeforeSuite(func() {
	f = e2e.NewFramework(GinkgoT())

	contourUpgradeFromVersion = os.Getenv("CONTOUR_UPGRADE_FROM_VERSION")
	require.NotEmpty(f.T(), contourUpgradeFromVersion, "CONTOUR_UPGRADE_FROM_VERSION environment variable not supplied")
	By("Testing Contour upgrade from " + contourUpgradeFromVersion)

	contourUpgradeToImage = os.Getenv("CONTOUR_UPGRADE_TO_IMAGE")
	require.NotEmpty(f.T(), contourUpgradeToImage, "CONTOUR_UPGRADE_TO_IMAGE environment variable not supplied")
	By("upgrading Contour image to " + contourUpgradeToImage)
})

var _ = Describe("upgrading Contour", func() {
	var namespace string

	const appHost = "upgrade-echo.test.com"

	BeforeEach(func() {
		namespace = "contour-upgrade-test"
		f.CreateNamespace(namespace)

		By("deploying an app")
		f.Fixtures.Echo.Deploy(namespace, "echo")
		i := &networking_v1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "echo",
			},
			Spec: networking_v1.IngressSpec{
				Rules: []networking_v1.IngressRule{
					{
						Host: appHost,
						IngressRuleValue: networking_v1.IngressRuleValue{
							HTTP: &networking_v1.HTTPIngressRuleValue{
								Paths: []networking_v1.HTTPIngressPath{
									{
										Path:     "/",
										PathType: ingressPathTypePtr(networking_v1.PathTypePrefix),
										Backend: networking_v1.IngressBackend{
											Service: &networking_v1.IngressServiceBackend{
												Name: "echo",
												Port: networking_v1.ServiceBackendPort{Number: 80},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}
		require.NoError(f.T(), f.Client.Create(context.TODO(), i))

		By("ensuring it is routable")
		checkRoutability(appHost)
	})

	AfterEach(func() {
		By("cleaning up test artifacts")
		f.DeleteNamespace(namespace)
	})

	Specify("applications remain routable after the upgrade", func() {
		updateContourDeploymentResources()

		By("waiting for contour deployment to be updated")
		require.NoError(f.T(), f.Deployment.WaitForContourDeploymentUpdated())

		By("waiting for envoy daemonset to be updated")
		require.NoError(f.T(), f.Deployment.WaitForEnvoyDaemonSetUpdated())

		By("ensuring app is still routable")
		checkRoutability(appHost)
	})
})

func ingressPathTypePtr(t networking_v1.PathType) *networking_v1.PathType {
	return &t
}

func checkRoutability(host string) {
	res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
		Host:      host,
		Path:      "/echo",
		Condition: e2e.HasStatusCode(200),
	})
	require.Truef(f.T(), ok, "expected 200 response code, got %d", res.StatusCode)
}

func updateContourDeploymentResources() {
	By("updating contour namespace")
	require.NoError(f.T(), f.Deployment.EnsureNamespace())

	By("updating contour service account")
	require.NoError(f.T(), f.Deployment.EnsureContourServiceAccount())

	By("updating envoy service account")
	require.NoError(f.T(), f.Deployment.EnsureEnvoyServiceAccount())

	By("updating contour config map")
	require.NoError(f.T(), f.Deployment.EnsureContourConfigMap())

	By("updating contour CRDs")
	require.NoError(f.T(), f.Deployment.EnsureExtensionServiceCRD())
	require.NoError(f.T(), f.Deployment.EnsureHTTPProxyCRD())
	require.NoError(f.T(), f.Deployment.EnsureTLSCertDelegationCRD())

	By("updating certgen service account")
	require.NoError(f.T(), f.Deployment.EnsureCertgenServiceAccount())

	By("updating contour role binding")
	require.NoError(f.T(), f.Deployment.EnsureContourRoleBinding())

	By("updating certgen role")
	require.NoError(f.T(), f.Deployment.EnsureCertgenRole())

	By("updating certgen job")
	// Update container image.
	require.Len(f.T(), f.Deployment.CertgenJob.Spec.Template.Spec.Containers, 1)
	f.Deployment.CertgenJob.Spec.Template.Spec.Containers[0].Image = contourUpgradeToImage
	f.Deployment.CertgenJob.Spec.Template.Spec.Containers[0].ImagePullPolicy = v1.PullIfNotPresent
	require.NoError(f.T(), f.Deployment.EnsureCertgenJob())

	By("updating contour cluster role binding")
	require.NoError(f.T(), f.Deployment.EnsureContourClusterRoleBinding())

	By("updating contour cluster role")
	require.NoError(f.T(), f.Deployment.EnsureContourClusterRole())

	By("updating contour service")
	// Set cluster ip.
	tempS := new(v1.Service)
	require.NoError(f.T(), f.Client.Get(context.TODO(), client.ObjectKeyFromObject(f.Deployment.ContourService), tempS))
	f.Deployment.ContourService.Spec.ClusterIP = tempS.Spec.ClusterIP
	f.Deployment.ContourService.Spec.ClusterIPs = tempS.Spec.ClusterIPs
	require.NoError(f.T(), f.Deployment.EnsureContourService())

	By("updating envoy service")
	// Set cluster ip and health check node port.
	require.NoError(f.T(), f.Client.Get(context.TODO(), client.ObjectKeyFromObject(f.Deployment.EnvoyService), tempS))
	f.Deployment.EnvoyService.Spec.ClusterIP = tempS.Spec.ClusterIP
	f.Deployment.EnvoyService.Spec.ClusterIPs = tempS.Spec.ClusterIPs
	f.Deployment.EnvoyService.Spec.HealthCheckNodePort = tempS.Spec.HealthCheckNodePort
	require.NoError(f.T(), f.Deployment.EnsureEnvoyService())

	By("updating contour deployment")
	// Update container image.
	require.Len(f.T(), f.Deployment.ContourDeployment.Spec.Template.Spec.Containers, 1)
	f.Deployment.ContourDeployment.Spec.Template.Spec.Containers[0].Image = contourUpgradeToImage
	f.Deployment.ContourDeployment.Spec.Template.Spec.Containers[0].ImagePullPolicy = v1.PullIfNotPresent
	require.NoError(f.T(), f.Deployment.EnsureContourDeployment())

	By("updating envoy daemonset")
	// Update container image.
	require.Len(f.T(), f.Deployment.EnvoyDaemonSet.Spec.Template.Spec.InitContainers, 1)
	f.Deployment.EnvoyDaemonSet.Spec.Template.Spec.InitContainers[0].Image = contourUpgradeToImage
	f.Deployment.EnvoyDaemonSet.Spec.Template.Spec.InitContainers[0].ImagePullPolicy = v1.PullIfNotPresent
	require.Len(f.T(), f.Deployment.EnvoyDaemonSet.Spec.Template.Spec.Containers, 2)
	f.Deployment.EnvoyDaemonSet.Spec.Template.Spec.Containers[0].Image = contourUpgradeToImage
	f.Deployment.EnvoyDaemonSet.Spec.Template.Spec.Containers[0].ImagePullPolicy = v1.PullIfNotPresent
	require.NoError(f.T(), f.Deployment.EnsureEnvoyDaemonSet())
}
