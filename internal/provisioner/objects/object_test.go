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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	core_v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	pkgclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/projectcontour/contour/internal/provisioner"
	"github.com/projectcontour/contour/internal/provisioner/model"
)

func TestEnsureObject_ErrorGettingObject(t *testing.T) {
	// An empty scheme is used to trigger an error getting the object.
	client := fake.NewClientBuilder().WithScheme(runtime.NewScheme()).Build()

	want := &core_v1.Service{
		TypeMeta: meta_v1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "obj-ns",
			Name:      "obj-name",
		},
	}

	require.ErrorContains(t, EnsureObject(context.Background(), client, want, nil, &core_v1.Service{}), "failed to get resource obj-ns/obj-name")
}

func TestEnsureObject_NonExistentObjectIsCreated(t *testing.T) {
	scheme, err := provisioner.CreateScheme()
	require.NoError(t, err)

	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	want := &core_v1.Service{
		TypeMeta: meta_v1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "obj-ns",
			Name:      "obj-name",
		},
	}

	require.NoError(t, EnsureObject(context.Background(), client, want, nil, &core_v1.Service{}))

	got := &core_v1.Service{}
	require.NoError(t, client.Get(context.Background(), pkgclient.ObjectKeyFromObject(want), got))

	assert.Equal(t, want, got)
}

func TestEnsureObject_ExistingObjectIsUpdated(t *testing.T) {
	scheme, err := provisioner.CreateScheme()
	require.NoError(t, err)

	existing := &core_v1.Service{
		TypeMeta: meta_v1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace:       "obj-ns",
			Name:            "obj-name",
			ResourceVersion: "1",
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()

	desired := &core_v1.Service{
		TypeMeta: meta_v1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "obj-ns",
			Name:      "obj-name",
			Annotations: map[string]string{
				"updated": "true",
			},
		},
	}

	updater := func(ctx context.Context, client pkgclient.Client, _, desired *core_v1.Service) error {
		// Set another annotation on "desired" so we can validate that
		// updater is actually being called.
		desired = desired.DeepCopy()
		desired.Annotations["called-updater"] = "true"
		return client.Update(ctx, desired)
	}

	require.NoError(t, EnsureObject(context.Background(), client, desired, updater, &core_v1.Service{}))

	want := desired.DeepCopy()
	want.ResourceVersion = "2"
	want.Annotations["called-updater"] = "true"

	got := &core_v1.Service{}
	require.NoError(t, client.Get(context.Background(), pkgclient.ObjectKeyFromObject(want), got))

	assert.Equal(t, want, got)
}

func TestEnsureObject_ErrorUpdatingObject(t *testing.T) {
	scheme, err := provisioner.CreateScheme()
	require.NoError(t, err)

	existing := &core_v1.Service{
		TypeMeta: meta_v1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "obj-ns",
			Name:      "obj-name",
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()

	desired := &core_v1.Service{
		TypeMeta: meta_v1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "obj-ns",
			Name:      "obj-name",
			Annotations: map[string]string{
				"updated": "true",
			},
		},
	}

	updater := func(_ context.Context, _ pkgclient.Client, _, _ *core_v1.Service) error {
		return errors.New("update error")
	}

	require.ErrorContains(t, EnsureObject(context.Background(), client, desired, updater, &core_v1.Service{}), "update error")
}

func TestEnsureObjectDeleted_ObjectNotFound(t *testing.T) {
	scheme, err := provisioner.CreateScheme()
	require.NoError(t, err)

	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	svc := &core_v1.Service{
		TypeMeta: meta_v1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "obj-ns",
			Name:      "obj-name",
		},
	}

	require.NoError(t, EnsureObjectDeleted(context.Background(), client, svc, nil))
}

func TestEnsureObjectDeleted_ErrorGettingObject(t *testing.T) {
	// An empty scheme is used to trigger an error getting the object.
	client := fake.NewClientBuilder().WithScheme(runtime.NewScheme()).Build()

	svc := &core_v1.Service{
		TypeMeta: meta_v1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "obj-ns",
			Name:      "obj-name",
		},
	}

	require.ErrorContains(t, EnsureObjectDeleted(context.Background(), client, svc, nil), "no kind is registered for the type v1.Service in scheme")
}

func TestEnsureObjectDeleted_ObjectExistsWithoutLabels(t *testing.T) {
	scheme, err := provisioner.CreateScheme()
	require.NoError(t, err)

	existing := &core_v1.Service{
		TypeMeta: meta_v1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "obj-ns",
			Name:      "obj-name",
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()

	require.NoError(t, EnsureObjectDeleted(context.Background(), client, existing, model.Default("projectcontour", "contour")))

	// Ensure service still exists.
	res := &core_v1.Service{}
	require.NoError(t, client.Get(context.Background(), pkgclient.ObjectKeyFromObject(existing), res))
}

func TestEnsureObjectDeleted_ObjectExistsWithNonMatchingLabels(t *testing.T) {
	scheme, err := provisioner.CreateScheme()
	require.NoError(t, err)

	existing := &core_v1.Service{
		TypeMeta: meta_v1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: meta_v1.ObjectMeta{
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
	res := &core_v1.Service{}
	require.NoError(t, client.Get(context.Background(), pkgclient.ObjectKeyFromObject(existing), res))
}

func TestEnsureObjectDeleted_ObjectExistsWithMatchingLabels(t *testing.T) {
	scheme, err := provisioner.CreateScheme()
	require.NoError(t, err)

	existing := &core_v1.Service{
		TypeMeta: meta_v1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: meta_v1.ObjectMeta{
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
	res := &core_v1.Service{}
	require.True(t, apierrors.IsNotFound(client.Get(context.Background(), pkgclient.ObjectKeyFromObject(existing), res)))
}
