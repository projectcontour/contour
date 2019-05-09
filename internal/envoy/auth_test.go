// Copyright © 2019 Heptio
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

	"github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/google/go-cmp/cmp"
)

func TestUpstreamTLSContext(t *testing.T) {
	tests := map[string]struct {
		ca            []byte
		subjectName   string
		alpnProtocols []string
		want          *auth.UpstreamTlsContext
	}{
		"no alpn, no validation": {
			want: &auth.UpstreamTlsContext{
				CommonTlsContext: &auth.CommonTlsContext{},
			},
		},
		"h2, no validation": {
			alpnProtocols: []string{"h2c"},
			want: &auth.UpstreamTlsContext{
				CommonTlsContext: &auth.CommonTlsContext{
					AlpnProtocols: []string{"h2c"},
				},
			},
		},
		"no alpn, missing altname": {
			ca: []byte("ca"),
			want: &auth.UpstreamTlsContext{
				CommonTlsContext: &auth.CommonTlsContext{},
			},
		},
		"no alpn, missing ca": {
			subjectName: "www.example.com",
			want: &auth.UpstreamTlsContext{
				CommonTlsContext: &auth.CommonTlsContext{},
			},
		},
		"no alpn, ca and altname": {
			ca:          []byte("ca"),
			subjectName: "www.example.com",
			want: &auth.UpstreamTlsContext{
				CommonTlsContext: &auth.CommonTlsContext{
					ValidationContextType: &auth.CommonTlsContext_ValidationContext{
						ValidationContext: &auth.CertificateValidationContext{
							TrustedCa: &core.DataSource{
								Specifier: &core.DataSource_InlineBytes{
									InlineBytes: []byte("ca"),
								},
							},
							VerifySubjectAltName: []string{"www.example.com"},
						},
					},
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := UpstreamTLSContext(tc.ca, tc.subjectName, tc.alpnProtocols...)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}
