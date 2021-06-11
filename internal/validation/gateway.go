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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayapi_v1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

// ValidateGateway validates gw according to the Gateway API specification.
// For additional details of the Gateway spec, refer to:
//   https://gateway-api.sigs.k8s.io/spec/#networking.x-k8s.io/v1alpha1.Gateway
func ValidateGateway(ctx context.Context, cli client.Client, gw *gatewayapi_v1alpha1.Gateway) field.ErrorList {
	var errs field.ErrorList

	errs = append(errs, validateGatewaySpec(ctx, cli, gw, field.NewPath("spec"))...)

	return errs
}

// validateGatewaySpec validates whether required fields of spec are set according
// to the Gateway API specification.
func validateGatewaySpec(ctx context.Context, cli client.Client, gw *gatewayapi_v1alpha1.Gateway, path *field.Path) field.ErrorList {
	var errs field.ErrorList

	// Get the gatewayclass referenced by gw.
	gcName := gw.Spec.GatewayClassName
	gc := &gatewayapi_v1alpha1.GatewayClass{}
	if err := cli.Get(ctx, types.NamespacedName{Name: gcName}, gc); err != nil {
		errs = append(errs, field.InternalError(path.Child("gatewayClassName"), fmt.Errorf("failed to get gatewayclass %s: %v", gcName, err)))
		// return early since additional validation checks require the gatewayclass.
		return errs
	}

	// See if the referenced gatewayclass is admitted.
	gcAdmitted := false
	for _, c := range gc.Status.Conditions {
		if c.Type == string(gatewayapi_v1alpha1.ConditionRouteAdmitted) && c.Status == metav1.ConditionTrue {
			gcAdmitted = true
		}
	}

	if !gcAdmitted {
		errs = append(errs, field.InternalError(path.Child("gatewayClassName"), fmt.Errorf("gatewayclass %q is not admitted", gcName)))
	}
	// TODO: Add additional validation checks from internal cache and upstream.

	return errs
}
