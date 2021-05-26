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
	"time"

	. "github.com/onsi/ginkgo"
	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/require"
	apps_v1 "k8s.io/api/apps/v1"
	batch_v1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	networking_v1 "k8s.io/api/networking/v1"
	rbac_v1 "k8s.io/api/rbac/v1"
	apiextensions_v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
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

	// TODO: Install "from" version here instead of relying on it existing in cluster.
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
		resources := updateContourDeploymentResources()

		By("waiting for contour deployment to be updated")
		require.Eventually(f.T(), func() bool {
			require.NoError(f.T(), f.Client.Get(context.TODO(), client.ObjectKeyFromObject(resources.contourDeployment), resources.contourDeployment))
			return resources.contourDeployment.Status.Replicas == *resources.contourDeployment.Spec.Replicas
		}, time.Minute*1, time.Millisecond*50)

		By("waiting for envoy daemonset to be updated")
		require.Eventually(f.T(), func() bool {
			require.NoError(f.T(), f.Client.Get(context.TODO(), client.ObjectKeyFromObject(resources.envoyDaemonSet), resources.envoyDaemonSet))
			// This might work for now while we have only one worker node, but
			// if we expand to more, we will have to rethink this.
			return resources.envoyDaemonSet.Status.NumberReady > 0
		}, time.Minute*3, time.Millisecond*50)
		// TODO: when we deploy the from version in this test, ensure envoy
		// shutdown time is configured to take less time.

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

type contourDeploymentResources struct {
	namespace                 *v1.Namespace
	contourServiceAccount     *v1.ServiceAccount
	envoyServiceAccount       *v1.ServiceAccount
	contourConfigMap          *v1.ConfigMap
	extensionServiceCRD       *apiextensions_v1.CustomResourceDefinition
	httpProxyCRD              *apiextensions_v1.CustomResourceDefinition
	tlsCertDelegationCRD      *apiextensions_v1.CustomResourceDefinition
	certgenServiceAccount     *v1.ServiceAccount
	contourRoleBinding        *rbac_v1.RoleBinding
	certgenRole               *rbac_v1.Role
	certgenJob                *batch_v1.Job
	contourClusterRoleBinding *rbac_v1.ClusterRoleBinding
	contourClusterRole        *rbac_v1.ClusterRole
	contourService            *v1.Service
	envoyService              *v1.Service
	contourDeployment         *apps_v1.Deployment
	envoyDaemonSet            *apps_v1.DaemonSet
}

