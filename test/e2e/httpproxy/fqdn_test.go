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

	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	networking_v1 "k8s.io/api/networking/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/test/e2e"
)

func testWildcardFQDN(namespace string) {
	Specify("invalid wildcard fqdn", func() {
		t := f.T()

		p := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "wildcard-subdomain",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "*",
				},
				Routes: []contour_v1.Route{{
					Services: []contour_v1.Service{{
						Name: "ingress-conformance-echo",
						Port: 80,
					}},
				}},
			},
		}

		// Creation should fail the kubebuilder CRD validations.
		err := f.CreateHTTPProxy(p)
		require.Error(t, err, "Expected invalid wildcard to be rejected.")
	})
}

func testWildcardSubdomainFQDN(namespace string) {
	Specify("wildcard routing works", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "wildcarddomainio")
		f.Fixtures.Echo.Deploy(namespace, "domainio")
		f.Fixtures.Echo.Deploy(namespace, "bardomainio")

		proxyWildcard := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "wildcard",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "*.domain.io",
				},
				Routes: []contour_v1.Route{{
					Services: []contour_v1.Service{
						{
							Name: "wildcarddomainio",
							Port: 80,
						},
					},
				}},
			},
		}
		proxyFullFQDN := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "full-fqdn",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "domain.io",
				},
				Routes: []contour_v1.Route{{
					Services: []contour_v1.Service{
						{
							Name: "domainio",
							Port: 80,
						},
					},
				}},
			},
		}
		proxyFullFQDNSubdomain := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "fqdn-subdomain",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "bar.domain.io",
				},
				Routes: []contour_v1.Route{{
					Services: []contour_v1.Service{
						{
							Name: "bardomainio",
							Port: 80,
						},
					},
				}},
			},
		}
		require.True(f.T(), f.CreateHTTPProxyAndWaitFor(proxyWildcard, e2e.HTTPProxyValid))
		require.True(f.T(), f.CreateHTTPProxyAndWaitFor(proxyFullFQDN, e2e.HTTPProxyValid))
		require.True(f.T(), f.CreateHTTPProxyAndWaitFor(proxyFullFQDNSubdomain, e2e.HTTPProxyValid))

		cases := map[string]struct {
			ServiceName   string
			ShouldSucceed bool
		}{
			"domain.io": {
				ServiceName:   "domainio",
				ShouldSucceed: true,
			},
			"www.domain.io": {
				ServiceName:   "wildcarddomainio",
				ShouldSucceed: true,
			},
			"bar.domain.io": {
				ServiceName:   "bardomainio",
				ShouldSucceed: true,
			},
			"foo.domain.io": {
				ServiceName:   "wildcarddomainio",
				ShouldSucceed: true,
			},
			"bar.foo.domain.io": {
				ShouldSucceed: false,
			},
		}

		for fqdn, expectedService := range cases {
			t.Logf("Querying %q, expecting service %q", fqdn, expectedService.ServiceName)

			if expectedService.ShouldSucceed {
				res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
					Host:      fqdn,
					Path:      "/",
					Condition: e2e.HasStatusCode(200),
				})

				if !assert.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode) {
					continue
				}

				body := f.GetEchoResponseBody(res.Body)
				assert.Equal(t, namespace, body.Namespace)
				assert.Equal(t, expectedService.ServiceName, body.Service)
			} else {
				res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
					Host:      fqdn,
					Path:      "/",
					Condition: e2e.HasStatusCode(404),
				})

				assert.Truef(t, ok, "expected 404 response code, got %d", res.StatusCode)
			}
		}
	})
}

func testIngressWildcardSubdomainFQDN(namespace string) {
	Specify("wildcard from an Ingress is overridden by a HTTPProxy with subdomain", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "wildcarddomainio")
		f.Fixtures.Echo.Deploy(namespace, "bardomainio")

		ingressWildcard := &networking_v1.Ingress{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "wildcard-ingress",
			},
			Spec: networking_v1.IngressSpec{
				Rules: []networking_v1.IngressRule{
					{
						Host: "*.wildcard-override.projectcontour.io",
						IngressRuleValue: networking_v1.IngressRuleValue{
							HTTP: &networking_v1.HTTPIngressRuleValue{
								Paths: []networking_v1.HTTPIngressPath{
									{
										PathType: ptr.To(networking_v1.PathTypePrefix),
										Path:     "/",
										Backend: networking_v1.IngressBackend{
											Service: &networking_v1.IngressServiceBackend{
												Name: "wildcarddomainio",
												Port: networking_v1.ServiceBackendPort{
													Number: 80,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}
		require.NoError(t, f.Client.Create(context.TODO(), ingressWildcard))

		proxySubdomain := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "fqdn-subdomain",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "bar.wildcard-override.projectcontour.io",
				},
				Routes: []contour_v1.Route{{
					Services: []contour_v1.Service{
						{
							Name: "bardomainio",
							Port: 80,
						},
					},
				}},
			},
		}
		require.True(f.T(), f.CreateHTTPProxyAndWaitFor(proxySubdomain, e2e.HTTPProxyValid))

		res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      "www.wildcard-override.projectcontour.io",
			Path:      "/",
			Condition: e2e.HasStatusCode(200),
		})
		require.NotNil(t, res, "request never succeeded")
		assert.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)
		body := f.GetEchoResponseBody(res.Body)
		assert.Equal(t, namespace, body.Namespace)
		assert.Equal(t, "wildcarddomainio", body.Service)

		res, ok = f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      "bar.wildcard-override.projectcontour.io",
			Path:      "/",
			Condition: e2e.HasStatusCode(200),
		})
		require.NotNil(t, res, "request never succeeded")
		assert.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)
		body = f.GetEchoResponseBody(res.Body)
		assert.Equal(t, namespace, body.Namespace)
		assert.Equal(t, "bardomainio", body.Service)
	})
}
