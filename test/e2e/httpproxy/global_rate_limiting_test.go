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

	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/require"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/test/e2e"
)

func testGlobalRateLimitingVirtualHostNonTLS(namespace string) {
	Specify("global rate limit policy set on non-TLS virtualhost is applied", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "echo")

		p := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "globalratelimitvhostnontls",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "globalratelimitvhostnontls.projectcontour.io",
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
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)

		require.NoError(t, retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			if err := f.Client.Get(context.TODO(), client.ObjectKeyFromObject(p), p); err != nil {
				return err
			}

			// Add a global rate limit policy on the virtual host.
			p.Spec.VirtualHost.RateLimitPolicy = &contour_v1.RateLimitPolicy{
				Global: &contour_v1.GlobalRateLimitPolicy{
					Descriptors: []contour_v1.RateLimitDescriptor{
						{
							Entries: []contour_v1.RateLimitDescriptorEntry{
								{
									GenericKey: &contour_v1.GenericKeyDescriptor{
										Value: "vhostlimit",
									},
								},
							},
						},
					},
				},
			}

			return f.Client.Update(context.TODO(), p)
		}))

		// Make a request against the proxy, confirm a 200 response
		// is returned since we're allowed one request per hour.
		res, ok = f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Condition: e2e.HasStatusCode(200),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)

		// Make another request against the proxy, confirm a 429 response
		// is now gotten since we've exceeded the rate limit.
		res, ok = f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Condition: e2e.HasStatusCode(429),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 429 response code, got %d", res.StatusCode)
	})
}

func testGlobalRateLimitingRouteNonTLS(namespace string) {
	Specify("global rate limit policy set on non-TLS route is applied", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "echo")

		p := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "globalratelimitroutenontls",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "globalratelimitroutenontls.projectcontour.io",
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
					{
						Services: []contour_v1.Service{
							{
								Name: "echo",
								Port: 80,
							},
						},
						Conditions: []contour_v1.MatchCondition{
							{
								Prefix: "/unlimited",
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
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)

		// Add a global rate limit policy on the first route.
		require.NoError(t, retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			if err := f.Client.Get(context.TODO(), client.ObjectKeyFromObject(p), p); err != nil {
				return err
			}

			p.Spec.Routes[0].RateLimitPolicy = &contour_v1.RateLimitPolicy{
				Global: &contour_v1.GlobalRateLimitPolicy{
					Descriptors: []contour_v1.RateLimitDescriptor{
						{
							Entries: []contour_v1.RateLimitDescriptorEntry{
								{
									GenericKey: &contour_v1.GenericKeyDescriptor{
										Key:   "route_limit_key",
										Value: "routelimit",
									},
								},
							},
						},
					},
				},
			}

			return f.Client.Update(context.TODO(), p)
		}))

		// Make a request against the proxy, confirm a 200 response
		// is returned since we're allowed one request per hour.
		res, ok = f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Condition: e2e.HasStatusCode(200),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)

		// Make another request against the proxy, confirm a 429 response
		// is now gotten since we've exceeded the rate limit.
		res, ok = f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Condition: e2e.HasStatusCode(429),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 429 response code, got %d", res.StatusCode)

		// Make a request against the route that doesn't have rate limiting
		// to confirm we still get a 200 for that route.
		res, ok = f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Path:      "/unlimited",
			Condition: e2e.HasStatusCode(200),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 200 response code for non-rate-limited route, got %d", res.StatusCode)
	})
}

func testGlobalRateLimitingVirtualHostTLS(namespace string) {
	Specify("global rate limit policy set on TLS virtualhost is applied", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "echo")
		f.Certs.CreateSelfSignedCert(namespace, "echo-cert", "echo", "globalratelimitvhosttls.projectcontour.io")

		p := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "globalratelimitvhosttls",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "globalratelimitvhosttls.projectcontour.io",
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

		// Wait until we get a 200 from the proxy confirming
		// the pods are up and serving traffic.
		res, ok := f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Condition: e2e.HasStatusCode(200),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)

		// Add a global rate limit policy on the virtual host.
		require.NoError(t, retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			if err := f.Client.Get(context.TODO(), client.ObjectKeyFromObject(p), p); err != nil {
				return err
			}

			p.Spec.VirtualHost.RateLimitPolicy = &contour_v1.RateLimitPolicy{
				Global: &contour_v1.GlobalRateLimitPolicy{
					Descriptors: []contour_v1.RateLimitDescriptor{
						{
							Entries: []contour_v1.RateLimitDescriptorEntry{
								{
									GenericKey: &contour_v1.GenericKeyDescriptor{
										Value: "tlsvhostlimit",
									},
								},
							},
						},
					},
				},
			}

			return f.Client.Update(context.TODO(), p)
		}))

		// Make a request against the proxy, confirm a 200 response
		// is returned since we're allowed one request per hour.
		res, ok = f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Condition: e2e.HasStatusCode(200),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)

		// Make another request against the proxy, confirm a 429 response
		// is now gotten since we've exceeded the rate limit.
		res, ok = f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Condition: e2e.HasStatusCode(429),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 429 response code, got %d", res.StatusCode)
	})
}

