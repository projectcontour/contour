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

package config

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	"k8s.io/utils/ptr"
)

func TestGetenvOr(t *testing.T) {
	assert.Equal(t, t.Name(), GetenvOr("B5E09AAD-DEFC-4650-9DE6-0F2E3AF7FCF2", t.Name()))

	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		assert.NotEqual(t, t.Name(), GetenvOr(parts[0], t.Name()))
	}
}

func TestParseDefaults(t *testing.T) {
	savedHome := os.Getenv("HOME")
	defer func() {
		os.Setenv("HOME", savedHome)
	}()

	require.NoError(t, os.Setenv("HOME", t.Name()))

	data, err := yaml.Marshal(Defaults())
	require.NoError(t, err)

	expected := `
debug: false
kubeconfig: TestParseDefaults/.kube/config
accesslog-format: envoy
json-fields:
    - '@timestamp'
    - authority
    - bytes_received
    - bytes_sent
    - downstream_local_address
    - downstream_remote_address
    - duration
    - method
    - path
    - protocol
    - request_id
    - requested_server_name
    - response_code
    - response_flags
    - uber_trace_id
    - upstream_cluster
    - upstream_host
    - upstream_local_address
    - upstream_service_time
    - user_agent
    - x_forwarded_for
    - grpc_status
    - grpc_status_number
accesslog-level: info
serverHeaderTransformation: overwrite
timeouts:
    connection-idle-timeout: 60s
    connect-timeout: 2s
envoy-service-namespace: projectcontour
envoy-service-name: envoy
default-http-versions: []
cluster:
    dns-lookup-family: auto
network:
    admin-port: 9001
`
	assert.Equal(t, strings.TrimSpace(expected), strings.TrimSpace(string(data)))

	conf, err := Parse(strings.NewReader(expected))
	require.NoError(t, err)
	require.NoError(t, conf.Validate())

	wanted := Defaults()
	assert.Equal(t, &wanted, conf)
}

func TestParseFailure(t *testing.T) {
	badYAML := `
foo: bad

`
	_, err := Parse(strings.NewReader(badYAML))
	require.Error(t, err)
}

func TestParseApplyToIngress(t *testing.T) {
	yaml := `
policy:
  applyToIngress: true
`

	conf, err := Parse(strings.NewReader((yaml)))
	require.NoError(t, err)

	wanted := Defaults()
	wanted.Policy.ApplyToIngress = true

	assert.Equal(t, &wanted, conf)
}

func TestValidateClusterDNSFamilyType(t *testing.T) {
	require.Error(t, ClusterDNSFamilyType("").Validate())
	require.Error(t, ClusterDNSFamilyType("foo").Validate())

	require.NoError(t, AutoClusterDNSFamily.Validate())
	require.NoError(t, IPv4ClusterDNSFamily.Validate())
	require.NoError(t, IPv6ClusterDNSFamily.Validate())
	require.NoError(t, AllClusterDNSFamily.Validate())
}

func TestValidateServerHeaderTranformationType(t *testing.T) {
	require.Error(t, ServerHeaderTransformationType("").Validate())
	require.Error(t, ServerHeaderTransformationType("foo").Validate())

	require.NoError(t, OverwriteServerHeader.Validate())
	require.NoError(t, AppendIfAbsentServerHeader.Validate())
	require.NoError(t, PassThroughServerHeader.Validate())
}

func TestValidateHeadersPolicy(t *testing.T) {
	require.Error(t, HeadersPolicy{
		Set: map[string]string{
			"inv@lid-header": "ook",
		},
	}.Validate())
	require.Error(t, HeadersPolicy{
		Remove: []string{"inv@lid-header"},
	}.Validate())
	require.NoError(t, HeadersPolicy{
		Set:    map[string]string{},
		Remove: []string{},
	}.Validate())
	require.NoError(t, HeadersPolicy{
		Set: map[string]string{"X-Envoy-Host": "envoy-a12345"},
	}.Validate())
	require.NoError(t, HeadersPolicy{
		Set: map[string]string{
			"X-Envoy-Host":     "envoy-s12345",
			"l5d-dst-override": "kuard.default.svc.cluster.local:80",
		},
		Remove: []string{"Sensitive-Header"},
	}.Validate())
	require.NoError(t, HeadersPolicy{
		Set: map[string]string{
			"X-Envoy-Host":     "%HOSTNAME%",
			"l5d-dst-override": "%CONTOUR_SERVICE_NAME%.%CONTOUR_NAMESPACE%.svc.cluster.local:%CONTOUR_SERVICE_PORT%",
		},
	}.Validate())
}

