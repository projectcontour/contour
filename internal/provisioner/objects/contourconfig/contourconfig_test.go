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

package contourconfig

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/provisioner/model"
)

func TestEnsureContourConfig(t *testing.T) {
	tests := map[string]struct {
		contour  *model.Contour
		existing *contour_v1alpha1.ContourConfiguration
		want     contour_v1alpha1.ContourConfigurationSpec
	}{
		"no existing ContourConfiguration": {
			contour: &model.Contour{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "contour-namespace-1",
					Name:      "contour-1",
				},
			},
			want: contour_v1alpha1.ContourConfigurationSpec{
				Gateway: &contour_v1alpha1.GatewayConfig{
					GatewayRef: contour_v1alpha1.NamespacedName{
						Namespace: "contour-namespace-1",
						Name:      "contour-1",
					},
				},
				Envoy: &contour_v1alpha1.EnvoyConfig{
					Service: &contour_v1alpha1.NamespacedName{
						Namespace: "contour-namespace-1",
						Name:      "envoy-contour-1",
					},
				},
			},
		},
		"existing ContourConfiguration found, with exactly the right spec": {
			contour: &model.Contour{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "contour-namespace-1",
					Name:      "contour-1",
				},
			},
			existing: &contour_v1alpha1.ContourConfiguration{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "contour-namespace-1",
					Name:      "contourconfig-contour-1",
				},
				Spec: contour_v1alpha1.ContourConfigurationSpec{
					Gateway: &contour_v1alpha1.GatewayConfig{
						GatewayRef: contour_v1alpha1.NamespacedName{
							Namespace: "contour-namespace-1",
							Name:      "contour-1",
						},
					},
					Envoy: &contour_v1alpha1.EnvoyConfig{
						Service: &contour_v1alpha1.NamespacedName{
							Namespace: "contour-namespace-1",
							Name:      "envoy-contour-1",
						},
					},
				},
			},
			want: contour_v1alpha1.ContourConfigurationSpec{
				Gateway: &contour_v1alpha1.GatewayConfig{
					GatewayRef: contour_v1alpha1.NamespacedName{
						Namespace: "contour-namespace-1",
						Name:      "contour-1",
					},
				},
				Envoy: &contour_v1alpha1.EnvoyConfig{
					Service: &contour_v1alpha1.NamespacedName{
						Namespace: "contour-namespace-1",
						Name:      "envoy-contour-1",
					},
				},
			},
		},
		"existing ContourConfiguration found, with the wrong spec": {
			contour: &model.Contour{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "contour-namespace-1",
					Name:      "contour-1",
				},
			},
			existing: &contour_v1alpha1.ContourConfiguration{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "contour-namespace-1",
					Name:      "contourconfig-contour-1",
				},
				Spec: contour_v1alpha1.ContourConfigurationSpec{
					Gateway: &contour_v1alpha1.GatewayConfig{
						GatewayRef: contour_v1alpha1.NamespacedName{
							Namespace: "some-other-namespace",
							Name:      "some-other-contour",
						},
					},
					Envoy: &contour_v1alpha1.EnvoyConfig{
						Service: &contour_v1alpha1.NamespacedName{
							Namespace: "yet-another-namespace",
							Name:      "some-other-envoy-service",
						},
					},
				},
			},
			want: contour_v1alpha1.ContourConfigurationSpec{
				Gateway: &contour_v1alpha1.GatewayConfig{
					GatewayRef: contour_v1alpha1.NamespacedName{
						Namespace: "contour-namespace-1",
						Name:      "contour-1",
					},
				},
				Envoy: &contour_v1alpha1.EnvoyConfig{
					Service: &contour_v1alpha1.NamespacedName{
						Namespace: "contour-namespace-1",
						Name:      "envoy-contour-1",
					},
				},
			},
		},
		"existing ContourConfiguration found, with additional fields specified": {
			contour: &model.Contour{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "contour-namespace-1",
					Name:      "contour-1",
				},
			},
			existing: &contour_v1alpha1.ContourConfiguration{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "contour-namespace-1",
					Name:      "contourconfig-contour-1",
				},
				Spec: contour_v1alpha1.ContourConfigurationSpec{
					Gateway: &contour_v1alpha1.GatewayConfig{
						GatewayRef: contour_v1alpha1.NamespacedName{
							Namespace: "contour-namespace-1",
							Name:      "contour-1",
						},
					},
					Envoy: &contour_v1alpha1.EnvoyConfig{
						Service: &contour_v1alpha1.NamespacedName{
							Namespace: "contour-namespace-1",
							Name:      "envoy-contour-1",
						},
						ClientCertificate: &contour_v1alpha1.NamespacedName{
							Namespace: "client-cert-namespace",
							Name:      "client-cert",
						},
					},
					HTTPProxy: &contour_v1alpha1.HTTPProxyConfig{
						RootNamespaces: []string{"ns-1", "ns-2"},
					},
				},
			},
			want: contour_v1alpha1.ContourConfigurationSpec{
				Gateway: &contour_v1alpha1.GatewayConfig{
					GatewayRef: contour_v1alpha1.NamespacedName{
						Namespace: "contour-namespace-1",
						Name:      "contour-1",
					},
				},
				Envoy: &contour_v1alpha1.EnvoyConfig{
					Service: &contour_v1alpha1.NamespacedName{
						Namespace: "contour-namespace-1",
						Name:      "envoy-contour-1",
					},
					ClientCertificate: &contour_v1alpha1.NamespacedName{
						Namespace: "client-cert-namespace",
						Name:      "client-cert",
					},
				},
				HTTPProxy: &contour_v1alpha1.HTTPProxyConfig{
					RootNamespaces: []string{"ns-1", "ns-2"},
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			require.NoError(t, contour_v1alpha1.AddToScheme(scheme))

			clientBuilder := fake.NewClientBuilder().WithScheme(scheme)
			if tc.existing != nil {
				clientBuilder.WithObjects(tc.existing)
			}
			client := clientBuilder.Build()

			require.NoError(t, EnsureContourConfig(context.Background(), client, tc.contour))

			got := &contour_v1alpha1.ContourConfiguration{}
			key := types.NamespacedName{
				Namespace: tc.contour.Namespace,
				Name:      "contourconfig-" + tc.contour.Name,
			}
			require.NoError(t, client.Get(context.Background(), key, got))

			assert.Equal(t, tc.want, got.Spec)
		})
	}
}

