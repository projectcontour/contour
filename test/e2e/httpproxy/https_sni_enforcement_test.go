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

	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/test/e2e"
)

func testHTTPSSNIEnforcement(namespace string) {
	Specify("SNI routing works and hostname must match", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "echo-one")
		f.Certs.CreateSelfSignedCert(namespace, "echo-one-cert", "echo-one", "sni-enforcement-echo-one.projectcontour.io")

		echoOneProxy := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "echo-one",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "sni-enforcement-echo-one.projectcontour.io",
					TLS: &contour_v1.TLS{
						SecretName: "echo-one",
					},
				},
				Routes: []contour_v1.Route{
					{
						Services: []contour_v1.Service{
							{
								Name: "echo-one",
								Port: 80,
							},
						},
					},
				},
			},
		}
		f.CreateHTTPProxyAndWaitFor(echoOneProxy, e2e.HTTPProxyValid)

		res, ok := f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
			Host:      echoOneProxy.Spec.VirtualHost.Fqdn,
			Path:      "/https-sni-enforcement",
			Condition: e2e.HasStatusCode(200),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)

		assert.Equal(t, "echo-one", f.GetEchoResponseBody(res.Body).Service)

		// echo-two
		f.Fixtures.Echo.Deploy(namespace, "echo-two")
		f.Certs.CreateSelfSignedCert(namespace, "echo-two-cert", "echo-two", "sni-enforcement-echo-two.projectcontour.io")

		echoTwoProxy := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "echo-two",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "sni-enforcement-echo-two.projectcontour.io",
					TLS: &contour_v1.TLS{
						SecretName: "echo-two",
					},
				},
				Routes: []contour_v1.Route{
					{
						Services: []contour_v1.Service{
							{
								Name: "echo-two",
								Port: 80,
							},
						},
					},
				},
			},
		}
		f.CreateHTTPProxyAndWaitFor(echoTwoProxy, e2e.HTTPProxyValid)

		res, ok = f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
			Host:      echoTwoProxy.Spec.VirtualHost.Fqdn,
			Path:      "/https-sni-enforcement",
			Condition: e2e.HasStatusCode(200),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)

		assert.Equal(t, "echo-two", f.GetEchoResponseBody(res.Body).Service)

		// Send a request to sni-enforcement-echo-two.projectcontour.io that has an SNI of
		// sni-enforcement-echo-one.projectcontour.io and ensure a 421 (Misdirected Request)
		// is returned.
		res, ok = f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
			Host: echoTwoProxy.Spec.VirtualHost.Fqdn,
			TLSConfigOpts: []func(*tls.Config){
				e2e.OptSetSNI(echoOneProxy.Spec.VirtualHost.Fqdn),
			},
			Condition: e2e.HasStatusCode(421),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 421 (Misdirected Request) response code, got %d", res.StatusCode)
	})
}
