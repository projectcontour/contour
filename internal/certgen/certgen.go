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
	"path"

	"k8s.io/client-go/kubernetes"
)

// WritePEM writes a certificate out to its filename in outputDir.
func writePEM(outputDir, filename string, data []byte) error {
	filepath := path.Join(outputDir, filename)
	f, err := createFile(filepath, false)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	return checkFile(filepath, err)
}

// WriteCertsPEM writes out all the certs in certdata to
// individual PEM files in outputDir
func WriteCertsPEM(outputDir string, certdata map[string][]byte) error {

	err := writePEM(outputDir, "cacert.pem", certdata["cacert.pem"])
	if err != nil {
		return err
	}
	err = writePEM(outputDir, "contourcert.pem", certdata["contourcert.pem"])
	if err != nil {
		return err
	}
	err = writePEM(outputDir, "contourkey.pem", certdata["contourkey.pem"])
	if err != nil {
		return err
	}
	err = writePEM(outputDir, "envoycert.pem", certdata["envoycert.pem"])
	if err != nil {
		return err
	}
	return writePEM(outputDir, "envoykey.pem", certdata["envoykey.pem"])

}

// WriteSecretsYAML writes all the keypairs out to Kube Secrets in YAML form
// in outputDir. The CA Secret only contains the cert.
func WriteSecretsYAML(outputDir, namespace string, certdata map[string][]byte) error {
	err := writeCACertSecret(outputDir, "cacert.pem", certdata["cacert.pem"])
	if err != nil {
		return err
	}
	err = writeKeyPairSecret(outputDir, "contour", namespace, certdata["contourcert.pem"], certdata["contourkey.pem"])
	if err != nil {
		return err
	}

	return writeKeyPairSecret(outputDir, "envoy", namespace, certdata["envoycert.pem"], certdata["envoykey.pem"])

}

// WriteSecretsKube writes all the keypairs out to Kube Secrets in the
// passed Kube context.
func WriteSecretsKube(client *kubernetes.Clientset, namespace string, certdata map[string][]byte) error {
	err := writeCACertKube(client, namespace, certdata["cacert.pem"])
	if err != nil {
		return err
	}
	err = writeKeyPairKube(client, "contour", namespace, certdata["contourcert.pem"], certdata["contourkey.pem"])
	if err != nil {
		return err
	}

	return writeKeyPairKube(client, "envoy", namespace, certdata["envoycert.pem"], certdata["envoykey.pem"])

}

func writeCACertSecret(outputDir, namespace string, cert []byte) error {
	filename := path.Join(outputDir, "cacert.yaml")
	secret := newCertOnlySecret("cacert", namespace, "cacert.pem", cert)
	f, err := createFile(filename, false)
	if err != nil {
		return err
	}
	return checkFile(filename, writeSecret(f, secret))
}

func writeCACertKube(client *kubernetes.Clientset, namespace string, cert []byte) error {
	secret := newCertOnlySecret("cacert", namespace, "cacert.pem", cert)
	_, err := client.CoreV1().Secrets(namespace).Create(secret)
	if err != nil {
		return err
	}
	fmt.Print("secret/cacert created\n")
	return nil
}

func writeKeyPairSecret(outputDir, service, namespace string, cert, key []byte) error {
	filename := service + "cert.yaml"
	secretname := service + "cert"

	secret := newTLSSecret(secretname, namespace, key, cert)
	filepath := path.Join(outputDir, filename)
	f, err := createFile(filepath, false)
	if err != nil {
		return err
	}
	err = writeSecret(f, secret)
	return checkFile(filepath, err)
}

func writeKeyPairKube(client *kubernetes.Clientset, service, namespace string, cert, key []byte) error {
	secretname := service + "cert"
	secret := newTLSSecret(secretname, namespace, key, cert)
	_, err := client.CoreV1().Secrets(namespace).Create(secret)
	if err != nil {
		return err
	}
	fmt.Printf("secret/%s created\n", secretname)
	return nil
}
