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

package rolebinding

import (
	"context"
	"fmt"

	equality "github.com/projectcontour/contour/internal/provisioner/equality"
	"github.com/projectcontour/contour/internal/provisioner/labels"
	"github.com/projectcontour/contour/internal/provisioner/model"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// EnsureRoleBinding ensures a RoleBinding resource exists with the provided
// ns/name and contour namespace/name for the owning contour labels.
// The RoleBinding will use svcAct for the subject and role for the role reference.
func EnsureRoleBinding(ctx context.Context, cli client.Client, name, svcAct, role string, contour *model.Contour) error {
	desired := desiredRoleBinding(name, svcAct, role, contour)
	current, err := CurrentRoleBinding(ctx, cli, contour.Namespace, name)
	if err != nil {
		if errors.IsNotFound(err) {
			if err := createRoleBinding(ctx, cli, desired); err != nil {
				return fmt.Errorf("failed to create role binding %s/%s: %w", desired.Namespace, desired.Name, err)
			}
			return nil
		}
		return fmt.Errorf("failed to get role binding %s/%s: %w", desired.Namespace, desired.Name, err)
	}

	if err := updateRoleBindingIfNeeded(ctx, cli, contour, current, desired); err != nil {
		return fmt.Errorf("failed to update role binding %s/%s: %w", desired.Namespace, desired.Name, err)
	}

	return nil
}

// desiredRoleBinding constructs an instance of the desired RoleBinding resource
// with the provided name in Contour spec Namespace, using contour namespace/name
// for the owning contour labels. The RoleBinding will use svcAct for the subject
// and role for the role reference.
func desiredRoleBinding(name, svcAcctRef, roleRef string, contour *model.Contour) *rbacv1.RoleBinding {
	rb := &rbacv1.RoleBinding{
		TypeMeta: metav1.TypeMeta{
			Kind: "RoleBinding",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: contour.Namespace,
			Name:      name,
			Labels:    model.CommonLabels(contour),
		},
	}
	rb.Subjects = []rbacv1.Subject{{
		Kind:      "ServiceAccount",
		APIGroup:  corev1.GroupName,
		Name:      svcAcctRef,
		Namespace: contour.Namespace,
	}}
	rb.RoleRef = rbacv1.RoleRef{
		APIGroup: rbacv1.GroupName,
		Kind:     "Role",
		Name:     roleRef,
	}

	return rb
}

// CurrentRoleBinding returns the current RoleBinding for the provided ns/name.
func CurrentRoleBinding(ctx context.Context, cli client.Client, ns, name string) (*rbacv1.RoleBinding, error) {
	current := &rbacv1.RoleBinding{}
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

// createRoleBinding creates a RoleBinding resource for the provided rb.
func createRoleBinding(ctx context.Context, cli client.Client, rb *rbacv1.RoleBinding) error {
	if err := cli.Create(ctx, rb); err != nil {
		return fmt.Errorf("failed to create role binding %s/%s: %w", rb.Namespace, rb.Name, err)
	}
	return nil
}

// updateRoleBindingIfNeeded updates a RoleBinding resource if current does
// not match desired.
func updateRoleBindingIfNeeded(ctx context.Context, cli client.Client, contour *model.Contour, current, desired *rbacv1.RoleBinding) error {
	if labels.Exist(current, model.OwnerLabels(contour)) {
		rb, updated := equality.RoleBindingConfigChanged(current, desired)
		if updated {
			if err := cli.Update(ctx, rb); err != nil {
				return fmt.Errorf("failed to update role binding %s/%s: %w", rb.Namespace, rb.Name, err)
			}
			return nil
		}
	}
	return nil
}
