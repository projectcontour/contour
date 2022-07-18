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

package v1alpha1_test

import (
	"testing"

	"github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/stretchr/testify/require"
)

func TestContourConfigurationSpecValidate(t *testing.T) {
	t.Run("xds server type validation", func(t *testing.T) {
		c := v1alpha1.ContourConfigurationSpec{
			XDSServer: &v1alpha1.XDSServerConfig{},
		}

		c.XDSServer.Type = v1alpha1.ContourServerType
		require.NoError(t, c.Validate())

		c.XDSServer.Type = v1alpha1.EnvoyServerType
		require.NoError(t, c.Validate())

		c.XDSServer.Type = "foo"
		require.Error(t, c.Validate())
	})

	t.Run("envoy validation", func(t *testing.T) {
		c := v1alpha1.ContourConfigurationSpec{
			Envoy: &v1alpha1.EnvoyConfig{
				Metrics: &v1alpha1.MetricsConfig{
					Address: "0.0.0.0",
					Port:    8080,
				},
				Health: &v1alpha1.HealthConfig{
					Address: "0.0.0.0",
					Port:    8080,
				},
			},
		}
		require.NoError(t, c.Validate())

		c = v1alpha1.ContourConfigurationSpec{
			Envoy: &v1alpha1.EnvoyConfig{
				Metrics: &v1alpha1.MetricsConfig{
					TLS:     &v1alpha1.MetricsTLS{},
					Address: "0.0.0.0",
					Port:    8080,
				},
				Health: &v1alpha1.HealthConfig{
					Address: "0.0.0.0",
					Port:    8080,
				},
			},
		}
		require.Error(t, c.Validate())

		c = v1alpha1.ContourConfigurationSpec{
			Envoy: &v1alpha1.EnvoyConfig{
				DefaultHTTPVersions: []v1alpha1.HTTPVersionType{v1alpha1.HTTPVersion1, v1alpha1.HTTPVersion2},
			},
		}
		require.NoError(t, c.Validate())

		c = v1alpha1.ContourConfigurationSpec{
			Envoy: &v1alpha1.EnvoyConfig{
				DefaultHTTPVersions: []v1alpha1.HTTPVersionType{v1alpha1.HTTPVersion1, v1alpha1.HTTPVersion2, "foo"},
			},
		}
		require.Error(t, c.Validate())

		c = v1alpha1.ContourConfigurationSpec{
			Envoy: &v1alpha1.EnvoyConfig{
				Cluster: &v1alpha1.ClusterParameters{},
			},
		}

		c.Envoy.Cluster.DNSLookupFamily = v1alpha1.AutoClusterDNSFamily
		require.NoError(t, c.Validate())

		c.Envoy.Cluster.DNSLookupFamily = v1alpha1.IPv4ClusterDNSFamily
		require.NoError(t, c.Validate())

		c.Envoy.Cluster.DNSLookupFamily = v1alpha1.IPv6ClusterDNSFamily
		require.NoError(t, c.Validate())

		c.Envoy.Cluster.DNSLookupFamily = "foo"
		require.Error(t, c.Validate())

		c = v1alpha1.ContourConfigurationSpec{
			Envoy: &v1alpha1.EnvoyConfig{
				Listener: &v1alpha1.EnvoyListenerConfig{
					TLS: &v1alpha1.EnvoyTLS{},
				},
			},
		}

		c.Envoy.Listener.TLS.CipherSuites = []v1alpha1.TLSCipherType{"ECDHE-ECDSA-AES128-GCM-SHA256"}
		require.NoError(t, c.Validate())

		c.Envoy.Listener.TLS.CipherSuites = []v1alpha1.TLSCipherType{"foo"}
		require.Error(t, c.Validate())
	})

	t.Run("gateway validation", func(t *testing.T) {
		c := v1alpha1.ContourConfigurationSpec{
			Gateway: &v1alpha1.GatewayConfig{},
		}

		c.Gateway.ControllerName = "foo"
		require.NoError(t, c.Validate())

		c.Gateway.ControllerName = ""
		c.Gateway.GatewayRef = &v1alpha1.NamespacedName{Namespace: "ns", Name: "name"}
		require.NoError(t, c.Validate())

		c.Gateway.ControllerName = "foo"
		c.Gateway.GatewayRef = &v1alpha1.NamespacedName{Namespace: "ns", Name: "name"}
		require.Error(t, c.Validate())
	})
}
