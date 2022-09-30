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

package rbac

import (
	"context"
	"fmt"

	"github.com/projectcontour/contour/internal/provisioner/labels"
	"github.com/projectcontour/contour/internal/provisioner/model"
	"github.com/projectcontour/contour/internal/provisioner/objects/rbac/clusterrole"
	"github.com/projectcontour/contour/internal/provisioner/objects/rbac/clusterrolebinding"
	"github.com/projectcontour/contour/internal/provisioner/objects/rbac/role"
	"github.com/projectcontour/contour/internal/provisioner/objects/rbac/rolebinding"
	"github.com/projectcontour/contour/internal/provisioner/objects/rbac/serviceaccount"

	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// EnsureRBAC ensures all the necessary RBAC resources exist for the
// provided contour.
func EnsureRBAC(ctx context.Context, cli client.Client, contour *model.Contour) error {
	if err := ensureContourRBAC(ctx, cli, contour); err != nil {
		return fmt.Errorf("failed to ensure Contour RBAC: %w", err)
	}

	if err := ensureEnvoyRBAC(ctx, cli, contour); err != nil {
		return fmt.Errorf("failed to ensure Envoy RBAC: %w", err)
	}

	return nil
}

func ensureContourRBAC(ctx context.Context, cli client.Client, contour *model.Contour) error {
	names := contour.ContourRBACNames()

	// Ensure service account.
	if err := serviceaccount.EnsureServiceAccount(ctx, cli, names.ServiceAccount, contour); err != nil {
		return fmt.Errorf("failed to ensure service account %s/%s: %w", contour.Namespace, names.ServiceAccount, err)
	}

	// Ensure cluster role & binding.
	if err := clusterrole.EnsureClusterRole(ctx, cli, names.ClusterRole, contour); err != nil {
		return fmt.Errorf("failed to ensure cluster role %s: %w", names.ClusterRole, err)
	}
	if err := clusterrolebinding.EnsureClusterRoleBinding(ctx, cli, names.ClusterRoleBinding, names.ClusterRole, names.ServiceAccount, contour); err != nil {
		return fmt.Errorf("failed to ensure cluster role binding %s: %w", names.ClusterRoleBinding, err)
	}

	// Ensure role & binding.
	if err := role.EnsureControllerRole(ctx, cli, names.Role, contour); err != nil {
		return fmt.Errorf("failed to ensure controller role %s/%s: %w", contour.Namespace, names.Role, err)
	}
	if err := rolebinding.EnsureRoleBinding(ctx, cli, names.RoleBinding, names.ServiceAccount, names.Role, contour); err != nil {
		return fmt.Errorf("failed to ensure controller role binding %s/%s: %w", contour.Namespace, names.RoleBinding, err)
	}
	return nil
}

func ensureEnvoyRBAC(ctx context.Context, cli client.Client, contour *model.Contour) error {
	names := contour.EnvoyRBACNames()

	// Ensure service account.
	if err := serviceaccount.EnsureServiceAccount(ctx, cli, names.ServiceAccount, contour); err != nil {
		return fmt.Errorf("failed to ensure service account %s/%s: %w", contour.Namespace, names.ServiceAccount, err)
	}
	return nil
}

// EnsureRBACDeleted ensures all the necessary RBAC resources for the provided
// contour are deleted if Contour owner labels exist.
func EnsureRBACDeleted(ctx context.Context, cli client.Client, contour *model.Contour) error {
	var deletions []client.Object

	for _, name := range []model.RBACNames{
		contour.ContourRBACNames(),
		contour.EnvoyRBACNames(),
	} {
		if len(name.RoleBinding) > 0 {
			rolebinding, err := rolebinding.CurrentRoleBinding(ctx, cli, contour.Namespace, name.RoleBinding)
			if err != nil && !errors.IsNotFound(err) {
				return err
			}
			if rolebinding != nil {
				deletions = append(deletions, rolebinding)
			}
		}

		if len(name.Role) > 0 {
			role, err := role.CurrentRole(ctx, cli, contour.Namespace, name.Role)
			if err != nil && !errors.IsNotFound(err) {
				return err
			}
			if role != nil {
				deletions = append(deletions, role)
			}
		}

		if len(name.ClusterRoleBinding) > 0 {
			clusterrolebinding, err := clusterrolebinding.CurrentClusterRoleBinding(ctx, cli, name.ClusterRoleBinding)
			if err != nil && !errors.IsNotFound(err) {
				return err
			}
			if clusterrolebinding != nil {
				deletions = append(deletions, clusterrolebinding)
			}
		}

		if len(name.ClusterRole) > 0 {
			clusterrole, err := clusterrole.CurrentClusterRole(ctx, cli, name.ClusterRole)
			if err != nil && !errors.IsNotFound(err) {
				return err
			}
			if clusterrole != nil {
				deletions = append(deletions, clusterrole)
			}
		}

		if len(name.ServiceAccount) > 0 {
			serviceaccount, err := serviceaccount.CurrentServiceAccount(ctx, cli, contour.Namespace, name.ServiceAccount)
			if err != nil && !errors.IsNotFound(err) {
				return err
			}
			if serviceaccount != nil {
				deletions = append(deletions, serviceaccount)
			}
		}
	}

	for _, deletion := range deletions {
		if !labels.Exist(deletion, model.OwnerLabels(contour)) {
			continue
		}

		if err := cli.Delete(ctx, deletion); err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("failed to delete %s %s/%s: %w", deletion.GetObjectKind().GroupVersionKind().Kind, deletion.GetNamespace(), deletion.GetName(), err)
		}
	}

	return nil
}
