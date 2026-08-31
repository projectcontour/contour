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
	var testName, fqdn, wantPath string
	if disableNormalizePath {
		testName = "when disable normalize path is true, dot segments in requests are not collapsed"
		fqdn = "disable.normalizepath.projectcontour.io"
		wantPath = "/foo/../bar"
	} else {
		testName = "when disable normalize path is false, dot segments in requests are collapsed"
		fqdn = "enable.normalizepath.projectcontour.io"
		wantPath = "/bar"
	}

	return func(namespace string) {
		Specify(testName, func() {
			t := f.T()

			f.Fixtures.Echo.Deploy(namespace, "echo-1")

			// A single catch-all route, so the request always reaches the
			// backend regardless of the setting. What varies is the ":path"
			// the backend observes, which is precisely what normalize_path
			// controls: "This affects the upstream :path header as well."
			p := &contour_v1.HTTPProxy{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: namespace,
					Name:      "echo",
				},
				Spec: contour_v1.HTTPProxySpec{
					VirtualHost: &contour_v1.VirtualHost{
						Fqdn: fqdn,
					},
					Routes: []contour_v1.Route{{
						Services: []contour_v1.Service{{
							Name: "echo-1",
							Port: 80,
						}},
						Conditions: []contour_v1.MatchCondition{{
							Prefix: "/",
						}},
					}},
				},
			}
			require.True(t, f.CreateHTTPProxyAndWaitFor(p, e2e.HTTPProxyValid))

			// Sanity check: a path with no dot segments passes through
			// untouched. Also confirms the proxy is programmed.
			requirePathSeenByUpstream(t, fqdn, "/bar", "/bar")

			// Report what Envoy actually has configured, so a failure here is
			// self-diagnosing: it distinguishes "the setting never reached
			// Envoy" from "Envoy did not honor the setting".
			logEnvoyNormalizePathConfig(t)

			requirePathSeenByUpstream(t, fqdn, "/foo/../bar", wantPath)
		})
	}
}

// requirePathSeenByUpstream asserts that the echo backend received the given
// ":path". The echo fixture reflects the request path back in its response body.
func requirePathSeenByUpstream(t require.TestingT, fqdn, requestPath, wantPath string) {
	var gotPath string
	res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
		Host: fqdn,
		Path: requestPath,
		Condition: func(res *e2e.HTTPResponse) bool {
			if !e2e.HasStatusCode(http.StatusOK)(res) {
				return false
			}
			gotPath = f.GetEchoResponseBody(res.Body).Path
			return gotPath == wantPath
		},
	})
	require.NotNil(t, res, "request for %q never succeeded", requestPath)
	require.Truef(t, ok,
		"requested %q: upstream saw path %q, want %q (status %d)",
		requestPath, gotPath, wantPath, res.StatusCode)
}

// logEnvoyNormalizePathConfig dumps how many of Envoy's HTTP connection
// managers have normalize_path true vs false, straight from Envoy's own
// /config_dump. Best effort: the admin listener is not part of what this spec
// is asserting, so an unreachable admin endpoint is logged, not fatal.
//
// Contour's stats/admin listener always hardcodes normalize_path: true, so at
// least one "true" is expected even when the user setting is applied.
func logEnvoyNormalizePathConfig(t require.TestingT) {
	logf, ok := t.(interface{ Logf(string, ...any) })
	if !ok {
		return
	}

	res, reqOK := f.HTTP.AdminRequestUntil(&e2e.HTTPRequestOpts{
		Path:      "/config_dump",
		Condition: e2e.HasStatusCode(http.StatusOK),
	})
	if !reqOK || res == nil {
		logf.Logf("DIAGNOSTIC: could not read Envoy /config_dump (admin listener unreachable from this suite)")
		return
	}

	// Strip whitespace so the match is independent of JSON indentation.
	dump := strings.Join(strings.Fields(string(res.Body)), "")
	logf.Logf("DIAGNOSTIC: Envoy /config_dump has %d occurrences of normalizePath:true, %d of normalizePath:false",
		strings.Count(dump, `"normalizePath":true`),
		strings.Count(dump, `"normalizePath":false`))
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
				// Envoy answers with 307 and a path-only Location header. See
				// NormalizePathAction::Redirect handling in Envoy's
				// conn_manager_impl.cc, which uses Code::TemporaryRedirect.
				res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
					Host:       fqdn,
					Path:       escapedPath,
					ClientOpts: []func(*http.Client){e2e.OptDontFollowRedirects},
					Condition:  e2e.HasStatusCode(http.StatusTemporaryRedirect),
				})
				require.NotNil(t, res, "request never succeeded")
				require.Truef(t, ok, "expected 307 response code, got %d", res.StatusCode)
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
