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
	"fmt"
	"testing"

	operatorv1alpha1 "github.com/projectcontour/contour-operator/api/v1alpha1"
	objcontour "github.com/projectcontour/contour-operator/internal/objects/contour"

	rbacv1 "k8s.io/api/rbac/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
)

func checkRoleBindingName(t *testing.T, rb *rbacv1.RoleBinding, expected string) {
	t.Helper()

	if rb.Name == expected {
		return
	}

	t.Errorf("role binding %q has unexpected name", rb.Name)
}

func checkRoleBindingLabels(t *testing.T, rb *rbacv1.RoleBinding, expected map[string]string) {
	t.Helper()

	if apiequality.Semantic.DeepEqual(rb.Labels, expected) {
		return
	}

	t.Errorf("role binding has unexpected %q labels", rb.Labels)
}

func checkRoleBindingSvcAcct(t *testing.T, rb *rbacv1.RoleBinding, name, ns string) {
	t.Helper()

	if rb.Subjects[0].Name == name && rb.Subjects[0].Namespace == ns {
		return
	}

	t.Errorf("role binding has unexpected %q/%q service account reference", rb.Subjects[0].Name, rb.Subjects[0].Namespace)
}

func checkRoleBindingRole(t *testing.T, rb *rbacv1.RoleBinding, expected string) {
	t.Helper()

	if rb.RoleRef.Name == expected {
		return
	}

	t.Errorf("role binding has unexpected %q role reference", rb.Subjects[0].Name)
}

func TestDesiredRoleBinding(t *testing.T) {
	name := "job-test"
	cfg := objcontour.Config{
		Name:        name,
		Namespace:   fmt.Sprintf("%s-ns", name),
		SpecNs:      "projectcontour",
		RemoveNs:    false,
		NetworkType: operatorv1alpha1.LoadBalancerServicePublishingType,
	}
	cntr := objcontour.New(cfg)
	cntr.Spec.Namespace.Name = "test-rb-ns"
	rbName := "test-rb"
	svcAcct := "test-svc-acct-ref"
	roleRef := "test-role-ref"
	rb := desiredRoleBinding(rbName, svcAcct, roleRef, cntr)
	checkRoleBindingName(t, rb, rbName)
	ownerLabels := map[string]string{
		operatorv1alpha1.OwningContourNameLabel: cntr.Name,
		operatorv1alpha1.OwningContourNsLabel:   cntr.Namespace,
	}
	checkRoleBindingLabels(t, rb, ownerLabels)
	checkRoleBindingSvcAcct(t, rb, svcAcct, cntr.Spec.Namespace.Name)
	checkRoleBindingRole(t, rb, roleRef)
}
