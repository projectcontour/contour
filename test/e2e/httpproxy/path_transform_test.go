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
	"strings"

	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/require"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/test/e2e"
)

// twoPrefixRouteProxy returns an HTTPProxy with a specific prefix route to
// echo-1 and a catch-all route to echo-2. Which service answers a request
// therefore reveals the path Envoy used for route matching.
func twoPrefixRouteProxy(namespace, fqdn, prefix string) *contour_v1.HTTPProxy {
	return &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: namespace,
			Name:      "echo",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: fqdn,
			},
			Routes: []contour_v1.Route{
				{
					Services: []contour_v1.Service{{
						Name: "echo-1",
						Port: 80,
					}},
					Conditions: []contour_v1.MatchCondition{{
						Prefix: prefix,
					}},
				},
				{
					Services: []contour_v1.Service{{
						Name: "echo-2",
						Port: 80,
					}},
					Conditions: []contour_v1.MatchCondition{{
						Prefix: "/",
					}},
				},
			},
		},
	}
}

func testDisableNormalizePath(disableNormalizePath bool) e2e.NamespacedTestBody {
	var testName, fqdn, wantService string
	if disableNormalizePath {
		testName = "when disable normalize path is true, dot segments in requests are not collapsed"
		fqdn = "disable.normalizepath.projectcontour.io"
		// The un-normalized path "/foo/../bar" does not match the "/bar"
		// prefix, so the request falls through to the catch-all route.
		wantService = "echo-2"
	} else {
		testName = "when disable normalize path is false, dot segments in requests are collapsed"
		fqdn = "enable.normalizepath.projectcontour.io"
		// "/foo/../bar" is normalized to "/bar" before route matching.
		wantService = "echo-1"
	}

	return func(namespace string) {
		Specify(testName, func() {
			t := f.T()

			f.Fixtures.Echo.Deploy(namespace, "echo-1")
			f.Fixtures.Echo.Deploy(namespace, "echo-2")

			p := twoPrefixRouteProxy(namespace, fqdn, "/bar")
			require.True(t, f.CreateHTTPProxyAndWaitFor(p, e2e.HTTPProxyValid))

			// Sanity check: "/bar" always reaches echo-1 regardless of the setting.
			// This also confirms the proxy is programmed before the real assertion.
			requireServiceForPath(t, p.Spec.VirtualHost.Fqdn, "/bar", "echo-1")

			requireServiceForPath(t, p.Spec.VirtualHost.Fqdn, "/foo/../bar", wantService)
		})
	}
}

func testPathWithEscapedSlashesAction(action contour_v1alpha1.PathWithEscapedSlashesActionType) e2e.NamespacedTestBody {
	return func(namespace string) {
		Specify("escaped slashes are handled according to "+string(action), func() {
			t := f.T()

			f.Fixtures.Echo.Deploy(namespace, "echo-1")
			f.Fixtures.Echo.Deploy(namespace, "echo-2")

			fqdn := strings.ReplaceAll(string(action), "_", "-") + ".escapedslashes.projectcontour.io"
			p := twoPrefixRouteProxy(namespace, fqdn, "/foo/")
			require.True(t, f.CreateHTTPProxyAndWaitFor(p, e2e.HTTPProxyValid))

			// Sanity check: an unescaped "/foo/bar" always reaches echo-1.
			requireServiceForPath(t, fqdn, "/foo/bar", "echo-1")

			const escapedPath = "/foo%2Fbar"

			switch action {
			case contour_v1alpha1.KeepUnchangedPathWithEscapedSlashes:
				// The escaped slash is left alone, so "/foo%2Fbar" does not
				// match the "/foo/" prefix and falls through to the catch-all.
				requireServiceForPath(t, fqdn, escapedPath, "echo-2")

			case contour_v1alpha1.UnescapeAndForwardPathWithEscapedSlashes:
				// The path is unescaped to "/foo/bar" before route matching.
				requireServiceForPath(t, fqdn, escapedPath, "echo-1")

			case contour_v1alpha1.RejectRequestPathWithEscapedSlashes:
				res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
					Host:      fqdn,
					Path:      escapedPath,
					Condition: e2e.HasStatusCode(http.StatusBadRequest),
				})
				require.NotNil(t, res, "request never succeeded")
				require.Truef(t, ok, "expected 400 response code, got %d", res.StatusCode)

			case contour_v1alpha1.UnescapeAndRedirectPathWithEscapedSlashes:
				res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
					Host:       fqdn,
					Path:       escapedPath,
					ClientOpts: []func(*http.Client){e2e.OptDontFollowRedirects},
					Condition:  e2e.HasStatusCode(http.StatusMovedPermanently),
				})
				require.NotNil(t, res, "request never succeeded")
				require.Truef(t, ok, "expected 301 response code, got %d", res.StatusCode)
				require.Equal(t, "/foo/bar", res.Headers.Get("Location"))
			}
		})
	}
}

// requireServiceForPath asserts that a request for the given path is answered
// with a 200 by the named echo service.
func requireServiceForPath(t require.TestingT, fqdn, path, service string) {
	res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
		Host: fqdn,
		Path: path,
		Condition: func(res *e2e.HTTPResponse) bool {
			if !e2e.HasStatusCode(http.StatusOK)(res) {
				return false
			}
			return f.GetEchoResponseBody(res.Body).Service == service
		},
	})
	require.NotNil(t, res, "request for %q never succeeded", path)
	require.Truef(t, ok, "request for %q: got status %d from service %q, want 200 from %q",
		path, res.StatusCode, f.GetEchoResponseBody(res.Body).Service, service)
}
