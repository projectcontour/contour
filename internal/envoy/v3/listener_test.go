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

package v3

import (
	"testing"
	"time"

	envoy_config_accesslog_v3 "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_compression_gzip_compressor_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/compression/gzip/compressor/v3"
	envoy_filter_http_compressor_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/compressor/v3"
	envoy_filter_http_cors_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/cors/v3"
	envoy_filter_http_grpc_stats_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/grpc_stats/v3"
	envoy_filter_http_grpc_web_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/grpc_web/v3"
	envoy_filter_http_local_ratelimit_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/local_ratelimit/v3"
	envoy_filter_http_lua_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/lua/v3"
	envoy_filter_http_rbac_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/rbac/v3"
	envoy_filter_http_router_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/router/v3"
	envoy_filter_network_http_connection_manager_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoy_filter_network_tcp_proxy_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	envoy_transport_socket_tls_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/envoy"
	"github.com/projectcontour/contour/internal/protobuf"
	"github.com/projectcontour/contour/internal/timeout"
)

var compressorContentTypes = []string{
	// Default content-types https://github.com/envoyproxy/envoy/blob/e74999dbdb12aa4d6b7a5d62d51731ea86bf72be/source/extensions/filters/http/compressor/compressor_filter.cc#L35-L38
	"text/html", "text/plain", "text/css", "application/javascript", "application/x-javascript",
	"text/javascript", "text/x-javascript", "text/ecmascript", "text/js", "text/jscript",
	"text/x-js", "application/ecmascript", "application/x-json", "application/xml",
	"application/json", "image/svg+xml", "text/xml", "application/xhtml+xml",
	// Additional content-types for grpc-web https://github.com/grpc/grpc/blob/master/doc/PROTOCOL-WEB.md#protocol-differences-vs-grpc-over-http2
	"application/grpc-web", "application/grpc-web+proto", "application/grpc-web+json", "application/grpc-web+thrift",
	"application/grpc-web-text", "application/grpc-web-text+proto", "application/grpc-web-text+thrift",
}

func TestCodecForVersions(t *testing.T) {
	assert.Equal(t, HTTPVersionAuto, CodecForVersions(HTTPVersionAuto))
	assert.Equal(t, HTTPVersionAuto, CodecForVersions(HTTPVersion1, HTTPVersion2))
	assert.Equal(t, HTTPVersion1, CodecForVersions(HTTPVersion1))
	assert.Equal(t, HTTPVersion2, CodecForVersions(HTTPVersion2))
}

func TestProtoNamesForVersions(t *testing.T) {
	assert.Equal(t, []string{"h2", "http/1.1"}, ProtoNamesForVersions())
	assert.Equal(t, []string{"h2", "http/1.1"}, ProtoNamesForVersions(HTTPVersionAuto))
	assert.Equal(t, []string{"http/1.1"}, ProtoNamesForVersions(HTTPVersion1))
	assert.Equal(t, []string{"h2"}, ProtoNamesForVersions(HTTPVersion2))
	assert.Equal(t, []string(nil), ProtoNamesForVersions(HTTPVersion3))
	assert.Equal(t, []string{"h2", "http/1.1"}, ProtoNamesForVersions(HTTPVersion1, HTTPVersion2))
}

func TestListener(t *testing.T) {
	envoygen := NewEnvoyGen(EnvoyGenOpt{
		XDSClusterName: DefaultXDSClusterName,
	})

	tests := map[string]struct {
		name, address                 string
		port                          int
		perConnectionBufferLimitBytes *uint32
		lf                            []*envoy_config_listener_v3.ListenerFilter
		f                             []*envoy_config_listener_v3.Filter
		want                          *envoy_config_listener_v3.Listener
	}{
		"insecure listener": {
			name:    "http",
			address: "0.0.0.0",
			port:    9000,
			f: []*envoy_config_listener_v3.Filter{
				envoygen.HTTPConnectionManager("http", FileAccessLogEnvoy("/dev/null", "", nil, contour_v1alpha1.LogLevelInfo), 0),
			},
			want: &envoy_config_listener_v3.Listener{
				Name:    "http",
				Address: SocketAddress("0.0.0.0", 9000),
				FilterChains: FilterChains(
					envoygen.HTTPConnectionManager("http", FileAccessLogEnvoy("/dev/null", "", nil, contour_v1alpha1.LogLevelInfo), 0),
				),
				SocketOptions: NewSocketOptions().TCPKeepalive().Build(),
			},
		},
		"insecure listener w/ proxy": {
			name:    "http-proxy",
			address: "0.0.0.0",
			port:    9000,
			lf: []*envoy_config_listener_v3.ListenerFilter{
				ProxyProtocol(),
			},
			f: []*envoy_config_listener_v3.Filter{
				envoygen.HTTPConnectionManager("http-proxy", FileAccessLogEnvoy("/dev/null", "", nil, contour_v1alpha1.LogLevelInfo), 0),
			},
			want: &envoy_config_listener_v3.Listener{
				Name:    "http-proxy",
				Address: SocketAddress("0.0.0.0", 9000),
				ListenerFilters: ListenerFilters(
					ProxyProtocol(),
				),
				FilterChains: FilterChains(
					envoygen.HTTPConnectionManager("http-proxy", FileAccessLogEnvoy("/dev/null", "", nil, contour_v1alpha1.LogLevelInfo), 0),
				),
				SocketOptions: NewSocketOptions().TCPKeepalive().Build(),
			},
		},
		"secure listener": {
			name:    "https",
			address: "0.0.0.0",
			port:    9000,
			lf: ListenerFilters(
				TLSInspector(),
			),
			want: &envoy_config_listener_v3.Listener{
				Name:    "https",
				Address: SocketAddress("0.0.0.0", 9000),
				ListenerFilters: ListenerFilters(
					TLSInspector(),
				),
				SocketOptions: NewSocketOptions().TCPKeepalive().Build(),
			},
		},
		"secure listener w/ proxy": {
			name:    "https-proxy",
			address: "0.0.0.0",
			port:    9000,
			lf: ListenerFilters(
				ProxyProtocol(),
				TLSInspector(),
			),
			want: &envoy_config_listener_v3.Listener{
				Name:    "https-proxy",
				Address: SocketAddress("0.0.0.0", 9000),
				ListenerFilters: ListenerFilters(
					ProxyProtocol(),
					TLSInspector(),
				),
				SocketOptions: NewSocketOptions().TCPKeepalive().Build(),
			},
		},
		"listener w/ connection buffer limits": {
			name:                          "http",
			address:                       "0.0.0.0",
			port:                          9000,
			perConnectionBufferLimitBytes: ptr.To(uint32(32768)),
			f: []*envoy_config_listener_v3.Filter{
				envoygen.HTTPConnectionManager("http", FileAccessLogEnvoy("/dev/null", "", nil, contour_v1alpha1.LogLevelInfo), 0),
			},
			want: &envoy_config_listener_v3.Listener{
				Name:                          "http",
				Address:                       SocketAddress("0.0.0.0", 9000),
				PerConnectionBufferLimitBytes: wrapperspb.UInt32(32768),
				FilterChains: FilterChains(
					envoygen.HTTPConnectionManager("http", FileAccessLogEnvoy("/dev/null", "", nil, contour_v1alpha1.LogLevelInfo), 0),
				),
				SocketOptions: NewSocketOptions().TCPKeepalive().Build(),
			},
		},
		"secure listener w/ connection buffer limits": {
			name:                          "https",
			address:                       "0.0.0.0",
			port:                          9000,
			perConnectionBufferLimitBytes: ptr.To(uint32(32768)),
			lf: ListenerFilters(
				TLSInspector(),
			),
			want: &envoy_config_listener_v3.Listener{
				Name:                          "https",
				Address:                       SocketAddress("0.0.0.0", 9000),
				PerConnectionBufferLimitBytes: wrapperspb.UInt32(32768),
				ListenerFilters: ListenerFilters(
					TLSInspector(),
				),
				SocketOptions: NewSocketOptions().TCPKeepalive().Build(),
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := Listener(tc.name, tc.address, tc.port, tc.perConnectionBufferLimitBytes, NewSocketOptions().TCPKeepalive(), tc.lf, tc.f...)
			protobuf.ExpectEqual(t, tc.want, got)
		})
	}
}

func TestSocketAddress(t *testing.T) {
	const (
		addr = "foo.example.com"
		port = 8123
	)

	got := SocketAddress(addr, port)
	want := &envoy_config_core_v3.Address{
		Address: &envoy_config_core_v3.Address_SocketAddress{
			SocketAddress: &envoy_config_core_v3.SocketAddress{
				Protocol: envoy_config_core_v3.SocketAddress_TCP,
				Address:  addr,
				PortSpecifier: &envoy_config_core_v3.SocketAddress_PortValue{
					PortValue: port,
				},
			},
		},
	}
	require.Equal(t, want, got)

	got = SocketAddress("::", port)
	want = &envoy_config_core_v3.Address{
		Address: &envoy_config_core_v3.Address_SocketAddress{
			SocketAddress: &envoy_config_core_v3.SocketAddress{
				Protocol:   envoy_config_core_v3.SocketAddress_TCP,
				Address:    "::",
				Ipv4Compat: true, // Set only for ipv6-any "::"
				PortSpecifier: &envoy_config_core_v3.SocketAddress_PortValue{
					PortValue: port,
				},
			},
		},
	}
	assert.Equal(t, want, got)
}

