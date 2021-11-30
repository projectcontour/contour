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
	"net/http"

	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"

	"github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo"
	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
)

func testRequestRedirectRuleNoService(namespace string) {
	Specify("redirects can be specified on route rule", func() {
		t := f.T()

		proxy := getHTTPProxy(namespace, true)

		require.Equal(t, 0, len(proxy.Spec.Routes))

		doTest(namespace, proxy, t)
	})
}

func testRequestRedirectRule(namespace string) {
	Specify("redirects can be specified on route rule", func() {
		t := f.T()

		proxy := getHTTPProxy(namespace, true)
		doTest(namespace, proxy, t)
	})
}

func doTest(namespace string, proxy *contour_api_v1.HTTPProxy, t ginkgo.GinkgoTInterface) {

	f.Fixtures.Echo.Deploy(namespace, "echo")

	f.CreateHTTPProxyAndWaitFor(proxy, e2e.HTTPProxyValid)

	// /basic-redirect only specifies a host name to
	// redirect to.
	res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
		Host: proxy.Spec.VirtualHost.Fqdn,
		Path: "/basic-redirect",
		ClientOpts: []func(*http.Client){
			e2e.OptDontFollowRedirects,
		},
		Condition: e2e.HasStatusCode(302),
	})
	require.NotNil(t, res, "request never succeeded")
	require.Truef(t, ok, "expected 302 response code, got %d", res.StatusCode)
	assert.Equal(t, "http://projectcontour.io/basic-redirect", res.Headers.Get("Location"))

	// /complex-redirect specifies a host name,
	// scheme, port and response code for the
	// redirect.
	res, ok = f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
		Host: proxy.Spec.VirtualHost.Fqdn,
		Path: "/complex-redirect",
		ClientOpts: []func(*http.Client){
			e2e.OptDontFollowRedirects,
		},
		Condition: e2e.HasStatusCode(301),
	})
	require.NotNil(t, res, "request never succeeded")
	require.Truef(t, ok, "expected 301 response code, got %d", res.StatusCode)
	assert.Equal(t, "https://envoyproxy.io:8080/complex-redirect", res.Headers.Get("Location"))
}

func getHTTPProxy(namespace string, removeServices bool) *contour_api_v1.HTTPProxy {

	proxy := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "redirect",
			Namespace: namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "requestredirectrule.projectcontour.io",
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/basic-redirect",
				}},
				Services: []contour_api_v1.Service{{
					Name: "echo",
					Port: 80,
				}},
				RequestRedirectPolicy: &contour_api_v1.HTTPRequestRedirectPolicy{
					Hostname: pointer.StringPtr("projectcontour.io"),
				},
			}, {
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/complex-redirect",
				}},
				Services: []contour_api_v1.Service{{
					Name: "echo",
					Port: 80,
				}},
				RequestRedirectPolicy: &contour_api_v1.HTTPRequestRedirectPolicy{
					Scheme:     pointer.StringPtr("https"),
					Hostname:   pointer.StringPtr("envoyproxy.io"),
					Port:       pointer.Int32Ptr(8080),
					StatusCode: pointer.Int(301),
				},
			}},
		},
	}

	if removeServices {
		// Remove the services from the proxy.
		for _, route := range proxy.Spec.Routes {
			route.Services = []contour_api_v1.Service{}
		}
	}

	return proxy
}
