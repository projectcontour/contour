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
	"crypto/tls"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/test/e2e"
)

func testClientCertAuth(namespace string) {
	Specify("client requests can be authenticated", func() {
		t := f.T()

		f.Certs.CreateCAWithIssuer(namespace, "ca-projectcontour-io", "ca-projectcontour-io")
		f.Certs.CreateCAWithIssuer(namespace, "ca-notprojectcontour-io", "ca-notprojectcontour-io")

		f.Fixtures.Echo.Deploy(namespace, "echo-no-auth")

		f.Certs.CreateCertificate(e2e.CertificateSpec{
			Namespace:  namespace,
			Name:       "echo-no-auth-cert",
			SecretName: "echo-no-auth",
			DNSNames:   []string{"echo-no-auth.projectcontour.io"},
			Usages: []e2e.KeyUsage{
				e2e.UsageServerAuth,
			},
			Issuer: "ca-projectcontour-io",
		})

		f.Fixtures.Echo.Deploy(namespace, "echo-with-auth")

		f.Certs.CreateCertificate(e2e.CertificateSpec{
			Namespace:  namespace,
			Name:       "echo-with-auth-cert",
			SecretName: "echo-with-auth",
			DNSNames:   []string{"echo-with-auth.projectcontour.io"},
			Usages: []e2e.KeyUsage{
				e2e.UsageServerAuth,
			},
			Issuer: "ca-projectcontour-io",
		})

		f.Fixtures.Echo.Deploy(namespace, "echo-with-auth-skip-verify")

		f.Certs.CreateCertificate(e2e.CertificateSpec{
			Namespace:  namespace,
			Name:       "echo-with-auth-skip-verify-cert",
			SecretName: "echo-with-auth-skip-verify",
			DNSNames:   []string{"echo-with-auth-skip-verify.projectcontour.io"},
			Usages: []e2e.KeyUsage{
				e2e.UsageServerAuth,
			},
			Issuer: "ca-projectcontour-io",
		})

		f.Fixtures.Echo.Deploy(namespace, "echo-with-auth-skip-verify-with-ca")

		f.Certs.CreateCertificate(e2e.CertificateSpec{
			Namespace:  namespace,
			Name:       "echo-with-auth-skip-verify-with-ca-cert",
			SecretName: "echo-with-auth-skip-verify-with-ca",
			DNSNames:   []string{"echo-with-auth-skip-verify-with-ca.projectcontour.io"},
			Usages: []e2e.KeyUsage{
				e2e.UsageServerAuth,
			},
			Issuer: "ca-projectcontour-io",
		})

		f.Fixtures.Echo.Deploy(namespace, "echo-with-optional-auth")

		f.Certs.CreateCertificate(e2e.CertificateSpec{
			Namespace:  namespace,
			Name:       "echo-with-optional-auth-cert",
			SecretName: "echo-with-optional-auth",
			DNSNames:   []string{"echo-with-optional-auth.projectcontour.io"},
			Usages: []e2e.KeyUsage{
				e2e.UsageServerAuth,
			},
			Issuer: "ca-projectcontour-io",
		})

		f.Fixtures.Echo.Deploy(namespace, "echo-with-optional-auth-no-ca")

		f.Certs.CreateCertificate(e2e.CertificateSpec{
			Namespace:  namespace,
			Name:       "echo-with-optional-auth-no-ca-cert",
			SecretName: "echo-with-optional-auth-no-ca",
			DNSNames:   []string{"echo-with-optional-auth-no-ca.projectcontour.io"},
			Usages: []e2e.KeyUsage{
				e2e.UsageServerAuth,
			},
			Issuer: "ca-projectcontour-io",
		})

		clientCert := e2e.CertificateSpec{
			Namespace:  namespace,
			Name:       "echo-client-cert",
			SecretName: "echo-client",
			CommonName: "client",
			EmailAddresses: []string{
				"client@projectcontour.io",
			},
			Usages: []e2e.KeyUsage{
				e2e.UsageClientAuth,
			},
			Issuer: "ca-projectcontour-io",
		}
		require.True(f.T(), f.Certs.CreateCertificateAndWait(clientCert))

		clientCertInvalid := e2e.CertificateSpec{
			Namespace:  namespace,
			Name:       "echo-client-cert-invalid",
			SecretName: "echo-client-invalid",
			CommonName: "badclient",
			EmailAddresses: []string{
				"badclient@projectcontour.io",
			},
			Usages: []e2e.KeyUsage{
				e2e.UsageClientAuth,
			},
			Issuer: "ca-notprojectcontour-io",
		}
		require.True(f.T(), f.Certs.CreateCertificateAndWait(clientCertInvalid))

		// This proxy does not require client certificate auth.
		noAuthProxy := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "echo-no-auth",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "echo-no-auth.projectcontour.io",
					TLS: &contour_v1.TLS{
						SecretName: "echo-no-auth",
					},
				},
				Routes: []contour_v1.Route{
					{
						Services: []contour_v1.Service{
							{
								Name: "echo-no-auth",
								Port: 80,
							},
						},
					},
				},
			},
		}
		require.True(f.T(), f.CreateHTTPProxyAndWaitFor(noAuthProxy, e2e.HTTPProxyValid))

		// This proxy requires client certificate auth.
		authProxy := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "echo-with-auth",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "echo-with-auth.projectcontour.io",
					TLS: &contour_v1.TLS{
						SecretName: "echo-with-auth",
						ClientValidation: &contour_v1.DownstreamValidation{
							CACertificate: "echo-with-auth",
						},
					},
				},
				Routes: []contour_v1.Route{
					{
						Services: []contour_v1.Service{
							{
								Name: "echo-with-auth",
								Port: 80,
							},
						},
					},
				},
			},
		}
		require.True(f.T(), f.CreateHTTPProxyAndWaitFor(authProxy, e2e.HTTPProxyValid))

		// This proxy does not verify client certs.
		authSkipVerifyProxy := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "echo-with-auth-skip-verify",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "echo-with-auth-skip-verify.projectcontour.io",
					TLS: &contour_v1.TLS{
						SecretName: "echo-with-auth-skip-verify",
						ClientValidation: &contour_v1.DownstreamValidation{
							SkipClientCertValidation: true,
						},
					},
				},
				Routes: []contour_v1.Route{
					{
						Services: []contour_v1.Service{
							{
								Name: "echo-with-auth-skip-verify",
								Port: 80,
							},
						},
					},
				},
			},
		}
		require.True(f.T(), f.CreateHTTPProxyAndWaitFor(authSkipVerifyProxy, e2e.HTTPProxyValid))

		// This proxy requires a client certificate but does not verify it.
		authSkipVerifyWithCAProxy := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "echo-with-auth-skip-verify-with-ca",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "echo-with-auth-skip-verify-with-ca.projectcontour.io",
					TLS: &contour_v1.TLS{
						SecretName: "echo-with-auth-skip-verify-with-ca",
						ClientValidation: &contour_v1.DownstreamValidation{
							SkipClientCertValidation: true,
							CACertificate:            "echo-with-auth",
						},
					},
				},
				Routes: []contour_v1.Route{
					{
						Services: []contour_v1.Service{
							{
								Name: "echo-with-auth-skip-verify-with-ca",
								Port: 80,
							},
						},
					},
				},
			},
		}
		require.True(f.T(), f.CreateHTTPProxyAndWaitFor(authSkipVerifyWithCAProxy, e2e.HTTPProxyValid))

		// This proxy requests a client certificate but only verifies it if sent.
		optionalAuthProxy := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "echo-with-optional-auth",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "echo-with-optional-auth.projectcontour.io",
					TLS: &contour_v1.TLS{
						SecretName: "echo-with-optional-auth",
						ClientValidation: &contour_v1.DownstreamValidation{
							OptionalClientCertificate: true,
							CACertificate:             "echo-with-auth",
						},
					},
				},
				Routes: []contour_v1.Route{
					{
						Services: []contour_v1.Service{
							{
								Name: "echo-with-optional-auth",
								Port: 80,
							},
						},
					},
				},
			},
		}
		require.True(f.T(), f.CreateHTTPProxyAndWaitFor(optionalAuthProxy, e2e.HTTPProxyValid))

		// This proxy requests a client certificate but doesn't verify it if sent.
		optionalAuthNoCAProxy := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "echo-with-optional-auth-no-ca",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "echo-with-optional-auth-no-ca.projectcontour.io",
					TLS: &contour_v1.TLS{
						SecretName: "echo-with-optional-auth-no-ca",
						ClientValidation: &contour_v1.DownstreamValidation{
							OptionalClientCertificate: true,
							SkipClientCertValidation:  true,
						},
					},
				},
				Routes: []contour_v1.Route{
					{
						Services: []contour_v1.Service{
							{
								Name: "echo-with-optional-auth-no-ca",
								Port: 80,
							},
						},
					},
				},
			},
		}
		require.True(f.T(), f.CreateHTTPProxyAndWaitFor(optionalAuthNoCAProxy, e2e.HTTPProxyValid))

		// get the valid & invalid client certs
		validClientCert, _ := f.Certs.GetTLSCertificate(namespace, clientCert.SecretName)
		invalidClientCert, _ := f.Certs.GetTLSCertificate(namespace, clientCertInvalid.SecretName)

		cases := map[string]struct {
			host       string
			clientCert *tls.Certificate
			wantErr    string
		}{
			"echo-no-auth without a client cert should succeed": {
				host:       noAuthProxy.Spec.VirtualHost.Fqdn,
				clientCert: nil,
				wantErr:    "",
			},
			"echo-no-auth with echo-client-cert should succeed": {
				host:       noAuthProxy.Spec.VirtualHost.Fqdn,
				clientCert: &validClientCert,
				wantErr:    "",
			},
			"echo-no-auth with echo-client-cert-invalid should succeed": {
				host:       noAuthProxy.Spec.VirtualHost.Fqdn,
				clientCert: &invalidClientCert,
				wantErr:    "",
			},

			"echo-with-auth without a client cert should error": {
				host:       authProxy.Spec.VirtualHost.Fqdn,
				clientCert: nil,
				wantErr:    "tls: certificate required",
			},
			"echo-with-auth with echo-client-cert should succeed": {
				host:       authProxy.Spec.VirtualHost.Fqdn,
				clientCert: &validClientCert,
				wantErr:    "",
			},
			"echo-with-auth with echo-client-cert-invalid should error": {
				host:       authProxy.Spec.VirtualHost.Fqdn,
				clientCert: &invalidClientCert,
				wantErr:    "tls: unknown certificate authority",
			},

			"echo-with-auth-skip-verify without a client cert should succeed": {
				host:       authSkipVerifyProxy.Spec.VirtualHost.Fqdn,
				clientCert: nil,
				wantErr:    "",
			},
			"echo-with-auth-skip-verify with echo-client-cert should succeed": {
				host:       authSkipVerifyProxy.Spec.VirtualHost.Fqdn,
				clientCert: &validClientCert,
				wantErr:    "",
			},
			"echo-with-auth-skip-verify with echo-client-cert-invalid should succeed": {
				host:       authSkipVerifyProxy.Spec.VirtualHost.Fqdn,
				clientCert: &invalidClientCert,
				wantErr:    "",
			},

			"echo-with-auth-skip-verify-with-ca without a client cert should error": {
				host:       authSkipVerifyWithCAProxy.Spec.VirtualHost.Fqdn,
				clientCert: nil,
				wantErr:    "tls: certificate required",
			},
			"echo-with-auth-skip-verify-with-ca with echo-client-cert should succeed": {
				host:       authSkipVerifyWithCAProxy.Spec.VirtualHost.Fqdn,
				clientCert: &validClientCert,
				wantErr:    "",
			},
			"echo-with-auth-skip-verify-with-ca with echo-client-cert-invalid should succeed": {
				host:       authSkipVerifyWithCAProxy.Spec.VirtualHost.Fqdn,
				clientCert: &invalidClientCert,
				wantErr:    "",
			},

			"echo-with-optional-auth without a client cert should succeed": {
				host:       optionalAuthProxy.Spec.VirtualHost.Fqdn,
				clientCert: nil,
				wantErr:    "",
			},
			"echo-with-optional-auth with echo-client-cert should succeed": {
				host:       optionalAuthProxy.Spec.VirtualHost.Fqdn,
				clientCert: &validClientCert,
				wantErr:    "",
			},
			"echo-with-optional-auth with echo-client-cert-invalid should error": {
				host:       optionalAuthProxy.Spec.VirtualHost.Fqdn,
				clientCert: &invalidClientCert,
				wantErr:    "tls: unknown certificate authority",
			},
			"echo-with-optional-auth-no-ca without a client cert should succeed": {
				host:       optionalAuthNoCAProxy.Spec.VirtualHost.Fqdn,
				clientCert: nil,
				wantErr:    "",
			},
			"echo-with-optional-auth-no-ca with echo-client-cert should succeed": {
				host:       optionalAuthNoCAProxy.Spec.VirtualHost.Fqdn,
				clientCert: &validClientCert,
				wantErr:    "",
			},
			"echo-with-optional-auth-no-ca with echo-client-cert-invalid should succeed": {
				host:       optionalAuthNoCAProxy.Spec.VirtualHost.Fqdn,
				clientCert: &invalidClientCert,
				wantErr:    "",
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

					return strings.Contains(err.Error(), tc.wantErr)
				}, f.RetryTimeout, f.RetryInterval)
			}
		}
	})
}

func optUseClientCert(cert *tls.Certificate) func(*tls.Config) {
	return func(c *tls.Config) {
		// Use c.GetClientCertificate rather than setting c.Certificates so the
		// client cert specified is always presented, regardless of the request
		// details from the server.
		c.GetClientCertificate = func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
			return cert, nil
		}
	}
}
