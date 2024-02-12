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

	rbac_v1 "k8s.io/api/rbac/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"

	"github.com/projectcontour/contour/internal/provisioner/model"
)

func checkRoleBindingName(t *testing.T, rb *rbac_v1.RoleBinding, expected string) {
	t.Helper()

	if rb.Name == expected {
		return
	}

	t.Errorf("role binding %q has unexpected name", rb.Name)
}

func checkRoleBindingNamespace(t *testing.T, rb *rbac_v1.RoleBinding, expected string) {
	t.Helper()

	if rb.Namespace == expected {
		return
	}

	t.Errorf("role binding %q has unexpected namespace", rb.Namespace)
}

func checkRoleBindingLabels(t *testing.T, rb *rbac_v1.RoleBinding, expected map[string]string) {
	t.Helper()

	if apiequality.Semantic.DeepEqual(rb.Labels, expected) {
		return
	}

	t.Errorf("role binding has unexpected %q labels", rb.Labels)
}

func checkRoleBindingSvcAcct(t *testing.T, rb *rbac_v1.RoleBinding, name, ns string) {
	t.Helper()

	if rb.Subjects[0].Name == name && rb.Subjects[0].Namespace == ns {
		return
	}

	t.Errorf("role binding has unexpected %q/%q service account reference", rb.Subjects[0].Name, rb.Subjects[0].Namespace)
}

func checkRoleBindingRole(t *testing.T, rb *rbac_v1.RoleBinding, expected string) {
	t.Helper()

	if rb.RoleRef.Name == expected {
		return
	}

	t.Errorf("role binding has unexpected %q role reference", rb.Subjects[0].Name)
}

func TestDesiredRoleBindingInNamespace(t *testing.T) {
	testCases := []struct {
		description string
		namespace   string
	}{
		{
			description: "namespace 1",
			namespace:   "ns1",
		},
		{
			description: "namespace 2",
			namespace:   "ns2",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			name := "job-test"
			cntr := model.Default(fmt.Sprintf("%s-ns", name), name)
			rbName := "test-rb"
			svcAcct := "test-svc-acct-ref"
			roleRef := "test-role-ref"
			rb := desiredRoleBindingInNamespace(rbName, svcAcct, roleRef, tc.namespace, cntr)
			checkRoleBindingName(t, rb, rbName)
			checkRoleBindingNamespace(t, rb, tc.namespace)
			ownerLabels := map[string]string{
				model.ContourOwningGatewayNameLabel:    cntr.Name,
				model.GatewayAPIOwningGatewayNameLabel: cntr.Name,
			}
			checkRoleBindingLabels(t, rb, ownerLabels)
			checkRoleBindingSvcAcct(t, rb, svcAcct, cntr.Namespace)
			checkRoleBindingRole(t, rb, roleRef)
		})
	}
}
