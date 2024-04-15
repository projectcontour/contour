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

	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/require"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/test/e2e"
)

func testDefaultGlobalRateLimitingVirtualHostNonTLS(namespace string) {
	Specify("default global rate limit policy is applied on non-TLS virtualhost", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "echo")

		p := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "defaultglobalratelimitvhostnontls",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "defaultglobalratelimitvhostnontls.projectcontour.io",
				},
				Routes: []contour_v1.Route{
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

		// Wait until we get a 429 from the proxy confirming
		// that we've exceeded the rate limit.
		res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Condition: e2e.HasStatusCode(429),
			RequestOpts: []func(*http.Request){
				e2e.OptSetHeaders(map[string]string{
					"X-Default-Header": "test_value_1",
				}),
			},
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 429 response code, got %d", res.StatusCode)
	})

	Specify("default global rate limit policy is set but HTTPProxy is opted-out", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "echo")

		p := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "defaultglobalratelimitvhostnontls",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "defaultglobalratelimitvhostnontls.projectcontour.io",
					RateLimitPolicy: &contour_v1.RateLimitPolicy{
						Global: &contour_v1.GlobalRateLimitPolicy{
							Disabled: true,
						},
					},
				},
				Routes: []contour_v1.Route{
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

		// Wait until we get a 200 from the proxy confirming
		// the pods are up and serving traffic.
		res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Condition: e2e.HasStatusCode(200),
			RequestOpts: []func(*http.Request){
				e2e.OptSetHeaders(map[string]string{
					"X-Default-Header": "test_value_2",
				}),
			},
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)

		// Make another request against the proxy to confirm a 200 response
		// which indicates that HTTPProxy has disabled the default global rate limiting.
		res, ok = f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Condition: e2e.HasStatusCode(200),
			RequestOpts: []func(*http.Request){
				e2e.OptSetHeaders(map[string]string{
					"X-Default-Header": "test_value_2",
				}),
			},
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)
	})

	Specify("default global rate limit policy is set but HTTPProxy has its own global rate limit policy", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "echo")

		p := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "defaultglobalratelimitvhostnontls",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "defaultglobalratelimitvhostnontls.projectcontour.io",
					RateLimitPolicy: &contour_v1.RateLimitPolicy{
						Global: &contour_v1.GlobalRateLimitPolicy{
							Descriptors: []contour_v1.RateLimitDescriptor{
								{
									Entries: []contour_v1.RateLimitDescriptorEntry{
										{
											GenericKey: &contour_v1.GenericKeyDescriptor{
												Value: "foo",
											},
										},
									},
								},
							},
						},
					},
				},
				Routes: []contour_v1.Route{
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

		// Make requests against the proxy, confirm a 429 response
		// is now gotten since we've exceeded the rate limit.
		res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Condition: e2e.HasStatusCode(429),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 429 response code, got %d", res.StatusCode)
	})
}

