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

	core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	tls_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/protobuf"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
		want          *tls_v3.UpstreamTlsContext
	}{
		"no alpn, no validation": {
			want: &tls_v3.UpstreamTlsContext{
				CommonTlsContext: &tls_v3.CommonTlsContext{},
			},
		},
		"h2, no validation": {
			alpnProtocols: []string{"h2c"},
			want: &tls_v3.UpstreamTlsContext{
				CommonTlsContext: &tls_v3.CommonTlsContext{
					AlpnProtocols: []string{"h2c"},
				},
			},
		},
		"no alpn, missing altname": {
			validation: &dag.PeerValidationContext{
				CACertificate: secret,
			},
			want: &tls_v3.UpstreamTlsContext{
				CommonTlsContext: &tls_v3.CommonTlsContext{},
			},
		},
		"no alpn, missing ca": {
			validation: &dag.PeerValidationContext{
				SubjectName: "www.example.com",
			},
			want: &tls_v3.UpstreamTlsContext{
				CommonTlsContext: &tls_v3.CommonTlsContext{},
			},
		},
		"no alpn, ca and altname": {
			validation: &dag.PeerValidationContext{
				CACertificate: secret,
				SubjectName:   "www.example.com",
			},
			want: &tls_v3.UpstreamTlsContext{
				CommonTlsContext: &tls_v3.CommonTlsContext{
					ValidationContextType: &tls_v3.CommonTlsContext_ValidationContext{
						ValidationContext: &tls_v3.CertificateValidationContext{
							TrustedCa: &core_v3.DataSource{
								Specifier: &core_v3.DataSource_InlineBytes{
									InlineBytes: []byte("ca"),
								},
							},
							MatchSubjectAltNames: []*matcher_v3.StringMatcher{{
								MatchPattern: &matcher_v3.StringMatcher_Exact{
									Exact: "www.example.com",
								}},
							},
						},
					},
				},
			},
		},
		"external name sni": {
			externalName: "projectcontour.local",
			want: &tls_v3.UpstreamTlsContext{
				CommonTlsContext: &tls_v3.CommonTlsContext{},
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
