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
	"fmt"

	core_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/projectcontour/contour/internal/provisioner/labels"
	"github.com/projectcontour/contour/internal/provisioner/model"
)

const (
	// XDSPort is the network port number of Contour's xDS service.
	XDSPort = int32(8001)
	// EnvoyInsecureContainerPort is the network port number of Envoy's insecure listener.
	EnvoyInsecureContainerPort = int32(8080)
	// EnvoySecureContainerPort is the network port number of Envoy's secure listener.
	EnvoySecureContainerPort = int32(8443)

	// EnvoyMetricsPort is the network port number of Envoy's metrics listener.
	EnvoyMetricsPort = int32(8002)

	// EnvoyHealthPort is the network port number of Envoy's health listener.
	EnvoyHealthPort = int32(8002)
)

// NewUnprivilegedPodSecurity makes a a non-root PodSecurityContext object
// using 65534 as the user and group ID.
func NewUnprivilegedPodSecurity() *core_v1.PodSecurityContext {
	user := int64(65534)
	group := int64(65534)
	nonRoot := true
	return &core_v1.PodSecurityContext{
		RunAsUser:    &user,
		RunAsGroup:   &group,
		RunAsNonRoot: &nonRoot,
	}
}

// EnsureObject ensures that object "desired" is created or updated.
// If it does not already exist, it will be created as specified in
// "desired". If it does already exist, the "updateObject" function
// will be called with the current and desired states, to update the
// object appropriately.
func EnsureObject[T client.Object](
	ctx context.Context,
	cli client.Client,
	desired T,
	updateObject func(ctx context.Context, cli client.Client, current, desired T) error,
	emptyObj T,
) error {
	// Rename just for clarity.
	current := emptyObj

	err := cli.Get(ctx, client.ObjectKeyFromObject(desired), current)
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to get resource %s/%s: %w", desired.GetNamespace(), desired.GetName(), err)
	}

	if errors.IsNotFound(err) {
		if err = cli.Create(ctx, desired); err != nil {
			return fmt.Errorf("failed to create resource %s/%s: %w", desired.GetNamespace(), desired.GetName(), err)
		}
		return nil
	}

	if err = updateObject(ctx, cli, current, desired); err != nil {
		return fmt.Errorf("failed to update resource %s/%s: %w", desired.GetNamespace(), desired.GetName(), err)
	}
	return nil
}

// EnsureObjectDeleted ensures that object "obj" is deleted.
// No error will be returned if it is successfully deleted, if
// it does not contain the appropriate Gateway owner label, or
// if it already does not exist.
func EnsureObjectDeleted[T client.Object](ctx context.Context, cli client.Client, obj T, contour *model.Contour) error {
	if err := cli.Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	if !labels.AnyExist(obj, model.OwnerLabels(contour)) {
		return nil
	}

	if err := cli.Delete(ctx, obj); err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
	}

	return nil
}
