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
	"slices"
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

func checkClusterRoleName(t *testing.T, cr *rbac_v1.ClusterRole, expected string) {
	t.Helper()

	if cr.Name == expected {
		return
	}

	t.Errorf("cluster role has unexpected name %q", cr.Name)
}

func checkClusterRoleLabels(t *testing.T, cr *rbac_v1.ClusterRole, expected map[string]string) {
	t.Helper()

	if apiequality.Semantic.DeepEqual(cr.Labels, expected) {
		return
	}

	t.Errorf("cluster role has unexpected %q labels", cr.Labels)
}

func clusterRoleRulesContainOnlyClusterScopeRules(cr *rbac_v1.ClusterRole) bool {
	for _, r := range cr.Rules {
		if !slices.Contains(r.Resources, "gatewayclasses") &&
			!slices.Contains(r.Resources, "gatewayclasses/status") &&
			!slices.Contains(r.Resources, "namespaces") {
			return false
		}
	}

	return true
}

func TestDesiredClusterRole(t *testing.T) {
	testCases := []struct {
		description      string
		clusterScopeOnly bool
	}{
		{
			description:      "gateway class rule only role",
			clusterScopeOnly: true,
		},
		{
			description:      "generic cluster role include all rules",
			clusterScopeOnly: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			name := "test-cr"
			cntr := model.Default(fmt.Sprintf("%s-ns", name), name)
			cr := desiredClusterRole(name, cntr, tc.clusterScopeOnly)
			checkClusterRoleName(t, cr, name)
			ownerLabels := map[string]string{
				model.ContourOwningGatewayNameLabel:    cntr.Name,
				model.GatewayAPIOwningGatewayNameLabel: cntr.Name,
			}
			checkClusterRoleLabels(t, cr, ownerLabels)
			if tc.clusterScopeOnly != clusterRoleRulesContainOnlyClusterScopeRules(cr) {
				t.Errorf("expect clusterScopeOnly to be %v, but clusterRoleRulesContainOnlyClusterScopeRules shows %v",
					tc.clusterScopeOnly, clusterRoleRulesContainOnlyClusterScopeRules(cr))
			}
		})
	}
}

func TestDesiredClusterRoleFilterResources(t *testing.T) {
	filterNamespacedGatewayResources := func(policyRules []rbac_v1.PolicyRule) [][]string {
		gatewayResources := [][]string{}
		for _, rule := range policyRules {
			for _, apigroup := range rule.APIGroups {
				// gatewayclass is in isolate rule
				if apigroup == gatewayapi_v1.GroupName && rule.Resources[0] != "gatewayclasses" && rule.Resources[0] != "gatewayclasses/status" {
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
		description               string
		disabledFeatures          []contour_v1.Feature
		clusterScopedResourceOnly bool
		expectedGateway           [][]string
		expectedContour           [][]string
	}{
		{
			description:               "empty disabled features",
			disabledFeatures:          nil,
			clusterScopedResourceOnly: false,
			expectedGateway:           [][]string{util.GatewayGroupNamespacedResource, util.GatewayGroupNamespacedResourceStatus},
			expectedContour:           [][]string{util.ContourGroupNamespacedResource, util.ContourGroupNamespacedResourceStatus},
		},
		{
			description:               "disable tlsroutes feature",
			disabledFeatures:          []contour_v1.Feature{"tlsroutes"},
			clusterScopedResourceOnly: false,
			expectedGateway: [][]string{
				removeFromStringArray(util.GatewayGroupNamespacedResource, "tlsroutes"),
				removeFromStringArray(util.GatewayGroupNamespacedResourceStatus, "tlsroutes/status"),
			},
			expectedContour: [][]string{util.ContourGroupNamespacedResource, util.ContourGroupNamespacedResourceStatus},
		},

		{
			description:               "disable extensionservices feature",
			disabledFeatures:          []contour_v1.Feature{"extensionservices"},
			clusterScopedResourceOnly: false,
			expectedGateway:           [][]string{util.GatewayGroupNamespacedResource, util.GatewayGroupNamespacedResourceStatus},
			expectedContour: [][]string{
				removeFromStringArray(util.ContourGroupNamespacedResource, "extensionservices"),
				removeFromStringArray(util.ContourGroupNamespacedResourceStatus, "extensionservices/status"),
			},
		},
		{
			description:               "disable non-existent features",
			disabledFeatures:          []contour_v1.Feature{"abc", "efg"},
			clusterScopedResourceOnly: false,
			expectedGateway:           [][]string{util.GatewayGroupNamespacedResource, util.GatewayGroupNamespacedResourceStatus},
			expectedContour:           [][]string{util.ContourGroupNamespacedResource, util.ContourGroupNamespacedResourceStatus},
		},
		{
			description:               "disable both gateway and contour features",
			disabledFeatures:          []contour_v1.Feature{"grpcroutes", "tlsroutes", "extensionservices", "backendtlspolicies"},
			clusterScopedResourceOnly: false,
			expectedGateway: [][]string{
				removeFromStringArray(util.GatewayGroupNamespacedResource, "tlsroutes", "grpcroutes", "backendtlspolicies"),
				removeFromStringArray(util.GatewayGroupNamespacedResourceStatus, "tlsroutes/status", "grpcroutes/status", "backendtlspolicies/status"),
			},
			expectedContour: [][]string{
				removeFromStringArray(util.ContourGroupNamespacedResource, "extensionservices"),
				removeFromStringArray(util.ContourGroupNamespacedResourceStatus, "extensionservices/status"),
			},
		},
		{
			description:               "empty disabled features but with clusterScoped only",
			disabledFeatures:          nil,
			clusterScopedResourceOnly: true,
			expectedGateway:           [][]string{},
			expectedContour:           [][]string{},
		},
	}

	cntrName := "test-filteredresources"
	cntr := model.Default(fmt.Sprintf("%s-ns", cntrName), cntrName)

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			cntrLocal := cntr

			// set the disableFeatures
			cntrLocal.Spec.DisabledFeatures = tt.disabledFeatures

			cr := desiredClusterRole(cntrName, cntrLocal, tt.clusterScopedResourceOnly)

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
