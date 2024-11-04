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

package httpproxy

import (
	"context"
	"crypto/tls"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tsaarni/certyaml"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/test/e2e"
)

func testClientCertRevocation(namespace string) {
	Specify("client requests with revoked certificates are rejected", func() {
		t := f.T()

		// Create certificate hierarchy according to figure:
		//
		//                    root-ca
		//                       |
		//       +---------------+---------------+
		//       |                               |
		// unrevoked-sub-ca               revoked-sub-ca
		//       |                               |
		//       |                    +----------+---------+
		//       |                    |                    |
		//   valid-client      unrevoked-client     revoked-client
		//
		isCA := true
		rootCA := certyaml.Certificate{
			Subject: "CN=root-ca",
		}
		unrevokedSubCA := certyaml.Certificate{
			Subject: "CN=unrevoked-sub-ca",
			IsCA:    &isCA,
			Issuer:  &rootCA,
		}
		revokedSubCA := certyaml.Certificate{
			Subject: "CN=revoked-sub-ca",
			IsCA:    &isCA,
			Issuer:  &rootCA,
		}
		server := certyaml.Certificate{
			Subject:         "CN=server",
			Issuer:          &rootCA,
			SubjectAltNames: []string{"DNS:*.projectcontour.io"},
		}
		validClient := certyaml.Certificate{
			Subject: "CN=valid-client",
			Issuer:  &unrevokedSubCA,
		}
		unrevokedClient := certyaml.Certificate{
			Subject: "CN=unrevoked-client",
			Issuer:  &revokedSubCA,
		}
		revokedClient := certyaml.Certificate{
			Subject: "CN=revoked-client",
			Issuer:  &revokedSubCA,
		}

		// Create CRLs for each CA
		rootCACRL := certyaml.CRL{
			Revoked: []*certyaml.Certificate{&revokedSubCA},
		}
		unrevokedSubCACRL := certyaml.CRL{
			// Empty: no revoked certificates.
			Issuer: &unrevokedSubCA,
		}
		revokedSubCACRL := certyaml.CRL{
			Revoked: []*certyaml.Certificate{&revokedClient},
		}

		// Create PEM bundle with all CRLs.
		crlBundle := append(crlPEMBytes(t, &rootCACRL), crlPEMBytes(t, &unrevokedSubCACRL)...)
		crlBundle = append(crlBundle, crlPEMBytes(t, &revokedSubCACRL)...)

		f.Fixtures.Echo.Deploy(namespace, "echo")

		// Create Secret for CA that is used to validate client certificates.
		require.NoError(t, f.Client.Create(context.TODO(),
			&core_v1.Secret{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "ca",
					Namespace: namespace,
				},
				Data: map[string][]byte{
					dag.CACertificateKey: certPEMBytes(t, &rootCA),
				},
			},
		))

		// Create Secret for server TLS credentials.
		require.NoError(t, f.Client.Create(context.TODO(),
			&core_v1.Secret{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "server-cert",
					Namespace: namespace,
				},
				Type: core_v1.SecretTypeTLS,
				Data: map[string][]byte{
					core_v1.TLSCertKey:       certPEMBytes(t, &server),
					core_v1.TLSPrivateKeyKey: keyPEMBytes(t, &server),
				},
			},
		))

		// Create Secret with CRLs from all CAs, combined as a PEM bundle.
		require.NoError(t, f.Client.Create(context.TODO(),
			&core_v1.Secret{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "all-crls",
					Namespace: namespace,
				},
				Data: map[string][]byte{
					dag.CRLKey: crlBundle,
				},
			},
		))

		// Create Secret with CRL from sub-CA only.
		require.NoError(t, f.Client.Create(context.TODO(),
			&core_v1.Secret{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "only-revoked-sub-ca-crl",
					Namespace: namespace,
				},
				Data: map[string][]byte{
					dag.CRLKey: crlPEMBytes(t, &revokedSubCACRL),
				},
			},
		))

		// Create HTTPProxy that does full chain CRL check.
		proxyWithFullCRLCheck := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      "crl-check-full",
				Namespace: namespace,
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "crl-check-full.projectcontour.io",
					TLS: &contour_v1.TLS{
						SecretName: "server-cert",
						ClientValidation: &contour_v1.DownstreamValidation{
							CACertificate:             "ca",
							CertificateRevocationList: "all-crls",
						},
					},
				},
				Routes: []contour_v1.Route{
					{
						Services: []contour_v1.Service{
							{
								Name: "echo",
								Port: 80,
							},
						},
					},
				},
			},
		}
		require.True(f.T(), f.CreateHTTPProxyAndWaitFor(proxyWithFullCRLCheck, e2e.HTTPProxyValid))

		// Create HTTPProxy that does CRL check for leaf-certificates only.
		proxyWithCRLLeafOnly := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      "crl-check-leaf-only",
				Namespace: namespace,
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "crl-check-leaf-only.projectcontour.io",
					TLS: &contour_v1.TLS{
						SecretName: "server-cert",
						ClientValidation: &contour_v1.DownstreamValidation{
							CACertificate:             "ca",
							CertificateRevocationList: "only-revoked-sub-ca-crl",
							OnlyVerifyLeafCertCrl:     true,
						},
					},
				},
				Routes: []contour_v1.Route{
					{
						Services: []contour_v1.Service{
							{
								Name: "echo",
								Port: 80,
							},
						},
					},
				},
			},
		}
		require.True(f.T(), f.CreateHTTPProxyAndWaitFor(proxyWithCRLLeafOnly, e2e.HTTPProxyValid))

		// HTTPProxy with full chain revocation but refers to Secret with only partial set of CRLs.
		proxyWithCRLMissing := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      "crl-check-full-but-missing-crl",
				Namespace: namespace,
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "crl-check-full-but-missing-crl.projectcontour.io",
					TLS: &contour_v1.TLS{
						SecretName: "server-cert",
						ClientValidation: &contour_v1.DownstreamValidation{
							CACertificate:             "ca",
							CertificateRevocationList: "only-revoked-sub-ca-crl",
						},
					},
				},
				Routes: []contour_v1.Route{
					{
						Services: []contour_v1.Service{
							{
								Name: "echo",
								Port: 80,
							},
						},
					},
				},
			},
		}
		require.True(f.T(), f.CreateHTTPProxyAndWaitFor(proxyWithCRLMissing, e2e.HTTPProxyValid))

		cases := map[string]struct {
			host       string
			clientCert *tls.Certificate
			wantErr    string
		}{
			"crl-check-full unrevoked client certificate issued under unrevoked CA should succeed": {
				host:       proxyWithFullCRLCheck.Spec.VirtualHost.Fqdn,
				clientCert: tlsCertificate(t, &validClient),
				wantErr:    "",
			},
			"crl-check-full revoked client certificate should fail": {
				host:       proxyWithFullCRLCheck.Spec.VirtualHost.Fqdn,
				clientCert: tlsCertificate(t, &revokedClient),
				wantErr:    "tls: revoked certificate",
			},
			"crl-check-full-but-missing-crl unrevoked client certificate should fail": {
				host:       proxyWithCRLMissing.Spec.VirtualHost.Fqdn,
				clientCert: tlsCertificate(t, &validClient),
				wantErr:    "tls: unknown certificate authority", // Error received when CRL is not configured.
			},
			"crl-check-leaf-only revoked client certificate should fail": {
				host:       proxyWithCRLLeafOnly.Spec.VirtualHost.Fqdn,
				clientCert: tlsCertificate(t, &revokedClient),
				wantErr:    "tls: revoked certificate",
			},
			"crl-check-leaf-only unrevoked client certificate under revoked CA should succeed": {
				host:       proxyWithCRLLeafOnly.Spec.VirtualHost.Fqdn,
				clientCert: tlsCertificate(t, &unrevokedClient),
				wantErr:    "",
			},
			"crl-check-full without client certificate should fail": {
				host:       proxyWithFullCRLCheck.Spec.VirtualHost.Fqdn,
				clientCert: nil,
				wantErr:    "tls: certificate required",
			},
			"crl-check-full-but-missing-crl without client certificate should fail": {
				host:       proxyWithCRLMissing.Spec.VirtualHost.Fqdn,
				clientCert: nil,
				wantErr:    "tls: certificate required",
			},
			"crl-check-leaf-only without client certificate should fail": {
				host:       proxyWithCRLLeafOnly.Spec.VirtualHost.Fqdn,
				clientCert: nil,
				wantErr:    "tls: certificate required",
			},
		}

		for name, tc := range cases {
			t.Logf("Running test case %s", name)
			opts := &e2e.HTTPSRequestOpts{
				Host: tc.host,
			}
			if tc.clientCert != nil {
				opts.TLSConfigOpts = append(opts.TLSConfigOpts, optUseClientCert(tc.clientCert))
			}

			switch {
			case len(tc.wantErr) == 0:
				opts.Condition = e2e.HasStatusCode(200)
				res, ok := f.HTTP.SecureRequestUntil(opts)
				require.NotNil(t, res, "expected 200 response code, request was never successful")
				assert.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)
			default:
				// Since we're expecting an error making the request
				// itself, SecureRequestUntil won't work since that
				// assumes an HTTP response is gotten.
				assert.Eventually(t, func() bool {
					_, err := f.HTTP.SecureRequest(opts)
					if err == nil {
						return false
					}
					t.Logf("Received error %s", err.Error())

					return strings.Contains(err.Error(), tc.wantErr)
				}, f.RetryTimeout, f.RetryInterval)
			}
		}
	})

	Specify("CRL can be rotated", func() {
		t := f.T()

		rootCA := certyaml.Certificate{
			Subject: "CN=root-ca",
		}
		server := certyaml.Certificate{
			Subject: "CN=server",
			Issuer:  &rootCA,
		}
		client := certyaml.Certificate{
			Subject: "CN=client",
			Issuer:  &rootCA,
		}
		crlEmpty := certyaml.CRL{
			Issuer: &rootCA,
		}
		crlRevokedClient := certyaml.CRL{
			Revoked: []*certyaml.Certificate{&client},
		}

		f.Fixtures.Echo.Deploy(namespace, "echo")

		// Create Secret for server credentials.
		require.NoError(t, f.Client.Create(context.TODO(),
			&core_v1.Secret{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "server-cert",
					Namespace: namespace,
				},
				Type: core_v1.SecretTypeTLS,
				Data: map[string][]byte{
					core_v1.TLSCertKey:       certPEMBytes(t, &server),
					core_v1.TLSPrivateKeyKey: keyPEMBytes(t, &server),
				},
			},
		))

		// Create Secret for CA to validate client certificates.
		require.NoError(t, f.Client.Create(context.TODO(),
			&core_v1.Secret{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "ca",
					Namespace: namespace,
				},
				Data: map[string][]byte{
					dag.CACertificateKey: certPEMBytes(t, &rootCA),
				},
			},
		))

		// Create HTTPProxy with client validation and CRL check.
		proxyWithCRLCheck := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      "crl-rotate",
				Namespace: namespace,
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "crl-rotate.projectcontour.io",
					TLS: &contour_v1.TLS{
						SecretName: "server-cert",
						ClientValidation: &contour_v1.DownstreamValidation{
							CACertificate:             "ca",
							CertificateRevocationList: "crl",
						},
					},
				},
				Routes: []contour_v1.Route{
					{
						Services: []contour_v1.Service{
							{
								Name: "echo",
								Port: 80,
							},
						},
					},
				},
			},
		}

		// HTTPProxy should be invalid since CertificateRevocationList refers to non-existent Secret.
		require.True(f.T(), f.CreateHTTPProxyAndWaitFor(proxyWithCRLCheck, e2e.HTTPProxyInvalid))

		// Create Secret with CRL where client certificate is revoked.
		require.NoError(t, f.Client.Create(context.TODO(),
			&core_v1.Secret{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "crl",
					Namespace: namespace,
				},
				Data: map[string][]byte{
					dag.CRLKey: crlPEMBytes(t, &crlRevokedClient),
				},
			},
		))

		opts := &e2e.HTTPSRequestOpts{
			Host:          "crl-rotate.projectcontour.io",
			TLSConfigOpts: []func(*tls.Config){optUseClientCert(tlsCertificate(t, &client))},
		}

		// TLS connection will fail since client certificate is revoked.
		assert.Eventually(t, func() bool {
			_, err := f.HTTP.SecureRequest(opts)
			if err == nil {
				return false
			}
			t.Logf("Received error %s", err.Error())

			return strings.Contains(err.Error(), "tls: revoked certificate")
		}, f.RetryTimeout, f.RetryInterval)

		// Update Secret with CRL where client certificate is not revoked.
		require.NoError(t, f.Client.Update(context.TODO(),
			&core_v1.Secret{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "crl",
					Namespace: namespace,
				},
				Data: map[string][]byte{
					dag.CRLKey: crlPEMBytes(t, &crlEmpty),
				},
			},
		))

		// HTTP request will succeed.
		opts.Condition = e2e.HasStatusCode(200)
		res, ok := f.HTTP.SecureRequestUntil(opts)

		require.NotNil(t, res, "expected 200 response code, request was never successful")
		assert.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)
	})
}

func tlsCertificate(t GinkgoTInterface, c *certyaml.Certificate) *tls.Certificate {
	cert, err := c.TLSCertificate()
	require.NoError(t, err)
	return &cert
}

func certPEMBytes(t GinkgoTInterface, c *certyaml.Certificate) []byte {
	cert, _, err := c.PEM()
	require.NoError(t, err)
	return cert
}

func keyPEMBytes(t GinkgoTInterface, c *certyaml.Certificate) []byte {
	_, key, err := c.PEM()
	require.NoError(t, err)
	return key
}

func crlPEMBytes(t GinkgoTInterface, c *certyaml.CRL) []byte {
	crl, err := c.PEM()
	require.NoError(t, err)
	return crl
}
