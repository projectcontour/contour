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
	"github.com/projectcontour/contour/internal/provisioner/objects"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// EnsureServiceAccount ensures a ServiceAccount resource exists with the provided name
// and contour namespace/name for the owning contour labels.
func EnsureServiceAccount(ctx context.Context, cli client.Client, name string, contour *model.Contour) error {
	desired := desiredServiceAccount(name, contour)

	updater := func(ctx context.Context, cli client.Client, current, desired *corev1.ServiceAccount) error {
		_, err := updateSvcAcctIfNeeded(ctx, cli, contour, current, desired)
		return err
	}

	return objects.EnsureObject(ctx, cli, desired, updater, &corev1.ServiceAccount{})
}

// desiredServiceAccount generates the desired ServiceAccount resource for the
// given contour.
func desiredServiceAccount(name string, contour *model.Contour) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			Kind: rbacv1.ServiceAccountKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   contour.Namespace,
			Name:        name,
			Labels:      contour.CommonLabels(),
			Annotations: contour.CommonAnnotations(),
		},
	}
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
