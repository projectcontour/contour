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

func TestValidSecrets(t *testing.T) {
	tests := map[string]struct {
		secret         *v1.Secret
		tlsSecretError error
		caSecretError  error
		crlSecretError error
	}{
		"TLS Secret, single certificate": {
			secret: &v1.Secret{
				Type: v1.SecretTypeTLS,
				Data: map[string][]byte{
					v1.TLSCertKey:       []byte(fixture.CERTIFICATE),
					v1.TLSPrivateKeyKey: []byte(fixture.RSA_PRIVATE_KEY),
				},
			},
			tlsSecretError: nil,
			caSecretError:  errors.New(`empty "ca.crt" key`),
			crlSecretError: errors.New(`empty "crl.pem" key`),
		},
		"TLS Secret, empty": {
			secret: &v1.Secret{
				Type: v1.SecretTypeTLS,
				Data: map[string][]byte{},
			},
			tlsSecretError: errors.New(`missing TLS certificate`),
			caSecretError:  errors.New(`empty "ca.crt" key`),
			crlSecretError: errors.New(`empty "crl.pem" key`),
		},
		"TLS Secret, certificate plus CA in bundle": {
			secret: &v1.Secret{
				Type: v1.SecretTypeTLS,
				Data: map[string][]byte{
					v1.TLSCertKey:       []byte(pemBundle(fixture.CERTIFICATE, fixture.CA_CERT)),
					v1.TLSPrivateKeyKey: []byte(fixture.RSA_PRIVATE_KEY),
				},
			},
			tlsSecretError: nil,
			caSecretError:  errors.New(`empty "ca.crt" key`),
			crlSecretError: errors.New(`empty "crl.pem" key`),
		},
		"TLS Secret, certificate plus CA with no CN in bundle": {
			secret: &v1.Secret{
				Type: v1.SecretTypeTLS,
				Data: map[string][]byte{
					v1.TLSCertKey:       []byte(pemBundle(fixture.CERTIFICATE, fixture.CA_CERT_NO_CN)),
					v1.TLSPrivateKeyKey: []byte(fixture.RSA_PRIVATE_KEY),
				},
			},
			tlsSecretError: nil,
			caSecretError:  errors.New(`empty "ca.crt" key`),
			crlSecretError: errors.New(`empty "crl.pem" key`),
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
			tlsSecretError: nil,
			caSecretError:  nil,
			crlSecretError: errors.New(`empty "crl.pem" key`),
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
			tlsSecretError: nil,
			caSecretError:  nil,
			crlSecretError: errors.New(`empty "crl.pem" key`),
		},
		"TLS Secret, missing CN": {
			secret: &v1.Secret{
				Type: v1.SecretTypeTLS,
				Data: map[string][]byte{
					v1.TLSCertKey:       []byte(fixture.MISSING_CN_CERT),
					v1.TLSPrivateKeyKey: []byte(fixture.MISSING_CN_KEY),
				},
			},
			tlsSecretError: errors.New(`invalid TLS certificate: certificate has no common name or subject alt name`),
			caSecretError:  errors.New(`empty "ca.crt" key`),
			crlSecretError: errors.New(`empty "crl.pem" key`),
		},
		"TLS Secret, CA cert": {
			secret: &v1.Secret{
				Type: v1.SecretTypeTLS,
				Data: map[string][]byte{
					v1.TLSCertKey:       []byte(fixture.CA_CERT),
					v1.TLSPrivateKeyKey: []byte(fixture.CA_KEY),
				},
			},
			tlsSecretError: nil,
			caSecretError:  errors.New(`empty "ca.crt" key`),
			crlSecretError: errors.New(`empty "crl.pem" key`),
		},
		"TLS Secret, CA cert, missing CN": {
			secret: &v1.Secret{
				Type: v1.SecretTypeTLS,
				Data: map[string][]byte{
					v1.TLSCertKey:       []byte(fixture.CA_CERT_NO_CN),
					v1.TLSPrivateKeyKey: []byte(fixture.CA_KEY_NO_CN),
				},
			},
			tlsSecretError: errors.New("invalid TLS certificate: certificate has no common name or subject alt name"),
			caSecretError:  errors.New(`empty "ca.crt" key`),
			crlSecretError: errors.New(`empty "crl.pem" key`),
		},

		"EC cert with SubjectAltName only": {
			secret: &v1.Secret{
				Type: v1.SecretTypeTLS,
				Data: map[string][]byte{
					v1.TLSCertKey:       []byte(fixture.EC_CERTIFICATE),
					v1.TLSPrivateKeyKey: []byte(fixture.EC_PRIVATE_KEY),
				},
			},
			tlsSecretError: nil,
			caSecretError:  errors.New(`empty "ca.crt" key`),
			crlSecretError: errors.New(`empty "crl.pem" key`),
		},
		"TLS Secret, certificate, missing key": {
			secret: &v1.Secret{
				Type: v1.SecretTypeTLS,
				Data: map[string][]byte{
					v1.TLSCertKey: []byte(fixture.CERTIFICATE),
				},
			},
			tlsSecretError: errors.New(`missing TLS private key`),
			caSecretError:  errors.New(`empty "ca.crt" key`),
			crlSecretError: errors.New(`empty "crl.pem" key`),
		},
		"TLS Secret, certificate, multiple keys, RSA and EC": {
			secret: &v1.Secret{
				Type: v1.SecretTypeTLS,
				Data: map[string][]byte{
					v1.TLSCertKey:       []byte(fixture.CERTIFICATE),
					v1.TLSPrivateKeyKey: []byte(fixture.RSA_PRIVATE_KEY + "\n" + fixture.EC_PRIVATE_KEY + "\n" + fixture.PKCS8_PRIVATE_KEY),
				},
			},
			tlsSecretError: errors.New(`invalid TLS private key: multiple private keys`),
			caSecretError:  errors.New(`empty "ca.crt" key`),
			crlSecretError: errors.New(`empty "crl.pem" key`),
		},
		"TLS Secret, certificate, multiple keys, PKCS1 and PKCS8": {
			secret: &v1.Secret{
				Type: v1.SecretTypeTLS,
				Data: map[string][]byte{
					v1.TLSCertKey:       []byte(fixture.CERTIFICATE),
					v1.TLSPrivateKeyKey: []byte(fixture.RSA_PRIVATE_KEY + "\n" + fixture.PKCS8_PRIVATE_KEY),
				},
			},
			tlsSecretError: errors.New("invalid TLS private key: multiple private keys"),
			caSecretError:  errors.New(`empty "ca.crt" key`),
			crlSecretError: errors.New(`empty "crl.pem" key`),
		},
		"TLS Secret, certificate, invalid key": {
			secret: &v1.Secret{
				Type: v1.SecretTypeTLS,
				Data: map[string][]byte{
					v1.TLSCertKey:       []byte(fixture.CERTIFICATE),
					v1.TLSPrivateKeyKey: []byte("-----BEGIN RSA PRIVATE KEY-----\ninvalid\n-----END RSA PRIVATE KEY-----"),
				},
			},
			tlsSecretError: errors.New("invalid TLS private key: failed to parse PEM block"),
			caSecretError:  errors.New(`empty "ca.crt" key`),
			crlSecretError: errors.New(`empty "crl.pem" key`),
		},
		"TLS Secret, certificate, only EC parameters": {
			secret: &v1.Secret{
				Type: v1.SecretTypeTLS,
				Data: map[string][]byte{
					v1.TLSCertKey:       []byte(fixture.CERTIFICATE),
					v1.TLSPrivateKeyKey: []byte(fixture.EC_PARAMETERS),
				},
			},
			tlsSecretError: errors.New("invalid TLS private key: failed to locate private key"),
			caSecretError:  errors.New(`empty "ca.crt" key`),
			crlSecretError: errors.New(`empty "crl.pem" key`),
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
			tlsSecretError: nil,
			caSecretError:  errors.New(`empty "ca.crt" key`),
			crlSecretError: errors.New(`empty "crl.pem" key`),
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
			tlsSecretError: nil,
			caSecretError:  nil,
			crlSecretError: errors.New(`empty "crl.pem" key`),
		},
		"Opaque Secret, CA Cert": {
			secret: &v1.Secret{
				Type: v1.SecretTypeOpaque,
				Data: map[string][]byte{
					CACertificateKey: []byte(fixture.CA_CERT),
				},
			},
			tlsSecretError: errors.New(`secret type is not "kubernetes.io/tls"`),
			caSecretError:  nil,
			crlSecretError: errors.New(`empty "crl.pem" key`),
		},
		"Opaque Secret, CA Cert with explanatory text": {
			secret: &v1.Secret{
				Type: v1.SecretTypeOpaque,
				Data: map[string][]byte{
					CACertificateKey: []byte(fixture.CERTIFICATE_WITH_TEXT),
				},
			},
			tlsSecretError: errors.New(`secret type is not "kubernetes.io/tls"`),
			caSecretError:  nil,
			crlSecretError: errors.New(`empty "crl.pem" key`),
		},
		"Opaque Secret, CA Cert with No CN": {
			secret: &v1.Secret{
				Type: v1.SecretTypeOpaque,
				Data: map[string][]byte{
					CACertificateKey: []byte(fixture.CA_CERT_NO_CN),
				},
			},
			tlsSecretError: errors.New(`secret type is not "kubernetes.io/tls"`),
			caSecretError:  nil,
			crlSecretError: errors.New(`empty "crl.pem" key`),
		},
		"Opaque Secret, CA Cert with non-PEM data": {
			secret: &v1.Secret{
				Type: v1.SecretTypeOpaque,
				Data: caBundleData(fixture.CERTIFICATE, fixture.CERTIFICATE, fixture.CERTIFICATE, fixture.CERTIFICATE),
			},
			tlsSecretError: errors.New(`secret type is not "kubernetes.io/tls"`),
			caSecretError:  nil,
			crlSecretError: errors.New(`empty "crl.pem" key`),
		},
		"Opaque Secret, CA Cert with non-PEM data and no certificates": {
			secret: &v1.Secret{
				Type: v1.SecretTypeOpaque,
				Data: caBundleData(),
			},
			tlsSecretError: errors.New(`secret type is not "kubernetes.io/tls"`),
			caSecretError:  errors.New(`invalid CA certificate bundle: failed to locate certificate`),
			crlSecretError: errors.New(`empty "crl.pem" key`),
		},
		"Opaque Secret, zero length CA Cert": {
			secret: &v1.Secret{
				Type: v1.SecretTypeOpaque,
				Data: map[string][]byte{
					CACertificateKey: []byte(""),
				},
			},
			tlsSecretError: errors.New(`secret type is not "kubernetes.io/tls"`),
			caSecretError:  errors.New(`empty "ca.crt" key`),
			crlSecretError: errors.New(`empty "crl.pem" key`),
		},
		"Opaque Secret, no CA Cert": {
			secret: &v1.Secret{
				Type: v1.SecretTypeOpaque,
				Data: map[string][]byte{
					"some-other-key": []byte("value"),
				},
			},
			tlsSecretError: errors.New(`secret type is not "kubernetes.io/tls"`),
			caSecretError:  errors.New(`empty "ca.crt" key`),
			crlSecretError: errors.New(`empty "crl.pem" key`),
		},
		"Opaque Secret, with TLS Cert and Key": {
			secret: &v1.Secret{
				Type: v1.SecretTypeOpaque,
				Data: map[string][]byte{
					v1.TLSCertKey:       []byte(fixture.WILDCARD_CERT),
					v1.TLSPrivateKeyKey: []byte(fixture.WILDCARD_KEY),
					CACertificateKey:    []byte(fixture.CA_CERT),
				},
			},
			tlsSecretError: errors.New(`secret type is not "kubernetes.io/tls"`),
			caSecretError:  nil,
			crlSecretError: errors.New(`empty "crl.pem" key`),
		},
		"Opaque Secret with CRL": {
			secret: &v1.Secret{
				Type: v1.SecretTypeOpaque,
				Data: map[string][]byte{
					CRLKey: []byte(fixture.CRL),
				},
			},
			tlsSecretError: errors.New(`secret type is not "kubernetes.io/tls"`),
			caSecretError:  errors.New(`empty "ca.crt" key`),
			crlSecretError: nil,
		},
		"Opaque Secret with zero-length CRL": {
			secret: &v1.Secret{
				Type: v1.SecretTypeOpaque,
				Data: map[string][]byte{
					CRLKey: []byte(""),
				},
			},
			tlsSecretError: errors.New(`secret type is not "kubernetes.io/tls"`),
			caSecretError:  errors.New(`empty "ca.crt" key`),
			crlSecretError: errors.New(`empty "crl.pem" key`),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			s := &Secret{Object: tc.secret}

			assert.Equal(t, tc.tlsSecretError, validTLSSecret(s))
			assert.Equal(t, tc.caSecretError, validCA(s))
			assert.Equal(t, tc.crlSecretError, validCRL(s))
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
