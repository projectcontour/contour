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
	"fmt"
	"testing"

	"github.com/projectcontour/contour/internal/k8s"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayapi_v1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

var (
	ctx = context.TODO()

	gc = &gatewayapi_v1alpha1.GatewayClass{
		Spec: gatewayapi_v1alpha1.GatewayClassSpec{
			Controller: "projectcontour/contour",
		},
	}
)

func TestGatewayClass(t *testing.T) {
	testCases := []struct {
		name   string
		mutate func(gc *gatewayapi_v1alpha1.GatewayClass)
		exist  bool
		expect bool
	}{
		{
			name: "valid gatewayclass",
			mutate: func(gc *gatewayapi_v1alpha1.GatewayClass) {
				gc.Name = "valid-gatewayclass"
			},
			expect: true,
		},
		{
			name: "invalid gatewayclass name",
			mutate: func(gc *gatewayapi_v1alpha1.GatewayClass) {
				gc.Name = "invalid name"
			},
			expect: false,
		},
		{
			name: "existing gatewayclass",
			mutate: func(gc *gatewayapi_v1alpha1.GatewayClass) {
				gc.Name = "existing-gatewayclass"
			},
			exist:  true,
			expect: false,
		},
		{
			name: "invalid gatewayclass params",
			mutate: func(gc *gatewayapi_v1alpha1.GatewayClass) {
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

	// Create the client
	builder := fake.NewClientBuilder()
	scheme, err := k8s.NewContourScheme()
	if err != nil {
		t.Fatalf("failed to build contour scheme: %v", err)
	}
	builder.WithScheme(scheme)
	cl := builder.Build()

	for i, tc := range testCases {
		mutated := gc.DeepCopy()
		tc.mutate(mutated)

		if err := cl.Create(context.TODO(), mutated); err != nil {
			t.Fatalf("failed to create gatewayclass: %v", err)
		}

		if tc.exist {
			existing := &gatewayapi_v1alpha1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("existing-%d", i),
				},
				Spec: gatewayapi_v1alpha1.GatewayClassSpec{
					Controller: "projectcontour/contour",
				},
			}

			if err := cl.Create(context.TODO(), existing); err != nil {
				t.Fatalf("failed to create gatewayclass: %v", err)
			}
		}

		actual := ValidateGatewayClass(ctx, cl, mutated)
		if actual != nil && tc.expect {
			t.Fatalf("%q: expected %#v, got %#v", tc.name, tc.expect, actual)
		}
		if actual == nil && !tc.expect {
			t.Fatalf("%q: expected %#v, got %#v", tc.name, tc.expect, actual)
		}
	}
}
