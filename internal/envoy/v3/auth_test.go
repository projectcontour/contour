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

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_transport_socket_tls_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoy_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/protobuf"
)

func TestUpstreamTLSContext(t *testing.T) {
	secret := &dag.Secret{
		Object: &core_v1.Secret{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      "secret",
				Namespace: "default",
			},
			Type: core_v1.SecretTypeTLS,
			Data: map[string][]byte{dag.CACertificateKey: []byte("ca")},
		},
	}

	tests := map[string]struct {
		validation    *dag.PeerValidationContext
		alpnProtocols []string
		externalName  string
		upstreamTLS   *dag.UpstreamTLS
		want          *envoy_transport_socket_tls_v3.UpstreamTlsContext
	}{
		"no alpn, no validation, no upstreamTLS": {
			want: &envoy_transport_socket_tls_v3.UpstreamTlsContext{
				CommonTlsContext: &envoy_transport_socket_tls_v3.CommonTlsContext{},
			},
		},
		"h2, no validation": {
			alpnProtocols: []string{"h2c"},
			want: &envoy_transport_socket_tls_v3.UpstreamTlsContext{
				CommonTlsContext: &envoy_transport_socket_tls_v3.CommonTlsContext{
					AlpnProtocols: []string{"h2c"},
				},
			},
		},
		"no alpn, missing altname": {
			validation: &dag.PeerValidationContext{
				CACertificates: []*dag.Secret{
					secret,
				},
			},
			want: &envoy_transport_socket_tls_v3.UpstreamTlsContext{
				CommonTlsContext: &envoy_transport_socket_tls_v3.CommonTlsContext{},
			},
		},
		"no alpn, missing ca": {
			validation: &dag.PeerValidationContext{
				SubjectNames: []string{"www.example.com"},
			},
			want: &envoy_transport_socket_tls_v3.UpstreamTlsContext{
				CommonTlsContext: &envoy_transport_socket_tls_v3.CommonTlsContext{},
			},
		},
		"no alpn, ca and altname": {
			validation: &dag.PeerValidationContext{
				CACertificates: []*dag.Secret{
					secret,
				},
				SubjectNames: []string{"www.example.com"},
			},
			want: &envoy_transport_socket_tls_v3.UpstreamTlsContext{
				CommonTlsContext: &envoy_transport_socket_tls_v3.CommonTlsContext{
					ValidationContextType: &envoy_transport_socket_tls_v3.CommonTlsContext_ValidationContext{
						ValidationContext: &envoy_transport_socket_tls_v3.CertificateValidationContext{
							TrustedCa: &envoy_config_core_v3.DataSource{
								Specifier: &envoy_config_core_v3.DataSource_InlineBytes{
									InlineBytes: []byte("ca"),
								},
							},
							MatchTypedSubjectAltNames: []*envoy_transport_socket_tls_v3.SubjectAltNameMatcher{
								{
									SanType: envoy_transport_socket_tls_v3.SubjectAltNameMatcher_DNS,
									Matcher: &envoy_matcher_v3.StringMatcher{
										MatchPattern: &envoy_matcher_v3.StringMatcher_Exact{
											Exact: "www.example.com",
										},
									},
								},
							},
						},
					},
				},
			},
		},
		"external name sni": {
			externalName: "projectcontour.local",
			want: &envoy_transport_socket_tls_v3.UpstreamTlsContext{
				CommonTlsContext: &envoy_transport_socket_tls_v3.CommonTlsContext{},
				Sni:              "projectcontour.local",
			},
		},
		"use TLS 1.3": {
			upstreamTLS: &dag.UpstreamTLS{
				MinimumProtocolVersion: "1.3",
				MaximumProtocolVersion: "1.3",
			},
			want: &envoy_transport_socket_tls_v3.UpstreamTlsContext{
				CommonTlsContext: &envoy_transport_socket_tls_v3.CommonTlsContext{
					TlsParams: &envoy_transport_socket_tls_v3.TlsParameters{
						TlsMinimumProtocolVersion: ParseTLSVersion("1.3"),
						TlsMaximumProtocolVersion: ParseTLSVersion("1.3"),
					},
				},
			},
		},
		"multiple subjectnames": {
			validation: &dag.PeerValidationContext{
				CACertificates: []*dag.Secret{
					secret,
				},
				SubjectNames: []string{
					"foo.com",
					"bar.com",
				},
			},
			want: &envoy_transport_socket_tls_v3.UpstreamTlsContext{
				CommonTlsContext: &envoy_transport_socket_tls_v3.CommonTlsContext{
					ValidationContextType: &envoy_transport_socket_tls_v3.CommonTlsContext_ValidationContext{
						ValidationContext: &envoy_transport_socket_tls_v3.CertificateValidationContext{
							TrustedCa: &envoy_config_core_v3.DataSource{
								Specifier: &envoy_config_core_v3.DataSource_InlineBytes{
									InlineBytes: []byte("ca"),
								},
							},
							MatchTypedSubjectAltNames: []*envoy_transport_socket_tls_v3.SubjectAltNameMatcher{
								{
									SanType: envoy_transport_socket_tls_v3.SubjectAltNameMatcher_DNS,
									Matcher: &envoy_matcher_v3.StringMatcher{
										MatchPattern: &envoy_matcher_v3.StringMatcher_Exact{
											Exact: "foo.com",
										},
									},
								},
								{
									SanType: envoy_transport_socket_tls_v3.SubjectAltNameMatcher_DNS,
									Matcher: &envoy_matcher_v3.StringMatcher{
										MatchPattern: &envoy_matcher_v3.StringMatcher_Exact{
											Exact: "bar.com",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			e := NewEnvoyGen(EnvoyGenOpt{
				XDSClusterName: DefaultXDSClusterName,
			})
			got := e.UpstreamTLSContext(tc.validation, tc.externalName, nil, tc.upstreamTLS, tc.alpnProtocols...)
			protobuf.ExpectEqual(t, tc.want, got)
		})
	}
}
