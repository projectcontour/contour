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

// +build e2e

package e2e

import (
	"context"
	"crypto/tls"

	certmanagerv1 "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1"
	certmanagermetav1 "github.com/jetstack/cert-manager/pkg/apis/meta/v1"
	"github.com/onsi/ginkgo"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Certs provides helpers for creating cert-manager certificates
// and related resources.
type Certs struct {
	client client.Client
	t      ginkgo.GinkgoTInterface
}

// CreateSelfSignedCert creates a self-signed Issuer if it doesn't already exist
// and uses it to create a self-signed Certificate. It returns a cleanup function.
func (c *Certs) CreateSelfSignedCert(ns, name, secretName, dnsName string) func() {
	issuer := &certmanagerv1.Issuer{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      "selfsigned",
		},
		Spec: certmanagerv1.IssuerSpec{
			IssuerConfig: certmanagerv1.IssuerConfig{
				SelfSigned: &certmanagerv1.SelfSignedIssuer{},
			},
		},
	}

	if err := c.client.Create(context.TODO(), issuer); err != nil && !errors.IsAlreadyExists(err) {
		require.FailNow(c.t, "error creating Issuer: %v", err)
	}

	cert := &certmanagerv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      name,
		},
		Spec: certmanagerv1.CertificateSpec{
			DNSNames:   []string{dnsName},
			SecretName: secretName,
			IssuerRef: certmanagermetav1.ObjectReference{
				Name: "selfsigned",
			},
		},
	}
	require.NoError(c.t, c.client.Create(context.TODO(), cert))

	return func() {
		require.NoError(c.t, c.client.Delete(context.TODO(), cert))
		require.NoError(c.t, c.client.Delete(context.TODO(), issuer))
	}
}

// GetTLSCertificate returns a tls.Certificate containing the data in the specified
// secret. The secret must have the "tls.crt" and "tls.key" keys.
func (c *Certs) GetTLSCertificate(secretNamespace, secretName string) tls.Certificate {
	secret := &corev1.Secret{}
	require.NoError(c.t, c.client.Get(context.TODO(), client.ObjectKey{Namespace: secretNamespace, Name: secretName}, secret))

	cert, err := tls.X509KeyPair(secret.Data["tls.crt"], secret.Data["tls.key"])
	require.NoError(c.t, err)

	return cert
}
