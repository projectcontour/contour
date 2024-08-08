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
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"

	contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
)

func TestContourConfigurationSpecValidate(t *testing.T) {
	t.Run("envoy validation", func(t *testing.T) {
		c := contour_v1alpha1.ContourConfigurationSpec{
			Envoy: &contour_v1alpha1.EnvoyConfig{
				Metrics: &contour_v1alpha1.MetricsConfig{
					Address: "0.0.0.0",
					Port:    8080,
				},
				Health: &contour_v1alpha1.HealthConfig{
					Address: "0.0.0.0",
					Port:    8080,
				},
			},
		}
		require.NoError(t, c.Validate())

		c = contour_v1alpha1.ContourConfigurationSpec{
			Envoy: &contour_v1alpha1.EnvoyConfig{
				Metrics: &contour_v1alpha1.MetricsConfig{
					TLS:     &contour_v1alpha1.MetricsTLS{},
					Address: "0.0.0.0",
					Port:    8080,
				},
				Health: &contour_v1alpha1.HealthConfig{
					Address: "0.0.0.0",
					Port:    8080,
				},
			},
		}
		require.Error(t, c.Validate())

		c = contour_v1alpha1.ContourConfigurationSpec{
			Envoy: &contour_v1alpha1.EnvoyConfig{
				DefaultHTTPVersions: []contour_v1alpha1.HTTPVersionType{contour_v1alpha1.HTTPVersion1, contour_v1alpha1.HTTPVersion2},
			},
		}
		require.NoError(t, c.Validate())

		c = contour_v1alpha1.ContourConfigurationSpec{
			Envoy: &contour_v1alpha1.EnvoyConfig{
				DefaultHTTPVersions: []contour_v1alpha1.HTTPVersionType{contour_v1alpha1.HTTPVersion1, contour_v1alpha1.HTTPVersion2, "foo"},
			},
		}
		require.Error(t, c.Validate())

		c = contour_v1alpha1.ContourConfigurationSpec{
			Envoy: &contour_v1alpha1.EnvoyConfig{
				Cluster: &contour_v1alpha1.ClusterParameters{},
			},
		}

		c.Envoy.Cluster.DNSLookupFamily = contour_v1alpha1.AutoClusterDNSFamily
		require.NoError(t, c.Validate())

		c.Envoy.Cluster.DNSLookupFamily = contour_v1alpha1.IPv4ClusterDNSFamily
		require.NoError(t, c.Validate())

		c.Envoy.Cluster.DNSLookupFamily = contour_v1alpha1.IPv6ClusterDNSFamily
		require.NoError(t, c.Validate())

		c.Envoy.Cluster.DNSLookupFamily = contour_v1alpha1.AllClusterDNSFamily
		require.NoError(t, c.Validate())

		c.Envoy.Cluster.DNSLookupFamily = "foo"
		require.Error(t, c.Validate())

		c = contour_v1alpha1.ContourConfigurationSpec{
			Envoy: &contour_v1alpha1.EnvoyConfig{
				Listener: &contour_v1alpha1.EnvoyListenerConfig{
					TLS: &contour_v1alpha1.EnvoyTLS{},
				},
			},
		}
		require.NoError(t, c.Validate())

		c.Envoy.Listener.TLS.MinimumProtocolVersion = "invalid"
		c.Envoy.Listener.TLS.MaximumProtocolVersion = ""
		require.Error(t, c.Validate())

		c.Envoy.Listener.TLS.MinimumProtocolVersion = "1.3"
		c.Envoy.Listener.TLS.MaximumProtocolVersion = ""
		require.NoError(t, c.Validate())

		c.Envoy.Listener.TLS.MinimumProtocolVersion = ""
		c.Envoy.Listener.TLS.MaximumProtocolVersion = "invalid"
		require.Error(t, c.Validate())

		c.Envoy.Listener.TLS.MinimumProtocolVersion = ""
		c.Envoy.Listener.TLS.MaximumProtocolVersion = "1.3"
		require.NoError(t, c.Validate())

		c.Envoy.Listener.TLS.MinimumProtocolVersion = "1.3"
		c.Envoy.Listener.TLS.MaximumProtocolVersion = "1.2"
		require.Error(t, c.Validate())

		c.Envoy.Listener.TLS.MinimumProtocolVersion = "1.2"
		c.Envoy.Listener.TLS.MaximumProtocolVersion = "1.3"
		require.NoError(t, c.Validate())

		c.Envoy.Listener.TLS.CipherSuites = []string{
			"[ECDHE-ECDSA-AES128-GCM-SHA256|ECDHE-ECDSA-CHACHA20-POLY1305]",
			"ECDHE-ECDSA-AES128-GCM-SHA256",
			"ECDHE-ECDSA-CHACHA20-POLY1305",
			"[ECDHE-RSA-AES128-GCM-SHA256|ECDHE-RSA-CHACHA20-POLY1305]",
			"ECDHE-RSA-AES128-GCM-SHA256",
			"ECDHE-RSA-CHACHA20-POLY1305",
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

		// Equal-preference group with invalid cipher.
		c.Envoy.Listener.TLS.CipherSuites = []string{
			"[ECDHE-ECDSA-AES128-GCM-SHA256|NOTAVALIDCIPHER]",
			"ECDHE-ECDSA-AES128-GCM-SHA256",
		}
		require.Error(t, c.Validate())

		// Unmatched brackets.
		c.Envoy.Listener.TLS.CipherSuites = []string{
			"[ECDHE-ECDSA-AES128-GCM-SHA256|ECDHE-ECDSA-CHACHA20-POLY1305",
			"ECDHE-ECDSA-AES128-GCM-SHA256",
		}
		require.Error(t, c.Validate())
	})

	t.Run("gateway validation", func(t *testing.T) {
		c := contour_v1alpha1.ContourConfigurationSpec{
			Gateway: &contour_v1alpha1.GatewayConfig{},
		}

		c.Gateway.GatewayRef = contour_v1alpha1.NamespacedName{Namespace: "ns", Name: "name"}
		require.NoError(t, c.Validate())

		// empty namespace is not allowed
		c.Gateway.GatewayRef = contour_v1alpha1.NamespacedName{Name: "name"}
		require.Error(t, c.Validate())

		// empty name is not allowed
		c.Gateway.GatewayRef = contour_v1alpha1.NamespacedName{Namespace: "ns"}
		require.Error(t, c.Validate())
	})

	t.Run("tracing validation", func(t *testing.T) {
		c := contour_v1alpha1.ContourConfigurationSpec{
			Tracing: &contour_v1alpha1.TracingConfig{},
		}

		require.Error(t, c.Validate())

		c.Tracing.ExtensionService = &contour_v1alpha1.NamespacedName{
			Name:      "otel-collector",
			Namespace: "projectcontour",
		}
		require.NoError(t, c.Validate())

		c.Tracing.OverallSampling = ptr.To("number")
		require.Error(t, c.Validate())

		c.Tracing.OverallSampling = ptr.To("10")
		require.NoError(t, c.Validate())

		customTags := []*contour_v1alpha1.CustomTag{
			{
				TagName: "first tag",
				Literal: "literal",
			},
		}
		c.Tracing.CustomTags = customTags
		require.NoError(t, c.Validate())

		customTags = append(customTags, &contour_v1alpha1.CustomTag{
			TagName:           "second tag",
			RequestHeaderName: "x-custom-header",
		})
		c.Tracing.CustomTags = customTags
		require.NoError(t, c.Validate())

		customTags = append(customTags, &contour_v1alpha1.CustomTag{
			TagName:           "first tag",
			RequestHeaderName: "x-custom-header",
		})
		c.Tracing.CustomTags = customTags
		require.Error(t, c.Validate())

		customTags = []*contour_v1alpha1.CustomTag{
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
			want:    contour_v1alpha1.DefaultTLSCiphers,
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
			e := &contour_v1alpha1.EnvoyTLS{
				CipherSuites: tc.ciphers,
			}
			assert.Equal(t, tc.want, e.SanitizedCipherSuites())
		})
	}
}

// TestAccessLogFormatExtensions tests that command operators requiring extensions are recognized for given access log format.
func TestAccessLogFormatExtensions(t *testing.T) {
	e1 := contour_v1alpha1.EnvoyLogging{
		AccessLogFormat:       contour_v1alpha1.EnvoyAccessLog,
		AccessLogFormatString: "[%START_TIME%] \"%REQ_WITHOUT_QUERY(X-ENVOY-ORIGINAL-PATH?:PATH)%\"\n",
	}
	assert.Equal(t, []string{"envoy.formatter.req_without_query"}, e1.AccessLogFormatterExtensions())

	e2 := contour_v1alpha1.EnvoyLogging{
		AccessLogFormat:     contour_v1alpha1.JSONAccessLog,
		AccessLogJSONFields: []string{"@timestamp", "path=%REQ_WITHOUT_QUERY(X-ENVOY-ORIGINAL-PATH?:PATH)%"},
	}
	assert.Equal(t, []string{"envoy.formatter.req_without_query"}, e2.AccessLogFormatterExtensions())

	e3 := contour_v1alpha1.EnvoyLogging{
		AccessLogFormat: contour_v1alpha1.EnvoyAccessLog,
	}
	assert.Empty(t, e3.AccessLogFormatterExtensions())
}

func TestFeatureFlagsValidate(t *testing.T) {
	tests := []struct {
		name     string
		flags    contour_v1alpha1.FeatureFlags
		expected error
	}{
		{
			name:     "valid flag: no value",
			flags:    contour_v1alpha1.FeatureFlags{"useEndpointSlices"},
			expected: nil,
		},
		{
			name:     "valid flag2: empty",
			flags:    contour_v1alpha1.FeatureFlags{"useEndpointSlices="},
			expected: nil,
		},
		{
			name:     "valid flag: true",
			flags:    contour_v1alpha1.FeatureFlags{"useEndpointSlices=true"},
			expected: nil,
		},
		{
			name:     "valid flag: false",
			flags:    contour_v1alpha1.FeatureFlags{"useEndpointSlices=false"},
			expected: nil,
		},

		{
			name:     "invalid flag",
			flags:    contour_v1alpha1.FeatureFlags{"invalidFlag"},
			expected: fmt.Errorf("invalid contour configuration, unknown feature flag:invalidFlag"),
		},
		{
			name:     "mix of valid and invalid flags",
			flags:    contour_v1alpha1.FeatureFlags{"useEndpointSlices", "invalidFlag"},
			expected: fmt.Errorf("invalid contour configuration, unknown feature flag:invalidFlag"),
		},
		{
			name:     "empty flags",
			flags:    contour_v1alpha1.FeatureFlags{},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.flags.Validate()
			assert.Equal(t, tt.expected, err)
		})
	}
}

