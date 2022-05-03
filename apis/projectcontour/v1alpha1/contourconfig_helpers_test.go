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

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestContourConfigurationSpecValidate(t *testing.T) {
	t.Run("xds server type validation", func(t *testing.T) {
		c := ContourConfigurationSpec{
			XDSServer: &XDSServerConfig{},
		}

		c.XDSServer.Type = ContourServerType
		require.NoError(t, c.Validate())

		c.XDSServer.Type = EnvoyServerType
		require.NoError(t, c.Validate())

		c.XDSServer.Type = "foo"
		require.Error(t, c.Validate())
	})

	t.Run("debug log level validation", func(t *testing.T) {
		c := ContourConfigurationSpec{
			Debug: &DebugConfig{},
		}

		c.Debug.DebugLogLevel = InfoLog
		require.NoError(t, c.Validate())

		c.Debug.DebugLogLevel = DebugLog
		require.NoError(t, c.Validate())

		c.Debug.DebugLogLevel = "foo"
		require.Error(t, c.Validate())
	})

	t.Run("envoy validation", func(t *testing.T) {
		c := ContourConfigurationSpec{
			Envoy: &EnvoyConfig{
				Metrics: &MetricsConfig{
					Address: "0.0.0.0",
					Port:    8080,
				},
				Health: &HealthConfig{
					Address: "0.0.0.0",
					Port:    8080,
				},
			},
		}
		require.NoError(t, c.Validate())

		c = ContourConfigurationSpec{
			Envoy: &EnvoyConfig{
				Metrics: &MetricsConfig{
					TLS:     &MetricsTLS{},
					Address: "0.0.0.0",
					Port:    8080,
				},
				Health: &HealthConfig{
					Address: "0.0.0.0",
					Port:    8080,
				},
			},
		}
		require.Error(t, c.Validate())

		c = ContourConfigurationSpec{
			Envoy: &EnvoyConfig{
				DefaultHTTPVersions: []HTTPVersionType{HTTPVersion1, HTTPVersion2},
			},
		}
		require.NoError(t, c.Validate())

		c = ContourConfigurationSpec{
			Envoy: &EnvoyConfig{
				DefaultHTTPVersions: []HTTPVersionType{HTTPVersion1, HTTPVersion2, "foo"},
			},
		}
		require.Error(t, c.Validate())

		c = ContourConfigurationSpec{
			Envoy: &EnvoyConfig{
				Cluster: &ClusterParameters{},
			},
		}

		c.Envoy.Cluster.DNSLookupFamily = AutoClusterDNSFamily
		require.NoError(t, c.Validate())

		c.Envoy.Cluster.DNSLookupFamily = IPv4ClusterDNSFamily
		require.NoError(t, c.Validate())

		c.Envoy.Cluster.DNSLookupFamily = IPv6ClusterDNSFamily
		require.NoError(t, c.Validate())

		c.Envoy.Cluster.DNSLookupFamily = "foo"
		require.Error(t, c.Validate())

		c = ContourConfigurationSpec{
			Envoy: &EnvoyConfig{
				Listener: &EnvoyListenerConfig{
					TLS: &EnvoyTLS{},
				},
			},
		}

		c.Envoy.Listener.TLS.CipherSuites = []TLSCipherType{"ECDHE-ECDSA-AES128-GCM-SHA256"}
		require.NoError(t, c.Validate())

		c.Envoy.Listener.TLS.CipherSuites = []TLSCipherType{"foo"}
		require.Error(t, c.Validate())
	})

	t.Run("gateway validation", func(t *testing.T) {
		c := ContourConfigurationSpec{
			Gateway: &GatewayConfig{},
		}

		c.Gateway.ControllerName = "foo"
		require.NoError(t, c.Validate())

		c.Gateway.ControllerName = ""
		c.Gateway.GatewayRef = &NamespacedName{Namespace: "ns", Name: "name"}
		require.NoError(t, c.Validate())

		c.Gateway.ControllerName = "foo"
		c.Gateway.GatewayRef = &NamespacedName{Namespace: "ns", Name: "name"}
		require.Error(t, c.Validate())
	})
}
