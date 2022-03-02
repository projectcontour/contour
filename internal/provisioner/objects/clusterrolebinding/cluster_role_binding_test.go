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
	"fmt"
	"testing"

	"github.com/projectcontour/contour/internal/provisioner/model"

	rbacv1 "k8s.io/api/rbac/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
)

func checkClusterRoleBindingName(t *testing.T, crb *rbacv1.ClusterRoleBinding, expected string) {
	t.Helper()

	if crb.Name == expected {
		return
	}

	t.Errorf("cluster role binding has unexpected name %q", crb.Name)
}

func checkClusterRoleBindingLabels(t *testing.T, crb *rbacv1.ClusterRoleBinding, expected map[string]string) {
	t.Helper()

	if apiequality.Semantic.DeepEqual(crb.Labels, expected) {
		return
	}

	t.Errorf("cluster role binding has unexpected %q labels", crb.Labels)
}

func checkClusterRoleBindingSvcAcct(t *testing.T, crb *rbacv1.ClusterRoleBinding, name, ns string) {
	t.Helper()

	if crb.Subjects[0].Name == name && crb.Subjects[0].Namespace == ns {
		return
	}

	t.Errorf("cluster role binding has unexpected %q/%q service account reference", crb.Subjects[0].Name, crb.Subjects[0].Namespace)
}

func checkClusterRoleBindingRole(t *testing.T, crb *rbacv1.ClusterRoleBinding, expected string) {
	t.Helper()

	if crb.RoleRef.Name == expected {
		return
	}

	t.Errorf("cluster role binding has unexpected %q role reference", crb.Subjects[0].Name)
}

func TestDesiredClusterRoleBinding(t *testing.T) {
	crbName := "test-crb"
	cfg := model.Config{
		Name:        crbName,
		Namespace:   fmt.Sprintf("%s-ns", crbName),
		SpecNs:      "projectcontour",
		RemoveNs:    true,
		NetworkType: model.LoadBalancerServicePublishingType,
	}
	cntr := model.New(cfg)
	testSvcAcct := "test-svc-acct-ref"
	testRoleRef := "test-role-ref"
	crb := desiredClusterRoleBinding(crbName, testRoleRef, testSvcAcct, cntr)
	checkClusterRoleBindingName(t, crb, crbName)
	ownerLabels := map[string]string{
		model.OwningContourNameLabel: cntr.Name,
		model.OwningContourNsLabel:   cntr.Namespace,
	}
	checkClusterRoleBindingLabels(t, crb, ownerLabels)
	checkClusterRoleBindingSvcAcct(t, crb, testSvcAcct, cntr.Spec.Namespace.Name)
	checkClusterRoleBindingRole(t, crb, testRoleRef)
}
