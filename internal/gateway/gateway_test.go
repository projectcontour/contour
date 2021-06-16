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

package gateway

import (
	"context"
	"testing"

	"github.com/projectcontour/contour/internal/k8s"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	core_v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayapi_v1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

func TestOthersExistInNs(t *testing.T) {
	ctx := context.TODO()

	testCases := map[string]struct {
		gw     *gatewayapi_v1alpha1.Gateway
		others []gatewayapi_v1alpha1.Gateway
		expect bool
	}{
		"one gateway": {
			gw: &gatewayapi_v1alpha1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "valid",
					Namespace: "valid-ns",
				},
				Spec: gatewayapi_v1alpha1.GatewaySpec{
					GatewayClassName: "foo",
				},
			},
			expect: true,
		},
		"two gateways in different namespaces": {
			gw: &gatewayapi_v1alpha1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "one",
					Namespace: "one",
				},
				Spec: gatewayapi_v1alpha1.GatewaySpec{
					GatewayClassName: "foo",
				},
			},
			others: []gatewayapi_v1alpha1.Gateway{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "two",
						Namespace: "two",
					},
					Spec: gatewayapi_v1alpha1.GatewaySpec{
						GatewayClassName: "foo",
					},
				}},
			expect: true,
		},
		"two gateways in the same namespace": {
			gw: &gatewayapi_v1alpha1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "one",
					Namespace: "one",
				},
				Spec: gatewayapi_v1alpha1.GatewaySpec{
					GatewayClassName: "foo",
				},
			},
			others: []gatewayapi_v1alpha1.Gateway{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "two",
						Namespace: "one",
					},
					Spec: gatewayapi_v1alpha1.GatewaySpec{
						GatewayClassName: "foo",
					},
				}},
			expect: false,
		},
		"one of three gateways in the same namespace": {
			gw: &gatewayapi_v1alpha1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "one",
					Namespace: "one",
				},
				Spec: gatewayapi_v1alpha1.GatewaySpec{
					GatewayClassName: "foo",
				},
			},
			others: []gatewayapi_v1alpha1.Gateway{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "two",
						Namespace: "one",
					},
					Spec: gatewayapi_v1alpha1.GatewaySpec{
						GatewayClassName: "foo",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "three",
						Namespace: "three",
					},
					Spec: gatewayapi_v1alpha1.GatewaySpec{
						GatewayClassName: "foo",
					},
				},
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

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// Create the client
			cl := builder.Build()

			// Create the namespace for the gateway under test.
			ns := &core_v1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: tc.gw.Namespace,
				},
			}
			err := cl.Create(ctx, ns)
			require.NoErrorf(t, err, "Failed to create namespace %s", tc.gw.Namespace)

			// Create the resources for the other gateways, if specified.
			if tc.others != nil {
				for i, gw := range tc.others {
					// Only create the namespace if it differs from the gateway under test.
					if gw.Namespace != tc.gw.Namespace {
						ns := &core_v1.Namespace{
							ObjectMeta: metav1.ObjectMeta{
								Name: gw.Namespace,
							},
						}
						err := cl.Create(ctx, ns)
						require.NoErrorf(t, err, "Failed to create namespace %s", tc.gw.Namespace)
					}
					// Create the other gateways.
					err = cl.Create(ctx, &tc.others[i])
					require.NoErrorf(t, err, "Failed to create gateway %s/%s", gw.Namespace, gw.Name)
				}
			}

			// Create the gateway under test.
			err = cl.Create(ctx, tc.gw)
			require.NoErrorf(t, err, "Failed to create gateway %s/%s", tc.gw.Namespace, tc.gw.Name)

			err = OthersExistInNs(ctx, cl, tc.gw)
			if tc.expect {
				assert.Nilf(t, err, "expected no error, got: %v", err)
			} else {
				assert.NotNilf(t, err, "expected error, got: %v", err)
			}
		})
	}
}

