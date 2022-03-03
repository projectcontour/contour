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

	"github.com/projectcontour/contour/internal/provisioner/model"

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
	cfg := model.Config{
		Name:        name,
		Namespace:   fmt.Sprintf("%s-ns", name),
		NetworkType: model.LoadBalancerServicePublishingType,
	}
	cntr := model.New(cfg)
	role := desiredCertgenRole(name, cntr)
	checkRoleName(t, role, name)
	ownerLabels := map[string]string{
		model.OwningGatewayNameLabel: cntr.Name,
	}
	checkRoleLabels(t, role, ownerLabels)
}

func TestDesiredControllerRole(t *testing.T) {
	name := "role-test"
	cfg := model.Config{
		Name:        name,
		Namespace:   fmt.Sprintf("%s-ns", name),
		NetworkType: model.LoadBalancerServicePublishingType,
	}
	cntr := model.New(cfg)
	role := desiredControllerRole(name, cntr)
	checkRoleName(t, role, name)
	ownerLabels := map[string]string{
		model.OwningGatewayNameLabel: cntr.Name,
	}
	checkRoleLabels(t, role, ownerLabels)
}
