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

package gateway

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/stretchr/testify/require"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	gatewayapi_v1 "sigs.k8s.io/gateway-api/apis/v1"

	contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/gatewayapi"
	"github.com/projectcontour/contour/pkg/config"
	"github.com/projectcontour/contour/test/e2e"
)

var f = e2e.NewFramework(false)

func TestGatewayAPI(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Gateway API tests")
}

var _ = BeforeSuite(func() {
	require.NoError(f.T(), f.Deployment.EnsureResourcesForLocalContour())
})

var _ = AfterSuite(func() {
	// Delete resources individually instead of deleting the entire contour
	// namespace as a performance optimization, because deleting non-empty
	// namespaces can take up to a couple minutes to complete.
	require.NoError(f.T(), f.Deployment.DeleteResourcesForLocalContour())
	gexec.CleanupBuildArtifacts()
})

var _ = Describe("Gateway API", func() {
	var (
		contourCmd            *gexec.Session
		contourConfig         *config.Parameters
		contourConfiguration  *contour_v1alpha1.ContourConfiguration
		contourConfigFile     string
		additionalContourArgs []string

		contourGatewayClass *gatewayapi_v1.GatewayClass
		contourGateway      *gatewayapi_v1.Gateway
	)

	// Creates specified gateway in namespace and runs namespaced test
	// body. Modifies contour config to point to gateway.
	testWithGateway := func(gateway *gatewayapi_v1.Gateway, gatewayClass *gatewayapi_v1.GatewayClass, body e2e.NamespacedGatewayTestBody) e2e.NamespacedTestBody {
		return func(namespace string) {
			Context(fmt.Sprintf("with gateway %s/%s, controllerName: %s", namespace, gateway.Name, gatewayClass.Spec.ControllerName), func() {
				BeforeEach(func() {
					// Ensure gateway created in this test's namespace.
					gateway.Namespace = namespace
					// Update contour config to point to specified gateway.
					contourConfig.GatewayConfig = &config.GatewayParameters{
						GatewayRef: config.NamespacedName{
							Namespace: gateway.Namespace,
							Name:      gateway.Name,
						},
					}

					// Update contour configuration to point to specified gateway.
					contourConfiguration.Spec.Gateway = &contour_v1alpha1.GatewayConfig{
						GatewayRef: contour_v1alpha1.NamespacedName{
							Namespace: gateway.Namespace,
							Name:      gateway.Name,
						},
					}

					contourGatewayClass = gatewayClass
					contourGateway = gateway
				})
				AfterEach(func() {
					require.NoError(f.T(), f.DeleteGateway(gateway, false))
				})

				body(namespace, types.NamespacedName{Namespace: namespace, Name: gateway.Name})
			})
		}
	}

	BeforeEach(func() {
		// Contour config file contents, can be modified in nested
		// BeforeEach.
		contourConfig = e2e.DefaultContourConfigFileParams()

		// Contour configuration crd, can be modified in nested
		// BeforeEach.
		contourConfiguration = e2e.DefaultContourConfiguration()

		// Default contour serve command line arguments can be appended to in
		// nested BeforeEach.
		additionalContourArgs = []string{}
	})

	// JustBeforeEach is called after each of the nested BeforeEach are
	// called, so it is a final setup step before running a test.
	// A nested BeforeEach may have modified Contour config, so we wait
	// until here to start Contour.
	JustBeforeEach(func() {
		var err error
		contourCmd, contourConfigFile, err = f.Deployment.StartLocalContour(contourConfig, contourConfiguration, additionalContourArgs...)
		require.NoError(f.T(), err)

		// Wait for Envoy to be healthy.
		require.NoError(f.T(), f.Deployment.WaitForEnvoyUpdated())

		// Since we're reconciling a specific Gateway,
		// we don't expect GatewayClasses to be reconciled
		// or become valid.
		gatewayClassCond := func(*gatewayapi_v1.GatewayClass) bool { return true }

		require.True(f.T(), f.CreateGatewayClassAndWaitFor(contourGatewayClass, gatewayClassCond))
		require.True(f.T(), f.CreateGatewayAndWaitFor(contourGateway, e2e.GatewayProgrammed))
	})

	AfterEach(func() {
		require.NoError(f.T(), f.DeleteGatewayClass(contourGatewayClass, false))
		require.NoError(f.T(), f.Deployment.StopLocalContour(contourCmd, contourConfigFile))
	})

	Describe("Gateway with one HTTP listener", func() {
		testWithHTTPGateway := func(body e2e.NamespacedGatewayTestBody) e2e.NamespacedTestBody {
			gatewayClass := getGatewayClass()
			gw := &gatewayapi_v1.Gateway{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: "http",
				},
				Spec: gatewayapi_v1.GatewaySpec{
					GatewayClassName: gatewayapi_v1.ObjectName(gatewayClass.Name),
					Listeners: []gatewayapi_v1.Listener{
						{
							Name:     "http",
							Protocol: gatewayapi_v1.HTTPProtocolType,
							Port:     gatewayapi_v1.PortNumber(80),
							AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
								Namespaces: &gatewayapi_v1.RouteNamespaces{
									From: ptr.To(gatewayapi_v1.NamespacesFromSame),
								},
							},
						},
					},
				},
			}

			return testWithGateway(gw, gatewayClass, body)
		}

		f.NamespacedTest("gateway-query-param-match", testWithHTTPGateway(testGatewayMultipleQueryParamMatch))

		f.NamespacedTest("gateway-request-header-modifier-backendref-filter", testWithHTTPGateway(testRequestHeaderModifierBackendRef))

		f.NamespacedTest("gateway-response-header-modifier-backendref-filter", testWithHTTPGateway(testResponseHeaderModifierBackendRef))

		f.NamespacedTest("gateway-host-rewrite", testWithHTTPGateway(testHostRewrite))

		f.NamespacedTest("gateway-request-redirect-rule", testWithHTTPGateway(testRequestRedirectRule))

		f.NamespacedTest("gateway-backend-tls-policy", testWithHTTPGateway(testBackendTLSPolicy))

		f.NamespacedTest("gateway-httproute-conflict-match", testWithHTTPGateway(testHTTPRouteConflictMatch))

		f.NamespacedTest("gateway-httproute-partially-conflict-match", testWithHTTPGateway(testHTTPRoutePartiallyConflictMatch))

		f.NamespacedTest("gateway-grpcroute-conflict-match", testWithHTTPGateway(testGRPCRouteConflictMatch))

		f.NamespacedTest("gateway-grpcroute-partially-conflict-match", testWithHTTPGateway(testGRPCRoutePartiallyConflictMatch))
	})

	Describe("Gateway with one HTTP listener and one HTTPS listener", func() {
		testWithHTTPSGateway := func(hostname string, body e2e.NamespacedGatewayTestBody) e2e.NamespacedTestBody {
			gatewayClass := getGatewayClass()

			gw := &gatewayapi_v1.Gateway{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: "https",
				},
				Spec: gatewayapi_v1.GatewaySpec{
					GatewayClassName: gatewayapi_v1.ObjectName(gatewayClass.Name),
					Listeners: []gatewayapi_v1.Listener{
						{
							Name:     "insecure",
							Protocol: gatewayapi_v1.HTTPProtocolType,
							Port:     gatewayapi_v1.PortNumber(80),
							AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
								Kinds: []gatewayapi_v1.RouteGroupKind{
									{Kind: "HTTPRoute"},
								},
								Namespaces: &gatewayapi_v1.RouteNamespaces{
									From: ptr.To(gatewayapi_v1.NamespacesFromSame),
								},
							},
						},
						{
							Name:     "secure",
							Protocol: gatewayapi_v1.HTTPSProtocolType,
							Port:     gatewayapi_v1.PortNumber(443),
							TLS: &gatewayapi_v1.GatewayTLSConfig{
								CertificateRefs: []gatewayapi_v1.SecretObjectReference{
									gatewayapi.CertificateRef("tlscert", ""),
								},
							},
							AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
								Kinds: []gatewayapi_v1.RouteGroupKind{
									{Kind: "HTTPRoute"},
								},
								Namespaces: &gatewayapi_v1.RouteNamespaces{
									From: ptr.To(gatewayapi_v1.NamespacesFromSame),
								},
							},
						},
					},
				},
			}
			return testWithGateway(gw, gatewayClass, func(namespace string, gateway types.NamespacedName) {
				Context(fmt.Sprintf("with TLS secret %s/tlscert for hostname %s", namespace, hostname), func() {
					BeforeEach(func() {
						f.Certs.CreateSelfSignedCert(namespace, "tlscert", "tlscert", hostname)
					})

					body(namespace, gateway)
				})
			})
		}

		f.NamespacedTest("gateway-httproute-tls-gateway", testWithHTTPSGateway("tls-gateway.projectcontour.io", testTLSGateway))

		f.NamespacedTest("gateway-httproute-tls-wildcard-host", testWithHTTPSGateway("*.wildcardhost.gateway.projectcontour.io", testTLSWildcardHost))
	})

	Describe("Gateway with multiple HTTPS listeners, each with a different hostname and TLS cert", func() {
		testWithMultipleHTTPSListenersGateway := func(body e2e.NamespacedTestBody) e2e.NamespacedTestBody {
			gatewayClass := getGatewayClass()
			gateway := &gatewayapi_v1.Gateway{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: "multiple-https-listeners",
				},
				Spec: gatewayapi_v1.GatewaySpec{
					GatewayClassName: gatewayapi_v1.ObjectName(gatewayClass.Name),
					Listeners: []gatewayapi_v1.Listener{
						{
							Name:     "https-1",
							Protocol: gatewayapi_v1.HTTPSProtocolType,
							Port:     gatewayapi_v1.PortNumber(443),
							Hostname: ptr.To(gatewayapi_v1.Hostname("https-1.gateway.projectcontour.io")),
							TLS: &gatewayapi_v1.GatewayTLSConfig{
								CertificateRefs: []gatewayapi_v1.SecretObjectReference{
									gatewayapi.CertificateRef("tlscert-1", ""),
								},
							},
							AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
								Kinds: []gatewayapi_v1.RouteGroupKind{
									{Kind: "HTTPRoute"},
								},
								Namespaces: &gatewayapi_v1.RouteNamespaces{
									From: ptr.To(gatewayapi_v1.NamespacesFromSame),
								},
							},
						},
						{
							Name:     "https-2",
							Protocol: gatewayapi_v1.HTTPSProtocolType,
							Port:     gatewayapi_v1.PortNumber(443),
							Hostname: ptr.To(gatewayapi_v1.Hostname("https-2.gateway.projectcontour.io")),
							TLS: &gatewayapi_v1.GatewayTLSConfig{
								CertificateRefs: []gatewayapi_v1.SecretObjectReference{
									gatewayapi.CertificateRef("tlscert-2", ""),
								},
							},
							AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
								Kinds: []gatewayapi_v1.RouteGroupKind{
									{Kind: "HTTPRoute"},
								},
								Namespaces: &gatewayapi_v1.RouteNamespaces{
									From: ptr.To(gatewayapi_v1.NamespacesFromSame),
								},
							},
						},
						{
							Name:     "https-3",
							Protocol: gatewayapi_v1.HTTPSProtocolType,
							Port:     gatewayapi_v1.PortNumber(443),
							Hostname: ptr.To(gatewayapi_v1.Hostname("https-3.gateway.projectcontour.io")),
							TLS: &gatewayapi_v1.GatewayTLSConfig{
								CertificateRefs: []gatewayapi_v1.SecretObjectReference{
									gatewayapi.CertificateRef("tlscert-3", ""),
								},
							},
							AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
								Kinds: []gatewayapi_v1.RouteGroupKind{
									{Kind: "HTTPRoute"},
								},
								Namespaces: &gatewayapi_v1.RouteNamespaces{
									From: ptr.To(gatewayapi_v1.NamespacesFromSame),
								},
							},
						},
					},
				},
			}

			return testWithGateway(gateway, gatewayClass, func(namespace string, _ types.NamespacedName) {
				BeforeEach(func() {
					f.Certs.CreateSelfSignedCert(namespace, "tlscert-1", "tlscert-1", "https-1.gateway.projectcontour.io")
					f.Certs.CreateSelfSignedCert(namespace, "tlscert-2", "tlscert-2", "https-2.gateway.projectcontour.io")
					f.Certs.CreateSelfSignedCert(namespace, "tlscert-3", "tlscert-3", "https-3.gateway.projectcontour.io")
				})

				body(namespace)
			})
		}

		f.NamespacedTest("gateway-multiple-https-listeners", testWithMultipleHTTPSListenersGateway(testMultipleHTTPSListeners))
	})

	Describe("Gateway with TCP listener", func() {
		testWithTCPGateway := func(body e2e.NamespacedGatewayTestBody) e2e.NamespacedTestBody {
			gatewayClass := getGatewayClass()
			gw := &gatewayapi_v1.Gateway{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: "tcp",
				},
				Spec: gatewayapi_v1.GatewaySpec{
					GatewayClassName: gatewayapi_v1.ObjectName(gatewayClass.Name),
					Listeners: []gatewayapi_v1.Listener{
						{
							Name:     "tcp",
							Protocol: gatewayapi_v1.TCPProtocolType,
							Port:     gatewayapi_v1.PortNumber(80),
							AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
								Namespaces: &gatewayapi_v1.RouteNamespaces{
									From: ptr.To(gatewayapi_v1.NamespacesFromSame),
								},
							},
						},
					},
				},
			}

			return testWithGateway(gw, gatewayClass, body)
		}

		f.NamespacedTest("gateway-tcproute", testWithTCPGateway(testTCPRoute))
	})
})

func getRandomNumber() int64 {
	nBig, err := rand.Int(rand.Reader, big.NewInt(10000))
	if err != nil {
		panic(err)
	}
	return nBig.Int64()
}

func getGatewayClass() *gatewayapi_v1.GatewayClass {
	randNumber := getRandomNumber()

	return &gatewayapi_v1.GatewayClass{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: fmt.Sprintf("contour-class-%d", randNumber),
		},
		Spec: gatewayapi_v1.GatewayClassSpec{
			ControllerName: gatewayapi_v1.GatewayController(fmt.Sprintf("projectcontour.io/ingress-controller-%d", randNumber)),
		},
	}
}
