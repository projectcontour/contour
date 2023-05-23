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

package upgrade

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/k8s"
	"github.com/projectcontour/contour/internal/ref"
	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayapi_v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

var (
	f = e2e.NewFramework(true)

	// Contour version we are upgrading from.
	contourUpgradeFromVersion string
)

func TestUpgrade(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Upgrade Suite")
}

var _ = BeforeSuite(func() {
	contourUpgradeFromVersion = os.Getenv("CONTOUR_UPGRADE_FROM_VERSION")
	require.NotEmpty(f.T(), contourUpgradeFromVersion, "CONTOUR_UPGRADE_FROM_VERSION environment variable not supplied")
	By("Testing upgrades from " + contourUpgradeFromVersion)
})

var _ = Describe("When upgrading", func() {
	Describe("Contour", func() {
		BeforeEach(func() {
			cmd := exec.Command("../../scripts/install-contour-release.sh", contourUpgradeFromVersion)
			sess, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
			require.NoError(f.T(), err)

			Eventually(sess, f.RetryTimeout, f.RetryInterval).Should(gexec.Exit(0))

			// We should be running in a multi-node cluster with a proper load
			// balancer, so fetch load balancer ip to make requests to.
			require.NoError(f.T(), f.Client.Get(context.TODO(), client.ObjectKeyFromObject(f.Deployment.EnvoyService), f.Deployment.EnvoyService))
			require.Greater(f.T(), len(f.Deployment.EnvoyService.Status.LoadBalancer.Ingress), 0)
			require.NotEmpty(f.T(), f.Deployment.EnvoyService.Status.LoadBalancer.Ingress[0].IP)
			f.HTTP.HTTPURLBase = "http://" + f.Deployment.EnvoyService.Status.LoadBalancer.Ingress[0].IP
			f.HTTP.HTTPSURLBase = "https://" + f.Deployment.EnvoyService.Status.LoadBalancer.Ingress[0].IP
		})

		AfterEach(func() {
			require.NoError(f.T(), f.Deployment.DeleteResourcesForInclusterContour())
		})

		const appHost = "upgrade-echo.test.com"

		f.NamespacedTest("contour-upgrade-test", func(namespace string) {
			Specify("applications remain routable after the upgrade", func() {
				By("deploying an app")
				f.Fixtures.Echo.DeployN(namespace, "echo", 2)
				p := &contourv1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: namespace,
						Name:      "echo",
					},
					Spec: contourv1.HTTPProxySpec{
						VirtualHost: &contourv1.VirtualHost{
							Fqdn: appHost,
						},
						Routes: []contourv1.Route{
							{
								Services: []contourv1.Service{
									{
										Name: "echo",
										Port: 80,
										ResponseHeadersPolicy: &contourv1.HeadersPolicy{
											Set: []contourv1.HeaderValue{
												{
													Name:  "X-Envoy-Response-Flags",
													Value: "%RESPONSE_FLAGS%",
												},
											},
										},
									},
								},
							},
						},
					},
				}
				require.NoError(f.T(), f.Client.Create(context.TODO(), p))

				By("ensuring it is routable")
				checkRoutability(appHost)

				poller, err := e2e.StartAppPoller(f.HTTP.HTTPURLBase, appHost, http.StatusOK, GinkgoWriter)
				require.NoError(f.T(), err)

				By("deploying updated contour resources")
				require.NoError(f.T(), f.Deployment.EnsureResourcesForInclusterContour(true))

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

	var _ = Describe("the Gateway provisioner", func() {
		const gatewayClassName = "upgrade-gc"

		BeforeEach(func() {
			cmd := exec.Command("../../scripts/install-provisioner-release.sh", contourUpgradeFromVersion)
			sess, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
			require.NoError(f.T(), err)

			Eventually(sess, f.RetryTimeout, f.RetryInterval).Should(gexec.Exit(0))

			gc, ok := f.CreateGatewayClassAndWaitFor(&gatewayapi_v1beta1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: gatewayClassName,
				},
				Spec: gatewayapi_v1beta1.GatewayClassSpec{
					ControllerName: gatewayapi_v1beta1.GatewayController("projectcontour.io/gateway-controller"),
				},
			}, e2e.GatewayClassAccepted)

			require.True(f.T(), ok)
			require.NotNil(f.T(), gc)
		})

		AfterEach(func() {
			require.NoError(f.T(), f.Provisioner.DeleteResourcesForInclusterProvisioner())

			gc := &gatewayapi_v1beta1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: gatewayClassName,
				},
			}

			require.NoError(f.T(), f.Client.Delete(context.Background(), gc))
		})

		f.NamespacedTest("provisioner-upgrade-test", func(namespace string) {
			Specify("provisioner upgrade test", func() {
				t := f.T()

				appHost := "upgrade.provisioner.projectcontour.io"

				gateway, ok := f.CreateGatewayAndWaitFor(&gatewayapi_v1beta1.Gateway{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: namespace,
						Name:      "upgrade-gateway",
					},
					Spec: gatewayapi_v1beta1.GatewaySpec{
						GatewayClassName: gatewayClassName,
						Listeners: []gatewayapi_v1beta1.Listener{
							{
								Name:     "http",
								Port:     gatewayapi_v1beta1.PortNumber(80),
								Protocol: gatewayapi_v1beta1.HTTPProtocolType,
								Hostname: ref.To(gatewayapi_v1beta1.Hostname(appHost)),
							},
						},
					},
				}, func(gw *gatewayapi_v1beta1.Gateway) bool {
					return e2e.GatewayProgrammed(gw) && e2e.GatewayHasAddress(gw)
				})
				require.True(t, ok)
				require.NotNil(t, gateway)

				f.HTTP.HTTPURLBase = "http://" + gateway.Status.Addresses[0].Value

				f.Fixtures.Echo.DeployN(namespace, "echo", 2)

				f.CreateHTTPRouteAndWaitFor(&gatewayapi_v1beta1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: namespace,
						Name:      "echo",
					},
					Spec: gatewayapi_v1beta1.HTTPRouteSpec{
						CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
							ParentRefs: []gatewayapi_v1beta1.ParentReference{
								{Name: gatewayapi_v1beta1.ObjectName(gateway.Name)},
							},
						},
						Rules: []gatewayapi_v1beta1.HTTPRouteRule{
							{
								BackendRefs: []gatewayapi_v1beta1.HTTPBackendRef{
									{
										BackendRef: gatewayapi_v1beta1.BackendRef{
											BackendObjectReference: gatewayapi_v1beta1.BackendObjectReference{
												Name: gatewayapi_v1beta1.ObjectName("echo"),
												Port: ref.To(gatewayapi_v1beta1.PortNumber(80)),
											},
										},
									},
								},
								Filters: []gatewayapi_v1beta1.HTTPRouteFilter{
									{
										Type: gatewayapi_v1beta1.HTTPRouteFilterResponseHeaderModifier,
										ResponseHeaderModifier: &gatewayapi_v1beta1.HTTPHeaderFilter{
											Set: []gatewayapi_v1beta1.HTTPHeader{
												{
													Name:  gatewayapi_v1beta1.HTTPHeaderName("X-Envoy-Response-Flags"),
													Value: "%RESPONSE_FLAGS%",
												},
											},
										},
									},
								},
							},
						},
					},
				}, e2e.HTTPRouteAccepted)

				By("ensuring it is routable")
				checkRoutability(appHost)

				poller, err := e2e.StartAppPoller(f.HTTP.HTTPURLBase, appHost, http.StatusOK, GinkgoWriter)
				require.NoError(f.T(), err)

				By("deploying updated provisioner")
				require.NoError(f.T(), f.Provisioner.EnsureResourcesForInclusterProvisioner())

				By("waiting for Gateway's Contour deployment to upgrade")
				deployment := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: namespace,
						Name:      fmt.Sprintf("contour-%s", gateway.Name),
					},
				}
				require.NoError(t, f.Client.Get(context.Background(), k8s.NamespacedNameOf(deployment), deployment))
				require.NoError(t, e2e.WaitForContourDeploymentUpdated(deployment, f.Client, os.Getenv("CONTOUR_E2E_IMAGE")))

				By("waiting for Gateway's Envoy daemonset to upgrade")
				daemonset := &appsv1.DaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: namespace,
						Name:      fmt.Sprintf("envoy-%s", gateway.Name),
					},
				}
				require.NoError(t, f.Client.Get(context.Background(), k8s.NamespacedNameOf(daemonset), daemonset))
				require.NoError(t, e2e.WaitForEnvoyDaemonSetUpdated(daemonset, f.Client, os.Getenv("CONTOUR_E2E_IMAGE")))

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
})

func checkRoutability(host string) {
	res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
		Host:      host,
		Path:      "/echo",
		Condition: e2e.HasStatusCode(200),
	})
	require.NotNil(f.T(), res, "request never succeeded")
	require.Truef(f.T(), ok, "expected 200 response code, got %d", res.StatusCode)
}
