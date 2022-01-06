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

package upgrade

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/require"
	networking_v1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	f = e2e.NewFramework(true)

	// Contour version we are upgrading from.
	contourUpgradeFromVersion string
)

func TestUpgrade(t *testing.T) {
	RunSpecs(t, "Upgrade Suite")
}

var _ = BeforeSuite(func() {
	contourUpgradeFromVersion = os.Getenv("CONTOUR_UPGRADE_FROM_VERSION")
	require.NotEmpty(f.T(), contourUpgradeFromVersion, "CONTOUR_UPGRADE_FROM_VERSION environment variable not supplied")
	By("Testing Contour upgrade from " + contourUpgradeFromVersion)

	// We should be running in a multi-node cluster with a proper load
	// balancer, so fetch load balancer ip to make requests to.
	require.NoError(f.T(), f.Client.Get(context.TODO(), client.ObjectKeyFromObject(f.Deployment.EnvoyService), f.Deployment.EnvoyService))
	require.Greater(f.T(), len(f.Deployment.EnvoyService.Status.LoadBalancer.Ingress), 0)
	require.NotEmpty(f.T(), f.Deployment.EnvoyService.Status.LoadBalancer.Ingress[0].IP)
	f.HTTP.HTTPURLBase = "http://" + f.Deployment.EnvoyService.Status.LoadBalancer.Ingress[0].IP
	f.HTTP.HTTPSURLBase = "https://" + f.Deployment.EnvoyService.Status.LoadBalancer.Ingress[0].IP
})

var _ = Describe("upgrading Contour", func() {
	const appHost = "upgrade-echo.test.com"

	f.NamespacedTest("contour-upgrade-test", func(namespace string) {
		Specify("applications remain routable after the upgrade", func() {
			By("deploying an app")
			f.Fixtures.Echo.DeployN(namespace, "echo", 2)
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

			poller, err := e2e.StartAppPoller(f.HTTP.HTTPURLBase, appHost, http.StatusOK)
			require.NoError(f.T(), err)

			By("deploying updated contour resources")
			require.NoError(f.T(), f.Deployment.EnsureResourcesForInclusterContour(true))

			By("waiting for contour deployment to be updated")
			require.NoError(f.T(), f.Deployment.WaitForContourDeploymentUpdated())

			By("waiting for envoy daemonset to be updated")
			require.NoError(f.T(), f.Deployment.WaitForEnvoyDaemonSetOutOfDate())
			require.NoError(f.T(), f.Deployment.WaitForEnvoyDaemonSetUpdated())

			By("ensuring app is still routable")
			checkRoutability(appHost)

			poller.Stop()
			totalRequests, successfulRequests := poller.Results()
			fmt.Fprintf(GinkgoWriter, "Total requests: %d, successful requests: %d\n", totalRequests, successfulRequests)
			require.Greater(f.T(), totalRequests, uint(0))
			successPercentage := 100 * float64(successfulRequests) / float64(totalRequests)
			require.Greaterf(f.T(), successPercentage, float64(90.0), "success rate of %.2f%% less than 90%", successPercentage)
		})
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
	require.NotNil(f.T(), res, "request never succeeded")
	require.Truef(f.T(), ok, "expected 200 response code, got %d", res.StatusCode)
}