func TestValidateNamespacedName(t *testing.T) {
	require.NoErrorf(t, NamespacedName{}.Validate(), "empty name should be OK")
	require.NoError(t, NamespacedName{Name: "name", Namespace: "ns"}.Validate())

	require.Error(t, NamespacedName{Name: "name"}.Validate())
	require.Error(t, NamespacedName{Namespace: "ns"}.Validate())
}

func TestValidateGatewayParameters(t *testing.T) {
	// Not required if nothing is passed.
	var gw *GatewayParameters
	require.NoError(t, gw.Validate())

	// Namespace and name are required
	gw = &GatewayParameters{GatewayRef: NamespacedName{Namespace: "foo", Name: "bar"}}
	require.NoError(t, gw.Validate())

	// Namespace is required
	gw = &GatewayParameters{GatewayRef: NamespacedName{Name: "bar"}}
	require.Error(t, gw.Validate())

	// Name is required
	gw = &GatewayParameters{GatewayRef: NamespacedName{Namespace: "foo"}}
	require.Error(t, gw.Validate())
}

func TestValidateHTTPVersionType(t *testing.T) {
	require.Error(t, HTTPVersionType("").Validate())
	require.Error(t, HTTPVersionType("foo").Validate())
	require.Error(t, HTTPVersionType("HTTP/1.1").Validate())
	require.Error(t, HTTPVersionType("HTTP/2").Validate())

	require.NoError(t, HTTPVersion1.Validate())
	require.NoError(t, HTTPVersion2.Validate())
}

func TestValidateTimeoutParams(t *testing.T) {
	require.NoError(t, TimeoutParameters{}.Validate())
	require.NoError(t, TimeoutParameters{
		RequestTimeout:                "infinite",
		ConnectionIdleTimeout:         "infinite",
		StreamIdleTimeout:             "infinite",
		MaxConnectionDuration:         "infinite",
		DelayedCloseTimeout:           "infinite",
		ConnectionShutdownGracePeriod: "infinite",
		ConnectTimeout:                "2s",
	}.Validate())
	require.NoError(t, TimeoutParameters{
		RequestTimeout:                "infinity",
		ConnectionIdleTimeout:         "infinity",
		StreamIdleTimeout:             "infinity",
		MaxConnectionDuration:         "infinity",
		DelayedCloseTimeout:           "infinity",
		ConnectionShutdownGracePeriod: "infinity",
		ConnectTimeout:                "2s",
	}.Validate())

	require.Error(t, TimeoutParameters{RequestTimeout: "foo"}.Validate())
	require.Error(t, TimeoutParameters{ConnectionIdleTimeout: "bar"}.Validate())
	require.Error(t, TimeoutParameters{StreamIdleTimeout: "baz"}.Validate())
	require.Error(t, TimeoutParameters{MaxConnectionDuration: "boop"}.Validate())
	require.Error(t, TimeoutParameters{DelayedCloseTimeout: "bebop"}.Validate())
	require.Error(t, TimeoutParameters{ConnectionShutdownGracePeriod: "bong"}.Validate())
	require.Error(t, TimeoutParameters{ConnectTimeout: "infinite"}.Validate())
}

