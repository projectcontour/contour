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
	"time"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	certmanagermetav1 "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	"github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/require"
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Certs provides helpers for creating cert-manager certificates
// and related resources.
type Certs struct {
	client        client.Client
	retryInterval time.Duration
	retryTimeout  time.Duration
	t             ginkgo.GinkgoTInterface
}

// CreateSelfSignedCert creates a self-signed Issuer if it doesn't already exist
// and uses it to create a self-signed Certificate. It returns a cleanup function.
func (c *Certs) CreateSelfSignedCert(ns, name, secretName, dnsName string) func() {
	issuer := &certmanagerv1.Issuer{
		ObjectMeta: meta_v1.ObjectMeta{
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
		ObjectMeta: meta_v1.ObjectMeta{
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

// CreateCertAndWaitFor creates the provided Certificate in the Kubernetes API
// and then waits for the specified condition to be true.
func (c *Certs) CreateCertAndWaitFor(cert *certmanagerv1.Certificate, condition func(cert *certmanagerv1.Certificate) bool) bool {
	return createAndWaitFor(c.t, c.client, cert, condition, c.retryInterval, c.retryTimeout)
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

// ensureSelfSignedIssuer ensuers that selfsigned issuer is created.
func (c *Certs) ensureSelfSignedIssuer(ns string) *certmanagerv1.Issuer {
	issuer := &certmanagerv1.Issuer{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: ns,
			Name:      "selfsigned",
		},
		Spec: certmanagerv1.IssuerSpec{
			IssuerConfig: certmanagerv1.IssuerConfig{
				SelfSigned: &certmanagerv1.SelfSignedIssuer{},
			},
		},
	}

	if err := c.client.Get(context.TODO(), client.ObjectKeyFromObject(issuer), issuer); err != nil {
		if api_errors.IsNotFound(err) {
			require.NoError(c.t, c.client.Create(context.TODO(), issuer))
		} else {
			require.NoError(c.t, err)
		}
	}

	return issuer
}

// Create CA creates root CA using selfsigned issuer.
func (c *Certs) CreateCA(ns, name string) func() {
	issuer := c.ensureSelfSignedIssuer(ns)

	caSigningCert := &certmanagerv1.Certificate{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: ns,
			Name:      name,
		},
		Spec: certmanagerv1.CertificateSpec{
			IsCA: true,
			Usages: []certmanagerv1.KeyUsage{
				certmanagerv1.UsageSigning,
				certmanagerv1.UsageCertSign,
			},
			Subject: &certmanagerv1.X509Subject{
				OrganizationalUnits: []string{
					"io",
					"projectcontour",
					"testsuite",
				},
			},
			CommonName: name,
			SecretName: name,
			IssuerRef: certmanagermetav1.ObjectReference{
				Name: "selfsigned",
			},
		},
	}
	require.NoError(c.t, c.client.Create(context.TODO(), caSigningCert))

	localCAIssuer := &certmanagerv1.Issuer{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: ns,
			Name:      name,
		},
		Spec: certmanagerv1.IssuerSpec{
			IssuerConfig: certmanagerv1.IssuerConfig{
				CA: &certmanagerv1.CAIssuer{
					SecretName: name,
				},
			},
		},
	}

	require.NoError(c.t, c.client.Create(context.TODO(), localCAIssuer))

	return func() {
		caSecret := &core_v1.Secret{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: ns,
				Name:      name,
			},
		}
		require.NoError(c.t, c.client.Delete(context.TODO(), caSigningCert))
		require.NoError(c.t, c.client.Delete(context.TODO(), localCAIssuer))
		require.NoError(c.t, c.client.Delete(context.TODO(), issuer))
		require.NoError(c.t, c.client.Delete(context.TODO(), caSecret))
	}
}

// CreateCert creates end-entity certificate using given CA issuer.
func (c *Certs) CreateCert(ns, name, issuer string, dnsNames ...string) func() {
	cert := &certmanagerv1.Certificate{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: ns,
			Name:      name,
		},
		Spec: certmanagerv1.CertificateSpec{
			CommonName: name,
			SecretName: name,
			DNSNames:   dnsNames,
			IssuerRef: certmanagermetav1.ObjectReference{
				Name: issuer,
			},
		},
	}
	require.NoError(c.t, c.client.Create(context.TODO(), cert))

	return func() {
		secret := &core_v1.Secret{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: ns,
				Name:      name,
			},
		}
		require.NoError(c.t, c.client.Delete(context.TODO(), cert))
		require.NoError(c.t, c.client.Delete(context.TODO(), secret))
	}
}
