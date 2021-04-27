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

	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

type testGroup struct {
	name    string
	gateway *gatewayv1alpha1.Gateway
	tests   map[string]func(t *testing.T, f *e2e.Framework)
}

// getTests defines the test groups that make up the Gateway test suite.
// Each group shares a single Gateway definition.
func getTests() []*testGroup {
	var testGroups []*testGroup

	testGroups = append(testGroups, &testGroup{
		name: "default gateway",
		tests: map[string]func(t *testing.T, f *e2e.Framework){
			"001-path-condition-match":               testGatewayPathConditionMatch,
			"002-header-condition-match":             testGatewayHeaderConditionMatch,
			"003-invalid-forward-to":                 testInvalidForwardTo,
			"005-request-header-modifier-forward-to": testRequestHeaderModifierForwardTo,
			"005-request-header-modifier-rule":       testRequestHeaderModifierRule,
		},
		gateway: &gatewayv1alpha1.Gateway{
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
		},
	})

	testGroups = append(testGroups, &testGroup{
		name: "TLS gateway",
		tests: map[string]func(t *testing.T, f *e2e.Framework){
			"004-tls-gateway": testTLSGateway,
		},
		gateway: &gatewayv1alpha1.Gateway{
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
		},
	})

	return testGroups
}

func TestGatewayAPI(t *testing.T) {
	f := e2e.NewFramework(t)

	for _, group := range getTests() {
		func() {
			require.NoError(t, f.Client.Create(context.TODO(), group.gateway))
			defer func() {
				// has to be wrapped in a defer func() {...} with no arguments because
				// otherwise the arguments to require.NoError(...) are evaluated right away,
				// i.e. the Delete(..) is called right away, not as part of the defer.
				require.NoError(t, f.Client.Delete(context.TODO(), group.gateway))
			}()

			f.RunParallel(group.name, group.tests)
		}()

	}
}

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
