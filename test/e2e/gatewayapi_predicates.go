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

//go:build e2e

package e2e

import (
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayapi_v1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

// GatewayClassAccepted returns true if the gateway has a .status.conditions
// entry of Accepted: true".
func GatewayClassAccepted(gatewayClass *gatewayapi_v1.GatewayClass) bool {
	if gatewayClass == nil {
		return false
	}

	return conditionExists(
		gatewayClass.Status.Conditions,
		string(gatewayapi_v1.GatewayClassConditionStatusAccepted),
		meta_v1.ConditionTrue,
	)
}

// GatewayClassNotAccepted returns true if the gateway has a .status.conditions
// entry of Accepted: false".
func GatewayClassNotAccepted(gatewayClass *gatewayapi_v1.GatewayClass) bool {
	if gatewayClass == nil {
		return false
	}

	return conditionExists(
		gatewayClass.Status.Conditions,
		string(gatewayapi_v1.GatewayClassConditionStatusAccepted),
		meta_v1.ConditionFalse,
	)
}

// GatewayAccepted returns true if the gateway has a .status.conditions
// entry of "Accepted: true".
func GatewayAccepted(gateway *gatewayapi_v1.Gateway) bool {
	if gateway == nil {
		return false
	}

	return conditionExists(
		gateway.Status.Conditions,
		string(gatewayapi_v1.GatewayConditionAccepted),
		meta_v1.ConditionTrue,
	)
}

// GatewayProgrammed returns true if the gateway has a .status.conditions
// entry of "Programmed: true".
func GatewayProgrammed(gateway *gatewayapi_v1.Gateway) bool {
	if gateway == nil {
		return false
	}

	return conditionExists(
		gateway.Status.Conditions,
		string(gatewayapi_v1.GatewayConditionProgrammed),
		meta_v1.ConditionTrue,
	)
}

// ListenerAccepted returns true if the gateway has status for the named
// listener with a condition of "Accepted: true".
func ListenerAccepted(gateway *gatewayapi_v1.Gateway, listener gatewayapi_v1.SectionName) bool {
	for _, listenerStatus := range gateway.Status.Listeners {
		if listenerStatus.Name == listener {
			return conditionExists(
				listenerStatus.Conditions,
				string(gatewayapi_v1.ListenerConditionAccepted),
				meta_v1.ConditionTrue,
			)
		}
	}

	return false
}

// GatewayHasAddress returns true if the gateway has a non-empty
// .status.addresses entry.
func GatewayHasAddress(gateway *gatewayapi_v1.Gateway) bool {
	if gateway == nil {
		return false
	}

	return len(gateway.Status.Addresses) > 0 && gateway.Status.Addresses[0].Value != ""
}

// HTTPRouteAccepted returns true if the route has a .status.conditions
// entry of "Accepted: true".
func HTTPRouteAccepted(route *gatewayapi_v1.HTTPRoute) bool {
	if route == nil {
		return false
	}

	for _, gw := range route.Status.Parents {
		if conditionExists(gw.Conditions, string(gatewayapi_v1.RouteConditionAccepted), meta_v1.ConditionTrue) {
			return true
		}
	}

	return false
}

// HTTPRouteIgnoredByContour returns true if the route has an empty .status.parents.conditions list
func HTTPRouteIgnoredByContour(route *gatewayapi_v1.HTTPRoute) bool {
	if route == nil {
		return false
	}

	return len(route.Status.Parents) == 0
}

// TCPRouteAccepted returns true if the route has a .status.conditions
// entry of "Accepted: true".
func TCPRouteAccepted(route *gatewayapi_v1alpha2.TCPRoute) bool {
	if route == nil {
		return false
	}

	for _, gw := range route.Status.Parents {
		if conditionExists(gw.Conditions, string(gatewayapi_v1.RouteConditionAccepted), meta_v1.ConditionTrue) {
			return true
		}
	}

	return false
}

// TLSRouteIgnoredByContour returns true if the route has an empty .status.parents.conditions list
func TLSRouteIgnoredByContour(route *gatewayapi_v1alpha2.TLSRoute) bool {
	if route == nil {
		return false
	}

	return len(route.Status.Parents) == 0
}

// TLSRouteAccepted returns true if the route has a .status.conditions
// entry of "Accepted: true".
func TLSRouteAccepted(route *gatewayapi_v1alpha2.TLSRoute) bool {
	if route == nil {
		return false
	}

	for _, gw := range route.Status.Parents {
		if conditionExists(gw.Conditions, string(gatewayapi_v1alpha2.RouteConditionAccepted), meta_v1.ConditionTrue) {
			return true
		}
	}

	return false
}

// BackendTLSPolicyAccepted returns true if the backend TLS policy has a .status.conditions
// entry of "Accepted: true".
func BackendTLSPolicyAccepted(btp *gatewayapi_v1alpha2.BackendTLSPolicy) bool {
	if btp == nil {
		return false
	}

	for _, gw := range btp.Status.Ancestors {
		if conditionExists(gw.Conditions, string(gatewayapi_v1alpha2.PolicyConditionAccepted), meta_v1.ConditionTrue) {
			return true
		}
	}

	return false
}

func conditionExists(conditions []meta_v1.Condition, conditionType string, conditionStatus meta_v1.ConditionStatus) bool {
	for _, cond := range conditions {
		if cond.Type == conditionType && cond.Status == conditionStatus {
			return true
		}
	}

	return false
}
