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
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"

	v1 "k8s.io/api/core/v1"
)

// CACertificateKey is the key name for accessing TLS CA certificate bundles in Kubernetes Secrets.
const CACertificateKey = "ca.crt"

// isValidSecret returns true if the secret is interesting and well
// formed. TLS certificate/key pairs must be secrets of type
// "kubernetes.io/tls". Certificate bundles may be "kubernetes.io/tls"
// or generic (type "Opaque" or "") secrets.
func isValidSecret(secret *v1.Secret) (bool, error) {
	switch secret.Type {
	// We will accept TLS secrets that also have the 'ca.crt' payload.
	case v1.SecretTypeTLS:
		data, ok := secret.Data[v1.TLSCertKey]
		if !ok {
			return false, errors.New("missing TLS certificate")
		}

		if err := validateCertificate(data); err != nil {
			return false, fmt.Errorf("invalid TLS certificate: %v", err)
		}

		data, ok = secret.Data[v1.TLSPrivateKeyKey]
		if !ok {
			return false, errors.New("missing TLS private key")
		}

		if err := validatePrivateKey(data); err != nil {
			return false, fmt.Errorf("invalid TLS private key: %v", err)
		}

	// Generic secrets may have a 'ca.crt' only.
	case v1.SecretTypeOpaque, "":
		if _, ok := secret.Data[v1.TLSCertKey]; ok {
			return false, nil
		}

		if _, ok := secret.Data[v1.TLSPrivateKeyKey]; ok {
			return false, nil
		}

		if data := secret.Data[CACertificateKey]; len(data) == 0 {
			return false, nil
		}

	default:
		return false, nil

	}

	// If the secret we propose to accept has a CA bundle key,
	// validate that it is PEM certificate(s). Note that the
	// CA bundle on TLS secrets is allowed to be an empty string
	// (see https://github.com/projectcontour/contour/issues/1644).
	if data := secret.Data[CACertificateKey]; len(data) > 0 {
		if err := validateCertificate(data); err != nil {
			return false, fmt.Errorf("invalid CA certificate bundle: %v", err)
		}
	}

	return true, nil
}

// containsPEMHeader returns true if the given slice contains a string
// that looks like a PEM header block. The problem is that pem.Decode
// does not give us a way to distinguish between a missing PEM block
// and an invalid PEM block. This means that if there is any non-PEM
// data at the end of a byte slice, we would normally detect it as an
// error. However, users of the OpenSSL API check for the
// `PEM_R_NO_START_LINE` error code and would accept files with
// trailing non-PEM data.
func containsPEMHeader(data []byte) bool {
	// A PEM header starts with the begin token.
	start := bytes.Index(data, []byte("-----BEGIN"))
	if start == -1 {
		return false
	}

	// And ends with the end token.
	end := bytes.Index(data[start+10:], []byte("-----"))
	if end == -1 {
		return false
	}

	// And must be on a single line.
	if bytes.Contains(data[start:start+end], []byte("\n")) {
		return false
	}

	return true
}

func validateCertificate(data []byte) error {
	var exists bool

	for containsPEMHeader(data) {
		var block *pem.Block
		block, data = pem.Decode(data)
		if block == nil {
			return errors.New("failed to parse PEM block")
		}
		if block.Type != "CERTIFICATE" {
			return fmt.Errorf("unexpected block type '%s'", block.Type)
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return err
		}

		if !hasCommonName(cert) && !hasSubjectAltNames(cert) {
			return errors.New("certificate has no common name or subject alt name")
		}

		exists = true
	}

	if !exists {
		return errors.New("failed to locate certificate")
	}

	return nil
}

func hasCommonName(c *x509.Certificate) bool {
	return strings.TrimSpace(c.Subject.CommonName) != ""
}

func hasSubjectAltNames(c *x509.Certificate) bool {
	return len(c.DNSNames) > 0 || len(c.IPAddresses) > 0
}

func validatePrivateKey(data []byte) error {
	var keys int

	for containsPEMHeader(data) {
		var block *pem.Block
		block, data = pem.Decode(data)
		if block == nil {
			return errors.New("failed to parse PEM block")
		}
		switch block.Type {
		case "PRIVATE KEY":
			if _, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
				return err
			}
			keys++
		case "RSA PRIVATE KEY":
			if _, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
				return err
			}
			keys++
		case "EC PRIVATE KEY":
			if _, err := x509.ParseECPrivateKey(block.Bytes); err != nil {
				return err
			}
			keys++
		case "EC PARAMETERS":
			// ignored
		default:
			return fmt.Errorf("unexpected block type '%s'", block.Type)
		}
	}

	switch keys {
	case 0:
		return errors.New("failed to locate private key")
	case 1:
		return nil
	default:
		return errors.New("multiple private keys")
	}
}
