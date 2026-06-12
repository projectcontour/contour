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
	"fmt"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/stretchr/testify/require"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/pkg/config"
	"github.com/projectcontour/contour/test/e2e"
)

// testExternalAuthzHTTP tests per-virtualhost HTTP external authorization.
// Registered in httpproxy_test.go via f.NamespacedTest.
func testExternalAuthzHTTP(namespace string) {
	Specify("external HTTP auth can be configured on an HTTPProxy", func() {
		f.Fixtures.Echo.Deploy(namespace, "echo")
		f.Certs.CreateSelfSignedCert(namespace, "echo", "externalauthhttp.projectcontour.io")

		setHandler := e2e.StartLocalHTTPService(GinkgoT(), f.Client, namespace, "http-auth-mock")

		extSvc := &contour_v1alpha1.ExtensionService{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      "http-auth-mock",
				Namespace: namespace,
			},
			Spec: contour_v1alpha1.ExtensionServiceSpec{
				Protocol: ptr.To("http/1.1"),
				Services: []contour_v1alpha1.ExtensionServiceTarget{
					{Name: "http-auth-mock", Port: 80},
				},
			},
		}
		Expect(f.Client.Create(context.TODO(), extSvc)).To(Succeed())

		p := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "external-auth-http",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "externalauthhttp.projectcontour.io",
					TLS: &contour_v1.TLS{
						SecretName: "echo",
					},
					Authorization: &contour_v1.AuthorizationServer{
						ResponseTimeout: "5s",
						ServiceType:     contour_v1.AuthorizationHTTPService,
						ExtensionServiceRef: contour_v1.ExtensionServiceReference{
							Name:      extSvc.Name,
							Namespace: extSvc.Namespace,
						},
						HTTPServerSettings: &contour_v1.HTTPAuthorizationServerSettings{
							PathPrefix: "/auth",
							AllowedAuthorizationHeaders: []contour_v1.HTTPAuthorizationServerAllowedHeaders{
								{Exact: "x-token"},
								{Prefix: "x-app-"},
							},
							AllowedUpstreamHeaders: []contour_v1.HTTPAuthorizationServerAllowedHeaders{
								{Exact: "x-auth-user"},
								{Prefix: "x-got-"},
							},
						},
					},
				},
				Routes: []contour_v1.Route{
					{
						Conditions: []contour_v1.MatchCondition{{Prefix: "/"}},
						Services:   []contour_v1.Service{{Name: "echo", Port: 80}},
					},
					{
						Conditions: []contour_v1.MatchCondition{{Prefix: "/open"}},
						AuthPolicy: &contour_v1.AuthorizationPolicy{Disabled: true},
						Services:   []contour_v1.Service{{Name: "echo", Port: 80}},
					},
				},
			},
		}
		Expect(f.CreateHTTPProxyAndWaitFor(p, e2e.HTTPProxyValid)).To(BeTrue())

		By("auth server rejects → 401")
		setHandler(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		})
		res, ok := f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Path:      "/test",
			Condition: e2e.HasStatusCode(401),
		})
		Expect(res).NotTo(BeNil(), "request never succeeded")
		Expect(ok).To(BeTrue(), "expected 401, got %d", res.StatusCode)

		By("auth server approves → 200")
		setHandler(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		res, ok = f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Path:      "/test",
			Condition: e2e.HasStatusCode(200),
		})
		Expect(res).NotTo(BeNil(), "request never succeeded")
		Expect(ok).To(BeTrue(), "expected 200, got %d", res.StatusCode)

		By("PathPrefix is prepended to auth request path")
		var capturedPath string
		setHandler(func(w http.ResponseWriter, r *http.Request) {
			capturedPath = r.URL.Path
			w.WriteHeader(http.StatusOK)
		})
		res, ok = f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Path:      "/test",
			Condition: e2e.HasStatusCode(200),
		})
		Expect(res).NotTo(BeNil(), "request never succeeded")
		Expect(ok).To(BeTrue(), "expected 200, got %d", res.StatusCode)
		Expect(capturedPath).To(HavePrefix("/auth/"), "auth server should receive path with configured prefix")

		By("AllowedAuthorizationHeaders are forwarded to auth server; others are filtered")
		setHandler(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("x-got-exact", r.Header.Get("x-token"))
			w.Header().Set("x-got-prefix", r.Header.Get("x-app-id"))
			w.Header().Set("x-got-blocked", r.Header.Get("x-secret"))
			w.Header().Set("x-auth-user", "jane")
			w.Header().Set("x-not-allowed", "dropped")
			w.WriteHeader(http.StatusOK)
		})
		res, ok = f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
			Host: p.Spec.VirtualHost.Fqdn,
			Path: "/test",
			RequestOpts: []func(*http.Request){
				e2e.OptSetHeaders(map[string]string{
					"x-token":  "abc",
					"x-app-id": "123",
					"x-secret": "should-be-blocked",
				}),
			},
			Condition: e2e.HasStatusCode(200),
		})
		Expect(res).NotTo(BeNil(), "request never succeeded")
		Expect(ok).To(BeTrue(), "expected 200, got %d", res.StatusCode)

		body := f.GetEchoResponseBody(res.Body)
		Expect(body.RequestHeaders.Get("x-got-exact")).To(Equal("abc"), "exact-matched header should reach auth server")
		Expect(body.RequestHeaders.Get("x-got-prefix")).To(Equal("123"), "prefix-matched header should reach auth server")
		Expect(body.RequestHeaders.Get("x-got-blocked")).To(BeEmpty(), "unallowed header should not reach auth server")

		By("AllowedUpstreamHeaders from auth response reach backend; others are filtered")
		Expect(body.RequestHeaders.Get("x-auth-user")).To(Equal("jane"), "allowed upstream header should reach backend")
		Expect(body.RequestHeaders.Get("x-not-allowed")).To(BeEmpty(), "disallowed upstream header should not reach backend")

		By("AuthPolicy.Disabled bypasses auth")
		setHandler(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		})
		res, ok = f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Path:      "/open",
			Condition: e2e.HasStatusCode(200),
		})
		Expect(res).NotTo(BeNil(), "request never succeeded")
		Expect(ok).To(BeTrue(), "expected 200 (auth bypassed), got %d", res.StatusCode)
	})
}

