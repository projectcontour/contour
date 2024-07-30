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
	"github.com/stretchr/testify/require"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/test/e2e"
)

func testExternalNameServiceInsecure(namespace string) {
	Specify("external name services work over http", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "ingress-conformance-echo")

		externalNameService := &core_v1.Service{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "external-name-service",
			},
			Spec: core_v1.ServiceSpec{
				Type:         core_v1.ServiceTypeExternalName,
				ExternalName: "ingress-conformance-echo." + namespace,
				Ports: []core_v1.ServicePort{
					{
						Name: "http",
						Port: 80,
					},
				},
			},
		}
		require.NoError(t, f.Client.Create(context.TODO(), externalNameService))

		p := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "external-name-proxy",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "externalnameservice.projectcontour.io",
				},
				Routes: []contour_v1.Route{
					{
						Services: []contour_v1.Service{
							{
								Name: externalNameService.Name,
								Port: 80,
							},
						},
						RequestHeadersPolicy: &contour_v1.HeadersPolicy{
							Set: []contour_v1.HeaderValue{
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
		require.True(f.T(), f.CreateHTTPProxyAndWaitFor(p, e2e.HTTPProxyValid))

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

		f.Fixtures.EchoSecure.Deploy(namespace, "echo-tls", nil)

		externalNameService := &core_v1.Service{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "external-name-service-tls",
			},
			Spec: core_v1.ServiceSpec{
				Type:         core_v1.ServiceTypeExternalName,
				ExternalName: "echo-tls." + namespace,
				Ports: []core_v1.ServicePort{
					{
						Name:     "https",
						Port:     443,
						Protocol: core_v1.ProtocolTCP,
					},
				},
			},
		}
		require.NoError(t, f.Client.Create(context.TODO(), externalNameService))

		p := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "external-name-proxy-tls",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "tls.externalnameservice.projectcontour.io",
				},
				Routes: []contour_v1.Route{
					{
						Services: []contour_v1.Service{
							{
								Name:     externalNameService.Name,
								Port:     443,
								Protocol: ptr.To("tls"),
							},
						},
						RequestHeadersPolicy: &contour_v1.HeadersPolicy{
							Set: []contour_v1.HeaderValue{
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
		require.True(f.T(), f.CreateHTTPProxyAndWaitFor(p, e2e.HTTPProxyValid))

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

		externalNameService := &core_v1.Service{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "external-name-service-localhost",
			},
			Spec: core_v1.ServiceSpec{
				Type: core_v1.ServiceTypeExternalName,
				// The unit tests test just `localhost`, so test another item from that
				// list.
				ExternalName: "localhost.localdomain",
				Ports: []core_v1.ServicePort{
					{
						Name: "http",
						Port: 80,
					},
				},
			},
		}
		require.NoError(t, f.Client.Create(context.TODO(), externalNameService))

		p := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "external-name-proxy",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "externalnameservice.projectcontour.io",
				},
				Routes: []contour_v1.Route{
					{
						Services: []contour_v1.Service{
							{
								Name: externalNameService.Name,
								Port: 80,
							},
						},
						RequestHeadersPolicy: &contour_v1.HeadersPolicy{
							Set: []contour_v1.HeaderValue{
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
		require.Truef(f.T(), f.CreateHTTPProxyAndWaitFor(p, func(proxy *contour_v1.HTTPProxy) bool {
			validCond := proxy.Status.GetConditionFor(contour_v1.ValidConditionType)
			if validCond == nil {
				return false
			}
			if validCond.Status != meta_v1.ConditionFalse {
				return false
			}

			for _, err := range validCond.Errors {
				if err.Type == contour_v1.ConditionTypeServiceError &&
					err.Reason == "ServiceUnresolvedReference" &&
					strings.Contains(err.Message, "is an ExternalName service that points to localhost") {
					return true
				}
			}

			return false
		}), "ExternalName with hostname %s was accepted by Contour.", externalNameService.Spec.ExternalName)
	})
}
