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
	"github.com/projectcontour/contour/internal/ref"
	"github.com/stretchr/testify/assert"
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

		c.Envoy.Cluster.DNSLookupFamily = v1alpha1.AllClusterDNSFamily
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
		require.NoError(t, c.Validate())

		c.Envoy.Listener.TLS.MinimumProtocolVersion = "invalid"
		require.Error(t, c.Validate())

		c.Envoy.Listener.TLS.MinimumProtocolVersion = "1.3"
		require.NoError(t, c.Validate())

		c.Envoy.Listener.TLS.CipherSuites = []string{
			"[ECDHE-ECDSA-AES128-GCM-SHA256|ECDHE-ECDSA-CHACHA20-POLY1305]",
			"ECDHE-ECDSA-AES128-GCM-SHA256",
			"[ECDHE-RSA-AES128-GCM-SHA256|ECDHE-RSA-CHACHA20-POLY1305]",
			"ECDHE-RSA-AES128-GCM-SHA256",
			"ECDHE-ECDSA-AES128-SHA",
			"AES128-GCM-SHA256",
			"AES128-SHA",
			"ECDHE-ECDSA-AES256-GCM-SHA384",
			"ECDHE-RSA-AES256-GCM-SHA384",
			"ECDHE-ECDSA-AES256-SHA",
			"ECDHE-RSA-AES256-SHA",
			"AES256-GCM-SHA384",
			"AES256-SHA",
		}
		require.NoError(t, c.Validate())

		c.Envoy.Listener.TLS.CipherSuites = []string{
			"[ECDHE-RSA-AES128-GCM-SHA256|ECDHE-RSA-CHACHA20-POLY1305]",
			"NOTAVALIDCIPHER",
			"AES128-GCM-SHA256",
		}
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

	t.Run("tracing validation", func(t *testing.T) {
		c := v1alpha1.ContourConfigurationSpec{
			Tracing: &v1alpha1.TracingConfig{},
		}

		require.Error(t, c.Validate())

		c.Tracing.ExtensionService = &v1alpha1.NamespacedName{
			Name:      "otel-collector",
			Namespace: "projectcontour",
		}
		require.NoError(t, c.Validate())

		c.Tracing.OverallSampling = ref.To("number")
		require.Error(t, c.Validate())

		c.Tracing.OverallSampling = ref.To("10")
		require.NoError(t, c.Validate())

		customTags := []*v1alpha1.CustomTag{
			{
				TagName: "first tag",
				Literal: "literal",
			},
		}
		c.Tracing.CustomTags = customTags
		require.NoError(t, c.Validate())

		customTags = append(customTags, &v1alpha1.CustomTag{
			TagName:           "second tag",
			RequestHeaderName: "x-custom-header",
		})
		c.Tracing.CustomTags = customTags
		require.NoError(t, c.Validate())

		customTags = append(customTags, &v1alpha1.CustomTag{
			TagName:           "first tag",
			RequestHeaderName: "x-custom-header",
		})
		c.Tracing.CustomTags = customTags
		require.Error(t, c.Validate())

		customTags = []*v1alpha1.CustomTag{
			{
				TagName:           "first tag",
				Literal:           "literal",
				RequestHeaderName: "x-custom-header",
			},
		}
		c.Tracing.CustomTags = customTags
		require.Error(t, c.Validate())

	})
}

func TestSanitizeCipherSuites(t *testing.T) {
	testCases := map[string]struct {
		ciphers []string
		want    []string
	}{
		"no ciphers": {
			ciphers: nil,
			want:    v1alpha1.DefaultTLSCiphers,
		},
		"valid list": {
			ciphers: []string{
				"[ECDHE-RSA-AES128-GCM-SHA256|ECDHE-RSA-CHACHA20-POLY1305]",
				"ECDHE-RSA-AES128-SHA",
				"AES128-SHA",
			},
			want: []string{
				"[ECDHE-RSA-AES128-GCM-SHA256|ECDHE-RSA-CHACHA20-POLY1305]",
				"ECDHE-RSA-AES128-SHA",
				"AES128-SHA",
			},
		},
		"cipher duplicated": {
			ciphers: []string{
				"ECDHE-RSA-AES128-SHA",
				"ECDHE-RSA-AES128-SHA",
			},
			want: []string{
				"ECDHE-RSA-AES128-SHA",
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			e := &v1alpha1.EnvoyTLS{
				CipherSuites: tc.ciphers,
			}
			assert.Equal(t, tc.want, e.SanitizedCipherSuites())
		})
	}
}

// TestAccessLogFormatExtensions tests that command operators requiring extensions are recognized for given access log format.
func TestAccessLogFormatExtensions(t *testing.T) {
	e1 := v1alpha1.EnvoyLogging{
		AccessLogFormat:       v1alpha1.EnvoyAccessLog,
		AccessLogFormatString: "[%START_TIME%] \"%REQ_WITHOUT_QUERY(X-ENVOY-ORIGINAL-PATH?:PATH)%\"\n",
	}
	assert.Equal(t, []string{"envoy.formatter.req_without_query"}, e1.AccessLogFormatterExtensions())

	e2 := v1alpha1.EnvoyLogging{
		AccessLogFormat:     v1alpha1.JSONAccessLog,
		AccessLogJSONFields: []string{"@timestamp", "path=%REQ_WITHOUT_QUERY(X-ENVOY-ORIGINAL-PATH?:PATH)%"},
	}
	assert.Equal(t, []string{"envoy.formatter.req_without_query"}, e2.AccessLogFormatterExtensions())

	e3 := v1alpha1.EnvoyLogging{
		AccessLogFormat: v1alpha1.EnvoyAccessLog,
	}
	assert.Empty(t, e3.AccessLogFormatterExtensions())
}
