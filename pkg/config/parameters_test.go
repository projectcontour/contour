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

	"github.com/projectcontour/contour/internal/ref"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
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
server:
    xds-server-type: contour
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
	assert.Error(t, ClusterDNSFamilyType("").Validate())
	assert.Error(t, ClusterDNSFamilyType("foo").Validate())

	assert.NoError(t, AutoClusterDNSFamily.Validate())
	assert.NoError(t, IPv4ClusterDNSFamily.Validate())
	assert.NoError(t, IPv6ClusterDNSFamily.Validate())
	assert.NoError(t, AllClusterDNSFamily.Validate())
}

func TestValidateServerHeaderTranformationType(t *testing.T) {
	assert.Error(t, ServerHeaderTransformationType("").Validate())
	assert.Error(t, ServerHeaderTransformationType("foo").Validate())

	assert.NoError(t, OverwriteServerHeader.Validate())
	assert.NoError(t, AppendIfAbsentServerHeader.Validate())
	assert.NoError(t, PassThroughServerHeader.Validate())
}

func TestValidateHeadersPolicy(t *testing.T) {
	assert.Error(t, HeadersPolicy{
		Set: map[string]string{
			"inv@lid-header": "ook",
		},
	}.Validate())
	assert.Error(t, HeadersPolicy{
		Remove: []string{"inv@lid-header"},
	}.Validate())
	assert.NoError(t, HeadersPolicy{
		Set:    map[string]string{},
		Remove: []string{},
	}.Validate())
	assert.NoError(t, HeadersPolicy{
		Set: map[string]string{"X-Envoy-Host": "envoy-a12345"},
	}.Validate())
	assert.NoError(t, HeadersPolicy{
		Set: map[string]string{
			"X-Envoy-Host":     "envoy-s12345",
			"l5d-dst-override": "kuard.default.svc.cluster.local:80",
		},
		Remove: []string{"Sensitive-Header"},
	}.Validate())
	assert.NoError(t, HeadersPolicy{
		Set: map[string]string{
			"X-Envoy-Host":     "%HOSTNAME%",
			"l5d-dst-override": "%CONTOUR_SERVICE_NAME%.%CONTOUR_NAMESPACE%.svc.cluster.local:%CONTOUR_SERVICE_PORT%",
		},
	}.Validate())
}

func TestValidateNamespacedName(t *testing.T) {
	assert.NoErrorf(t, NamespacedName{}.Validate(), "empty name should be OK")
	assert.NoError(t, NamespacedName{Name: "name", Namespace: "ns"}.Validate())

	assert.Error(t, NamespacedName{Name: "name"}.Validate())
	assert.Error(t, NamespacedName{Namespace: "ns"}.Validate())
}

func TestValidateServerType(t *testing.T) {
	assert.Error(t, ServerType("").Validate())
	assert.Error(t, ServerType("foo").Validate())

	assert.NoError(t, EnvoyServerType.Validate())
	assert.NoError(t, ContourServerType.Validate())
}

func TestValidateGatewayParameters(t *testing.T) {
	// Not required if nothing is passed.
	var gw *GatewayParameters
	assert.Equal(t, nil, gw.Validate())

	// ControllerName is required.
	gw = &GatewayParameters{ControllerName: "controller"}
	assert.Equal(t, nil, gw.Validate())
}

func TestValidateHTTPVersionType(t *testing.T) {
	assert.Error(t, HTTPVersionType("").Validate())
	assert.Error(t, HTTPVersionType("foo").Validate())
	assert.Error(t, HTTPVersionType("HTTP/1.1").Validate())
	assert.Error(t, HTTPVersionType("HTTP/2").Validate())

	assert.NoError(t, HTTPVersion1.Validate())
	assert.NoError(t, HTTPVersion2.Validate())
}

