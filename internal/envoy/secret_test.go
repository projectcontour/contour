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
	"github.com/google/go-cmp/cmp"
	"github.com/projectcontour/contour/internal/dag"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSecret(t *testing.T) {
	tests := map[string]struct {
		secret *dag.Secret
		want   *envoy_api_v2_auth.Secret
	}{
		"simple secret": {
			secret: &dag.Secret{
				Object: &v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Data: map[string][]byte{
						v1.TLSCertKey:       []byte("cert"),
						v1.TLSPrivateKeyKey: []byte("key"),
					},
				},
			},
			want: &envoy_api_v2_auth.Secret{
				Name: "default/simple/cd1b506996",
				Type: &envoy_api_v2_auth.Secret_TlsCertificate{
					TlsCertificate: &envoy_api_v2_auth.TlsCertificate{
						PrivateKey: &envoy_api_v2_core.DataSource{
							Specifier: &envoy_api_v2_core.DataSource_InlineBytes{
								InlineBytes: []byte("key"),
							},
						},
						CertificateChain: &envoy_api_v2_core.DataSource{
							Specifier: &envoy_api_v2_core.DataSource_InlineBytes{
								InlineBytes: []byte("cert"),
							},
						},
					},
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := Secret(tc.secret)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func TestSecretname(t *testing.T) {
	tests := map[string]struct {
		secret *dag.Secret
		want   string
	}{
		"simple": {
			secret: &dag.Secret{
				Object: &v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Data: map[string][]byte{
						v1.TLSCertKey:       []byte("cert"),
						v1.TLSPrivateKeyKey: []byte("key"),
					},
				},
			},
			want: "default/simple/cd1b506996",
		},
		"far too long": {
			secret: &dag.Secret{
				Object: &v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "must-be-in-want-of-a-wife",
						Namespace: "it-is-a-truth-universally-acknowledged-that-a-single-man-in-possession-of-a-good-fortune",
					},
					Data: map[string][]byte{
						v1.TLSCertKey:       []byte("cert"),
						v1.TLSPrivateKeyKey: []byte("key"),
					},
				},
			},
			want: "it-is-a-truth-7e164b/must-be-in-wa-7e164b/cd1b506996",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := Secretname(tc.secret)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}
