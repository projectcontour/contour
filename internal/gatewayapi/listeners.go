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
	"fmt"
	"net"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	gatewayapi_v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

type ValidateListenersResult struct {
	InsecurePort int
	SecurePort   int

	InvalidListenerConditions map[gatewayapi_v1beta1.SectionName]metav1.Condition
}

// ValidateListeners validates protocols, ports and hostnames on a set of listeners.
// It ensures that:
//   - all protocols are supported
//   - each listener group (grouped by protocol, with HTTPS & TLS going together) uses a single port
//   - listener hostnames are syntactically valid
//   - hostnames within each listener group are unique
//
// It returns the insecure & secure ports to use, as well as conditions for all invalid listeners.
// If a listener is not in the "InvalidListenerConditions" map, it is assumed to be valid according
// to the above rules.
func ValidateListeners(listeners []gatewayapi_v1beta1.Listener) ValidateListenersResult {
	result := ValidateListenersResult{
		InvalidListenerConditions: map[gatewayapi_v1beta1.SectionName]metav1.Condition{},
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
		hostname := HostnameDeref(listener.Hostname)

		switch listener.Protocol {
		case gatewayapi_v1beta1.HTTPProtocolType:
			// Keep the first insecure listener port we see
			if result.InsecurePort == 0 {
				result.InsecurePort = int(listener.Port)
			}

			// Count hostnames among insecure listeners with the "valid" port.
			// For other insecure listeners with an "invalid" port, the
			// "PortUnavailable" reason will take precedence.
			if int(listener.Port) == result.InsecurePort {
				insecureHostnames[hostname]++
			}
		case gatewayapi_v1beta1.HTTPSProtocolType, gatewayapi_v1beta1.TLSProtocolType:
			// Keep the first secure listener port we see
			if result.SecurePort == 0 {
				result.SecurePort = int(listener.Port)
			}

			// Count hostnames among secure listeners with the "valid" port.
			// For other secure listeners with an "invalid" port, the
			// "PortUnavailable" reason will take precedence.
			if int(listener.Port) == result.SecurePort {
				secureHostnames[hostname]++
			}
		}
	}

	for _, listener := range listeners {
		hostname := HostnameDeref(listener.Hostname)

		if len(hostname) > 0 {
			if err := IsValidHostname(hostname); err != nil {
				result.InvalidListenerConditions[listener.Name] = metav1.Condition{
					Type:    string(gatewayapi_v1beta1.ListenerConditionReady),
					Status:  metav1.ConditionFalse,
					Reason:  string(gatewayapi_v1beta1.ListenerReasonInvalid),
					Message: err.Error(),
				}
			}
		}

		switch listener.Protocol {
		case gatewayapi_v1beta1.HTTPProtocolType:
			switch {
			case int(listener.Port) != result.InsecurePort:
				result.InvalidListenerConditions[listener.Name] = metav1.Condition{
					Type:    string(gatewayapi_v1beta1.ListenerConditionDetached),
					Status:  metav1.ConditionTrue,
					Reason:  string(gatewayapi_v1beta1.ListenerReasonPortUnavailable),
					Message: "Only one HTTP port is supported",
				}
			case insecureHostnames[hostname] > 1:
				result.InvalidListenerConditions[listener.Name] = metav1.Condition{
					Type:    string(gatewayapi_v1beta1.ListenerConditionConflicted),
					Status:  metav1.ConditionTrue,
					Reason:  string(gatewayapi_v1beta1.ListenerReasonHostnameConflict),
					Message: "Hostname must be unique among HTTP listeners",
				}
			}
		case gatewayapi_v1beta1.HTTPSProtocolType, gatewayapi_v1beta1.TLSProtocolType:
			switch {
			case int(listener.Port) != result.SecurePort:
				result.InvalidListenerConditions[listener.Name] = metav1.Condition{
					Type:    string(gatewayapi_v1beta1.ListenerConditionDetached),
					Status:  metav1.ConditionTrue,
					Reason:  string(gatewayapi_v1beta1.ListenerReasonPortUnavailable),
					Message: "Only one HTTPS/TLS port is supported",
				}
			case secureHostnames[hostname] > 1:
				result.InvalidListenerConditions[listener.Name] = metav1.Condition{
					Type:    string(gatewayapi_v1beta1.ListenerConditionConflicted),
					Status:  metav1.ConditionTrue,
					Reason:  string(gatewayapi_v1beta1.ListenerReasonHostnameConflict),
					Message: "Hostname must be unique among HTTPS/TLS listeners",
				}
			}
		default:
			result.InvalidListenerConditions[listener.Name] = metav1.Condition{
				Type:    string(gatewayapi_v1beta1.ListenerConditionDetached),
				Status:  metav1.ConditionTrue,
				Reason:  string(gatewayapi_v1beta1.ListenerReasonUnsupportedProtocol),
				Message: fmt.Sprintf("Listener protocol %q is unsupported, must be one of HTTP, HTTPS or TLS", listener.Protocol),
			}
		}
	}

	return result
}

// HostnameDeref returns the hostname as a string if it's not nil,
// or an empty string otherwise.
func HostnameDeref(hostname *gatewayapi_v1beta1.Hostname) string {
	if hostname == nil {
		return ""
	}

	return string(*hostname)
}

// IsValidHostname validates that a given hostname is syntactically valid.
// It returns nil if valid and an error if not valid.
func IsValidHostname(hostname string) error {
	if net.ParseIP(hostname) != nil {
		return fmt.Errorf("invalid hostname %q: must be a DNS name, not an IP address", hostname)
	}

	if strings.Contains(hostname, "*") {
		if errs := validation.IsWildcardDNS1123Subdomain(hostname); errs != nil {
			return fmt.Errorf("invalid hostname %q: %v", hostname, errs)
		}
	} else {
		if errs := validation.IsDNS1123Subdomain(hostname); errs != nil {
			return fmt.Errorf("invalid hostname %q: %v", hostname, errs)
		}
	}

	return nil
}