func TestDownstreamTLSContext(t *testing.T) {
	subjectNames := []string{"client-subject-name"}
	ca := []byte("client-ca-cert")
	crl := []byte("crl-data")

	serverSecret := &dag.Secret{
		Object: &core_v1.Secret{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      "tls-cert",
				Namespace: "default",
			},
			Data: map[string][]byte{
				core_v1.TLSCertKey:       []byte("cert"),
				core_v1.TLSPrivateKeyKey: []byte("key"),
			},
		},
	}

	cipherSuites := []string{
		"[ECDHE-ECDSA-AES128-GCM-SHA256|ECDHE-ECDSA-CHACHA20-POLY1305]",
		"ECDHE-ECDSA-AES256-GCM-SHA384",
	}

	tlsParams := &envoy_transport_socket_tls_v3.TlsParameters{
		TlsMinimumProtocolVersion: envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2,
		TlsMaximumProtocolVersion: envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3,
		CipherSuites:              cipherSuites,
	}

	tlsCertificateSdsSecretConfigs := []*envoy_transport_socket_tls_v3.SdsSecretConfig{{
		Name: envoy.Secretname(serverSecret),
		SdsConfig: &envoy_config_core_v3.ConfigSource{
			ResourceApiVersion: envoy_config_core_v3.ApiVersion_V3,
			ConfigSourceSpecifier: &envoy_config_core_v3.ConfigSource_ApiConfigSource{
				ApiConfigSource: &envoy_config_core_v3.ApiConfigSource{
					ApiType:             envoy_config_core_v3.ApiConfigSource_GRPC,
					TransportApiVersion: envoy_config_core_v3.ApiVersion_V3,
					GrpcServices: []*envoy_config_core_v3.GrpcService{{
						TargetSpecifier: &envoy_config_core_v3.GrpcService_EnvoyGrpc_{
							EnvoyGrpc: &envoy_config_core_v3.GrpcService_EnvoyGrpc{
								ClusterName: "contour",
								Authority:   "contour",
							},
						},
					}},
				},
			},
		},
	}}

	alpnProtocols := []string{"h2", "http/1.1"}
	validationContext := &envoy_transport_socket_tls_v3.CommonTlsContext_ValidationContext{
		ValidationContext: &envoy_transport_socket_tls_v3.CertificateValidationContext{
			TrustedCa: &envoy_config_core_v3.DataSource{
				Specifier: &envoy_config_core_v3.DataSource_InlineBytes{
					InlineBytes: ca,
				},
			},
		},
	}

	peerValidationContext := &dag.PeerValidationContext{
		CACertificates: []*dag.Secret{
			{
				Object: &core_v1.Secret{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "secret",
						Namespace: "default",
					},
					Data: map[string][]byte{
						dag.CACertificateKey: ca,
					},
				},
			},
		},
	}

	// Negative test case: downstream validation should not contain subjectname.
	peerValidationContextWithSubjectName := &dag.PeerValidationContext{
		CACertificates: []*dag.Secret{
			{
				Object: &core_v1.Secret{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "secret",
						Namespace: "default",
					},
					Data: map[string][]byte{
						dag.CACertificateKey: ca,
					},
				},
			},
		},
		SubjectNames: subjectNames,
	}

	peerValidationContextSkipClientCertValidation := &dag.PeerValidationContext{
		SkipClientCertValidation: true,
	}
	validationContextSkipVerify := &envoy_transport_socket_tls_v3.CommonTlsContext_ValidationContext{
		ValidationContext: &envoy_transport_socket_tls_v3.CertificateValidationContext{
			TrustChainVerification: envoy_transport_socket_tls_v3.CertificateValidationContext_ACCEPT_UNTRUSTED,
		},
	}
	peerValidationContextSkipClientCertValidationWithCA := &dag.PeerValidationContext{
		CACertificates: []*dag.Secret{
			{
				Object: &core_v1.Secret{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "secret",
						Namespace: "default",
					},
					Data: map[string][]byte{
						dag.CACertificateKey: ca,
					},
				},
			},
		},
		SkipClientCertValidation: true,
	}
	validationContextSkipVerifyWithCA := &envoy_transport_socket_tls_v3.CommonTlsContext_ValidationContext{
		ValidationContext: &envoy_transport_socket_tls_v3.CertificateValidationContext{
			TrustChainVerification: envoy_transport_socket_tls_v3.CertificateValidationContext_ACCEPT_UNTRUSTED,
			TrustedCa: &envoy_config_core_v3.DataSource{
				Specifier: &envoy_config_core_v3.DataSource_InlineBytes{
					InlineBytes: ca,
				},
			},
		},
	}
	peerValidationContextOptionalClientCertValidationWithCA := &dag.PeerValidationContext{
		CACertificates: []*dag.Secret{
			{
				Object: &core_v1.Secret{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "secret",
						Namespace: "default",
					},
					Data: map[string][]byte{
						dag.CACertificateKey: ca,
					},
				},
			},
		},
		OptionalClientCertificate: true,
	}
	peerValidationContextWithCRLCheck := &dag.PeerValidationContext{
		CACertificates: []*dag.Secret{
			{
				Object: &core_v1.Secret{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "secret",
						Namespace: "default",
					},
					Data: map[string][]byte{
						dag.CACertificateKey: ca,
					},
				},
			},
		},
		CRL: &dag.Secret{
			Object: &core_v1.Secret{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "crl",
					Namespace: "default",
				},
				Data: map[string][]byte{
					dag.CRLKey: crl,
				},
			},
		},
	}
	validationContextWithCRLCheck := &envoy_transport_socket_tls_v3.CommonTlsContext_ValidationContext{
		ValidationContext: &envoy_transport_socket_tls_v3.CertificateValidationContext{
			TrustedCa: &envoy_config_core_v3.DataSource{
				Specifier: &envoy_config_core_v3.DataSource_InlineBytes{
					InlineBytes: ca,
				},
			},
			Crl: &envoy_config_core_v3.DataSource{
				Specifier: &envoy_config_core_v3.DataSource_InlineBytes{
					InlineBytes: crl,
				},
			},
		},
	}

	peerValidationContextWithCRLCheckOnlyLeaf := &dag.PeerValidationContext{
		CACertificates: []*dag.Secret{
			{
				Object: &core_v1.Secret{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "secret",
						Namespace: "default",
					},
					Data: map[string][]byte{
						dag.CACertificateKey: ca,
					},
				},
			},
		},
		CRL: &dag.Secret{
			Object: &core_v1.Secret{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "crl",
					Namespace: "default",
				},
				Data: map[string][]byte{
					dag.CRLKey: crl,
				},
			},
		},
		OnlyVerifyLeafCertCrl: true,
	}
	validationContextWithCRLCheckOnlyLeaf := &envoy_transport_socket_tls_v3.CommonTlsContext_ValidationContext{
		ValidationContext: &envoy_transport_socket_tls_v3.CertificateValidationContext{
			TrustedCa: &envoy_config_core_v3.DataSource{
				Specifier: &envoy_config_core_v3.DataSource_InlineBytes{
					InlineBytes: ca,
				},
			},
			Crl: &envoy_config_core_v3.DataSource{
				Specifier: &envoy_config_core_v3.DataSource_InlineBytes{
					InlineBytes: crl,
				},
			},
			OnlyVerifyLeafCertCrl: true,
		},
	}

	envoyGen := NewEnvoyGen(EnvoyGenOpt{
		XDSClusterName: DefaultXDSClusterName,
	})

	tests := map[string]struct {
		got  *envoy_transport_socket_tls_v3.DownstreamTlsContext
		want *envoy_transport_socket_tls_v3.DownstreamTlsContext
	}{
		"TLS context without client authentication": {
			envoyGen.DownstreamTLSContext(serverSecret, envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2, envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3, cipherSuites, nil, "h2", "http/1.1"),
			&envoy_transport_socket_tls_v3.DownstreamTlsContext{
				CommonTlsContext: &envoy_transport_socket_tls_v3.CommonTlsContext{
					TlsParams:                      tlsParams,
					TlsCertificateSdsSecretConfigs: tlsCertificateSdsSecretConfigs,
					AlpnProtocols:                  alpnProtocols,
				},
			},
		},
		"TLS context with client authentication": {
			envoyGen.DownstreamTLSContext(serverSecret, envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2, envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3, cipherSuites, peerValidationContext, "h2", "http/1.1"),
			&envoy_transport_socket_tls_v3.DownstreamTlsContext{
				CommonTlsContext: &envoy_transport_socket_tls_v3.CommonTlsContext{
					TlsParams:                      tlsParams,
					TlsCertificateSdsSecretConfigs: tlsCertificateSdsSecretConfigs,
					AlpnProtocols:                  alpnProtocols,
					ValidationContextType:          validationContext,
				},
				RequireClientCertificate: wrapperspb.Bool(true),
			},
		},
		"Downstream validation shall not support subjectName validation": {
			envoyGen.DownstreamTLSContext(serverSecret, envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2, envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3, cipherSuites, peerValidationContextWithSubjectName, "h2", "http/1.1"),
			&envoy_transport_socket_tls_v3.DownstreamTlsContext{
				CommonTlsContext: &envoy_transport_socket_tls_v3.CommonTlsContext{
					TlsParams:                      tlsParams,
					TlsCertificateSdsSecretConfigs: tlsCertificateSdsSecretConfigs,
					AlpnProtocols:                  alpnProtocols,
					ValidationContextType:          validationContext,
				},
				RequireClientCertificate: wrapperspb.Bool(true),
			},
		},
		"skip client cert validation": {
			envoyGen.DownstreamTLSContext(serverSecret, envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2, envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3, cipherSuites, peerValidationContextSkipClientCertValidation, "h2", "http/1.1"),
			&envoy_transport_socket_tls_v3.DownstreamTlsContext{
				CommonTlsContext: &envoy_transport_socket_tls_v3.CommonTlsContext{
					TlsParams:                      tlsParams,
					TlsCertificateSdsSecretConfigs: tlsCertificateSdsSecretConfigs,
					AlpnProtocols:                  alpnProtocols,
					ValidationContextType:          validationContextSkipVerify,
				},
				RequireClientCertificate: wrapperspb.Bool(true),
			},
		},
		"skip client cert validation with ca": {
			envoyGen.DownstreamTLSContext(serverSecret, envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2, envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3, cipherSuites, peerValidationContextSkipClientCertValidationWithCA, "h2", "http/1.1"),
			&envoy_transport_socket_tls_v3.DownstreamTlsContext{
				CommonTlsContext: &envoy_transport_socket_tls_v3.CommonTlsContext{
					TlsParams:                      tlsParams,
					TlsCertificateSdsSecretConfigs: tlsCertificateSdsSecretConfigs,
					AlpnProtocols:                  alpnProtocols,
					ValidationContextType:          validationContextSkipVerifyWithCA,
				},
				RequireClientCertificate: wrapperspb.Bool(true),
			},
		},
		"optional client cert validation with ca": {
			envoyGen.DownstreamTLSContext(serverSecret, envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2, envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3, cipherSuites, peerValidationContextOptionalClientCertValidationWithCA, "h2", "http/1.1"),
			&envoy_transport_socket_tls_v3.DownstreamTlsContext{
				CommonTlsContext: &envoy_transport_socket_tls_v3.CommonTlsContext{
					TlsParams:                      tlsParams,
					TlsCertificateSdsSecretConfigs: tlsCertificateSdsSecretConfigs,
					AlpnProtocols:                  alpnProtocols,
					ValidationContextType:          validationContext,
				},
				RequireClientCertificate: wrapperspb.Bool(false),
			},
		},
		"Downstream validation with CRL check": {
			envoyGen.DownstreamTLSContext(serverSecret, envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2, envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3, cipherSuites, peerValidationContextWithCRLCheck, "h2", "http/1.1"),
			&envoy_transport_socket_tls_v3.DownstreamTlsContext{
				CommonTlsContext: &envoy_transport_socket_tls_v3.CommonTlsContext{
					TlsParams:                      tlsParams,
					TlsCertificateSdsSecretConfigs: tlsCertificateSdsSecretConfigs,
					AlpnProtocols:                  alpnProtocols,
					ValidationContextType:          validationContextWithCRLCheck,
				},
				RequireClientCertificate: wrapperspb.Bool(true),
			},
		},
		"Downstream validation with CRL check but only for leaf-certificate": {
			envoyGen.DownstreamTLSContext(serverSecret, envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2, envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3, cipherSuites, peerValidationContextWithCRLCheckOnlyLeaf, "h2", "http/1.1"),
			&envoy_transport_socket_tls_v3.DownstreamTlsContext{
				CommonTlsContext: &envoy_transport_socket_tls_v3.CommonTlsContext{
					TlsParams:                      tlsParams,
					TlsCertificateSdsSecretConfigs: tlsCertificateSdsSecretConfigs,
					AlpnProtocols:                  alpnProtocols,
					ValidationContextType:          validationContextWithCRLCheckOnlyLeaf,
				},
				RequireClientCertificate: wrapperspb.Bool(true),
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			protobuf.ExpectEqual(t, tc.want, tc.got)
		})
	}
}

