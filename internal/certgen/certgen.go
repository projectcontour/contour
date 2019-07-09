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
	"fmt"
	"os"
	"path"

	"k8s.io/client-go/kubernetes"
)

type certData struct {
	Filename string
	Data     []byte
}

// writePEM writes a certificate out to its filename in outputDir.
func (cd *certData) writePEM(outputDir string) error {
	filename := path.Join(outputDir, cd.Filename)
	f, err := createFile(filename, false)
	if err != nil {
		return err
	}
	_, err = f.Write(cd.Data)
	return checkFile(filename, err)
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
	filename := path.Join(outputDir, kp.SecretName+".yaml")
	f, err := createFile(filename, false)
	if err != nil {
		return err
	}
	err = writeSecret(f, secret)
	return checkFile(filename, err)
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
	CA               *certData
	Contour          *keyPair
	Envoy            *keyPair
	CACertSecretName string
}

// NewContourCerts generates a new ContourCerts with default values.
// Note that all of these filename values are efffectively constants,
// they're fixed by the deployment YAMLs and shouldn't be changed without
// updating the deployment yamls and/or the Makefile.
func NewContourCerts() *ContourCerts {
	return &ContourCerts{
		CA: &certData{
			Filename: "CAcert.pem",
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
	if err := cc.CA.writePEM(outputDir); err != nil {
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
	secret := newCertOnlySecret(cc.CACertSecretName, namespace, cc.CA.Filename, cc.CA.Data)
	f, err := createFile(outputDir+"/"+cc.CACertSecretName+".yaml", false)
	if err != nil {
		return err
	}
	return writeSecret(f, secret)
}

// writeCACertKube writes out just the CA's certificate as a Kubernetes Secret to Kubernetes.
// Required so that we don't give access to the full keypair to consumers like Contour and Envoy.
func (cc *ContourCerts) writeCACertKube(namespace string, client *kubernetes.Clientset) error {
	secret := newCertOnlySecret(cc.CACertSecretName, namespace, cc.CA.Filename, cc.CA.Data)
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
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return err
	}
	// First, write out just the CA Cert secret.
	if err := cc.writeCACertYAML(outputDir, namespace); err != nil {
		return err
	}
	// Next, Contour's keypair
	if err := cc.Contour.writeTLSYAML(outputDir, namespace); err != nil {
		return err
	}
	// Lastly, Envoy's keypair
	return cc.Envoy.writeTLSYAML(outputDir, namespace)
}

// writeToKube writes all the secrets directly to Kubernetes, in the given
// `namespace`.
func (cc *ContourCerts) writeToKube(namespace string, client *kubernetes.Clientset) error {
	// First, write out just the CA Cert secret.
	if err := cc.writeCACertKube(namespace, client); err != nil {
		return err
	}
	// Next, Contour's keypair
	if err := cc.Contour.writeTLSKube(namespace, client); err != nil {
		return err
	}
	// Lastly, Envoy's keypair
	return cc.Envoy.writeTLSKube(namespace, client)
}

// OutputCerts outputs the certs in certs as directed by config.
func OutputCerts(outputDir, namespace string,
	outputPEM, outputYAML, outputKube bool,
	kubeclient *kubernetes.Clientset,
	certs *ContourCerts) error {

	if outputPEM {
		fmt.Printf("Outputting certs to PEM files in %s/\n", outputDir)
		if err := certs.writeCertPEMs(outputDir); err != nil {
			return err
		}
	}

	if outputYAML {
		fmt.Printf("Outputting certs to YAML files in %s/\n", outputDir)
		if err := certs.writeSecretYAMLs(outputDir, namespace); err != nil {
			return err
		}
	}

	if outputKube {
		fmt.Printf("Outputting certs to Kubernetes in namespace %s/\n", namespace)
		if err := certs.writeToKube(namespace, kubeclient); err != nil {
			return err
		}

	}

	return nil

}
