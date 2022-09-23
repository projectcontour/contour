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

package gateway

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"testing"

	contour_api_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/projectcontour/contour/internal/gatewayapi"
	"github.com/projectcontour/contour/pkg/config"
	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayapi_v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

// ReconcileModeController means Contour should be configured
// to reconcile based on a Gateway controller string.
const ReconcileModeController = "controller"

// ReconcileModeGateway means Contour should be configured
// to reconcile a specific named Gateway.
const ReconcileModeGateway = "gateway"

var f = e2e.NewFramework(false)
var reconcileMode = ReconcileModeController

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
	// Run all tests for both gateway reconciliation modes.
	for _, mode := range []string{ReconcileModeController, ReconcileModeGateway} {
		reconcileMode = mode

		Context(fmt.Sprintf("Reconcile mode %s", mode), func() {
			runGatewayTests()
		})
	}
})

func runGatewayTests() {
	var (
		contourCmd            *gexec.Session
		contourConfig         *config.Parameters
		contourConfiguration  *contour_api_v1alpha1.ContourConfiguration
		contourConfigFile     string
		additionalContourArgs []string

		contourGatewayClass *gatewayapi_v1beta1.GatewayClass
		contourGateway      *gatewayapi_v1beta1.Gateway
	)

	// Creates specified gateway in namespace and runs namespaced test
	// body. Modifies contour config to point to gateway.
	testWithGateway := func(gateway *gatewayapi_v1beta1.Gateway, gatewayClass *gatewayapi_v1beta1.GatewayClass, body e2e.NamespacedGatewayTestBody) e2e.NamespacedTestBody {
		return func(namespace string) {
			Context(fmt.Sprintf("with gateway %s/%s, controllerName: %s", namespace, gateway.Name, gatewayClass.Spec.ControllerName), func() {
				BeforeEach(func() {
					// Ensure gateway created in this test's namespace.
					gateway.Namespace = namespace
					// Update contour config to point to specified gateway.
					contourConfig.GatewayConfig = &config.GatewayParameters{}
					if reconcileMode == ReconcileModeGateway {
						contourConfig.GatewayConfig.GatewayRef = &config.NamespacedName{
							Namespace: gateway.Namespace,
							Name:      gateway.Name,
						}
					} else {
						contourConfig.GatewayConfig.ControllerName = string(gatewayClass.Spec.ControllerName)
					}

					// Update contour configuration to point to specified gateway.
					contourConfiguration.Spec.Gateway = &contour_api_v1alpha1.GatewayConfig{}
					if reconcileMode == ReconcileModeGateway {
						contourConfiguration.Spec.Gateway.GatewayRef = &contour_api_v1alpha1.NamespacedName{
							Namespace: gateway.Namespace,
							Name:      gateway.Name,
						}
					} else {
						contourConfiguration.Spec.Gateway.ControllerName = string(gatewayClass.Spec.ControllerName)
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

		gatewayClassCond := gatewayClassValid
		// If we're reconciling a specific Gateway,
		// we don't expect GatewayClasses to be reconciled
		// or become valid.
		if reconcileMode == ReconcileModeGateway {
			gatewayClassCond = func(*gatewayapi_v1beta1.GatewayClass) bool { return true }
		}

		f.CreateGatewayClassAndWaitFor(contourGatewayClass, gatewayClassCond)
		f.CreateGatewayAndWaitFor(contourGateway, gatewayValid)
	})

	AfterEach(func() {
		require.NoError(f.T(), f.DeleteGatewayClass(contourGatewayClass, false))
		require.NoError(f.T(), f.Deployment.StopLocalContour(contourCmd, contourConfigFile))
	})

	Describe("Gateway with one HTTP listener", func() {
		testWithHTTPGateway := func(body e2e.NamespacedGatewayTestBody) e2e.NamespacedTestBody {
			gatewayClass := getGatewayClass()
			gw := &gatewayapi_v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name: "http",
				},
				Spec: gatewayapi_v1beta1.GatewaySpec{
					GatewayClassName: gatewayapi_v1beta1.ObjectName(gatewayClass.Name),
					Listeners: []gatewayapi_v1beta1.Listener{
						{
							Name:     "http",
							Protocol: gatewayapi_v1beta1.HTTPProtocolType,
							Port:     gatewayapi_v1beta1.PortNumber(80),
							AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
								Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
									From: gatewayapi.FromNamespacesPtr(gatewayapi_v1beta1.NamespacesFromSame),
								},
							},
						},
					},
				},
			}

			return testWithGateway(gw, gatewayClass, body)
		}

		f.NamespacedTest("gateway-path-condition-match", testWithHTTPGateway(testGatewayPathConditionMatch))

		f.NamespacedTest("gateway-header-condition-match", testWithHTTPGateway(testGatewayHeaderConditionMatch))

		f.NamespacedTest("gateway-query-param-match", testWithHTTPGateway(testGatewayQueryParamMatch))

		f.NamespacedTest("gateway-invalid-forward-to", testWithHTTPGateway(testInvalidBackendRef))

		f.NamespacedTest("gateway-request-header-modifier-forward-to", testWithHTTPGateway(testRequestHeaderModifierForwardTo))

		f.NamespacedTest("gateway-request-header-modifier-rule", testWithHTTPGateway(testRequestHeaderModifierRule))

		f.NamespacedTest("gateway-host-rewrite", testWithHTTPGateway(testHostRewrite))

		f.NamespacedTest("gateway-route-parent-refs", testWithHTTPGateway(testRouteParentRefs))

		f.NamespacedTest("gateway-request-redirect-rule", testWithHTTPGateway(testRequestRedirectRule))

		f.NamespacedTest("gateway-request-mirror-rule", testWithHTTPGateway(testRequestMirrorRule))
	})

	Describe("Gateway with one HTTP listener and one HTTPS listener", func() {
		testWithHTTPSGateway := func(hostname string, body e2e.NamespacedGatewayTestBody) e2e.NamespacedTestBody {
			gatewayClass := getGatewayClass()

			gw := &gatewayapi_v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name: "https",
				},
				Spec: gatewayapi_v1beta1.GatewaySpec{
					GatewayClassName: gatewayapi_v1beta1.ObjectName(gatewayClass.Name),
					Listeners: []gatewayapi_v1beta1.Listener{
						{
							Name:     "insecure",
							Protocol: gatewayapi_v1beta1.HTTPProtocolType,
							Port:     gatewayapi_v1beta1.PortNumber(80),
							AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
								Kinds: []gatewayapi_v1beta1.RouteGroupKind{
									{Kind: "HTTPRoute"},
								},
								Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
									From: gatewayapi.FromNamespacesPtr(gatewayapi_v1beta1.NamespacesFromSame),
								},
							},
						},
						{
							Name:     "secure",
							Protocol: gatewayapi_v1beta1.HTTPSProtocolType,
							Port:     gatewayapi_v1beta1.PortNumber(443),
							TLS: &gatewayapi_v1beta1.GatewayTLSConfig{
								CertificateRefs: []gatewayapi_v1beta1.SecretObjectReference{
									gatewayapi.CertificateRef("tlscert", ""),
								},
							},
							AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
								Kinds: []gatewayapi_v1beta1.RouteGroupKind{
									{Kind: "HTTPRoute"},
								},
								Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
									From: gatewayapi.FromNamespacesPtr(gatewayapi_v1beta1.NamespacesFromSame),
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

	Describe("Gateway with one TLS listener with Mode=Passthrough", func() {
		gatewayClass := getGatewayClass()
		gw := &gatewayapi_v1beta1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name: "tls-passthrough",
			},
			Spec: gatewayapi_v1beta1.GatewaySpec{
				GatewayClassName: gatewayapi_v1beta1.ObjectName(gatewayClass.Name),
				Listeners: []gatewayapi_v1beta1.Listener{
					{
						Name:     "tls-passthrough",
						Protocol: gatewayapi_v1beta1.TLSProtocolType,
						Port:     gatewayapi_v1beta1.PortNumber(443),
						TLS: &gatewayapi_v1beta1.GatewayTLSConfig{
							Mode: gatewayapi.TLSModeTypePtr(gatewayapi_v1beta1.TLSModePassthrough),
						},
						AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
							Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
								From: gatewayapi.FromNamespacesPtr(gatewayapi_v1beta1.NamespacesFromSame),
							},
						},
					},
				},
			},
		}
		f.NamespacedTest("gateway-tlsroute-mode-passthrough", testWithGateway(gw, gatewayClass, testTLSRoutePassthrough))
	})

	Describe("Gateway with multiple HTTPS listeners, each with a different hostname and TLS cert", func() {
		testWithMultipleHTTPSListenersGateway := func(body e2e.NamespacedTestBody) e2e.NamespacedTestBody {
			gatewayClass := getGatewayClass()
			gateway := &gatewayapi_v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name: "multiple-https-listeners",
				},
				Spec: gatewayapi_v1beta1.GatewaySpec{
					GatewayClassName: gatewayapi_v1beta1.ObjectName(gatewayClass.Name),
					Listeners: []gatewayapi_v1beta1.Listener{
						{
							Name:     "https-1",
							Protocol: gatewayapi_v1beta1.HTTPSProtocolType,
							Port:     gatewayapi_v1beta1.PortNumber(443),
							Hostname: gatewayapi.ListenerHostname("https-1.gateway.projectcontour.io"),
							TLS: &gatewayapi_v1beta1.GatewayTLSConfig{
								CertificateRefs: []gatewayapi_v1beta1.SecretObjectReference{
									gatewayapi.CertificateRef("tlscert-1", ""),
								},
							},
							AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
								Kinds: []gatewayapi_v1beta1.RouteGroupKind{
									{Kind: "HTTPRoute"},
								},
								Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
									From: gatewayapi.FromNamespacesPtr(gatewayapi_v1beta1.NamespacesFromSame),
								},
							},
						},
						{
							Name:     "https-2",
							Protocol: gatewayapi_v1beta1.HTTPSProtocolType,
							Port:     gatewayapi_v1beta1.PortNumber(443),
							Hostname: gatewayapi.ListenerHostname("https-2.gateway.projectcontour.io"),
							TLS: &gatewayapi_v1beta1.GatewayTLSConfig{
								CertificateRefs: []gatewayapi_v1beta1.SecretObjectReference{
									gatewayapi.CertificateRef("tlscert-2", ""),
								},
							},
							AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
								Kinds: []gatewayapi_v1beta1.RouteGroupKind{
									{Kind: "HTTPRoute"},
								},
								Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
									From: gatewayapi.FromNamespacesPtr(gatewayapi_v1beta1.NamespacesFromSame),
								},
							},
						},
						{
							Name:     "https-3",
							Protocol: gatewayapi_v1beta1.HTTPSProtocolType,
							Port:     gatewayapi_v1beta1.PortNumber(443),
							Hostname: gatewayapi.ListenerHostname("https-3.gateway.projectcontour.io"),
							TLS: &gatewayapi_v1beta1.GatewayTLSConfig{
								CertificateRefs: []gatewayapi_v1beta1.SecretObjectReference{
									gatewayapi.CertificateRef("tlscert-3", ""),
								},
							},
							AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
								Kinds: []gatewayapi_v1beta1.RouteGroupKind{
									{Kind: "HTTPRoute"},
								},
								Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
									From: gatewayapi.FromNamespacesPtr(gatewayapi_v1beta1.NamespacesFromSame),
								},
							},
						},
					},
				},
			}

			return testWithGateway(gateway, gatewayClass, func(namespace string, gateway types.NamespacedName) {
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
}

// httpRouteAccepted returns true if the route has a .status.conditions
// entry of "Accepted: true".
func httpRouteAccepted(route *gatewayapi_v1beta1.HTTPRoute) bool {
	if route == nil {
		return false
	}

	for _, gw := range route.Status.Parents {
		for _, cond := range gw.Conditions {
			if cond.Type == string(gatewayapi_v1beta1.RouteConditionAccepted) && cond.Status == metav1.ConditionTrue {
				return true
			}
		}
	}

	return false
}

// tlsRouteAccepted returns true if the route has a .status.conditions
// entry of "Accepted: true".
func tlsRouteAccepted(route *gatewayapi_v1alpha2.TLSRoute) bool {
	if route == nil {
		return false
	}

	for _, gw := range route.Status.Parents {
		for _, cond := range gw.Conditions {
			if cond.Type == string(gatewayapi_v1alpha2.RouteConditionAccepted) && cond.Status == metav1.ConditionTrue {
				return true
			}
		}
	}

	return false
}

// gatewayValid returns true if the gateway has a .status.conditions
// entry of Ready: true".
func gatewayValid(gateway *gatewayapi_v1beta1.Gateway) bool {
	if gateway == nil {
		return false
	}

	for _, cond := range gateway.Status.Conditions {
		if cond.Type == string(gatewayapi_v1beta1.GatewayConditionReady) && cond.Status == metav1.ConditionTrue {
			return true
		}
	}

	return false
}

// gatewayClassValid returns true if the gateway has a .status.conditions
// entry of Accepted: true".
func gatewayClassValid(gatewayClass *gatewayapi_v1beta1.GatewayClass) bool {
	if gatewayClass == nil {
		return false
	}

	for _, cond := range gatewayClass.Status.Conditions {
		if cond.Type == string(gatewayapi_v1beta1.GatewayClassConditionStatusAccepted) && cond.Status == metav1.ConditionTrue {
			return true
		}
	}

	return false
}

func getRandomNumber() int64 {
	nBig, err := rand.Int(rand.Reader, big.NewInt(10000))
	if err != nil {
		panic(err)
	}
	return nBig.Int64()
}

func getGatewayClass() *gatewayapi_v1beta1.GatewayClass {
	randNumber := getRandomNumber()

	return &gatewayapi_v1beta1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("contour-class-%d", randNumber),
		},
		Spec: gatewayapi_v1beta1.GatewayClassSpec{
			ControllerName: gatewayapi_v1beta1.GatewayController(fmt.Sprintf("projectcontour.io/ingress-controller-%d", randNumber)),
		},
	}
}
