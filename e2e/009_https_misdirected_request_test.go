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
	"crypto/tls"
	"net/http"
	"testing"

	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestHTTPSMisdirectedRequest(t *testing.T) {
	t.Parallel()

	// Start by assuming install-contour-working.sh has been run, so we
	// have Contour running in a cluster. Later we may want to move part
	// or all of that script into the E2E framework.

	var (
		fx        = NewFramework(t)
		namespace = "009-https-misdirected-request"
	)

	fx.CreateNamespace(namespace)
	defer fx.DeleteNamespace(namespace)

	fx.CreateEchoWorkload(namespace, "echo")
	fx.CreateSelfSignedCert(namespace, "echo-cert", "echo", "https-misdirected-request.projectcontour.io")

	p := &contourv1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "echo",
		},
		Spec: contourv1.HTTPProxySpec{
			VirtualHost: &contourv1.VirtualHost{
				Fqdn: "https-misdirected-request.projectcontour.io",
				TLS: &contourv1.TLS{
					SecretName: "echo",
				},
			},
			Routes: []contourv1.Route{
				{
					Services: []contourv1.Service{
						{
							Name: "echo",
							Port: 80,
						},
					},
				},
			},
		},
	}
	fx.CreateHTTPProxyAndWaitFor(p, HTTPProxyValid)

	res, ok := fx.HTTPSRequestUntil(IsOK, "/", p.Spec.VirtualHost.Fqdn)
	require.Truef(t, ok, "did not receive 200 response")

	assert.Equal(t, "echo", fx.GetEchoResponseBody(res.Body).Service)

	// Send a request to sni-enforcement-echo-two.projectcontour.io that has an SNI of
	// sni-enforcement-echo-one.projectcontour.io and ensure a 421 (Misdirected Request)
	// is returned.
	//
	// TODO can I make this a little cleaner?
	res, ok = fx.requestUntil(func() (*http.Response, error) {
		req, err := http.NewRequest("GET", fx.httpsUrlBase, nil)
		require.NoError(t, err, "error creating HTTP request")

		// this Host value does not match the SNI name.
		req.Host = "non-matching-host.projectcontour.io"

		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.TLSClientConfig = &tls.Config{
			ServerName:         p.Spec.VirtualHost.Fqdn,
			InsecureSkipVerify: true,
		}

		client := &http.Client{
			Transport: transport,
		}

		return client.Do(req)
	}, HasStatusCode(421))
	require.Truef(t, ok, "did not receive 421 (Misdirected Request) response")

	res, ok = fx.requestUntil(func() (*http.Response, error) {
		req, err := http.NewRequest("GET", fx.httpsUrlBase, nil)
		require.NoError(t, err, "error creating HTTP request")

		// The virtual host name is port-insensitive, so verify that we can
		// stuff any old port number is and still succeed.
		req.Host = p.Spec.VirtualHost.Fqdn + ":9999"

		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.TLSClientConfig = &tls.Config{
			ServerName:         p.Spec.VirtualHost.Fqdn,
			InsecureSkipVerify: true,
		}

		client := &http.Client{
			Transport: transport,
		}

		return client.Do(req)
	}, IsOK)
	require.Truef(t, ok, "did not receive 200 response")

	// Verify that the hostname match is case-insensitive.
	// The SNI server name match is still case sensitive,
	// see https://github.com/envoyproxy/envoy/issues/6199.
	res, ok = fx.requestUntil(func() (*http.Response, error) {
		req, err := http.NewRequest("GET", fx.httpsUrlBase, nil)
		require.NoError(t, err, "error creating HTTP request")

		req.Host = "HTTPS-Misdirected-reQUest.projectcontour.io"

		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.TLSClientConfig = &tls.Config{
			ServerName:         p.Spec.VirtualHost.Fqdn,
			InsecureSkipVerify: true,
		}

		client := &http.Client{
			Transport: transport,
		}

		return client.Do(req)
	}, IsOK)
	require.Truef(t, ok, "did not receive 200 response")
}