func TestTLSParametersValidation(t *testing.T) {
	// Fallback certificate validation
	require.NoError(t, TLSParameters{
		FallbackCertificate: NamespacedName{
			Name:      "  ",
			Namespace: "  ",
		},
	}.Validate())
	require.Error(t, TLSParameters{
		FallbackCertificate: NamespacedName{
			Name:      "somename",
			Namespace: "  ",
		},
	}.Validate())

	// Client certificate validation
	require.NoError(t, TLSParameters{
		ClientCertificate: NamespacedName{
			Name:      "  ",
			Namespace: "  ",
		},
	}.Validate())
	require.Error(t, TLSParameters{
		ClientCertificate: NamespacedName{
			Name:      "",
			Namespace: "somenamespace  ",
		},
	}.Validate())

	// Cipher suites validation
	require.NoError(t, ProtocolParameters{
		CipherSuites: []string{},
	}.Validate())
	require.NoError(t, ProtocolParameters{
		CipherSuites: []string{
			"[ECDHE-ECDSA-AES128-GCM-SHA256|ECDHE-ECDSA-CHACHA20-POLY1305]",
			"ECDHE-ECDSA-AES128-GCM-SHA256",
		},
	}.Validate())
	require.Error(t, ProtocolParameters{
		CipherSuites: []string{
			"NOTAVALIDCIPHER",
		},
	}.Validate())

	// TLS protocol version validation
	require.NoError(t, ProtocolParameters{
		MinimumProtocolVersion: "1.2",
	}.Validate())
	require.Error(t, ProtocolParameters{
		MinimumProtocolVersion: "1.1",
	}.Validate())
	require.NoError(t, ProtocolParameters{
		MaximumProtocolVersion: "1.3",
	}.Validate())
	require.Error(t, ProtocolParameters{
		MaximumProtocolVersion: "invalid",
	}.Validate())
	require.NoError(t, ProtocolParameters{
		MinimumProtocolVersion: "1.2",
		MaximumProtocolVersion: "1.3",
	}.Validate())
	require.Error(t, ProtocolParameters{
		MinimumProtocolVersion: "1.3",
		MaximumProtocolVersion: "1.2",
	}.Validate())
	require.NoError(t, ProtocolParameters{
		MinimumProtocolVersion: "1.2",
		MaximumProtocolVersion: "1.2",
	}.Validate())
	require.NoError(t, ProtocolParameters{
		MinimumProtocolVersion: "1.3",
		MaximumProtocolVersion: "1.3",
	}.Validate())
	require.Error(t, ProtocolParameters{
		MinimumProtocolVersion: "1.1",
		MaximumProtocolVersion: "1.3",
	}.Validate())
}

func TestCompressionValidation(t *testing.T) {
	require.NoError(t, CompressionParameters{""}.Validate())
	require.NoError(t, CompressionParameters{CompressionBrotli}.Validate())
	require.NoError(t, CompressionParameters{CompressionDisabled}.Validate())
	require.NoError(t, CompressionParameters{CompressionGzip}.Validate())
	require.NoError(t, CompressionParameters{CompressionZstd}.Validate())
	require.Contains(t, CompressionParameters{"bogus"}.Validate().Error(), "invalid compression type")
}

func TestConfigFileValidation(t *testing.T) {
	check := func(yamlIn string) {
		t.Helper()

		conf, err := Parse(strings.NewReader(yamlIn))
		require.NoError(t, err)
		require.Error(t, conf.Validate())
	}

	check(`
cluster:
  dns-lookup-family: stone
`)

	check(`
accesslog-format: /dev/null
`)

	check(`
accesslog-format-string: "%REQ%"
`)

	check(`
json-fields:
- one
`)

	check(`
accesslog-level: invalid
`)

	check(`
tls:
  fallback-certificate:
    name: foo
`)

	check(`
tls:
  envoy-client-certificate:
    name: foo
`)

	check(`
tls:
  cipher-suites:
  - NOTVALID
`)

	check(`
timeouts:
  request-timeout: none
`)

	check(`
default-http-versions:
- http/0.9
`)

	check(`
listener:
  connection-balancer: notexact
`)
}

