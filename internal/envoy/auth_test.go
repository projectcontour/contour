// Copyright © 2019 VMware
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
	"github.com/google/go-cmp/cmp"
)

func TestUpstreamTLSContext(t *testing.T) {
	tests := map[string]struct {
		ca            []byte
		subjectName   string
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
			ca: []byte("ca"),
			want: &envoy_api_v2_auth.UpstreamTlsContext{
				CommonTlsContext: &envoy_api_v2_auth.CommonTlsContext{},
			},
		},
		"no alpn, missing ca": {
			subjectName: "www.example.com",
			want: &envoy_api_v2_auth.UpstreamTlsContext{
				CommonTlsContext: &envoy_api_v2_auth.CommonTlsContext{},
			},
		},
		"no alpn, ca and altname": {
			ca:          []byte("ca"),
			subjectName: "www.example.com",
			want: &envoy_api_v2_auth.UpstreamTlsContext{
				CommonTlsContext: &envoy_api_v2_auth.CommonTlsContext{
					ValidationContextType: &envoy_api_v2_auth.CommonTlsContext_ValidationContext{
						ValidationContext: &envoy_api_v2_auth.CertificateValidationContext{
							TrustedCa: &envoy_api_v2_core.DataSource{
								Specifier: &envoy_api_v2_core.DataSource_InlineBytes{
									InlineBytes: []byte("ca"),
								},
							},
							VerifySubjectAltName: []string{"www.example.com"},
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
			got := UpstreamTLSContext(tc.ca, tc.subjectName, tc.externalName, tc.alpnProtocols...)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}
