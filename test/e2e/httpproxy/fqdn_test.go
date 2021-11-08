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
// +build e2e

package httpproxy

import (
	. "github.com/onsi/ginkgo"
	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func testWildcardFQDN(namespace string) {
	Specify("invalid wildcard fqdn", func() {
		t := f.T()

		p := &contourv1.HTTPProxy{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "wildcard-subdomain",
			},
			Spec: contourv1.HTTPProxySpec{
				VirtualHost: &contourv1.VirtualHost{
					Fqdn: "*",
				},
				Routes: []contourv1.Route{{
					Services: []contourv1.Service{{
						Name: "ingress-conformance-echo",
						Port: 80,
					}},
				}},
			},
		}

		// Creation should fail the kubebuilder CRD validations.
		err := f.CreateHTTPProxy(p)
		require.NotNil(t, err, "Expected invalid wildcard to be rejected.")
	})

}

func testWildcardSubdomainFQDN(namespace string) {

	Specify("wildcard routing works", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "wildcarddomainio")
		f.Fixtures.Echo.Deploy(namespace, "domainio")
		f.Fixtures.Echo.Deploy(namespace, "bardomainio")

		proxyWildcard := &contourv1.HTTPProxy{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "wildcard",
			},
			Spec: contourv1.HTTPProxySpec{
				VirtualHost: &contourv1.VirtualHost{
					Fqdn: "*.domain.io",
				},
				Routes: []contourv1.Route{{
					Services: []contourv1.Service{
						{
							Name: "wildcarddomainio",
							Port: 80,
						},
					},
				}},
			},
		}
		proxyFullFQDN := &contourv1.HTTPProxy{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "full-fqdn",
			},
			Spec: contourv1.HTTPProxySpec{
				VirtualHost: &contourv1.VirtualHost{
					Fqdn: "domain.io",
				},
				Routes: []contourv1.Route{{
					Services: []contourv1.Service{
						{
							Name: "domainio",
							Port: 80,
						},
					},
				}},
			},
		}
		proxyFullFQDNSubdomain := &contourv1.HTTPProxy{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "fqdn-subdomain",
			},
			Spec: contourv1.HTTPProxySpec{
				VirtualHost: &contourv1.VirtualHost{
					Fqdn: "bar.domain.io",
				},
				Routes: []contourv1.Route{{
					Services: []contourv1.Service{
						{
							Name: "bardomainio",
							Port: 80,
						},
					},
				}},
			},
		}
		f.CreateHTTPProxyAndWaitFor(proxyWildcard, httpProxyValid)
		f.CreateHTTPProxyAndWaitFor(proxyFullFQDN, httpProxyValid)
		f.CreateHTTPProxyAndWaitFor(proxyFullFQDNSubdomain, httpProxyValid)

		cases := map[string]ServiceResult{
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

type ServiceResult struct {
	ServiceName   string
	ShouldSucceed bool
}
