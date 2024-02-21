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
	envoy_transport_socket_tls_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
)

func ParseTLSVersion(version string) envoy_transport_socket_tls_v3.TlsParameters_TlsProtocol {
	switch version {
	case "1.2":
		return envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2
	case "1.3":
		return envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3
	default:
		return envoy_transport_socket_tls_v3.TlsParameters_TLS_AUTO
	}
}
