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

package envoy

import (
	"testing"

	envoy_api_v2_auth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	envoy_api_v2_core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	matcher "github.com/envoyproxy/go-control-plane/envoy/type/matcher"
	"github.com/google/go-cmp/cmp"
	"github.com/projectcontour/contour/internal/dag"
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
		want          *envoy_api_v2_auth.UpstreamTlsContext
	}{
		"no alpn, no validation": {
			want: &envoy_api_v2_auth.UpstreamTlsContext{
				CommonTlsContext: &envoy_api_v2_auth.CommonTlsContext{},
			},
		},
		"h2, no validation": {
			alpnProtocols: []string{"h2c"},
			want: &envoy_api_v2_auth.UpstreamTlsContext{
				CommonTlsContext: &envoy_api_v2_auth.CommonTlsContext{
					AlpnProtocols: []string{"h2c"},
				},
			},
		},
		"no alpn, missing altname": {
			validation: &dag.PeerValidationContext{
				CACertificate: secret,
			},
			want: &envoy_api_v2_auth.UpstreamTlsContext{
				CommonTlsContext: &envoy_api_v2_auth.CommonTlsContext{},
			},
		},
		"no alpn, missing ca": {
			validation: &dag.PeerValidationContext{
				SubjectName: "www.example.com",
			},
			want: &envoy_api_v2_auth.UpstreamTlsContext{
				CommonTlsContext: &envoy_api_v2_auth.CommonTlsContext{},
			},
		},
		"no alpn, ca and altname": {
			validation: &dag.PeerValidationContext{
				CACertificate: secret,
				SubjectName:   "www.example.com",
			},
			want: &envoy_api_v2_auth.UpstreamTlsContext{
				CommonTlsContext: &envoy_api_v2_auth.CommonTlsContext{
					ValidationContextType: &envoy_api_v2_auth.CommonTlsContext_ValidationContext{
						ValidationContext: &envoy_api_v2_auth.CertificateValidationContext{
							TrustedCa: &envoy_api_v2_core.DataSource{
								Specifier: &envoy_api_v2_core.DataSource_InlineBytes{
									InlineBytes: []byte("ca"),
								},
							},
							MatchSubjectAltNames: []*matcher.StringMatcher{{
								MatchPattern: &matcher.StringMatcher_Exact{
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
			want: &envoy_api_v2_auth.UpstreamTlsContext{
				CommonTlsContext: &envoy_api_v2_auth.CommonTlsContext{},
				Sni:              "projectcontour.local",
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := UpstreamTLSContext(tc.validation, tc.externalName, tc.alpnProtocols...)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}