func TestOthersRefClass(t *testing.T) {
	ctx := context.TODO()

	testCases := map[string]struct {
		gw     *gatewayapi_v1alpha1.Gateway
		others []gatewayapi_v1alpha1.Gateway
		expect bool
	}{
		"one gateway": {
			gw: &gatewayapi_v1alpha1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "valid",
					Namespace: "valid-ns",
				},
				Spec: gatewayapi_v1alpha1.GatewaySpec{
					GatewayClassName: "foo",
				},
			},
			expect: false,
		},
		"two gateways with different classes": {
			gw: &gatewayapi_v1alpha1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "one",
					Namespace: "one",
				},
				Spec: gatewayapi_v1alpha1.GatewaySpec{
					GatewayClassName: "foo",
				},
			},
			others: []gatewayapi_v1alpha1.Gateway{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "two",
						Namespace: "two",
					},
					Spec: gatewayapi_v1alpha1.GatewaySpec{
						GatewayClassName: "foo-two",
					},
				}},
			expect: false,
		},
		"two gateways with same class": {
			gw: &gatewayapi_v1alpha1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "one",
					Namespace: "one",
				},
				Spec: gatewayapi_v1alpha1.GatewaySpec{
					GatewayClassName: "foo",
				},
			},
			others: []gatewayapi_v1alpha1.Gateway{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "two",
						Namespace: "two",
					},
					Spec: gatewayapi_v1alpha1.GatewaySpec{
						GatewayClassName: "foo",
					},
				}},
			expect: true,
		},
		"one of three gateways with the same class": {
			gw: &gatewayapi_v1alpha1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "one",
					Namespace: "one",
				},
				Spec: gatewayapi_v1alpha1.GatewaySpec{
					GatewayClassName: "foo",
				},
			},
			others: []gatewayapi_v1alpha1.Gateway{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "two",
						Namespace: "one",
					},
					Spec: gatewayapi_v1alpha1.GatewaySpec{
						GatewayClassName: "foo",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "three",
						Namespace: "three",
					},
					Spec: gatewayapi_v1alpha1.GatewaySpec{
						GatewayClassName: "foo-three",
					},
				},
			},
			expect: true,
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

			// Create the namespace for the gateway under test.
			ns := &core_v1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: tc.gw.Namespace,
				},
			}
			err := cl.Create(ctx, ns)
			require.NoErrorf(t, err, "Failed to create namespace %s", tc.gw.Namespace)

			// Create the resources for the other gateways, if specified.
			if tc.others != nil {
				for i, gw := range tc.others {
					// Only create the namespace if it differs from the gateway under test.
					if gw.Namespace != tc.gw.Namespace {
						ns := &core_v1.Namespace{
							ObjectMeta: metav1.ObjectMeta{
								Name: gw.Namespace,
							},
						}
						err := cl.Create(ctx, ns)
						require.NoErrorf(t, err, "Failed to create namespace %s", tc.gw.Namespace)
					}
					// Create the other gateways.
					err = cl.Create(ctx, &tc.others[i])
					require.NoErrorf(t, err, "Failed to create gateway %s/%s", gw.Namespace, gw.Name)
				}
			}

			// Create the gateway under test.
			err = cl.Create(ctx, tc.gw)
			require.NoErrorf(t, err, "Failed to create gateway %s/%s", tc.gw.Namespace, tc.gw.Name)

			others, _ := OthersRefClass(ctx, cl, tc.gw)
			if tc.expect && others {
				assert.Truef(t, others, "expected other gateways to reference the same class, got: %v", others)
			}
			if !tc.expect && !others {
				assert.Falsef(t, others, "expected no other gateways to reference the same class, got: %v", others)
			}
		})
	}
}

func TestOthersExist(t *testing.T) {
	ctx := context.TODO()

	testCases := map[string]struct {
		gw     *gatewayapi_v1alpha1.Gateway
		others []gatewayapi_v1alpha1.Gateway
		expect bool
	}{
		"one gateway": {
			gw: &gatewayapi_v1alpha1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "valid",
					Namespace: "valid-ns",
				},
				Spec: gatewayapi_v1alpha1.GatewaySpec{
					GatewayClassName: "foo",
				},
			},
			expect: false,
		},
		"two gateways in different namespaces": {
			gw: &gatewayapi_v1alpha1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "one",
					Namespace: "one",
				},
				Spec: gatewayapi_v1alpha1.GatewaySpec{
					GatewayClassName: "foo",
				},
			},
			others: []gatewayapi_v1alpha1.Gateway{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "two",
						Namespace: "two",
					},
					Spec: gatewayapi_v1alpha1.GatewaySpec{
						GatewayClassName: "foo",
					},
				}},
			expect: true,
		},
		"two gateways in the same namespace": {
			gw: &gatewayapi_v1alpha1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "one",
					Namespace: "one",
				},
				Spec: gatewayapi_v1alpha1.GatewaySpec{
					GatewayClassName: "foo",
				},
			},
			others: []gatewayapi_v1alpha1.Gateway{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "two",
						Namespace: "one",
					},
					Spec: gatewayapi_v1alpha1.GatewaySpec{
						GatewayClassName: "foo",
					},
				}},
			expect: true,
		},
		"one of three gateways in the same namespace": {
			gw: &gatewayapi_v1alpha1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "one",
					Namespace: "one",
				},
				Spec: gatewayapi_v1alpha1.GatewaySpec{
					GatewayClassName: "foo",
				},
			},
			others: []gatewayapi_v1alpha1.Gateway{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "two",
						Namespace: "one",
					},
					Spec: gatewayapi_v1alpha1.GatewaySpec{
						GatewayClassName: "foo",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "three",
						Namespace: "three",
					},
					Spec: gatewayapi_v1alpha1.GatewaySpec{
						GatewayClassName: "foo",
					},
				},
			},
			expect: true,
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

			// Create the namespace for the gateway under test.
			ns := &core_v1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: tc.gw.Namespace,
				},
			}
			err := cl.Create(ctx, ns)
			require.NoErrorf(t, err, "Failed to create namespace %s", tc.gw.Namespace)

			// Create the resources for the other gateways, if specified.
			if tc.others != nil {
				for i, gw := range tc.others {
					// Only create the namespace if it differs from the gateway under test.
					if gw.Namespace != tc.gw.Namespace {
						ns := &core_v1.Namespace{
							ObjectMeta: metav1.ObjectMeta{
								Name: gw.Namespace,
							},
						}
						err := cl.Create(ctx, ns)
						require.NoErrorf(t, err, "Failed to create namespace %s", tc.gw.Namespace)
					}
					// Create the other gateways.
					err = cl.Create(ctx, &tc.others[i])
					require.NoErrorf(t, err, "Failed to create gateway %s/%s", gw.Namespace, gw.Name)
				}
			}

			// Create the gateway under test.
			err = cl.Create(ctx, tc.gw)
			require.NoErrorf(t, err, "Failed to create gateway %s/%s", tc.gw.Namespace, tc.gw.Name)

			others, _ := OthersExist(ctx, cl, tc.gw)
			if tc.expect && others == nil {
				assert.Nilf(t, others, "expected other gateways, got: %v", others)
			}
			if !tc.expect && others != nil {
				assert.NotNilf(t, others, "expected no other gateways, got: %v", others)
			}
		})
	}
}