func testGlobalRateLimitingRouteTLS(namespace string) {
	Specify("global rate limit policy set on TLS route is applied", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "echo")
		f.Certs.CreateSelfSignedCert(namespace, "echo-cert", "echo", "globalratelimitroutetls.projectcontour.io")

		p := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "globalratelimitroutetls",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "globalratelimitroutetls.projectcontour.io",
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
					{
						Services: []contour_v1.Service{
							{
								Name: "echo",
								Port: 80,
							},
						},
						Conditions: []contour_v1.MatchCondition{
							{
								Prefix: "/unlimited",
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
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)

		// Add a global rate limit policy on the first route.
		require.NoError(t, retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			if err := f.Client.Get(context.TODO(), client.ObjectKeyFromObject(p), p); err != nil {
				return err
			}

			p.Spec.Routes[0].RateLimitPolicy = &contour_v1.RateLimitPolicy{
				Global: &contour_v1.GlobalRateLimitPolicy{
					Descriptors: []contour_v1.RateLimitDescriptor{
						{
							Entries: []contour_v1.RateLimitDescriptorEntry{
								{
									GenericKey: &contour_v1.GenericKeyDescriptor{
										Value: "tlsroutelimit",
									},
								},
							},
						},
					},
				},
			}

			return f.Client.Update(context.TODO(), p)
		}))

		// Make a request against the proxy, confirm a 200 response
		// is returned since we're allowed one request per hour.
		res, ok = f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Condition: e2e.HasStatusCode(200),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)

		// Make another request against the proxy, confirm a 429 response
		// is now gotten since we've exceeded the rate limit.
		res, ok = f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Condition: e2e.HasStatusCode(429),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 429 response code, got %d", res.StatusCode)

		// Make a request against the route that doesn't have rate limiting
		// to confirm we still get a 200 for that route.
		res, ok = f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Path:      "/unlimited",
			Condition: e2e.HasStatusCode(200),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 200 response code for non-rate-limited route, got %d", res.StatusCode)
	})
}

func testDisableVirtualHostGlobalRateLimitingOnRoute(namespace string) {
	Specify("global rate limit policy set on virtualhost is applied with disabled set to false on a route", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "echo")

		p := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "globalratelimitvhostnontls",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "globalratelimitvhostnontls.projectcontour.io",
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

		// Wait until we get a 200 from the proxy confirming
		// the pods are up and serving traffic.
		res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Path:      "/echo",
			Condition: e2e.HasStatusCode(200),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)

		require.NoError(t, retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			if err := f.Client.Get(context.TODO(), client.ObjectKeyFromObject(p), p); err != nil {
				return err
			}

			// Add a global rate limit policy on the virtual host.
			p.Spec.VirtualHost.RateLimitPolicy = &contour_v1.RateLimitPolicy{
				Global: &contour_v1.GlobalRateLimitPolicy{
					Descriptors: []contour_v1.RateLimitDescriptor{
						{
							Entries: []contour_v1.RateLimitDescriptorEntry{
								{
									GenericKey: &contour_v1.GenericKeyDescriptor{
										Value: "randomvalue",
									},
								},
							},
						},
					},
				},
			}

			return f.Client.Update(context.TODO(), p)
		}))

		// Wait until we confirm a 429 response is now gotten when we exceed the rate limit.
		res, ok = f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Path:      "/echo",
			Condition: e2e.HasStatusCode(429),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 429 response code, got %d", res.StatusCode)

		require.NoError(t, retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			if err := f.Client.Get(context.TODO(), client.ObjectKeyFromObject(p), p); err != nil {
				return err
			}

			// Set disabled to false explicitly on the route.
			p.Spec.Routes[0].RateLimitPolicy = &contour_v1.RateLimitPolicy{
				Global: &contour_v1.GlobalRateLimitPolicy{
					Disabled: false,
				},
			}

			return f.Client.Update(context.TODO(), p)
		}))

		// Confirm we still see a 429 response.
		res, ok = f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Path:      "/echo",
			Condition: e2e.HasStatusCode(429),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 429 response code, got %d", res.StatusCode)
	})

	Specify("global rate limit policy set on virtualhost is applied with disabled set to true on a route", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "echo")

		p := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "globalratelimitvhostnontls",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "globalratelimitvhostnontls.projectcontour.io",
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

		// Wait until we get a 200 from the proxy confirming
		// the pods are up and serving traffic.
		res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Path:      "/echo",
			Condition: e2e.HasStatusCode(200),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)

		require.NoError(t, retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			if err := f.Client.Get(context.TODO(), client.ObjectKeyFromObject(p), p); err != nil {
				return err
			}

			// Add a global rate limit policy on the virtual host.
			p.Spec.VirtualHost.RateLimitPolicy = &contour_v1.RateLimitPolicy{
				Global: &contour_v1.GlobalRateLimitPolicy{
					Descriptors: []contour_v1.RateLimitDescriptor{
						{
							Entries: []contour_v1.RateLimitDescriptorEntry{
								{
									GenericKey: &contour_v1.GenericKeyDescriptor{
										Value: "randomvalue",
									},
								},
							},
						},
					},
				},
			}

			return f.Client.Update(context.TODO(), p)
		}))

		// Wait until we confirm a 429 response is now gotten when we exceed the rate limit.
		res, ok = f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Path:      "/echo",
			Condition: e2e.HasStatusCode(429),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 429 response code, got %d", res.StatusCode)

		require.NoError(t, retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			if err := f.Client.Get(context.TODO(), client.ObjectKeyFromObject(p), p); err != nil {
				return err
			}

			// Disable Vhost global rate limit policy on the route.
			p.Spec.Routes[0].RateLimitPolicy = &contour_v1.RateLimitPolicy{
				Global: &contour_v1.GlobalRateLimitPolicy{
					Disabled: true,
				},
			}

			return f.Client.Update(context.TODO(), p)
		}))

		// Make another request against the proxy, confirm a 200 response
		// is now gotten since the route explicitly opted out from the vhost global rate limiting
		res, ok = f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Path:      "/echo",
			Condition: e2e.HasStatusCode(200),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)
	})
}
