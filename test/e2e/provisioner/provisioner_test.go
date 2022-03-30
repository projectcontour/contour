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

package provisioner

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/projectcontour/contour/internal/gatewayapi"
	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

var f = e2e.NewFramework(true)

func TestProvisioner(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Gateway provisioner tests")
}

var _ = BeforeSuite(func() {
	require.NoError(f.T(), f.Provisioner.EnsureResourcesForInclusterProvisioner())

	gc := &gatewayapi_v1alpha2.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "contour",
		},
		Spec: gatewayapi_v1alpha2.GatewayClassSpec{
			ControllerName: gatewayapi_v1alpha2.GatewayController("projectcontour.io/gateway-provisioner"),
		},
	}

	_, ok := f.CreateGatewayClassAndWaitFor(gc, gatewayClassAccepted)
	require.True(f.T(), ok)

})

var _ = AfterSuite(func() {
	// Delete resources individually instead of deleting the entire contour
	// namespace as a performance optimization, because deleting non-empty
	// namespaces can take up to a couple minutes to complete.
	require.NoError(f.T(), f.Provisioner.DeleteResourcesForInclusterProvisioner())

	gc := &gatewayapi_v1alpha2.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "contour",
		},
	}
	require.NoError(f.T(), f.DeleteGatewayClass(gc, false))
})

var _ = Describe("Gateway provisioner", func() {
	f.NamespacedTest("basic-provisioned-gateway", func(namespace string) {
		Specify("A basic one-listener HTTP gateway can be provisioned", func() {
			gateway := &gatewayapi_v1alpha2.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "http",
					Namespace: namespace,
				},
				Spec: gatewayapi_v1alpha2.GatewaySpec{
					GatewayClassName: gatewayapi_v1alpha2.ObjectName("contour"),
					Listeners: []gatewayapi_v1alpha2.Listener{
						{
							Name:     "http",
							Protocol: gatewayapi_v1alpha2.HTTPProtocolType,
							Port:     gatewayapi_v1alpha2.PortNumber(80),
							AllowedRoutes: &gatewayapi_v1alpha2.AllowedRoutes{
								Namespaces: &gatewayapi_v1alpha2.RouteNamespaces{
									From: gatewayapi.FromNamespacesPtr(gatewayapi_v1alpha2.NamespacesFromSame),
								},
							},
						},
					},
				},
			}

			_, ok := f.CreateGatewayAndWaitFor(gateway, gatewayReady)
			require.True(f.T(), ok)
		})
	})
})

// gatewayClassAccepted returns true if the gateway has a .status.conditions
// entry of Accepted: true".
func gatewayClassAccepted(gatewayClass *gatewayapi_v1alpha2.GatewayClass) bool {
	if gatewayClass == nil {
		return false
	}

	for _, cond := range gatewayClass.Status.Conditions {
		if cond.Type == string(gatewayapi_v1alpha2.GatewayClassConditionStatusAccepted) && cond.Status == metav1.ConditionTrue {
			return true
		}
	}

	return false
}

// gatewayReady returns true if the gateway has a .status.conditions
// entry of Ready: true".
func gatewayReady(gateway *gatewayapi_v1alpha2.Gateway) bool {
	if gateway == nil {
		return false
	}

	for _, cond := range gateway.Status.Conditions {
		if cond.Type == string(gatewayapi_v1alpha2.GatewayConditionReady) && cond.Status == metav1.ConditionTrue {
			return true
		}
	}

	return false
}
