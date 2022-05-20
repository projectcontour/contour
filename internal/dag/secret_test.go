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

package dag

import (
	"errors"
	"fmt"
	"testing"

	"github.com/projectcontour/contour/internal/fixture"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
)

func TestIsValidSecret(t *testing.T) {
	tests := map[string]struct {
		secret *v1.Secret
		valid  bool
		err    error
	}{
		// TLS Secret, single cert
		"TLS Secret, single certificate": {
			secret: &v1.Secret{
				Type: v1.SecretTypeTLS,
				Data: map[string][]byte{
					v1.TLSCertKey:       []byte(fixture.CERTIFICATE),
					v1.TLSPrivateKeyKey: []byte(fixture.RSA_PRIVATE_KEY),
				},
			},
			valid: true,
			err:   nil,
		},
		"TLS Secret, empty": {
			secret: &v1.Secret{
				Type: v1.SecretTypeTLS,
				Data: map[string][]byte{},
			},
			valid: false,
			err:   errors.New("missing TLS certificate"),
		},
		"TLS Secret, certificate plus CA in bundle": {
			secret: &v1.Secret{
				Type: v1.SecretTypeTLS,
				Data: map[string][]byte{
					v1.TLSCertKey:       []byte(pemBundle(fixture.CERTIFICATE, fixture.CA_CERT)),
					v1.TLSPrivateKeyKey: []byte(fixture.RSA_PRIVATE_KEY),
				},
			},
			valid: true,
			err:   nil,
		},
		"TLS Secret, certificate plus CA with no CN in bundle": {
			secret: &v1.Secret{
				Type: v1.SecretTypeTLS,
				Data: map[string][]byte{
					v1.TLSCertKey:       []byte(pemBundle(fixture.CERTIFICATE, fixture.CA_CERT_NO_CN)),
					v1.TLSPrivateKeyKey: []byte(fixture.RSA_PRIVATE_KEY),
				},
			},
			valid: true,
			err:   nil,
		},
		"TLS Secret, single certificate plus CA in ca.crt key": {
			secret: &v1.Secret{
				Type: v1.SecretTypeTLS,
				Data: map[string][]byte{
					v1.TLSCertKey:       []byte(fixture.CERTIFICATE),
					v1.TLSPrivateKeyKey: []byte(fixture.RSA_PRIVATE_KEY),
					CACertificateKey:    []byte(fixture.CA_CERT),
				},
			},
			valid: true,
			err:   nil,
		},
		"TLS Secret, single certificate plus CA with no CN in ca.crt key": {
			secret: &v1.Secret{
				Type: v1.SecretTypeTLS,
				Data: map[string][]byte{
					v1.TLSCertKey:       []byte(fixture.CERTIFICATE),
					v1.TLSPrivateKeyKey: []byte(fixture.RSA_PRIVATE_KEY),
					CACertificateKey:    []byte(fixture.CA_CERT_NO_CN),
				},
			},
			valid: true,
			err:   nil,
		},
		"TLS Secret, missing CN": {
			secret: &v1.Secret{
				Type: v1.SecretTypeTLS,
				Data: map[string][]byte{
					v1.TLSCertKey:       []byte(fixture.MISSING_CN_CERT),
					v1.TLSPrivateKeyKey: []byte(fixture.MISSING_CN_KEY),
				},
			},
			valid: false,
			err:   errors.New("invalid TLS certificate: certificate has no common name or subject alt name"),
		},
		"TLS Secret, CA cert": {
			secret: &v1.Secret{
				Type: v1.SecretTypeTLS,
				Data: map[string][]byte{
					v1.TLSCertKey:       []byte(fixture.CA_CERT),
					v1.TLSPrivateKeyKey: []byte(fixture.CA_KEY),
				},
			},
			valid: true,
			err:   nil,
		},
		"TLS Secret, CA cert, missing CN": {
			secret: &v1.Secret{
				Type: v1.SecretTypeTLS,
				Data: map[string][]byte{
					v1.TLSCertKey:       []byte(fixture.CA_CERT_NO_CN),
					v1.TLSPrivateKeyKey: []byte(fixture.CA_KEY_NO_CN),
				},
			},
			valid: false,
			err:   errors.New("invalid TLS certificate: certificate has no common name or subject alt name"),
		},

		"EC cert with SubjectAltName only": {
			secret: &v1.Secret{
				Type: v1.SecretTypeTLS,
				Data: map[string][]byte{
					v1.TLSCertKey:       []byte(fixture.EC_CERTIFICATE),
					v1.TLSPrivateKeyKey: []byte(fixture.EC_PRIVATE_KEY),
				},
			},
			valid: true,
			err:   nil,
		},
		"TLS Secret, certificate, missing key": {
			secret: &v1.Secret{
				Type: v1.SecretTypeTLS,
				Data: map[string][]byte{
					v1.TLSCertKey: []byte(fixture.CERTIFICATE),
				},
			},
			valid: false,
			err:   errors.New("missing TLS private key"),
		},
		"TLS Secret, certificate, multiple keys, RSA and EC": {
			secret: &v1.Secret{
				Type: v1.SecretTypeTLS,
				Data: map[string][]byte{
					v1.TLSCertKey:       []byte(fixture.CERTIFICATE),
					v1.TLSPrivateKeyKey: []byte(fixture.RSA_PRIVATE_KEY + "\n" + fixture.EC_PRIVATE_KEY + "\n" + fixture.PKCS8_PRIVATE_KEY),
				},
			},
			valid: false,
			err:   errors.New("invalid TLS private key: multiple private keys"),
		},
		"TLS Secret, certificate, multiple keys, PKCS1 and PKCS8": {
			secret: &v1.Secret{
				Type: v1.SecretTypeTLS,
				Data: map[string][]byte{
					v1.TLSCertKey:       []byte(fixture.CERTIFICATE),
					v1.TLSPrivateKeyKey: []byte(fixture.RSA_PRIVATE_KEY + "\n" + fixture.PKCS8_PRIVATE_KEY),
				},
			},
			valid: false,
			err:   errors.New("invalid TLS private key: multiple private keys"),
		},
		"TLS Secret, certificate, invalid key": {
			secret: &v1.Secret{
				Type: v1.SecretTypeTLS,
				Data: map[string][]byte{
					v1.TLSCertKey:       []byte(fixture.CERTIFICATE),
					v1.TLSPrivateKeyKey: []byte("-----BEGIN RSA PRIVATE KEY-----\ninvalid\n-----END RSA PRIVATE KEY-----"),
				},
			},
			valid: false,
			err:   errors.New("invalid TLS private key: failed to parse PEM block"),
		},
		"TLS Secret, certificate, only EC parameters": {
			secret: &v1.Secret{
				Type: v1.SecretTypeTLS,
				Data: map[string][]byte{
					v1.TLSCertKey:       []byte(fixture.CERTIFICATE),
					v1.TLSPrivateKeyKey: []byte(fixture.EC_PARAMETERS),
				},
			},
			valid: false,
			err:   errors.New("invalid TLS private key: failed to locate private key"),
		},
		// The next two test cases are to cover
		// #3496.
		//
		"TLS Secret, wildcard cert with different SANs": {
			secret: &v1.Secret{
				Type: v1.SecretTypeTLS,
				Data: map[string][]byte{
					v1.TLSCertKey:       []byte(fixture.WILDCARD_CERT),
					v1.TLSPrivateKeyKey: []byte(fixture.WILDCARD_KEY),
				},
			},
			valid: true,
			err:   nil,
		},
		"TLS Secret, wildcard cert with different SANs plus CA cert": {
			secret: &v1.Secret{
				Type: v1.SecretTypeTLS,
				Data: map[string][]byte{
					v1.TLSCertKey:       []byte(fixture.WILDCARD_CERT),
					v1.TLSPrivateKeyKey: []byte(fixture.WILDCARD_KEY),
					CACertificateKey:    []byte(fixture.CA_CERT),
				},
			},
			valid: true,
			err:   nil,
		},
		"Opaque Secret, CA Cert": {
			secret: &v1.Secret{
				Type: v1.SecretTypeOpaque,
				Data: map[string][]byte{
					CACertificateKey: []byte(fixture.CA_CERT),
				},
			},
			valid: true,
			err:   nil,
		},
		// Opaque Secret, CA Cert with No CN
		"Opaque Secret, CA Cert with No CN": {
			secret: &v1.Secret{
				Type: v1.SecretTypeOpaque,
				Data: map[string][]byte{
					CACertificateKey: []byte(fixture.CA_CERT_NO_CN),
				},
			},
			valid: true,
			err:   nil,
		},
		"Opaque Secret, zero length CA Cert": {
			secret: &v1.Secret{
				Type: v1.SecretTypeOpaque,
				Data: map[string][]byte{
					CACertificateKey: []byte(""),
				},
			},
			valid: false,
			err:   errors.New("can't use zero-length ca.crt value"),
		},
		// Opaque Secret with TLS cert details won't be added.
		"Opaque Secret, with TLS Cert and Key": {
			secret: &v1.Secret{
				Type: v1.SecretTypeOpaque,
				Data: map[string][]byte{
					v1.TLSCertKey:       []byte(fixture.WILDCARD_CERT),
					v1.TLSPrivateKeyKey: []byte(fixture.WILDCARD_KEY),
					CACertificateKey:    []byte(fixture.CA_CERT),
				},
			},
			valid: false,
			err:   nil,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			type Result struct {
				Valid bool
				Err   error
			}

			want := Result{Valid: tc.valid, Err: tc.err}

			valid, err := isValidSecret(tc.secret)
			got := Result{Valid: valid, Err: err}

			assert.Equal(t, want, got)
		})
	}
}

func secretdata(cert, key string) map[string][]byte {
	return map[string][]byte{
		v1.TLSCertKey:       []byte(cert),
		v1.TLSPrivateKeyKey: []byte(key),
	}
}

// caBundleData returns a CA certificate bundle map whose value is
// the given set of PEM certificates intermingled with some non-PEM
// data.
//
// See also: https://tools.ietf.org/html/rfc7468#section-5.2
func caBundleData(cert ...string) map[string][]byte {
	var data string

	data += "start of CA bundle\n"

	for n, c := range cert {
		data += fmt.Sprintf("certificate %d\n", n)
		data += c
		data += "\n"
	}

	data += "end of CA bundle\n"

	return map[string][]byte{
		CACertificateKey: []byte(data),
	}
}

// pemBundle concatenates supplied PEM strings
// into a valid PEM bundle (just add newline!)
func pemBundle(cert ...string) string {
	var data string
	for _, c := range cert {
		data += c
		data += "\n"
	}

	return data
}
