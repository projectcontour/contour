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

	coordination_v1 "k8s.io/api/coordination/v1"
	core_v1 "k8s.io/api/core/v1"
	rbac_v1 "k8s.io/api/rbac/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	equality "github.com/projectcontour/contour/internal/provisioner/equality"
	"github.com/projectcontour/contour/internal/provisioner/labels"
	"github.com/projectcontour/contour/internal/provisioner/model"
	"github.com/projectcontour/contour/internal/provisioner/objects"
	"github.com/projectcontour/contour/internal/provisioner/objects/rbac/util"
)

// EnsureControllerRole ensures a Role resource exists with the for the Contour
// controller.
func EnsureControllerRole(ctx context.Context, cli client.Client, name string, contour *model.Contour) error {
	desired := desiredControllerRole(name, contour)

	updater := func(ctx context.Context, cli client.Client, current, desired *rbac_v1.Role) error {
		err := updateRoleIfNeeded(ctx, cli, contour, current, desired)
		return err
	}

	return objects.EnsureObject(ctx, cli, desired, updater, &rbac_v1.Role{})
}

// EnsureRolesInNamespaces ensures a set of Role resources exist in namespaces
// specified, for contour to manage resources under these namespaces. And
// contour namespace/name for the owning contour labels for the Contour
// controller
func EnsureRolesInNamespaces(ctx context.Context, cli client.Client, name string, contour *model.Contour, namespaces []string) error {
	errs := []error{}
	for _, ns := range namespaces {
		desired := desiredRoleForResourceInNamespace(name, ns, contour)

		updater := func(ctx context.Context, cli client.Client, current, desired *rbac_v1.Role) error {
			err := updateRoleIfNeeded(ctx, cli, contour, current, desired)
			return err
		}
		if err := objects.EnsureObject(ctx, cli, desired, updater, &rbac_v1.Role{}); err != nil {
			errs = append(errs, err)
		}
	}

	return kerrors.NewAggregate(errs)
}

// desiredControllerRole constructs an instance of the desired Role resource with the
// provided ns/name and using contour namespace/name for the owning contour labels for
// the Contour controller.
func desiredControllerRole(name string, contour *model.Contour) *rbac_v1.Role {
	role := &rbac_v1.Role{
		TypeMeta: meta_v1.TypeMeta{
			Kind: "Role",
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace:   contour.Namespace,
			Name:        name,
			Labels:      contour.CommonLabels(),
			Annotations: contour.CommonAnnotations(),
		},
	}
	verbCGU := []string{"create", "get", "update"}
	role.Rules = []rbac_v1.PolicyRule{
		{
			Verbs:     verbCGU,
			APIGroups: []string{core_v1.GroupName},
			Resources: []string{"events"},
		},
		{
			Verbs:     verbCGU,
			APIGroups: []string{coordination_v1.GroupName},
			Resources: []string{"leases"},
		},
	}
	return role
}

// desiredRoleForResourceInNamespace constructs an instance of the desired Role resource with the
// provided ns/name and using contour namespace/name for the corresponding Contour instance
func desiredRoleForResourceInNamespace(name, namespace string, contour *model.Contour) *rbac_v1.Role {
	return &rbac_v1.Role{
		TypeMeta: meta_v1.TypeMeta{
			Kind: "Role",
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Labels:      contour.CommonLabels(),
			Annotations: contour.CommonAnnotations(),
		},
		Rules: util.NamespacedResourcePolicyRules(contour.Spec.DisabledFeatures),
	}
}

// updateRoleIfNeeded updates a Role resource if current does not match desired,
// using contour to verify the existence of owner labels.
func updateRoleIfNeeded(ctx context.Context, cli client.Client, contour *model.Contour, current, desired *rbac_v1.Role) error {
	if labels.AnyExist(current, model.OwnerLabels(contour)) {
		role, updated := equality.RoleConfigChanged(current, desired)
		if updated {
			if err := cli.Update(ctx, role); err != nil {
				return fmt.Errorf("failed to update cluster role %s/%s: %w", role.Namespace, role.Name, err)
			}
			return nil
		}
	}
	return nil
}
