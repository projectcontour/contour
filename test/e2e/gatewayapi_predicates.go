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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayapi_v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

// GatewayClassAccepted returns true if the gateway has a .status.conditions
// entry of Accepted: true".
func GatewayClassAccepted(gatewayClass *gatewayapi_v1beta1.GatewayClass) bool {
	if gatewayClass == nil {
		return false
	}

	return conditionExists(
		gatewayClass.Status.Conditions,
		string(gatewayapi_v1beta1.GatewayClassConditionStatusAccepted),
		metav1.ConditionTrue,
	)
}

// GatewayClassNotAccepted returns true if the gateway has a .status.conditions
// entry of Accepted: false".
func GatewayClassNotAccepted(gatewayClass *gatewayapi_v1beta1.GatewayClass) bool {
	if gatewayClass == nil {
		return false
	}

	return conditionExists(
		gatewayClass.Status.Conditions,
		string(gatewayapi_v1beta1.GatewayClassConditionStatusAccepted),
		metav1.ConditionFalse,
	)
}

// GatewayAccepted returns true if the gateway has a .status.conditions
// entry of "Accepted: true".
func GatewayAccepted(gateway *gatewayapi_v1beta1.Gateway) bool {
	if gateway == nil {
		return false
	}

	return conditionExists(
		gateway.Status.Conditions,
		string(gatewayapi_v1beta1.GatewayConditionAccepted),
		metav1.ConditionTrue,
	)
}

// GatewayProgrammed returns true if the gateway has a .status.conditions
// entry of "Programmed: true".
func GatewayProgrammed(gateway *gatewayapi_v1beta1.Gateway) bool {
	if gateway == nil {
		return false
	}

	return conditionExists(
		gateway.Status.Conditions,
		string(gatewayapi_v1beta1.GatewayConditionProgrammed),
		metav1.ConditionTrue,
	)
}

// GatewayHasAddress returns true if the gateway has a non-empty
// .status.addresses entry.
func GatewayHasAddress(gateway *gatewayapi_v1beta1.Gateway) bool {
	if gateway == nil {
		return false
	}

	return len(gateway.Status.Addresses) > 0 && gateway.Status.Addresses[0].Value != ""
}

// HTTPRouteAccepted returns true if the route has a .status.conditions
// entry of "Accepted: true".
func HTTPRouteAccepted(route *gatewayapi_v1beta1.HTTPRoute) bool {
	if route == nil {
		return false
	}

	for _, gw := range route.Status.Parents {
		if conditionExists(gw.Conditions, string(gatewayapi_v1beta1.RouteConditionAccepted), metav1.ConditionTrue) {
			return true
		}
	}

	return false
}

func conditionExists(conditions []metav1.Condition, conditionType string, conditionStatus metav1.ConditionStatus) bool {
	for _, cond := range conditions {
		if cond.Type == conditionType && cond.Status == conditionStatus {
			return true
		}
	}

	return false
}