// Unmarshals resources from rendered Contour manifest in order and updates
// each.
// Note: This will need to be updated if any new resources are added to the
// rendered deployment manifest.
func updateContourDeploymentResources() contourDeploymentResources {
	file, err := os.Open("../../../examples/render/contour.yaml")
	require.NoError(f.T(), err)
	defer file.Close()
	decoder := yaml.NewYAMLToJSONDecoder(file)

	resources := contourDeploymentResources{}

	// Use a temp to fetch the resource version in case the original resource
	// specifies a field the updated resource does not.

	// Discard empty document.
	require.NoError(f.T(), decoder.Decode(new(struct{})))

	By("updating contour namespace")
	resources.namespace = new(v1.Namespace)
	require.NoError(f.T(), decoder.Decode(resources.namespace))
	tempNS := new(v1.Namespace)
	require.NoError(f.T(), f.Client.Get(context.TODO(), client.ObjectKeyFromObject(resources.namespace), tempNS))
	resources.namespace.SetResourceVersion(tempNS.GetResourceVersion())
	require.NoError(f.T(), f.Client.Update(context.TODO(), resources.namespace))

	By("updating contour service account")
	resources.contourServiceAccount = new(v1.ServiceAccount)
	require.NoError(f.T(), decoder.Decode(resources.contourServiceAccount))
	tempSA := new(v1.ServiceAccount)
	require.NoError(f.T(), f.Client.Get(context.TODO(), client.ObjectKeyFromObject(resources.contourServiceAccount), tempSA))
	resources.contourServiceAccount.SetResourceVersion(tempSA.GetResourceVersion())
	require.NoError(f.T(), f.Client.Update(context.TODO(), resources.contourServiceAccount))

	By("updating envoy service account")
	resources.envoyServiceAccount = new(v1.ServiceAccount)
	require.NoError(f.T(), decoder.Decode(resources.envoyServiceAccount))
	require.NoError(f.T(), f.Client.Get(context.TODO(), client.ObjectKeyFromObject(resources.envoyServiceAccount), tempSA))
	resources.envoyServiceAccount.SetResourceVersion(tempSA.GetResourceVersion())
	require.NoError(f.T(), f.Client.Update(context.TODO(), resources.envoyServiceAccount))

	By("updating contour config map")
	resources.contourConfigMap = new(v1.ConfigMap)
	require.NoError(f.T(), decoder.Decode(resources.contourConfigMap))
	tempCM := new(v1.ConfigMap)
	require.NoError(f.T(), f.Client.Get(context.TODO(), client.ObjectKeyFromObject(resources.contourConfigMap), tempCM))
	resources.contourConfigMap.SetResourceVersion(tempCM.GetResourceVersion())
	require.NoError(f.T(), f.Client.Update(context.TODO(), resources.contourConfigMap))

	By("updating contour CRDs")
	// ExtensionService
	resources.extensionServiceCRD = new(apiextensions_v1.CustomResourceDefinition)
	require.NoError(f.T(), decoder.Decode(resources.extensionServiceCRD))
	tempCRD := new(apiextensions_v1.CustomResourceDefinition)
	require.NoError(f.T(), f.Client.Get(context.TODO(), client.ObjectKeyFromObject(resources.extensionServiceCRD), tempCRD))
	resources.extensionServiceCRD.SetResourceVersion(tempCRD.GetResourceVersion())
	require.NoError(f.T(), f.Client.Update(context.TODO(), resources.extensionServiceCRD))
	// HTTPProxy
	resources.httpProxyCRD = new(apiextensions_v1.CustomResourceDefinition)
	require.NoError(f.T(), decoder.Decode(resources.httpProxyCRD))
	require.NoError(f.T(), f.Client.Get(context.TODO(), client.ObjectKeyFromObject(resources.httpProxyCRD), tempCRD))
	resources.httpProxyCRD.SetResourceVersion(tempCRD.GetResourceVersion())
	require.NoError(f.T(), f.Client.Update(context.TODO(), resources.httpProxyCRD))
	// TLSCertificateDelegation
	resources.tlsCertDelegationCRD = new(apiextensions_v1.CustomResourceDefinition)
	require.NoError(f.T(), decoder.Decode(resources.tlsCertDelegationCRD))
	require.NoError(f.T(), f.Client.Get(context.TODO(), client.ObjectKeyFromObject(resources.tlsCertDelegationCRD), tempCRD))
	resources.tlsCertDelegationCRD.SetResourceVersion(tempCRD.GetResourceVersion())
	require.NoError(f.T(), f.Client.Update(context.TODO(), resources.tlsCertDelegationCRD))

	By("updating certgen service account")
	resources.certgenServiceAccount = new(v1.ServiceAccount)
	require.NoError(f.T(), decoder.Decode(resources.certgenServiceAccount))
	require.NoError(f.T(), f.Client.Get(context.TODO(), client.ObjectKeyFromObject(resources.certgenServiceAccount), tempSA))
	resources.certgenServiceAccount.SetResourceVersion(tempSA.GetResourceVersion())
	require.NoError(f.T(), f.Client.Update(context.TODO(), resources.certgenServiceAccount))

	By("updating contour role binding")
	resources.contourRoleBinding = new(rbac_v1.RoleBinding)
	require.NoError(f.T(), decoder.Decode(resources.contourRoleBinding))
	tempRB := new(rbac_v1.RoleBinding)
	require.NoError(f.T(), f.Client.Get(context.TODO(), client.ObjectKeyFromObject(resources.contourRoleBinding), tempRB))
	resources.contourRoleBinding.SetResourceVersion(tempRB.GetResourceVersion())
	require.NoError(f.T(), f.Client.Update(context.TODO(), resources.contourRoleBinding))

	By("updating certgen role")
	resources.certgenRole = new(rbac_v1.Role)
	require.NoError(f.T(), decoder.Decode(resources.certgenRole))
	tempR := new(rbac_v1.Role)
	require.NoError(f.T(), f.Client.Get(context.TODO(), client.ObjectKeyFromObject(resources.certgenRole), tempR))
	resources.certgenRole.SetResourceVersion(tempR.GetResourceVersion())
	require.NoError(f.T(), f.Client.Update(context.TODO(), resources.certgenRole))

	By("updating certgen job")
	resources.certgenJob = new(batch_v1.Job)
	require.NoError(f.T(), decoder.Decode(resources.certgenJob))
	// Update container image.
	require.Len(f.T(), resources.certgenJob.Spec.Template.Spec.Containers, 1)
	resources.certgenJob.Spec.Template.Spec.Containers[0].Image = contourUpgradeToImage
	resources.certgenJob.Spec.Template.Spec.Containers[0].ImagePullPolicy = v1.PullIfNotPresent
	require.NoError(f.T(), f.Client.Create(context.TODO(), resources.certgenJob))

	By("updating contour cluster role binding")
	resources.contourClusterRoleBinding = new(rbac_v1.ClusterRoleBinding)
	require.NoError(f.T(), decoder.Decode(resources.contourClusterRoleBinding))
	tempCRB := new(rbac_v1.ClusterRoleBinding)
	require.NoError(f.T(), f.Client.Get(context.TODO(), client.ObjectKeyFromObject(resources.contourClusterRoleBinding), tempCRB))
	resources.contourClusterRoleBinding.SetResourceVersion(tempCRB.GetResourceVersion())
	require.NoError(f.T(), f.Client.Update(context.TODO(), resources.contourClusterRoleBinding))

	By("updating contour cluster role")
	resources.contourClusterRole = new(rbac_v1.ClusterRole)
	require.NoError(f.T(), decoder.Decode(resources.contourClusterRole))
	tempCR := new(rbac_v1.ClusterRole)
	require.NoError(f.T(), f.Client.Get(context.TODO(), client.ObjectKeyFromObject(resources.contourClusterRole), tempCR))
	resources.contourClusterRole.SetResourceVersion(tempCR.GetResourceVersion())
	require.NoError(f.T(), f.Client.Update(context.TODO(), resources.contourClusterRole))

	By("updating contour service")
	resources.contourService = new(v1.Service)
	require.NoError(f.T(), decoder.Decode(resources.contourService))
	tempS := new(v1.Service)
	require.NoError(f.T(), f.Client.Get(context.TODO(), client.ObjectKeyFromObject(resources.contourService), tempS))
	resources.contourService.SetResourceVersion(tempS.GetResourceVersion())
	// Set cluster ip.
	resources.contourService.Spec.ClusterIP = tempS.Spec.ClusterIP
	resources.contourService.Spec.ClusterIPs = tempS.Spec.ClusterIPs
	require.NoError(f.T(), f.Client.Update(context.TODO(), resources.contourService))

	By("updating envoy service")
	resources.envoyService = new(v1.Service)
	require.NoError(f.T(), decoder.Decode(resources.envoyService))
	require.NoError(f.T(), f.Client.Get(context.TODO(), client.ObjectKeyFromObject(resources.envoyService), tempS))
	resources.envoyService.SetResourceVersion(tempS.GetResourceVersion())
	// Set cluster ip and health check node port.
	resources.envoyService.Spec.ClusterIP = tempS.Spec.ClusterIP
	resources.envoyService.Spec.ClusterIPs = tempS.Spec.ClusterIPs
	resources.envoyService.Spec.HealthCheckNodePort = tempS.Spec.HealthCheckNodePort
	require.NoError(f.T(), f.Client.Update(context.TODO(), resources.envoyService))

	By("updating contour deployment")
	resources.contourDeployment = new(apps_v1.Deployment)
	require.NoError(f.T(), decoder.Decode(resources.contourDeployment))
	tempDep := new(apps_v1.Deployment)
	require.NoError(f.T(), f.Client.Get(context.TODO(), client.ObjectKeyFromObject(resources.contourDeployment), tempDep))
	resources.contourDeployment.SetResourceVersion(tempDep.GetResourceVersion())
	// Update container image.
	require.Len(f.T(), resources.contourDeployment.Spec.Template.Spec.Containers, 1)
	resources.contourDeployment.Spec.Template.Spec.Containers[0].Image = contourUpgradeToImage
	resources.contourDeployment.Spec.Template.Spec.Containers[0].ImagePullPolicy = v1.PullIfNotPresent
	require.NoError(f.T(), f.Client.Update(context.TODO(), resources.contourDeployment))

	By("updating envoy daemonset")
	resources.envoyDaemonSet = new(apps_v1.DaemonSet)
	require.NoError(f.T(), decoder.Decode(resources.envoyDaemonSet))
	tempDS := new(apps_v1.DaemonSet)
	require.NoError(f.T(), f.Client.Get(context.TODO(), client.ObjectKeyFromObject(resources.envoyDaemonSet), tempDS))
	resources.envoyDaemonSet.SetResourceVersion(tempDS.GetResourceVersion())
	// Update container image.
	require.Len(f.T(), resources.envoyDaemonSet.Spec.Template.Spec.InitContainers, 1)
	resources.envoyDaemonSet.Spec.Template.Spec.InitContainers[0].Image = contourUpgradeToImage
	resources.envoyDaemonSet.Spec.Template.Spec.InitContainers[0].ImagePullPolicy = v1.PullIfNotPresent
	require.Len(f.T(), resources.envoyDaemonSet.Spec.Template.Spec.Containers, 2)
	resources.envoyDaemonSet.Spec.Template.Spec.Containers[0].Image = contourUpgradeToImage
	resources.envoyDaemonSet.Spec.Template.Spec.Containers[0].ImagePullPolicy = v1.PullIfNotPresent
	require.NoError(f.T(), f.Client.Update(context.TODO(), resources.envoyDaemonSet))

	return resources
}
