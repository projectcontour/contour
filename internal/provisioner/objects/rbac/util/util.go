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
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

const (
	ContourV1GroupName = "projectcontour.io"
)

var (
	createGetUpdate = []string{"create", "get", "update"}
	getListWatch    = []string{"get", "list", "watch"}
	update          = []string{"update"}
)

// PolicyRuleFor returns PolicyRule object with provided apiGroup, verbs and resources
func PolicyRuleFor(apiGroup string, verbs []string, resources ...string) rbacv1.PolicyRule {
	return rbacv1.PolicyRule{
		Verbs:     verbs,
		APIGroups: []string{apiGroup},
		Resources: resources,
	}
}

// BasicPolicyRulesForContour returns set of basic rules that contour requires
func BasicPolicyRulesForContour() []rbacv1.PolicyRule {
	return []rbacv1.PolicyRule{
		// Core Contour-watched resources.
		PolicyRuleFor(corev1.GroupName, getListWatch, "secrets", "endpoints", "services", "namespaces"),

		// Discovery Contour-watched resources.
		PolicyRuleFor(discoveryv1.GroupName, getListWatch, "endpointslices"),

		// Gateway API resources.
		// Note, ReferenceGrant does not currently have a .status field so it's omitted from the status rule.
		PolicyRuleFor(gatewayv1alpha2.GroupName, getListWatch, "gateways", "httproutes", "tlsroutes", "grpcroutes", "tcproutes", "referencegrants"),
		PolicyRuleFor(gatewayv1alpha2.GroupName, update, "gateways/status", "httproutes/status", "tlsroutes/status", "grpcroutes/status", "tcproutes/status"),

		// Ingress resources.
		PolicyRuleFor(networkingv1.GroupName, getListWatch, "ingresses"),
		PolicyRuleFor(networkingv1.GroupName, createGetUpdate, "ingresses/status"),

		// Contour CRDs.
		PolicyRuleFor(ContourV1GroupName, getListWatch, "httpproxies", "tlscertificatedelegations", "extensionservices", "contourconfigurations"),
		PolicyRuleFor(ContourV1GroupName, createGetUpdate, "httpproxies/status", "extensionservices/status", "contourconfigurations/status"),
	}
}

// ClusterScopePolicyRulesForContour returns set of rules only for cluster scope object
func ClusterScopePolicyRulesForContour() []rbacv1.PolicyRule {
	return []rbacv1.PolicyRule{
		// GatewayClass only.
		PolicyRuleFor(gatewayv1alpha2.GroupName, getListWatch, "gatewayclasses"),
		PolicyRuleFor(gatewayv1alpha2.GroupName, update, "gatewayclasses/status"),
	}
}
