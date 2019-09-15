package envoy

import (
	"crypto/sha1"
	"fmt"

	envoy_api_v2_auth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	envoy_api_v2_core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/projectcontour/contour/internal/dag"
)

// Secretname returns the name of the SDS secret for this secret.
func Secretname(s *dag.Secret) string {
	hash := sha1.Sum(s.Cert())
	ns := s.Namespace()
	name := s.Name()
	return hashname(60, ns, name, fmt.Sprintf("%x", hash[:5]))
}

// Secret creates new envoy_api_v2_auth.Secret from secret.
func Secret(s *dag.Secret) *envoy_api_v2_auth.Secret {
	return &envoy_api_v2_auth.Secret{
		Name: Secretname(s),
		Type: &envoy_api_v2_auth.Secret_TlsCertificate{
			TlsCertificate: &envoy_api_v2_auth.TlsCertificate{
				PrivateKey: &envoy_api_v2_core.DataSource{
					Specifier: &envoy_api_v2_core.DataSource_InlineBytes{
						InlineBytes: s.PrivateKey(),
					},
				},
				CertificateChain: &envoy_api_v2_core.DataSource{
					Specifier: &envoy_api_v2_core.DataSource_InlineBytes{
						InlineBytes: s.Cert(),
					},
				},
			},
		},
	}
}
