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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/test/e2e"
)

func testExternalAuth(namespace string) {
	Specify("external auth can be configured on an HTTPProxy", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "echo")
		f.Certs.CreateSelfSignedCert(namespace, "echo", "externalauth.projectcontour.io")

		auth := e2e.StartLocalGRPCAuthService(GinkgoT(), f.Client, namespace, "testserver")

		extSvc := &contour_v1alpha1.ExtensionService{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      "testserver",
				Namespace: namespace,
			},
			Spec: contour_v1alpha1.ExtensionServiceSpec{
				Protocol: ptr.To("h2c"),
				Services: []contour_v1alpha1.ExtensionServiceTarget{
					{
						Name: "testserver",
						Port: 9443,
					},
				},
			},
		}
		require.NoError(t, f.Client.Create(context.TODO(), extSvc))

		p := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "external-auth",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "externalauth.projectcontour.io",
					TLS: &contour_v1.TLS{
						SecretName: "echo",
					},
					Authorization: &contour_v1.AuthorizationServer{
						ResponseTimeout: "500ms",
						ExtensionServiceRef: contour_v1.ExtensionServiceReference{
							Name:      extSvc.Name,
							Namespace: extSvc.Namespace,
						},
						AuthPolicy: &contour_v1.AuthorizationPolicy{
							Context: map[string]string{
								"hostname": "externalauth.projectcontour.io",
							},
						},
					},
				},
				Routes: []contour_v1.Route{
					{
						Conditions: []contour_v1.MatchCondition{
							{
								Prefix: "/first",
							},
						},
						AuthPolicy: &contour_v1.AuthorizationPolicy{
							Context: map[string]string{
								"target": "first",
							},
						},
						Services: []contour_v1.Service{
							{
								Name: "echo",
								Port: 80,
							},
						},
					},

					{
						Conditions: []contour_v1.MatchCondition{
							{
								Prefix: "/second",
							},
						},
						AuthPolicy: &contour_v1.AuthorizationPolicy{
							Disabled: true,
						},
						Services: []contour_v1.Service{
							{
								Name: "echo",
								Port: 80,
							},
						},
					},
					{
						Conditions: []contour_v1.MatchCondition{
							{Prefix: "/direct-response-auth-enabled"},
						},
						DirectResponsePolicy: &contour_v1.HTTPDirectResponsePolicy{
							StatusCode: http.StatusTeapot,
						},
					},
					{
						Conditions: []contour_v1.MatchCondition{
							{Prefix: "/direct-response-auth-disabled"},
						},
						DirectResponsePolicy: &contour_v1.HTTPDirectResponsePolicy{
							StatusCode: http.StatusTeapot,
						},
						AuthPolicy: &contour_v1.AuthorizationPolicy{
							Disabled: true,
						},
					},

					{
						AuthPolicy: &contour_v1.AuthorizationPolicy{
							Context: map[string]string{
								"target": "default",
							},
						},
						Services: []contour_v1.Service{
							{
								Name: "echo",
								Port: 80,
							},
						},
					},
				},
			},
		}
		require.True(f.T(), f.CreateHTTPProxyAndWaitFor(p, e2e.HTTPProxyValid))

		// By default requests to /first should not be authorized.
		By("auth server denies request")
		auth.Deny(http.StatusUnauthorized)
		res, ok := f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Path:      "/first",
			Condition: e2e.HasStatusCode(401),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 401 response code, got %d", res.StatusCode)

		By("auth server allows request, context_extensions forwarded to auth service")
		auth.Handle(func(req *envoy_service_auth_v3.CheckRequest) *envoy_service_auth_v3.CheckResponse {
			assert.Equal(t, "first", req.Attributes.ContextExtensions["target"])
			assert.Equal(t, "externalauth.projectcontour.io", req.Attributes.ContextExtensions["hostname"])
			return e2e.AllowResponse().Build()
		})
		res, ok = f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Path:      "/first/allow",
			Condition: e2e.HasStatusCode(200),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)

		By("route with AuthPolicy.Disabled=true bypasses auth")
		auth.Deny(http.StatusUnauthorized)
		res, ok = f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Path:      "/second",
			Condition: e2e.HasStatusCode(200),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)

		// The default route should not authorize by default.
		res, ok = f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Path:      "/matches-default-route",
			Condition: e2e.HasStatusCode(401),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 401 response code, got %d", res.StatusCode)

		By("context_extensions forwarded for the default route")
		auth.Handle(func(req *envoy_service_auth_v3.CheckRequest) *envoy_service_auth_v3.CheckResponse {
			assert.Equal(t, "default", req.Attributes.ContextExtensions["target"])
			assert.Equal(t, "externalauth.projectcontour.io", req.Attributes.ContextExtensions["hostname"])
			return e2e.AllowResponse().Build()
		})
		res, ok = f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Path:      "/matches-default-route/allow",
			Condition: e2e.HasStatusCode(200),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)

		By("direct-response route with auth enabled is subject to auth")
		auth.Deny(http.StatusUnauthorized)
		res, ok = f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Path:      "/direct-response-auth-enabled",
			Condition: e2e.HasStatusCode(401),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 401 response code, got %d", res.StatusCode)

		By("direct-response route with auth enabled returns configured status on allow")
		auth.Allow()
		res, ok = f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Path:      "/direct-response-auth-enabled/allow",
			Condition: e2e.HasStatusCode(http.StatusTeapot),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 418 response code, got %d", res.StatusCode)

		By("direct-response route with auth disabled bypasses auth")
		auth.Deny(http.StatusUnauthorized)
		res, ok = f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Path:      "/direct-response-auth-disabled",
			Condition: e2e.HasStatusCode(http.StatusTeapot),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 418 response code, got %d", res.StatusCode)

		// Create a Service with no endpoints so Envoy has nothing to connect to,
		// simulating an unreachable auth server for FailOpen/FailClosed tests.
		require.NoError(t, f.Client.Create(context.TODO(), &core_v1.Service{
			ObjectMeta: meta_v1.ObjectMeta{Name: "unreachable-authserver", Namespace: namespace},
			Spec: core_v1.ServiceSpec{
				Ports: []core_v1.ServicePort{{Name: "grpc", Protocol: core_v1.ProtocolTCP, Port: 9443}},
			},
		}))

		unreachableExtSvc := &contour_v1alpha1.ExtensionService{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      "unreachable-authserver",
				Namespace: namespace,
			},
			Spec: contour_v1alpha1.ExtensionServiceSpec{
				Protocol: ptr.To("h2c"),
				Services: []contour_v1alpha1.ExtensionServiceTarget{
					{Name: "unreachable-authserver", Port: 9443},
				},
			},
		}
		require.NoError(t, f.Client.Create(context.TODO(), unreachableExtSvc))

		By("FailOpen=false, unreachable auth server returns 503")
		f.Certs.CreateSelfSignedCert(namespace, "failopen", "failopen.externalauth.projectcontour.io")
		failOpenFalseProxy := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "external-auth-failopen-false",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "failopen.externalauth.projectcontour.io",
					TLS: &contour_v1.TLS{
						SecretName: "failopen",
					},
					Authorization: &contour_v1.AuthorizationServer{
						FailOpen:        false,
						ResponseTimeout: "500ms",
						ExtensionServiceRef: contour_v1.ExtensionServiceReference{
							Name:      unreachableExtSvc.Name,
							Namespace: unreachableExtSvc.Namespace,
						},
					},
				},
				Routes: []contour_v1.Route{
					{
						Services: []contour_v1.Service{
							{Name: "echo", Port: 80},
						},
					},
				},
			},
		}
		require.True(f.T(), f.CreateHTTPProxyAndWaitFor(failOpenFalseProxy, e2e.HTTPProxyValid))

		res, ok = f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
			Host:      failOpenFalseProxy.Spec.VirtualHost.Fqdn,
			Path:      "/test",
			Condition: e2e.HasStatusCode(503),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 503 response code, got %d", res.StatusCode)

		By("FailOpen=true, unreachable auth server allows request through")
		f.Certs.CreateSelfSignedCert(namespace, "failopen-true", "failopen-true.externalauth.projectcontour.io")
		failOpenTrueProxy := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "external-auth-failopen-true",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "failopen-true.externalauth.projectcontour.io",
					TLS: &contour_v1.TLS{
						SecretName: "failopen-true",
					},
					Authorization: &contour_v1.AuthorizationServer{
						FailOpen:        true,
						ResponseTimeout: "500ms",
						ExtensionServiceRef: contour_v1.ExtensionServiceReference{
							Name:      unreachableExtSvc.Name,
							Namespace: unreachableExtSvc.Namespace,
						},
					},
				},
				Routes: []contour_v1.Route{
					{
						Services: []contour_v1.Service{
							{Name: "echo", Port: 80},
						},
					},
				},
			},
		}
		require.True(f.T(), f.CreateHTTPProxyAndWaitFor(failOpenTrueProxy, e2e.HTTPProxyValid))

		res, ok = f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
			Host:      failOpenTrueProxy.Spec.VirtualHost.Fqdn,
			Path:      "/test",
			Condition: e2e.HasStatusCode(200),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)
	})
}
