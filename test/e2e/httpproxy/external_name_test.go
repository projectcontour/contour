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
	"strings"

	. "github.com/onsi/ginkgo/v2"
	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/ref"
	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func testExternalNameServiceInsecure(namespace string) {
	Specify("external name services work over http", func() {
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
				Name:      "external-name-proxy",
			},
			Spec: contourv1.HTTPProxySpec{
				VirtualHost: &contourv1.VirtualHost{
					Fqdn: "externalnameservice.projectcontour.io",
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
									Value: externalNameService.Spec.ExternalName,
								},
							},
						},
					},
				},
			},
		}
		proxy, ok := f.CreateHTTPProxyAndWaitFor(p, e2e.HTTPProxyValid)
		if !ok {
			t.Fatalf("The HTTPProxy did not become valid, here are the Valid condition's Errors: %s", e2e.HTTPProxyErrors(proxy))
		}

		res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Condition: e2e.HasStatusCode(200),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)
	})
}

func testExternalNameServiceTLS(namespace string) {
	Specify("external name services work over https", func() {
		t := f.T()

		f.Certs.CreateSelfSignedCert(namespace, "backend-server-cert", "backend-server-cert", "echo")

		f.Fixtures.EchoSecure.Deploy(namespace, "echo-tls")

		externalNameService := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "external-name-service-tls",
			},
			Spec: corev1.ServiceSpec{
				Type:         corev1.ServiceTypeExternalName,
				ExternalName: "echo-tls." + namespace,
				Ports: []corev1.ServicePort{
					{
						Name:     "https",
						Port:     443,
						Protocol: corev1.ProtocolTCP,
					},
				},
			},
		}
		require.NoError(t, f.Client.Create(context.TODO(), externalNameService))

		p := &contourv1.HTTPProxy{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "external-name-proxy-tls",
			},
			Spec: contourv1.HTTPProxySpec{
				VirtualHost: &contourv1.VirtualHost{
					Fqdn: "tls.externalnameservice.projectcontour.io",
				},
				Routes: []contourv1.Route{
					{
						Services: []contourv1.Service{
							{
								Name:     externalNameService.Name,
								Port:     443,
								Protocol: ref.To("tls"),
							},
						},
						RequestHeadersPolicy: &contourv1.HeadersPolicy{
							Set: []contourv1.HeaderValue{
								{
									Name:  "Host",
									Value: externalNameService.Spec.ExternalName,
								},
							},
						},
					},
				},
			},
		}
		proxy, ok := f.CreateHTTPProxyAndWaitFor(p, e2e.HTTPProxyValid)
		if !ok {
			t.Fatalf("The HTTPProxy did not become valid, here are the Valid condition's Errors: %s", e2e.HTTPProxyErrors(proxy))
		}

		res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Condition: e2e.HasStatusCode(200),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)
	})
}

func testExternalNameServiceLocalhostInvalid(namespace string) {
	Specify("external name services with localhost are rejected", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "ingress-conformance-echo")

		externalNameService := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "external-name-service-localhost",
			},
			Spec: corev1.ServiceSpec{
				Type: corev1.ServiceTypeExternalName,
				// The unit tests test just `localhost`, so test another item from that
				// list.
				ExternalName: "localhost.localdomain",
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
				Name:      "external-name-proxy",
			},
			Spec: contourv1.HTTPProxySpec{
				VirtualHost: &contourv1.VirtualHost{
					Fqdn: "externalnameservice.projectcontour.io",
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
									Value: externalNameService.Spec.ExternalName,
								},
							},
						},
					},
				},
			},
		}

		// The HTTPProxy should be marked invalid due to the service
		// using localhost.localdomain.
		_, invalid := f.CreateHTTPProxyAndWaitFor(p, func(proxy *contourv1.HTTPProxy) bool {
			validCond := proxy.Status.GetConditionFor(contourv1.ValidConditionType)
			if validCond == nil {
				return false
			}
			if validCond.Status != metav1.ConditionFalse {
				return false
			}

			for _, err := range validCond.Errors {
				if err.Type == contourv1.ConditionTypeServiceError &&
					err.Reason == "ServiceUnresolvedReference" &&
					strings.Contains(err.Message, "is an ExternalName service that points to localhost") {
					return true
				}
			}

			return false
		})
		require.Truef(t, invalid, "ExternalName with hostname %s was accepted by Contour.", externalNameService.Spec.ExternalName)
	})
}
