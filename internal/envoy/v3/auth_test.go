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

	envoy_api_v3_core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_v3_tls "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	matcher "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/protobuf"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestUpstreamTLSContext(t *testing.T) {
	secret := &dag.Secret{
		Object: &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "secret",
				Namespace: "default",
			},
			Type: v1.SecretTypeTLS,
			Data: map[string][]byte{dag.CACertificateKey: []byte("ca")},
		},
	}

	tests := map[string]struct {
		validation    *dag.PeerValidationContext
		alpnProtocols []string
		externalName  string
		want          *envoy_v3_tls.UpstreamTlsContext
	}{
		"no alpn, no validation": {
			want: &envoy_v3_tls.UpstreamTlsContext{
				CommonTlsContext: &envoy_v3_tls.CommonTlsContext{},
			},
		},
		"h2, no validation": {
			alpnProtocols: []string{"h2c"},
			want: &envoy_v3_tls.UpstreamTlsContext{
				CommonTlsContext: &envoy_v3_tls.CommonTlsContext{
					AlpnProtocols: []string{"h2c"},
				},
			},
		},
		"no alpn, missing altname": {
			validation: &dag.PeerValidationContext{
				CACertificate: secret,
			},
			want: &envoy_v3_tls.UpstreamTlsContext{
				CommonTlsContext: &envoy_v3_tls.CommonTlsContext{},
			},
		},
		"no alpn, missing ca": {
			validation: &dag.PeerValidationContext{
				SubjectName: "www.example.com",
			},
			want: &envoy_v3_tls.UpstreamTlsContext{
				CommonTlsContext: &envoy_v3_tls.CommonTlsContext{},
			},
		},
		"no alpn, ca and altname": {
			validation: &dag.PeerValidationContext{
				CACertificate: secret,
				SubjectName:   "www.example.com",
			},
			want: &envoy_v3_tls.UpstreamTlsContext{
				CommonTlsContext: &envoy_v3_tls.CommonTlsContext{
					ValidationContextType: &envoy_v3_tls.CommonTlsContext_ValidationContext{
						ValidationContext: &envoy_v3_tls.CertificateValidationContext{
							TrustedCa: &envoy_api_v3_core.DataSource{
								Specifier: &envoy_api_v3_core.DataSource_InlineBytes{
									InlineBytes: []byte("ca"),
								},
							},
							MatchTypedSubjectAltNames: []*envoy_v3_tls.SubjectAltNameMatcher{
								{
									SanType: envoy_v3_tls.SubjectAltNameMatcher_DNS,
									Matcher: &matcher.StringMatcher{
										MatchPattern: &matcher.StringMatcher_Exact{
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
			want: &envoy_v3_tls.UpstreamTlsContext{
				CommonTlsContext: &envoy_v3_tls.CommonTlsContext{},
				Sni:              "projectcontour.local",
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := UpstreamTLSContext(tc.validation, tc.externalName, nil, tc.alpnProtocols...)
			protobuf.ExpectEqual(t, tc.want, got)
		})
	}
}
