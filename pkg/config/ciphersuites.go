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

package config

import (
	contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
)

// TLSCiphers holds a list of TLS ciphers
type TLSCiphers []string

// DefaultTLSCiphers contains the list of default ciphers used by Contour. A handful are
// commented out, as they're arguably less secure. They're also unnecessary
// - most of the clients that might need to use the commented ciphers are
// unable to connect without TLS 1.0, which contour never enables.
//
// This list is ignored if the client and server negotiate TLS 1.3.
//
// The commented ciphers are left in place to simplify updating this list for future
// versions of envoy.
var DefaultTLSCiphers = TLSCiphers(contour_v1alpha1.DefaultTLSCiphers)

// ValidTLSCiphers contains the list of TLS ciphers that Envoy supports
// See: https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/transport_sockets/tls/v3/common.proto#extensions-transport-sockets-tls-v3-tlsparameters
// Note: This list is a superset of what is valid for stock Envoy builds and those using BoringSSL FIPS.
var ValidTLSCiphers = contour_v1alpha1.ValidTLSCiphers

// SanitizeCipherSuites trims a list of ciphers to remove whitespace and
// duplicates, returning the passed in default if the corrected list is empty.
// The ciphers argument should be a list of valid ciphers.
func SanitizeCipherSuites(ciphers []string) []string {
	e := &contour_v1alpha1.EnvoyTLS{
		CipherSuites: ciphers,
	}
	return e.SanitizedCipherSuites()
}

// Validate ciphers. Returns error on unsupported cipher.
func (tlsCiphers TLSCiphers) Validate() error {
	e := &contour_v1alpha1.EnvoyTLS{
		CipherSuites: tlsCiphers,
	}
	return e.Validate()
}