func TestEnsureContourConfigDeleted(t *testing.T) {
	tests := map[string]struct {
		contour    *model.Contour
		existing   *contour_v1alpha1.ContourConfiguration
		wantDelete bool
	}{
		"ContourConfiguration exists with the proper labels": {
			contour: &model.Contour{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "contour-namespace",
					Name:      "contour-1",
				},
			},
			existing: &contour_v1alpha1.ContourConfiguration{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "contour-namespace",
					Name:      "contourconfig-contour-1",
					Labels: map[string]string{
						model.ContourOwningGatewayNameLabel: "contour-1",
					},
				},
			},
			wantDelete: true,
		},
		"ContourConfiguration exists without the proper labels (no labels)": {
			contour: &model.Contour{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "contour-namespace",
					Name:      "contour-1",
				},
			},
			existing: &contour_v1alpha1.ContourConfiguration{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "contour-namespace",
					Name:      "contourconfig-contour-1",
				},
			},
			wantDelete: false,
		},
		"ContourConfiguration exists without the proper labels (wrong key)": {
			contour: &model.Contour{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "contour-namespace",
					Name:      "contour-1",
				},
			},
			existing: &contour_v1alpha1.ContourConfiguration{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "contour-namespace",
					Name:      "contourconfig-contour-1",
					Labels: map[string]string{
						"some-other-label-key": "contour-1",
					},
				},
			},
			wantDelete: false,
		},
		"ContourConfiguration exists without the proper labels (wrong value)": {
			contour: &model.Contour{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "contour-namespace",
					Name:      "contour-1",
				},
			},
			existing: &contour_v1alpha1.ContourConfiguration{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "contour-namespace",
					Name:      "contourconfig-contour-1",
					Labels: map[string]string{
						model.ContourOwningGatewayNameLabel: "some-other-contour",
					},
				},
			},
			wantDelete: false,
		},
		"ContourConfiguration does not exist": {
			contour: &model.Contour{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "contour-namespace",
					Name:      "contour-1",
				},
			},
			wantDelete: true, // meaning we expect no object at the end of the test
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			require.NoError(t, contour_v1alpha1.AddToScheme(scheme))

			clientBuilder := fake.NewClientBuilder().WithScheme(scheme)
			if tc.existing != nil {
				clientBuilder.WithObjects(tc.existing)
			}
			client := clientBuilder.Build()

			// We don't have the ability to inject fake errors into the fake client,
			// so all code paths we can trigger in this test are expected to return
			// no error.
			require.NoError(t, EnsureContourConfigDeleted(context.Background(), client, tc.contour))

			remaining := &contour_v1alpha1.ContourConfiguration{}
			key := types.NamespacedName{
				Namespace: tc.contour.Namespace,
				Name:      "contourconfig-" + tc.contour.Name,
			}
			err := client.Get(context.Background(), key, remaining)
			if tc.wantDelete {
				require.True(t, errors.IsNotFound(err))
			} else {
				require.NoError(t, err)
			}
		})
	}
}
