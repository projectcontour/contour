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

package role

import (
	"fmt"
	"testing"

	operatorv1alpha1 "github.com/projectcontour/contour-operator/api/v1alpha1"
	objcontour "github.com/projectcontour/contour-operator/internal/objects/contour"

	rbacv1 "k8s.io/api/rbac/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
)

func checkRoleName(t *testing.T, role *rbacv1.Role, expected string) {
	t.Helper()

	if role.Name == expected {
		return
	}

	t.Errorf("role %q has unexpected name", role.Name)
}

func checkRoleLabels(t *testing.T, role *rbacv1.Role, expected map[string]string) {
	t.Helper()

	if apiequality.Semantic.DeepEqual(role.Labels, expected) {
		return
	}

	t.Errorf("role has unexpected %q labels", role.Labels)
}

func TestDesiredCertgenRole(t *testing.T) {
	name := "role-test"
	cfg := objcontour.Config{
		Name:        name,
		Namespace:   fmt.Sprintf("%s-ns", name),
		SpecNs:      "projectcontour",
		RemoveNs:    false,
		NetworkType: operatorv1alpha1.LoadBalancerServicePublishingType,
	}
	cntr := objcontour.New(cfg)
	role := desiredCertgenRole(name, cntr)
	checkRoleName(t, role, name)
	ownerLabels := map[string]string{
		operatorv1alpha1.OwningContourNameLabel: cntr.Name,
		operatorv1alpha1.OwningContourNsLabel:   cntr.Namespace,
	}
	checkRoleLabels(t, role, ownerLabels)
}

func TestDesiredControllerRole(t *testing.T) {
	name := "role-test"
	cfg := objcontour.Config{
		Name:        name,
		Namespace:   fmt.Sprintf("%s-ns", name),
		SpecNs:      "projectcontour",
		RemoveNs:    false,
		NetworkType: operatorv1alpha1.LoadBalancerServicePublishingType,
	}
	cntr := objcontour.New(cfg)
	role := desiredControllerRole(name, cntr)
	checkRoleName(t, role, name)
	ownerLabels := map[string]string{
		operatorv1alpha1.OwningContourNameLabel: cntr.Name,
		operatorv1alpha1.OwningContourNsLabel:   cntr.Namespace,
	}
	checkRoleLabels(t, role, ownerLabels)
}
