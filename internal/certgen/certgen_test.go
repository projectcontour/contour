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

package certgen

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"testing"
	"time"
)

func TestGeneratedCertsValid(t *testing.T) {

	now := time.Now()
	expiry := now.Add(24 * 365 * time.Hour)

	cacert, cakey, err := NewCA("contour", expiry)
	if err != nil {
		t.Fatalf("Failed to generate CA cert: %s", err)
	}

	contourcert, _, err := NewCert(cacert, cakey, expiry, "contour", "projectcontour")
	if err != nil {
		t.Fatalf("Failed to generate Contour cert: %s", err)
	}

	roots := x509.NewCertPool()
	ok := roots.AppendCertsFromPEM(cacert)
	if !ok {
		t.Fatal("Failed to set up CA cert for testing, maybe it's an invalid PEM")
	}
	envoycert, _, err := NewCert(cacert, cakey, expiry, "envoy", "projectcontour")
	if err != nil {
		t.Fatalf("Failed to generate Envoy cert: %s", err)
	}

	tests := map[string]struct {
		cert    []byte
		dnsname string
	}{
		"contour cert": {
			cert:    contourcert,
			dnsname: "contour",
		},
		"envoy cert": {
			cert:    envoycert,
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
