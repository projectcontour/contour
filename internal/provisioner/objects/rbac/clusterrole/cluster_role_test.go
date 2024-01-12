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

package clusterrole

import (
	"fmt"
	"slices"
	"testing"

	"github.com/projectcontour/contour/internal/provisioner/model"

	rbacv1 "k8s.io/api/rbac/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
)

func checkClusterRoleName(t *testing.T, cr *rbacv1.ClusterRole, expected string) {
	t.Helper()

	if cr.Name == expected {
		return
	}

	t.Errorf("cluster role has unexpected name %q", cr.Name)
}

func checkClusterRoleLabels(t *testing.T, cr *rbacv1.ClusterRole, expected map[string]string) {
	t.Helper()

	if apiequality.Semantic.DeepEqual(cr.Labels, expected) {
		return
	}

	t.Errorf("cluster role has unexpected %q labels", cr.Labels)
}

func clusterRoleRulesContainOnlyGatewayClass(cr *rbacv1.ClusterRole) bool {
	for _, r := range cr.Rules {
		if !slices.Contains(r.Resources, "gatewayclasses") &&
			!slices.Contains(r.Resources, "gatewayclasses/status") {
			return false
		}
	}

	return true
}

func TestDesiredClusterRole(t *testing.T) {
	testCases := []struct {
		description      string
		gatewayclassOnly bool
	}{
		{
			description:      "gateway class rule only role",
			gatewayclassOnly: true,
		},
		{
			description:      "generic cluster role include all rules",
			gatewayclassOnly: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			name := "test-cr"
			cntr := model.Default(fmt.Sprintf("%s-ns", name), name)
			cr := desiredClusterRole(name, cntr, tc.gatewayclassOnly)
			checkClusterRoleName(t, cr, name)
			ownerLabels := map[string]string{
				model.ContourOwningGatewayNameLabel:    cntr.Name,
				model.GatewayAPIOwningGatewayNameLabel: cntr.Name,
			}
			checkClusterRoleLabels(t, cr, ownerLabels)
			fmt.Println(cr.Rules)
			if tc.gatewayclassOnly != clusterRoleRulesContainOnlyGatewayClass(cr) {
				t.Errorf("expect gateayClassOnly to be %v, but clusterRoleRulesContainGatewayClass shows %v",
					tc.gatewayclassOnly, clusterRoleRulesContainOnlyGatewayClass(cr))
			}
		})
	}
}
