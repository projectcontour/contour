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

package policyrule

import (
	"strings"

	core_v1 "k8s.io/api/core/v1"
	discovery_v1 "k8s.io/api/discovery/v1"
	networking_v1 "k8s.io/api/networking/v1"
	rbac_v1 "k8s.io/api/rbac/v1"
	gatewayapi_v1 "sigs.k8s.io/gateway-api/apis/v1"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/provisioner/model"
	"github.com/projectcontour/contour/internal/provisioner/objects/rbac/resources"
	"github.com/projectcontour/contour/internal/provisioner/slice"
)

const contourV1GroupName = "projectcontour.io"

var (
	createGetUpdate = []string{"create", "get", "update"}
	getListWatch    = []string{"get", "list", "watch"}
	update          = []string{"update"}
)

// policyRuleFor returns PolicyRule object with provided apiGroup, verbs and resources
func policyRuleFor(apiGroup string, verbs []string, resources ...string) rbac_v1.PolicyRule {
	return rbac_v1.PolicyRule{
		Verbs:     verbs,
		APIGroups: []string{apiGroup},
		Resources: resources,
	}
}

// NamespacedResourcePolicyRules returns a set of policy rules for resources that are
// namespaced-scoped. If resourcesToSkip is not empty, skip creating RBAC for those
// CRDs.
func NamespacedResourcePolicyRules(resourcesToSkip []contour_v1.Feature) []rbac_v1.PolicyRule {
	return []rbac_v1.PolicyRule{
		// Core Contour-watched resources.
		policyRuleFor(core_v1.GroupName, getListWatch, "secrets", "services", "configmaps"),

		// Discovery Contour-watched resources.
		policyRuleFor(discovery_v1.GroupName, getListWatch, "endpointslices"),

		// Gateway API resources.
		// Note, ReferenceGrant does not currently have a .status field so it's omitted from the status rule.
		policyRuleFor(gatewayapi_v1.GroupName, getListWatch, filterResources(resourcesToSkip, resources.GatewayGroupNamespaced...)...),
		policyRuleFor(gatewayapi_v1.GroupName, update, filterResources(resourcesToSkip, resources.GatewayGroupNamespacedStatus...)...),

		// Ingress resources.
		policyRuleFor(networking_v1.GroupName, getListWatch, "ingresses"),
		policyRuleFor(networking_v1.GroupName, createGetUpdate, "ingresses/status"),

		// Contour CRDs.
		policyRuleFor(contourV1GroupName, getListWatch, filterResources(resourcesToSkip, resources.ContourGroupNamespaced...)...),
		policyRuleFor(contourV1GroupName, createGetUpdate, filterResources(resourcesToSkip, resources.ContourGroupNamespacedStatus...)...),
	}
}

// ClusterScopedResourcePolicyRules returns a set of policy rules for
// cluster-scoped resources.
func ClusterScopedResourcePolicyRules() []rbac_v1.PolicyRule {
	return []rbac_v1.PolicyRule{
		// GatewayClass.
		policyRuleFor(gatewayapi_v1.GroupName, getListWatch, "gatewayclasses"),
		policyRuleFor(gatewayapi_v1.GroupName, update, "gatewayclasses/status"),

		// Namespaces
		policyRuleFor(core_v1.GroupName, getListWatch, "namespaces"),
	}
}

func filterResources(resourcesToSkip []contour_v1.Feature, resources ...string) []string {
	if len(resourcesToSkip) == 0 {
		return resources
	}
	filteredResources := []string{}
	rts := model.FeaturesToStrings(resourcesToSkip)
	for _, resource := range resources {
		resourceCopy := resource
		// handle status resources by splitting and using the first part
		if strings.Contains(resourceCopy, "/") {
			parts := strings.Split(resourceCopy, "/")
			resourceCopy = parts[0]
		}
		if !slice.ContainsString(rts, resourceCopy) {
			filteredResources = append(filteredResources, resource)
		}
	}
	return filteredResources
}
