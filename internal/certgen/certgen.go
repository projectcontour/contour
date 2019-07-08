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
	"path"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
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

type certData struct {
	Filename string
	Data     []byte
}

// writePEM writes a certificate out to its filename in outputDir.
func (cd *certData) writePEM(outputDir string) error {
	return writePEMFile(outputDir+"/"+cd.Filename, cd.Data)

}

type keyPair struct {
	SecretName string
	Cert       *certData
	Key        *certData
}

// writeTLSYAML writes out Kubernetes Secret YAML for a keypair
// in a Secret of type `kubernetes.io/tls`. This prescribes the
// filenames/keynames for the cert and key.
func (kp *keyPair) writeTLSYAML(outputDir, namespace string) error {
	secret := newTLSSecret(kp.SecretName, namespace, kp.Key.Data, kp.Cert.Data)
	return writeSecret(outputDir+"/"+kp.SecretName+".yaml", secret)
}

// writeTLSKube writes a TLS Secret of the keypair out to Kubernetes.
func (kp *keyPair) writeTLSKube(namespace string, client *kubernetes.Clientset) error {
	secret := newTLSSecret(kp.SecretName, namespace, kp.Key.Data, kp.Cert.Data)
	_, err := client.CoreV1().Secrets(namespace).Create(secret)
	if err != nil {
		return err
	}
	fmt.Printf("secret/%s created\n", kp.SecretName)
	return nil
}

// writePEMs writes both certificates of a keypair out to outputDir.
func (kp *keyPair) writePEMs(outputDir string) error {
	err := kp.Cert.writePEM(outputDir)
	if err != nil {
		return err
	}
	return kp.Key.writePEM(outputDir)
}

// ContourCerts holds all three keypairs required, the CA, the Contour, and Envoy keypairs.
type ContourCerts struct {
	CA               *keyPair
	Contour          *keyPair
	Envoy            *keyPair
	CACertSecretName string
}

