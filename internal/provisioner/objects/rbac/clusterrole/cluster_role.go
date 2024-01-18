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
	"github.com/projectcontour/contour/internal/provisioner/objects/rbac/util"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// EnsureClusterRole ensures a ClusterRole resource exists with the provided name
// and contour namespace/name for the owning contour labels.
func EnsureClusterRole(ctx context.Context, cli client.Client, name string, contour *model.Contour, gatewayClassOnly bool) error {
	desired := desiredClusterRole(name, contour, gatewayClassOnly)

	// Enclose contour.
	updater := func(ctx context.Context, cli client.Client, current, desired *rbacv1.ClusterRole) error {
		return updateClusterRoleIfNeeded(ctx, cli, contour, current, desired)
	}

	return objects.EnsureObject(ctx, cli, desired, updater, &rbacv1.ClusterRole{})
}

// desiredClusterRole constructs an instance of the desired ClusterRole resource with
// the provided name and contour namespace/name for the owning contour labels.
func desiredClusterRole(name string, contour *model.Contour, gatewayClassOnly bool) *rbacv1.ClusterRole {
	role := &rbacv1.ClusterRole{
		TypeMeta: metav1.TypeMeta{
			Kind: "Role",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Labels:      contour.CommonLabels(),
			Annotations: contour.CommonAnnotations(),
		},
		Rules: util.ClusterScopedResourcePolicyRules(),
	}
	if gatewayClassOnly {
		return role
	}

	// add basic rules to role
	role.Rules = append(role.Rules, util.NamespacedResourcePolicyRules()...)
	return role
}

// updateClusterRoleIfNeeded updates a ClusterRole resource if current does not match desired,
// using contour to verify the existence of owner labels.
func updateClusterRoleIfNeeded(ctx context.Context, cli client.Client, contour *model.Contour, current, desired *rbacv1.ClusterRole) error {
	if labels.AnyExist(current, model.OwnerLabels(contour)) {
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
