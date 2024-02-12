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

	core_v1 "k8s.io/api/core/v1"
	rbac_v1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/provisioner/model"
	"github.com/projectcontour/contour/internal/provisioner/objects"
	"github.com/projectcontour/contour/internal/provisioner/objects/rbac/clusterrole"
	"github.com/projectcontour/contour/internal/provisioner/objects/rbac/clusterrolebinding"
	"github.com/projectcontour/contour/internal/provisioner/objects/rbac/role"
	"github.com/projectcontour/contour/internal/provisioner/objects/rbac/rolebinding"
	"github.com/projectcontour/contour/internal/provisioner/objects/rbac/serviceaccount"
	"github.com/projectcontour/contour/internal/provisioner/slice"
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

	// By default, Contour watches all namespaces, use default cluster role and rolebinding
	clusterRoleForClusterScopedResourcesOnly := true
	if contour.WatchAllNamespaces() {
		// Ensure cluster role & binding.
		if err := clusterrole.EnsureClusterRole(ctx, cli, names.ClusterRole, contour, !clusterRoleForClusterScopedResourcesOnly); err != nil {
			return fmt.Errorf("failed to ensure cluster role %s: %w", names.ClusterRole, err)
		}
		if err := clusterrolebinding.EnsureClusterRoleBinding(ctx, cli, names.ClusterRoleBinding, names.ClusterRole, names.ServiceAccount, contour); err != nil {
			return fmt.Errorf("failed to ensure cluster role binding %s: %w", names.ClusterRoleBinding, err)
		}
	} else {
		// validate whether all namespaces exist
		if err := validateNamespacesExist(ctx, cli, contour.Spec.WatchNamespaces); err != nil {
			return fmt.Errorf("failed when validating watchNamespaces:%w", err)
		}
		// Ensure cluster role & cluster binding for gatewayclass and other cluster scope resource first since it's cluster scope variables
		if err := clusterrole.EnsureClusterRole(ctx, cli, names.ClusterRole, contour, clusterRoleForClusterScopedResourcesOnly); err != nil {
			return fmt.Errorf("failed to ensure cluster role %s: %w", names.ClusterRole, err)
		}
		if err := clusterrolebinding.EnsureClusterRoleBinding(ctx, cli, names.ClusterRoleBinding, names.ClusterRole, names.ServiceAccount, contour); err != nil {
			return fmt.Errorf("failed to ensure cluster role binding %s: %w", names.ClusterRoleBinding, err)
		}

		// includes contour's namespace if it's not inside watchNamespaces
		ns := model.NamespacesToStrings(contour.Spec.WatchNamespaces)
		if !slice.ContainsString(ns, contour.Namespace) {
			ns = append(ns, contour.Namespace)
		}
		// Ensures role and rolebinding for namespaced scope resources in namespaces specified in contour.spec.watchNamespaces variable and contour's namespace
		if err := role.EnsureRolesInNamespaces(ctx, cli, names.NamespaceScopedResourceRole, contour, ns); err != nil {
			return fmt.Errorf("failed to ensure roles in namespace %s: %w", contour.Spec.WatchNamespaces, err)
		}
		if err := rolebinding.EnsureRoleBindingsInNamespaces(ctx, cli, names.NamespaceScopedResourceRoleBinding, names.ServiceAccount, names.NamespaceScopedResourceRole, contour, ns); err != nil {
			return fmt.Errorf("failed to ensure rolebindings in namespace %s: %w", contour.Spec.WatchNamespaces, err)
		}
	}

	// Ensure role & binding.
	if err := role.EnsureControllerRole(ctx, cli, names.Role, contour); err != nil {
		return fmt.Errorf("failed to ensure controller role %s/%s: %w", contour.Namespace, names.Role, err)
	}
	if err := rolebinding.EnsureControllerRoleBinding(ctx, cli, names.RoleBinding, names.ServiceAccount, names.Role, contour); err != nil {
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
			deletions = append(deletions, &rbac_v1.RoleBinding{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: contour.Namespace,
					Name:      name.RoleBinding,
				},
			})
		}

		if len(name.Role) > 0 {
			deletions = append(deletions, &rbac_v1.Role{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: contour.Namespace,
					Name:      name.Role,
				},
			})
		}

		if len(name.ClusterRoleBinding) > 0 {
			deletions = append(deletions, &rbac_v1.ClusterRoleBinding{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: contour.Namespace,
					Name:      name.ClusterRoleBinding,
				},
			})
		}

		if len(name.ClusterRole) > 0 {
			deletions = append(deletions, &rbac_v1.ClusterRole{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: contour.Namespace,
					Name:      name.ClusterRole,
				},
			})
		}

		if len(name.ServiceAccount) > 0 {
			deletions = append(deletions, &core_v1.ServiceAccount{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: contour.Namespace,
					Name:      name.ServiceAccount,
				},
			})
		}
	}

	for _, deletion := range deletions {
		if err := objects.EnsureObjectDeleted(ctx, cli, deletion, contour); err != nil {
			return err
		}
	}

	return nil
}

func validateNamespacesExist(ctx context.Context, cli client.Client, ns []contour_v1.Namespace) error {
	errs := []error{}
	for _, n := range ns {
		namespace := &core_v1.Namespace{}
		// Check if the namespace exists
		err := cli.Get(ctx, types.NamespacedName{Name: string(n)}, namespace)
		if err != nil {
			if apierrors.IsNotFound(err) {
				errs = append(errs, fmt.Errorf("failed to find namespace %s in watchNamespace. Please make sure it exist", n))
			} else {
				errs = append(errs, fmt.Errorf("failed to get namespace %s because of: %w", n, err))
			}
		}
	}

	return kerrors.NewAggregate(errs)
}
