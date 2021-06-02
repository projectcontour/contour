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

package gateway

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo"
	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

func TestGatewayAPI(t *testing.T) {
	RunSpecs(t, "Gateway API tests")
}

var _ = Describe("Gateway API", func() {
	var f *e2e.Framework

	BeforeEach(func() {
		f = e2e.NewFramework(GinkgoT())
	})

	Describe("Insecure (Non-TLS) Gateway", func() {
		var gateway *gatewayv1alpha1.Gateway

		// Note, this ends up creating the Gateway before each spec
		// case (and deleting it after) which is not really necessary
		// since all of these specs use the same Gateway. Consider
		// moving each unique Gateway into its own test suite and using
		// BeforeSuite/AfterSuit to create/delete the Gateway once, or
		// some other similar structure.
		BeforeEach(func() {
			gateway = &gatewayv1alpha1.Gateway{
				// Namespace and name need to match what's
				// configured in the Contour config file.
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "contour",
				},
				Spec: gatewayv1alpha1.GatewaySpec{
					GatewayClassName: "contour-class",
					Listeners: []gatewayv1alpha1.Listener{
						{
							Protocol: gatewayv1alpha1.HTTPProtocolType,
							Port:     gatewayv1alpha1.PortNumber(80),
							Routes: gatewayv1alpha1.RouteBindingSelector{
								Kind: "HTTPRoute",
								Namespaces: &gatewayv1alpha1.RouteNamespaces{
									From: routeSelectTypePtr(gatewayv1alpha1.RouteSelectAll),
								},
								Selector: &metav1.LabelSelector{
									MatchLabels: map[string]string{"app": "filter"},
								},
							},
						},
					},
				},
			}

			require.NoError(f.T(), f.Client.Create(context.TODO(), gateway))
		})

		AfterEach(func() {
			require.NoError(f.T(), f.Client.Delete(context.TODO(), gateway))
		})

		It("001-path-condition-match", func() {
			testGatewayPathConditionMatch(f)
		})

		It("002-header-condition-match", func() {
			testGatewayHeaderConditionMatch(f)
		})

		It("003-invalid-forward-to", func() {
			testInvalidForwardTo(f)
		})

		It("005-request-header-modifier-forward-to", func() {
			testRequestHeaderModifierForwardTo(f)
		})

		It("005-request-header-modifier-rule", func() {
			testRequestHeaderModifierRule(f)
		})

		It("006-host-rewrite", func() {
			testHostRewrite(f)
		})

		It("007-gateway-allow-type", func() {
			testGatewayAllowType(f)
		})
	})

	Describe("HTTPRoute: TLS Gateway", func() {
		var gateway *gatewayv1alpha1.Gateway
		var cleanupCert func()

		BeforeEach(func() {
			gateway = &gatewayv1alpha1.Gateway{
				// Namespace and name need to match what's
				// configured in the Contour config file.
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "contour",
				},
				Spec: gatewayv1alpha1.GatewaySpec{
					GatewayClassName: "contour-class",
					Listeners: []gatewayv1alpha1.Listener{
						{
							Protocol: gatewayv1alpha1.HTTPProtocolType,
							Port:     gatewayv1alpha1.PortNumber(80),
							Routes: gatewayv1alpha1.RouteBindingSelector{
								Kind: "HTTPRoute",
								Namespaces: &gatewayv1alpha1.RouteNamespaces{
									From: routeSelectTypePtr(gatewayv1alpha1.RouteSelectAll),
								},
								Selector: &metav1.LabelSelector{
									MatchLabels: map[string]string{"type": "insecure"},
								},
							},
						},
						{
							Protocol: gatewayv1alpha1.HTTPSProtocolType,
							Port:     gatewayv1alpha1.PortNumber(443),
							TLS: &gatewayv1alpha1.GatewayTLSConfig{
								CertificateRef: &gatewayv1alpha1.LocalObjectReference{
									Group: "core",
									Kind:  "Secret",
									Name:  "tlscert",
								},
							},
							Routes: gatewayv1alpha1.RouteBindingSelector{
								Kind: "HTTPRoute",
								Namespaces: &gatewayv1alpha1.RouteNamespaces{
									From: routeSelectTypePtr(gatewayv1alpha1.RouteSelectAll),
								},
								Selector: &metav1.LabelSelector{
									MatchLabels: map[string]string{"type": "secure"},
								},
							},
						},
					},
				},
			}

			require.NoError(f.T(), f.Client.Create(context.TODO(), gateway))
			cleanupCert = f.Certs.CreateSelfSignedCert("projectcontour", "tlscert", "tlscert", "tls-gateway.projectcontour.io")
		})

		AfterEach(func() {
			require.NoError(f.T(), f.Client.Delete(context.TODO(), gateway))
			cleanupCert()
		})

		It("004-httproute-tls-gateway", func() {
			testTLSGateway(f)
		})
	})

	Describe("TLSRoute: Gateway", func() {
		var gateway *gatewayv1alpha1.Gateway

		BeforeEach(func() {
			gateway = &gatewayv1alpha1.Gateway{
				// Namespace and name need to match what's
				// configured in the Contour config file.
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "contour",
				},
				Spec: gatewayv1alpha1.GatewaySpec{
					GatewayClassName: "contour-class",
					Listeners: []gatewayv1alpha1.Listener{
						{
							Protocol: gatewayv1alpha1.TLSProtocolType,
							Port:     gatewayv1alpha1.PortNumber(443),
							Routes: gatewayv1alpha1.RouteBindingSelector{
								Kind: "TLSRoute",
								Namespaces: &gatewayv1alpha1.RouteNamespaces{
									From: routeSelectTypePtr(gatewayv1alpha1.RouteSelectAll),
								},
							},
						},
					},
				},
			}

			require.NoError(f.T(), f.Client.Create(context.TODO(), gateway))
		})

		AfterEach(func() {
			require.NoError(f.T(), f.Client.Delete(context.TODO(), gateway))
		})

		It("008-tlsroute", func() {
			testTLSRoutePassthrough(f, "gateway-008-tlsroute")
		})
	})

	Describe("TLSRoute Gateway: Mode: Passthrough", func() {
		var gateway *gatewayv1alpha1.Gateway

		BeforeEach(func() {
			gateway = &gatewayv1alpha1.Gateway{
				// Namespace and name need to match what's
				// configured in the Contour config file.
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "contour",
				},
				Spec: gatewayv1alpha1.GatewaySpec{
					GatewayClassName: "contour-class",
					Listeners: []gatewayv1alpha1.Listener{
						{
							Protocol: gatewayv1alpha1.TLSProtocolType,
							Port:     gatewayv1alpha1.PortNumber(443),
							TLS: &gatewayv1alpha1.GatewayTLSConfig{
								Mode: tlsModeTypePtr(gatewayv1alpha1.TLSModePassthrough),
							},
							Routes: gatewayv1alpha1.RouteBindingSelector{
								Kind: "TLSRoute",
								Namespaces: &gatewayv1alpha1.RouteNamespaces{
									From: routeSelectTypePtr(gatewayv1alpha1.RouteSelectAll),
								},
							},
						},
					},
				},
			}

			require.NoError(f.T(), f.Client.Create(context.TODO(), gateway))
		})

		AfterEach(func() {
			require.NoError(f.T(), f.Client.Delete(context.TODO(), gateway))
		})

		It("008-tlsroute-mode-passthrough", func() {
			testTLSRoutePassthrough(f, "gateway-008-tlsroute-mode-passthrough")
		})
	})

	Describe("Wildcard TLS Gateway", func() {
		var gateway *gatewayv1alpha1.Gateway
		var cleanupCert func()

		BeforeEach(func() {
			gateway = &gatewayv1alpha1.Gateway{
				// Namespace and name need to match what's
				// configured in the Contour config file.
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "contour",
				},
				Spec: gatewayv1alpha1.GatewaySpec{
					GatewayClassName: "contour-class",
					Listeners: []gatewayv1alpha1.Listener{
						{
							Protocol: gatewayv1alpha1.HTTPProtocolType,
							Port:     gatewayv1alpha1.PortNumber(80),
							Routes: gatewayv1alpha1.RouteBindingSelector{
								Kind: "HTTPRoute",
								Namespaces: &gatewayv1alpha1.RouteNamespaces{
									From: routeSelectTypePtr(gatewayv1alpha1.RouteSelectAll),
								},
								Selector: &metav1.LabelSelector{
									MatchLabels: map[string]string{"type": "insecure"},
								},
							},
						},
						{
							Protocol: gatewayv1alpha1.HTTPSProtocolType,
							Port:     gatewayv1alpha1.PortNumber(443),
							TLS: &gatewayv1alpha1.GatewayTLSConfig{
								CertificateRef: &gatewayv1alpha1.LocalObjectReference{
									Group: "core",
									Kind:  "Secret",
									Name:  "tlscert",
								},
							},
							Routes: gatewayv1alpha1.RouteBindingSelector{
								Kind: "HTTPRoute",
								Namespaces: &gatewayv1alpha1.RouteNamespaces{
									From: routeSelectTypePtr(gatewayv1alpha1.RouteSelectAll),
								},
								Selector: &metav1.LabelSelector{
									MatchLabels: map[string]string{"type": "secure"},
								},
							},
						},
					},
				},
			}

			require.NoError(f.T(), f.Client.Create(context.TODO(), gateway))
			cleanupCert = f.Certs.CreateSelfSignedCert("projectcontour", "tlscert", "tlscert", "*.wildcardhost.gateway.projectcontour.io")
		})

		AfterEach(func() {
			require.NoError(f.T(), f.Client.Delete(context.TODO(), gateway))
			cleanupCert()
		})

		It("009-tls-wildcard-host", func() {
			testTLSWildcardHost(f)
		})
	})
})

