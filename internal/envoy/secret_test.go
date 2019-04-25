package envoy

import (
	"testing"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/google/go-cmp/cmp"
	"github.com/heptio/contour/internal/dag"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSecret(t *testing.T) {
	tests := map[string]struct {
		secret *dag.Secret
		want   *auth.Secret
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
			want: &auth.Secret{
				Name: "default/simple/cd1b506996",
				Type: &auth.Secret_TlsCertificate{
					TlsCertificate: &auth.TlsCertificate{
						PrivateKey: &core.DataSource{
							Specifier: &core.DataSource_InlineBytes{
								InlineBytes: []byte("key"),
							},
						},
						CertificateChain: &core.DataSource{
							Specifier: &core.DataSource_InlineBytes{
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
