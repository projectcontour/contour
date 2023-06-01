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

package clusterrole

import (
	"fmt"
	"testing"

	"github.com/projectcontour/contour/internal/provisioner/model"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	rbacv1 "k8s.io/api/rbac/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/util/diff"
)

func checkClusterRoleName(t *testing.T, cr *rbacv1.ClusterRole, expected string) {
	t.Helper()

	if cr.Name == expected {
		return
	}

	t.Errorf("cluster role has unexpected name %q", cr.Name)
}

func checkClusterRoleLabels(t *testing.T, cr *rbacv1.ClusterRole, expected map[string]string) {
	t.Helper()

	if apiequality.Semantic.DeepEqual(cr.Labels, expected) {
		return
	}

	t.Errorf("cluster role has unexpected %q labels", cr.Labels)
}

func TestDesiredClusterRole(t *testing.T) {
	name := "test-cr"
	cntr := model.Default(fmt.Sprintf("%s-ns", name), name)
	cr := desiredClusterRole(name, cntr)
	checkClusterRoleName(t, cr, name)
	ownerLabels := map[string]string{
		model.OwningGatewayNameLabel: cntr.Name,
	}
	checkClusterRoleLabels(t, cr, ownerLabels)
}

func TestDesiredClusterRoleFilterResources(t *testing.T) {
	filterGatewayResources := func(policyRules []rbacv1.PolicyRule) [][]string {
		gatewayResources := [][]string{}
		for _, rule := range policyRules {
			for _, apigroup := range rule.APIGroups {
				if apigroup == gatewayv1alpha2.GroupName {
					gatewayResources = append(gatewayResources, rule.Resources)
					break
				}
			}
		}
		return gatewayResources
	}

	gatewayResources := []string{"gatewayclasses", "gateways", "httproutes", "tlsroutes", "grpcroutes", "referencegrants"}
	gatewayStatusResources := []string{"gatewayclasses/status", "gateways/status", "httproutes/status", "grpcroutes/status", "tlsroutes/status"}

	tests := map[string]struct {
		disabledFeatures string
		expected         [][]string
	}{
		"empty disabled features": {
			expected: [][]string{gatewayResources, gatewayStatusResources},
		},
		"disable tlsroutes feature": {
			disabledFeatures: "tlsroutes",
			expected: [][]string{
				{"gatewayclasses", "gateways", "httproutes", "grpcroutes", "referencegrants"},
				{"gatewayclasses/status", "gateways/status", "httproutes/status", "grpcroutes/status"},
			},
		},
		"disable referencegrant feature": {
			disabledFeatures: "referencegrants",
			expected: [][]string{
				{"gatewayclasses", "gateways", "httproutes", "tlsroutes", "grpcroutes"},
				{"gatewayclasses/status", "gateways/status", "httproutes/status", "grpcroutes/status", "tlsroutes/status"},
			},
		},
		"disable 2 features": {
			disabledFeatures: "httproutes,referencegrants",
			expected: [][]string{
				{"gatewayclasses", "gateways", "tlsroutes", "grpcroutes"},
				{"gatewayclasses/status", "gateways/status", "grpcroutes/status", "tlsroutes/status"},
			},
		},
		"disable features in caps": {
			disabledFeatures: "httpRoutes,REFERENCEGRANTS",
			expected: [][]string{
				{"gatewayclasses", "gateways", "tlsroutes", "grpcroutes"},
				{"gatewayclasses/status", "gateways/status", "grpcroutes/status", "tlsroutes/status"},
			},
		},
		"disable non-existent features": {
			disabledFeatures: "http2routes,referencegrants",
			expected: [][]string{
				{"gatewayclasses", "gateways", "httproutes", "tlsroutes", "grpcroutes"},
				{"gatewayclasses/status", "gateways/status", "httproutes/status", "grpcroutes/status", "tlsroutes/status"},
			},
		},
		"disable non-existent features 2": {
			disabledFeatures: "http2routes,referencegrants2",
			expected:         [][]string{gatewayResources, gatewayStatusResources},
		},
		"disable all the features": {
			disabledFeatures: "gatewayclasses,gateways,httproutes,tlsroutes,grpcroutes,referencegrants",
			expected:         [][]string{{}, {}},
		},
	}

	cntrName := "test-filteredresources"
	cntr := model.Default(fmt.Sprintf("%s-ns", cntrName), cntrName)

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {

			cntrLocal := cntr

			// set the disableFeatures
			cntrLocal.Spec.DisabledFeatures = test.disabledFeatures

			cr := desiredClusterRole(cntrName, cntrLocal)

			// fetch gateway resources
			gatewayResources := filterGatewayResources(cr.Rules)

			if !apiequality.Semantic.DeepEqual(gatewayResources, test.expected) {
				t.Errorf("filtered resources didn't match: %v", diff.ObjectReflectDiff(gatewayResources, test.expected))
			}
		})
	}
}
