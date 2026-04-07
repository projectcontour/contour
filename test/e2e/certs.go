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
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/require"
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	e2eCertificateLifetime = 365 * 24 * time.Hour
	e2eCertKeySize         = 2048
	e2eCertSyncInterval    = 100 * time.Millisecond
	selfSignedIssuerName   = "selfsigned"
)

// KeyUsage declares the X509 usage requested for a generated test certificate.
type KeyUsage string

const (
	UsageCertSign   KeyUsage = "cert sign"
	UsageClientAuth KeyUsage = "client auth"
	UsageServerAuth KeyUsage = "server auth"
	UsageSigning    KeyUsage = "signing"
)

// X509Subject defines the subset of subject fields used in the e2e suite.
type X509Subject struct {
	OrganizationalUnits []string
}

// CertificateSpec defines the inputs used to generate a test certificate and
// its backing Secret.
type CertificateSpec struct {
	Namespace      string
	Name           string
	SecretName     string
	CommonName     string
	DNSNames       []string
	EmailAddresses []string
	IsCA           bool
	Usages         []KeyUsage
	Subject        *X509Subject
	Issuer         string
}

type certificateMaterial struct {
	certificate *x509.Certificate
	privateKey  *rsa.PrivateKey
	certPEM     []byte
	keyPEM      []byte
	caPEM       []byte
}

type managedCertificate struct {
	namespace string
	name      string
	material  certificateMaterial
}

type managedIssuer struct {
	selfSigned bool
	signer     *certificateMaterial
}

// Certs provides helpers for generating TLS Secrets used by e2e tests.
type Certs struct {
	client        client.Client
	retryInterval time.Duration
	retryTimeout  time.Duration
	t             ginkgo.GinkgoTInterface

	mu           sync.RWMutex
	issuers      map[types.NamespacedName]managedIssuer
	certificates map[types.NamespacedName]managedCertificate
}

func newCerts(cli client.Client, t ginkgo.GinkgoTInterface) *Certs {
	c := &Certs{
		client:        cli,
		retryInterval: time.Second,
		retryTimeout:  60 * time.Second,
		t:             t,
		issuers:       map[types.NamespacedName]managedIssuer{},
		certificates:  map[types.NamespacedName]managedCertificate{},
	}

	go c.reconcileSecrets()

	return c
}

// CreateSelfSignedCert creates a self-signed certificate Secret. It returns a
// cleanup function that unregisters the managed Secret and removes it.
func (c *Certs) CreateSelfSignedCert(ns, name, secretName, dnsName string) func() {
	c.ensureSelfSignedIssuer(ns)

	return c.CreateCertificate(CertificateSpec{
		Namespace:  ns,
		Name:       name,
		SecretName: secretName,
		CommonName: dnsName,
		DNSNames:   []string{dnsName},
		Issuer:     selfSignedIssuerName,
	})
}

// CreateCA creates a root CA Secret and an issuer with the same name.
func (c *Certs) CreateCA(ns, name string) func() {
	return c.CreateCAWithIssuer(ns, name, name)
}

// CreateCAWithIssuer creates a root CA Secret and registers a named issuer that
// signs future certificates with it.
func (c *Certs) CreateCAWithIssuer(ns, secretName, issuerName string) func() {
	c.ensureSelfSignedIssuer(ns)

	spec := CertificateSpec{
		Namespace:  ns,
		Name:       secretName,
		SecretName: secretName,
		CommonName: secretName,
		IsCA:       true,
		Usages: []KeyUsage{
			UsageSigning,
			UsageCertSign,
		},
		Subject: &X509Subject{
			OrganizationalUnits: []string{
				"io",
				"projectcontour",
				"testsuite",
			},
		},
		Issuer: selfSignedIssuerName,
	}

	material, err := c.issueCertificate(spec)
	require.NoError(c.t, err)

	c.registerIssuer(ns, issuerName, managedIssuer{
		signer: &material,
	})

	return c.manageCertificate(spec, material, func() {
		c.unregisterIssuer(ns, issuerName)
	})
}

