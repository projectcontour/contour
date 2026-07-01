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

package v1alpha1

// DefaultECDHCurves contains the list of default ECDH curves used by Contour.
// When this list is empty/nil, Envoy will use its built-in defaults (X25519, P-256).
//
// We deliberately leave this nil so that Envoy's compiled-in defaults are used,
// which track BoringSSL's safe defaults. Users who want to enable post-quantum
// key exchange (e.g. X25519MLKEM768) can explicitly configure this field.
var DefaultECDHCurves []string

// ValidECDHCurves contains the set of ECDH curves that Envoy supports.
// See Envoy configuration: https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/transport_sockets/tls/v3/common.proto#extensions-transport-sockets-tls-v3-tlsparameters
var ValidECDHCurves = map[string]struct{}{
	"X25519":         {},
	"P-256":          {},
	"P-384":          {},
	"P-521":          {},
	"X25519MLKEM768": {},
}
