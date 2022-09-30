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
	"context"
	"fmt"

	"github.com/projectcontour/contour/internal/provisioner/equality"
	"github.com/projectcontour/contour/internal/provisioner/labels"
	"github.com/projectcontour/contour/internal/provisioner/model"
	"github.com/projectcontour/contour/internal/provisioner/objects"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

const (
	contourV1GroupName = "projectcontour.io"
)

// EnsureClusterRole ensures a ClusterRole resource exists with the provided name
// and contour namespace/name for the owning contour labels.
func EnsureClusterRole(ctx context.Context, cli client.Client, name string, contour *model.Contour) error {
	desired := desiredClusterRole(name, contour)

	updater := func(ctx context.Context, cli client.Client, contour *model.Contour, current, desired *rbacv1.ClusterRole) error {
		return updateClusterRoleIfNeeded(ctx, cli, contour, current, desired)
	}

	getter := func(ctx context.Context, cli client.Client, namespace, name string) (*rbacv1.ClusterRole, error) {
		return CurrentClusterRole(ctx, cli, name)
	}

	return objects.EnsureObject(ctx, cli, contour, desired, getter, updater)
}

// desiredClusterRole constructs an instance of the desired ClusterRole resource with
// the provided name and contour namespace/name for the owning contour labels.
func desiredClusterRole(name string, contour *model.Contour) *rbacv1.ClusterRole {
	var (
		createGetUpdate = []string{"create", "get", "update"}
		getListWatch    = []string{"get", "list", "watch"}
		update          = []string{"update"}
	)

	policyRuleFor := func(apiGroup string, verbs []string, resources ...string) rbacv1.PolicyRule {
		return rbacv1.PolicyRule{
			Verbs:     verbs,
			APIGroups: []string{apiGroup},
			Resources: resources,
		}
	}

	return &rbacv1.ClusterRole{
		TypeMeta: metav1.TypeMeta{
			Kind: "Role",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: model.CommonLabels(contour),
		},
		Rules: []rbacv1.PolicyRule{
			// Core Contour-watched resources.
			policyRuleFor(corev1.GroupName, getListWatch, "secrets", "endpoints", "services", "namespaces"),

			// Gateway API resources.
			// Note, ReferencePolicy/ReferenceGrant does not currently have a .status field so it's omitted from the status rule.
			policyRuleFor(gatewayv1alpha2.GroupName, getListWatch, "gatewayclasses", "gateways", "httproutes", "tlsroutes", "referencepolicies", "referencegrants"),
			policyRuleFor(gatewayv1alpha2.GroupName, update, "gatewayclasses/status", "gateways/status", "httproutes/status", "tlsroutes/status"),

			// Ingress resources.
			policyRuleFor(networkingv1.GroupName, getListWatch, "ingresses"),
			policyRuleFor(networkingv1.GroupName, createGetUpdate, "ingresses/status"),

			// Contour CRDs.
			policyRuleFor(contourV1GroupName, getListWatch, "httpproxies", "tlscertificatedelegations", "extensionservices", "contourconfigurations"),
			policyRuleFor(contourV1GroupName, createGetUpdate, "httpproxies/status", "extensionservices/status", "contourconfigurations/status"),
		},
	}
}

// CurrentClusterRole returns the current ClusterRole for the provided name.
func CurrentClusterRole(ctx context.Context, cli client.Client, name string) (*rbacv1.ClusterRole, error) {
	current := &rbacv1.ClusterRole{}
	key := types.NamespacedName{Name: name}
	err := cli.Get(ctx, key, current)
	if err != nil {
		return nil, err
	}
	return current, nil
}

// updateClusterRoleIfNeeded updates a ClusterRole resource if current does not match desired,
// using contour to verify the existence of owner labels.
func updateClusterRoleIfNeeded(ctx context.Context, cli client.Client, contour *model.Contour, current, desired *rbacv1.ClusterRole) error {
	if labels.Exist(current, model.OwnerLabels(contour)) {
		cr, updated := equality.ClusterRoleConfigChanged(current, desired)
		if updated {
			if err := cli.Update(ctx, cr); err != nil {
				return fmt.Errorf("failed to update cluster role %s: %w", cr.Name, err)
			}
			return nil
		}
	}
	return nil
}