// NewContourCerts generates a new ContourCerts with default values.
// NOTE(youngnick) This means that there's only one place all these names
// are hard-coded.
func NewContourCerts() *ContourCerts {
	return &ContourCerts{
		CA: &keyPair{
			SecretName: "cakeypair",
			Cert: &certData{
				Filename: "CAcert.pem",
			},
			Key: &certData{
				Filename: "CAkey.pem",
			},
		},
		CACertSecretName: "cacert",
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

// writeCertPEMs writes out all the PEMs for all the certs, using
// the stored filenames.
func (cc *ContourCerts) writeCertPEMs(outputDir string) error {
	if err := cc.CA.writePEMs(outputDir); err != nil {
		return err
	}
	if err := cc.Contour.writePEMs(outputDir); err != nil {
		return err
	}
	return cc.Envoy.writePEMs(outputDir)

}

// writeCACertYAML is a helper function to write out just the CA's certificate
// as a Kubernetes Secret. Required so that we don't give access to the full
// keypair to consumers like Contour and Envoy.
func (cc *ContourCerts) writeCACertYAML(outputDir, namespace string) error {
	secret := newCertOnlySecret(cc.CACertSecretName, namespace, cc.CA.Cert.Filename, cc.CA.Cert.Data)
	return writeSecret(outputDir+"/"+cc.CACertSecretName+".yaml", secret)
}

// writeCACertKube writes out just the CA's certificate as a Kubernetes Secret.
// Required so that we don't give access to the full keypair to consumers like Contour and Envoy.
func (cc *ContourCerts) writeCACertKube(namespace string, client *kubernetes.Clientset) error {
	secret := newCertOnlySecret(cc.CACertSecretName, namespace, cc.CA.Cert.Filename, cc.CA.Cert.Data)
	_, err := client.CoreV1().Secrets(namespace).Create(secret)
	if err != nil {
		return err
	}
	fmt.Printf("secret/%s created\n", cc.CACertSecretName)
	return nil
}

// writeSecretYAMLs writes out all the secret YAMLs to `outputDir`, putting
// the generated Secrets into the Kube `namespace`
func (cc *ContourCerts) writeSecretYAMLs(outputDir, namespace string) error {
	// First, write out just the CA Cert secret.
	if err := cc.writeCACertYAML(outputDir, namespace); err != nil {
		return err
	}
	// Next, write out the full CA keypair
	if err := cc.CA.writeTLSYAML(outputDir, namespace); err != nil {
		return err
	}
	// Next, Contour's keypair
	if err := cc.Contour.writeTLSYAML(outputDir, namespace); err != nil {
		return err
	}
	return cc.Envoy.writeTLSYAML(outputDir, namespace)
}

// writeToKube writes all the secrets directly to Kubernetes, in the given
// `namespace`.
func (cc *ContourCerts) writeToKube(namespace string, client *kubernetes.Clientset) error {
	// First, write out just the CA Cert secret.
	if err := cc.writeCACertKube(namespace, client); err != nil {
		return err
	}
	// Next, write out the full CA keypair
	if err := cc.CA.writeTLSKube(namespace, client); err != nil {
		return err
	}
	// Next, Contour's keypair
	if err := cc.Contour.writeTLSKube(namespace, client); err != nil {
		return err
	}
	return cc.Envoy.writeTLSKube(namespace, client)
}

// GenerateCerts performs the actual cert generation steps and then returns the certs for the output function.
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
		if err := os.MkdirAll(certgenConfig.OutputDir, 0755); err != nil {
			return err
		}

		fmt.Printf("Outputting certs to PEM files in %s/\n", certgenConfig.OutputDir)
		if err := certs.writeCertPEMs(certgenConfig.OutputDir); err != nil {
			return err
		}
	}

	if certgenConfig.OutputYAML {
		// TODO(youngnick): Should we sanitize this value?
		if err := os.MkdirAll(certgenConfig.OutputDir, 0755); err != nil {
			return err
		}
		fmt.Printf("Outputting certs to YAML files in %s/\n", certgenConfig.OutputDir)
		if err := certs.writeSecretYAMLs(certgenConfig.OutputDir, certgenConfig.Namespace); err != nil {
			return err
		}
	}

	if certgenConfig.OutputKube {
		fmt.Printf("Outputting certs to Kubernetes in namespace %s/\n", certgenConfig.Namespace)
		client, err := newClient(certgenConfig.KubeConfig, certgenConfig.InCluster)
		if err != nil {
			return err
		}
		if err := certs.writeToKube(certgenConfig.Namespace, client); err != nil {
			return err
		}

	}

	return nil

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

func newTLSSecret(secretname, namespace string, keyPEM, certPEM []byte) *corev1.Secret {

	return &corev1.Secret{
		Type: corev1.SecretTypeTLS,
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretname,
			Namespace: namespace,
			Labels: map[string]string{
				"app": "contour",
			},
		},
		Data: map[string][]byte{
			corev1.TLSCertKey:       certPEM,
			corev1.TLSPrivateKeyKey: keyPEM,
		},
	}
}

func newCertOnlySecret(secretname, namespace, certfilename string, certPEM []byte) *corev1.Secret {

	return &corev1.Secret{
		Type: corev1.SecretTypeOpaque,
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretname,
			Namespace: namespace,
			Labels: map[string]string{
				"app": "contour",
			},
		},
		Data: map[string][]byte{
			path.Base(certfilename): certPEM,
		},
	}
}

func newClient(kubeconfig string, inCluster bool) (*kubernetes.Clientset, error) {
	var err error
	var config *rest.Config
	if kubeconfig != "" && !inCluster {
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, err
		}
	} else {
		config, err = rest.InClusterConfig()
		if err != nil {
			return nil, err
		}
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return client, nil
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

func writePEMFile(filename string, data []byte) error {
	_, err := os.Stat(filename)
	if err != nil {
		// Can't stat the file, so we'll create it
		err = ioutil.WriteFile(filename, data, 0644)
		if err != nil {
			return err
		}
		fmt.Printf("Created %s\n", filename)
		return nil
	}
	return fmt.Errorf("can't overwrite %s", filename)

}

func writeSecret(filename string, secret *corev1.Secret) error {
	s := json.NewYAMLSerializer(json.DefaultMetaFactory, scheme.Scheme, scheme.Scheme)
	_, err := os.Stat(filename)
	if err != nil {
		// Can't stat the file, we'll create it
		f, err := os.Create(filename)
		if err != nil {
			return err
		}
		defer f.Close()
		err = s.Encode(secret, f)
		if err != nil {
			return err
		}
		fmt.Printf("Created %s\n", filename)
		return nil
	}
	return fmt.Errorf("can't overwrite %s", filename)

}