func TestValidateTimeoutParams(t *testing.T) {
	assert.NoError(t, TimeoutParameters{}.Validate())
	assert.NoError(t, TimeoutParameters{
		RequestTimeout:                "infinite",
		ConnectionIdleTimeout:         "infinite",
		StreamIdleTimeout:             "infinite",
		MaxConnectionDuration:         "infinite",
		DelayedCloseTimeout:           "infinite",
		ConnectionShutdownGracePeriod: "infinite",
		ConnectTimeout:                "2s",
	}.Validate())
	assert.NoError(t, TimeoutParameters{
		RequestTimeout:                "infinity",
		ConnectionIdleTimeout:         "infinity",
		StreamIdleTimeout:             "infinity",
		MaxConnectionDuration:         "infinity",
		DelayedCloseTimeout:           "infinity",
		ConnectionShutdownGracePeriod: "infinity",
		ConnectTimeout:                "2s",
	}.Validate())

	assert.Error(t, TimeoutParameters{RequestTimeout: "foo"}.Validate())
	assert.Error(t, TimeoutParameters{ConnectionIdleTimeout: "bar"}.Validate())
	assert.Error(t, TimeoutParameters{StreamIdleTimeout: "baz"}.Validate())
	assert.Error(t, TimeoutParameters{MaxConnectionDuration: "boop"}.Validate())
	assert.Error(t, TimeoutParameters{DelayedCloseTimeout: "bebop"}.Validate())
	assert.Error(t, TimeoutParameters{ConnectionShutdownGracePeriod: "bong"}.Validate())
	assert.Error(t, TimeoutParameters{ConnectTimeout: "infinite"}.Validate())

}