func TestFeatureFlagsIsEndpointSliceEnabled(t *testing.T) {
	tests := []struct {
		name     string
		flags    contour_v1alpha1.FeatureFlags
		expected bool
	}{
		{
			name:     "valid flag: no value",
			flags:    contour_v1alpha1.FeatureFlags{"useEndpointSlices"},
			expected: true,
		},
		{
			name:     "valid flag2: empty",
			flags:    contour_v1alpha1.FeatureFlags{"useEndpointSlices="},
			expected: true,
		},
		{
			name:     "valid flag: true",
			flags:    contour_v1alpha1.FeatureFlags{"useEndpointSlices=true"},
			expected: true,
		},
		{
			name:     "valid flag: ANY",
			flags:    contour_v1alpha1.FeatureFlags{"useEndpointSlices=ANY"},
			expected: true,
		},

		{
			name:     "empty flags",
			flags:    contour_v1alpha1.FeatureFlags{},
			expected: true,
		},
		{
			name:     "empty string",
			flags:    contour_v1alpha1.FeatureFlags{""},
			expected: true,
		},

		{
			name:     "multi-flags",
			flags:    contour_v1alpha1.FeatureFlags{"useEndpointSlices", "otherFlag"},
			expected: true,
		},

		{
			name:     "valid flag: false",
			flags:    contour_v1alpha1.FeatureFlags{"useEndpointSlices=false"},
			expected: false,
		},

		{
			name:     "valid flag: FALSE",
			flags:    contour_v1alpha1.FeatureFlags{"useEndpointSlices=FALSE"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.flags.IsEndpointSliceEnabled()
			assert.Equal(t, tt.expected, err)
		})
	}
}
