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
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func testHostRewriteLiteral(namespace string) {
	Specify("hostname can be rewritten with policy on route", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "ingress-conformance-echo")

		p := &contourv1.HTTPProxy{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "host-header-rewrite",
			},
			Spec: contourv1.HTTPProxySpec{
				VirtualHost: &contourv1.VirtualHost{
					Fqdn: "hostheaderrewrite.projectcontour.io",
				},
				Routes: []contourv1.Route{
					{
						Services: []contourv1.Service{
							{
								Name: "ingress-conformance-echo",
								Port: 80,
							},
						},
						RequestHeadersPolicy: &contourv1.HeadersPolicy{
							Set: []contourv1.HeaderValue{
								{
									Name:  "Host",
									Value: "rewritten.com",
								},
							},
						},
					},
				},
			},
		}
		f.CreateHTTPProxyAndWaitFor(p, e2e.HTTPProxyValid)

		res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Condition: e2e.HasStatusCode(200),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)

		assert.Equal(t, "rewritten.com", f.GetEchoResponseBody(res.Body).Host)
	})
}

func testHostRewriteHeaderHTTPService(namespace string) {
	opts := []func(*http.Request){
		e2e.OptSetHeaders(map[string]string{
			"x-host-rewrite": "dynamichostrewritten.com",
		}),
	}

	Specify("hostname can be rewritten from header with policy on route", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "ingress-conformance-echo")

		p := &contourv1.HTTPProxy{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "host-header-rewrite",
			},
			Spec: contourv1.HTTPProxySpec{
				VirtualHost: &contourv1.VirtualHost{
					Fqdn: "dynamichostrewrite.projectcontour.io",
				},
				Routes: []contourv1.Route{
					{
						Services: []contourv1.Service{
							{
								Name: "ingress-conformance-echo",
								Port: 80,
							},
						},
						RequestHeadersPolicy: &contourv1.HeadersPolicy{
							Set: []contourv1.HeaderValue{
								{
									Name:  "Host",
									Value: "%REQ(x-host-rewrite)%",
								},
							},
						},
					},
				},
			},
		}
		f.CreateHTTPProxyAndWaitFor(p, e2e.HTTPProxyValid)

		res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:        p.Spec.VirtualHost.Fqdn,
			Condition:   e2e.HasStatusCode(200),
			RequestOpts: opts,
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)

		assert.Equal(t, "dynamichostrewritten.com", f.GetEchoResponseBody(res.Body).Host)
	})
}

func testHostRewriteHeaderHTTPSService(namespace string) {
	opts := []func(*http.Request){
		e2e.OptSetHeaders(map[string]string{
			"x-host-rewrite": "securedynamichostrewritten.com",
		}),
	}

	Specify("hostname can be rewritten with policy on route with https", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "ingress-conformance-echo")
		f.Certs.CreateSelfSignedCert(namespace, "ingress-conformance-echo", "ingress-conformance-echo", "https.hostheaderrewrite.projectcontour.io")

		p := &contourv1.HTTPProxy{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "host-header-rewrite",
			},
			Spec: contourv1.HTTPProxySpec{
				VirtualHost: &contourv1.VirtualHost{
					Fqdn: "https.dynamichostrewrite.projectcontour.io",
					TLS: &contourv1.TLS{
						SecretName: "ingress-conformance-echo",
					},
				},
				Routes: []contourv1.Route{
					{
						Services: []contourv1.Service{
							{
								Name: "ingress-conformance-echo",
								Port: 80,
							},
						},
						RequestHeadersPolicy: &contourv1.HeadersPolicy{
							Set: []contourv1.HeaderValue{
								{
									Name:  "Host",
									Value: "%REQ(x-host-rewrite)%",
								},
							},
						},
					},
				},
			},
		}
		f.CreateHTTPProxyAndWaitFor(p, e2e.HTTPProxyValid)

		res, ok := f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
			Host:        p.Spec.VirtualHost.Fqdn,
			Condition:   e2e.HasStatusCode(200),
			RequestOpts: opts,
		})

		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)

		assert.Equal(t, "securedynamichostrewritten.com", f.GetEchoResponseBody(res.Body).Host)
	})
}

func testHostRewriteHeaderExternalNameService(namespace string) {
	opts := []func(*http.Request){
		e2e.OptSetHeaders(map[string]string{
			"x-host-rewrite": "external.newhostrewritten.com",
		}),
	}

	Specify("hostname can be rewritten from header with policy on route", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "ingress-conformance-echo")

		externalNameService := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "external-name-service",
			},
			Spec: corev1.ServiceSpec{
				Type:         corev1.ServiceTypeExternalName,
				ExternalName: "ingress-conformance-echo." + namespace,
				Ports: []corev1.ServicePort{
					{
						Name: "http",
						Port: 80,
					},
				},
			},
		}
		require.NoError(t, f.Client.Create(context.TODO(), externalNameService))

		p := &contourv1.HTTPProxy{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "host-header-rewrite",
			},
			Spec: contourv1.HTTPProxySpec{
				VirtualHost: &contourv1.VirtualHost{
					Fqdn: "externalhostheaderrewrite.projectcontour.io",
				},
				Routes: []contourv1.Route{
					{
						Services: []contourv1.Service{
							{
								Name: externalNameService.Name,
								Port: 80,
							},
						},
						RequestHeadersPolicy: &contourv1.HeadersPolicy{
							Set: []contourv1.HeaderValue{
								{
									Name:  "Host",
									Value: "%REQ(x-host-rewrite)%",
								},
							},
						},
					},
				},
			},
		}
		f.CreateHTTPProxyAndWaitFor(p, e2e.HTTPProxyValid)

		res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:        p.Spec.VirtualHost.Fqdn,
			Condition:   e2e.HasStatusCode(200),
			RequestOpts: opts,
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)

		assert.Equal(t, "external.newhostrewritten.com", f.GetEchoResponseBody(res.Body).Host)
	})
}
