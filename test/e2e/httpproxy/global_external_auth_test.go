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
	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/require"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/test/e2e"
)

func testGlobalExternalAuthVirtualHostNonTLS(namespace string) {
	Specify("global external auth can be configured on a non TLS HTTPProxy", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "echo")

		p := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "external-auth",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "http.globalexternalauth.projectcontour.io",
				},
				Routes: []contour_v1.Route{
					{
						Conditions: []contour_v1.MatchCondition{
							{
								Prefix: "/first",
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
		res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Path:      "/first",
			Condition: e2e.HasStatusCode(401),
		})

		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 401 response code, got %d", res.StatusCode)

		// THe /second route disables authorization so this request should succeed.
		res, ok = f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Path:      "/second",
			Condition: e2e.HasStatusCode(200),
		})

		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)

		// The default route should not authorize by default.
		res, ok = f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Path:      "/matches-default-route",
			Condition: e2e.HasStatusCode(401),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 401 response code, got %d", res.StatusCode)
	})
}

func testGlobalExternalAuthTLS(namespace string) {
	Specify("global external auth can be configured on a TLS HTTPProxy", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "echo")
		f.Certs.CreateSelfSignedCert(namespace, "echo", "echo", "https.globalexternalauth.projectcontour.io")

		p := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "external-auth",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "https.globalexternalauth.projectcontour.io",
					TLS: &contour_v1.TLS{
						SecretName: "echo",
					},
				},
				Routes: []contour_v1.Route{
					{
						Conditions: []contour_v1.MatchCondition{
							{
								Prefix: "/first",
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
		res, ok := f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Path:      "/first",
			Condition: e2e.HasStatusCode(401),
		})

		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 401 response code, got %d", res.StatusCode)

		// THe /second route disables authorization so this request should succeed.
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
	})
}

func testGlobalExternalAuthNonTLSAuthDisabled(namespace string) {
	Specify("global external auth can be disabled on a non TLS HTTPProxy", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "echo")

		p := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "external-auth",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "disabled.http.globalexternalauth.projectcontour.io",
					Authorization: &contour_v1.AuthorizationServer{
						AuthPolicy: &contour_v1.AuthorizationPolicy{
							Disabled: true,
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
						Services: []contour_v1.Service{
							{
								Name: "echo",
								Port: 80,
							},
						},
					},
					{
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

		res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Path:      "/first",
			Condition: e2e.HasStatusCode(200),
		})

		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)

		res, ok = f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Path:      "/second",
			Condition: e2e.HasStatusCode(200),
		})

		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)

		res, ok = f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Path:      "/matches-default-route",
			Condition: e2e.HasStatusCode(200),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)
	})
}

func testGlobalExternalAuthTLSAuthDisabled(namespace string) {
	Specify("global external auth can be disabled on a TLS HTTPProxy", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "echo")
		f.Certs.CreateSelfSignedCert(namespace, "echo", "echo", "disabled.https.globalexternalauth.projectcontour.io")

		p := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "external-auth",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "disabled.https.globalexternalauth.projectcontour.io",
					TLS: &contour_v1.TLS{
						SecretName: "echo",
					},
					Authorization: &contour_v1.AuthorizationServer{
						AuthPolicy: &contour_v1.AuthorizationPolicy{
							Disabled: true,
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
						Services: []contour_v1.Service{
							{
								Name: "echo",
								Port: 80,
							},
						},
					},
					{
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

		res, ok := f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Path:      "/first",
			Condition: e2e.HasStatusCode(200),
		})

		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)

		res, ok = f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Path:      "/second",
			Condition: e2e.HasStatusCode(200),
		})

		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)

		res, ok = f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Path:      "/matches-default-route",
			Condition: e2e.HasStatusCode(200),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)
	})
}
