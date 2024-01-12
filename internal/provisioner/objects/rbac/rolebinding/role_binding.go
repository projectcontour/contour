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
	"github.com/projectcontour/contour/internal/provisioner/objects"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// EnsureRoleBinding ensures a RoleBinding resource exists with the provided
// ns/name and using contour namespace/name for the owning contour labels.
// The RoleBinding will use svcAct for the subject and role for the role reference.
func EnsureRoleBinding(ctx context.Context, cli client.Client, name, svcAct, role string, contour *model.Contour) error {
	desired := desiredRoleBindingInNamespace(name, svcAct, role, contour.Namespace, contour)

	// Enclose contour.
	updater := func(ctx context.Context, cli client.Client, current, desired *rbacv1.RoleBinding) error {
		return updateRoleBindingIfNeeded(ctx, cli, contour, current, desired)
	}

	return objects.EnsureObject(ctx, cli, desired, updater, &rbacv1.RoleBinding{})
}

// EnsureRoleBindingsInNamespaces ensures a set of RoleBindings resource exist with the provided
// namespaces/name using contour namespace/name for the owning contour labels.
// The RoleBindings will use same svcAct for the subject and role for the role reference.
func EnsureRoleBindingsInNamespaces(ctx context.Context, cli client.Client, name, svcAct, role string, contour *model.Contour, namespaces []string) error {
	errs := []error{}
	for _, ns := range namespaces {
		desired := desiredRoleBindingInNamespace(name, svcAct, role, ns, contour)

		// Enclose contour.
		updater := func(ctx context.Context, cli client.Client, current, desired *rbacv1.RoleBinding) error {
			return updateRoleBindingIfNeeded(ctx, cli, contour, current, desired)
		}
		err := objects.EnsureObject(ctx, cli, desired, updater, &rbacv1.RoleBinding{})
		errs = append(errs, err)
	}

	return kerrors.NewAggregate(errs)
}

// desiredRoleBindingInNamespace constructs an instance of the desired RoleBinding resource
// with the provided name in provided namespace, using contour namespace/name
// for the owning contour labels. The RoleBinding will use svcAct for the subject
// and role for the role reference.
func desiredRoleBindingInNamespace(name, svcAcctRef, roleRef, namespace string, contour *model.Contour) *rbacv1.RoleBinding {
	rb := &rbacv1.RoleBinding{
		TypeMeta: metav1.TypeMeta{
			Kind: "RoleBinding",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   namespace,
			Name:        name,
			Labels:      contour.CommonLabels(),
			Annotations: contour.CommonAnnotations(),
		},
	}
	rb.Subjects = []rbacv1.Subject{{
		Kind:     "ServiceAccount",
		APIGroup: corev1.GroupName,
		Name:     svcAcctRef,
		// service account will be the same one
		Namespace: contour.Namespace,
	}}
	rb.RoleRef = rbacv1.RoleRef{
		APIGroup: rbacv1.GroupName,
		Kind:     "Role",
		Name:     roleRef,
	}

	return rb
}

// updateRoleBindingIfNeeded updates a RoleBinding resource if current does
// not match desired.
func updateRoleBindingIfNeeded(ctx context.Context, cli client.Client, contour *model.Contour, current, desired *rbacv1.RoleBinding) error {
	if labels.AnyExist(current, model.OwnerLabels(contour)) {
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
