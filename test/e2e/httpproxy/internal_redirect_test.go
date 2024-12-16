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
	"context"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/test/e2e"
)

func testInternalRedirectValidation(namespace string) {
	Specify("invalid cross scheme mode", func() {
		t := f.T()

		p := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "invalid-cross-scheme",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "example.com",
				},
				Routes: []contour_v1.Route{{
					Services: []contour_v1.Service{{
						Name: "ingress-conformance-echo",
						Port: 80,
					}},
					InternalRedirectPolicy: &contour_v1.HTTPInternalRedirectPolicy{
						AllowCrossSchemeRedirect: "MaybeSafe",
					},
				}},
			},
		}

		// Creation should fail the kubebuilder CRD validations.
		err := f.CreateHTTPProxy(p)
		require.Error(t, err, "Expected invalid AllowCrossSchemeRedirect to be rejected.")
	})

	Specify("invalid redirect code", func() {
		t := f.T()

		p := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "invalid-redirect-code",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "example.com",
				},
				Routes: []contour_v1.Route{{
					Services: []contour_v1.Service{{
						Name: "ingress-conformance-echo",
						Port: 80,
					}},
					InternalRedirectPolicy: &contour_v1.HTTPInternalRedirectPolicy{
						RedirectResponseCodes: []contour_v1.RedirectResponseCode{301, 310},
					},
				}},
			},
		}

		// Creation should fail the kubebuilder CRD validations.
		err := f.CreateHTTPProxy(p)
		require.Error(t, err, "Expected invalid RedirectResponseCodes to be rejected.")
	})
}

func testInternalRedirectPolicy(namespace string) {
	Specify("internal redirect policy", func() {
		t := f.T()

		proxy := getInternalRedirectHTTPProxy(namespace)

		doInternalRedirectTest(namespace, proxy, t)
	})
}

func doInternalRedirectTest(namespace string, proxy *contour_v1.HTTPProxy, t GinkgoTInterface) {
	f.Fixtures.Echo.Deploy(namespace, "echo")

	envoyService := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: namespace,
			Name:      "envoy-service",
		},
		Spec: core_v1.ServiceSpec{
			Type:         core_v1.ServiceTypeExternalName,
			ExternalName: f.Deployment.EnvoyService.ObjectMeta.Name + "." + f.Deployment.EnvoyService.ObjectMeta.Namespace,
			Ports: []core_v1.ServicePort{
				{
					Name: "http",
					Port: 80,
				},
			},
		},
	}
	require.NoError(t, f.Client.Create(context.TODO(), envoyService))

	require.True(f.T(), f.CreateHTTPProxyAndWaitFor(proxy, e2e.HTTPProxyValid))

	// /redirect ensure the redirect works as expected.
	assertInternalRedirectRequest(t, proxy.Spec.VirtualHost.Fqdn, "/redirect",
		"http://internalredirectpolicy.projectcontour.io/echo", 302)

	// /internal-redirect-301 check if status code properly handled
	assertInternalRedirectRequest(t, proxy.Spec.VirtualHost.Fqdn, "/internal-redirect-301",
		"http://internalredirectpolicy.projectcontour.io/echo", 302)

	// /internal-redirect generates a redirect that is handled by the proxy.
	res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
		Host: proxy.Spec.VirtualHost.Fqdn,
		Path: "/internal-redirect",
		ClientOpts: []func(*http.Client){
			e2e.OptDontFollowRedirects,
		},
		Condition: e2e.HasStatusCode(200),
	})
	require.NotNil(t, res, "request never succeeded")
	require.Truef(t, ok, "expected %d response code, got %d", 200, res.StatusCode)
}

func assertInternalRedirectRequest(t GinkgoTInterface, fqdn, path, expectedLocation string, expectedStatusCode int) {
	res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
		Host: fqdn,
		Path: path,
		ClientOpts: []func(*http.Client){
			e2e.OptDontFollowRedirects,
		},
		Condition: e2e.HasStatusCode(expectedStatusCode),
	})
	require.NotNil(t, res, "request never succeeded")
	require.Truef(t, ok, "expected %d response code, got %d", expectedStatusCode, res.StatusCode)
	assert.Equal(t, expectedLocation, res.Headers.Get("Location"))
}

func getInternalRedirectHTTPProxy(namespace string) *contour_v1.HTTPProxy {
	fqdn := "internalredirectpolicy.projectcontour.io"
	proxy := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "internal-redirect",
			Namespace: namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: fqdn,
			},

			Routes: []contour_v1.Route{
				// Simple route that forward request to echo service
				{
					Conditions: []contour_v1.MatchCondition{{
						Prefix: "/echo",
					}},
					Services: []contour_v1.Service{{
						Name: "echo",
						Port: 80,
					}},
				},
				// Route that returns a 302 redirect to the /echo route
				{
					Conditions: []contour_v1.MatchCondition{{
						Prefix: "/redirect",
					}},
					Services: []contour_v1.Service{},
					RequestRedirectPolicy: &contour_v1.HTTPRequestRedirectPolicy{
						Hostname:   ptr.To(fqdn),
						StatusCode: ptr.To(contour_v1.RedirectResponseCode(302)),
						Path:       ptr.To("/echo"),
					},
				},
				{
					Conditions: []contour_v1.MatchCondition{{
						Prefix: "/internal-redirect",
					}},
					Services: []contour_v1.Service{{
						Name: "envoy-service",
						Port: 80,
					}},
					PathRewritePolicy: &contour_v1.PathRewritePolicy{
						ReplacePrefix: []contour_v1.ReplacePrefix{
							{
								Prefix:      "/internal-redirect",
								Replacement: "/redirect",
							},
						},
					},
					InternalRedirectPolicy: &contour_v1.HTTPInternalRedirectPolicy{},
				},
				{
					Conditions: []contour_v1.MatchCondition{{
						Prefix: "/internal-redirect-301",
					}},
					Services: []contour_v1.Service{{
						Name: "envoy-service",
						Port: 80,
					}},
					PathRewritePolicy: &contour_v1.PathRewritePolicy{
						ReplacePrefix: []contour_v1.ReplacePrefix{
							{
								Prefix:      "/internal-redirect-301",
								Replacement: "/redirect",
							},
						},
					},
					// only allows 301
					InternalRedirectPolicy: &contour_v1.HTTPInternalRedirectPolicy{
						RedirectResponseCodes: []contour_v1.RedirectResponseCode{301},
					},
				},
			},
		},
	}

	return proxy
}