func TestConfigFileDefaultOverrideImport(t *testing.T) {
	check := func(verifier func(*testing.T, *Parameters), yamlIn string) {
		t.Helper()
		conf, err := Parse(strings.NewReader(yamlIn))
		require.NoError(t, err)
		verifier(t, conf)
	}

	check(func(t *testing.T, conf *Parameters) {
		wanted := Defaults()
		assert.Equal(t, &wanted, conf)
	}, "")

	check(func(t *testing.T, conf *Parameters) {
		wanted := Defaults()
		assert.Equal(t, &wanted, conf)
	}, `
incluster: false
disablePermitInsecure: false
disableAllowChunkedLength: false
disableMergeSlashes: false
serverHeaderTransformation: overwrite
`,
	)

	check(func(t *testing.T, conf *Parameters) {
		wanted := Defaults()
		assert.Equal(t, &wanted, conf)
	}, `
tls:
`)

	check(func(t *testing.T, conf *Parameters) {
		assert.Equal(t, "1.2", conf.TLS.MinimumProtocolVersion)
		assert.Equal(t, "1.3", conf.TLS.MaximumProtocolVersion)
		assert.Equal(t, TLSCiphers{"ECDHE-RSA-AES256-GCM-SHA384"}, conf.TLS.CipherSuites)
	}, `
tls:
  minimum-protocol-version: 1.2
  maximum-protocol-version: 1.3
  cipher-suites:
  - ECDHE-RSA-AES256-GCM-SHA384
`)

	check(func(t *testing.T, conf *Parameters) {
		assert.ElementsMatch(t,
			[]HTTPVersionType{HTTPVersion1, HTTPVersion2, HTTPVersion2, HTTPVersion1},
			conf.DefaultHTTPVersions,
		)
	}, `
default-http-versions:
- http/1.1
- http/2
- HTTP/2
- HTTP/1.1
`)

	check(func(t *testing.T, conf *Parameters) {
		assert.Equal(t, uint32(1), conf.Network.XffNumTrustedHops)
	}, `
network:
  num-trusted-hops: 1
  admin-port: 9001
`)

	check(func(t *testing.T, conf *Parameters) {
		assert.True(t, conf.Network.EnvoyStripTrailingHostDot)
	}, `
network:
  strip-trailing-host-dot: true
  num-trusted-hops: 0
  admin-port: 9001
`)

	check(func(t *testing.T, conf *Parameters) {
		assert.Equal(t, ptr.To(uint32(1)), conf.Listener.MaxRequestsPerConnection)
	}, `
listener:
  max-requests-per-connection: 1
`)

	check(func(t *testing.T, conf *Parameters) {
		assert.Equal(t, ptr.To(uint32(10)), conf.Listener.HTTP2MaxConcurrentStreams)
	}, `
listener:
  http2-max-concurrent-streams: 10
`)

	check(func(t *testing.T, conf *Parameters) {
		assert.Equal(t, ptr.To(uint32(1)), conf.Listener.PerConnectionBufferLimitBytes)
	}, `
listener:
  per-connection-buffer-limit-bytes: 1
`)

	check(func(t *testing.T, conf *Parameters) {
		assert.Equal(t, ptr.To(uint32(1)), conf.Listener.MaxRequestsPerIOCycle)
	}, `
listener:
  max-requests-per-io-cycle: 1
`)

	check(func(t *testing.T, conf *Parameters) {
		assert.Equal(t, ptr.To(uint32(1)), conf.Listener.MaxConnectionsPerListener)
	}, `
listener:
  max-connections-per-listener: 1
`)

	check(func(t *testing.T, conf *Parameters) {
		assert.Equal(t, ptr.To(uint32(1)), conf.Cluster.MaxRequestsPerConnection)
	}, `
cluster:
  max-requests-per-connection: 1
`)

	check(func(t *testing.T, conf *Parameters) {
		assert.Equal(t, uint32(42), conf.Cluster.GlobalCircuitBreakerDefaults.MaxConnections)
		assert.Equal(t, uint32(43), conf.Cluster.GlobalCircuitBreakerDefaults.MaxPendingRequests)
		assert.Equal(t, uint32(44), conf.Cluster.GlobalCircuitBreakerDefaults.MaxRequests)
		assert.Equal(t, uint32(0), conf.Cluster.GlobalCircuitBreakerDefaults.MaxRetries)
	}, `
cluster:
  circuit-breakers:
    max-connections: 42
    max-pending-requests: 43
    max-requests: 44
`)
}

