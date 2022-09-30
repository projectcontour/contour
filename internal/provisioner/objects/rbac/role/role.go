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
	"context"
	"fmt"

	equality "github.com/projectcontour/contour/internal/provisioner/equality"
	"github.com/projectcontour/contour/internal/provisioner/labels"
	"github.com/projectcontour/contour/internal/provisioner/model"
	"github.com/projectcontour/contour/internal/provisioner/objects"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// EnsureControllerRole ensures a Role resource exists with the for the Contour
// controller.
func EnsureControllerRole(ctx context.Context, cli client.Client, name string, contour *model.Contour) error {
	desired := desiredControllerRole(name, contour)

	updater := func(ctx context.Context, cli client.Client, current, desired *rbacv1.Role) error {
		_, err := updateRoleIfNeeded(ctx, cli, contour, current, desired)
		return err
	}

	return objects.EnsureObject(ctx, cli, &rbacv1.Role{}, desired, updater)
}

// desiredControllerRole constructs an instance of the desired Role resource with the
// provided ns/name and contour namespace/name for the owning contour labels for
// the Contour controller.
func desiredControllerRole(name string, contour *model.Contour) *rbacv1.Role {
	role := &rbacv1.Role{
		TypeMeta: metav1.TypeMeta{
			Kind: "Role",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: contour.Namespace,
			Name:      name,
			Labels:    model.CommonLabels(contour),
		},
	}
	verbCGU := []string{"create", "get", "update"}
	role.Rules = []rbacv1.PolicyRule{
		{
			Verbs:     verbCGU,
			APIGroups: []string{corev1.GroupName},
			Resources: []string{"events"},
		},
		{
			Verbs:     verbCGU,
			APIGroups: []string{coordinationv1.GroupName},
			Resources: []string{"leases"},
		},
	}
	return role
}

// CurrentRole returns the current Role for the provided ns/name.
func CurrentRole(ctx context.Context, cli client.Client, ns, name string) (*rbacv1.Role, error) {
	current := &rbacv1.Role{}
	key := types.NamespacedName{
		Namespace: ns,
		Name:      name,
	}
	err := cli.Get(ctx, key, current)
	if err != nil {
		return nil, err
	}
	return current, nil
}

// updateRoleIfNeeded updates a Role resource if current does not match desired,
// using contour to verify the existence of owner labels.
func updateRoleIfNeeded(ctx context.Context, cli client.Client, contour *model.Contour, current, desired *rbacv1.Role) (*rbacv1.Role, error) {
	if labels.Exist(current, model.OwnerLabels(contour)) {
		role, updated := equality.RoleConfigChanged(current, desired)
		if updated {
			if err := cli.Update(ctx, role); err != nil {
				return nil, fmt.Errorf("failed to update cluster role %s/%s: %w", role.Namespace, role.Name, err)
			}
			return role, nil
		}
	}
	return current, nil
}
