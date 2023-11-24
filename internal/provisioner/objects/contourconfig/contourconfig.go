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

	"k8s.io/apimachinery/pkg/api/equality"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/provisioner/model"
	"github.com/projectcontour/contour/internal/provisioner/objects"
)

// EnsureContourConfig ensures that a ContourConfiguration exists for the given contour.
func EnsureContourConfig(ctx context.Context, cli client.Client, contour *model.Contour) error {
	desired := &contour_v1alpha1.ContourConfiguration{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace:   contour.Namespace,
			Name:        contour.ContourConfigurationName(),
			Labels:      contour.CommonLabels(),
			Annotations: contour.CommonAnnotations(),
		},
	}

	// Take any user-provided Config as a base.
	if contour.Spec.RuntimeSettings != nil {
		desired.Spec = *contour.Spec.RuntimeSettings
	}

	// Override Gateway-specific settings to ensure the Contour is
	// being configured correctly for the Gateway being provisioned.
	setGatewayConfig(desired, contour)

	updater := func(ctx context.Context, cli client.Client, current, _ *contour_v1alpha1.ContourConfiguration) error {
		maybeUpdated := current.DeepCopy()
		setGatewayConfig(maybeUpdated, contour)

		if !equality.Semantic.DeepEqual(current, maybeUpdated) {
			return cli.Update(ctx, maybeUpdated)
		}
		return nil
	}

	return objects.EnsureObject(ctx, cli, desired, updater, new(contour_v1alpha1.ContourConfiguration))
}

func setGatewayConfig(config *contour_v1alpha1.ContourConfiguration, contour *model.Contour) {
	config.Spec.Gateway = &contour_v1alpha1.GatewayConfig{
		GatewayRef: contour_v1alpha1.NamespacedName{
			Namespace: contour.Namespace,
			Name:      contour.Name,
		},
	}

	if config.Spec.Envoy == nil {
		config.Spec.Envoy = &contour_v1alpha1.EnvoyConfig{}
	}
	config.Spec.Envoy.Service = &contour_v1alpha1.NamespacedName{
		Namespace: contour.Namespace,
		Name:      contour.EnvoyServiceName(),
	}
}

// EnsureContourConfigDeleted deletes a ContourConfig for the provided contour, if the configured owner labels exist.
func EnsureContourConfigDeleted(ctx context.Context, cli client.Client, contour *model.Contour) error {
	obj := &contour_v1alpha1.ContourConfiguration{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: contour.Namespace,
			Name:      contour.ContourConfigurationName(),
		},
	}

	return objects.EnsureObjectDeleted(ctx, cli, obj, contour)
}
