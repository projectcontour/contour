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

	rbac_v1 "k8s.io/api/rbac/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/utils/diff"
	gatewayapi_v1 "sigs.k8s.io/gateway-api/apis/v1"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/provisioner/model"
	"github.com/projectcontour/contour/internal/provisioner/objects/rbac/util"
	"github.com/projectcontour/contour/internal/provisioner/slice"
)

func checkRoleName(t *testing.T, role *rbac_v1.Role, expected string) {
	t.Helper()

	if role.Name == expected {
		return
	}

	t.Errorf("role %q has unexpected name", role.Name)
}

func checkRoleLabels(t *testing.T, role *rbac_v1.Role, expected map[string]string) {
	t.Helper()

	if apiequality.Semantic.DeepEqual(role.Labels, expected) {
		return
	}

	t.Errorf("role has unexpected %q labels", role.Labels)
}

func checkRoleNamespace(t *testing.T, role *rbac_v1.Role, namespace string) {
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
	filterNamespacedGatewayResources := func(policyRules []rbac_v1.PolicyRule) [][]string {
		gatewayResources := [][]string{}
		for _, rule := range policyRules {
			for _, apigroup := range rule.APIGroups {
				if apigroup == gatewayapi_v1.GroupName {
					gatewayResources = append(gatewayResources, rule.Resources)
					break
				}
			}
		}
		return gatewayResources
	}

	filterContourResources := func(policyRules []rbac_v1.PolicyRule) [][]string {
		contourResources := [][]string{}
		for _, rule := range policyRules {
			for _, apigroup := range rule.APIGroups {
				if apigroup == contour_v1.GroupName {
					contourResources = append(contourResources, rule.Resources)
					break
				}
			}
		}
		return contourResources
	}

	tests := []struct {
		description      string
		disabledFeatures []contour_v1.Feature
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
			disabledFeatures: []contour_v1.Feature{"tlsroutes"},
			expectedGateway: [][]string{
				removeFromStringArray(util.GatewayGroupNamespacedResource, "tlsroutes"),
				removeFromStringArray(util.GatewayGroupNamespacedResourceStatus, "tlsroutes/status"),
			},
			expectedContour: [][]string{util.ContourGroupNamespacedResource, util.ContourGroupNamespacedResourceStatus},
		},

		{
			description:      "disable extensionservices feature",
			disabledFeatures: []contour_v1.Feature{"extensionservices"},
			expectedGateway:  [][]string{util.GatewayGroupNamespacedResource, util.GatewayGroupNamespacedResourceStatus},
			expectedContour: [][]string{
				removeFromStringArray(util.ContourGroupNamespacedResource, "extensionservices"),
				removeFromStringArray(util.ContourGroupNamespacedResourceStatus, "extensionservices/status"),
			},
		},
		{
			description:      "disable non-existent features",
			disabledFeatures: []contour_v1.Feature{"abc", "efg"},
			expectedGateway:  [][]string{util.GatewayGroupNamespacedResource, util.GatewayGroupNamespacedResourceStatus},
			expectedContour:  [][]string{util.ContourGroupNamespacedResource, util.ContourGroupNamespacedResourceStatus},
		},
		{
			description:      "disable both gateway and contour features",
			disabledFeatures: []contour_v1.Feature{"grpcroutes", "tlsroutes", "backendtlspolicies", "extensionservices"},
			expectedGateway: [][]string{
				removeFromStringArray(util.GatewayGroupNamespacedResource, "tlsroutes", "grpcroutes", "backendtlspolicies"),
				removeFromStringArray(util.GatewayGroupNamespacedResourceStatus, "tlsroutes/status", "grpcroutes/status", "backendtlspolicies/status"),
			},
			expectedContour: [][]string{
				removeFromStringArray(util.ContourGroupNamespacedResource, "extensionservices"),
				removeFromStringArray(util.ContourGroupNamespacedResourceStatus, "extensionservices/status"),
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

func removeFromStringArray(arr []string, s ...string) []string {
	res := []string{}
	for _, a := range arr {
		if !slice.ContainsString(s, a) {
			res = append(res, a)
		}
	}
	return res
}
