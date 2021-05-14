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
	"k8s.io/apimachinery/pkg/runtime"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayapi_v1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

var (
	ctx = context.TODO()

	gc = &gatewayapi_v1alpha1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-gc",
		},
		Spec: gatewayapi_v1alpha1.GatewayClassSpec{
			Controller: "projectcontour/contour",
		},
	}
)

func TestGatewayClass(t *testing.T) {
	testCases := []struct {
		name   string
		mutate func(gc *gatewayapi_v1alpha1.GatewayClass)
		expect bool
	}{
		{
			name: "valid gatewayclass",
			mutate: func(_ *gatewayapi_v1alpha1.GatewayClass) {},
			expect: true,
		},
	}

	// Create the client
	builder := fake.NewClientBuilder()
	scheme := gatewayapi_v1alpha1.SchemeBuilder.AddToScheme(addKnownTypes)
	builder.WithScheme()
	cl := builder.Build()

	for _, tc := range testCases {
		mutated := gc.DeepCopy()
		tc.mutate(mutated)

		actual := ValidateGatewayClass(ctx, cli, mutated)
		if actual != nil && tc.expect {
			t.Fatalf("%q: expected %#v, got %#v", tc.name, tc.expect, actual)
		}
		if actual == nil && !tc.expect {
			t.Fatalf("%q: expected %#v, got %#v", tc.name, tc.expect, actual)
		}
	}
}

// Adds the list of known types to Scheme.
func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(gatewayapi_v1alpha1.SchemeGroupVersion,
		&gatewayapi_v1alpha1.GatewayClass{},
		&gatewayapi_v1alpha1.GatewayClassList{},
	)
	// AddToGroupVersion allows the serialization of client types like ListOptions.
	metav1.AddToGroupVersion(scheme, gatewayapi_v1alpha1.SchemeGroupVersion)
	return nil
}
