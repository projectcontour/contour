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

// +build e2e

package e2e

import (
	"context"
	"net/http"
	"testing"

	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestLocalRateLimitingVirtualHost(t *testing.T) {
	t.Parallel()

	// Start by assuming install-contour-working.sh has been run, so we
	// have Contour running in a cluster. Later we may want to move part
	// or all of that script into the E2E framework.

	var (
		fx        = NewFramework(t)
		namespace = "019-local-rate-limiting-vhost"
	)

	fx.CreateNamespace(namespace)
	defer fx.DeleteNamespace(namespace)

	fx.CreateEchoWorkload(namespace, "echo")

	p := &contourv1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "vhostlocalratelimit",
		},
		Spec: contourv1.HTTPProxySpec{
			VirtualHost: &contourv1.VirtualHost{
				Fqdn: "vhostlocalratelimit.projectcontour.io",
			},
			Routes: []contourv1.Route{
				{
					Services: []contourv1.Service{
						{
							Name: "echo",
							Port: 80,
						},
					},
				},
			},
		},
	}
	p, _ = fx.CreateHTTPProxyAndWaitFor(p, HTTPProxyValid)

	// Wait until we get a 200 from the proxy confirming
	// the pods are up and serving traffic.
	_, ok := fx.HTTPRequestUntil(IsOK, "/", p.Spec.VirtualHost.Fqdn)
	require.True(t, ok, "did not get 200 response")

	// Add a local rate limit policy on the virtual host.
	p.Spec.VirtualHost.RateLimitPolicy = &contourv1.RateLimitPolicy{
		Local: &contourv1.LocalRateLimitPolicy{
			Requests: 1,
			Unit:     "hour",
		},
	}
	require.NoError(t, fx.Client.Update(context.TODO(), p))

	// Make a request against the proxy, confirm a 200 response
	// is returned since we're allowed one request per hour.
	//
	// TODO it'd be better to just make a single request.
	_, ok = fx.HTTPRequestUntil(IsOK, "/", p.Spec.VirtualHost.Fqdn)
	require.True(t, ok, "did not get 200 response")

	// Make another request against the proxy, confirm a 429 response
	// is now gotten since we've exceeded the rate limit.
	//
	// TODO it'd be better to just make a single request.
	_, ok = fx.HTTPRequestUntil(func(r *http.Response) bool { return r.StatusCode == 429 }, "/", p.Spec.VirtualHost.Fqdn)
	require.True(t, ok, "did not get 429 response")
}

func TestLocalRateLimitingRoute(t *testing.T) {
	t.Parallel()

	// Start by assuming install-contour-working.sh has been run, so we
	// have Contour running in a cluster. Later we may want to move part
	// or all of that script into the E2E framework.

	var (
		fx        = NewFramework(t)
		namespace = "019-local-rate-limiting-route"
	)

	fx.CreateNamespace(namespace)
	defer fx.DeleteNamespace(namespace)

	fx.CreateEchoWorkload(namespace, "echo")

	p := &contourv1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "routelocalratelimit",
		},
		Spec: contourv1.HTTPProxySpec{
			VirtualHost: &contourv1.VirtualHost{
				Fqdn: "routelocalratelimit.projectcontour.io",
			},
			Routes: []contourv1.Route{
				{
					Services: []contourv1.Service{
						{
							Name: "echo",
							Port: 80,
						},
					},
				},
				{
					Services: []contourv1.Service{
						{
							Name: "echo",
							Port: 80,
						},
					},
					Conditions: []contourv1.MatchCondition{
						{
							Prefix: "/unlimited",
						},
					},
				},
			},
		},
	}
	p, _ = fx.CreateHTTPProxyAndWaitFor(p, HTTPProxyValid)

	// Wait until we get a 200 from the proxy confirming
	// the pods are up and serving traffic.
	_, ok := fx.HTTPRequestUntil(IsOK, "/", p.Spec.VirtualHost.Fqdn)
	require.True(t, ok, "did not get 200 response")

	// Add a local rate limit policy on the first route.
	p.Spec.Routes[0].RateLimitPolicy = &contourv1.RateLimitPolicy{
		Local: &contourv1.LocalRateLimitPolicy{
			Requests: 1,
			Unit:     "hour",
		},
	}
	require.NoError(t, fx.Client.Update(context.TODO(), p))

	// Make a request against the proxy, confirm a 200 response
	// is returned since we're allowed one request per hour.
	//
	// TODO it'd be better to just make a single request.
	_, ok = fx.HTTPRequestUntil(IsOK, "/", p.Spec.VirtualHost.Fqdn)
	require.True(t, ok, "did not get 200 response")

	// Make another request against the proxy, confirm a 429 response
	// is now gotten since we've exceeded the rate limit.
	//
	// TODO it'd be better to just make a single request.
	_, ok = fx.HTTPRequestUntil(func(r *http.Response) bool { return r.StatusCode == 429 }, "/", p.Spec.VirtualHost.Fqdn)
	require.True(t, ok, "did not get 429 response")

	// Make a request against the route that doesn't have rate limiting
	// to confirm we still get a 200 for that route.
	_, ok = fx.HTTPRequestUntil(IsOK, "/unlimited", p.Spec.VirtualHost.Fqdn)
	require.True(t, ok, "did not get 200 response for non-rate-limited route")
}
