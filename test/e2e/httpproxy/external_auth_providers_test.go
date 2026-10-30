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
	"net/http"

	envoy_service_auth_v3 "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/test/e2e"
)

func testExternalAuthProviders(namespace string) {
	Specify("routes can use different authorization providers", func() {
		fqdn := "externalauthproviders.projectcontour.io"
		f.Fixtures.Echo.Deploy(namespace, "echo")
		f.Fixtures.Echo.Deploy(namespace, "echo-admin")
		f.Certs.CreateSelfSignedCert(namespace, "echo", fqdn)

		setHTTPHandler := e2e.StartLocalHTTPService(GinkgoT(), f.Client, namespace, "http-authz")
		grpcAuth := e2e.StartLocalGRPCAuthService(GinkgoT(), f.Client, namespace, "grpc-authz")

		// Service without endpoints, so the auth server is unreachable.
		Expect(f.Client.Create(context.TODO(), &core_v1.Service{
			ObjectMeta: meta_v1.ObjectMeta{Name: "unreachable-authz", Namespace: namespace},
			Spec: core_v1.ServiceSpec{
				Ports: []core_v1.ServicePort{{Name: "grpc", Protocol: core_v1.ProtocolTCP, Port: 9443}},
			},
		})).To(Succeed())

		for _, ext := range []struct {
			name, protocol string
			port           int
		}{
			{"http-authz", "http/1.1", 80},
			{"grpc-authz", "h2c", 9443},
			{"unreachable-authz", "h2c", 9443},
		} {
			Expect(f.Client.Create(context.TODO(), &contour_v1alpha1.ExtensionService{
				ObjectMeta: meta_v1.ObjectMeta{Name: ext.name, Namespace: namespace},
				Spec: contour_v1alpha1.ExtensionServiceSpec{
					Protocol: ptr.To(ext.protocol),
					Services: []contour_v1alpha1.ExtensionServiceTarget{{Name: ext.name, Port: ext.port}},
				},
			})).To(Succeed())
		}

		p := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "external-auth-providers",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: fqdn,
					TLS:  &contour_v1.TLS{SecretName: "echo"},
					AuthzProviders: []contour_v1.AuthorizationProvider{
						{
							Name:                "http-authz",
							Default:             true,
							ServiceType:         contour_v1.AuthorizationHTTPService,
							ExtensionServiceRef: contour_v1.ExtensionServiceReference{Name: "http-authz"},
							HTTPServerSettings: &contour_v1.PerRouteHTTPAuthorizationServerSettings{
								PathOverride: "/check",
								HeadersToAdd: map[string]string{
									"cookie":    "%REQ(cookie)%",
									"x-literal": "100%",
								},
								AllowedUpstreamHeaders: []contour_v1.HTTPAuthorizationServerAllowedHeaders{{Exact: "x-role"}},
							},
						},
						{
							Name:                "grpc-authz",
							ExtensionServiceRef: contour_v1.ExtensionServiceReference{Name: "grpc-authz"},
							Context:             map[string]string{"hostname": fqdn},
						},
						{
							Name:                "unreachable-authz",
							ResponseTimeout:     "500ms",
							ExtensionServiceRef: contour_v1.ExtensionServiceReference{Name: "unreachable-authz"},
						},
					},
				},
				Routes: []contour_v1.Route{
					{
						Conditions: []contour_v1.MatchCondition{{Prefix: "/grpc"}},
						AuthPolicy: &contour_v1.RouteAuthorizationPolicy{
							Require: "grpc-authz",
							Context: map[string]string{"target": "grpc"},
						},
						Services: []contour_v1.Service{{Name: "echo", Port: 80}},
					},
					{
						Conditions: []contour_v1.MatchCondition{{Prefix: "/unreachable"}},
						AuthPolicy: &contour_v1.RouteAuthorizationPolicy{Require: "unreachable-authz"},
						Services:   []contour_v1.Service{{Name: "echo", Port: 80}},
					},
					{
						Conditions: []contour_v1.MatchCondition{
							{Prefix: "/"},
							{Header: &contour_v1.HeaderMatchCondition{Name: "x-role", Exact: "admin"}},
						},
						AuthPolicy: &contour_v1.RouteAuthorizationPolicy{Require: "grpc-authz"},
						Services:   []contour_v1.Service{{Name: "echo-admin", Port: 80}},
					},
					{
						Conditions: []contour_v1.MatchCondition{{Prefix: "/open"}},
						AuthPolicy: &contour_v1.RouteAuthorizationPolicy{Disabled: true},
						Services:   []contour_v1.Service{{Name: "echo", Port: 80}},
					},
					{
						Services: []contour_v1.Service{{Name: "echo", Port: 80}},
					},
				},
			},
		}
		Expect(f.CreateHTTPProxyAndWaitFor(p, e2e.HTTPProxyValid)).To(BeTrue())

		expectStatus := func(path string, code int, opts ...func(*http.Request)) *e2e.HTTPResponse {
			res, ok := f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
				Host:        fqdn,
				Path:        path,
				RequestOpts: opts,
				Condition:   e2e.HasStatusCode(code),
			})
			Expect(res).NotTo(BeNil(), "request never succeeded")
			Expect(ok).To(BeTrue(), "expected %d for %s, got %d", code, path, res.StatusCode)
			return res
		}

		By("default provider applies to a route without a policy")
		setHTTPHandler(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		})
		expectStatus("/", 401)

		By("HTTP provider settings are applied to the check request")
		var httpReq *http.Request
		setHTTPHandler(func(w http.ResponseWriter, r *http.Request) {
			httpReq = r
			w.WriteHeader(http.StatusOK)
		})
		expectStatus("/", 200, e2e.OptSetHeaders(map[string]string{"cookie": "session=abc"}))
		Expect(httpReq.URL.Path).To(Equal("/check"))
		Expect(httpReq.Header.Get("cookie")).To(Equal("session=abc"))
		Expect(httpReq.Header.Get("x-literal")).To(Equal("100%"))

		By("route selects the gRPC provider, which gets client headers and merged context")
		setHTTPHandler(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		})
		var grpcReq *envoy_service_auth_v3.CheckRequest
		grpcAuth.Handle(func(req *envoy_service_auth_v3.CheckRequest) *envoy_service_auth_v3.CheckResponse {
			grpcReq = req
			return e2e.AllowResponse().Build()
		})
		expectStatus("/grpc", 200, e2e.OptSetHeaders(map[string]string{"x-tenant": "team-a"}))
		Expect(grpcReq.Attributes.ContextExtensions).To(Equal(map[string]string{"hostname": fqdn, "target": "grpc"}))
		Expect(grpcReq.Attributes.Request.Http.Headers).To(HaveKeyWithValue("x-tenant", "team-a"))

		By("gRPC provider denies")
		grpcAuth.Deny(http.StatusUnauthorized)
		expectStatus("/grpc", 401)

		By("unreachable provider fails closed")
		expectStatus("/unreachable", 403)

		By("route with auth disabled bypasses providers")
		expectStatus("/open", 200)

		By("route matching x-role uses its own provider")
		expectStatus("/", 401, e2e.OptSetHeaders(map[string]string{"x-role": "admin"}))

		By("x-role set by a provider does not change the route")
		setHTTPHandler(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("x-role", "admin")
			w.WriteHeader(http.StatusOK)
		})
		body := f.GetEchoResponseBody(expectStatus("/", 200).Body)
		Expect(body.Service).To(Equal("echo"))
		Expect(body.RequestHeaders.Get("x-role")).To(Equal("admin"))
	})
}

