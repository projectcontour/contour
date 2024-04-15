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

func testIPFilterPolicy(namespace string) {
	Specify("requests can be filtered by ip address", func() {
		t := f.T()
		ctx, cancel := context.WithCancel(context.Background())
		DeferCleanup(cancel)

		f.Fixtures.Echo.Deploy(namespace, "echo")

		p := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "ipfilter1",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "ipfilter1.projectcontour.io",
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

		// Deny all ips so that the next request fails
		require.NoError(t, retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			if err := f.Client.Get(ctx, client.ObjectKeyFromObject(p), p); err != nil {
				return err
			}

			p.Spec.Routes[0].IPDenyFilterPolicy = ipFilterDenyAll()

			return f.Client.Update(ctx, p)
		}))

		// Make a request against the proxy, it should fail
		res, ok = f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Condition: e2e.HasStatusCode(403),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 403 response code, got %d", res.StatusCode)

		// Only allow requests from 10.10.10.10
		require.NoError(t, retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			if err := f.Client.Get(ctx, client.ObjectKeyFromObject(p), p); err != nil {
				return err
			}

			p.Spec.Routes[0].IPAllowFilterPolicy = []contour_v1.IPFilterPolicy{
				{
					Source: contour_v1.IPFilterSourceRemote,
					CIDR:   "10.10.10.10/32",
				},
			}
			p.Spec.Routes[0].IPDenyFilterPolicy = nil

			return f.Client.Update(ctx, p)
		}))

		// Add an X-Forwarded-For header to match the allowed ip, it should succeed
		res, ok = f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Condition: e2e.HasStatusCode(200),
			RequestOpts: []func(*http.Request){
				e2e.OptSetHeaders(map[string]string{"X-Forwarded-For": "10.10.10.10"}),
			},
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)

		// A request with the wrong ip should fail
		res, ok = f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Condition: e2e.HasStatusCode(403),
			RequestOpts: []func(*http.Request){
				e2e.OptSetHeaders(map[string]string{"X-Forwarded-For": "10.10.10.0"}),
			},
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 403 response code, got %d", res.StatusCode)
	})

	Specify("per-route ip filters override virtualhost ipfilters", func() {
		t := f.T()
		ctx, cancel := context.WithCancel(context.Background())
		DeferCleanup(cancel)

		f.Fixtures.Echo.Deploy(namespace, "echo")

		p := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "ipfilter2",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "ipfilter2.projectcontour.io",
				},
				Routes: []contour_v1.Route{
					{
						Conditions: []contour_v1.MatchCondition{{
							Prefix: "/one",
						}},
						Services: []contour_v1.Service{
							{
								Name: "echo",
								Port: 80,
							},
						},
					},
					{
						Conditions: []contour_v1.MatchCondition{{
							Prefix: "/other",
						}},
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
			Path:      "/one",
			Host:      p.Spec.VirtualHost.Fqdn,
			Condition: e2e.HasStatusCode(200),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)

		// Deny all ips so that the next request fails
		require.NoError(t, retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			if err := f.Client.Get(ctx, client.ObjectKeyFromObject(p), p); err != nil {
				return err
			}

			p.Spec.VirtualHost.IPDenyFilterPolicy = ipFilterDenyAll()

			return f.Client.Update(ctx, p)
		}))

		// Make a request against the proxy, it should fail
		res, ok = f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Path:      "/one",
			Host:      p.Spec.VirtualHost.Fqdn,
			Condition: e2e.HasStatusCode(403),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 403 response code, got %d", res.StatusCode)

		// Allow requests from 10.10.10.10 on the route
		require.NoError(t, retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			if err := f.Client.Get(ctx, client.ObjectKeyFromObject(p), p); err != nil {
				return err
			}

			p.Spec.Routes[0].IPAllowFilterPolicy = []contour_v1.IPFilterPolicy{
				{
					Source: contour_v1.IPFilterSourceRemote,
					CIDR:   "10.10.10.10",
				},
			}
			p.Spec.Routes[0].IPDenyFilterPolicy = nil

			return f.Client.Update(ctx, p)
		}))

		// Add an X-Forwarded-For header to match the allowed ip, it should succeed
		res, ok = f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Path:      "/one",
			Host:      p.Spec.VirtualHost.Fqdn,
			Condition: e2e.HasStatusCode(200),
			RequestOpts: []func(*http.Request){
				e2e.OptSetHeaders(map[string]string{"X-Forwarded-For": "10.10.10.10"}),
			},
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)

		// A request with the wrong ip should fail
		res, ok = f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Path:      "/one",
			Host:      p.Spec.VirtualHost.Fqdn,
			Condition: e2e.HasStatusCode(403),
			RequestOpts: []func(*http.Request){
				e2e.OptSetHeaders(map[string]string{"X-Forwarded-For": "10.10.10.0"}),
			},
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 403 response code, got %d", res.StatusCode)

		// A request against the other route should fail (virtualhost-level filter applies)
		res, ok = f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Path:      "/other",
			Host:      p.Spec.VirtualHost.Fqdn,
			Condition: e2e.HasStatusCode(403),
			RequestOpts: []func(*http.Request){
				e2e.OptSetHeaders(map[string]string{"X-Forwarded-For": "10.10.10.0"}),
			},
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 403 response code, got %d", res.StatusCode)
	})

	Specify("requests can be filtered by ip address in included routes", func() {
		t := f.T()
		ctx, cancel := context.WithCancel(context.Background())
		DeferCleanup(cancel)

		f.Fixtures.Echo.Deploy(namespace, "echo")

		r := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "ipfilter3-root",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "ipfilter3.projectcontour.io",
				},
				Includes: []contour_v1.Include{{
					Namespace: namespace,
					Name:      "ipfilter3-child",
				}},
			},
		}
		// root will be missing an include when created
		require.True(f.T(), f.CreateHTTPProxyAndWaitFor(r, e2e.HTTPProxyInvalid))

		p := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "ipfilter3-child",
			},
			Spec: contour_v1.HTTPProxySpec{
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
			Host:      r.Spec.VirtualHost.Fqdn,
			Condition: e2e.HasStatusCode(200),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)

		// Deny all ips so that the next request fails
		require.NoError(t, retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			if err := f.Client.Get(ctx, client.ObjectKeyFromObject(p), p); err != nil {
				return err
			}

			p.Spec.Routes[0].IPDenyFilterPolicy = ipFilterDenyAll()

			return f.Client.Update(ctx, p)
		}))

		// Make a request against the proxy, it should fail
		res, ok = f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      r.Spec.VirtualHost.Fqdn,
			Condition: e2e.HasStatusCode(403),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 403 response code, got %d", res.StatusCode)

		// Only allow requests from 10.10.10.10
		require.NoError(t, retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			if err := f.Client.Get(ctx, client.ObjectKeyFromObject(p), p); err != nil {
				return err
			}

			p.Spec.Routes[0].IPAllowFilterPolicy = []contour_v1.IPFilterPolicy{
				{
					Source: contour_v1.IPFilterSourceRemote,
					CIDR:   "10.10.10.10/32",
				},
			}
			p.Spec.Routes[0].IPDenyFilterPolicy = nil

			return f.Client.Update(ctx, p)
		}))

		// Add an X-Forwarded-For header to match the allowed ip, it should succeed
		res, ok = f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      r.Spec.VirtualHost.Fqdn,
			Condition: e2e.HasStatusCode(200),
			RequestOpts: []func(*http.Request){
				e2e.OptSetHeaders(map[string]string{"X-Forwarded-For": "10.10.10.10"}),
			},
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)

		// A request with the wrong ip should fail
		res, ok = f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      r.Spec.VirtualHost.Fqdn,
			Condition: e2e.HasStatusCode(403),
			RequestOpts: []func(*http.Request){
				e2e.OptSetHeaders(map[string]string{"X-Forwarded-For": "10.10.10.0"}),
			},
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 403 response code, got %d", res.StatusCode)
	})
}

// Needs IPv4 and IPv6 rules to ensure this test works in both types of clusters.
func ipFilterDenyAll() []contour_v1.IPFilterPolicy {
	return []contour_v1.IPFilterPolicy{
		{
			Source: contour_v1.IPFilterSourcePeer,
			CIDR:   "10.8.8.8/0",
		},
		{
			Source: contour_v1.IPFilterSourceRemote,
			CIDR:   "10.8.8.8/0",
		},
		{
			Source: contour_v1.IPFilterSourcePeer,
			CIDR:   "::/0",
		},
		{
			Source: contour_v1.IPFilterSourceRemote,
			CIDR:   "::/0",
		},
	}
}
