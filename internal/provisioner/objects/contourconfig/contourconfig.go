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

	contour_api_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/provisioner/model"
	"github.com/projectcontour/contour/internal/provisioner/objects"

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// EnsureContourConfig ensures that a ContourConfiguration exists for the given contour.
func EnsureContourConfig(ctx context.Context, cli client.Client, contour *model.Contour) error {
	current, err := current(ctx, cli, contour.Namespace, contour.ContourConfigurationName())

	switch {
	// Legitimate error: return it
	case err != nil && !errors.IsNotFound(err):
		return err
	// ContourConfiguration not found: create it
	case errors.IsNotFound(err):
		contourConfig := &contour_api_v1alpha1.ContourConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: contour.Namespace,
				Name:      contour.ContourConfigurationName(),
				Labels:    model.CommonLabels(contour),
			},
		}

		// Take any user-provided Config as a base.
		if contour.Spec.RuntimeSettings != nil {
			contourConfig.Spec = *contour.Spec.RuntimeSettings
		}

		// Override Gateway-specific settings to ensure the Contour is
		// being configured correctly for the Gateway being provisioned.
		setGatewayConfig(contourConfig, contour)

		return cli.Create(ctx, contourConfig)
	// Already exists: ensure it has the relevant fields set correctly.
	default:
		maybeUpdated := current.DeepCopy()

		setGatewayConfig(maybeUpdated, contour)

		if !equality.Semantic.DeepEqual(current, maybeUpdated) {
			return cli.Update(ctx, maybeUpdated)
		}

		return nil
	}
}

func setGatewayConfig(config *contour_api_v1alpha1.ContourConfiguration, contour *model.Contour) {
	config.Spec.Gateway = &contour_api_v1alpha1.GatewayConfig{
		GatewayRef: &contour_api_v1alpha1.NamespacedName{
			Namespace: contour.Namespace,
			Name:      contour.Name,
		},
	}

	if config.Spec.Envoy == nil {
		config.Spec.Envoy = &contour_api_v1alpha1.EnvoyConfig{}
	}
	config.Spec.Envoy.Service = &contour_api_v1alpha1.NamespacedName{
		Namespace: contour.Namespace,
		Name:      contour.EnvoyServiceName(),
	}
}

// EnsureContourConfigDeleted deletes a ContourConfig for the provided contour, if the configured owner labels exist.
func EnsureContourConfigDeleted(ctx context.Context, cli client.Client, contour *model.Contour) error {
	getter := func(ctx context.Context, cli client.Client, namespace, name string) (client.Object, error) {
		return current(ctx, cli, namespace, name)
	}
	return objects.EnsureObjectDeleted(ctx, cli, contour, contour.ContourConfigurationName(), getter)

}

// current gets the ContourConfiguration for the provided contour from the api server.
func current(ctx context.Context, cli client.Client, namespace, name string) (*contour_api_v1alpha1.ContourConfiguration, error) {
	current := &contour_api_v1alpha1.ContourConfiguration{}
	key := types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}

	if err := cli.Get(ctx, key, current); err != nil {
		return nil, err
	}

	return current, nil
}
