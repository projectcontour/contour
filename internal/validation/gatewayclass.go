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

	apivalidation "k8s.io/apimachinery/pkg/api/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayapi_v1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

// Note: gatewayclass controller immutability cannot be enforced without the
// webhook or https://github.com/kubernetes/enhancements/issues/1101.

// ValidateGatewayClassName validates that the given name can be used as
// a GatewayClass name.
var ValidateGatewayClassName = apivalidation.NameIsDNSSubdomain

// ValidateGatewayClass validates gc according to the Gateway API specification.
// For additional details of the Gateway spec, refer to:
//   https://gateway-api.sigs.k8s.io/spec/#networking.x-k8s.io/v1alpha1.Gateway
func ValidateGatewayClass(ctx context.Context, cli client.Client, gc *gatewayapi_v1alpha1.GatewayClass) field.ErrorList {
	var errs field.ErrorList

	errs = append(errs, validateGatewayClassObjMeta(ctx, cli, &gc.ObjectMeta, field.NewPath("metadata"))...)
	errs = append(errs, validateGatewayClassSpec(&gc.Spec, field.NewPath("spec"))...)

	return errs
}

// validateGatewayClassObjMeta validates whether required fields of metadata are set according
// to the Gateway API specification.
func validateGatewayClassObjMeta(ctx context.Context, cli client.Client, meta *metav1.ObjectMeta, path *field.Path) field.ErrorList {
	errs := apivalidation.ValidateObjectMeta(meta, false, ValidateGatewayClassName, path.Child("name"))

	classes := &gatewayapi_v1alpha1.GatewayClassList{}
	if err := cli.List(ctx, classes); err != nil {
		errs = append(errs, field.InternalError(path, fmt.Errorf("failed to list gatewayclasses: %v", err)))
	} else {
		if len(classes.Items) > 1 {
			errs = append(errs, field.InternalError(path, fmt.Errorf("only 1 gatewayclass is supported")))
		}
	}

	return errs
}

// validateGatewayClassSpec validates whether required fields of spec are set according
// to the Gateway API specification.
func validateGatewayClassSpec(spec *gatewayapi_v1alpha1.GatewayClassSpec, path *field.Path) field.ErrorList {
	var errs field.ErrorList
	if spec.ParametersRef != nil {
		errs = append(errs, field.NotSupported(path.Child("parametersRef"), spec.ParametersRef, []string{"nil"}))
	}
	return errs
}
