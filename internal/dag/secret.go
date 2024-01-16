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

	core_v1 "k8s.io/api/core/v1"
)

const (
	// CACertificateKey is the key name for accessing TLS CA certificate bundles in Kubernetes Secrets.
	CACertificateKey = "ca.crt"

	// CRLKey is the key name for accessing CRL bundles in Kubernetes Secrets.
	CRLKey = "crl.pem"
)

// validTLSSecret returns an error if the Secret is not of type TLS or Opaque or
// if it doesn't contain valid certificate and private key material in
// the tls.crt and tls.key keys.
func validTLSSecret(secret *core_v1.Secret) error {
	if secret.Type != core_v1.SecretTypeTLS && secret.Type != core_v1.SecretTypeOpaque {
		return fmt.Errorf("secret type is not %q or %q", core_v1.SecretTypeTLS, core_v1.SecretTypeOpaque)
	}

	data, ok := secret.Data[core_v1.TLSCertKey]
	if !ok {
		return errors.New("missing TLS certificate")
	}

	if err := validateServingBundle(data); err != nil {
		return fmt.Errorf("invalid TLS certificate: %v", err)
	}

	data, ok = secret.Data[core_v1.TLSPrivateKeyKey]
	if !ok {
		return errors.New("missing TLS private key")
	}

	if err := validatePrivateKey(data); err != nil {
		return fmt.Errorf("invalid TLS private key: %v", err)
	}

	return nil
}

// validCASecret returns an error if the Secret is not of type TLS or Opaque or
// if it doesn't contain a valid CA bundle in the ca.crt key.
func validCASecret(secret *core_v1.Secret) error {
	if secret.Type != core_v1.SecretTypeTLS && secret.Type != core_v1.SecretTypeOpaque {
		return fmt.Errorf("secret type is not %q or %q", core_v1.SecretTypeTLS, core_v1.SecretTypeOpaque)
	}

	if len(secret.Data[CACertificateKey]) == 0 {
		return fmt.Errorf("empty %q key", CACertificateKey)
	}

	if err := validateCABundle(secret.Data[CACertificateKey]); err != nil {
		return fmt.Errorf("invalid CA certificate bundle: %v", err)
	}

	return nil
}

// validCRLSecret returns an error if the Secret is not of type TLS or Opaque or
// if it doesn't contain a valid CRL in the crl.pem key.
func validCRLSecret(secret *core_v1.Secret) error {
	if secret.Type != core_v1.SecretTypeTLS && secret.Type != core_v1.SecretTypeOpaque {
		return fmt.Errorf("secret type is not %q or %q", core_v1.SecretTypeTLS, core_v1.SecretTypeOpaque)
	}

	if len(secret.Data[CRLKey]) == 0 {
		return fmt.Errorf("empty %q key", CRLKey)
	}

	if err := validateCRL(secret.Data[CRLKey]); err != nil {
		return err
	}

	return nil
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

// validateServingBundle validates that a PEM bundle contains at least one
// certificate, and that the first certificate has a
// CN or SAN set.
func validateServingBundle(data []byte) error {
	var exists bool

	// The first PEM in a bundle should always have a CN set.
	i := 0

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

		// Only run the CN and SAN checks on the first cert in a bundle
		if i == 0 && !hasCommonName(cert) && !hasSubjectAltNames(cert) {
			return errors.New("certificate has no common name or subject alt name")
		}

		exists = true
		i++
	}

	if !exists {
		return errors.New("failed to locate certificate")
	}

	return nil
}

// validateCABundle validates that a PEM bundle contains at least
// one valid certificate.
func validateCABundle(data []byte) error {
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
			if _, err := x509.ParsePKCS8PrivateKey(block.Bytes); err != nil {
				return err
			}
			keys++
		case "RSA PRIVATE KEY":
			if _, err := x509.ParsePKCS1PrivateKey(block.Bytes); err != nil {
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

// validateCRL checks that PEM file contains at least one CRL.
func validateCRL(data []byte) error {
	for containsPEMHeader(data) {
		var block *pem.Block
		block, data = pem.Decode(data)
		if block == nil {
			return errors.New("failed to parse PEM block")
		}
		if block.Type == "X509 CRL" {
			return nil
		}
	}

	return errors.New("failed to locate CRL")
}
