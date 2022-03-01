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

package namespace

import (
	"context"
	"fmt"

	operatorv1alpha1 "github.com/projectcontour/contour-operator/api/v1alpha1"
	"github.com/projectcontour/contour-operator/internal/equality"
	objcontour "github.com/projectcontour/contour-operator/internal/objects/contour"
	"github.com/projectcontour/contour-operator/pkg/labels"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// namespaceCoreList is a list of namespace names that should not be removed.
var namespaceCoreList = []string{"contour-operator", "default", "kube-system"}

// EnsureNamespace ensures the namespace for the provided name exists.
func EnsureNamespace(ctx context.Context, cli client.Client, contour *operatorv1alpha1.Contour) error {
	desired := DesiredNamespace(contour)
	current, err := currentSpecNsName(ctx, cli, contour.Spec.Namespace.Name)
	if err != nil {
		if errors.IsNotFound(err) {
			return createNamespace(ctx, cli, desired)
		}
		return fmt.Errorf("failed to get namespace %s: %w", desired.Name, err)
	}

	if err := updateNamespaceIfNeeded(ctx, cli, contour, current, desired); err != nil {
		return fmt.Errorf("failed to update namespace %s: %w", desired.Name, err)
	}

	return nil
}

// EnsureNamespaceDeleted ensures the namespace for the provided contour is removed,
// bypassing deletion if any of the following conditions apply:
//   - RemoveOnDeletion is unspecified or set to false.
//   - Another contour exists in the same namespace.
//   - The namespace of contour matches a name in namespaceCoreList.
//   - The namespace does not contain the Contour owner labels.
// Returns a boolean indicating if the delete was expected to occur and an error.
func EnsureNamespaceDeleted(ctx context.Context, cli client.Client, contour *operatorv1alpha1.Contour) (bool, error) {
	name := contour.Spec.Namespace.Name
	if !contour.Spec.Namespace.RemoveOnDeletion {
		return false, nil
	}
	for _, ns := range namespaceCoreList {
		if name == ns {
			return false, nil
		}
	}
	ns, err := currentSpecNsName(ctx, cli, name)
	if err != nil {
		if errors.IsNotFound(err) {
			return true, nil
		}
		return true, err
	}
	if labels.Exist(ns, objcontour.OwnerLabels(contour)) {
		contoursExist, err := objcontour.OtherContoursExistInSpecNs(ctx, cli, contour)
		if err != nil {
			return true, fmt.Errorf("failed to verify if contours exist in namespace %s: %w", name, err)
		}
		if !contoursExist {
			if err := cli.Delete(ctx, ns); err != nil {
				if errors.IsNotFound(err) {
					return true, nil
				}
				return true, fmt.Errorf("failed to delete namespace %s: %w", ns.Name, err)
			}
		}
	}
	return true, nil
}

// DesiredNamespace returns the desired Namespace resource for the provided contour.
func DesiredNamespace(contour *operatorv1alpha1.Contour) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   contour.Spec.Namespace.Name,
			Labels: objcontour.OwnerLabels(contour),
		},
	}
}

// createNamespace creates a Namespace resource for the provided ns.
func createNamespace(ctx context.Context, cli client.Client, ns *corev1.Namespace) error {
	if err := cli.Create(ctx, ns); err != nil {
		return fmt.Errorf("failed to create namespace %s: %w", ns.Name, err)
	}
	return nil
}

// currentSpecNsName returns the Namespace resource for spec.namespace.name of
// the provided contour.
func currentSpecNsName(ctx context.Context, cli client.Client, name string) (*corev1.Namespace, error) {
	current := &corev1.Namespace{}
	key := types.NamespacedName{Name: name}
	err := cli.Get(ctx, key, current)
	if err != nil {
		return nil, err
	}
	return current, nil
}

// updateNamespaceIfNeeded updates a Namespace if current does not match desired,
// using contour to verify the existence of owner labels.
func updateNamespaceIfNeeded(ctx context.Context, cli client.Client, contour *operatorv1alpha1.Contour, current, desired *corev1.Namespace) error {
	if labels.Exist(current, objcontour.OwnerLabels(contour)) {
		ns, updated := equality.NamespaceConfigChanged(current, desired)
		if updated {
			if err := cli.Update(ctx, ns); err != nil {
				return fmt.Errorf("failed to update namespace %s: %w", ns.Name, err)
			}
			return nil
		}
	}
	return nil
}
