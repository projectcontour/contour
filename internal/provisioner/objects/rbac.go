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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// ContourRbacName is the name used for Contour RBAC resources.
	ContourRbacName = "contour"
	// ContourRoleBindingName is a special case RoleBinding name since
	// the certgen RoleBinding name is "contour"
	ContourRoleBindingName = "contour-rolebinding"
	// EnvoyRbacName is the name used for Envoy RBAC resources.
	EnvoyRbacName = "envoy"
	// CertGenRbacName is the name used for Contour certificate
	// generation RBAC resources.
	CertGenRbacName = "contour-certgen"
)

// EnsureRBAC ensures all the necessary RBAC resources exist for the
// provided contour.
func EnsureRBAC(ctx context.Context, cli client.Client, contour *model.Contour) error {
	ns := contour.Namespace
	names := []string{ContourRbacName, EnvoyRbacName, CertGenRbacName}
	for _, name := range names {
		_, err := objsa.EnsureServiceAccount(ctx, cli, name, contour)
		if err != nil {
			return fmt.Errorf("failed to ensure service account %s/%s: %w", ns, name, err)
		}
	}
	// ClusterRole and ClusterRoleBinding resources are namespace-named to allow ownership
	// from individual instances of Contour.
	nsName := fmt.Sprintf("%s-%s", ContourRbacName, ns)
	cr, err := objcr.EnsureClusterRole(ctx, cli, nsName, contour)
	if err != nil {
		return fmt.Errorf("failed to ensure cluster role %s: %w", ContourRbacName, err)
	}
	if err := objcrb.EnsureClusterRoleBinding(ctx, cli, nsName, cr.Name, ContourRbacName, contour); err != nil {
		return fmt.Errorf("failed to ensure cluster role binding %s: %w", ContourRbacName, err)
	}
	certRole, err := objrole.EnsureCertgenRole(ctx, cli, CertGenRbacName, contour)
	if err != nil {
		return fmt.Errorf("failed to ensure certgen role %s/%s: %w", ns, CertGenRbacName, err)
	}
	if err := objrb.EnsureRoleBinding(ctx, cli, ContourRbacName, CertGenRbacName, certRole.Name, contour); err != nil {
		return fmt.Errorf("failed to ensure certgen role binding %s/%s: %w", ns, ContourRbacName, err)
	}
	controllerRole, err := objrole.EnsureControllerRole(ctx, cli, ContourRbacName, contour)
	if err != nil {
		return fmt.Errorf("failed to ensure controller role %s/%s: %w", ns, ContourRbacName, err)
	}
	if err := objrb.EnsureRoleBinding(ctx, cli, ContourRoleBindingName, ContourRbacName, controllerRole.Name, contour); err != nil {
		return fmt.Errorf("failed to ensure controller role binding %s/%s: %w", ns, ContourRbacName, err)
	}
	return nil
}

// EnsureRBACDeleted ensures all the necessary RBAC resources for the provided
// contour are deleted if Contour owner labels exist.
func EnsureRBACDeleted(ctx context.Context, cli client.Client, contour *model.Contour) error {
	var errs []error
	ns := contour.Namespace
	objectsToDelete := []client.Object{}

	// TODO(sk) right now we can't support running more than one Contour instance
	// per namespace, so just assume there's only one.
	otherContoursExistInSpecNs := false

	if !otherContoursExistInSpecNs {
		controllerRoleBind, err := objrb.CurrentRoleBinding(ctx, cli, ns, ContourRoleBindingName)
		if err != nil && !errors.IsNotFound(err) {
			return err
		}
		if controllerRoleBind != nil {
			objectsToDelete = append(objectsToDelete, controllerRoleBind)
		}
		controllerRole, err := objrole.CurrentRole(ctx, cli, ns, ContourRbacName)
		if err != nil && !errors.IsNotFound(err) {
			return err
		}
		if controllerRole != nil {
			objectsToDelete = append(objectsToDelete, controllerRole)
		}
		certRoleBind, err := objrb.CurrentRoleBinding(ctx, cli, ns, ContourRbacName)
		if err != nil && !errors.IsNotFound(err) {
			return err
		}
		if certRoleBind != nil {
			objectsToDelete = append(objectsToDelete, certRoleBind)
		}
		certRole, err := objrole.CurrentRole(ctx, cli, ns, CertGenRbacName)
		if err != nil && !errors.IsNotFound(err) {
			return err
		}
		if certRole != nil {
			objectsToDelete = append(objectsToDelete, certRole)
		}
		names := []string{ContourRbacName, EnvoyRbacName, CertGenRbacName}
		for _, name := range names {
			svcAct, err := objsa.CurrentServiceAccount(ctx, cli, ns, name)
			if err != nil && !errors.IsNotFound(err) {
				return err
			}
			if svcAct != nil {
				objectsToDelete = append(objectsToDelete, svcAct)
			}
		}
	}

	// ClusterRole and ClusterRoleBinding resources are namespace-named to allow ownership
	// from individual instances of Contour.
	nsName := fmt.Sprintf("%s-%s", ContourRbacName, contour.Namespace)
	crb, err := objcrb.CurrentClusterRoleBinding(ctx, cli, nsName)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}
	if crb != nil {
		objectsToDelete = append(objectsToDelete, crb)
	}
	cr, err := objcr.CurrentClusterRole(ctx, cli, nsName)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}
	if cr != nil {
		objectsToDelete = append(objectsToDelete, cr)
	}

	for _, object := range objectsToDelete {
		kind := object.GetObjectKind().GroupVersionKind().Kind
		namespace := object.(metav1.Object).GetNamespace()
		name := object.(metav1.Object).GetName()
		if labels.Exist(object, model.OwnerLabels(contour)) {
			if err := cli.Delete(ctx, object); err != nil {
				if errors.IsNotFound(err) {
					continue
				}
				return fmt.Errorf("failed to delete %s %s/%s: %w", kind, namespace, name, err)
			}
		}
	}
	return utilerrors.NewAggregate(errs)
}
