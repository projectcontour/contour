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

	"github.com/projectcontour/contour/e2e"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

// subtests defines the tests to run as part of the Gateway API
// suite.
var subtests = map[string]func(t *testing.T, f *e2e.Framework){
	"001-path-condition-match":   testGatewayPathConditionMatch,
	"002-header-condition-match": testGatewayHeaderConditionMatch,
}

func TestGatewayAPI(t *testing.T) {
	f := e2e.NewFramework(t)

	// Gateway
	gateway := &gatewayv1alpha1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "projectcontour", // TODO needs to be this to match default settings, but need to clean it up!
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
						Namespaces: gatewayv1alpha1.RouteNamespaces{
							From: gatewayv1alpha1.RouteSelectAll,
						},
						Selector: metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "filter"},
						},
					},
				},
			},
		},
	}
	require.NoError(t, f.Client.Create(context.TODO(), gateway))
	// TODO it'd be nice to have automatic object tracking
	defer f.Client.Delete(context.TODO(), gateway)

	for name, tc := range subtests {
		t.Run(name, func(t *testing.T) {
			tc(t, f)
		})
	}
}

func stringPtr(s string) *string {
	return &s
}

func portNumPtr(port int) *gatewayv1alpha1.PortNumber {
	pn := gatewayv1alpha1.PortNumber(port)
	return &pn
}
