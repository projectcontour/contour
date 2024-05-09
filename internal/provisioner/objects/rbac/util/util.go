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

package util

import (
	"strings"

	core_v1 "k8s.io/api/core/v1"
	discovery_v1 "k8s.io/api/discovery/v1"
	networking_v1 "k8s.io/api/networking/v1"
	rbac_v1 "k8s.io/api/rbac/v1"
	gatewayapi_v1 "sigs.k8s.io/gateway-api/apis/v1"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/provisioner/model"
	"github.com/projectcontour/contour/internal/provisioner/slice"
)

const contourV1GroupName = "projectcontour.io"

var (
	GatewayGroupNamespacedResource       = []string{"gateways", "httproutes", "tlsroutes", "grpcroutes", "tcproutes", "referencegrants", "backendtlspolicies"}
	GatewayGroupNamespacedResourceStatus = []string{"gateways/status", "httproutes/status", "tlsroutes/status", "grpcroutes/status", "tcproutes/status", "backendtlspolicies/status"}
	ContourGroupNamespacedResource       = []string{"httpproxies", "tlscertificatedelegations", "extensionservices", "contourconfigurations"}
	ContourGroupNamespacedResourceStatus = []string{"httpproxies/status", "extensionservices/status", "contourconfigurations/status"}
)

var (
	createGetUpdate = []string{"create", "get", "update"}
	getListWatch    = []string{"get", "list", "watch"}
	update          = []string{"update"}
)

// PolicyRuleFor returns PolicyRule object with provided apiGroup, verbs and resources
func PolicyRuleFor(apiGroup string, verbs []string, resources ...string) rbac_v1.PolicyRule {
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
		PolicyRuleFor(core_v1.GroupName, getListWatch, "secrets", "endpoints", "services", "configmaps"),

		// Discovery Contour-watched resources.
		PolicyRuleFor(discovery_v1.GroupName, getListWatch, "endpointslices"),

		// Gateway API resources.
		// Note, ReferenceGrant does not currently have a .status field so it's omitted from the status rule.
		PolicyRuleFor(gatewayapi_v1.GroupName, getListWatch, filterResources(resourcesToSkip, GatewayGroupNamespacedResource...)...),
		PolicyRuleFor(gatewayapi_v1.GroupName, update, filterResources(resourcesToSkip, GatewayGroupNamespacedResourceStatus...)...),

		// Ingress resources.
		PolicyRuleFor(networking_v1.GroupName, getListWatch, "ingresses"),
		PolicyRuleFor(networking_v1.GroupName, createGetUpdate, "ingresses/status"),

		// Contour CRDs.
		PolicyRuleFor(contourV1GroupName, getListWatch, filterResources(resourcesToSkip, ContourGroupNamespacedResource...)...),
		PolicyRuleFor(contourV1GroupName, createGetUpdate, filterResources(resourcesToSkip, ContourGroupNamespacedResourceStatus...)...),
	}
}

// ClusterScopedResourcePolicyRules returns a set of policy rules for
// cluster-scoped resources.
func ClusterScopedResourcePolicyRules() []rbac_v1.PolicyRule {
	return []rbac_v1.PolicyRule{
		// GatewayClass.
		PolicyRuleFor(gatewayapi_v1.GroupName, getListWatch, "gatewayclasses"),
		PolicyRuleFor(gatewayapi_v1.GroupName, update, "gatewayclasses/status"),

		// Namespaces
		PolicyRuleFor(core_v1.GroupName, getListWatch, "namespaces"),
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
