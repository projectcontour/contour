package envoy

import (
	"crypto/sha1"
	"fmt"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/heptio/contour/internal/dag"
)

// Secretname returns the name of the SDS secret for this secret.
func Secretname(s *dag.Secret) string {
	hash := sha1.Sum(s.Cert())
	ns := s.Namespace()
	name := s.Name()
	return hashname(60, ns, name, fmt.Sprintf("%x", hash[:5]))
}

// Secret creates new v2auth.Secret from secret.
func Secret(s *dag.Secret) *auth.Secret {
	return &auth.Secret{
		Name: Secretname(s),
		Type: &auth.Secret_TlsCertificate{
			TlsCertificate: &auth.TlsCertificate{
				PrivateKey: &core.DataSource{
					Specifier: &core.DataSource_InlineBytes{
						InlineBytes: s.PrivateKey(),
					},
				},
				CertificateChain: &core.DataSource{
					Specifier: &core.DataSource_InlineBytes{
						InlineBytes: s.Cert(),
					},
				},
			},
		},
	}
}
