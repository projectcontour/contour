// Copyright © 2018 Heptio
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
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"time"

	"github.com/heptio/contour/internal/certgen"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

// registercertgen registers the certgen subcommand and flags
// with the Application provided.
func registerCertGen(app *kingpin.Application) (*kingpin.CmdClause, *certgenConfig) {
	var certgenConfig certgenConfig
	certgenApp := app.Command("certgen", "Generate new TLS certs for bootstrapping gRPC over TLS")
	certgenApp.Flag("kube", "Apply the generated certs directly to the current Kubernetes cluster").BoolVar(&certgenConfig.OutputKube)
	certgenApp.Flag("yaml", "Render the generated certs as Kubernetes Secrets in YAML form to the current directory").BoolVar(&certgenConfig.OutputYAML)
	certgenApp.Flag("pem", "Render the generated certs as individual PEM files to the current directory").BoolVar(&certgenConfig.OutputPEM)
	certgenApp.Flag("incluster", "use in cluster configuration.").BoolVar(&certgenConfig.InCluster)
	certgenApp.Flag("kubeconfig", "path to kubeconfig (if not in running inside a cluster)").Default(filepath.Join(os.Getenv("HOME"), ".kube", "config")).StringVar(&certgenConfig.KubeConfig)
	certgenApp.Flag("namespace", "Kubernetes namespace, used for Kube objects").Default("heptio-contour").Envar("CONTOUR_NAMESPACE").StringVar(&certgenConfig.Namespace)
	certgenApp.Arg("outputdir", "Directory to output any files to").Default("certs").StringVar(&certgenConfig.OutputDir)

	return certgenApp, &certgenConfig
}

// keySize sets the RSA key size to 2048 bits. This is minimum recommended size
// for RSA keys.
const keySize = 2048

// certgenConfig holds the configuration for the certifcate generation process.

type certgenConfig struct {

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

// GenerateCerts performs the actual cert generation steps and then returns the certs for the output function.
func GenerateCerts(certConfig *certgenConfig) (*certgen.ContourCerts, error) {

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
	newCerts := certgen.NewContourCerts()
	newCerts.CA.Data = caCertPEM
	newCerts.Contour.Cert.Data = contourCert
	newCerts.Contour.Key.Data = contourKey
	newCerts.Envoy.Cert.Data = envoyCert
	newCerts.Envoy.Key.Data = envoyKey

	return newCerts, nil

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

func serviceNames(service, namespace string) []string {
	return []string{
		service,
		fmt.Sprintf("%s.%s", service, namespace),
		fmt.Sprintf("%s.%s.svc", service, namespace),
		fmt.Sprintf("%s.%s.svc.cluster.local", service, namespace),
	}
}

func doCertgen(config *certgenConfig) {
	generatedCerts, err := GenerateCerts(config)
	check(err)
	kubeclient, _ := newClient(config.KubeConfig, config.InCluster)
	check(certgen.OutputCerts(config.OutputDir,
		config.Namespace,
		config.OutputPEM,
		config.OutputYAML,
		config.OutputKube,
		kubeclient,
		generatedCerts))

}
