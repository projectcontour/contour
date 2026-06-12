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

//go:build e2e

package e2e

import (
	"context"
	"crypto/tls"
	"crypto/x509"

	"github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/require"
	"github.com/tsaarni/certyaml"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Certs provides helpers for generating TLS Secrets used by e2e tests.
type Certs struct {
	client client.Client
	t      ginkgo.GinkgoTInterface
}

func newCerts(cli client.Client, t ginkgo.GinkgoTInterface) *Certs {
	return &Certs{client: cli, t: t}
}

// CreateSelfSignedCert creates a self-signed certificate Secret.
func (c *Certs) CreateSelfSignedCert(ns, secretName, dnsName string) {
	isCA := false
	c.CreateCertificate(ns, secretName, &certyaml.Certificate{
		Subject:         "cn=" + dnsName,
		SubjectAltNames: []string{"DNS:" + dnsName},
		IsCA:            &isCA,
	})
}

// CreateCA creates a root CA Secret and returns the CA for use as Issuer.
func (c *Certs) CreateCA(ns, secretName string) *certyaml.Certificate {
	ca := &certyaml.Certificate{Subject: "cn=" + secretName}
	c.CreateCertificate(ns, secretName, ca)
	return ca
}

// CreateCertificate creates a TLS Secret from a certyaml.Certificate.
func (c *Certs) CreateCertificate(ns, secretName string, cert *certyaml.Certificate) {
	certPEM, keyPEM, err := cert.PEM()
	require.NoError(c.t, err)

	var caPEM []byte
	if cert.Issuer != nil {
		caPEM, _, err = cert.Issuer.PEM()
		require.NoError(c.t, err)
	} else {
		caPEM = certPEM
	}

	secret := &core_v1.Secret{
		ObjectMeta: meta_v1.ObjectMeta{Namespace: ns, Name: secretName},
		Type:       core_v1.SecretTypeTLS,
		Data: map[string][]byte{
			core_v1.TLSCertKey:       certPEM,
			core_v1.TLSPrivateKeyKey: keyPEM,
			"ca.crt":                 caPEM,
		},
	}
	require.NoError(c.t, c.client.Create(context.TODO(), secret))
}

// GetTLSCertificate returns a tls.Certificate containing the data in the specified
// secret and optional CA certificate. The secret must have the "tls.crt" and "tls.key" keys,
// and "ca.crt" if CA certificate is also provided.
func (c *Certs) GetTLSCertificate(secretNamespace, secretName string) (tls.Certificate, *x509.CertPool) {
	secret := &core_v1.Secret{}
	require.NoError(c.t, c.client.Get(context.TODO(), client.ObjectKey{Namespace: secretNamespace, Name: secretName}, secret))

	cert, err := tls.X509KeyPair(secret.Data["tls.crt"], secret.Data["tls.key"])
	require.NoError(c.t, err)

	var caBundle *x509.CertPool
	ca, ok := secret.Data["ca.crt"]
	if ok {
		caBundle = x509.NewCertPool()
		caBundle.AppendCertsFromPEM(ca)
	}

	return cert, caBundle
}