func stringPtr(s string) *string {
	return &s
}

func portNumPtr(port int) *gatewayv1alpha1.PortNumber {
	pn := gatewayv1alpha1.PortNumber(port)
	return &pn
}

func routeSelectTypePtr(val gatewayv1alpha1.RouteSelectType) *gatewayv1alpha1.RouteSelectType {
	return &val
}

func pathMatchTypePtr(val gatewayv1alpha1.PathMatchType) *gatewayv1alpha1.PathMatchType {
	return &val
}

func headerMatchTypePtr(val gatewayv1alpha1.HeaderMatchType) *gatewayv1alpha1.HeaderMatchType {
	return &val
}

func gatewayAllowTypePtr(val gatewayv1alpha1.GatewayAllowType) *gatewayv1alpha1.GatewayAllowType {
	return &val
}

func tlsModeTypePtr(mode gatewayv1alpha1.TLSModeType) *gatewayv1alpha1.TLSModeType {
	return &mode
}

// httpRouteAdmitted returns true if the route has a .status.conditions
// entry of "Admitted: true".
func httpRouteAdmitted(route *gatewayv1alpha1.HTTPRoute) bool {
	if route == nil {
		return false
	}

	for _, gw := range route.Status.Gateways {
		for _, cond := range gw.Conditions {
			if cond.Type == string(gatewayv1alpha1.ConditionRouteAdmitted) && cond.Status == metav1.ConditionTrue {
				return true
			}
		}
	}

	return false
}

// tlsRouteAdmitted returns true if the route has a .status.conditions
// entry of "Admitted: true".
func tlsRouteAdmitted(route *gatewayv1alpha1.TLSRoute) bool {
	if route == nil {
		return false
	}

	for _, gw := range route.Status.Gateways {
		for _, cond := range gw.Conditions {
			if cond.Type == string(gatewayv1alpha1.ConditionRouteAdmitted) && cond.Status == metav1.ConditionTrue {
				return true
			}
		}
	}

	return false
}
