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
	"testing"

	"github.com/stretchr/testify/assert"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	gatewayapi_v1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestValidateListeners(t *testing.T) {
	t.Run("All HTTP listeners are valid on a single port, some non-HTTP listeners as well", func(t *testing.T) {
		listeners := []gatewayapi_v1.Listener{
			{
				Name:     "listener-1",
				Protocol: gatewayapi_v1.HTTPProtocolType,
				Port:     80,
			},
			{
				Name:     "listener-2",
				Protocol: gatewayapi_v1.HTTPProtocolType,
				Port:     80,
				Hostname: ptr.To(gatewayapi_v1.Hostname("local.projectcontour.io")),
			},
			{
				Name:     "listener-3",
				Protocol: gatewayapi_v1.HTTPProtocolType,
				Port:     80,
				Hostname: ptr.To(gatewayapi_v1.Hostname("*.projectcontour.io")),
			},
			{
				Name:     "listener-4",
				Protocol: gatewayapi_v1.HTTPProtocolType,
				Port:     80,
				Hostname: ptr.To(gatewayapi_v1.Hostname("local.envoyproxy.io")),
			},
			{
				Name:     "non-http-listener-1",
				Protocol: gatewayapi_v1.TLSProtocolType,
				Port:     443,
				Hostname: ptr.To(gatewayapi_v1.Hostname("local.projectcontour.io")),
			},
		}

		res := ValidateListeners(listeners)
		assert.ElementsMatch(t, res.Ports, []ListenerPort{
			{Name: "http-80", Port: 80, ContainerPort: 8080, Protocol: "http"},
			{Name: "https-443", Port: 443, ContainerPort: 8443, Protocol: "https"},
		})
		assert.Empty(t, res.InvalidListenerConditions)
	})

	t.Run("HTTP listeners on multiple ports, some non-HTTP listeners as well", func(t *testing.T) {
		listeners := []gatewayapi_v1.Listener{
			{
				Name:     "listener-1",
				Protocol: gatewayapi_v1.HTTPProtocolType,
				Port:     80,
			},
			{
				Name:     "listener-2",
				Protocol: gatewayapi_v1.HTTPProtocolType,
				Port:     80,
				Hostname: ptr.To(gatewayapi_v1.Hostname("local.projectcontour.io")),
			},
			{
				Name:     "listener-3",
				Protocol: gatewayapi_v1.HTTPProtocolType,
				Port:     80,
				Hostname: ptr.To(gatewayapi_v1.Hostname("*.projectcontour.io")),
			},
			{
				Name:     "listener-4",
				Protocol: gatewayapi_v1.HTTPProtocolType,
				Port:     8080,
				Hostname: ptr.To(gatewayapi_v1.Hostname("local.projectcontour.io")),
			},
			{
				Name:     "non-http-listener-1",
				Protocol: gatewayapi_v1.TLSProtocolType,
				Port:     443,
				Hostname: ptr.To(gatewayapi_v1.Hostname("local.projectcontour.io")),
			},
		}

		res := ValidateListeners(listeners)
		assert.ElementsMatch(t, res.Ports, []ListenerPort{
			{Name: "http-80", Port: 80, ContainerPort: 8080, Protocol: "http"},
			{Name: "http-8080", Port: 8080, ContainerPort: 16080, Protocol: "http"},
			{Name: "https-443", Port: 443, ContainerPort: 8443, Protocol: "https"},
		})
		assert.Empty(t, res.InvalidListenerConditions)
	})

	t.Run("Two HTTP listeners with the same hostname, some HTTP listeners on another port, some non-HTTP listeners as well", func(t *testing.T) {
		listeners := []gatewayapi_v1.Listener{
			{
				Name:     "listener-1",
				Protocol: gatewayapi_v1.HTTPProtocolType,
				Port:     80,
			},
			{
				Name:     "listener-2",
				Protocol: gatewayapi_v1.HTTPProtocolType,
				Port:     80,
				Hostname: ptr.To(gatewayapi_v1.Hostname("local.projectcontour.io")),
			},
			{
				Name:     "listener-3",
				Protocol: gatewayapi_v1.HTTPProtocolType,
				Port:     80,
				Hostname: ptr.To(gatewayapi_v1.Hostname("local.projectcontour.io")), // duplicate hostname
			},
			{
				Name:     "listener-4",
				Protocol: gatewayapi_v1.HTTPProtocolType,
				Port:     80,
				Hostname: ptr.To(gatewayapi_v1.Hostname("local.envoyproxy.io")),
			},
			{
				Name:     "listener-5",
				Protocol: gatewayapi_v1.HTTPProtocolType,
				Port:     8080,
				Hostname: ptr.To(gatewayapi_v1.Hostname("local.envoyproxy.io")),
			},
			{
				Name:     "non-http-listener-1",
				Protocol: gatewayapi_v1.TLSProtocolType, // non-HTTP
				Port:     443,
				Hostname: ptr.To(gatewayapi_v1.Hostname("local.projectcontour.io")),
			},
		}

		res := ValidateListeners(listeners)
		assert.ElementsMatch(t, res.Ports, []ListenerPort{
			{Name: "http-80", Port: 80, ContainerPort: 8080, Protocol: "http"},
			{Name: "http-8080", Port: 8080, ContainerPort: 16080, Protocol: "http"},
			{Name: "https-443", Port: 443, ContainerPort: 8443, Protocol: "https"},
		})
		assert.Equal(t, map[gatewayapi_v1.SectionName]meta_v1.Condition{
			"listener-3": {
				Type:    string(gatewayapi_v1.ListenerConditionConflicted),
				Status:  meta_v1.ConditionTrue,
				Reason:  string(gatewayapi_v1.ListenerReasonHostnameConflict),
				Message: "All Listener hostnames for a given port must be unique",
			},
		}, res.InvalidListenerConditions)
	})

	t.Run("All HTTPS/TLS listeners are valid, some non-HTTPS/TLS listeners as well", func(t *testing.T) {
		listeners := []gatewayapi_v1.Listener{
			{
				Name:     "listener-1",
				Protocol: gatewayapi_v1.HTTPSProtocolType,
				Port:     443,
			},
			{
				Name:     "listener-2",
				Protocol: gatewayapi_v1.TLSProtocolType,
				Port:     443,
				Hostname: ptr.To(gatewayapi_v1.Hostname("local.projectcontour.io")),
			},
			{
				Name:     "listener-3",
				Protocol: gatewayapi_v1.HTTPSProtocolType,
				Port:     443,
				Hostname: ptr.To(gatewayapi_v1.Hostname("*.projectcontour.io")),
			},
			{
				Name:     "listener-4",
				Protocol: gatewayapi_v1.TLSProtocolType,
				Port:     443,
				Hostname: ptr.To(gatewayapi_v1.Hostname("local.envoyproxy.io")),
			},
			{
				Name:     "non-http-listener-1",
				Protocol: gatewayapi_v1.HTTPProtocolType,
				Port:     80,
				Hostname: ptr.To(gatewayapi_v1.Hostname("local.projectcontour.io")),
			},
		}

		res := ValidateListeners(listeners)
		assert.ElementsMatch(t, res.Ports, []ListenerPort{
			{Name: "http-80", Port: 80, ContainerPort: 8080, Protocol: "http"},
			{Name: "https-443", Port: 443, ContainerPort: 8443, Protocol: "https"},
		})
		assert.Empty(t, res.InvalidListenerConditions)
	})

	t.Run("HTTPS listeners on two different ports, some non-HTTPS listeners as well", func(t *testing.T) {
		listeners := []gatewayapi_v1.Listener{
			{
				Name:     "listener-1",
				Protocol: gatewayapi_v1.HTTPSProtocolType,
				Port:     443,
			},
			{
				Name:     "listener-2",
				Protocol: gatewayapi_v1.TLSProtocolType,
				Port:     443,
				Hostname: ptr.To(gatewayapi_v1.Hostname("local.projectcontour.io")),
			},
			{
				Name:     "listener-3",
				Protocol: gatewayapi_v1.HTTPSProtocolType,
				Port:     443,
				Hostname: ptr.To(gatewayapi_v1.Hostname("*.projectcontour.io")),
			},
			{
				Name:     "listener-4",
				Protocol: gatewayapi_v1.HTTPSProtocolType,
				Port:     8443,
				Hostname: ptr.To(gatewayapi_v1.Hostname("local.projectcontour.io")),
			},
			{
				Name:     "http-listener-1",
				Protocol: gatewayapi_v1.HTTPProtocolType,
				Port:     80,
				Hostname: ptr.To(gatewayapi_v1.Hostname("local.projectcontour.io")),
			},
		}

		res := ValidateListeners(listeners)
		assert.ElementsMatch(t, res.Ports, []ListenerPort{
			{Name: "http-80", Port: 80, ContainerPort: 8080, Protocol: "http"},
			{Name: "https-443", Port: 443, ContainerPort: 8443, Protocol: "https"},
			{Name: "https-8443", Port: 8443, ContainerPort: 16443, Protocol: "https"},
		})
		assert.Empty(t, res.InvalidListenerConditions)
	})

	t.Run("Two HTTPS/TLS listeners on same port with the same hostname, some HTTPS/TLS listeners on another port, some HTTP listeners as well", func(t *testing.T) {
		listeners := []gatewayapi_v1.Listener{
			{
				Name:     "listener-1",
				Protocol: gatewayapi_v1.HTTPSProtocolType,
				Port:     443,
			},
			{
				Name:     "listener-2",
				Protocol: gatewayapi_v1.HTTPSProtocolType,
				Port:     443,
				Hostname: ptr.To(gatewayapi_v1.Hostname("local.projectcontour.io")),
			},
			{
				Name:     "listener-3",
				Protocol: gatewayapi_v1.TLSProtocolType,
				Port:     443,
				Hostname: ptr.To(gatewayapi_v1.Hostname("local.projectcontour.io")), // duplicate hostname
			},
			{
				Name:     "listener-4",
				Protocol: gatewayapi_v1.HTTPSProtocolType,
				Port:     443,
				Hostname: ptr.To(gatewayapi_v1.Hostname("local.envoyproxy.io")),
			},
			{
				Name:     "listener-5",
				Protocol: gatewayapi_v1.HTTPSProtocolType,
				Port:     8443,
				Hostname: ptr.To(gatewayapi_v1.Hostname("local.envoyproxy.io")),
			},
			{
				Name:     "http-listener-1",
				Protocol: gatewayapi_v1.HTTPProtocolType,
				Port:     80,
				Hostname: ptr.To(gatewayapi_v1.Hostname("local.projectcontour.io")),
			},
		}

		res := ValidateListeners(listeners)
		assert.ElementsMatch(t, res.Ports, []ListenerPort{
			{Name: "http-80", Port: 80, ContainerPort: 8080, Protocol: "http"},
			{Name: "https-443", Port: 443, ContainerPort: 8443, Protocol: "https"},
			{Name: "https-8443", Port: 8443, ContainerPort: 16443, Protocol: "https"},
		})
		assert.Equal(t, map[gatewayapi_v1.SectionName]meta_v1.Condition{
			"listener-3": {
				Type:    string(gatewayapi_v1.ListenerConditionConflicted),
				Status:  meta_v1.ConditionTrue,
				Reason:  string(gatewayapi_v1.ListenerReasonHostnameConflict),
				Message: "All Listener hostnames for a given port must be unique",
			},
		}, res.InvalidListenerConditions)
	})

	t.Run("Two HTTP and one HTTPS listeners, each with an invalid hostname", func(t *testing.T) {
		listeners := []gatewayapi_v1.Listener{
			{
				Name:     "listener-1",
				Protocol: gatewayapi_v1.HTTPProtocolType,
				Port:     80,
				Hostname: ptr.To(gatewayapi_v1.Hostname("192.168.1.1")),
			},
			{
				Name:     "listener-2",
				Protocol: gatewayapi_v1.HTTPProtocolType,
				Port:     80,
				Hostname: ptr.To(gatewayapi_v1.Hostname("*.*.projectcontour.io")),
			},
			{
				Name:     "listener-3",
				Protocol: gatewayapi_v1.HTTPSProtocolType,
				Port:     443,
				Hostname: ptr.To(gatewayapi_v1.Hostname(".invalid.$.")),
			},
		}

		res := ValidateListeners(listeners)
		assert.Empty(t, res.Ports)
		assert.Equal(t, map[gatewayapi_v1.SectionName]meta_v1.Condition{
			"listener-1": {
				Type:    string(gatewayapi_v1.ListenerConditionProgrammed),
				Status:  meta_v1.ConditionFalse,
				Reason:  string(gatewayapi_v1.ListenerReasonInvalid),
				Message: "invalid hostname \"192.168.1.1\": must be a DNS name, not an IP address",
			},
			"listener-2": {
				Type:    string(gatewayapi_v1.ListenerConditionProgrammed),
				Status:  meta_v1.ConditionFalse,
				Reason:  string(gatewayapi_v1.ListenerReasonInvalid),
				Message: "invalid hostname \"*.*.projectcontour.io\": [a wildcard DNS-1123 subdomain must start with '*.', followed by a valid DNS subdomain, which must consist of lower case alphanumeric characters, '-' or '.' and end with an alphanumeric character (e.g. '*.example.com', regex used for validation is '\\*\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*')]",
			},
			"listener-3": {
				Type:    string(gatewayapi_v1.ListenerConditionProgrammed),
				Status:  meta_v1.ConditionFalse,
				Reason:  string(gatewayapi_v1.ListenerReasonInvalid),
				Message: "invalid hostname \".invalid.$.\": [a lowercase RFC 1123 subdomain must consist of lower case alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character (e.g. 'example.com', regex used for validation is '[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*')]",
			},
		}, res.InvalidListenerConditions)
	})

	t.Run("Three HTTPS listeners on the same port, each with a different hostname", func(t *testing.T) {
		listeners := []gatewayapi_v1.Listener{
			{
				Name:     "https-1",
				Protocol: gatewayapi_v1.HTTPSProtocolType,
				Hostname: ptr.To(gatewayapi_v1.Hostname("https-1.gateway.projectcontour.io")),
				Port:     443,
			},
			{
				Name:     "https-2",
				Protocol: gatewayapi_v1.HTTPSProtocolType,
				Hostname: ptr.To(gatewayapi_v1.Hostname("https-2.gateway.projectcontour.io")),
				Port:     443,
			},
			{
				Name:     "https-3",
				Protocol: gatewayapi_v1.HTTPSProtocolType,
				Hostname: ptr.To(gatewayapi_v1.Hostname("https-3.gateway.projectcontour.io")),
				Port:     443,
			},
		}

		res := ValidateListeners(listeners)
		assert.Empty(t, res.InvalidListenerConditions)
		assert.Len(t, res.Ports, 1)
		assert.Len(t, res.ListenerNames, 3)
	})

	t.Run("Conflicting protocols on a port", func(t *testing.T) {
		listeners := []gatewayapi_v1.Listener{
			{
				Name:     "http",
				Protocol: gatewayapi_v1.HTTPProtocolType,
				Port:     7777,
			},
			{
				Name:     "https",
				Protocol: gatewayapi_v1.HTTPSProtocolType,
				Port:     7777,
			},

			{
				Name:     "http-2",
				Protocol: gatewayapi_v1.HTTPProtocolType,
				Port:     9999,
			},
			{
				Name:     "projectcontour-io-https",
				Protocol: ContourHTTPSProtocolType,
				Port:     9999,
			},

			{
				Name:     "tcp-1",
				Protocol: gatewayapi_v1.TCPProtocolType,
				Port:     11111,
			},
			{
				Name:     "tls-1",
				Protocol: gatewayapi_v1.TLSProtocolType,
				Port:     11111,
			},
		}

		res := ValidateListeners(listeners)
		assert.ElementsMatch(t, res.Ports, []ListenerPort{
			{Name: "http-7777", Port: 7777, ContainerPort: 15777, Protocol: "http"},
			{Name: "http-9999", Port: 9999, ContainerPort: 17999, Protocol: "http"},
			{Name: "tcp-11111", Port: 11111, ContainerPort: 19111, Protocol: "tcp"},
		})
		assert.Equal(t, map[gatewayapi_v1.SectionName]meta_v1.Condition{
			"https": {
				Type:    string(gatewayapi_v1.ListenerConditionConflicted),
				Status:  meta_v1.ConditionTrue,
				Reason:  string(gatewayapi_v1.ListenerReasonProtocolConflict),
				Message: "All Listener protocols for a given port must be compatible",
			},
			"projectcontour-io-https": {
				Type:    string(gatewayapi_v1.ListenerConditionConflicted),
				Status:  meta_v1.ConditionTrue,
				Reason:  string(gatewayapi_v1.ListenerReasonProtocolConflict),
				Message: "All Listener protocols for a given port must be compatible",
			},
			"tls-1": {
				Type:    string(gatewayapi_v1.ListenerConditionConflicted),
				Status:  meta_v1.ConditionTrue,
				Reason:  string(gatewayapi_v1.ListenerReasonProtocolConflict),
				Message: "All Listener protocols for a given port must be compatible",
			},
		}, res.InvalidListenerConditions)
	})

	t.Run("Conflicting protocols on a port (reverse order)", func(t *testing.T) {
		listeners := []gatewayapi_v1.Listener{
			{
				Name:     "https",
				Protocol: gatewayapi_v1.HTTPSProtocolType,
				Port:     7777,
			},
			{
				Name:     "http",
				Protocol: gatewayapi_v1.HTTPProtocolType,
				Port:     7777,
			},

			{
				Name:     "projectcontour-io-https",
				Protocol: ContourHTTPSProtocolType,
				Port:     9999,
			},
			{
				Name:     "http-2",
				Protocol: gatewayapi_v1.HTTPProtocolType,
				Port:     9999,
			},

			{
				Name:     "tls-1",
				Protocol: gatewayapi_v1.TLSProtocolType,
				Port:     11111,
			},
			{
				Name:     "tcp-1",
				Protocol: gatewayapi_v1.TCPProtocolType,
				Port:     11111,
			},
		}

		res := ValidateListeners(listeners)
		assert.ElementsMatch(t, res.Ports, []ListenerPort{
			{Name: "https-7777", Port: 7777, ContainerPort: 15777, Protocol: "https"},
			{Name: "https-9999", Port: 9999, ContainerPort: 17999, Protocol: "https"},
			{Name: "https-11111", Port: 11111, ContainerPort: 19111, Protocol: "https"},
		})
		assert.Equal(t, map[gatewayapi_v1.SectionName]meta_v1.Condition{
			"http": {
				Type:    string(gatewayapi_v1.ListenerConditionConflicted),
				Status:  meta_v1.ConditionTrue,
				Reason:  string(gatewayapi_v1.ListenerReasonProtocolConflict),
				Message: "All Listener protocols for a given port must be compatible",
			},
			"http-2": {
				Type:    string(gatewayapi_v1.ListenerConditionConflicted),
				Status:  meta_v1.ConditionTrue,
				Reason:  string(gatewayapi_v1.ListenerReasonProtocolConflict),
				Message: "All Listener protocols for a given port must be compatible",
			},
			"tcp-1": {
				Type:    string(gatewayapi_v1.ListenerConditionConflicted),
				Status:  meta_v1.ConditionTrue,
				Reason:  string(gatewayapi_v1.ListenerReasonProtocolConflict),
				Message: "All Listener protocols for a given port must be compatible",
			},
		}, res.InvalidListenerConditions)
	})

	t.Run("Three TCP listeners on different ports", func(t *testing.T) {
		listeners := []gatewayapi_v1.Listener{
			{
				Name:     "tcp-1",
				Protocol: gatewayapi_v1.TCPProtocolType,
				Port:     10000,
			},
			{
				Name:     "tcp-2",
				Protocol: gatewayapi_v1.TCPProtocolType,
				Port:     10001,
			},
			{
				Name:     "tcp-3",
				Protocol: gatewayapi_v1.TCPProtocolType,
				Port:     10002,
			},
		}
		res := ValidateListeners(listeners)
		assert.Empty(t, res.InvalidListenerConditions)
		assert.Len(t, res.Ports, 3)
		assert.Len(t, res.ListenerNames, 3)
	})

	t.Run("Listeners with various edge-case port numbers", func(t *testing.T) {
		listeners := []gatewayapi_v1.Listener{
			{
				Name:     "http-1",
				Protocol: gatewayapi_v1.HTTPProtocolType,
				Port:     65535, // wraps around, does not hit a privileged port
			},
			{
				Name:     "http-2",
				Protocol: gatewayapi_v1.HTTPProtocolType,
				Port:     57536, // wraps around, hits a privileged port
			},
			{
				Name:     "http-3",
				Protocol: gatewayapi_v1.HTTPProtocolType,
				Port:     58560, // wraps around, does not hit a privileged port
			},
		}

		res := ValidateListeners(listeners)
		assert.ElementsMatch(t, res.Ports, []ListenerPort{
			{Name: "http-65535", Port: 65535, ContainerPort: 8000, Protocol: "http"},
			{Name: "http-57536", Port: 57536, ContainerPort: 1024, Protocol: "http"},
			{Name: "http-58560", Port: 58560, ContainerPort: 1025, Protocol: "http"},
		})
		assert.Empty(t, res.InvalidListenerConditions)
	})

	t.Run("Listeners with ports that map to the same container ports", func(t *testing.T) {
		listeners := []gatewayapi_v1.Listener{
			{
				Name:     "http-1",
				Protocol: gatewayapi_v1.HTTPProtocolType,
				Port:     58000,
			},
			{
				Name:     "http-2",
				Protocol: gatewayapi_v1.HTTPProtocolType,
				Port:     59023,
			},
		}

		res := ValidateListeners(listeners)
		assert.ElementsMatch(t, res.Ports, []ListenerPort{
			{Name: "http-58000", Port: 58000, ContainerPort: 1488, Protocol: "http"},
		})
		assert.Equal(t, map[gatewayapi_v1.SectionName]meta_v1.Condition{
			"http-2": {
				Type:    string(gatewayapi_v1.ListenerConditionAccepted),
				Status:  meta_v1.ConditionFalse,
				Reason:  string(gatewayapi_v1.ListenerReasonPortUnavailable),
				Message: "Listener port conflicts with a previous Listener's port",
			},
		}, res.InvalidListenerConditions)
	})

	t.Run("Listeners with ports that map to the same container ports, reverse order", func(t *testing.T) {
		listeners := []gatewayapi_v1.Listener{
			{
				Name:     "http-1",
				Protocol: gatewayapi_v1.HTTPProtocolType,
				Port:     59000,
			},
			{
				Name:     "http-2",
				Protocol: gatewayapi_v1.HTTPProtocolType,
				Port:     57977,
			},
		}

		res := ValidateListeners(listeners)
		assert.ElementsMatch(t, res.Ports, []ListenerPort{
			{Name: "http-59000", Port: 59000, ContainerPort: 1465, Protocol: "http"},
		})
		assert.Equal(t, map[gatewayapi_v1.SectionName]meta_v1.Condition{
			"http-2": {
				Type:    string(gatewayapi_v1.ListenerConditionAccepted),
				Status:  meta_v1.ConditionFalse,
				Reason:  string(gatewayapi_v1.ListenerReasonPortUnavailable),
				Message: "Listener port conflicts with a previous Listener's port",
			},
		}, res.InvalidListenerConditions)
	})
}
