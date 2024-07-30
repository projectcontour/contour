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

	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/require"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/test/e2e"
)

func testTCPRouteHTTPSTermination(namespace string) {
	Specify("tcproute can terminate HTTPS", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "ingress-conformance-echo")
		f.Certs.CreateSelfSignedCert(namespace, "echo-cert", "echo-cert", "tcp-route-https-termination.projectcontour.io")

		p := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "echo-tcpproxy",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "tcp-route-https-termination.projectcontour.io",
					TLS: &contour_v1.TLS{
						SecretName: "echo-cert",
					},
				},
				TCPProxy: &contour_v1.TCPProxy{
					Services: []contour_v1.Service{
						{
							Name: "ingress-conformance-echo",
							Port: 80,
						},
					},
				},
			},
		}
		require.True(f.T(), f.CreateHTTPProxyAndWaitFor(p, e2e.HTTPProxyValid))

		certSecret := &core_v1.Secret{}
		key := client.ObjectKey{Namespace: namespace, Name: "echo-cert"}
		require.NoError(t, f.Client.Get(context.TODO(), key, certSecret))

		res, ok := f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
			Host: p.Spec.VirtualHost.Fqdn,
			TLSConfigOpts: []func(*tls.Config){
				e2e.VerifyTLSServerCert(certSecret.Data["ca.crt"]),
			},
			Condition: e2e.HasStatusCode(200),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)
	})
}