func TestMetricsParametersValidation(t *testing.T) {
	valid := MetricsParameters{
		Contour: MetricsServerParameters{
			Address: "0.0.0.0",
			Port:    1234,
		},
		Envoy: MetricsServerParameters{
			Address: "0.0.0.0",
			Port:    1234,
		},
	}
	require.NoError(t, valid.Validate())

	tlsValid := MetricsParameters{
		Contour: MetricsServerParameters{
			Address:    "0.0.0.0",
			Port:       1234,
			ServerCert: "cert.pem",
			ServerKey:  "key.pem",
		},
		Envoy: MetricsServerParameters{
			Address: "0.0.0.0",
			Port:    1234,
		},
	}
	require.NoError(t, valid.Validate())
	assert.True(t, tlsValid.Contour.HasTLS())
	assert.False(t, tlsValid.Envoy.HasTLS())

	tlsKeyMissing := MetricsParameters{
		Contour: MetricsServerParameters{
			Address:    "0.0.0.0",
			Port:       1234,
			ServerCert: "cert.pem",
		},
		Envoy: MetricsServerParameters{
			Address: "0.0.0.0",
			Port:    1234,
		},
	}
	require.Error(t, tlsKeyMissing.Validate())

	tlsCAWithoutServerCert := MetricsParameters{
		Contour: MetricsServerParameters{
			Address: "0.0.0.0",
			Port:    1234,
		},
		Envoy: MetricsServerParameters{
			Address:  "0.0.0.0",
			Port:     1234,
			CABundle: "ca.pem",
		},
	}
	require.Error(t, tlsCAWithoutServerCert.Validate())
}

func TestListenerValidation(t *testing.T) {
	var l *ListenerParameters
	require.NoError(t, l.Validate())
	l = &ListenerParameters{
		ConnectionBalancer: "",
	}
	require.NoError(t, l.Validate())
	l = &ListenerParameters{
		ConnectionBalancer: "exact",
	}
	require.NoError(t, l.Validate())
	l = &ListenerParameters{
		ConnectionBalancer: "invalid",
	}
	require.Error(t, l.Validate())
	l = &ListenerParameters{
		MaxRequestsPerConnection: ptr.To(uint32(1)),
	}
	require.NoError(t, l.Validate())
	l = &ListenerParameters{
		MaxRequestsPerConnection: ptr.To(uint32(0)),
	}
	require.Error(t, l.Validate())
	l = &ListenerParameters{
		PerConnectionBufferLimitBytes: ptr.To(uint32(1)),
	}
	require.NoError(t, l.Validate())
	l = &ListenerParameters{
		PerConnectionBufferLimitBytes: ptr.To(uint32(0)),
	}
	require.Error(t, l.Validate())
	l = &ListenerParameters{
		MaxRequestsPerIOCycle: ptr.To(uint32(1)),
	}
	require.NoError(t, l.Validate())
	l = &ListenerParameters{
		MaxRequestsPerIOCycle: ptr.To(uint32(0)),
	}
	require.Error(t, l.Validate())
	l = &ListenerParameters{
		HTTP2MaxConcurrentStreams: ptr.To(uint32(1)),
	}
	require.NoError(t, l.Validate())
	l = &ListenerParameters{
		HTTP2MaxConcurrentStreams: ptr.To(uint32(0)),
	}
	require.Error(t, l.Validate())
	l = &ListenerParameters{
		SocketOptions: SocketOptions{
			TOS:          64,
			TrafficClass: 64,
		},
	}
	require.NoError(t, l.Validate())
	l = &ListenerParameters{SocketOptions: SocketOptions{TOS: 256}}
	require.Error(t, l.Validate())
	l = &ListenerParameters{SocketOptions: SocketOptions{TrafficClass: 256}}
	require.Error(t, l.Validate())
	l = &ListenerParameters{SocketOptions: SocketOptions{TOS: -1}}
	require.Error(t, l.Validate())
	l = &ListenerParameters{SocketOptions: SocketOptions{TrafficClass: -1}}
	require.Error(t, l.Validate())

	l = &ListenerParameters{
		MaxConnectionsPerListener: ptr.To(uint32(1)),
	}
	require.NoError(t, l.Validate())
	l = &ListenerParameters{
		MaxConnectionsPerListener: ptr.To(uint32(0)),
	}
	require.Error(t, l.Validate())
}

