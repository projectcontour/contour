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

package clusterrolebinding

import (
	"context"
	"fmt"

	"github.com/projectcontour/contour/internal/provisioner/equality"
	"github.com/projectcontour/contour/internal/provisioner/labels"
	"github.com/projectcontour/contour/internal/provisioner/model"
	"github.com/projectcontour/contour/internal/provisioner/objects"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// EnsureClusterRoleBinding ensures a ClusterRoleBinding resource with the provided
// name exists, using roleRef for the role reference, svcAct for the subject and
// the contour namespace/name for the owning contour labels.
func EnsureClusterRoleBinding(ctx context.Context, cli client.Client, name, roleRef, svcAct string, contour *model.Contour) error {
	desired := desiredClusterRoleBinding(name, roleRef, svcAct, contour)

	// Enclose contour.
	updater := func(ctx context.Context, cli client.Client, current, desired *rbacv1.ClusterRoleBinding) error {
		return updateClusterRoleBindingIfNeeded(ctx, cli, contour, current, desired)
	}

	return objects.EnsureObject(ctx, cli, desired, updater, &rbacv1.ClusterRoleBinding{})
}

// desiredClusterRoleBinding constructs an instance of the desired ClusterRoleBinding
// resource with the provided name, contour namespace/name for the owning contour
// labels, roleRef for the role reference, and svcAcctRef for the subject.
func desiredClusterRoleBinding(name, roleRef, svcAcctRef string, contour *model.Contour) *rbacv1.ClusterRoleBinding {
	crb := &rbacv1.ClusterRoleBinding{
		TypeMeta: metav1.TypeMeta{
			Kind: "RoleBinding",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Labels:      contour.CommonLabels(),
			Annotations: contour.CommonAnnotations(),
		},
	}
	crb.Subjects = []rbacv1.Subject{
		{
			Kind:      "ServiceAccount",
			APIGroup:  corev1.GroupName,
			Name:      svcAcctRef,
			Namespace: contour.Namespace,
		},
	}
	crb.RoleRef = rbacv1.RoleRef{
		APIGroup: rbacv1.GroupName,
		Kind:     "ClusterRole",
		Name:     roleRef,
	}
	return crb
}

// updateClusterRoleBindingIfNeeded updates a ClusterRoleBinding resource if current
// does not match desired, using contour to verify the existence of owner labels.
func updateClusterRoleBindingIfNeeded(ctx context.Context, cli client.Client, contour *model.Contour, current, desired *rbacv1.ClusterRoleBinding) error {
	if labels.AnyExist(current, model.OwnerLabels(contour)) {
		crb, updated := equality.ClusterRoleBindingConfigChanged(current, desired)
		if updated {
			if err := cli.Update(ctx, crb); err != nil {
				return fmt.Errorf("failed to update cluster role binding %s: %w", crb.Name, err)
			}
			return nil
		}
	}
	return nil
}
