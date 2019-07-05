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

// Package certgen contains the code that handles the `certgen` subcommand
// for the main `contour` binary.
package certgen

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"io/ioutil"
	"math/big"
	"os"
	"time"
)

// keySize sets the RSA key size to 2048 bits. This is minimum recommended size
// for RSA keys.
const keySize = 2048

// Config holds the configuration for the certifcate generation process.
type Config struct {

	// KubeConfig is the path to the Kubeconfig file if we're not running in a cluster
	KubeConfig string

	// Incluster means that we should assume we are running in a Kubernetes cluster and work accordingly.
	InCluster bool

	// Namespace is the namespace to put any generated config into for YAML or Kube outputs.
	Namespace string

	// OutputDir stores the directory where any requested files will be output.
	OutputDir string

	// OutputKube means that the certs generated will be output into a Kubernetes cluster as secrets.
	OutputKube bool

	// OutputYAML means that the certs generated will be output into Kubernetes secrets as YAML in the current directory.
	OutputYAML bool

	// OutputPEM means that the certs generated will be output as PEM files in the current directory.
	OutputPEM bool
}

// ContourCerts holds all three keypairs required, the CA, the Contour, and Envoy keypairs.
type ContourCerts struct {
	CA      *keyPair
	Contour *keyPair
	Envoy   *keyPair
}

type keyPair struct {
	SecretName string
	Cert       *certData
	Key        *certData
}

type certData struct {
	Filename string
	Data     []byte
}

// NewContourCerts generates a new ContourCerts with default values.
// NOTE(youngnick) This means that there's only one place all these names
// are hard-coded.
func NewContourCerts() *ContourCerts {
	return &ContourCerts{
		CA: &keyPair{
			SecretName: "cacert",
			Cert: &certData{
				Filename: "CAcert.pem",
			},
			Key: &certData{
				Filename: "CAkey.pem",
			},
		},
		Contour: &keyPair{
			SecretName: "contourcert",
			Cert: &certData{
				Filename: "contourcert.pem",
			},
			Key: &certData{
				Filename: "contourkey.pem",
			},
		},
		Envoy: &keyPair{
			SecretName: "envoycert",
			Cert: &certData{
				Filename: "envoycert.pem",
			},
			Key: &certData{
				Filename: "envoykey.pem",
			},
		},
	}
}

func (kp *keyPair) writePEMs(outputDir string) error {
	err := kp.Cert.writePEM(outputDir)
	if err != nil {
		return err
	}
	return kp.Key.writePEM(outputDir)
}

func (cd *certData) writePEM(outputDir string) error {
	return dumpFile(outputDir+"/"+cd.Filename, cd.Data)

}

// writeCertPEMs writes out all the PEMs for all the certs, using
// the stored filenames.
// TODO(youngnick) we should be able to use a similar pattern for the
// secrets, hopefully.
func (cc *ContourCerts) writeCertPEMs(outputDir string) error {
	err := cc.CA.writePEMs(outputDir)
	if err != nil {
		return err
	}
	err = cc.Contour.writePEMs(outputDir)
	if err != nil {
		return err
	}
	return cc.Envoy.writePEMs(outputDir)

}

// GenerateCerts performs the actual cert generation steps and then returns the certs for the output functions.
func GenerateCerts(certConfig *Config) (*ContourCerts, error) {

	now := time.Now()
	expiry := now.Add(24 * 365 * time.Hour)
	caKeyPEM, caCertPEM, err := newCA(rand.Reader, "Project Contour", expiry)
	if err != nil {
		return nil, err
	}

	contourKey, contourCert, err := newCert(rand.Reader,
		caCertPEM,
		caKeyPEM,
		expiry,
		"contour",
		certConfig.Namespace,
	)
	if err != nil {
		return nil, err
	}
	envoyKey, envoyCert, err := newCert(rand.Reader,
		caCertPEM,
		caKeyPEM,
		expiry,
		"envoy",
		certConfig.Namespace,
	)
	if err != nil {
		return nil, err
	}
	newCerts := NewContourCerts()
	newCerts.CA.Cert.Data = caCertPEM
	newCerts.CA.Key.Data = caKeyPEM
	newCerts.Contour.Cert.Data = contourCert
	newCerts.Contour.Key.Data = contourKey
	newCerts.Envoy.Cert.Data = envoyCert
	newCerts.Envoy.Key.Data = envoyKey

	return newCerts, nil

}

// OutputCerts outputs the certs in certs as directed by config.
func OutputCerts(certgenConfig *Config, certs *ContourCerts) error {

	if certgenConfig.OutputPEM {
		// TODO(youngnick): Should we sanitize this value?
		err := os.MkdirAll(certgenConfig.OutputDir, 0755)
		if err != nil {
			return err
		}

		fmt.Printf("Outputting certs to PEM files in %s/\n", certgenConfig.OutputDir)
		certs.writeCertPEMs(certgenConfig.OutputDir)
	}

	if certgenConfig.OutputYAML {
		// TODO(youngnick): Should we sanitize this value?
		err := os.MkdirAll(certgenConfig.OutputDir, 0755)
		if err != nil {
			return err
		}
		fmt.Printf("Would configure Kube secrets here in YAML to '%s/'. Not implemented yet.\n", certgenConfig.OutputDir)
	}

	if certgenConfig.OutputKube {
		fmt.Print("Would configure Kube secrets here. Not implemented yet.\n")
	}

	return nil

}
func dumpFile(filename string, data []byte) error {
	return ioutil.WriteFile(filename, data, 0644)

}

func newCert(r io.Reader, caCertPEM, caKeyPEM []byte, expiry time.Time, service, namespace string) ([]byte, []byte, error) {

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

	newKey, err := rsa.GenerateKey(r, keySize)
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
	return newKeyPEM, newCertPEM, nil

}

func serviceNames(service, namespace string) []string {
	return []string{
		service,
		fmt.Sprintf("%s.%s", service, namespace),
		fmt.Sprintf("%s.%s.svc", service, namespace),
		fmt.Sprintf("%s.%s.svc.cluster.local", service, namespace),
	}
}

func newCA(r io.Reader, cn string, expiry time.Time) ([]byte, []byte, error) {
	key, err := rsa.GenerateKey(r, keySize)
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
	return keyPEMData, certPEMData, nil
}

func newSerial(now time.Time) *big.Int {
	return big.NewInt(int64(now.Nanosecond()))
}

func bigIntHash(n *big.Int) []byte {
	h := sha1.New()
	h.Write(n.Bytes())
	return h.Sum(nil)
}
