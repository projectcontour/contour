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

	"github.com/projectcontour/contour/internal/ref"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	gatewayapi_v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

type ValidateListenersResult struct {
	// ListenerNames is a map from Gateway Listener name
	// to DAG/Envoy Listener name. All Gateway Listeners
	// that share a port map to the same DAG/Envoy Listener
	// name.
	ListenerNames map[string]string

	// Ports is a list of ports to listen on.
	Ports []ListenerPort

	// InvalidListenerConditions is a map from Gateway Listener name
	// to a condition to set, if the Listener is invalid.
	InvalidListenerConditions map[gatewayapi_v1beta1.SectionName]metav1.Condition
}

type ListenerPort struct {
	Name          string
	Port          int32
	ContainerPort int32
}

// ValidateListeners validates protocols, ports and hostnames on a set of listeners.
// It ensures that:
//   - all protocols are supported
//   - each listener group (grouped by protocol, with HTTPS & TLS going together) uses a single port
//   - listener hostnames are syntactically valid
//   - hostnames within each listener group are unique
//
// It returns a Listener name map, the ports to use, and conditions for all invalid listeners.
// If a listener is not in the "InvalidListenerConditions" map, it is assumed to be valid according
// to the above rules.
func ValidateListeners(listeners []gatewayapi_v1beta1.Listener) ValidateListenersResult {
	result := ValidateListenersResult{
		ListenerNames:             map[string]string{},
		InvalidListenerConditions: map[gatewayapi_v1beta1.SectionName]metav1.Condition{},
	}

	for i, listener := range listeners {
		// Check for a valid hostname.
		if hostname := ref.Val(listener.Hostname, ""); len(hostname) > 0 {
			if err := IsValidHostname(string(hostname)); err != nil {
				result.InvalidListenerConditions[listener.Name] = metav1.Condition{
					Type:    string(gatewayapi_v1beta1.ListenerConditionProgrammed),
					Status:  metav1.ConditionFalse,
					Reason:  string(gatewayapi_v1beta1.ListenerReasonInvalid),
					Message: err.Error(),
				}
				continue
			}
		}

		// Check for a supported protocol.
		switch listener.Protocol {
		case gatewayapi_v1beta1.HTTPProtocolType, gatewayapi_v1beta1.HTTPSProtocolType, gatewayapi_v1beta1.TLSProtocolType:
		default:
			result.InvalidListenerConditions[listener.Name] = metav1.Condition{
				Type:    string(gatewayapi_v1beta1.ListenerConditionAccepted),
				Status:  metav1.ConditionFalse,
				Reason:  string(gatewayapi_v1beta1.ListenerReasonUnsupportedProtocol),
				Message: fmt.Sprintf("Listener protocol %q is unsupported, must be one of HTTP, HTTPS or TLS", listener.Protocol),
			}
			continue
		}

		// Check for conflicts with other listeners.
		conflicted := false
		for j, otherListener := range listeners {
			// Don't self-compare.
			if i == j {
				continue
			}

			// Listeners on other ports never conflict.
			if listener.Port != otherListener.Port {
				continue
			}

			// Protocol conflict
			switch listener.Protocol {
			case gatewayapi_v1beta1.HTTPProtocolType:
				if otherListener.Protocol != gatewayapi_v1beta1.HTTPProtocolType {
					result.InvalidListenerConditions[listener.Name] = metav1.Condition{
						Type:    string(gatewayapi_v1beta1.ListenerConditionConflicted),
						Status:  metav1.ConditionTrue,
						Reason:  string(gatewayapi_v1beta1.ListenerReasonProtocolConflict),
						Message: "All Listener protocols for a given port must be compatible",
					}
					conflicted = true
				}
			case gatewayapi_v1beta1.HTTPSProtocolType, gatewayapi_v1beta1.TLSProtocolType:
				if otherListener.Protocol != gatewayapi_v1beta1.HTTPSProtocolType && otherListener.Protocol != gatewayapi_v1beta1.TLSProtocolType {
					result.InvalidListenerConditions[listener.Name] = metav1.Condition{
						Type:    string(gatewayapi_v1beta1.ListenerConditionConflicted),
						Status:  metav1.ConditionTrue,
						Reason:  string(gatewayapi_v1beta1.ListenerReasonProtocolConflict),
						Message: "All Listener protocols for a given port must be compatible",
					}
					conflicted = true
				}
			}
			if conflicted {
				break
			}

			// Hostname conflict
			if ref.Val(listener.Hostname, "") == ref.Val(otherListener.Hostname, "") {
				result.InvalidListenerConditions[listener.Name] = metav1.Condition{
					Type:    string(gatewayapi_v1beta1.ListenerConditionConflicted),
					Status:  metav1.ConditionTrue,
					Reason:  string(gatewayapi_v1beta1.ListenerReasonHostnameConflict),
					Message: "All Listener hostnames for a given port must be unique",
				}
				conflicted = true
			}

			if conflicted {
				break
			}
		}

		if conflicted {
			continue
		}

		// Add an entry in the Listener name map.
		var prefix string
		if listener.Protocol == gatewayapi_v1beta1.HTTPProtocolType {
			prefix = "http"
		} else {
			prefix = "https"
		}
		envoyListenerName := fmt.Sprintf("%s-%d", prefix, listener.Port)

		result.ListenerNames[string(listener.Name)] = envoyListenerName

		// Add the port to the list if it hasn't been added already.
		found := false
		for _, port := range result.Ports {
			if port.Name == envoyListenerName {
				found = true
				break
			}
		}

		if !found {
			// Map privileged ports (1-1023) to the range 64513-65535 within
			// the container. All other ports can be used as-is inside the
			// container.
			containerPort := listener.Port
			if containerPort < 1024 {
				containerPort += 64512
			}

			result.Ports = append(result.Ports, ListenerPort{
				Name:          envoyListenerName,
				Port:          int32(listener.Port),
				ContainerPort: int32(containerPort),
			})
		}
	}

	return result
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
