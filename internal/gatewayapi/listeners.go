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
	InsecurePort int
	SecurePort   int

	InvalidListenerConditions map[gatewayapi_v1alpha2.SectionName]metav1.Condition
}

func ValidateListeners(listeners []gatewayapi_v1alpha2.Listener) ValidateListenersResult {
	result := ValidateListenersResult{
		InvalidListenerConditions: map[gatewayapi_v1alpha2.SectionName]metav1.Condition{},
	}

	// All listeners with a protocol of "HTTP" must use the same port number
	// Heuristic: the first port number encountered is allowed, any other listeners with a different port number are marked "Detached" with "PortUnavailable"
	// All listeners with a protocol of "HTTP" using the one allowed port must have a unique hostname
	// Any listener with a duplicate hostname is marked "Conflicted" with "HostnameConflict"

	var (
		insecureHostnames = map[string]int{}
		secureHostnames   = map[string]int{}
	)

	for _, listener := range listeners {
		switch listener.Protocol {
		case gatewayapi_v1alpha2.HTTPProtocolType:
			// Keep the first insecure listener port we see
			if result.InsecurePort == 0 {
				result.InsecurePort = int(listener.Port)
			}

			// Count hostnames among insecure listeners with the "valid" port.
			// For other insecure listeners with an "invalid" port, the
			// "PortUnavailable" reason will take precedence.
			if int(listener.Port) == result.InsecurePort {
				insecureHostnames[listenerHostname(listener)]++
			}
		case gatewayapi_v1alpha2.HTTPSProtocolType, gatewayapi_v1alpha2.TLSProtocolType:
			// Keep the first secure listener port we see
			if result.SecurePort == 0 {
				result.SecurePort = int(listener.Port)
			}

			// Count hostnames among secure listeners with the "valid" port.
			// For other secure listeners with an "invalid" port, the
			// "PortUnavailable" reason will take precedence.
			if int(listener.Port) == result.SecurePort {
				secureHostnames[listenerHostname(listener)]++
			}
		}
	}

	for _, listener := range listeners {
		switch listener.Protocol {
		case gatewayapi_v1alpha2.HTTPProtocolType:
			switch {
			case int(listener.Port) != result.InsecurePort:
				result.InvalidListenerConditions[listener.Name] = metav1.Condition{
					Type:    string(gatewayapi_v1alpha2.ListenerConditionDetached),
					Status:  metav1.ConditionTrue,
					Reason:  string(gatewayapi_v1alpha2.ListenerReasonPortUnavailable),
					Message: "Only one HTTP port is supported",
				}
			case insecureHostnames[listenerHostname(listener)] > 1:
				result.InvalidListenerConditions[listener.Name] = metav1.Condition{
					Type:    string(gatewayapi_v1alpha2.ListenerConditionConflicted),
					Status:  metav1.ConditionTrue,
					Reason:  string(gatewayapi_v1alpha2.ListenerReasonHostnameConflict),
					Message: "Hostname must be unique among HTTP listeners",
				}
			}
		case gatewayapi_v1alpha2.HTTPSProtocolType, gatewayapi_v1alpha2.TLSProtocolType:
			switch {
			case int(listener.Port) != result.SecurePort:
				result.InvalidListenerConditions[listener.Name] = metav1.Condition{
					Type:    string(gatewayapi_v1alpha2.ListenerConditionDetached),
					Status:  metav1.ConditionTrue,
					Reason:  string(gatewayapi_v1alpha2.ListenerReasonPortUnavailable),
					Message: "Only one HTTPS/TLS port is supported",
				}
			case secureHostnames[listenerHostname(listener)] > 1:
				result.InvalidListenerConditions[listener.Name] = metav1.Condition{
					Type:    string(gatewayapi_v1alpha2.ListenerConditionConflicted),
					Status:  metav1.ConditionTrue,
					Reason:  string(gatewayapi_v1alpha2.ListenerReasonHostnameConflict),
					Message: "Hostname must be unique among HTTPS/TLS listeners",
				}
			}
		default:
			// Unsupported protocol: ignore (will be handled in DAG processing)
			// TODO(sk) probably makes sense to move the handling of unsupported
			// protocols in here for cohesion.
			continue
		}
	}

	return result
}

func listenerHostname(listener gatewayapi_v1alpha2.Listener) string {
	if listener.Hostname != nil {
		return string(*listener.Hostname)
	}
	return ""
}
