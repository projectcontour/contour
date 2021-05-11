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

package httpproxy

import (
	"context"

	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func testHTTPHealthChecks(fx *e2e.Framework) {
	t := fx.T()
	namespace := "015-http-health-checks"

	fx.CreateNamespace(namespace)
	defer fx.DeleteNamespace(namespace)

	fx.Fixtures.HTTPBin.Deploy(namespace, "httpbin")

	p := &contourv1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "health-checks",
		},
		Spec: contourv1.HTTPProxySpec{
			VirtualHost: &contourv1.VirtualHost{
				Fqdn: "healthchecks.projectcontour.io",
			},
			Routes: []contourv1.Route{
				{
					Services: []contourv1.Service{
						{
							Name: "httpbin",
							Port: 80,
						},
					},
				},
			},
		},
	}
	fx.CreateHTTPProxyAndWaitFor(p, httpProxyValid)

	res, ok := fx.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
		Host:      p.Spec.VirtualHost.Fqdn,
		Condition: e2e.HasStatusCode(200),
	})
	require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)

	// set the health check policy to always fail
	require.NoError(t, retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if err := fx.Client.Get(context.TODO(), client.ObjectKeyFromObject(p), p); err != nil {
			return err
		}

		p.Spec.Routes[0].HealthCheckPolicy = &contourv1.HTTPHealthCheckPolicy{
			Path: "/status/418",
		}

		return fx.Client.Update(context.TODO(), p)
	}))

	// the health check is set to always fail so the service should
	// be unavailable.
	res, ok = fx.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
		Host:      p.Spec.VirtualHost.Fqdn,
		Condition: e2e.HasStatusCode(503),
	})
	require.Truef(t, ok, "expected 503 response code, got %d", res.StatusCode)

	// set the health check policy to always pass
	require.NoError(t, retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if err := fx.Client.Get(context.TODO(), client.ObjectKeyFromObject(p), p); err != nil {
			return err
		}

		p.Spec.Routes[0].HealthCheckPolicy = &contourv1.HTTPHealthCheckPolicy{
			Path: "/status/200",
		}

		return fx.Client.Update(context.TODO(), p)
	}))

	// the health check is set to always pass so the service should
	// return a 200.
	res, ok = fx.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
		Host:      p.Spec.VirtualHost.Fqdn,
		Condition: e2e.HasStatusCode(200),
	})
	require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)
}