func TestClusterParametersValidation(t *testing.T) {
	var l *ClusterParameters
	l = &ClusterParameters{
		MaxRequestsPerConnection: ptr.To(uint32(0)),
	}
	require.Error(t, l.Validate())
	l = &ClusterParameters{
		MaxRequestsPerConnection: ptr.To(uint32(1)),
	}
	require.NoError(t, l.Validate())
	l = &ClusterParameters{
		PerConnectionBufferLimitBytes: ptr.To(uint32(0)),
	}
	require.Error(t, l.Validate())
	l = &ClusterParameters{
		UpstreamTLS: ProtocolParameters{
			MaximumProtocolVersion: "invalid",
		},
	}
	require.Error(t, l.Validate())
	l = &ClusterParameters{
		PerConnectionBufferLimitBytes: ptr.To(uint32(1)),
	}
	require.NoError(t, l.Validate())
}

func TestTracingConfigValidation(t *testing.T) {
	var trace *Tracing
	require.NoError(t, trace.Validate())

	trace = &Tracing{
		IncludePodDetail: ptr.To(false),
		ServiceName:      ptr.To("contour"),
		OverallSampling:  ptr.To("100"),
		MaxPathTagLength: ptr.To(uint32(256)),
		CustomTags:       nil,
		ExtensionService: "projectcontour/otel-collector",
	}
	require.NoError(t, trace.Validate())

	trace = &Tracing{
		IncludePodDetail: ptr.To(false),
		ServiceName:      ptr.To("contour"),
		OverallSampling:  ptr.To("100"),
		MaxPathTagLength: ptr.To(uint32(256)),
		CustomTags:       nil,
	}
	require.Error(t, trace.Validate())

	trace = &Tracing{
		IncludePodDetail: ptr.To(false),
		OverallSampling:  ptr.To("100"),
		MaxPathTagLength: ptr.To(uint32(256)),
		CustomTags:       nil,
		ExtensionService: "projectcontour/otel-collector",
	}
	require.NoError(t, trace.Validate())

	trace = &Tracing{
		OverallSampling:  ptr.To("100"),
		MaxPathTagLength: ptr.To(uint32(256)),
		CustomTags: []CustomTag{
			{
				TagName:           "first",
				Literal:           "literal",
				RequestHeaderName: ":path",
			},
		},
		ExtensionService: "projectcontour/otel-collector",
	}
	require.Error(t, trace.Validate())

	trace = &Tracing{
		OverallSampling:  ptr.To("100"),
		MaxPathTagLength: ptr.To(uint32(256)),
		CustomTags: []CustomTag{
			{
				Literal: "literal",
			},
		},
		ExtensionService: "projectcontour/otel-collector",
	}
	require.Error(t, trace.Validate())

	trace = &Tracing{
		IncludePodDetail: ptr.To(true),
		OverallSampling:  ptr.To("100"),
		MaxPathTagLength: ptr.To(uint32(256)),
		CustomTags: []CustomTag{
			{
				TagName: "first",
				Literal: "literal",
			},
			{
				TagName:           "first",
				RequestHeaderName: ":path",
			},
		},
		ExtensionService: "projectcontour/otel-collector",
	}
	require.Error(t, trace.Validate())
}
