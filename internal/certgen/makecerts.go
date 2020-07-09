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
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1" // nolint:gosec
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"
)

// keySize sets the RSA key size to 2048 bits. This is minimum recommended size
// for RSA keys.
const keySize = 2048

// NewCert generates a new keypair given the CA keypair, the expiry time, the service name
// ("contour" or "envoy"), and the Kubernetes namespace the service will run in (because
// of the Kubernetes DNS schema.)
// The return values are cert, key, err.
func NewCert(caCertPEM, caKeyPEM []byte, expiry time.Time, service, namespace string) ([]byte, []byte, error) {

	caKeyPair, err := tls.X509KeyPair(caCertPEM, caKeyPEM)
	if err != nil {
		return nil, nil, err
	}
	caCert, err := x509.ParseCertificate(caKeyPair.Certificate[0])
	if err != nil {
		return nil, nil, err
	}
	caKey, ok := caKeyPair.PrivateKey.(*rsa.PrivateKey)
	if !ok {
		return nil, nil, fmt.Errorf("CA private key has unexpected type %T", caKeyPair.PrivateKey)
	}

	newKey, err := rsa.GenerateKey(rand.Reader, keySize)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot generate key: %v", err)
	}

	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: newSerial(now),
		Subject: pkix.Name{
			CommonName: service,
		},
		NotBefore:    now.UTC().AddDate(0, 0, -1),
		NotAfter:     expiry.UTC(),
		SubjectKeyId: bigIntHash(newKey.N),
		KeyUsage: x509.KeyUsageDigitalSignature |
			x509.KeyUsageDataEncipherment |
			x509.KeyUsageKeyEncipherment |
			x509.KeyUsageContentCommitment,
		DNSNames: serviceNames(service, namespace),
	}
	newCert, err := x509.CreateCertificate(rand.Reader, template, caCert, &newKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, err
	}

	newKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(newKey),
	})
	newCertPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: newCert,
	})
	return newCertPEM, newKeyPEM, nil

}

// NewCA generates a new CA, given the CA's CN and an expiry time.
// The return order is cacert, cakey, error.
func NewCA(cn string, expiry time.Time) ([]byte, []byte, error) {

	key, err := rsa.GenerateKey(rand.Reader, keySize)
	if err != nil {
		return nil, nil, err
	}

	now := time.Now()
	serial := newSerial(now)
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   cn,
			SerialNumber: serial.String(),
		},
		NotBefore:             now.UTC().AddDate(0, 0, -1),
		NotAfter:              expiry.UTC(),
		SubjectKeyId:          bigIntHash(key.N),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	certPEMData := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})
	keyPEMData := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	return certPEMData, keyPEMData, nil
}

func newSerial(now time.Time) *big.Int {
	return big.NewInt(int64(now.Nanosecond()))
}

// bigIntHash generates a SubjectKeyId by hashing the modulus of the private
// key. This isn't one of the methods listed in RFC 5280 4.2.1.2, but that also
// notes that other methods are acceptable.
//
// gosec makes a blanket claim that SHA-1 is unacceptable, which is
// false here. The core Go method of generations the SubjectKeyId (see
// https://github.com/golang/go/issues/26676) also uses SHA-1, as recommended
// by RFC 5280.
func bigIntHash(n *big.Int) []byte {
	h := sha1.New()    // nolint:gosec
	h.Write(n.Bytes()) // nolint:errcheck
	return h.Sum(nil)
}

func serviceNames(service, namespace string) []string {
	return []string{
		service,
		fmt.Sprintf("%s.%s", service, namespace),
		fmt.Sprintf("%s.%s.svc", service, namespace),
		fmt.Sprintf("%s.%s.svc.cluster.local", service, namespace),
	}
}