// testGlobalExternalAuthzHTTP tests global HTTP external authorization.
// This requires restarting Contour with global ext auth config, so it uses
// its own Describe block with a dedicated Contour lifecycle.
var _ = Describe("httpproxy-ext-auth-http-global", func() {
	var (
		contourCmd           *gexec.Session
		contourConfig        *config.Parameters
		contourConfiguration *contour_v1alpha1.ContourConfiguration
		contourConfigFile    string
	)

	BeforeEach(func() {
		contourConfig = e2e.DefaultContourConfigFileParams()
		contourConfiguration = e2e.DefaultContourConfiguration()
	})

	JustBeforeEach(func() {
		var err error
		contourCmd, contourConfigFile, err = f.Deployment.StartLocalContour(contourConfig, contourConfiguration)
		require.NoError(f.T(), err)
		require.NoError(f.T(), f.Deployment.WaitForEnvoyUpdated())
	})

	AfterEach(func() {
		require.NoError(f.T(), f.Deployment.StopLocalContour(contourCmd, contourConfigFile))
	})

	f.NamespacedTest("global-ext-auth-http", func(namespace string) {
		var setHandler func(http.HandlerFunc)

		BeforeEach(func() {
			setHandler = e2e.StartLocalHTTPService(GinkgoT(), f.Client, namespace, "http-auth-mock")

			extSvc := &contour_v1alpha1.ExtensionService{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "http-auth-mock",
					Namespace: namespace,
				},
				Spec: contour_v1alpha1.ExtensionServiceSpec{
					Protocol: ptr.To("http/1.1"),
					Services: []contour_v1alpha1.ExtensionServiceTarget{
						{Name: "http-auth-mock", Port: 80},
					},
				},
			}
			require.NoError(f.T(), f.Client.Create(context.TODO(), extSvc))

			contourConfig.GlobalExternalAuthorization = config.GlobalExternalAuthorization{
				ExtensionService: fmt.Sprintf("%s/%s", namespace, "http-auth-mock"),
				ServiceType:      contour_v1.AuthorizationHTTPService,
				FailOpen:         false,
				ResponseTimeout:  "10s",
				HTTPServerSettings: &contour_v1.HTTPAuthorizationServerSettings{
					PathPrefix: "/auth",
				},
			}
			contourConfiguration.Spec.GlobalExternalAuthorization = &contour_v1.AuthorizationServer{
				ServiceType: contour_v1.AuthorizationHTTPService,
				ExtensionServiceRef: contour_v1.ExtensionServiceReference{
					Namespace: namespace,
					Name:      "http-auth-mock",
				},
				FailOpen:        false,
				ResponseTimeout: "10s",
				HTTPServerSettings: &contour_v1.HTTPAuthorizationServerSettings{
					PathPrefix: "/auth",
				},
			}
		})

		Specify("global HTTP ext auth applies to virtual hosts", func() {
			f.Fixtures.Echo.Deploy(namespace, "echo")
			f.Certs.CreateSelfSignedCert(namespace, "echo", "globalexternalauthhttp.projectcontour.io")

			p := &contour_v1.HTTPProxy{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: namespace,
					Name:      "global-ext-auth-http",
				},
				Spec: contour_v1.HTTPProxySpec{
					VirtualHost: &contour_v1.VirtualHost{
						Fqdn: "globalexternalauthhttp.projectcontour.io",
						TLS:  &contour_v1.TLS{SecretName: "echo"},
					},
					Routes: []contour_v1.Route{
						{
							Conditions: []contour_v1.MatchCondition{{Prefix: "/"}},
							Services:   []contour_v1.Service{{Name: "echo", Port: 80}},
						},
						{
							Conditions: []contour_v1.MatchCondition{{Prefix: "/open"}},
							AuthPolicy: &contour_v1.AuthorizationPolicy{Disabled: true},
							Services:   []contour_v1.Service{{Name: "echo", Port: 80}},
						},
					},
				},
			}
			Expect(f.CreateHTTPProxyAndWaitFor(p, e2e.HTTPProxyValid)).To(BeTrue())

			By("auth server rejects → 401")
			setHandler(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
			})
			res, ok := f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
				Host:      p.Spec.VirtualHost.Fqdn,
				Path:      "/test",
				Condition: e2e.HasStatusCode(401),
			})
			Expect(res).NotTo(BeNil(), "request never succeeded")
			Expect(ok).To(BeTrue(), "expected 401, got %d", res.StatusCode)

			By("auth server approves → 200")
			setHandler(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			})
			res, ok = f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
				Host:      p.Spec.VirtualHost.Fqdn,
				Path:      "/test",
				Condition: e2e.HasStatusCode(200),
			})
			Expect(res).NotTo(BeNil(), "request never succeeded")
			Expect(ok).To(BeTrue(), "expected 200, got %d", res.StatusCode)

			By("AuthPolicy.Disabled bypasses global auth")
			setHandler(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
			})
			res, ok = f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
				Host:      p.Spec.VirtualHost.Fqdn,
				Path:      "/open",
				Condition: e2e.HasStatusCode(200),
			})
			Expect(res).NotTo(BeNil(), "request never succeeded")
			Expect(ok).To(BeTrue(), "expected 200 (auth bypassed), got %d", res.StatusCode)
		})
	})
})
