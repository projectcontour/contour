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
	"net/http"
	"testing"

	certmanagerv1 "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1"
	certmanagermetav1 "github.com/jetstack/cert-manager/pkg/apis/meta/v1"
	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestHTTPSSNIEnforcement(t *testing.T) {
	t.Parallel()

	// Start by assuming install-contour-working.sh has been run, so we
	// have Contour running in a cluster. Later we may want to move part
	// or all of that script into the E2E framework.

	var (
		fx        = NewFramework(t)
		namespace = "004-https-sni-enforcement"
	)

	fx.CreateNamespace(namespace)
	defer fx.DeleteNamespace(namespace)

	issuer := &certmanagerv1.Issuer{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "selfsigned",
		},
		Spec: certmanagerv1.IssuerSpec{
			IssuerConfig: certmanagerv1.IssuerConfig{
				SelfSigned: &certmanagerv1.SelfSignedIssuer{},
			},
		},
	}
	require.NoError(t, fx.Client.Create(context.TODO(), issuer))

	fx.CreateEchoWorkload(namespace, "echo-one")

	echoOneCert := &certmanagerv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "echo-one-cert",
		},
		Spec: certmanagerv1.CertificateSpec{
			DNSNames:   []string{"sni-enforcement-echo-one.projectcontour.io"},
			SecretName: "echo-one",
			IssuerRef: certmanagermetav1.ObjectReference{
				Name: "selfsigned",
			},
		},
	}
	require.NoError(t, fx.Client.Create(context.TODO(), echoOneCert))

	echoOneProxy := &contourv1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "echo-one",
		},
		Spec: contourv1.HTTPProxySpec{
			VirtualHost: &contourv1.VirtualHost{
				Fqdn: "sni-enforcement-echo-one.projectcontour.io",
				TLS: &contourv1.TLS{
					SecretName: "echo-one",
				},
			},
			Routes: []contourv1.Route{
				{
					Services: []contourv1.Service{
						{
							Name: "echo-one",
							Port: 80,
						},
					},
				},
			},
		},
	}
	fx.CreateHTTPProxyAndWaitFor(echoOneProxy, HTTPProxyValid)

	res, ok := fx.HTTPSRequestUntil(IsOK, "/https-sni-enforcement", echoOneProxy.Spec.VirtualHost.Fqdn)
	require.Truef(t, ok, "did not receive 200 response")

	assert.Equal(t, "echo-one", fx.GetEchoResponseBody(res.Body).Service)

	// echo-two
	fx.CreateEchoWorkload(namespace, "echo-two")

	echoTwoCert := &certmanagerv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "echo-two-cert",
		},
		Spec: certmanagerv1.CertificateSpec{
			DNSNames:   []string{"sni-enforcement-echo-two.projectcontour.io"},
			SecretName: "echo-two",
			IssuerRef: certmanagermetav1.ObjectReference{
				Name: "selfsigned",
			},
		},
	}
	require.NoError(t, fx.Client.Create(context.TODO(), echoTwoCert))

	echoTwoProxy := &contourv1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "echo-two",
		},
		Spec: contourv1.HTTPProxySpec{
			VirtualHost: &contourv1.VirtualHost{
				Fqdn: "sni-enforcement-echo-two.projectcontour.io",
				TLS: &contourv1.TLS{
					SecretName: "echo-two",
				},
			},
			Routes: []contourv1.Route{
				{
					Services: []contourv1.Service{
						{
							Name: "echo-two",
							Port: 80,
						},
					},
				},
			},
		},
	}
	fx.CreateHTTPProxyAndWaitFor(echoTwoProxy, HTTPProxyValid)

	res, ok = fx.HTTPSRequestUntil(IsOK, "/https-sni-enforcement", echoTwoProxy.Spec.VirtualHost.Fqdn)
	require.Truef(t, ok, "did not receive 200 response")

	assert.Equal(t, "echo-two", fx.GetEchoResponseBody(res.Body).Service)

	// Send a request to sni-enforcement-echo-two.projectcontour.io that has an SNI of
	// sni-enforcement-echo-one.projectcontour.io and ensure a 421 (Misdirected Request)
	// is returned.
	//
	// TODO can I make this a little cleaner?
	res, ok = fx.requestUntil(func() (*http.Response, error) {
		req, err := http.NewRequest("GET", fx.httpsUrlBase, nil)
		require.NoError(t, err, "error creating HTTP request")

		req.Host = echoTwoProxy.Spec.VirtualHost.Fqdn

		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.TLSClientConfig = &tls.Config{
			ServerName:         echoOneProxy.Spec.VirtualHost.Fqdn, // this SNI does not match the Host
			InsecureSkipVerify: true,
		}

		client := &http.Client{
			Transport: transport,
		}

		return client.Do(req)
	}, func(res *http.Response) bool {
		return res.StatusCode == 421 // misdirected request
	})
	require.Truef(t, ok, "did not receive 421 (Misdirected Request) response")
}
