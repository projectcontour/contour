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

package v1alpha1

const (
	// GatewayClassControllerRef identifies contour operator as the managing controller
	// of a GatewayClass.
	// DEPRECATED: The contour operator no longer reconciles GatewayClasses.
	GatewayClassControllerRef = "projectcontour.io/contour-operator"

	// GatewayClassParamsRefGroup identifies contour operator as the group name of a
	// GatewayClass.
	// DEPRECATED: The contour operator no longer reconciles GatewayClasses.
	GatewayClassParamsRefGroup = "operator.projectcontour.io"

	// GatewayClassParamsRefKind identifies Contour as the kind name of a GatewayClass.
	// DEPRECATED: The contour operator no longer reconciles GatewayClasses.
	GatewayClassParamsRefKind = "Contour"

	// GatewayFinalizer is the name of the finalizer used for a Gateway.
	// DEPRECATED: The contour operator no longer reconciles Gateways.
	GatewayFinalizer = "gateway.networking.x-k8s.io/finalizer"

	// OwningGatewayNameLabel is the owner reference label used for a Gateway
	// managed by the operator. The value should be the name of the Gateway.
	// DEPRECATED: The contour operator no longer reconciles Gateways.
	OwningGatewayNameLabel = "contour.operator.projectcontour.io/owning-gateway-name"

	// OwningGatewayNsLabel is the owner reference label used for a Gateway
	// managed by the operator. The value should be the namespace of the Gateway.
	// DEPRECATED: The contour operator no longer reconciles Gateways.
	OwningGatewayNsLabel = "contour.operator.projectcontour.io/owning-gateway-namespace"
)

// IsFinalized returns true if Contour is finalized.
func (c *Contour) IsFinalized() bool {
	for _, f := range c.Finalizers {
		if f == ContourFinalizer {
			return true
		}
	}
	return false
}

// GatewayClassSet returns true if gatewayClassRef is set for Contour.
// DEPRECATED: The GatewayClassRef field is deprecated.
func (c *Contour) GatewayClassSet() bool {
	return c.Spec.GatewayClassRef != nil
}

// ContourNodeSelectorExists returns true if a nodeSelector is specified for Contour.
func (c *Contour) ContourNodeSelectorExists() bool {
	if c.Spec.NodePlacement != nil &&
		c.Spec.NodePlacement.Contour != nil &&
		c.Spec.NodePlacement.Contour.NodeSelector != nil {
		return true
	}

	return false
}

// ContourTolerationsExist returns true if tolerations are set for Contour.
func (c *Contour) ContourTolerationsExist() bool {
	if c.Spec.NodePlacement != nil &&
		c.Spec.NodePlacement.Contour != nil &&
		len(c.Spec.NodePlacement.Contour.Tolerations) > 0 {
		return true
	}

	return false
}

// EnvoyNodeSelectorExists returns true if a nodeSelector is specified for Envoy.
func (c *Contour) EnvoyNodeSelectorExists() bool {
	if c.Spec.NodePlacement != nil &&
		c.Spec.NodePlacement.Envoy != nil &&
		c.Spec.NodePlacement.Envoy.NodeSelector != nil {
		return true
	}

	return false
}

// EnvoyTolerationsExist returns true if tolerations are set for Envoy.
func (c *Contour) EnvoyTolerationsExist() bool {
	if c.Spec.NodePlacement != nil &&
		c.Spec.NodePlacement.Envoy != nil &&
		len(c.Spec.NodePlacement.Envoy.Tolerations) > 0 {
		return true
	}

	return false
}
