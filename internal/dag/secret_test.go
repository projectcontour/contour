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

	"github.com/stretchr/testify/assert"
	core_v1 "k8s.io/api/core/v1"

	"github.com/projectcontour/contour/internal/fixture"
)

func TestValidSecrets(t *testing.T) {
	type test struct {
		secret         *core_v1.Secret
		tlsSecretError error
		caSecretError  error
		crlSecretError error
	}
	makeTest := func(s *core_v1.Secret, tlsErr, caErr, crlErr error) *test {
		return &test{secret: s, tlsSecretError: tlsErr, caSecretError: caErr, crlSecretError: crlErr}
	}

	var (
		errEmptyCAKey        = errors.New(`empty "ca.crt" key`)
		errEmptyCRLKey       = errors.New(`empty "crl.pem" key`)
		errTLSCertMissing    = errors.New(`missing TLS certificate`)
		errInvalidSecretType = errors.New(`secret type is not "kubernetes.io/tls" or "Opaque"`)
	)

	tests := map[string]*test{
		"TLS Secret, single certificate": makeTest(
			makeTLSSecret(map[string][]byte{
				core_v1.TLSCertKey:       []byte(fixture.CERTIFICATE),
				core_v1.TLSPrivateKeyKey: []byte(fixture.RSA_PRIVATE_KEY),
			}),
			nil, errEmptyCAKey, errEmptyCRLKey),

		"TLS Secret, empty": makeTest(
			makeTLSSecret(map[string][]byte{}),
			errTLSCertMissing, errEmptyCAKey, errEmptyCRLKey),

		"TLS Secret, certificate plus CA in bundle": makeTest(
			makeTLSSecret(map[string][]byte{
				core_v1.TLSCertKey:       []byte(pemBundle(fixture.CERTIFICATE, fixture.CA_CERT)),
				core_v1.TLSPrivateKeyKey: []byte(fixture.RSA_PRIVATE_KEY),
			}),
			nil, errEmptyCAKey, errEmptyCRLKey),

		"TLS Secret, certificate plus CA with no CN in bundle": makeTest(
			makeTLSSecret(map[string][]byte{
				core_v1.TLSCertKey:       []byte(pemBundle(fixture.CERTIFICATE, fixture.CA_CERT_NO_CN)),
				core_v1.TLSPrivateKeyKey: []byte(fixture.RSA_PRIVATE_KEY),
			}),
			nil, errEmptyCAKey, errEmptyCRLKey),

		"TLS Secret, single certificate plus CA in ca.crt key": makeTest(
			makeTLSSecret(map[string][]byte{
				core_v1.TLSCertKey:       []byte(fixture.CERTIFICATE),
				core_v1.TLSPrivateKeyKey: []byte(fixture.RSA_PRIVATE_KEY),
				CACertificateKey:         []byte(fixture.CA_CERT),
			}),
			nil, nil, errEmptyCRLKey),

		"TLS Secret, single certificate plus CA with no CN in ca.crt key": makeTest(
			makeTLSSecret(map[string][]byte{
				core_v1.TLSCertKey:       []byte(fixture.CERTIFICATE),
				core_v1.TLSPrivateKeyKey: []byte(fixture.RSA_PRIVATE_KEY),
				CACertificateKey:         []byte(fixture.CA_CERT_NO_CN),
			}),
			nil, nil, errEmptyCRLKey),

		"TLS Secret, missing CN": makeTest(
			makeTLSSecret(map[string][]byte{
				core_v1.TLSCertKey:       []byte(fixture.MISSING_CN_CERT),
				core_v1.TLSPrivateKeyKey: []byte(fixture.MISSING_CN_KEY),
			}),
			errors.New(`invalid TLS certificate: certificate has no common name or subject alt name`), errEmptyCAKey, errEmptyCRLKey),

		"TLS Secret, CA cert": makeTest(
			makeTLSSecret(map[string][]byte{
				core_v1.TLSCertKey:       []byte(fixture.CA_CERT),
				core_v1.TLSPrivateKeyKey: []byte(fixture.CA_KEY),
			}),
			nil, errEmptyCAKey, errEmptyCRLKey),

		"TLS Secret, CA cert, missing CN": makeTest(
			makeTLSSecret(map[string][]byte{
				core_v1.TLSCertKey:       []byte(fixture.CA_CERT_NO_CN),
				core_v1.TLSPrivateKeyKey: []byte(fixture.CA_KEY_NO_CN),
			}),
			errors.New("invalid TLS certificate: certificate has no common name or subject alt name"), errEmptyCAKey, errEmptyCRLKey),

		"EC cert with SubjectAltName only": makeTest(
			makeTLSSecret(map[string][]byte{
				core_v1.TLSCertKey:       []byte(fixture.EC_CERTIFICATE),
				core_v1.TLSPrivateKeyKey: []byte(fixture.EC_PRIVATE_KEY),
			}),
			nil, errEmptyCAKey, errEmptyCRLKey),

		"TLS Secret, certificate, missing key": makeTest(
			makeTLSSecret(map[string][]byte{
				core_v1.TLSCertKey: []byte(fixture.CERTIFICATE),
			}),
			errors.New(`missing TLS private key`), errEmptyCAKey, errEmptyCRLKey),

		"TLS Secret, certificate, multiple keys, RSA and EC": makeTest(
			makeTLSSecret(map[string][]byte{
				core_v1.TLSCertKey:       []byte(fixture.CERTIFICATE),
				core_v1.TLSPrivateKeyKey: []byte(fixture.RSA_PRIVATE_KEY + "\n" + fixture.EC_PRIVATE_KEY + "\n" + fixture.PKCS8_PRIVATE_KEY),
			}),
			errors.New(`invalid TLS private key: multiple private keys`), errEmptyCAKey, errEmptyCRLKey),

		"TLS Secret, certificate, multiple keys, PKCS1 and PKCS8": makeTest(
			makeTLSSecret(map[string][]byte{
				core_v1.TLSCertKey:       []byte(fixture.CERTIFICATE),
				core_v1.TLSPrivateKeyKey: []byte(fixture.RSA_PRIVATE_KEY + "\n" + fixture.PKCS8_PRIVATE_KEY),
			}),
			errors.New("invalid TLS private key: multiple private keys"), errEmptyCAKey, errEmptyCRLKey),

		"TLS Secret, certificate, invalid key": makeTest(
			makeTLSSecret(map[string][]byte{
				core_v1.TLSCertKey:       []byte(fixture.CERTIFICATE),
				core_v1.TLSPrivateKeyKey: []byte("-----BEGIN RSA PRIVATE KEY-----\ninvalid\n-----END RSA PRIVATE KEY-----"),
			}),
			errors.New("invalid TLS private key: failed to parse PEM block"), errEmptyCAKey, errEmptyCRLKey),

		"TLS Secret, certificate, only EC parameters": makeTest(
			makeTLSSecret(map[string][]byte{
				core_v1.TLSCertKey:       []byte(fixture.CERTIFICATE),
				core_v1.TLSPrivateKeyKey: []byte(fixture.EC_PARAMETERS),
			}),
			errors.New("invalid TLS private key: failed to locate private key"), errEmptyCAKey, errEmptyCRLKey),

		// The next two test cases are to cover
		// #3496.
		//
		"TLS Secret, wildcard cert with different SANs": makeTest(
			makeTLSSecret(map[string][]byte{
				core_v1.TLSCertKey:       []byte(fixture.WILDCARD_CERT),
				core_v1.TLSPrivateKeyKey: []byte(fixture.WILDCARD_KEY),
			}),
			nil, errEmptyCAKey, errEmptyCRLKey),

		"TLS Secret, wildcard cert with different SANs plus CA cert": makeTest(
			makeTLSSecret(map[string][]byte{
				core_v1.TLSCertKey:       []byte(fixture.WILDCARD_CERT),
				core_v1.TLSPrivateKeyKey: []byte(fixture.WILDCARD_KEY),
				CACertificateKey:         []byte(fixture.CA_CERT),
			}),
			nil, nil, errEmptyCRLKey),

		"Opaque Secret, CA Cert": makeTest(
			makeOpaqueSecret(map[string][]byte{
				CACertificateKey: []byte(fixture.CA_CERT),
			}),
			errTLSCertMissing, nil, errEmptyCRLKey),

		"Opaque Secret, CA Cert with explanatory text": makeTest(
			makeOpaqueSecret(map[string][]byte{
				CACertificateKey: []byte(fixture.CERTIFICATE_WITH_TEXT),
			}),
			errTLSCertMissing, nil, errEmptyCRLKey),

		"Opaque Secret, CA Cert with No CN": makeTest(
			makeOpaqueSecret(map[string][]byte{
				CACertificateKey: []byte(fixture.CA_CERT_NO_CN),
			}),
			errTLSCertMissing, nil, errEmptyCRLKey),

		"Opaque Secret, CA Cert with non-PEM data": makeTest(
			makeOpaqueSecret(caBundleData(fixture.CERTIFICATE, fixture.CERTIFICATE, fixture.CERTIFICATE, fixture.CERTIFICATE)),
			errTLSCertMissing, nil, errEmptyCRLKey),

		"Opaque Secret, CA Cert with non-PEM data and no certificates": makeTest(
			makeOpaqueSecret(caBundleData()),
			errTLSCertMissing, errors.New(`invalid CA certificate bundle: failed to locate certificate`), errEmptyCRLKey),
		"Opaque Secret, zero length CA Cert": makeTest(
			makeOpaqueSecret(map[string][]byte{
				CACertificateKey: []byte(""),
			}),
			errTLSCertMissing, errEmptyCAKey, errEmptyCRLKey),

		"Opaque Secret, no CA Cert": makeTest(
			makeOpaqueSecret(map[string][]byte{
				"some-other-key": []byte("value"),
			}),
			errTLSCertMissing, errEmptyCAKey, errEmptyCRLKey),

		"Opaque Secret, with TLS Cert and Key": makeTest(
			makeOpaqueSecret(map[string][]byte{
				core_v1.TLSCertKey:       []byte(fixture.WILDCARD_CERT),
				core_v1.TLSPrivateKeyKey: []byte(fixture.WILDCARD_KEY),
				CACertificateKey:         []byte(fixture.CA_CERT),
			}),
			nil, nil, errEmptyCRLKey),

		"Opaque Secret with CRL": makeTest(
			makeOpaqueSecret(map[string][]byte{
				CRLKey: []byte(fixture.CRL),
			}),
			errTLSCertMissing, errEmptyCAKey, nil),

		"Opaque Secret with zero-length CRL": makeTest(
			makeOpaqueSecret(map[string][]byte{
				CRLKey: []byte(""),
			}),
			errTLSCertMissing, errEmptyCAKey, errEmptyCRLKey),

		"kubernetes.io/dockercfg Secret, with TLS cert, CA cert and CRL": {
			secret: &core_v1.Secret{
				Type: core_v1.SecretTypeDockercfg,
				Data: map[string][]byte{
					core_v1.TLSCertKey:       []byte(fixture.CERTIFICATE),
					core_v1.TLSPrivateKeyKey: []byte(fixture.RSA_PRIVATE_KEY),
					CACertificateKey:         []byte(fixture.CA_CERT),
					CRLKey:                   []byte(fixture.CRL),
				},
			},
			tlsSecretError: errInvalidSecretType,
			caSecretError:  errInvalidSecretType,
			crlSecretError: errInvalidSecretType,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.tlsSecretError, validTLSSecret(tc.secret))
			assert.Equal(t, tc.caSecretError, validCASecret(tc.secret))
			assert.Equal(t, tc.crlSecretError, validCRLSecret(tc.secret))
		})
	}
}

func secretdata(cert, key string) map[string][]byte {
	return map[string][]byte{
		core_v1.TLSCertKey:       []byte(cert),
		core_v1.TLSPrivateKeyKey: []byte(key),
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

func makeTLSSecret(data map[string][]byte) *core_v1.Secret {
	return &core_v1.Secret{Type: core_v1.SecretTypeTLS, Data: data}
}

func makeOpaqueSecret(data map[string][]byte) *core_v1.Secret {
	return &core_v1.Secret{Type: core_v1.SecretTypeOpaque, Data: data}
}