// testGlobalExternalAuthWithProviders runs with a global auth server that denies
// every request.
func testGlobalExternalAuthWithProviders(namespace string) {
	Specify("authorization providers replace global external auth", func() {
		fqdn := "providers.globalexternalauth.projectcontour.io"
		f.Fixtures.Echo.Deploy(namespace, "echo")
		f.Certs.CreateSelfSignedCert(namespace, "echo", fqdn)

		setHTTPHandler := e2e.StartLocalHTTPService(GinkgoT(), f.Client, namespace, "http-authz")
		Expect(f.Client.Create(context.TODO(), &contour_v1alpha1.ExtensionService{
			ObjectMeta: meta_v1.ObjectMeta{Name: "http-authz", Namespace: namespace},
			Spec: contour_v1alpha1.ExtensionServiceSpec{
				Protocol: ptr.To("http/1.1"),
				Services: []contour_v1alpha1.ExtensionServiceTarget{{Name: "http-authz", Port: 80}},
			},
		})).To(Succeed())

		p := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "global-external-auth-providers",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: fqdn,
					TLS:  &contour_v1.TLS{SecretName: "echo"},
					AuthzProviders: []contour_v1.AuthorizationProvider{
						{
							Name:                "http-authz",
							ServiceType:         contour_v1.AuthorizationHTTPService,
							ExtensionServiceRef: contour_v1.ExtensionServiceReference{Name: "http-authz"},
						},
					},
				},
				Routes: []contour_v1.Route{
					{
						Conditions: []contour_v1.MatchCondition{{Prefix: "/http"}},
						AuthPolicy: &contour_v1.RouteAuthorizationPolicy{Require: "http-authz"},
						Services:   []contour_v1.Service{{Name: "echo", Port: 80}},
					},
					{
						Services: []contour_v1.Service{{Name: "echo", Port: 80}},
					},
				},
			},
		}
		Expect(f.CreateHTTPProxyAndWaitFor(p, e2e.HTTPProxyValid)).To(BeTrue())

		expectStatus := func(path string, code int) {
			res, ok := f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
				Host:      fqdn,
				Path:      path,
				Condition: e2e.HasStatusCode(code),
			})
			Expect(res).NotTo(BeNil(), "request never succeeded")
			Expect(ok).To(BeTrue(), "expected %d for %s, got %d", code, path, res.StatusCode)
		}

		By("route using a provider is checked by the provider")
		setHTTPHandler(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		})
		expectStatus("/http", 403)

		By("provider allows the request even though global auth denies all")
		setHTTPHandler(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		expectStatus("/http", 200)

		By("route without a provider and no default provider has auth disabled")
		expectStatus("/", 200)
	})
}
