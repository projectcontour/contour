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

package objects

import (
	"context"
	"fmt"

	"github.com/projectcontour/contour/internal/provisioner/labels"
	"github.com/projectcontour/contour/internal/provisioner/model"
	objcr "github.com/projectcontour/contour/internal/provisioner/objects/clusterrole"
	objcrb "github.com/projectcontour/contour/internal/provisioner/objects/clusterrolebinding"
	objrole "github.com/projectcontour/contour/internal/provisioner/objects/role"
	objrb "github.com/projectcontour/contour/internal/provisioner/objects/rolebinding"
	objsa "github.com/projectcontour/contour/internal/provisioner/objects/serviceaccount"

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

	if err := ensureCertgenRBAC(ctx, cli, contour); err != nil {
		return fmt.Errorf("failed to ensure Certgen RBAC: %w", err)
	}

	return nil
}

func ensureContourRBAC(ctx context.Context, cli client.Client, contour *model.Contour) error {
	names := contour.ContourRBACNames()

	// Ensure service account.
	if _, err := objsa.EnsureServiceAccount(ctx, cli, names.ServiceAccount, contour); err != nil {
		return fmt.Errorf("failed to ensure service account %s/%s: %w", contour.Namespace, names.ServiceAccount, err)
	}

	// Ensure cluster role & binding.
	if _, err := objcr.EnsureClusterRole(ctx, cli, names.ClusterRole, contour); err != nil {
		return fmt.Errorf("failed to ensure cluster role %s: %w", names.ClusterRole, err)
	}
	if err := objcrb.EnsureClusterRoleBinding(ctx, cli, names.ClusterRoleBinding, names.ClusterRole, names.ServiceAccount, contour); err != nil {
		return fmt.Errorf("failed to ensure cluster role binding %s: %w", names.ClusterRoleBinding, err)
	}

	// Ensure role & binding.
	if _, err := objrole.EnsureControllerRole(ctx, cli, names.Role, contour); err != nil {
		return fmt.Errorf("failed to ensure controller role %s/%s: %w", contour.Namespace, names.Role, err)
	}
	if err := objrb.EnsureRoleBinding(ctx, cli, names.RoleBinding, names.ServiceAccount, names.Role, contour); err != nil {
		return fmt.Errorf("failed to ensure controller role binding %s/%s: %w", contour.Namespace, names.RoleBinding, err)
	}

	return nil
}

func ensureEnvoyRBAC(ctx context.Context, cli client.Client, contour *model.Contour) error {
	names := contour.EnvoyRBACNames()

	// Ensure service account.
	if _, err := objsa.EnsureServiceAccount(ctx, cli, names.ServiceAccount, contour); err != nil {
		return fmt.Errorf("failed to ensure service account %s/%s: %w", contour.Namespace, names.ServiceAccount, err)
	}

	return nil
}

func ensureCertgenRBAC(ctx context.Context, cli client.Client, contour *model.Contour) error {
	names := contour.CertgenRBACNames()

	// Ensure service account.
	if _, err := objsa.EnsureServiceAccount(ctx, cli, names.ServiceAccount, contour); err != nil {
		return fmt.Errorf("failed to ensure service account %s/%s: %w", contour.Namespace, names.ServiceAccount, err)
	}

	// Ensure role & binding.
	if _, err := objrole.EnsureCertgenRole(ctx, cli, names.Role, contour); err != nil {
		return fmt.Errorf("failed to ensure certgen role %s/%s: %w", contour.Namespace, names.Role, err)
	}
	if err := objrb.EnsureRoleBinding(ctx, cli, names.RoleBinding, names.ServiceAccount, names.Role, contour); err != nil {
		return fmt.Errorf("failed to ensure certgen role binding %s/%s: %w", contour.Namespace, names.RoleBinding, err)
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
		contour.CertgenRBACNames(),
	} {
		if len(name.RoleBinding) > 0 {
			rolebinding, err := objrb.CurrentRoleBinding(ctx, cli, contour.Namespace, name.RoleBinding)
			if err != nil && !errors.IsNotFound(err) {
				return err
			}
			if rolebinding != nil {
				deletions = append(deletions, rolebinding)
			}
		}

		if len(name.Role) > 0 {
			role, err := objrole.CurrentRole(ctx, cli, contour.Namespace, name.Role)
			if err != nil && !errors.IsNotFound(err) {
				return err
			}
			if role != nil {
				deletions = append(deletions, role)
			}
		}

		if len(name.ClusterRoleBinding) > 0 {
			clusterrolebinding, err := objcrb.CurrentClusterRoleBinding(ctx, cli, name.ClusterRoleBinding)
			if err != nil && !errors.IsNotFound(err) {
				return err
			}
			if clusterrolebinding != nil {
				deletions = append(deletions, clusterrolebinding)
			}
		}

		if len(name.ClusterRole) > 0 {
			clusterrole, err := objcr.CurrentClusterRole(ctx, cli, name.ClusterRole)
			if err != nil && !errors.IsNotFound(err) {
				return err
			}
			if clusterrole != nil {
				deletions = append(deletions, clusterrole)
			}
		}

		if len(name.ServiceAccount) > 0 {
			serviceaccount, err := objsa.CurrentServiceAccount(ctx, cli, contour.Namespace, name.ServiceAccount)
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
