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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayapi_v1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

func TestGateway(t *testing.T) {
	ctx := context.TODO()

	gc := gatewayapi_v1alpha1.GatewayClass{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-gc",
		},
		Spec: gatewayapi_v1alpha1.GatewayClassSpec{
			Controller: defaultController,
		},
		Status: gatewayapi_v1alpha1.GatewayClassStatus{
			Conditions: []metav1.Condition{
				{
					Type:   string(gatewayapi_v1alpha1.ConditionRouteAdmitted),
					Status: metav1.ConditionTrue,
				},
			},
		},
	}

	gw := &gatewayapi_v1alpha1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name: "gw",
		},
		Spec: gatewayapi_v1alpha1.GatewaySpec{
			GatewayClassName: gc.Name,
		},
	}

	testCases := map[string]struct {
		mutateGw func(gw *gatewayapi_v1alpha1.Gateway)
		mutateGc func(gc *gatewayapi_v1alpha1.GatewayClass)
		errType  field.ErrorType
		errField string
		expect   bool
	}{
		"valid gateway": {
			mutateGw: func(_ *gatewayapi_v1alpha1.Gateway) {},
			mutateGc: func(_ *gatewayapi_v1alpha1.GatewayClass) {},
			expect:   true,
		},
		"gateway references non-existent gatewayclass": {
			mutateGw: func(gw *gatewayapi_v1alpha1.Gateway) {
				gw.Spec.GatewayClassName = "non-existent"
			},
			errType:  field.ErrorTypeInternal,
			errField: "spec.gatewayClassName",
			expect:   false,
		},
		"existing non-admitted gatewayclass with same controller": {
			mutateGw: func(_ *gatewayapi_v1alpha1.Gateway) {},
			mutateGc: func(gc *gatewayapi_v1alpha1.GatewayClass) {
				gc.Status.Conditions[0].Status = metav1.ConditionFalse
			},
			errType:  field.ErrorTypeInternal,
			errField: "spec.gatewayClassName",
			expect:   false,
		},
	}

	// Build the client
	builder := fake.NewClientBuilder()
	scheme, err := k8s.NewContourScheme()
	if err != nil {
		t.Fatalf("failed to build contour scheme: %v", err)
	}
	builder.WithScheme(scheme)

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// Create the client
			cl := builder.Build()

			// Mutate and create the referenced gatewayclass.
			muGc := gc.DeepCopy()
			if tc.mutateGc != nil {
				tc.mutateGc(muGc)
			}
			err := cl.Create(context.TODO(), muGc)
			require.NoErrorf(t, err, "Failed to create gatewayclass %s", muGc.Name)

			// Mutate and create the gateway being tested.
			muGw := gw.DeepCopy()
			tc.mutateGw(muGw)

			err = cl.Create(context.TODO(), muGw)
			require.NoErrorf(t, err, "Failed to create gateway %s/%s", muGw.Namespace, muGw.Name)

			actual := ValidateGateway(ctx, cl, muGw)
			if tc.expect {
				assert.Nilf(t, actual, "expected no error, got: %v", actual)
			} else {
				// Not asserting an error since a field.ErrorList is being returned.
				assert.NotNilf(t, actual, "expected no error, got: %v", actual)
				for _, a := range actual {
					assert.Contains(t, a.Type, tc.errType)
					assert.Contains(t, a.Field, tc.errField)
				}
			}
		})
	}
}
