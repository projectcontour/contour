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

package contourconfig_test

import (
	"testing"
	"time"

	contour_api_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/contourconfig"
	"github.com/projectcontour/contour/internal/timeout"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/utils/pointer"
)

func TestOverlayOnDefaults(t *testing.T) {
	allFieldsSpecified := contour_api_v1alpha1.ContourConfigurationSpec{
		XDSServer: &contour_api_v1alpha1.XDSServerConfig{
			Type:    contour_api_v1alpha1.EnvoyServerType,
			Address: "7.7.7.7",
			Port:    7777,
			TLS: &contour_api_v1alpha1.TLS{
				CAFile:   "/foo/ca.crt",
				CertFile: "/foo/tls.crt",
				KeyFile:  "/foo/tls.key",
				Insecure: pointer.Bool(true),
			},
		},
		Ingress: &contour_api_v1alpha1.IngressConfig{
			ClassNames:    []string{"coolclass"},
			StatusAddress: "7.7.7.7",
		},
		Debug: &contour_api_v1alpha1.DebugConfig{
			Address: "1.2.3.4",
			Port:    6789,
		},
		Health: &contour_api_v1alpha1.HealthConfig{
			Address: "2.3.4.5",
			Port:    8888,
		},
		Envoy: &contour_api_v1alpha1.EnvoyConfig{
			Listener: &contour_api_v1alpha1.EnvoyListenerConfig{
				UseProxyProto:             pointer.Bool(true),
				DisableAllowChunkedLength: pointer.Bool(true),
				DisableMergeSlashes:       pointer.Bool(true),
				ConnectionBalancer:        "yesplease",
				TLS: &contour_api_v1alpha1.EnvoyTLS{
					MinimumProtocolVersion: "1.7",
					CipherSuites: []contour_api_v1alpha1.TLSCipherType{
						"foo",
						"bar",
					},
				},
			},
			Service: &contour_api_v1alpha1.NamespacedName{
				Namespace: "coolnamespace",
				Name:      "coolname",
			},
			HTTPListener: &contour_api_v1alpha1.EnvoyListener{
				Address:   "3.4.5.6",
				Port:      8989,
				AccessLog: "/dev/oops",
			},
			HTTPSListener: &contour_api_v1alpha1.EnvoyListener{
				Address:   "4.5.6.7",
				Port:      8445,
				AccessLog: "/dev/oops",
			},
			Health: &contour_api_v1alpha1.HealthConfig{
				Address: "1.1.1.1",
				Port:    8222,
			},
			Metrics: &contour_api_v1alpha1.MetricsConfig{
				Address: "1.2.12.1212",
				Port:    8882,
				TLS: &contour_api_v1alpha1.MetricsTLS{
					CAFile:   "cafile",
					CertFile: "certfile",
					KeyFile:  "keyfile",
				},
			},
			ClientCertificate: &contour_api_v1alpha1.NamespacedName{
				Namespace: "clientcertnamespace",
				Name:      "clientcertname",
			},
			Logging: &contour_api_v1alpha1.EnvoyLogging{
				AccessLogFormat:       contour_api_v1alpha1.JSONAccessLog,
				AccessLogFormatString: "foo",
				AccessLogFields:       []string{"field-1", "field-2"},
				AccessLogLevel:        contour_api_v1alpha1.LogLevelError,
			},
			DefaultHTTPVersions: []contour_api_v1alpha1.HTTPVersionType{
				"HTTP/2.2",
				"HTTP/3",
			},
			Timeouts: &contour_api_v1alpha1.TimeoutParameters{
				RequestTimeout:                pointer.String("1s"),
				ConnectionIdleTimeout:         pointer.String("2s"),
				StreamIdleTimeout:             pointer.String("3s"),
				MaxConnectionDuration:         pointer.String("4s"),
				DelayedCloseTimeout:           pointer.String("5s"),
				ConnectionShutdownGracePeriod: pointer.String("6s"),
				ConnectTimeout:                pointer.String("7s"),
			},
			Cluster: &contour_api_v1alpha1.ClusterParameters{
				DNSLookupFamily: contour_api_v1alpha1.IPv4ClusterDNSFamily,
			},
			Network: &contour_api_v1alpha1.NetworkParameters{
				XffNumTrustedHops: contourconfig.UInt32Ptr(77),
				EnvoyAdminPort:    pointer.Int(9997),
			},
		},
		Gateway: &contour_api_v1alpha1.GatewayConfig{
			ControllerName: "gatewaycontroller",
			GatewayRef: &contour_api_v1alpha1.NamespacedName{
				Namespace: "gatewaynamespace",
				Name:      "gatewayname",
			},
		},
		HTTPProxy: &contour_api_v1alpha1.HTTPProxyConfig{
			DisablePermitInsecure: pointer.Bool(true),
			RootNamespaces:        []string{"rootnamespace"},
			FallbackCertificate: &contour_api_v1alpha1.NamespacedName{
				Namespace: "fallbackcertificatenamespace",
				Name:      "fallbackcertificatename",
			},
		},
		EnableExternalNameService: pointer.Bool(true),
		RateLimitService: &contour_api_v1alpha1.RateLimitServiceConfig{
			ExtensionService: contour_api_v1alpha1.NamespacedName{
				Namespace: "ratelimitservicenamespace",
				Name:      "ratelimitservicename",
			},
			Domain:                  "ratelimitservicedomain",
			FailOpen:                pointer.Bool(true),
			EnableXRateLimitHeaders: pointer.Bool(true),
		},
		Policy: &contour_api_v1alpha1.PolicyConfig{
			RequestHeadersPolicy: &contour_api_v1alpha1.HeadersPolicy{
				Set:    map[string]string{"set": "val"},
				Remove: []string{"remove"},
			},
			ResponseHeadersPolicy: &contour_api_v1alpha1.HeadersPolicy{
				Set:    map[string]string{"set": "val"},
				Remove: []string{"remove"},
			},
			ApplyToIngress: pointer.Bool(true),
		},
		Metrics: &contour_api_v1alpha1.MetricsConfig{
			Address: "9.8.7.6",
			Port:    9876,
			TLS: &contour_api_v1alpha1.MetricsTLS{
				CAFile:   "cafile.cafile",
				CertFile: "certfile.certfile",
				KeyFile:  "keyfile.keyfile",
			},
		},
	}

	tests := map[string]struct {
		contourConfig contour_api_v1alpha1.ContourConfigurationSpec
		want          func() contour_api_v1alpha1.ContourConfigurationSpec
	}{
		"empty ContourConfig results in all the defaults": {
			contourConfig: contour_api_v1alpha1.ContourConfigurationSpec{},
			want:          contourconfig.Defaults,
		},
		"ContourConfig with single non-default field is overlaid correctly": {
			contourConfig: contour_api_v1alpha1.ContourConfigurationSpec{
				XDSServer: &contour_api_v1alpha1.XDSServerConfig{
					Type: contour_api_v1alpha1.EnvoyServerType,
				},
			},
			want: func() contour_api_v1alpha1.ContourConfigurationSpec {
				res := contourconfig.Defaults()
				res.XDSServer.Type = contour_api_v1alpha1.EnvoyServerType
				return res
			},
		},
		"ContourConfig with every field specified with a non-default value results in all of those values used": {
			contourConfig: allFieldsSpecified,
			want: func() contour_api_v1alpha1.ContourConfigurationSpec {
				return allFieldsSpecified
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			res, err := contourconfig.OverlayOnDefaults(tc.contourConfig)
			require.NoError(t, err)
			assert.Equal(t, tc.want(), res)
		})
	}
}

