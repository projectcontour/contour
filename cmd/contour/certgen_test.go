// Copyright © 2019 Heptio
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

package main

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"testing"
)

func TestGeneratedCertsValid(t *testing.T) {
	genConfig := &certgenConfig{
		Namespace: "heptio-contour",
	}
	generatedCerts, err := GenerateCerts(genConfig)
	if err != nil {
		t.Fatalf("Failed to generate certs: %s", err)
	}

	roots := x509.NewCertPool()
	ok := roots.AppendCertsFromPEM(generatedCerts["cacert"])
	if !ok {
		t.Fatal("Failed to set up CA cert for testing, maybe it's an invalid PEM")
	}

	tests := map[string]struct {
		cert    []byte
		dnsname string
	}{
		"contour cert": {
			cert:    generatedCerts["contourcert"],
			dnsname: "contour",
		},
		"envoy cert": {
			cert:    generatedCerts["envoycert"],
			dnsname: "envoy",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := verifyCert(tc.cert, roots, tc.dnsname)
			if err != nil {
				t.Fatalf("Validating %s failed: %s", name, err)
			}
		})
	}

}

func verifyCert(certPEM []byte, roots *x509.CertPool, dnsname string) error {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return fmt.Errorf("Failed to decode %s certificate from PEM form", dnsname)
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return err
	}

	opts := x509.VerifyOptions{
		DNSName: dnsname,
		Roots:   roots,
	}
	if _, err = cert.Verify(opts); err != nil {
		return fmt.Errorf("Certificate verification failed: %s", err)
	}

	return nil
}
