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

package validation

import (
	"context"
	"testing"

	"github.com/projectcontour/contour/internal/k8s"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayapi_v1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

var (
	ctx = context.TODO()

	defaultController = "projectcontour.io/projectcontour/contour"

	new = &gatewayapi_v1alpha1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "new",
		},
		Spec: gatewayapi_v1alpha1.GatewayClassSpec{
			Controller: defaultController,
		},
	}

	old = &gatewayapi_v1alpha1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "old",
		},
	}
)

func TestGatewayClass(t *testing.T) {
	testCases := []struct {
		name      string
		mutateNew func(gc *gatewayapi_v1alpha1.GatewayClass)
		mutateOld func(gc *gatewayapi_v1alpha1.GatewayClass)
		expect    bool
	}{
		{
			name:      "valid gatewayclass",
			mutateNew: func(_ *gatewayapi_v1alpha1.GatewayClass) {},
			expect:    true,
		},
		{
			name: "invalid name",
			mutateNew: func(gc *gatewayapi_v1alpha1.GatewayClass) {
				gc.Name = "invalid name"
			},
			expect: false,
		},
		{
			name:      "existing admitted gatewayclass with same controller",
			mutateNew: func(_ *gatewayapi_v1alpha1.GatewayClass) {},
			mutateOld: func(gc *gatewayapi_v1alpha1.GatewayClass) {
				gc.Spec.Controller = defaultController
				gc.Status = gatewayapi_v1alpha1.GatewayClassStatus{
					Conditions: []metav1.Condition{
						{
							Type:   string(gatewayapi_v1alpha1.GatewayClassConditionStatusAdmitted),
							Status: metav1.ConditionTrue,
						},
					},
				}
			},
			expect: false,
		},
		{
			name:      "existing non-admitted gatewayclass with same controller",
			mutateNew: func(_ *gatewayapi_v1alpha1.GatewayClass) {},
			mutateOld: func(gc *gatewayapi_v1alpha1.GatewayClass) {
				gc.Spec.Controller = defaultController
				gc.Status = gatewayapi_v1alpha1.GatewayClassStatus{
					Conditions: []metav1.Condition{
						{
							Type:   string(gatewayapi_v1alpha1.GatewayClassConditionStatusAdmitted),
							Status: metav1.ConditionFalse,
						},
					},
				}
			},
			expect: true,
		},
		{
			name:      "existing gatewayclass with different controller",
			mutateNew: func(_ *gatewayapi_v1alpha1.GatewayClass) {},
			mutateOld: func(gc *gatewayapi_v1alpha1.GatewayClass) {
				gc.Spec.Controller = "foo.io/bar"
				gc.Status = gatewayapi_v1alpha1.GatewayClassStatus{
					Conditions: []metav1.Condition{
						{
							Type:   string(gatewayapi_v1alpha1.GatewayClassConditionStatusAdmitted),
							Status: metav1.ConditionTrue,
						},
					},
				}
			},
			expect: true,
		},
		{
			name: "invalid gatewayclass params",
			mutateNew: func(gc *gatewayapi_v1alpha1.GatewayClass) {
				gc.Name = "invalid-gatewayclass-params"
				gc.Spec.ParametersRef = &gatewayapi_v1alpha1.ParametersReference{
					Group: "foo",
					Kind:  "bar",
					Name:  "baz",
				}
			},
			expect: false,
		},
	}

	// Build the client
	builder := fake.NewClientBuilder()
	scheme, err := k8s.NewContourScheme()
	if err != nil {
		t.Fatalf("failed to build contour scheme: %v", err)
	}
	builder.WithScheme(scheme)

	for _, tc := range testCases {
		// Create the client
		cl := builder.Build()

		newMutated := new.DeepCopy()
		tc.mutateNew(newMutated)

		if err := cl.Create(context.TODO(), newMutated); err != nil {
			t.Fatalf("failed to create gatewayclass: %v", err)
		}

		if tc.mutateOld != nil {
			oldMutated := old.DeepCopy()
			tc.mutateOld(oldMutated)

			if err := cl.Create(context.TODO(), oldMutated); err != nil {
				t.Fatalf("failed to create gatewayclass %q: %v", oldMutated.Name, err)
			}
		}

		actual := ValidateGatewayClass(ctx, cl, newMutated, newMutated.Spec.Controller)
		if actual != nil && tc.expect {
			t.Fatalf("%q: expected %#v, got %v", tc.name, tc.expect, actual)
		}
		if actual == nil && !tc.expect {
			t.Fatalf("%q: expected %#v, got %v", tc.name, tc.expect, actual)
		}
	}
}
