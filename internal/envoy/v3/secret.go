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
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_transport_socket_tls_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"

	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/envoy"
)

// Secret creates new envoy_transport_socket_tls_v3.Secret from secret.
func Secret(s *dag.Secret) *envoy_transport_socket_tls_v3.Secret {
	return &envoy_transport_socket_tls_v3.Secret{
		Name: envoy.Secretname(s),
		Type: &envoy_transport_socket_tls_v3.Secret_TlsCertificate{
			TlsCertificate: &envoy_transport_socket_tls_v3.TlsCertificate{
				PrivateKey: &envoy_config_core_v3.DataSource{
					Specifier: &envoy_config_core_v3.DataSource_InlineBytes{
						InlineBytes: s.PrivateKey(),
					},
				},
				CertificateChain: &envoy_config_core_v3.DataSource{
					Specifier: &envoy_config_core_v3.DataSource_InlineBytes{
						InlineBytes: s.Cert(),
					},
				},
			},
		},
	}
}
