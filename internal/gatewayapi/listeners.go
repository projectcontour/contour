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

package gatewayapi

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

type ValidateListenersResult struct {
	HTTPPort                      int
	ValidHTTPListeners            []gatewayapi_v1alpha2.Listener
	InvalidHTTPListenerConditions map[gatewayapi_v1alpha2.SectionName]metav1.Condition
}

func ValidateListeners(listeners []gatewayapi_v1alpha2.Listener) ValidateListenersResult {

	// All listeners with a protocol of "HTTP" must use the same port number
	// Heuristic: the first port number encountered is allowed, any other listeners with a different port number are marked "Detached" with "PortUnavailable"
	// All listeners with a protocol of "HTTP" using the one allowed port must have a unique hostname
	// Any listener with a duplicate hostname is marked "Conflicted" with "HostnameConflict"

	httpPort := 0
	hostnames := map[string]int{}

	for _, listener := range listeners {
		if listener.Protocol != gatewayapi_v1alpha2.HTTPProtocolType {
			continue
		}

		// First listener with "HTTP" protocol we've seen: keep its port
		if httpPort == 0 {
			httpPort = int(listener.Port)
		}

		// Count hostnames among HTTP listeners with the "valid" port.
		// For other HTTP listeners with an "invalid" port, the
		// "PortUnavailable" reason will take precedence.
		if int(listener.Port) == httpPort {
			hostnames[listenerHostname(listener)]++
		}
	}

	var validListeners []gatewayapi_v1alpha2.Listener
	invalidListenerConditions := map[gatewayapi_v1alpha2.SectionName]metav1.Condition{}

	for _, listener := range listeners {
		if listener.Protocol != gatewayapi_v1alpha2.HTTPProtocolType {
			continue
		}

		switch {
		case int(listener.Port) != httpPort:
			invalidListenerConditions[listener.Name] = metav1.Condition{
				Type:    string(gatewayapi_v1alpha2.ListenerConditionDetached),
				Status:  metav1.ConditionTrue,
				Reason:  string(gatewayapi_v1alpha2.ListenerReasonPortUnavailable),
				Message: "Only one HTTP port is supported",
			}
		case hostnames[listenerHostname(listener)] > 1:
			invalidListenerConditions[listener.Name] = metav1.Condition{
				Type:    string(gatewayapi_v1alpha2.ListenerConditionConflicted),
				Status:  metav1.ConditionTrue,
				Reason:  string(gatewayapi_v1alpha2.ListenerReasonHostnameConflict),
				Message: "Hostname must be unique among HTTP listeners",
			}
		default:
			validListeners = append(validListeners, listener)
		}

	}

	return ValidateListenersResult{
		HTTPPort:                      httpPort,
		ValidHTTPListeners:            validListeners,
		InvalidHTTPListenerConditions: invalidListenerConditions,
	}
}

func listenerHostname(listener gatewayapi_v1alpha2.Listener) string {
	if listener.Hostname != nil {
		return string(*listener.Hostname)
	}
	return ""
}
