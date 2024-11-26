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
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/test/e2e"
)

func testRequestRedirectRuleNoService(namespace string) {
	Specify("redirects can be specified on route rule", func() {
		t := f.T()

		proxy := getRedirectHTTPProxy(namespace, true)

		for _, route := range proxy.Spec.Routes {
			require.Empty(t, route.Services)
		}

		doRedirectTest(namespace, proxy, t)
	})
}

func testRequestRedirectRuleInvalid(namespace string) {
	Specify("invalid policy specified on route rule", func() {
		f.Fixtures.Echo.Deploy(namespace, "echo")
		proxy := getRedirectHTTPProxyInvalid(namespace)

		require.True(f.T(), f.CreateHTTPProxyAndWaitFor(proxy, e2e.HTTPProxyInvalid))
	})
}

func doRedirectTest(namespace string, proxy *contour_v1.HTTPProxy, t GinkgoTInterface) {
	f.Fixtures.Echo.Deploy(namespace, "echo")

	require.True(f.T(), f.CreateHTTPProxyAndWaitFor(proxy, e2e.HTTPProxyValid))

	// /basic-redirect only specifies a host name to redirect to.
	assertRequest(t, proxy.Spec.VirtualHost.Fqdn, "/basic-redirect",
		"http://projectcontour.io/basic-redirect", 302)

	// /complex-redirect specifies a host name, scheme, port and response code for the redirect.
	assertRequest(t, proxy.Spec.VirtualHost.Fqdn, "/complex-redirect",
		"https://envoyproxy.io:8080/complex-redirect", 301)

	// /path-rewrite specifies a path to redirect to.
	assertRequest(t, proxy.Spec.VirtualHost.Fqdn, "/path-rewrite",
		"http://requestredirectrule.projectcontour.io/path", 302)

	// /prefix-rewrite specifies a prefix to redirect to.
	assertRequest(t, proxy.Spec.VirtualHost.Fqdn, "/prefix-rewrite/foo/bar/zed",
		"http://requestredirectrule.projectcontour.io/v2/foo/bar/zed", 302)

	// //prefix-rewrite-trailing-slash specifies a prefix with a trailing slash and a prefix redirect.
	assertRequest(t, proxy.Spec.VirtualHost.Fqdn, "/prefix-rewrite-trailing-slash/foo/bar",
		"http://requestredirectrule.projectcontour.io/v2foo/bar", 302)
}

func assertRequest(t GinkgoTInterface, fqdn, path, expectedLocation string, expectedStatusCode int) {
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

func getRedirectHTTPProxy(namespace string, removeServices bool) *contour_v1.HTTPProxy {
	proxy := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "redirect",
			Namespace: namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "requestredirectrule.projectcontour.io",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/basic-redirect",
				}},
				Services: []contour_v1.Service{{
					Name: "echo",
					Port: 80,
				}},
				RequestRedirectPolicy: &contour_v1.HTTPRequestRedirectPolicy{
					Hostname: ptr.To("projectcontour.io"),
				},
			}, {
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/complex-redirect",
				}},
				Services: []contour_v1.Service{{
					Name: "echo",
					Port: 80,
				}},
				RequestRedirectPolicy: &contour_v1.HTTPRequestRedirectPolicy{
					Scheme:     ptr.To("https"),
					Hostname:   ptr.To("envoyproxy.io"),
					Port:       ptr.To(int32(8080)),
					StatusCode: ptr.To(contour_v1.RedirectResponseCode(301)),
				},
			}, {
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/path-rewrite",
				}},
				Services: []contour_v1.Service{{
					Name: "echo",
					Port: 80,
				}},
				RequestRedirectPolicy: &contour_v1.HTTPRequestRedirectPolicy{
					Path: ptr.To("/path"),
				},
			}, {
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/prefix-rewrite",
				}},
				Services: []contour_v1.Service{{
					Name: "echo",
					Port: 80,
				}},
				RequestRedirectPolicy: &contour_v1.HTTPRequestRedirectPolicy{
					Prefix: ptr.To("/v2"),
				},
			}, {
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/prefix-rewrite-trailing-slash/",
				}},
				Services: []contour_v1.Service{{
					Name: "echo",
					Port: 80,
				}},
				RequestRedirectPolicy: &contour_v1.HTTPRequestRedirectPolicy{
					Prefix: ptr.To("/v2"),
				},
			}},
		},
	}

	if removeServices {
		// Remove the services from the proxy.
		for i := range proxy.Spec.Routes {
			proxy.Spec.Routes[i].Services = []contour_v1.Service{}
		}
	}

	return proxy
}

func getRedirectHTTPProxyInvalid(namespace string) *contour_v1.HTTPProxy {
	proxy := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "invalid",
			Namespace: namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "requestredirectrule.projectcontour.io",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/basic-redirect",
				}},
				Services: []contour_v1.Service{{
					Name: "echo",
					Port: 80,
				}},
				RequestRedirectPolicy: &contour_v1.HTTPRequestRedirectPolicy{
					Path:   ptr.To("/path"),
					Prefix: ptr.To("/path"),
				},
			}},
		},
	}

	return proxy
}