func TestTLSParametersValidation(t *testing.T) {
	// Fallback certificate validation
	assert.NoError(t, TLSParameters{
		FallbackCertificate: NamespacedName{
			Name:      "  ",
			Namespace: "  ",
		},
	}.Validate())
	assert.Error(t, TLSParameters{
		FallbackCertificate: NamespacedName{
			Name:      "somename",
			Namespace: "  ",
		},
	}.Validate())

	// Client certificate validation
	assert.NoError(t, TLSParameters{
		ClientCertificate: NamespacedName{
			Name:      "  ",
			Namespace: "  ",
		},
	}.Validate())
	assert.Error(t, TLSParameters{
		ClientCertificate: NamespacedName{
			Name:      "",
			Namespace: "somenamespace  ",
		},
	}.Validate())

	// Cipher suites validation
	assert.NoError(t, ProtocolParameters{
		CipherSuites: []string{},
	}.Validate())
	assert.NoError(t, ProtocolParameters{
		CipherSuites: []string{
			"[ECDHE-ECDSA-AES128-GCM-SHA256|ECDHE-ECDSA-CHACHA20-POLY1305]",
			"ECDHE-ECDSA-AES128-GCM-SHA256",
		},
	}.Validate())
	assert.Error(t, ProtocolParameters{
		CipherSuites: []string{
			"NOTAVALIDCIPHER",
		},
	}.Validate())

	// TLS protocol version validation
	assert.NoError(t, ProtocolParameters{
		MinimumProtocolVersion: "1.2",
	}.Validate())
	assert.Error(t, ProtocolParameters{
		MinimumProtocolVersion: "1.1",
	}.Validate())
	assert.NoError(t, ProtocolParameters{
		MaximumProtocolVersion: "1.3",
	}.Validate())
	assert.Error(t, ProtocolParameters{
		MaximumProtocolVersion: "invalid",
	}.Validate())
	assert.NoError(t, ProtocolParameters{
		MinimumProtocolVersion: "1.2",
		MaximumProtocolVersion: "1.3",
	}.Validate())
	assert.Error(t, ProtocolParameters{
		MinimumProtocolVersion: "1.3",
		MaximumProtocolVersion: "1.2",
	}.Validate())
	assert.NoError(t, ProtocolParameters{
		MinimumProtocolVersion: "1.2",
		MaximumProtocolVersion: "1.2",
	}.Validate())
	assert.NoError(t, ProtocolParameters{
		MinimumProtocolVersion: "1.3",
		MaximumProtocolVersion: "1.3",
	}.Validate())
	assert.Error(t, ProtocolParameters{
		MinimumProtocolVersion: "1.1",
		MaximumProtocolVersion: "1.3",
	}.Validate())
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
server:
  xds-server-type: magic
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
		assert.Equal(t, ref.To(uint32(1)), conf.Listener.MaxRequestsPerConnection)
	}, `
listener:
  max-requests-per-connection: 1
`)

	check(func(t *testing.T, conf *Parameters) {
		assert.Equal(t, ref.To(uint32(10)), conf.Listener.HTTP2MaxConcurrentStreams)
	}, `
listener:
  http2-max-concurrent-streams: 10
`)

	check(func(t *testing.T, conf *Parameters) {
		assert.Equal(t, ref.To(uint32(1)), conf.Listener.PerConnectionBufferLimitBytes)
	}, `
listener:
  per-connection-buffer-limit-bytes: 1
`)

	check(func(t *testing.T, conf *Parameters) {
		assert.Equal(t, ref.To(uint32(1)), conf.Listener.MaxRequestsPerIOCycle)
	}, `
listener:
  max-requests-per-io-cycle: 1
`)

	check(func(t *testing.T, conf *Parameters) {
		assert.Equal(t, ref.To(uint32(1)), conf.Cluster.MaxRequestsPerConnection)
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
	assert.NoError(t, valid.Validate())

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
	assert.NoError(t, valid.Validate())
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
	assert.Error(t, tlsKeyMissing.Validate())

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
	assert.Error(t, tlsCAWithoutServerCert.Validate())

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
		MaxRequestsPerConnection: ref.To(uint32(1)),
	}
	require.NoError(t, l.Validate())
	l = &ListenerParameters{
		MaxRequestsPerConnection: ref.To(uint32(0)),
	}
	require.Error(t, l.Validate())
	l = &ListenerParameters{
		PerConnectionBufferLimitBytes: ref.To(uint32(1)),
	}
	require.NoError(t, l.Validate())
	l = &ListenerParameters{
		PerConnectionBufferLimitBytes: ref.To(uint32(0)),
	}
	require.Error(t, l.Validate())
	l = &ListenerParameters{
		MaxRequestsPerIOCycle: ref.To(uint32(1)),
	}
	require.NoError(t, l.Validate())
	l = &ListenerParameters{
		MaxRequestsPerIOCycle: ref.To(uint32(0)),
	}
	require.Error(t, l.Validate())
	l = &ListenerParameters{
		HTTP2MaxConcurrentStreams: ref.To(uint32(1)),
	}
	require.NoError(t, l.Validate())
	l = &ListenerParameters{
		HTTP2MaxConcurrentStreams: ref.To(uint32(0)),
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
}

func TestClusterParametersValidation(t *testing.T) {
	var l *ClusterParameters
	l = &ClusterParameters{
		MaxRequestsPerConnection: ref.To(uint32(0)),
	}
	require.Error(t, l.Validate())
	l = &ClusterParameters{
		MaxRequestsPerConnection: ref.To(uint32(1)),
	}
	require.NoError(t, l.Validate())
	l = &ClusterParameters{
		PerConnectionBufferLimitBytes: ref.To(uint32(0)),
	}
	require.Error(t, l.Validate())
	l = &ClusterParameters{
		UpstreamTLS: ProtocolParameters{
			MaximumProtocolVersion: "invalid",
		},
	}
	require.Error(t, l.Validate())
	l = &ClusterParameters{
		PerConnectionBufferLimitBytes: ref.To(uint32(1)),
	}
	require.NoError(t, l.Validate())
}

func TestTracingConfigValidation(t *testing.T) {
	var trace *Tracing
	require.NoError(t, trace.Validate())

	trace = &Tracing{
		IncludePodDetail: ref.To(false),
		ServiceName:      ref.To("contour"),
		OverallSampling:  ref.To("100"),
		MaxPathTagLength: ref.To(uint32(256)),
		CustomTags:       nil,
		ExtensionService: "projectcontour/otel-collector",
	}
	require.NoError(t, trace.Validate())

	trace = &Tracing{
		IncludePodDetail: ref.To(false),
		ServiceName:      ref.To("contour"),
		OverallSampling:  ref.To("100"),
		MaxPathTagLength: ref.To(uint32(256)),
		CustomTags:       nil,
	}
	require.Error(t, trace.Validate())

	trace = &Tracing{
		IncludePodDetail: ref.To(false),
		OverallSampling:  ref.To("100"),
		MaxPathTagLength: ref.To(uint32(256)),
		CustomTags:       nil,
		ExtensionService: "projectcontour/otel-collector",
	}
	require.NoError(t, trace.Validate())

	trace = &Tracing{
		OverallSampling:  ref.To("100"),
		MaxPathTagLength: ref.To(uint32(256)),
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
		OverallSampling:  ref.To("100"),
		MaxPathTagLength: ref.To(uint32(256)),
		CustomTags: []CustomTag{
			{
				Literal: "literal",
			},
		},
		ExtensionService: "projectcontour/otel-collector",
	}
	require.Error(t, trace.Validate())

	trace = &Tracing{
		IncludePodDetail: ref.To(true),
		OverallSampling:  ref.To("100"),
		MaxPathTagLength: ref.To(uint32(256)),
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
