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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func TestValidateListeners(t *testing.T) {
	// All HTTP listeners are valid, some non-HTTP listeners
	// as well.
	listeners := []gatewayapi_v1alpha2.Listener{
		{
			Name:     "listener-1",
			Protocol: gatewayapi_v1alpha2.HTTPProtocolType,
			Port:     80,
		},
		{
			Name:     "listener-2",
			Protocol: gatewayapi_v1alpha2.HTTPProtocolType,
			Port:     80,
			Hostname: ListenerHostname("local.projectcontour.io"),
		},
		{
			Name:     "listener-3",
			Protocol: gatewayapi_v1alpha2.HTTPProtocolType,
			Port:     80,
			Hostname: ListenerHostname("*.projectcontour.io"),
		},
		{
			Name:     "listener-4",
			Protocol: gatewayapi_v1alpha2.HTTPProtocolType,
			Port:     80,
			Hostname: ListenerHostname("local.envoyproxy.io"),
		},
		{
			Name:     "non-http-listener-1",
			Protocol: gatewayapi_v1alpha2.TLSProtocolType,
			Port:     443,
			Hostname: ListenerHostname("local.projectcontour.io"),
		},
	}

	res := ValidateListeners(listeners)
	assert.Equal(t, 80, res.InsecurePort)
	assert.Empty(t, res.InvalidListenerConditions)

	// One HTTP listener with an invalid port number, some
	// non-HTTP listeners as well.
	listeners = []gatewayapi_v1alpha2.Listener{
		{
			Name:     "listener-1",
			Protocol: gatewayapi_v1alpha2.HTTPProtocolType,
			Port:     80,
		},
		{
			Name:     "listener-2",
			Protocol: gatewayapi_v1alpha2.HTTPProtocolType,
			Port:     80,
			Hostname: ListenerHostname("local.projectcontour.io"),
		},
		{
			Name:     "listener-3",
			Protocol: gatewayapi_v1alpha2.HTTPProtocolType,
			Port:     80,
			Hostname: ListenerHostname("*.projectcontour.io"),
		},
		{
			Name:     "listener-4",
			Protocol: gatewayapi_v1alpha2.HTTPProtocolType,
			Port:     8080,
			Hostname: ListenerHostname("local.projectcontour.io"),
		},
		{
			Name:     "non-http-listener-1",
			Protocol: gatewayapi_v1alpha2.TLSProtocolType,
			Port:     443,
			Hostname: ListenerHostname("local.projectcontour.io"),
		},
	}

	res = ValidateListeners(listeners)
	assert.Equal(t, 80, res.InsecurePort)
	assert.Equal(t, map[gatewayapi_v1alpha2.SectionName]metav1.Condition{
		"listener-4": {
			Type:    string(gatewayapi_v1alpha2.ListenerConditionDetached),
			Status:  metav1.ConditionTrue,
			Reason:  string(gatewayapi_v1alpha2.ListenerReasonPortUnavailable),
			Message: "Only one HTTP port is supported",
		},
	}, res.InvalidListenerConditions)

	// Two HTTP listeners with the same hostname, some HTTP
	// listeners with invalid port, some non-HTTP listeners as well.
	listeners = []gatewayapi_v1alpha2.Listener{
		{
			Name:     "listener-1",
			Protocol: gatewayapi_v1alpha2.HTTPProtocolType,
			Port:     80,
		},
		{
			Name:     "listener-2",
			Protocol: gatewayapi_v1alpha2.HTTPProtocolType,
			Port:     80,
			Hostname: ListenerHostname("local.projectcontour.io"), // duplicate hostname
		},
		{
			Name:     "listener-3",
			Protocol: gatewayapi_v1alpha2.HTTPProtocolType,
			Port:     80,
			Hostname: ListenerHostname("local.projectcontour.io"), // duplicate hostname
		},
		{
			Name:     "listener-4",
			Protocol: gatewayapi_v1alpha2.HTTPProtocolType,
			Port:     80,
			Hostname: ListenerHostname("local.envoyproxy.io"),
		},
		{
			Name:     "listener-5",
			Protocol: gatewayapi_v1alpha2.HTTPProtocolType,
			Port:     8080, // invalid port
			Hostname: ListenerHostname("local.envoyproxy.io"),
		},
		{
			Name:     "non-http-listener-1",
			Protocol: gatewayapi_v1alpha2.TLSProtocolType, // non-HTTP
			Port:     443,
			Hostname: ListenerHostname("local.projectcontour.io"),
		},
	}

	res = ValidateListeners(listeners)
	assert.Equal(t, 80, res.InsecurePort)
	assert.Equal(t, map[gatewayapi_v1alpha2.SectionName]metav1.Condition{
		"listener-2": {
			Type:    string(gatewayapi_v1alpha2.ListenerConditionConflicted),
			Status:  metav1.ConditionTrue,
			Reason:  string(gatewayapi_v1alpha2.ListenerReasonHostnameConflict),
			Message: "Hostname must be unique among HTTP listeners",
		},
		"listener-3": {
			Type:    string(gatewayapi_v1alpha2.ListenerConditionConflicted),
			Status:  metav1.ConditionTrue,
			Reason:  string(gatewayapi_v1alpha2.ListenerReasonHostnameConflict),
			Message: "Hostname must be unique among HTTP listeners",
		},
		"listener-5": {
			Type:    string(gatewayapi_v1alpha2.ListenerConditionDetached),
			Status:  metav1.ConditionTrue,
			Reason:  string(gatewayapi_v1alpha2.ListenerReasonPortUnavailable),
			Message: "Only one HTTP port is supported",
		},
	}, res.InvalidListenerConditions)

	// All HTTPS/TLS listeners are valid, some non-HTTPS/TLS listeners
	// as well.
	listeners = []gatewayapi_v1alpha2.Listener{
		{
			Name:     "listener-1",
			Protocol: gatewayapi_v1alpha2.HTTPSProtocolType,
			Port:     443,
		},
		{
			Name:     "listener-2",
			Protocol: gatewayapi_v1alpha2.TLSProtocolType,
			Port:     443,
			Hostname: ListenerHostname("local.projectcontour.io"),
		},
		{
			Name:     "listener-3",
			Protocol: gatewayapi_v1alpha2.HTTPSProtocolType,
			Port:     443,
			Hostname: ListenerHostname("*.projectcontour.io"),
		},
		{
			Name:     "listener-4",
			Protocol: gatewayapi_v1alpha2.TLSProtocolType,
			Port:     443,
			Hostname: ListenerHostname("local.envoyproxy.io"),
		},
		{
			Name:     "non-http-listener-1",
			Protocol: gatewayapi_v1alpha2.HTTPProtocolType,
			Port:     80,
			Hostname: ListenerHostname("local.projectcontour.io"),
		},
	}

	res = ValidateListeners(listeners)
	assert.Equal(t, 443, res.SecurePort)
	assert.Empty(t, res.InvalidListenerConditions)

	// One HTTPS listener with an invalid port number, some
	// non-HTTPS listeners as well.
	listeners = []gatewayapi_v1alpha2.Listener{
		{
			Name:     "listener-1",
			Protocol: gatewayapi_v1alpha2.HTTPSProtocolType,
			Port:     443,
		},
		{
			Name:     "listener-2",
			Protocol: gatewayapi_v1alpha2.TLSProtocolType,
			Port:     443,
			Hostname: ListenerHostname("local.projectcontour.io"),
		},
		{
			Name:     "listener-3",
			Protocol: gatewayapi_v1alpha2.HTTPSProtocolType,
			Port:     443,
			Hostname: ListenerHostname("*.projectcontour.io"),
		},
		{
			Name:     "listener-4",
			Protocol: gatewayapi_v1alpha2.HTTPSProtocolType,
			Port:     8443,
			Hostname: ListenerHostname("local.projectcontour.io"),
		},
		{
			Name:     "http-listener-1",
			Protocol: gatewayapi_v1alpha2.HTTPProtocolType,
			Port:     80,
			Hostname: ListenerHostname("local.projectcontour.io"),
		},
	}

	res = ValidateListeners(listeners)
	assert.Equal(t, 443, res.SecurePort)
	assert.Equal(t, map[gatewayapi_v1alpha2.SectionName]metav1.Condition{
		"listener-4": {
			Type:    string(gatewayapi_v1alpha2.ListenerConditionDetached),
			Status:  metav1.ConditionTrue,
			Reason:  string(gatewayapi_v1alpha2.ListenerReasonPortUnavailable),
			Message: "Only one HTTPS/TLS port is supported",
		},
	}, res.InvalidListenerConditions)

	// Two HTTPS/TLS listeners with the same hostname, some HTTPS/TLS
	// listeners with invalid port, some HTTP listeners as well.
	listeners = []gatewayapi_v1alpha2.Listener{
		{
			Name:     "listener-1",
			Protocol: gatewayapi_v1alpha2.HTTPSProtocolType,
			Port:     443,
		},
		{
			Name:     "listener-2",
			Protocol: gatewayapi_v1alpha2.HTTPSProtocolType,
			Port:     443,
			Hostname: ListenerHostname("local.projectcontour.io"), // duplicate hostname
		},
		{
			Name:     "listener-3",
			Protocol: gatewayapi_v1alpha2.TLSProtocolType,
			Port:     443,
			Hostname: ListenerHostname("local.projectcontour.io"), // duplicate hostname
		},
		{
			Name:     "listener-4",
			Protocol: gatewayapi_v1alpha2.HTTPSProtocolType,
			Port:     443,
			Hostname: ListenerHostname("local.envoyproxy.io"),
		},
		{
			Name:     "listener-5",
			Protocol: gatewayapi_v1alpha2.HTTPSProtocolType,
			Port:     8443, // invalid port
			Hostname: ListenerHostname("local.envoyproxy.io"),
		},
		{
			Name:     "http-listener-1",
			Protocol: gatewayapi_v1alpha2.HTTPProtocolType,
			Port:     80,
			Hostname: ListenerHostname("local.projectcontour.io"),
		},
	}

	res = ValidateListeners(listeners)
	assert.Equal(t, 443, res.SecurePort)
	assert.Equal(t, map[gatewayapi_v1alpha2.SectionName]metav1.Condition{
		"listener-2": {
			Type:    string(gatewayapi_v1alpha2.ListenerConditionConflicted),
			Status:  metav1.ConditionTrue,
			Reason:  string(gatewayapi_v1alpha2.ListenerReasonHostnameConflict),
			Message: "Hostname must be unique among HTTPS/TLS listeners",
		},
		"listener-3": {
			Type:    string(gatewayapi_v1alpha2.ListenerConditionConflicted),
			Status:  metav1.ConditionTrue,
			Reason:  string(gatewayapi_v1alpha2.ListenerReasonHostnameConflict),
			Message: "Hostname must be unique among HTTPS/TLS listeners",
		},
		"listener-5": {
			Type:    string(gatewayapi_v1alpha2.ListenerConditionDetached),
			Status:  metav1.ConditionTrue,
			Reason:  string(gatewayapi_v1alpha2.ListenerReasonPortUnavailable),
			Message: "Only one HTTPS/TLS port is supported",
		},
	}, res.InvalidListenerConditions)

	// Two HTTP and one HTTPS listeners, each with an invalid hostname.
	listeners = []gatewayapi_v1alpha2.Listener{
		{
			Name:     "listener-1",
			Protocol: gatewayapi_v1alpha2.HTTPProtocolType,
			Port:     80,
			Hostname: ListenerHostname("192.168.1.1"),
		},
		{
			Name:     "listener-2",
			Protocol: gatewayapi_v1alpha2.HTTPProtocolType,
			Port:     80,
			Hostname: ListenerHostname("*.*.projectcontour.io"),
		},
		{
			Name:     "listener-3",
			Protocol: gatewayapi_v1alpha2.HTTPSProtocolType,
			Port:     443,
			Hostname: ListenerHostname(".invalid.$."),
		},
	}

	res = ValidateListeners(listeners)
	assert.Equal(t, map[gatewayapi_v1alpha2.SectionName]metav1.Condition{
		"listener-1": {
			Type:    string(gatewayapi_v1alpha2.ListenerConditionReady),
			Status:  metav1.ConditionFalse,
			Reason:  string(gatewayapi_v1alpha2.ListenerReasonInvalid),
			Message: "invalid hostname \"192.168.1.1\": must be a DNS name, not an IP address",
		},
		"listener-2": {
			Type:    string(gatewayapi_v1alpha2.ListenerConditionReady),
			Status:  metav1.ConditionFalse,
			Reason:  string(gatewayapi_v1alpha2.ListenerReasonInvalid),
			Message: "invalid hostname \"*.*.projectcontour.io\": [a wildcard DNS-1123 subdomain must start with '*.', followed by a valid DNS subdomain, which must consist of lower case alphanumeric characters, '-' or '.' and end with an alphanumeric character (e.g. '*.example.com', regex used for validation is '\\*\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*')]",
		},
		"listener-3": {
			Type:    string(gatewayapi_v1alpha2.ListenerConditionReady),
			Status:  metav1.ConditionFalse,
			Reason:  string(gatewayapi_v1alpha2.ListenerReasonInvalid),
			Message: "invalid hostname \".invalid.$.\": [a lowercase RFC 1123 subdomain must consist of lower case alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character (e.g. 'example.com', regex used for validation is '[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*')]",
		},
	}, res.InvalidListenerConditions)
}