// CreateCertificate creates a certificate Secret using the named issuer.
func (c *Certs) CreateCertificate(spec CertificateSpec) func() {
	material, err := c.issueCertificate(spec)
	require.NoError(c.t, err)

	return c.manageCertificate(spec, material, nil)
}

// CreateCertificateAndWait creates the certificate Secret and returns true once
// it has been persisted. Generation is synchronous, so no additional polling is
// required beyond the initial write.
func (c *Certs) CreateCertificateAndWait(spec CertificateSpec) bool {
	c.CreateCertificate(spec)
	return true
}

// CreateCert creates end-entity certificate using given CA issuer.
func (c *Certs) CreateCert(ns, name, issuer string, dnsNames ...string) func() {
	return c.CreateCertificate(CertificateSpec{
		Namespace:  ns,
		Name:       name,
		SecretName: name,
		CommonName: name,
		DNSNames:   dnsNames,
		Issuer:     issuer,
	})
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

func (c *Certs) ensureSelfSignedIssuer(namespace string) {
	c.registerIssuer(namespace, selfSignedIssuerName, managedIssuer{
		selfSigned: true,
	})
}

func (c *Certs) manageCertificate(spec CertificateSpec, material certificateMaterial, cleanup func()) func() {
	name := spec.SecretName
	if name == "" {
		name = spec.Name
	}

	managed := managedCertificate{
		namespace: spec.Namespace,
		name:      name,
		material:  material,
	}

	c.registerManagedCertificate(managed)
	require.NoError(c.t, c.ensureSecret(context.TODO(), managed))

	return func() {
		c.unregisterManagedCertificate(spec.Namespace, name)
		if cleanup != nil {
			cleanup()
		}

		secret := &core_v1.Secret{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: spec.Namespace,
				Name:      name,
			},
		}
		err := c.client.Delete(context.TODO(), secret)
		if err != nil && !errors.IsNotFound(err) {
			require.NoError(c.t, err)
		}
	}
}

func (c *Certs) issueCertificate(spec CertificateSpec) (certificateMaterial, error) {
	issuer, err := c.getIssuer(spec.Namespace, spec.Issuer)
	if err != nil {
		return certificateMaterial{}, err
	}

	privateKey, err := rsa.GenerateKey(rand.Reader, e2eCertKeySize)
	if err != nil {
		return certificateMaterial{}, err
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return certificateMaterial{}, err
	}

	subject := pkix.Name{
		CommonName: spec.CommonName,
	}
	if spec.Subject != nil {
		subject.OrganizationalUnit = append(subject.OrganizationalUnit, spec.Subject.OrganizationalUnits...)
	}

	keyUsage, extKeyUsage := x509Usages(spec)
	template := &x509.Certificate{
		SerialNumber:          serialNumber,
		Subject:               subject,
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(e2eCertificateLifetime),
		BasicConstraintsValid: true,
		IsCA:                  spec.IsCA,
		DNSNames:              append([]string(nil), spec.DNSNames...),
		EmailAddresses:        append([]string(nil), spec.EmailAddresses...),
		KeyUsage:              keyUsage,
		ExtKeyUsage:           extKeyUsage,
	}

	parent := template
	signer := privateKey
	caPEM := []byte(nil)

	if issuer.selfSigned {
		caPEM = make([]byte, 0)
	} else {
		parent = issuer.signer.certificate
		signer = issuer.signer.privateKey
		caPEM = append([]byte(nil), issuer.signer.caPEM...)
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, parent, &privateKey.PublicKey, signer)
	if err != nil {
		return certificateMaterial{}, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})
	if issuer.selfSigned {
		caPEM = append([]byte(nil), certPEM...)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return certificateMaterial{}, err
	}

	return certificateMaterial{
		certificate: cert,
		privateKey:  privateKey,
		certPEM:     certPEM,
		keyPEM:      keyPEM,
		caPEM:       caPEM,
	}, nil
}

