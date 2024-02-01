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

package role

import (
	"fmt"
	"testing"

	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/provisioner/model"
	"github.com/projectcontour/contour/internal/provisioner/objects/rbac/util"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	rbacv1 "k8s.io/api/rbac/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/utils/diff"
)

func checkRoleName(t *testing.T, role *rbacv1.Role, expected string) {
	t.Helper()

	if role.Name == expected {
		return
	}

	t.Errorf("role %q has unexpected name", role.Name)
}

func checkRoleLabels(t *testing.T, role *rbacv1.Role, expected map[string]string) {
	t.Helper()

	if apiequality.Semantic.DeepEqual(role.Labels, expected) {
		return
	}

	t.Errorf("role has unexpected %q labels", role.Labels)
}

func checkRoleNamespace(t *testing.T, role *rbacv1.Role, namespace string) {
	t.Helper()

	if role.Namespace == namespace {
		return
	}

	t.Errorf("role has unexpected '%q' namespace", role.Namespace)
}

func TestDesiredControllerRole(t *testing.T) {
	name := "role-test"
	cntr := model.Default(fmt.Sprintf("%s-ns", name), name)
	role := desiredControllerRole(name, cntr)
	checkRoleName(t, role, name)
	ownerLabels := map[string]string{
		model.ContourOwningGatewayNameLabel:    cntr.Name,
		model.GatewayAPIOwningGatewayNameLabel: cntr.Name,
	}
	checkRoleLabels(t, role, ownerLabels)
}

func TestDesiredRoleForContourInNamespace(t *testing.T) {
	testCases := []struct {
		description string
		namespace   string
	}{
		{
			description: "namespace 1",
			namespace:   "ns1",
		},
		{
			description: "namespace 2",
			namespace:   "ns2",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			name := "role-test"
			cntr := model.Default(fmt.Sprintf("%s-ns", name), name)
			role := desiredRoleForResourceInNamespace(name, tc.namespace, cntr)
			checkRoleName(t, role, name)
			ownerLabels := map[string]string{
				model.ContourOwningGatewayNameLabel:    cntr.Name,
				model.GatewayAPIOwningGatewayNameLabel: cntr.Name,
			}
			checkRoleLabels(t, role, ownerLabels)
			checkRoleNamespace(t, role, tc.namespace)
		})
	}
}

func TestDesiredRoleFilterResources(t *testing.T) {
	filterNamespacedGatewayResources := func(policyRules []rbacv1.PolicyRule) [][]string {
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

	filterContourResources := func(policyRules []rbacv1.PolicyRule) [][]string {
		contourResources := [][]string{}
		for _, rule := range policyRules {
			for _, apigroup := range rule.APIGroups {
				if apigroup == contourv1.GroupName {
					contourResources = append(contourResources, rule.Resources)
					break
				}
			}
		}
		return contourResources
	}

	tests := []struct {
		description      string
		disabledFeatures []contourv1.Feature
		expectedGateway  [][]string
		expectedContour  [][]string
	}{
		{
			description:      "empty disabled features",
			disabledFeatures: nil,
			expectedGateway:  [][]string{util.GatewayGroupNamespacedResource, util.GatewayGroupNamespacedResourceStatus},
			expectedContour:  [][]string{util.ContourGroupNamespacedResource, util.ContourGroupNamespacedResourceStatus},
		},
		{
			description:      "disable tlsroutes feature",
			disabledFeatures: []contourv1.Feature{"tlsroutes"},
			expectedGateway: [][]string{
				{"gateways", "httproutes", "grpcroutes", "tcproutes", "referencegrants", "backendtlspolicies"},
				{"gateways/status", "httproutes/status", "grpcroutes/status", "tcproutes/status", "backendtlspolicies/status"},
			},
			expectedContour: [][]string{util.ContourGroupNamespacedResource, util.ContourGroupNamespacedResourceStatus},
		},

		{
			description:      "disable extensionservices feature",
			disabledFeatures: []contourv1.Feature{"extensionservices"},
			expectedGateway:  [][]string{util.GatewayGroupNamespacedResource, util.GatewayGroupNamespacedResourceStatus},
			expectedContour: [][]string{
				{"httpproxies", "tlscertificatedelegations", "contourconfigurations"},
				{"httpproxies/status", "contourconfigurations/status"},
			},
		},
		{
			description:      "disable 2 features",
			disabledFeatures: []contourv1.Feature{"tlsroutes", "grpcroutes"},
			expectedGateway: [][]string{
				{"gateways", "httproutes", "tcproutes", "referencegrants", "backendtlspolicies"},
				{"gateways/status", "httproutes/status", "tcproutes/status", "backendtlspolicies/status"},
			},
			expectedContour: [][]string{util.ContourGroupNamespacedResource, util.ContourGroupNamespacedResourceStatus},
		},
		{
			description:      "disable non-existent features",
			disabledFeatures: []contourv1.Feature{"abc", "efg"},
			expectedGateway:  [][]string{util.GatewayGroupNamespacedResource, util.GatewayGroupNamespacedResourceStatus},
			expectedContour:  [][]string{util.ContourGroupNamespacedResource, util.ContourGroupNamespacedResourceStatus},
		},
		{
			description:      "disable both gateway and contour features",
			disabledFeatures: []contourv1.Feature{"grpcroutes", "tlsroutes", "backendtlspolicies", "extensionservices"},
			expectedGateway: [][]string{
				{"gateways", "httproutes", "tcproutes", "referencegrants"},
				{"gateways/status", "httproutes/status", "tcproutes/status"},
			},
			expectedContour: [][]string{
				{"httpproxies", "tlscertificatedelegations", "contourconfigurations"},
				{"httpproxies/status", "contourconfigurations/status"},
			},
		},
	}

	cntrName := "test-filteredresources"
	cntr := model.Default(fmt.Sprintf("%s-ns", cntrName), cntrName)

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			cntrLocal := cntr

			// set the disableFeatures
			cntrLocal.Spec.DisabledFeatures = tt.disabledFeatures

			cr := desiredRoleForResourceInNamespace(cntrName, "test", cntrLocal)

			// fetch gateway resources
			gatewayResources := filterNamespacedGatewayResources(cr.Rules)
			contourResources := filterContourResources(cr.Rules)
			if !apiequality.Semantic.DeepEqual(gatewayResources, tt.expectedGateway) {
				t.Errorf("filtered gateway resources didn't match: %v", diff.ObjectReflectDiff(gatewayResources, tt.expectedGateway))
			}

			if !apiequality.Semantic.DeepEqual(contourResources, tt.expectedContour) {
				t.Errorf("filtered contour resources didn't match: %v", diff.ObjectReflectDiff(contourResources, tt.expectedContour))
			}
		})
	}
}