func TestHTTPConnectionManager(t *testing.T) {
	envoyGen := NewEnvoyGen(EnvoyGenOpt{
		XDSClusterName: DefaultXDSClusterName,
	})
	defaultHTTPFilters := []*envoy_filter_network_http_connection_manager_v3.HttpFilter{
		{
			Name: CompressorFilterName,
			ConfigType: &envoy_filter_network_http_connection_manager_v3.HttpFilter_TypedConfig{
				TypedConfig: protobuf.MustMarshalAny(&envoy_filter_http_compressor_v3.Compressor{
					CompressorLibrary: &envoy_config_core_v3.TypedExtensionConfig{
						Name: "gzip",
						TypedConfig: protobuf.MustMarshalAny(
							&envoy_compression_gzip_compressor_v3.Gzip{},
						),
					},
					ResponseDirectionConfig: &envoy_filter_http_compressor_v3.Compressor_ResponseDirectionConfig{
						CommonConfig: &envoy_filter_http_compressor_v3.Compressor_CommonDirectionConfig{
							ContentType: compressorContentTypes,
						},
					},
				}),
			},
		}, {
			Name: GRPCWebFilterName,
			ConfigType: &envoy_filter_network_http_connection_manager_v3.HttpFilter_TypedConfig{
				TypedConfig: protobuf.MustMarshalAny(&envoy_filter_http_grpc_web_v3.GrpcWeb{}),
			},
		}, {
			Name: GRPCStatsFilterName,
			ConfigType: &envoy_filter_network_http_connection_manager_v3.HttpFilter_TypedConfig{
				TypedConfig: protobuf.MustMarshalAny(
					&envoy_filter_http_grpc_stats_v3.FilterConfig{
						EmitFilterState:     true,
						EnableUpstreamStats: true,
					},
				),
			},
		}, {
			Name: CORSFilterName,
			ConfigType: &envoy_filter_network_http_connection_manager_v3.HttpFilter_TypedConfig{
				TypedConfig: protobuf.MustMarshalAny(&envoy_filter_http_cors_v3.Cors{}),
			},
		}, {
			Name: LocalRateLimitFilterName,
			ConfigType: &envoy_filter_network_http_connection_manager_v3.HttpFilter_TypedConfig{
				TypedConfig: protobuf.MustMarshalAny(
					&envoy_filter_http_local_ratelimit_v3.LocalRateLimit{
						StatPrefix: "http",
					},
				),
			},
		}, {
			Name: LuaFilterName,
			ConfigType: &envoy_filter_network_http_connection_manager_v3.HttpFilter_TypedConfig{
				TypedConfig: protobuf.MustMarshalAny(&envoy_filter_http_lua_v3.Lua{
					DefaultSourceCode: &envoy_config_core_v3.DataSource{
						Specifier: &envoy_config_core_v3.DataSource_InlineString{
							InlineString: "-- Placeholder for per-Route or per-Cluster overrides.",
						},
					},
				}),
			},
		}, {
			Name: RBACFilterName,
			ConfigType: &envoy_filter_network_http_connection_manager_v3.HttpFilter_TypedConfig{
				TypedConfig: protobuf.MustMarshalAny(&envoy_filter_http_rbac_v3.RBAC{}),
			},
		}, {
			Name: "router",
			ConfigType: &envoy_filter_network_http_connection_manager_v3.HttpFilter_TypedConfig{
				TypedConfig: protobuf.MustMarshalAny(&envoy_filter_http_router_v3.Router{}),
			},
		},
	}

	tests := map[string]struct {
		routename                     string
		accesslogger                  []*envoy_config_accesslog_v3.AccessLog
		requestTimeout                timeout.Setting
		connectionIdleTimeout         timeout.Setting
		streamIdleTimeout             timeout.Setting
		maxConnectionDuration         timeout.Setting
		delayedCloseTimeout           timeout.Setting
		connectionShutdownGracePeriod timeout.Setting
		allowChunkedLength            bool
		mergeSlashes                  bool
		serverHeaderTranformation     contour_v1alpha1.ServerHeaderTransformationType
		forwardClientCertificate      *dag.ClientCertificateDetails
		xffNumTrustedHops             uint32
		stripTrailingHostDot          bool
		maxRequestsPerConnection      *uint32
		http2MaxConcurrentStreams     *uint32
		want                          *envoy_config_listener_v3.Filter
	}{
		"default": {
			routename:    "default/kuard",
			accesslogger: FileAccessLogEnvoy("/dev/stdout", "", nil, contour_v1alpha1.LogLevelInfo),
			want: &envoy_config_listener_v3.Filter{
				Name: wellknown.HTTPConnectionManager,
				ConfigType: &envoy_config_listener_v3.Filter_TypedConfig{
					TypedConfig: protobuf.MustMarshalAny(&envoy_filter_network_http_connection_manager_v3.HttpConnectionManager{
						StatPrefix: "default/kuard",
						RouteSpecifier: &envoy_filter_network_http_connection_manager_v3.HttpConnectionManager_Rds{
							Rds: &envoy_filter_network_http_connection_manager_v3.Rds{
								RouteConfigName: "default/kuard",
								ConfigSource: &envoy_config_core_v3.ConfigSource{
									ResourceApiVersion: envoy_config_core_v3.ApiVersion_V3,
									ConfigSourceSpecifier: &envoy_config_core_v3.ConfigSource_ApiConfigSource{
										ApiConfigSource: &envoy_config_core_v3.ApiConfigSource{
											ApiType:             envoy_config_core_v3.ApiConfigSource_GRPC,
											TransportApiVersion: envoy_config_core_v3.ApiVersion_V3,
											GrpcServices: []*envoy_config_core_v3.GrpcService{{
												TargetSpecifier: &envoy_config_core_v3.GrpcService_EnvoyGrpc_{
													EnvoyGrpc: &envoy_config_core_v3.GrpcService_EnvoyGrpc{
														ClusterName: "contour",
														Authority:   "contour",
													},
												},
											}},
										},
									},
								},
							},
						},
						HttpFilters: defaultHTTPFilters,
						HttpProtocolOptions: &envoy_config_core_v3.Http1ProtocolOptions{
							// Enable support for HTTP/1.0 requests that carry
							// a Host: header. See #537.
							AcceptHttp_10: true,
						},
						CommonHttpProtocolOptions: &envoy_config_core_v3.HttpProtocolOptions{},
						AccessLog:                 FileAccessLogEnvoy("/dev/stdout", "", nil, contour_v1alpha1.LogLevelInfo),
						UseRemoteAddress:          wrapperspb.Bool(true),
						NormalizePath:             wrapperspb.Bool(true),
						PreserveExternalRequestId: true,
						MergeSlashes:              false,
					}),
				},
			},
		},
		"request timeout of 10s": {
			routename:      "default/kuard",
			accesslogger:   FileAccessLogEnvoy("/dev/stdout", "", nil, contour_v1alpha1.LogLevelInfo),
			requestTimeout: timeout.DurationSetting(10 * time.Second),
			want: &envoy_config_listener_v3.Filter{
				Name: wellknown.HTTPConnectionManager,
				ConfigType: &envoy_config_listener_v3.Filter_TypedConfig{
					TypedConfig: protobuf.MustMarshalAny(&envoy_filter_network_http_connection_manager_v3.HttpConnectionManager{
						StatPrefix: "default/kuard",
						RouteSpecifier: &envoy_filter_network_http_connection_manager_v3.HttpConnectionManager_Rds{
							Rds: &envoy_filter_network_http_connection_manager_v3.Rds{
								RouteConfigName: "default/kuard",
								ConfigSource: &envoy_config_core_v3.ConfigSource{
									ResourceApiVersion: envoy_config_core_v3.ApiVersion_V3,
									ConfigSourceSpecifier: &envoy_config_core_v3.ConfigSource_ApiConfigSource{
										ApiConfigSource: &envoy_config_core_v3.ApiConfigSource{
											ApiType:             envoy_config_core_v3.ApiConfigSource_GRPC,
											TransportApiVersion: envoy_config_core_v3.ApiVersion_V3,
											GrpcServices: []*envoy_config_core_v3.GrpcService{{
												TargetSpecifier: &envoy_config_core_v3.GrpcService_EnvoyGrpc_{
													EnvoyGrpc: &envoy_config_core_v3.GrpcService_EnvoyGrpc{
														ClusterName: "contour",
														Authority:   "contour",
													},
												},
											}},
										},
									},
								},
							},
						},
						HttpFilters: defaultHTTPFilters,
						HttpProtocolOptions: &envoy_config_core_v3.Http1ProtocolOptions{
							// Enable support for HTTP/1.0 requests that carry
							// a Host: header. See #537.
							AcceptHttp_10: true,
						},
						CommonHttpProtocolOptions: &envoy_config_core_v3.HttpProtocolOptions{},
						AccessLog:                 FileAccessLogEnvoy("/dev/stdout", "", nil, contour_v1alpha1.LogLevelInfo),
						UseRemoteAddress:          wrapperspb.Bool(true),
						NormalizePath:             wrapperspb.Bool(true),
						RequestTimeout:            durationpb.New(10 * time.Second),
						PreserveExternalRequestId: true,
						MergeSlashes:              false,
					}),
				},
			},
		},
		"connection idle timeout of 90s": {
			routename:             "default/kuard",
			accesslogger:          FileAccessLogEnvoy("/dev/stdout", "", nil, contour_v1alpha1.LogLevelInfo),
			connectionIdleTimeout: timeout.DurationSetting(90 * time.Second),
			want: &envoy_config_listener_v3.Filter{
				Name: wellknown.HTTPConnectionManager,
				ConfigType: &envoy_config_listener_v3.Filter_TypedConfig{
					TypedConfig: protobuf.MustMarshalAny(&envoy_filter_network_http_connection_manager_v3.HttpConnectionManager{
						StatPrefix: "default/kuard",
						RouteSpecifier: &envoy_filter_network_http_connection_manager_v3.HttpConnectionManager_Rds{
							Rds: &envoy_filter_network_http_connection_manager_v3.Rds{
								RouteConfigName: "default/kuard",
								ConfigSource: &envoy_config_core_v3.ConfigSource{
									ResourceApiVersion: envoy_config_core_v3.ApiVersion_V3,
									ConfigSourceSpecifier: &envoy_config_core_v3.ConfigSource_ApiConfigSource{
										ApiConfigSource: &envoy_config_core_v3.ApiConfigSource{
											ApiType:             envoy_config_core_v3.ApiConfigSource_GRPC,
											TransportApiVersion: envoy_config_core_v3.ApiVersion_V3,
											GrpcServices: []*envoy_config_core_v3.GrpcService{{
												TargetSpecifier: &envoy_config_core_v3.GrpcService_EnvoyGrpc_{
													EnvoyGrpc: &envoy_config_core_v3.GrpcService_EnvoyGrpc{
														ClusterName: "contour",
														Authority:   "contour",
													},
												},
											}},
										},
									},
								},
							},
						},
						HttpFilters: defaultHTTPFilters,
						HttpProtocolOptions: &envoy_config_core_v3.Http1ProtocolOptions{
							// Enable support for HTTP/1.0 requests that carry
							// a Host: header. See #537.
							AcceptHttp_10: true,
						},
						CommonHttpProtocolOptions: &envoy_config_core_v3.HttpProtocolOptions{
							IdleTimeout: durationpb.New(90 * time.Second),
						},
						AccessLog:                 FileAccessLogEnvoy("/dev/stdout", "", nil, contour_v1alpha1.LogLevelInfo),
						UseRemoteAddress:          wrapperspb.Bool(true),
						NormalizePath:             wrapperspb.Bool(true),
						PreserveExternalRequestId: true,
						MergeSlashes:              false,
					}),
				},
			},
		},
		"stream idle timeout of 90s": {
			routename:         "default/kuard",
			accesslogger:      FileAccessLogEnvoy("/dev/stdout", "", nil, contour_v1alpha1.LogLevelInfo),
			streamIdleTimeout: timeout.DurationSetting(90 * time.Second),
			want: &envoy_config_listener_v3.Filter{
				Name: wellknown.HTTPConnectionManager,
				ConfigType: &envoy_config_listener_v3.Filter_TypedConfig{
					TypedConfig: protobuf.MustMarshalAny(&envoy_filter_network_http_connection_manager_v3.HttpConnectionManager{
						StatPrefix: "default/kuard",
						RouteSpecifier: &envoy_filter_network_http_connection_manager_v3.HttpConnectionManager_Rds{
							Rds: &envoy_filter_network_http_connection_manager_v3.Rds{
								RouteConfigName: "default/kuard",
								ConfigSource: &envoy_config_core_v3.ConfigSource{
									ResourceApiVersion: envoy_config_core_v3.ApiVersion_V3,
									ConfigSourceSpecifier: &envoy_config_core_v3.ConfigSource_ApiConfigSource{
										ApiConfigSource: &envoy_config_core_v3.ApiConfigSource{
											ApiType:             envoy_config_core_v3.ApiConfigSource_GRPC,
											TransportApiVersion: envoy_config_core_v3.ApiVersion_V3,
											GrpcServices: []*envoy_config_core_v3.GrpcService{{
												TargetSpecifier: &envoy_config_core_v3.GrpcService_EnvoyGrpc_{
													EnvoyGrpc: &envoy_config_core_v3.GrpcService_EnvoyGrpc{
														ClusterName: "contour",
														Authority:   "contour",
													},
												},
											}},
										},
									},
								},
							},
						},
						HttpFilters: defaultHTTPFilters,
						HttpProtocolOptions: &envoy_config_core_v3.Http1ProtocolOptions{
							// Enable support for HTTP/1.0 requests that carry
							// a Host: header. See #537.
							AcceptHttp_10: true,
						},
						CommonHttpProtocolOptions: &envoy_config_core_v3.HttpProtocolOptions{},
						AccessLog:                 FileAccessLogEnvoy("/dev/stdout", "", nil, contour_v1alpha1.LogLevelInfo),
						UseRemoteAddress:          wrapperspb.Bool(true),
						NormalizePath:             wrapperspb.Bool(true),
						PreserveExternalRequestId: true,
						MergeSlashes:              false,
						StreamIdleTimeout:         durationpb.New(90 * time.Second),
					}),
				},
			},
		},
		"max connection duration of 90s": {
			routename:             "default/kuard",
			accesslogger:          FileAccessLogEnvoy("/dev/stdout", "", nil, contour_v1alpha1.LogLevelInfo),
			maxConnectionDuration: timeout.DurationSetting(90 * time.Second),
			want: &envoy_config_listener_v3.Filter{
				Name: wellknown.HTTPConnectionManager,
				ConfigType: &envoy_config_listener_v3.Filter_TypedConfig{
					TypedConfig: protobuf.MustMarshalAny(&envoy_filter_network_http_connection_manager_v3.HttpConnectionManager{
						StatPrefix: "default/kuard",
						RouteSpecifier: &envoy_filter_network_http_connection_manager_v3.HttpConnectionManager_Rds{
							Rds: &envoy_filter_network_http_connection_manager_v3.Rds{
								RouteConfigName: "default/kuard",
								ConfigSource: &envoy_config_core_v3.ConfigSource{
									ResourceApiVersion: envoy_config_core_v3.ApiVersion_V3,
									ConfigSourceSpecifier: &envoy_config_core_v3.ConfigSource_ApiConfigSource{
										ApiConfigSource: &envoy_config_core_v3.ApiConfigSource{
											ApiType:             envoy_config_core_v3.ApiConfigSource_GRPC,
											TransportApiVersion: envoy_config_core_v3.ApiVersion_V3,
											GrpcServices: []*envoy_config_core_v3.GrpcService{{
												TargetSpecifier: &envoy_config_core_v3.GrpcService_EnvoyGrpc_{
													EnvoyGrpc: &envoy_config_core_v3.GrpcService_EnvoyGrpc{
														ClusterName: "contour",
														Authority:   "contour",
													},
												},
											}},
										},
									},
								},
							},
						},
						HttpFilters: defaultHTTPFilters,
						HttpProtocolOptions: &envoy_config_core_v3.Http1ProtocolOptions{
							// Enable support for HTTP/1.0 requests that carry
							// a Host: header. See #537.
							AcceptHttp_10: true,
						},
						CommonHttpProtocolOptions: &envoy_config_core_v3.HttpProtocolOptions{
							MaxConnectionDuration: durationpb.New(90 * time.Second),
						},
						AccessLog:                 FileAccessLogEnvoy("/dev/stdout", "", nil, contour_v1alpha1.LogLevelInfo),
						UseRemoteAddress:          wrapperspb.Bool(true),
						NormalizePath:             wrapperspb.Bool(true),
						PreserveExternalRequestId: true,
						MergeSlashes:              false,
					}),
				},
			},
		},
		"when max connection duration is disabled, it's omitted": {
			routename:             "default/kuard",
			accesslogger:          FileAccessLogEnvoy("/dev/stdout", "", nil, contour_v1alpha1.LogLevelInfo),
			maxConnectionDuration: timeout.DisabledSetting(),
			want: &envoy_config_listener_v3.Filter{
				Name: wellknown.HTTPConnectionManager,
				ConfigType: &envoy_config_listener_v3.Filter_TypedConfig{
					TypedConfig: protobuf.MustMarshalAny(&envoy_filter_network_http_connection_manager_v3.HttpConnectionManager{
						StatPrefix: "default/kuard",
						RouteSpecifier: &envoy_filter_network_http_connection_manager_v3.HttpConnectionManager_Rds{
							Rds: &envoy_filter_network_http_connection_manager_v3.Rds{
								RouteConfigName: "default/kuard",
								ConfigSource: &envoy_config_core_v3.ConfigSource{
									ResourceApiVersion: envoy_config_core_v3.ApiVersion_V3,
									ConfigSourceSpecifier: &envoy_config_core_v3.ConfigSource_ApiConfigSource{
										ApiConfigSource: &envoy_config_core_v3.ApiConfigSource{
											ApiType:             envoy_config_core_v3.ApiConfigSource_GRPC,
											TransportApiVersion: envoy_config_core_v3.ApiVersion_V3,
											GrpcServices: []*envoy_config_core_v3.GrpcService{{
												TargetSpecifier: &envoy_config_core_v3.GrpcService_EnvoyGrpc_{
													EnvoyGrpc: &envoy_config_core_v3.GrpcService_EnvoyGrpc{
														ClusterName: "contour",
														Authority:   "contour",
													},
												},
											}},
										},
									},
								},
							},
						},
						HttpFilters: defaultHTTPFilters,
						HttpProtocolOptions: &envoy_config_core_v3.Http1ProtocolOptions{
							// Enable support for HTTP/1.0 requests that carry
							// a Host: header. See #537.
							AcceptHttp_10: true,
						},
						CommonHttpProtocolOptions: &envoy_config_core_v3.HttpProtocolOptions{},
						AccessLog:                 FileAccessLogEnvoy("/dev/stdout", "", nil, contour_v1alpha1.LogLevelInfo),
						UseRemoteAddress:          wrapperspb.Bool(true),
						NormalizePath:             wrapperspb.Bool(true),
						PreserveExternalRequestId: true,
						MergeSlashes:              false,
					}),
				},
			},
		},
		"delayed close timeout of 90s": {
			routename:           "default/kuard",
			accesslogger:        FileAccessLogEnvoy("/dev/stdout", "", nil, contour_v1alpha1.LogLevelInfo),
			delayedCloseTimeout: timeout.DurationSetting(90 * time.Second),
			want: &envoy_config_listener_v3.Filter{
				Name: wellknown.HTTPConnectionManager,
				ConfigType: &envoy_config_listener_v3.Filter_TypedConfig{
					TypedConfig: protobuf.MustMarshalAny(&envoy_filter_network_http_connection_manager_v3.HttpConnectionManager{
						StatPrefix: "default/kuard",
						RouteSpecifier: &envoy_filter_network_http_connection_manager_v3.HttpConnectionManager_Rds{
							Rds: &envoy_filter_network_http_connection_manager_v3.Rds{
								RouteConfigName: "default/kuard",
								ConfigSource: &envoy_config_core_v3.ConfigSource{
									ResourceApiVersion: envoy_config_core_v3.ApiVersion_V3,
									ConfigSourceSpecifier: &envoy_config_core_v3.ConfigSource_ApiConfigSource{
										ApiConfigSource: &envoy_config_core_v3.ApiConfigSource{
											ApiType:             envoy_config_core_v3.ApiConfigSource_GRPC,
											TransportApiVersion: envoy_config_core_v3.ApiVersion_V3,
											GrpcServices: []*envoy_config_core_v3.GrpcService{{
												TargetSpecifier: &envoy_config_core_v3.GrpcService_EnvoyGrpc_{
													EnvoyGrpc: &envoy_config_core_v3.GrpcService_EnvoyGrpc{
														ClusterName: "contour",
														Authority:   "contour",
													},
												},
											}},
										},
									},
								},
							},
						},
						HttpFilters: defaultHTTPFilters,
						HttpProtocolOptions: &envoy_config_core_v3.Http1ProtocolOptions{
							// Enable support for HTTP/1.0 requests that carry
							// a Host: header. See #537.
							AcceptHttp_10: true,
						},
						CommonHttpProtocolOptions: &envoy_config_core_v3.HttpProtocolOptions{},
						AccessLog:                 FileAccessLogEnvoy("/dev/stdout", "", nil, contour_v1alpha1.LogLevelInfo),
						UseRemoteAddress:          wrapperspb.Bool(true),
						NormalizePath:             wrapperspb.Bool(true),
						PreserveExternalRequestId: true,
						MergeSlashes:              false,
						DelayedCloseTimeout:       durationpb.New(90 * time.Second),
					}),
				},
			},
		},
		"drain timeout of 90s": {
			routename:                     "default/kuard",
			accesslogger:                  FileAccessLogEnvoy("/dev/stdout", "", nil, contour_v1alpha1.LogLevelInfo),
			connectionShutdownGracePeriod: timeout.DurationSetting(90 * time.Second),
			want: &envoy_config_listener_v3.Filter{
				Name: wellknown.HTTPConnectionManager,
				ConfigType: &envoy_config_listener_v3.Filter_TypedConfig{
					TypedConfig: protobuf.MustMarshalAny(&envoy_filter_network_http_connection_manager_v3.HttpConnectionManager{
						StatPrefix: "default/kuard",
						RouteSpecifier: &envoy_filter_network_http_connection_manager_v3.HttpConnectionManager_Rds{
							Rds: &envoy_filter_network_http_connection_manager_v3.Rds{
								RouteConfigName: "default/kuard",
								ConfigSource: &envoy_config_core_v3.ConfigSource{
									ResourceApiVersion: envoy_config_core_v3.ApiVersion_V3,
									ConfigSourceSpecifier: &envoy_config_core_v3.ConfigSource_ApiConfigSource{
										ApiConfigSource: &envoy_config_core_v3.ApiConfigSource{
											ApiType:             envoy_config_core_v3.ApiConfigSource_GRPC,
											TransportApiVersion: envoy_config_core_v3.ApiVersion_V3,
											GrpcServices: []*envoy_config_core_v3.GrpcService{{
												TargetSpecifier: &envoy_config_core_v3.GrpcService_EnvoyGrpc_{
													EnvoyGrpc: &envoy_config_core_v3.GrpcService_EnvoyGrpc{
														ClusterName: "contour",
														Authority:   "contour",
													},
												},
											}},
										},
									},
								},
							},
						},
						HttpFilters: defaultHTTPFilters,
						HttpProtocolOptions: &envoy_config_core_v3.Http1ProtocolOptions{
							// Enable support for HTTP/1.0 requests that carry
							// a Host: header. See #537.
							AcceptHttp_10: true,
						},
						CommonHttpProtocolOptions: &envoy_config_core_v3.HttpProtocolOptions{},
						AccessLog:                 FileAccessLogEnvoy("/dev/stdout", "", nil, contour_v1alpha1.LogLevelInfo),
						UseRemoteAddress:          wrapperspb.Bool(true),
						NormalizePath:             wrapperspb.Bool(true),
						PreserveExternalRequestId: true,
						MergeSlashes:              false,
						DrainTimeout:              durationpb.New(90 * time.Second),
					}),
				},
			},
		},
		"enable allow_chunked_length": {
			routename:                     "default/kuard",
			accesslogger:                  FileAccessLogEnvoy("/dev/stdout", "", nil, contour_v1alpha1.LogLevelInfo),
			connectionShutdownGracePeriod: timeout.DurationSetting(90 * time.Second),
			allowChunkedLength:            true,
			want: &envoy_config_listener_v3.Filter{
				Name: wellknown.HTTPConnectionManager,
				ConfigType: &envoy_config_listener_v3.Filter_TypedConfig{
					TypedConfig: protobuf.MustMarshalAny(&envoy_filter_network_http_connection_manager_v3.HttpConnectionManager{
						StatPrefix: "default/kuard",
						RouteSpecifier: &envoy_filter_network_http_connection_manager_v3.HttpConnectionManager_Rds{
							Rds: &envoy_filter_network_http_connection_manager_v3.Rds{
								RouteConfigName: "default/kuard",
								ConfigSource: &envoy_config_core_v3.ConfigSource{
									ResourceApiVersion: envoy_config_core_v3.ApiVersion_V3,
									ConfigSourceSpecifier: &envoy_config_core_v3.ConfigSource_ApiConfigSource{
										ApiConfigSource: &envoy_config_core_v3.ApiConfigSource{
											ApiType:             envoy_config_core_v3.ApiConfigSource_GRPC,
											TransportApiVersion: envoy_config_core_v3.ApiVersion_V3,
											GrpcServices: []*envoy_config_core_v3.GrpcService{{
												TargetSpecifier: &envoy_config_core_v3.GrpcService_EnvoyGrpc_{
													EnvoyGrpc: &envoy_config_core_v3.GrpcService_EnvoyGrpc{
														ClusterName: "contour",
														Authority:   "contour",
													},
												},
											}},
										},
									},
								},
							},
						},
						HttpFilters: defaultHTTPFilters,
						HttpProtocolOptions: &envoy_config_core_v3.Http1ProtocolOptions{
							// Enable support for HTTP/1.0 requests that carry
							// a Host: header. See #537.
							AcceptHttp_10:      true,
							AllowChunkedLength: true,
						},
						CommonHttpProtocolOptions: &envoy_config_core_v3.HttpProtocolOptions{},
						AccessLog:                 FileAccessLogEnvoy("/dev/stdout", "", nil, contour_v1alpha1.LogLevelInfo),
						UseRemoteAddress:          wrapperspb.Bool(true),
						NormalizePath:             wrapperspb.Bool(true),
						PreserveExternalRequestId: true,
						MergeSlashes:              false,
						DrainTimeout:              durationpb.New(90 * time.Second),
					}),
				},
			},
		},
		"enable merge slashes": {
			routename:    "default/kuard",
			accesslogger: FileAccessLogEnvoy("/dev/stdout", "", nil, contour_v1alpha1.LogLevelInfo),
			mergeSlashes: true,
			want: &envoy_config_listener_v3.Filter{
				Name: wellknown.HTTPConnectionManager,
				ConfigType: &envoy_config_listener_v3.Filter_TypedConfig{
					TypedConfig: protobuf.MustMarshalAny(&envoy_filter_network_http_connection_manager_v3.HttpConnectionManager{
						StatPrefix: "default/kuard",
						RouteSpecifier: &envoy_filter_network_http_connection_manager_v3.HttpConnectionManager_Rds{
							Rds: &envoy_filter_network_http_connection_manager_v3.Rds{
								RouteConfigName: "default/kuard",
								ConfigSource: &envoy_config_core_v3.ConfigSource{
									ResourceApiVersion: envoy_config_core_v3.ApiVersion_V3,
									ConfigSourceSpecifier: &envoy_config_core_v3.ConfigSource_ApiConfigSource{
										ApiConfigSource: &envoy_config_core_v3.ApiConfigSource{
											ApiType:             envoy_config_core_v3.ApiConfigSource_GRPC,
											TransportApiVersion: envoy_config_core_v3.ApiVersion_V3,
											GrpcServices: []*envoy_config_core_v3.GrpcService{{
												TargetSpecifier: &envoy_config_core_v3.GrpcService_EnvoyGrpc_{
													EnvoyGrpc: &envoy_config_core_v3.GrpcService_EnvoyGrpc{
														ClusterName: "contour",
														Authority:   "contour",
													},
												},
											}},
										},
									},
								},
							},
						},
						HttpFilters: defaultHTTPFilters,
						HttpProtocolOptions: &envoy_config_core_v3.Http1ProtocolOptions{
							// Enable support for HTTP/1.0 requests that carry
							// a Host: header. See #537.
							AcceptHttp_10: true,
						},
						CommonHttpProtocolOptions: &envoy_config_core_v3.HttpProtocolOptions{},
						AccessLog:                 FileAccessLogEnvoy("/dev/stdout", "", nil, contour_v1alpha1.LogLevelInfo),
						UseRemoteAddress:          wrapperspb.Bool(true),
						NormalizePath:             wrapperspb.Bool(true),
						PreserveExternalRequestId: true,
						MergeSlashes:              true,
					}),
				},
			},
		},
		"server header transform set to pass through": {
			routename:                 "default/kuard",
			accesslogger:              FileAccessLogEnvoy("/dev/stdout", "", nil, contour_v1alpha1.LogLevelInfo),
			serverHeaderTranformation: contour_v1alpha1.PassThroughServerHeader,
			want: &envoy_config_listener_v3.Filter{
				Name: wellknown.HTTPConnectionManager,
				ConfigType: &envoy_config_listener_v3.Filter_TypedConfig{
					TypedConfig: protobuf.MustMarshalAny(&envoy_filter_network_http_connection_manager_v3.HttpConnectionManager{
						StatPrefix: "default/kuard",
						RouteSpecifier: &envoy_filter_network_http_connection_manager_v3.HttpConnectionManager_Rds{
							Rds: &envoy_filter_network_http_connection_manager_v3.Rds{
								RouteConfigName: "default/kuard",
								ConfigSource: &envoy_config_core_v3.ConfigSource{
									ResourceApiVersion: envoy_config_core_v3.ApiVersion_V3,
									ConfigSourceSpecifier: &envoy_config_core_v3.ConfigSource_ApiConfigSource{
										ApiConfigSource: &envoy_config_core_v3.ApiConfigSource{
											ApiType:             envoy_config_core_v3.ApiConfigSource_GRPC,
											TransportApiVersion: envoy_config_core_v3.ApiVersion_V3,
											GrpcServices: []*envoy_config_core_v3.GrpcService{{
												TargetSpecifier: &envoy_config_core_v3.GrpcService_EnvoyGrpc_{
													EnvoyGrpc: &envoy_config_core_v3.GrpcService_EnvoyGrpc{
														ClusterName: "contour",
														Authority:   "contour",
													},
												},
											}},
										},
									},
								},
							},
						},
						HttpFilters: defaultHTTPFilters,
						HttpProtocolOptions: &envoy_config_core_v3.Http1ProtocolOptions{
							// Enable support for HTTP/1.0 requests that carry
							// a Host: header. See #537.
							AcceptHttp_10: true,
						},
						CommonHttpProtocolOptions:  &envoy_config_core_v3.HttpProtocolOptions{},
						AccessLog:                  FileAccessLogEnvoy("/dev/stdout", "", nil, contour_v1alpha1.LogLevelInfo),
						UseRemoteAddress:           wrapperspb.Bool(true),
						NormalizePath:              wrapperspb.Bool(true),
						PreserveExternalRequestId:  true,
						ServerHeaderTransformation: envoy_filter_network_http_connection_manager_v3.HttpConnectionManager_PASS_THROUGH,
					}),
				},
			},
		},
		"enable xfcc": {
			routename:    "default/kuard",
			accesslogger: FileAccessLogEnvoy("/dev/stdout", "", nil, contour_v1alpha1.LogLevelInfo),
			forwardClientCertificate: &dag.ClientCertificateDetails{
				Subject: true,
				Cert:    true,
				Chain:   true,
				DNS:     true,
				URI:     true,
			},
			want: &envoy_config_listener_v3.Filter{
				Name: wellknown.HTTPConnectionManager,
				ConfigType: &envoy_config_listener_v3.Filter_TypedConfig{
					TypedConfig: protobuf.MustMarshalAny(&envoy_filter_network_http_connection_manager_v3.HttpConnectionManager{
						StatPrefix:               "default/kuard",
						ForwardClientCertDetails: envoy_filter_network_http_connection_manager_v3.HttpConnectionManager_SANITIZE_SET,
						SetCurrentClientCertDetails: &envoy_filter_network_http_connection_manager_v3.HttpConnectionManager_SetCurrentClientCertDetails{
							Subject: wrapperspb.Bool(true),
							Cert:    true,
							Chain:   true,
							Dns:     true,
							Uri:     true,
						},
						RouteSpecifier: &envoy_filter_network_http_connection_manager_v3.HttpConnectionManager_Rds{
							Rds: &envoy_filter_network_http_connection_manager_v3.Rds{
								RouteConfigName: "default/kuard",
								ConfigSource: &envoy_config_core_v3.ConfigSource{
									ResourceApiVersion: envoy_config_core_v3.ApiVersion_V3,
									ConfigSourceSpecifier: &envoy_config_core_v3.ConfigSource_ApiConfigSource{
										ApiConfigSource: &envoy_config_core_v3.ApiConfigSource{
											ApiType:             envoy_config_core_v3.ApiConfigSource_GRPC,
											TransportApiVersion: envoy_config_core_v3.ApiVersion_V3,
											GrpcServices: []*envoy_config_core_v3.GrpcService{{
												TargetSpecifier: &envoy_config_core_v3.GrpcService_EnvoyGrpc_{
													EnvoyGrpc: &envoy_config_core_v3.GrpcService_EnvoyGrpc{
														ClusterName: "contour",
														Authority:   "contour",
													},
												},
											}},
										},
									},
								},
							},
						},
						HttpFilters: defaultHTTPFilters,
						HttpProtocolOptions: &envoy_config_core_v3.Http1ProtocolOptions{
							// Enable support for HTTP/1.0 requests that carry
							// a Host: header. See #537.
							AcceptHttp_10: true,
						},
						CommonHttpProtocolOptions: &envoy_config_core_v3.HttpProtocolOptions{},
						AccessLog:                 FileAccessLogEnvoy("/dev/stdout", "", nil, contour_v1alpha1.LogLevelInfo),
						UseRemoteAddress:          wrapperspb.Bool(true),
						NormalizePath:             wrapperspb.Bool(true),
						PreserveExternalRequestId: true,
						MergeSlashes:              false,
					}),
				},
			},
		},
		"enable partial xfcc": {
			routename:    "default/kuard",
			accesslogger: FileAccessLogEnvoy("/dev/stdout", "", nil, contour_v1alpha1.LogLevelInfo),
			forwardClientCertificate: &dag.ClientCertificateDetails{
				Subject: true,
				DNS:     true,
				URI:     true,
			},
			want: &envoy_config_listener_v3.Filter{
				Name: wellknown.HTTPConnectionManager,
				ConfigType: &envoy_config_listener_v3.Filter_TypedConfig{
					TypedConfig: protobuf.MustMarshalAny(&envoy_filter_network_http_connection_manager_v3.HttpConnectionManager{
						StatPrefix:               "default/kuard",
						ForwardClientCertDetails: envoy_filter_network_http_connection_manager_v3.HttpConnectionManager_SANITIZE_SET,
						SetCurrentClientCertDetails: &envoy_filter_network_http_connection_manager_v3.HttpConnectionManager_SetCurrentClientCertDetails{
							Subject: wrapperspb.Bool(true),
							Dns:     true,
							Uri:     true,
						},
						RouteSpecifier: &envoy_filter_network_http_connection_manager_v3.HttpConnectionManager_Rds{
							Rds: &envoy_filter_network_http_connection_manager_v3.Rds{
								RouteConfigName: "default/kuard",
								ConfigSource: &envoy_config_core_v3.ConfigSource{
									ResourceApiVersion: envoy_config_core_v3.ApiVersion_V3,
									ConfigSourceSpecifier: &envoy_config_core_v3.ConfigSource_ApiConfigSource{
										ApiConfigSource: &envoy_config_core_v3.ApiConfigSource{
											ApiType:             envoy_config_core_v3.ApiConfigSource_GRPC,
											TransportApiVersion: envoy_config_core_v3.ApiVersion_V3,
											GrpcServices: []*envoy_config_core_v3.GrpcService{{
												TargetSpecifier: &envoy_config_core_v3.GrpcService_EnvoyGrpc_{
													EnvoyGrpc: &envoy_config_core_v3.GrpcService_EnvoyGrpc{
														ClusterName: "contour",
														Authority:   "contour",
													},
												},
											}},
										},
									},
								},
							},
						},
						HttpFilters: defaultHTTPFilters,
						HttpProtocolOptions: &envoy_config_core_v3.Http1ProtocolOptions{
							// Enable support for HTTP/1.0 requests that carry
							// a Host: header. See #537.
							AcceptHttp_10: true,
						},
						CommonHttpProtocolOptions: &envoy_config_core_v3.HttpProtocolOptions{},
						AccessLog:                 FileAccessLogEnvoy("/dev/stdout", "", nil, contour_v1alpha1.LogLevelInfo),
						UseRemoteAddress:          wrapperspb.Bool(true),
						NormalizePath:             wrapperspb.Bool(true),
						PreserveExternalRequestId: true,
						MergeSlashes:              false,
					}),
				},
			},
		},
		"enable XffNumTrustedHops": {
			routename:         "default/kuard",
			accesslogger:      FileAccessLogEnvoy("/dev/stdout", "", nil, contour_v1alpha1.LogLevelInfo),
			xffNumTrustedHops: 1,
			want: &envoy_config_listener_v3.Filter{
				Name: wellknown.HTTPConnectionManager,
				ConfigType: &envoy_config_listener_v3.Filter_TypedConfig{
					TypedConfig: protobuf.MustMarshalAny(&envoy_filter_network_http_connection_manager_v3.HttpConnectionManager{
						StatPrefix: "default/kuard",
						RouteSpecifier: &envoy_filter_network_http_connection_manager_v3.HttpConnectionManager_Rds{
							Rds: &envoy_filter_network_http_connection_manager_v3.Rds{
								RouteConfigName: "default/kuard",
								ConfigSource: &envoy_config_core_v3.ConfigSource{
									ResourceApiVersion: envoy_config_core_v3.ApiVersion_V3,
									ConfigSourceSpecifier: &envoy_config_core_v3.ConfigSource_ApiConfigSource{
										ApiConfigSource: &envoy_config_core_v3.ApiConfigSource{
											ApiType:             envoy_config_core_v3.ApiConfigSource_GRPC,
											TransportApiVersion: envoy_config_core_v3.ApiVersion_V3,
											GrpcServices: []*envoy_config_core_v3.GrpcService{{
												TargetSpecifier: &envoy_config_core_v3.GrpcService_EnvoyGrpc_{
													EnvoyGrpc: &envoy_config_core_v3.GrpcService_EnvoyGrpc{
														ClusterName: "contour",
														Authority:   "contour",
													},
												},
											}},
										},
									},
								},
							},
						},
						HttpFilters: defaultHTTPFilters,
						HttpProtocolOptions: &envoy_config_core_v3.Http1ProtocolOptions{
							// Enable support for HTTP/1.0 requests that carry
							// a Host: header. See #537.
							AcceptHttp_10: true,
						},
						CommonHttpProtocolOptions: &envoy_config_core_v3.HttpProtocolOptions{},
						AccessLog:                 FileAccessLogEnvoy("/dev/stdout", "", nil, contour_v1alpha1.LogLevelInfo),
						UseRemoteAddress:          wrapperspb.Bool(true),
						NormalizePath:             wrapperspb.Bool(true),
						PreserveExternalRequestId: true,
						MergeSlashes:              false,
						XffNumTrustedHops:         uint32(1),
					}),
				},
			},
		},
		"enable StripTrailingHostDot": {
			routename:            "default/kuard",
			accesslogger:         FileAccessLogEnvoy("/dev/stdout", "", nil, contour_v1alpha1.LogLevelInfo),
			stripTrailingHostDot: true,
			want: &envoy_config_listener_v3.Filter{
				Name: wellknown.HTTPConnectionManager,
				ConfigType: &envoy_config_listener_v3.Filter_TypedConfig{
					TypedConfig: protobuf.MustMarshalAny(&envoy_filter_network_http_connection_manager_v3.HttpConnectionManager{
						StatPrefix: "default/kuard",
						RouteSpecifier: &envoy_filter_network_http_connection_manager_v3.HttpConnectionManager_Rds{
							Rds: &envoy_filter_network_http_connection_manager_v3.Rds{
								RouteConfigName: "default/kuard",
								ConfigSource: &envoy_config_core_v3.ConfigSource{
									ResourceApiVersion: envoy_config_core_v3.ApiVersion_V3,
									ConfigSourceSpecifier: &envoy_config_core_v3.ConfigSource_ApiConfigSource{
										ApiConfigSource: &envoy_config_core_v3.ApiConfigSource{
											ApiType:             envoy_config_core_v3.ApiConfigSource_GRPC,
											TransportApiVersion: envoy_config_core_v3.ApiVersion_V3,
											GrpcServices: []*envoy_config_core_v3.GrpcService{{
												TargetSpecifier: &envoy_config_core_v3.GrpcService_EnvoyGrpc_{
													EnvoyGrpc: &envoy_config_core_v3.GrpcService_EnvoyGrpc{
														ClusterName: "contour",
														Authority:   "contour",
													},
												},
											}},
										},
									},
								},
							},
						},
						HttpFilters: defaultHTTPFilters,
						HttpProtocolOptions: &envoy_config_core_v3.Http1ProtocolOptions{
							// Enable support for HTTP/1.0 requests that carry
							// a Host: header. See #537.
							AcceptHttp_10: true,
						},
						CommonHttpProtocolOptions: &envoy_config_core_v3.HttpProtocolOptions{},
						AccessLog:                 FileAccessLogEnvoy("/dev/stdout", "", nil, contour_v1alpha1.LogLevelInfo),
						UseRemoteAddress:          wrapperspb.Bool(true),
						NormalizePath:             wrapperspb.Bool(true),
						PreserveExternalRequestId: true,
						MergeSlashes:              false,
						StripTrailingHostDot:      true,
					}),
				},
			},
		},
		"maxRequestsPerConnection set to 1": {
			routename:                "default/kuard",
			accesslogger:             FileAccessLogEnvoy("/dev/stdout", "", nil, contour_v1alpha1.LogLevelInfo),
			maxRequestsPerConnection: ptr.To(uint32(1)),
			want: &envoy_config_listener_v3.Filter{
				Name: wellknown.HTTPConnectionManager,
				ConfigType: &envoy_config_listener_v3.Filter_TypedConfig{
					TypedConfig: protobuf.MustMarshalAny(&envoy_filter_network_http_connection_manager_v3.HttpConnectionManager{
						StatPrefix: "default/kuard",
						RouteSpecifier: &envoy_filter_network_http_connection_manager_v3.HttpConnectionManager_Rds{
							Rds: &envoy_filter_network_http_connection_manager_v3.Rds{
								RouteConfigName: "default/kuard",
								ConfigSource: &envoy_config_core_v3.ConfigSource{
									ResourceApiVersion: envoy_config_core_v3.ApiVersion_V3,
									ConfigSourceSpecifier: &envoy_config_core_v3.ConfigSource_ApiConfigSource{
										ApiConfigSource: &envoy_config_core_v3.ApiConfigSource{
											ApiType:             envoy_config_core_v3.ApiConfigSource_GRPC,
											TransportApiVersion: envoy_config_core_v3.ApiVersion_V3,
											GrpcServices: []*envoy_config_core_v3.GrpcService{{
												TargetSpecifier: &envoy_config_core_v3.GrpcService_EnvoyGrpc_{
													EnvoyGrpc: &envoy_config_core_v3.GrpcService_EnvoyGrpc{
														ClusterName: "contour",
														Authority:   "contour",
													},
												},
											}},
										},
									},
								},
							},
						},
						HttpFilters: defaultHTTPFilters,
						HttpProtocolOptions: &envoy_config_core_v3.Http1ProtocolOptions{
							// Enable support for HTTP/1.0 requests that carry
							// a Host: header. See #537.
							AcceptHttp_10: true,
						},
						CommonHttpProtocolOptions: &envoy_config_core_v3.HttpProtocolOptions{
							MaxRequestsPerConnection: wrapperspb.UInt32(1),
						},
						AccessLog:                 FileAccessLogEnvoy("/dev/stdout", "", nil, contour_v1alpha1.LogLevelInfo),
						UseRemoteAddress:          wrapperspb.Bool(true),
						NormalizePath:             wrapperspb.Bool(true),
						PreserveExternalRequestId: true,
						MergeSlashes:              false,
					}),
				},
			},
		},
		"http2MaxConcurrentStreams set": {
			routename:                 "default/kuard",
			accesslogger:              FileAccessLogEnvoy("/dev/stdout", "", nil, contour_v1alpha1.LogLevelInfo),
			http2MaxConcurrentStreams: ptr.To(uint32(50)),
			want: &envoy_config_listener_v3.Filter{
				Name: wellknown.HTTPConnectionManager,
				ConfigType: &envoy_config_listener_v3.Filter_TypedConfig{
					TypedConfig: protobuf.MustMarshalAny(&envoy_filter_network_http_connection_manager_v3.HttpConnectionManager{
						StatPrefix: "default/kuard",
						RouteSpecifier: &envoy_filter_network_http_connection_manager_v3.HttpConnectionManager_Rds{
							Rds: &envoy_filter_network_http_connection_manager_v3.Rds{
								RouteConfigName: "default/kuard",
								ConfigSource: &envoy_config_core_v3.ConfigSource{
									ResourceApiVersion: envoy_config_core_v3.ApiVersion_V3,
									ConfigSourceSpecifier: &envoy_config_core_v3.ConfigSource_ApiConfigSource{
										ApiConfigSource: &envoy_config_core_v3.ApiConfigSource{
											ApiType:             envoy_config_core_v3.ApiConfigSource_GRPC,
											TransportApiVersion: envoy_config_core_v3.ApiVersion_V3,
											GrpcServices: []*envoy_config_core_v3.GrpcService{{
												TargetSpecifier: &envoy_config_core_v3.GrpcService_EnvoyGrpc_{
													EnvoyGrpc: &envoy_config_core_v3.GrpcService_EnvoyGrpc{
														ClusterName: "contour",
														Authority:   "contour",
													},
												},
											}},
										},
									},
								},
							},
						},
						HttpFilters: defaultHTTPFilters,
						HttpProtocolOptions: &envoy_config_core_v3.Http1ProtocolOptions{
							// Enable support for HTTP/1.0 requests that carry
							// a Host: header. See #537.
							AcceptHttp_10: true,
						},
						CommonHttpProtocolOptions: &envoy_config_core_v3.HttpProtocolOptions{},
						Http2ProtocolOptions: &envoy_config_core_v3.Http2ProtocolOptions{
							MaxConcurrentStreams: wrapperspb.UInt32(50),
						},
						AccessLog:                 FileAccessLogEnvoy("/dev/stdout", "", nil, contour_v1alpha1.LogLevelInfo),
						UseRemoteAddress:          wrapperspb.Bool(true),
						NormalizePath:             wrapperspb.Bool(true),
						PreserveExternalRequestId: true,
						MergeSlashes:              false,
					}),
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := envoyGen.HTTPConnectionManagerBuilder().
				RouteConfigName(tc.routename).
				MetricsPrefix(tc.routename).
				AccessLoggers(tc.accesslogger).
				RequestTimeout(tc.requestTimeout).
				ConnectionIdleTimeout(tc.connectionIdleTimeout).
				StreamIdleTimeout(tc.streamIdleTimeout).
				MaxConnectionDuration(tc.maxConnectionDuration).
				DelayedCloseTimeout(tc.delayedCloseTimeout).
				ConnectionShutdownGracePeriod(tc.connectionShutdownGracePeriod).
				AllowChunkedLength(tc.allowChunkedLength).
				MergeSlashes(tc.mergeSlashes).
				ServerHeaderTransformation(tc.serverHeaderTranformation).
				NumTrustedHops(tc.xffNumTrustedHops).
				StripTrailingHostDot(tc.stripTrailingHostDot).
				ForwardClientCertificate(tc.forwardClientCertificate).
				MaxRequestsPerConnection(tc.maxRequestsPerConnection).
				HTTP2MaxConcurrentStreams(tc.http2MaxConcurrentStreams).
				DefaultFilters().
				Get()

			protobuf.ExpectEqual(t, tc.want, got)
		})
	}
}

func TestTCPProxy(t *testing.T) {
	const (
		statPrefix    = "ingress_https"
		accessLogPath = "/dev/stdout"
	)

	// c1 has no weight
	c1 := &dag.Cluster{
		Upstream: &dag.Service{
			Weighted: dag.WeightedService{
				Weight:           1,
				ServiceName:      "example",
				ServiceNamespace: "default",
				ServicePort: core_v1.ServicePort{
					Protocol:   "TCP",
					Port:       443,
					TargetPort: intstr.FromInt(8443),
				},
			},
		},
	}
	// c2 has a non-zero weight
	c2 := &dag.Cluster{
		Upstream: &dag.Service{
			Weighted: dag.WeightedService{
				ServiceName:      "example2",
				ServiceNamespace: "default",
				ServicePort: core_v1.ServicePort{
					Protocol:   "TCP",
					Port:       443,
					TargetPort: intstr.FromInt(8443),
				},
			},
		},
		Weight: 20,
	}
	// c3 has a non-zero weight
	c3 := &dag.Cluster{
		Upstream: &dag.Service{
			Weighted: dag.WeightedService{
				ServiceName:      "example3",
				ServiceNamespace: "default",
				ServicePort: core_v1.ServicePort{
					Protocol:   "TCP",
					Port:       443,
					TargetPort: intstr.FromInt(8443),
				},
			},
		},
		Weight: 40,
	}
	// c4 has no weight
	c4 := &dag.Cluster{
		Upstream: &dag.Service{
			Weighted: dag.WeightedService{
				Weight:           1,
				ServiceName:      "example4",
				ServiceNamespace: "default",
				ServicePort: core_v1.ServicePort{
					Protocol:   "TCP",
					Port:       443,
					TargetPort: intstr.FromInt(8443),
				},
			},
		},
	}

	tests := map[string]struct {
		proxy *dag.TCPProxy
		want  *envoy_config_listener_v3.Filter
	}{
		"single cluster": {
			proxy: &dag.TCPProxy{
				Clusters: []*dag.Cluster{c1},
			},
			want: &envoy_config_listener_v3.Filter{
				Name: wellknown.TCPProxy,
				ConfigType: &envoy_config_listener_v3.Filter_TypedConfig{
					TypedConfig: protobuf.MustMarshalAny(&envoy_filter_network_tcp_proxy_v3.TcpProxy{
						StatPrefix: statPrefix,
						ClusterSpecifier: &envoy_filter_network_tcp_proxy_v3.TcpProxy_Cluster{
							Cluster: envoy.Clustername(c1),
						},
						AccessLog:   FileAccessLogEnvoy(accessLogPath, "", nil, contour_v1alpha1.LogLevelInfo),
						IdleTimeout: durationpb.New(9001 * time.Second),
					}),
				},
			},
		},
		"three clusters, one has no weight specified": {
			proxy: &dag.TCPProxy{
				Clusters: []*dag.Cluster{c1, c2, c3},
			},
			want: &envoy_config_listener_v3.Filter{
				Name: wellknown.TCPProxy,
				ConfigType: &envoy_config_listener_v3.Filter_TypedConfig{
					TypedConfig: protobuf.MustMarshalAny(&envoy_filter_network_tcp_proxy_v3.TcpProxy{
						StatPrefix: statPrefix,
						ClusterSpecifier: &envoy_filter_network_tcp_proxy_v3.TcpProxy_WeightedClusters{
							WeightedClusters: &envoy_filter_network_tcp_proxy_v3.TcpProxy_WeightedCluster{
								Clusters: []*envoy_filter_network_tcp_proxy_v3.TcpProxy_WeightedCluster_ClusterWeight{
									{
										Name:   envoy.Clustername(c2),
										Weight: 20,
									},
									{
										Name:   envoy.Clustername(c3),
										Weight: 40,
									},
								},
							},
						},
						AccessLog:   FileAccessLogEnvoy(accessLogPath, "", nil, contour_v1alpha1.LogLevelInfo),
						IdleTimeout: durationpb.New(9001 * time.Second),
					}),
				},
			},
		},
		"three clusters, two have no weights specified": {
			proxy: &dag.TCPProxy{
				Clusters: []*dag.Cluster{c1, c2, c4},
			},
			want: &envoy_config_listener_v3.Filter{
				Name: wellknown.TCPProxy,
				ConfigType: &envoy_config_listener_v3.Filter_TypedConfig{
					TypedConfig: protobuf.MustMarshalAny(&envoy_filter_network_tcp_proxy_v3.TcpProxy{
						StatPrefix: statPrefix,
						ClusterSpecifier: &envoy_filter_network_tcp_proxy_v3.TcpProxy_Cluster{
							Cluster: envoy.Clustername(c2),
						},
						AccessLog:   FileAccessLogEnvoy(accessLogPath, "", nil, contour_v1alpha1.LogLevelInfo),
						IdleTimeout: durationpb.New(9001 * time.Second),
					}),
				},
			},
		},
		"multiple clusters, all have weights specified": {
			proxy: &dag.TCPProxy{
				Clusters: []*dag.Cluster{c2, c3},
			},
			want: &envoy_config_listener_v3.Filter{
				Name: wellknown.TCPProxy,
				ConfigType: &envoy_config_listener_v3.Filter_TypedConfig{
					TypedConfig: protobuf.MustMarshalAny(&envoy_filter_network_tcp_proxy_v3.TcpProxy{
						StatPrefix: statPrefix,
						ClusterSpecifier: &envoy_filter_network_tcp_proxy_v3.TcpProxy_WeightedClusters{
							WeightedClusters: &envoy_filter_network_tcp_proxy_v3.TcpProxy_WeightedCluster{
								Clusters: []*envoy_filter_network_tcp_proxy_v3.TcpProxy_WeightedCluster_ClusterWeight{{
									Name:   envoy.Clustername(c2),
									Weight: 20,
								}, {
									Name:   envoy.Clustername(c3),
									Weight: 40,
								}},
							},
						},
						AccessLog:   FileAccessLogEnvoy(accessLogPath, "", nil, contour_v1alpha1.LogLevelInfo),
						IdleTimeout: durationpb.New(9001 * time.Second),
					}),
				},
			},
		},
		"multiple clusters, none have weights specified": {
			proxy: &dag.TCPProxy{
				Clusters: []*dag.Cluster{c1, c4},
			},
			want: &envoy_config_listener_v3.Filter{
				Name: wellknown.TCPProxy,
				ConfigType: &envoy_config_listener_v3.Filter_TypedConfig{
					TypedConfig: protobuf.MustMarshalAny(&envoy_filter_network_tcp_proxy_v3.TcpProxy{
						StatPrefix: statPrefix,
						ClusterSpecifier: &envoy_filter_network_tcp_proxy_v3.TcpProxy_WeightedClusters{
							WeightedClusters: &envoy_filter_network_tcp_proxy_v3.TcpProxy_WeightedCluster{
								Clusters: []*envoy_filter_network_tcp_proxy_v3.TcpProxy_WeightedCluster_ClusterWeight{{
									Name:   envoy.Clustername(c1),
									Weight: 1,
								}, {
									Name:   envoy.Clustername(c4),
									Weight: 1,
								}},
							},
						},
						AccessLog:   FileAccessLogEnvoy(accessLogPath, "", nil, contour_v1alpha1.LogLevelInfo),
						IdleTimeout: durationpb.New(9001 * time.Second),
					}),
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := TCPProxy(statPrefix, tc.proxy, FileAccessLogEnvoy(accessLogPath, "", nil, contour_v1alpha1.LogLevelInfo))
			protobuf.ExpectEqual(t, tc.want, got)
		})
	}
}

func TestFilterChainTLS_Match(t *testing.T) {
	tests := map[string]struct {
		domain     string
		downstream *envoy_transport_socket_tls_v3.DownstreamTlsContext
		filters    []*envoy_config_listener_v3.Filter
		want       *envoy_config_listener_v3.FilterChain
	}{
		"SNI": {
			domain: "projectcontour.io",
			want: &envoy_config_listener_v3.FilterChain{
				FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
					ServerNames: []string{"projectcontour.io"},
				},
			},
		},
		"No SNI": {
			domain: "*",
			want: &envoy_config_listener_v3.FilterChain{
				FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
					TransportProtocol: "tls",
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := FilterChainTLS(tc.domain, tc.downstream, tc.filters)
			protobuf.ExpectEqual(t, tc.want, got)
		})
	}
}

// TestBuilderValidation tests that validation checks that
// DefaultFilters adds the required HTTP connection manager filters.
func TestBuilderValidation(t *testing.T) {
	envoygen := NewEnvoyGen(EnvoyGenOpt{
		XDSClusterName: "notcontour",
	})

	require.Error(t, envoygen.HTTPConnectionManagerBuilder().Validate(),
		"ConnectionManager with no filters should not pass validation")

	require.Error(t, envoygen.HTTPConnectionManagerBuilder().AddFilter(&envoy_filter_network_http_connection_manager_v3.HttpFilter{
		Name: "foo",
		ConfigType: &envoy_filter_network_http_connection_manager_v3.HttpFilter_TypedConfig{
			TypedConfig: &anypb.Any{
				TypeUrl: "foo",
			},
		},
	}).Validate(),
		"ConnectionManager with only non-router filter should not pass validation")

	require.NoError(t, envoygen.HTTPConnectionManagerBuilder().DefaultFilters().Validate(),
		"ConnectionManager with default filters failed validation")

	badBuilder := envoygen.HTTPConnectionManagerBuilder().DefaultFilters()
	badBuilder.filters = append(badBuilder.filters, &envoy_filter_network_http_connection_manager_v3.HttpFilter{
		Name: "foo",
		ConfigType: &envoy_filter_network_http_connection_manager_v3.HttpFilter_TypedConfig{
			TypedConfig: &anypb.Any{
				TypeUrl: "foo",
			},
		},
	})
	require.Errorf(t, badBuilder.Validate(), "Adding a filter after the Router filter should fail")
}

func TestAddFilter(t *testing.T) {
	routerFilter := &envoy_filter_network_http_connection_manager_v3.HttpFilter{
		Name: "router",
		ConfigType: &envoy_filter_network_http_connection_manager_v3.HttpFilter_TypedConfig{
			TypedConfig: protobuf.MustMarshalAny(&envoy_filter_http_router_v3.Router{}),
		},
	}
	grpcWebFilter := &envoy_filter_network_http_connection_manager_v3.HttpFilter{
		Name: GRPCWebFilterName,
		ConfigType: &envoy_filter_network_http_connection_manager_v3.HttpFilter_TypedConfig{
			TypedConfig: protobuf.MustMarshalAny(&envoy_filter_http_grpc_web_v3.GrpcWeb{}),
		},
	}
	corsFilter := &envoy_filter_network_http_connection_manager_v3.HttpFilter{
		Name: CORSFilterName,
		ConfigType: &envoy_filter_network_http_connection_manager_v3.HttpFilter_TypedConfig{
			TypedConfig: protobuf.MustMarshalAny(&envoy_filter_http_cors_v3.Cors{}),
		},
	}

	grpcStatsFilter := &envoy_filter_network_http_connection_manager_v3.HttpFilter{
		Name: GRPCStatsFilterName,
		ConfigType: &envoy_filter_network_http_connection_manager_v3.HttpFilter_TypedConfig{
			TypedConfig: protobuf.MustMarshalAny(
				&envoy_filter_http_grpc_stats_v3.FilterConfig{
					EmitFilterState:     true,
					EnableUpstreamStats: true,
				},
			),
		},
	}
	luaFilter := &envoy_filter_network_http_connection_manager_v3.HttpFilter{
		Name: LuaFilterName,
		ConfigType: &envoy_filter_network_http_connection_manager_v3.HttpFilter_TypedConfig{
			TypedConfig: protobuf.MustMarshalAny(&envoy_filter_http_lua_v3.Lua{
				DefaultSourceCode: &envoy_config_core_v3.DataSource{
					Specifier: &envoy_config_core_v3.DataSource_InlineString{
						InlineString: "-- Placeholder for per-Route or per-Cluster overrides.",
					},
				},
			}),
		},
	}
	rbacFilter := &envoy_filter_network_http_connection_manager_v3.HttpFilter{
		Name: RBACFilterName,
		ConfigType: &envoy_filter_network_http_connection_manager_v3.HttpFilter_TypedConfig{
			TypedConfig: protobuf.MustMarshalAny(&envoy_filter_http_rbac_v3.RBAC{}),
		},
	}

	localRateLimitFilter := &envoy_filter_network_http_connection_manager_v3.HttpFilter{
		Name: LocalRateLimitFilterName,
		ConfigType: &envoy_filter_network_http_connection_manager_v3.HttpFilter_TypedConfig{
			TypedConfig: protobuf.MustMarshalAny(
				&envoy_filter_http_local_ratelimit_v3.LocalRateLimit{
					StatPrefix: "http",
				},
			),
		},
	}

	compressFilter := &envoy_filter_network_http_connection_manager_v3.HttpFilter{
		Name: CompressorFilterName,
		ConfigType: &envoy_filter_network_http_connection_manager_v3.HttpFilter_TypedConfig{
			TypedConfig: protobuf.MustMarshalAny(&envoy_filter_http_compressor_v3.Compressor{
				CompressorLibrary: &envoy_config_core_v3.TypedExtensionConfig{
					Name: "gzip",
					TypedConfig: protobuf.MustMarshalAny(
						&envoy_compression_gzip_compressor_v3.Gzip{},
					),
				},
				ResponseDirectionConfig: &envoy_filter_http_compressor_v3.Compressor_ResponseDirectionConfig{
					CommonConfig: &envoy_filter_http_compressor_v3.Compressor_CommonDirectionConfig{
						ContentType: compressorContentTypes,
					},
				},
			}),
		},
	}

	envoygen := NewEnvoyGen(EnvoyGenOpt{
		XDSClusterName: DefaultXDSClusterName,
	})

	tests := map[string]struct {
		builder *httpConnectionManagerBuilder
		add     *envoy_filter_network_http_connection_manager_v3.HttpFilter
		want    []*envoy_filter_network_http_connection_manager_v3.HttpFilter
	}{
		"Nil add to empty builder": {
			builder: envoygen.HTTPConnectionManagerBuilder(),
			add:     nil,
			want:    nil,
		},
		"Add a single router filter to empty builder": {
			builder: envoygen.HTTPConnectionManagerBuilder(),
			add:     routerFilter,
			want:    []*envoy_filter_network_http_connection_manager_v3.HttpFilter{routerFilter},
		},
		"Add a single non-router filter to empty builder": {
			builder: envoygen.HTTPConnectionManagerBuilder(),
			add:     grpcWebFilter,
			want:    []*envoy_filter_network_http_connection_manager_v3.HttpFilter{grpcWebFilter},
		},
		"Add a single router filter to non-empty builder": {
			builder: envoygen.HTTPConnectionManagerBuilder().AddFilter(grpcWebFilter),
			add:     routerFilter,
			want:    []*envoy_filter_network_http_connection_manager_v3.HttpFilter{grpcWebFilter, routerFilter},
		},

		"Add a filter to a builder with a router": {
			builder: envoygen.HTTPConnectionManagerBuilder().AddFilter(routerFilter),
			add:     grpcWebFilter,
			want:    []*envoy_filter_network_http_connection_manager_v3.HttpFilter{grpcWebFilter, routerFilter},
		},
		"Add to the default filters": {
			builder: envoygen.HTTPConnectionManagerBuilder().DefaultFilters(),
			add:     authzFilter(),
			want: []*envoy_filter_network_http_connection_manager_v3.HttpFilter{
				compressFilter,
				grpcWebFilter,
				grpcStatsFilter,
				corsFilter,
				localRateLimitFilter,
				luaFilter,
				rbacFilter,
				authzFilter(),
				routerFilter,
			},
		},
		"Add to the default filters with AuthorizationServerBufferSettings": {
			builder: envoygen.HTTPConnectionManagerBuilder().DefaultFilters(),
			add: authzFilter("ext-auth-server.com", &dag.AuthorizationServerBufferSettings{
				MaxRequestBytes:     10,
				AllowPartialMessage: true,
				PackAsBytes:         true,
			}),
			want: []*envoy_filter_network_http_connection_manager_v3.HttpFilter{
				compressFilter,
				grpcWebFilter,
				grpcStatsFilter,
				corsFilter,
				localRateLimitFilter,
				luaFilter,
				rbacFilter,
				authzFilter("ext-auth-server.com", &dag.AuthorizationServerBufferSettings{
					MaxRequestBytes:     10,
					AllowPartialMessage: true,
					PackAsBytes:         true,
				}),
				routerFilter,
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := tc.builder.AddFilter(tc.add)
			assert.Equal(t, tc.want, got.filters)
		})
	}

	assert.Panics(t, func() {
		envoygen.HTTPConnectionManagerBuilder().DefaultFilters().AddFilter(routerFilter)
	})
}

func authzFilter(extras ...any) *envoy_filter_network_http_connection_manager_v3.HttpFilter {
	sni := ""
	if len(extras) > 0 {
		sni = extras[0].(string)
	}

	var body *dag.AuthorizationServerBufferSettings
	if len(extras) > 1 {
		body = extras[1].(*dag.AuthorizationServerBufferSettings)
	}

	return FilterExternalAuthz(&dag.ExternalAuthorization{
		AuthorizationService: &dag.ExtensionCluster{
			Name: "test",
			SNI:  sni,
		},
		AuthorizationFailOpen:              false,
		AuthorizationResponseTimeout:       timeout.Setting{},
		AuthorizationServerWithRequestBody: body,
	})
}