func x509Usages(spec CertificateSpec) (x509.KeyUsage, []x509.ExtKeyUsage) {
	if !spec.IsCA && len(spec.Usages) == 0 {
		spec.Usages = []KeyUsage{UsageServerAuth, UsageClientAuth}
	}

	keyUsage := x509.KeyUsageDigitalSignature
	var extKeyUsage []x509.ExtKeyUsage

	for _, usage := range spec.Usages {
		switch usage {
		case UsageCertSign, UsageSigning:
			keyUsage |= x509.KeyUsageCertSign | x509.KeyUsageCRLSign
		case UsageClientAuth:
			extKeyUsage = append(extKeyUsage, x509.ExtKeyUsageClientAuth)
		case UsageServerAuth:
			keyUsage |= x509.KeyUsageKeyEncipherment
			extKeyUsage = append(extKeyUsage, x509.ExtKeyUsageServerAuth)
		}
	}

	if spec.IsCA {
		keyUsage |= x509.KeyUsageCertSign | x509.KeyUsageCRLSign
	}

	return keyUsage, extKeyUsage
}

func (c *Certs) reconcileSecrets() {
	ticker := time.NewTicker(e2eCertSyncInterval)
	defer ticker.Stop()

	for range ticker.C {
		for _, cert := range c.managedCertificates() {
			_ = c.ensureSecret(context.Background(), cert)
		}
	}
}

func (c *Certs) ensureSecret(ctx context.Context, managed managedCertificate) error {
	desired := &core_v1.Secret{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: managed.namespace,
			Name:      managed.name,
		},
		Type: core_v1.SecretTypeTLS,
		Data: map[string][]byte{
			core_v1.TLSCertKey:       managed.material.certPEM,
			core_v1.TLSPrivateKeyKey: managed.material.keyPEM,
			"ca.crt":                 managed.material.caPEM,
		},
	}

	current := &core_v1.Secret{}
	key := client.ObjectKeyFromObject(desired)

	if err := c.client.Get(ctx, key, current); err != nil {
		if !errors.IsNotFound(err) {
			return err
		}

		if err := c.client.Create(ctx, desired); err != nil && !errors.IsAlreadyExists(err) {
			if errors.IsNotFound(err) {
				return nil
			}
			return err
		}

		return nil
	}

	if current.Type == desired.Type &&
		bytesEqual(current.Data[core_v1.TLSCertKey], desired.Data[core_v1.TLSCertKey]) &&
		bytesEqual(current.Data[core_v1.TLSPrivateKeyKey], desired.Data[core_v1.TLSPrivateKeyKey]) &&
		bytesEqual(current.Data["ca.crt"], desired.Data["ca.crt"]) {
		return nil
	}

	current.Type = desired.Type
	current.Data = desired.Data

	return c.client.Update(ctx, current)
}

func (c *Certs) managedCertificates() []managedCertificate {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]managedCertificate, 0, len(c.certificates))
	for _, cert := range c.certificates {
		result = append(result, cert)
	}

	return result
}

func (c *Certs) registerIssuer(namespace, name string, issuer managedIssuer) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.issuers[types.NamespacedName{Namespace: namespace, Name: name}] = issuer
}

func (c *Certs) unregisterIssuer(namespace, name string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.issuers, types.NamespacedName{Namespace: namespace, Name: name})
}

func (c *Certs) getIssuer(namespace, name string) (managedIssuer, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	issuer, ok := c.issuers[types.NamespacedName{Namespace: namespace, Name: name}]
	if !ok {
		return managedIssuer{}, fmt.Errorf("issuer %s/%s not found", namespace, name)
	}

	return issuer, nil
}

func (c *Certs) registerManagedCertificate(cert managedCertificate) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.certificates[types.NamespacedName{Namespace: cert.namespace, Name: cert.name}] = cert
}

func (c *Certs) unregisterManagedCertificate(namespace, name string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.certificates, types.NamespacedName{Namespace: namespace, Name: name})
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}