func testDefaultGlobalRateLimitingVirtualHostTLS(namespace string) {
	Specify("default global rate limit policy is applied on TLS virtualhost", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "echo")
		f.Certs.CreateSelfSignedCert(namespace, "echo-cert", "echo", "globalratelimitvhosttls.projectcontour.io")

		p := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "defaultglobalratelimitvhostnontls",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "defaultglobalratelimitvhostnontls.projectcontour.io",
					TLS: &contour_v1.TLS{
						SecretName: "echo",
					},
				},
				Routes: []contour_v1.Route{
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

		// Wait until we get a 429 from the proxy confirming
		// that we've exceeded the rate limit.
		res, ok := f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Condition: e2e.HasStatusCode(429),
			RequestOpts: []func(*http.Request){
				e2e.OptSetHeaders(map[string]string{
					"X-Default-Header": "test_value_3",
				}),
			},
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 429 response code, got %d", res.StatusCode)
	})

	Specify("default global rate limit policy is set but HTTPProxy opts out", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "echo")
		f.Certs.CreateSelfSignedCert(namespace, "echo-cert", "echo", "globalratelimitroutetls.projectcontour.io")

		p := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "defaultglobalratelimitvhostnontls",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "defaultglobalratelimitvhostnontls.projectcontour.io",
					TLS: &contour_v1.TLS{
						SecretName: "echo",
					},
					RateLimitPolicy: &contour_v1.RateLimitPolicy{
						Global: &contour_v1.GlobalRateLimitPolicy{
							Disabled: true,
						},
					},
				},
				Routes: []contour_v1.Route{
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

		// Wait until we get a 200 from the proxy confirming
		// the pods are up and serving traffic.
		res, ok := f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Condition: e2e.HasStatusCode(200),
			RequestOpts: []func(*http.Request){
				e2e.OptSetHeaders(map[string]string{
					"X-Default-Header": "test_value_4",
				}),
			},
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)

		// Make another request against the proxy to confirm a 200 response
		// which indicates that HTTPProxy has disabled the default global rate limiting.
		res, ok = f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Condition: e2e.HasStatusCode(200),
			RequestOpts: []func(*http.Request){
				e2e.OptSetHeaders(map[string]string{
					"X-Default-Header": "test_value_4",
				}),
			},
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)
	})

	Specify("default global rate limit policy is set but HTTPProxy has its own global rate limit policy", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "echo")
		f.Certs.CreateSelfSignedCert(namespace, "echo-cert", "echo", "globalratelimitroutetls.projectcontour.io")

		p := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "defaultglobalratelimitvhostnontls",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "defaultglobalratelimitvhostnontls.projectcontour.io",
					TLS: &contour_v1.TLS{
						SecretName: "echo",
					},
					RateLimitPolicy: &contour_v1.RateLimitPolicy{
						Global: &contour_v1.GlobalRateLimitPolicy{
							Descriptors: []contour_v1.RateLimitDescriptor{
								{
									Entries: []contour_v1.RateLimitDescriptorEntry{
										{
											RequestHeader: &contour_v1.RequestHeaderDescriptor{
												HeaderName:    "X-HTTPProxy-Descriptor",
												DescriptorKey: "customHeader",
											},
										},
									},
								},
							},
						},
					},
				},
				Routes: []contour_v1.Route{
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

		// Make requests against the proxy, confirm a 429 response
		// is now gotten since we've exceeded the rate limit.
		res, ok := f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Condition: e2e.HasStatusCode(429),
			RequestOpts: []func(*http.Request){
				e2e.OptSetHeaders(map[string]string{
					"X-HTTPProxy-Descriptor": "test_value",
				}),
			},
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 429 response code, got %d", res.StatusCode)
	})
}

func testDefaultGlobalRateLimitingWithVhRateLimitsIgnore(namespace string) {
	Specify("default global rate limit policy is applied and route opted out from the virtual host rate limit policy", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "echo")

		p := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "defaultglobalratelimitvhratelimits",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "defaultglobalratelimitvhratelimits.projectcontour.io",
				},
				Routes: []contour_v1.Route{
					{
						Services: []contour_v1.Service{
							{
								Name: "echo",
								Port: 80,
							},
						},
						Conditions: []contour_v1.MatchCondition{
							{
								Prefix: "/echo",
							},
						},
					},
				},
			},
		}
		require.True(f.T(), f.CreateHTTPProxyAndWaitFor(p, e2e.HTTPProxyValid))

		// Wait until we get a 429 from the proxy confirming
		// that we've exceeded the rate limit.
		res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Condition: e2e.HasStatusCode(429),
			Path:      "/echo",
			RequestOpts: []func(*http.Request){
				e2e.OptSetHeaders(map[string]string{
					"X-Another-Header": "randomvalue",
				}),
			},
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 429 response code, got %d", res.StatusCode)

		require.NoError(t, retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			if err := f.Client.Get(context.TODO(), client.ObjectKeyFromObject(p), p); err != nil {
				return err
			}

			// Add a global rate limit policy on the route.
			p.Spec.Routes[0].RateLimitPolicy = &contour_v1.RateLimitPolicy{
				Global: &contour_v1.GlobalRateLimitPolicy{
					Disabled: true,
				},
			}

			return f.Client.Update(context.TODO(), p)
		}))

		// We set vh_rate_limits to ignore, which means the route should ignore any rate limit policy
		// set by the virtual host. Make another request to confirm 200.
		res, ok = f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Path:      "/echo",
			Condition: e2e.HasStatusCode(200),
			RequestOpts: []func(*http.Request){
				e2e.OptSetHeaders(map[string]string{
					"X-Another-Header": "randomvalue",
				}),
			},
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)
	})
}
