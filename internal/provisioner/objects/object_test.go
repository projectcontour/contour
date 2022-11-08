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

package objects

import (
	"context"
	"errors"
	"testing"

	"github.com/projectcontour/contour/internal/provisioner"
	"github.com/projectcontour/contour/internal/provisioner/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	pkgclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestEnsureObject_ErrorGettingObject(t *testing.T) {
	// An empty scheme is used to trigger an error getting the object.
	client := fake.NewClientBuilder().WithScheme(runtime.NewScheme()).Build()

	want := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "obj-ns",
			Name:      "obj-name",
		},
	}

	assert.ErrorContains(t, EnsureObject(context.Background(), client, want, nil, &corev1.Service{}), "failed to get resource obj-ns/obj-name")
}

func TestEnsureObject_NonExistentObjectIsCreated(t *testing.T) {
	scheme, err := provisioner.CreateScheme()
	require.NoError(t, err)

	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	want := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "obj-ns",
			Name:      "obj-name",
		},
	}

	require.NoError(t, EnsureObject(context.Background(), client, want, nil, &corev1.Service{}))

	got := &corev1.Service{}
	require.NoError(t, client.Get(context.Background(), pkgclient.ObjectKeyFromObject(want), got))

	assert.Equal(t, want, got)
}

func TestEnsureObject_ExistingObjectIsUpdated(t *testing.T) {
	scheme, err := provisioner.CreateScheme()
	require.NoError(t, err)

	existing := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       "obj-ns",
			Name:            "obj-name",
			ResourceVersion: "1",
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()

	desired := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "obj-ns",
			Name:      "obj-name",
			Annotations: map[string]string{
				"updated": "true",
			},
		},
	}

	updater := func(ctx context.Context, client pkgclient.Client, current, desired *corev1.Service) error {
		// Set another annotation on "desired" so we can validate that
		// updater is actually being called.
		desired = desired.DeepCopy()
		desired.Annotations["called-updater"] = "true"
		return client.Update(ctx, desired)
	}

	require.NoError(t, EnsureObject(context.Background(), client, desired, updater, &corev1.Service{}))

	want := desired.DeepCopy()
	want.ResourceVersion = "2"
	want.Annotations["called-updater"] = "true"

	got := &corev1.Service{}
	require.NoError(t, client.Get(context.Background(), pkgclient.ObjectKeyFromObject(want), got))

	assert.Equal(t, want, got)
}

func TestEnsureObject_ErrorUpdatingObject(t *testing.T) {
	scheme, err := provisioner.CreateScheme()
	require.NoError(t, err)

	existing := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "obj-ns",
			Name:      "obj-name",
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()

	desired := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "obj-ns",
			Name:      "obj-name",
			Annotations: map[string]string{
				"updated": "true",
			},
		},
	}

	updater := func(ctx context.Context, client pkgclient.Client, current, desired *corev1.Service) error {
		return errors.New("update error")
	}

	assert.ErrorContains(t, EnsureObject(context.Background(), client, desired, updater, &corev1.Service{}), "update error")
}

func TestEnsureObjectDeleted_ObjectNotFound(t *testing.T) {
	scheme, err := provisioner.CreateScheme()
	require.NoError(t, err)

	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	svc := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "obj-ns",
			Name:      "obj-name",
		},
	}

	require.NoError(t, EnsureObjectDeleted(context.Background(), client, svc, nil))
}

func TestEnsureObjectDeleted_ErrorGettingObject(t *testing.T) {
	// An empty scheme is used to trigger an error getting the object.
	client := fake.NewClientBuilder().WithScheme(runtime.NewScheme()).Build()

	svc := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "obj-ns",
			Name:      "obj-name",
		},
	}

	require.ErrorContains(t, EnsureObjectDeleted(context.Background(), client, svc, nil), "no kind is registered for the type v1.Service in scheme")
}

func TestEnsureObjectDeleted_ObjectExistsWithoutLabels(t *testing.T) {
	scheme, err := provisioner.CreateScheme()
	require.NoError(t, err)

	existing := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "obj-ns",
			Name:      "obj-name",
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()

	require.NoError(t, EnsureObjectDeleted(context.Background(), client, existing, model.Default("projectcontour", "contour")))

	// Ensure service still exists.
	res := &corev1.Service{}
	require.NoError(t, client.Get(context.Background(), pkgclient.ObjectKeyFromObject(existing), res))
}

func TestEnsureObjectDeleted_ObjectExistsWithNonMatchingLabels(t *testing.T) {
	scheme, err := provisioner.CreateScheme()
	require.NoError(t, err)

	existing := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "obj-ns",
			Name:      "obj-name",
			Labels: map[string]string{
				"projectcontour.io/owning-gateway-name": "some-other-gateway",
			},
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()

	require.NoError(t, EnsureObjectDeleted(context.Background(), client, existing, model.Default("projectcontour", "contour")))

	// Ensure service still exists.
	res := &corev1.Service{}
	require.NoError(t, client.Get(context.Background(), pkgclient.ObjectKeyFromObject(existing), res))
}

func TestEnsureObjectDeleted_ObjectExistsWithMatchingLabels(t *testing.T) {
	scheme, err := provisioner.CreateScheme()
	require.NoError(t, err)

	existing := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "obj-ns",
			Name:      "obj-name",
			Labels: map[string]string{
				"projectcontour.io/owning-gateway-name": "contour",
			},
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()

	require.NoError(t, EnsureObjectDeleted(context.Background(), client, existing, model.Default("projectcontour", "contour")))

	// Ensure service no longer exists.
	res := &corev1.Service{}
	require.True(t, apierrors.IsNotFound(client.Get(context.Background(), pkgclient.ObjectKeyFromObject(existing), res)))
}