func TestParseTimeoutPolicy(t *testing.T) {
	testCases := map[string]struct {
		config   *contour_api_v1alpha1.TimeoutParameters
		expected contourconfig.Timeouts
		errorMsg string
	}{
		"nil timeout parameters": {
			config: nil,
			expected: contourconfig.Timeouts{
				Request:                       timeout.DefaultSetting(),
				ConnectionIdle:                timeout.DefaultSetting(),
				StreamIdle:                    timeout.DefaultSetting(),
				MaxConnectionDuration:         timeout.DefaultSetting(),
				DelayedClose:                  timeout.DefaultSetting(),
				ConnectionShutdownGracePeriod: timeout.DefaultSetting(),
				ConnectTimeout:                0,
			},
		},
		"timeouts not set": {
			config: &contour_api_v1alpha1.TimeoutParameters{},
			expected: contourconfig.Timeouts{
				Request:                       timeout.DefaultSetting(),
				ConnectionIdle:                timeout.DefaultSetting(),
				StreamIdle:                    timeout.DefaultSetting(),
				MaxConnectionDuration:         timeout.DefaultSetting(),
				DelayedClose:                  timeout.DefaultSetting(),
				ConnectionShutdownGracePeriod: timeout.DefaultSetting(),
				ConnectTimeout:                0,
			},
		},
		"timeouts all set": {
			config: &contour_api_v1alpha1.TimeoutParameters{
				RequestTimeout:                pointer.String("1s"),
				ConnectionIdleTimeout:         pointer.String("2s"),
				StreamIdleTimeout:             pointer.String("3s"),
				MaxConnectionDuration:         pointer.String("infinity"),
				DelayedCloseTimeout:           pointer.String("5s"),
				ConnectionShutdownGracePeriod: pointer.String("6s"),
				ConnectTimeout:                pointer.String("8s"),
			},
			expected: contourconfig.Timeouts{
				Request:                       timeout.DurationSetting(time.Second),
				ConnectionIdle:                timeout.DurationSetting(time.Second * 2),
				StreamIdle:                    timeout.DurationSetting(time.Second * 3),
				MaxConnectionDuration:         timeout.DisabledSetting(),
				DelayedClose:                  timeout.DurationSetting(time.Second * 5),
				ConnectionShutdownGracePeriod: timeout.DurationSetting(time.Second * 6),
				ConnectTimeout:                8 * time.Second,
			},
		},
		"request timeout invalid": {
			config: &contour_api_v1alpha1.TimeoutParameters{
				RequestTimeout: pointer.String("xxx"),
			},
			errorMsg: "failed to parse request timeout",
		},
		"connection idle timeout invalid": {
			config: &contour_api_v1alpha1.TimeoutParameters{
				ConnectionIdleTimeout: pointer.String("a"),
			},
			errorMsg: "failed to parse connection idle timeout",
		},
		"stream idle timeout invalid": {
			config: &contour_api_v1alpha1.TimeoutParameters{
				StreamIdleTimeout: pointer.String("invalid"),
			},
			errorMsg: "failed to parse stream idle timeout",
		},
		"max connection duration invalid": {
			config: &contour_api_v1alpha1.TimeoutParameters{
				MaxConnectionDuration: pointer.String("xxx"),
			},
			errorMsg: "failed to parse max connection duration",
		},
		"delayed close timeout invalid": {
			config: &contour_api_v1alpha1.TimeoutParameters{
				DelayedCloseTimeout: pointer.String("xxx"),
			},
			errorMsg: "failed to parse delayed close timeout",
		},
		"connection shutdown grace period invalid": {
			config: &contour_api_v1alpha1.TimeoutParameters{
				ConnectionShutdownGracePeriod: pointer.String("xxx"),
			},
			errorMsg: "failed to parse connection shutdown grace period",
		},
		"connect timeout invalid": {
			config: &contour_api_v1alpha1.TimeoutParameters{
				ConnectTimeout: pointer.String("infinite"),
			},
			errorMsg: "failed to parse connect timeout",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			parsed, err := contourconfig.ParseTimeoutPolicy(tc.config)
			if len(tc.errorMsg) > 0 {
				require.Error(t, err, "expected error to be returned")
				require.Contains(t, err.Error(), tc.errorMsg)
			} else {
				require.Nil(t, err)
				require.Equal(t, tc.expected, parsed)
			}
		})
	}
}
