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
	"crypto/x509"
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tsaarni/certyaml"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/test/e2e"
)

func testBackendTLS(namespace string, ca func() *certyaml.Certificate) {
	Specify("mTLS to backends can be configured", func() {
		f.Certs.CreateCertificate(namespace, "backend-server-cert", &certyaml.Certificate{
			Subject:         "cn=echo-secure",
			SubjectAltNames: []string{"DNS:echo-secure"},
			ExtKeyUsage:     []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
			Issuer:          ca(),
		})
		f.Fixtures.EchoSecure.Deploy(namespace, "echo-secure", nil)

		p := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "backend-tls",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "backend-tls.projectcontour.io",
				},
				Routes: []contour_v1.Route{
					{
						Services: []contour_v1.Service{
							{
								Name: "echo-secure",
								Port: 443,
								UpstreamValidation: &contour_v1.UpstreamValidation{
									CACertificate: "backend-client-cert",
									SubjectName:   "echo-secure",
								},
							},
						},
					},
				},
			},
		}
		require.True(f.T(), f.CreateHTTPProxyAndWaitFor(p, e2e.HTTPProxyValid))

		type responseTLSDetails struct {
			TLS struct {
				PeerCertificates []string
			}
		}

		// Send HTTP request, we will check backend connection was over HTTPS.
		res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Condition: e2e.HasStatusCode(200),
		})
		require.NotNil(f.T(), res, "request never succeeded")
		require.Truef(f.T(), ok, "expected 200 response code, got %d", res.StatusCode)

		// Get cert presented to backend app.
		tlsInfo := new(responseTLSDetails)
		require.NoError(f.T(), json.Unmarshal(res.Body, tlsInfo))
		require.Len(f.T(), tlsInfo.TLS.PeerCertificates, 1)

		// Get value of client cert Envoy should have presented.
		clientSecretKey := client.ObjectKey{Namespace: namespace, Name: "backend-client-cert"}
		clientSecret := &core_v1.Secret{}
		require.NoError(f.T(), f.Client.Get(context.TODO(), clientSecretKey, clientSecret))

		assert.Equal(f.T(), tlsInfo.TLS.PeerCertificates[0], string(clientSecret.Data["tls.crt"]))

		// Delete client cert so it is rotated.
		require.NoError(f.T(), f.Client.Delete(context.TODO(), clientSecret))
		// Rotate cert by re-creating the secret with same name but different cert.
		f.Certs.CreateCertificate(namespace, "backend-client-cert", &certyaml.Certificate{
			Subject:     "cn=client",
			ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
			Issuer:      ca(),
		})
		require.NoError(f.T(), f.Client.Get(context.TODO(), clientSecretKey, clientSecret))

		// Send HTTP request again until we get a 200 and new cert is presented.
		require.Eventually(f.T(), func() bool {
			res, ok = f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
				Host:      p.Spec.VirtualHost.Fqdn,
				Condition: e2e.HasStatusCode(200),
			})
			if ok {
				// Get cert presented to backend app.
				tlsInfo = new(responseTLSDetails)
				require.NoError(f.T(), json.Unmarshal(res.Body, tlsInfo))
				require.Len(f.T(), tlsInfo.TLS.PeerCertificates, 1)
				return tlsInfo.TLS.PeerCertificates[0] == string(clientSecret.Data["tls.crt"])
			}
			return false
		}, time.Second*10, time.Millisecond*50)
	})
}
