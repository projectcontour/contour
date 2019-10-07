// Copyright © 2019 VMware
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
	"crypto/x509"
	"encoding/pem"
	"errors"

	v1 "k8s.io/api/core/v1"
)

// isValidSecret returns true if the secret is interesting and well formed.
func isValidSecret(secret *v1.Secret) (bool, error) {
	if secret.Type == v1.SecretTypeServiceAccountToken {
		// ignore service account tokens, see #1419
		return false, nil
	}
	if _, hasCA := secret.Data["ca.crt"]; secret.Type != v1.SecretTypeTLS && !hasCA {
		// ignore everything but kubernetes.io/tls secrets
		// and secrets with a ca.crt key.
		return false, nil
	}
	for key, data := range secret.Data {
		switch key {
		case v1.TLSCertKey:
			if err := validateCertificate(data); err != nil {
				return false, err
			}
		case v1.TLSPrivateKeyKey:
			if err := validatePrivateKey(data); err != nil {
				return false, err
			}
		case "ca.crt":
			// nothing yet, see #1644
		}
	}
	return true, nil
}

func validateCertificate(data []byte) error {
	cert, err := decodePEM(data)
	if err != nil {
		return err
	}
	_, err = x509.ParseCertificate(cert.Bytes)
	return err
}

func validatePrivateKey(data []byte) error {
	key, err := decodePEM(data)
	if err != nil {
		return nil
	}
	if _, err := x509.ParsePKCS1PrivateKey(key.Bytes); err == nil {
		return nil
	}
	if _, err := x509.ParsePKCS8PrivateKey(key.Bytes); err == nil {
		return nil
	}
	if _, err := x509.ParseECPrivateKey(key.Bytes); err == nil {
		return nil
	}
	return errors.New("unknown private key encoding")
}

func decodePEM(in []byte) (*pem.Block, error) {
	block, _ := pem.Decode(in)
	if block == nil {
		return nil, errors.New("failed to parse PEM")
	}
	return block, nil
}
