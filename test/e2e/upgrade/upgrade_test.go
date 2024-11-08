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
	"github.com/stretchr/testify/require"
	apps_v1 "k8s.io/api/apps/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayapi_v1 "sigs.k8s.io/gateway-api/apis/v1"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/k8s"
	"github.com/projectcontour/contour/test/e2e"
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
			require.NotEmpty(f.T(), f.Deployment.EnvoyService.Status.LoadBalancer.Ingress)
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
				p := &contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace: namespace,
						Name:      "echo",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: appHost,
						},
						Routes: []contour_v1.Route{
							{
								Services: []contour_v1.Service{
									{
										Name: "echo",
										Port: 80,
										ResponseHeadersPolicy: &contour_v1.HeadersPolicy{
											Set: []contour_v1.HeaderValue{
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
				f.T().Logf("Total requests: %d, successful requests: %d\n", totalRequests, successfulRequests)
				require.Positive(f.T(), totalRequests)
				successPercentage := 100 * float64(successfulRequests) / float64(totalRequests)
				require.Greaterf(f.T(), successPercentage, float64(90.0), "success rate of %.2f%% less than 90%%", successPercentage)
			})
		})
	})

	_ = Describe("the Gateway provisioner", func() {
		const gatewayClassName = "upgrade-gc"

		BeforeEach(func() {
			cmd := exec.Command("../../scripts/install-provisioner-release.sh", contourUpgradeFromVersion)
			sess, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
			require.NoError(f.T(), err)
			Eventually(sess, f.RetryTimeout, f.RetryInterval).Should(gexec.Exit(0))

			require.True(f.T(), f.CreateGatewayClassAndWaitFor(&gatewayapi_v1.GatewayClass{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: gatewayClassName,
				},
				Spec: gatewayapi_v1.GatewayClassSpec{
					ControllerName: gatewayapi_v1.GatewayController("projectcontour.io/gateway-controller"),
				},
			}, e2e.GatewayClassAccepted))
		})

		AfterEach(func() {
			require.NoError(f.T(), f.Provisioner.DeleteResourcesForInclusterProvisioner())

			gc := &gatewayapi_v1.GatewayClass{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: gatewayClassName,
				},
			}

			require.NoError(f.T(), f.Client.Delete(context.Background(), gc))
		})

		f.NamespacedTest("provisioner-upgrade-test", func(namespace string) {
			Specify("provisioner upgrade test", func() {
				t := f.T()

				appHost := "upgrade.provisioner.projectcontour.io"

				gateway := &gatewayapi_v1.Gateway{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace: namespace,
						Name:      "upgrade-gateway",
					},
					Spec: gatewayapi_v1.GatewaySpec{
						GatewayClassName: gatewayClassName,
						Listeners: []gatewayapi_v1.Listener{
							{
								Name:     "http",
								Port:     gatewayapi_v1.PortNumber(80),
								Protocol: gatewayapi_v1.HTTPProtocolType,
								Hostname: ptr.To(gatewayapi_v1.Hostname(appHost)),
							},
						},
					},
				}

				require.True(f.T(), f.CreateGatewayAndWaitFor(gateway, func(gw *gatewayapi_v1.Gateway) bool {
					return e2e.GatewayProgrammed(gw) && e2e.GatewayHasAddress(gw)
				}))

				require.NoError(f.T(), f.Client.Get(context.Background(), k8s.NamespacedNameOf(gateway), gateway))

				f.HTTP.HTTPURLBase = "http://" + gateway.Status.Addresses[0].Value

				f.Fixtures.Echo.DeployN(namespace, "echo", 2)

				require.True(f.T(), f.CreateHTTPRouteAndWaitFor(&gatewayapi_v1.HTTPRoute{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace: namespace,
						Name:      "echo",
					},
					Spec: gatewayapi_v1.HTTPRouteSpec{
						CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
							ParentRefs: []gatewayapi_v1.ParentReference{
								{Name: gatewayapi_v1.ObjectName(gateway.Name)},
							},
						},
						Rules: []gatewayapi_v1.HTTPRouteRule{
							{
								BackendRefs: []gatewayapi_v1.HTTPBackendRef{
									{
										BackendRef: gatewayapi_v1.BackendRef{
											BackendObjectReference: gatewayapi_v1.BackendObjectReference{
												Name: gatewayapi_v1.ObjectName("echo"),
												Port: ptr.To(gatewayapi_v1.PortNumber(80)),
											},
										},
									},
								},
								Filters: []gatewayapi_v1.HTTPRouteFilter{
									{
										Type: gatewayapi_v1.HTTPRouteFilterResponseHeaderModifier,
										ResponseHeaderModifier: &gatewayapi_v1.HTTPHeaderFilter{
											Set: []gatewayapi_v1.HTTPHeader{
												{
													Name:  gatewayapi_v1.HTTPHeaderName("X-Envoy-Response-Flags"),
													Value: "%RESPONSE_FLAGS%",
												},
											},
										},
									},
								},
							},
						},
					},
				}, e2e.HTTPRouteAccepted))

				By("ensuring it is routable")
				checkRoutability(appHost)

				poller, err := e2e.StartAppPoller(f.HTTP.HTTPURLBase, appHost, http.StatusOK, GinkgoWriter)
				require.NoError(f.T(), err)

				By("updating gateway-api CRDs to latest")
				// Delete existing BackendTLSPolicy CRD.
				// TODO: remove this hack once BackendTLSPolicy v1alpha3 or
				// above is available in multiple consecutive releases.
				cmd := exec.Command("kubectl", "delete", "crd", "backendtlspolicies.gateway.networking.k8s.io")
				sess, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
				require.NoError(f.T(), err)
				Eventually(sess, f.RetryTimeout, f.RetryInterval).Should(gexec.Exit(0))

				cmd = exec.Command("kubectl", "apply", "-f", "../../../examples/gateway/00-crds.yaml")
				sess, err = gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
				require.NoError(f.T(), err)
				Eventually(sess, f.RetryTimeout, f.RetryInterval).Should(gexec.Exit(0))

				By("deploying updated provisioner")
				require.NoError(f.T(), f.Provisioner.EnsureResourcesForInclusterProvisioner())

				By("waiting for Gateway's Contour deployment to upgrade")
				deployment := &apps_v1.Deployment{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace: namespace,
						Name:      fmt.Sprintf("contour-%s", gateway.Name),
					},
				}
				require.NoError(t, f.Client.Get(context.Background(), k8s.NamespacedNameOf(deployment), deployment))
				require.NoError(t, e2e.WaitForContourDeploymentUpdated(deployment, f.Client, os.Getenv("CONTOUR_E2E_IMAGE")))

				By("waiting for Gateway's Envoy daemonset to upgrade")
				daemonset := &apps_v1.DaemonSet{
					ObjectMeta: meta_v1.ObjectMeta{
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
				f.T().Logf("Total requests: %d, successful requests: %d\n", totalRequests, successfulRequests)
				require.Positive(f.T(), totalRequests)
				successPercentage := 100 * float64(successfulRequests) / float64(totalRequests)
				// Success threshold is somewhat arbitrary but less than the standalone
				// Contour upgrade threshold because the Gateway provisioner does not
				// currently fully upgrade the control plane before the data plane which
				// can lead to additional downtime when both are upgrading at the same
				// time.
				// ref. https://github.com/projectcontour/contour/issues/5375.
				require.Greaterf(f.T(), successPercentage, float64(80.0), "success rate of %.2f%% less than 80%%", successPercentage)
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
