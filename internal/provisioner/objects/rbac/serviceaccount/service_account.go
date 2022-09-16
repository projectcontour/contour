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

package serviceaccount

import (
	"context"
	"fmt"

	utilequality "github.com/projectcontour/contour/internal/provisioner/equality"
	"github.com/projectcontour/contour/internal/provisioner/labels"
	"github.com/projectcontour/contour/internal/provisioner/model"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// EnsureServiceAccount ensures a ServiceAccount resource exists with the provided name
// and contour namespace/name for the owning contour labels.
func EnsureServiceAccount(ctx context.Context, cli client.Client, name string, contour *model.Contour) (*corev1.ServiceAccount, error) {
	desired := DesiredServiceAccount(name, contour)
	current, err := CurrentServiceAccount(ctx, cli, contour.Namespace, name)
	if err != nil {
		if errors.IsNotFound(err) {
			updated, err := createServiceAccount(ctx, cli, desired)
			if err != nil {
				return nil, fmt.Errorf("failed to create service account %s/%s: %w", desired.Namespace, desired.Name, err)
			}
			return updated, nil
		}
		return nil, fmt.Errorf("failed to get service account %s/%s: %w", desired.Namespace, desired.Name, err)
	}
	updated, err := updateSvcAcctIfNeeded(ctx, cli, contour, current, desired)
	if err != nil {
		return nil, fmt.Errorf("failed to update service account %s/%s: %w", desired.Namespace, desired.Name, err)
	}
	return updated, nil
}

// DesiredServiceAccount generates the desired ServiceAccount resource for the
// given contour.
func DesiredServiceAccount(name string, contour *model.Contour) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			Kind: rbacv1.ServiceAccountKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: contour.Namespace,
			Name:      name,
			Labels:    model.CommonLabels(contour),
		},
	}
}

// CurrentServiceAccount returns the current ServiceAccount for the provided ns/name.
func CurrentServiceAccount(ctx context.Context, cli client.Client, ns, name string) (*corev1.ServiceAccount, error) {
	current := &corev1.ServiceAccount{}
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

// createServiceAccount creates a ServiceAccount resource for the provided sa.
func createServiceAccount(ctx context.Context, cli client.Client, sa *corev1.ServiceAccount) (*corev1.ServiceAccount, error) {
	if err := cli.Create(ctx, sa); err != nil {
		return nil, fmt.Errorf("failed to create service account %s/%s: %w", sa.Namespace, sa.Name, err)
	}
	return sa, nil
}

// updateSvcAcctIfNeeded updates a ServiceAccount resource if current does not match desired,
// using contour to verify the existence of owner labels.
func updateSvcAcctIfNeeded(ctx context.Context, cli client.Client, contour *model.Contour, current, desired *corev1.ServiceAccount) (*corev1.ServiceAccount, error) {
	if labels.Exist(current, model.OwnerLabels(contour)) {
		sa, updated := utilequality.ServiceAccountConfigChanged(current, desired)
		if updated {
			if err := cli.Update(ctx, sa); err != nil {
				return nil, fmt.Errorf("failed to update service account %s/%s: %w", sa.Namespace, sa.Name, err)
			}
			return sa, nil
		}
	}
	return current, nil
}
