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

	core_v1 "k8s.io/api/core/v1"
	rbac_v1 "k8s.io/api/rbac/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	equality "github.com/projectcontour/contour/internal/provisioner/equality"
	"github.com/projectcontour/contour/internal/provisioner/labels"
	"github.com/projectcontour/contour/internal/provisioner/model"
	"github.com/projectcontour/contour/internal/provisioner/objects"
)

// EnsureControllerRoleBinding ensures a RoleBinding resource exists with the provided
// ns/name and using contour namespace/name for the owning contour labels.
// The RoleBinding will use svcAct for the subject and role for the role reference.
func EnsureControllerRoleBinding(ctx context.Context, cli client.Client, name, svcAct, role string, contour *model.Contour) error {
	desired := desiredRoleBindingInNamespace(name, svcAct, role, contour.Namespace, contour)

	// Enclose contour.
	updater := func(ctx context.Context, cli client.Client, current, desired *rbac_v1.RoleBinding) error {
		return updateRoleBindingIfNeeded(ctx, cli, contour, current, desired)
	}

	return objects.EnsureObject(ctx, cli, desired, updater, &rbac_v1.RoleBinding{})
}

// EnsureRoleBindingsInNamespaces ensures a set of RoleBinding resources exist with the provided
// namespaces/contour-resource-<name> and using contour namespace/name for the owning contour labels.
// The RoleBindings will use same svcAct for the subject and role for the role reference.
func EnsureRoleBindingsInNamespaces(ctx context.Context, cli client.Client, name, svcAct, role string, contour *model.Contour, namespaces []string) error {
	errs := []error{}
	for _, ns := range namespaces {
		desired := desiredRoleBindingInNamespace(name, svcAct, role, ns, contour)

		// Enclose contour.
		updater := func(ctx context.Context, cli client.Client, current, desired *rbac_v1.RoleBinding) error {
			return updateRoleBindingIfNeeded(ctx, cli, contour, current, desired)
		}
		err := objects.EnsureObject(ctx, cli, desired, updater, &rbac_v1.RoleBinding{})
		errs = append(errs, err)
	}

	return kerrors.NewAggregate(errs)
}

// desiredRoleBindingInNamespace constructs an instance of the desired RoleBinding resource
// with the provided name in provided namespace, using contour namespace/name
// for the owning contour labels. The RoleBinding will use svcAct for the subject
// and role for the role reference.
func desiredRoleBindingInNamespace(name, svcAcctRef, roleRef, namespace string, contour *model.Contour) *rbac_v1.RoleBinding {
	rb := &rbac_v1.RoleBinding{
		TypeMeta: meta_v1.TypeMeta{
			Kind: "RoleBinding",
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace:   namespace,
			Name:        name,
			Labels:      contour.CommonLabels(),
			Annotations: contour.CommonAnnotations(),
		},
	}
	rb.Subjects = []rbac_v1.Subject{{
		Kind:     "ServiceAccount",
		APIGroup: core_v1.GroupName,
		Name:     svcAcctRef,
		// service account will be the same one
		Namespace: contour.Namespace,
	}}
	rb.RoleRef = rbac_v1.RoleRef{
		APIGroup: rbac_v1.GroupName,
		Kind:     "Role",
		Name:     roleRef,
	}

	return rb
}

// updateRoleBindingIfNeeded updates a RoleBinding resource if current does
// not match desired.
func updateRoleBindingIfNeeded(ctx context.Context, cli client.Client, contour *model.Contour, current, desired *rbac_v1.RoleBinding) error {
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
