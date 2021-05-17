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

package httpproxy

import (
	"context"

	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func testExternalNameServiceInsecure(fx *e2e.Framework) {
	t := fx.T()
	namespace := "018-external-name-service-insecure"

	fx.CreateNamespace(namespace)
	defer fx.DeleteNamespace(namespace)

	fx.Fixtures.Echo.Deploy(namespace, "ingress-conformance-echo")

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
	require.NoError(t, fx.Client.Create(context.TODO(), externalNameService))

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
	fx.CreateHTTPProxyAndWaitFor(p, httpProxyValid)

	res, ok := fx.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
		Host:      p.Spec.VirtualHost.Fqdn,
		Condition: e2e.HasStatusCode(200),
	})
	require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)
}

func testExternalNameServiceTLS(fx *e2e.Framework) {
	t := fx.T()
	namespace := "018-external-name-service-tls"

	fx.CreateNamespace(namespace)
	defer fx.DeleteNamespace(namespace)

	fx.Certs.CreateSelfSignedCert(namespace, "backend-server-cert", "backend-server-cert", "echo")

	fx.Fixtures.EchoSecure.Deploy(namespace, "echo-tls")

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
	require.NoError(t, fx.Client.Create(context.TODO(), externalNameService))

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
							Protocol: stringPtr("tls"),
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
	fx.CreateHTTPProxyAndWaitFor(p, httpProxyValid)

	res, ok := fx.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
		Host:      p.Spec.VirtualHost.Fqdn,
		Condition: e2e.HasStatusCode(200),
	})
	require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)
}

func stringPtr(s string) *string {
	return &s
}
