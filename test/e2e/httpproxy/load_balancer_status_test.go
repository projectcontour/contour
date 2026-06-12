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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	networking_v1 "k8s.io/api/networking/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/test/e2e"
)

func testLoadBalancerStatusFromIngress(namespace string) {
	Specify("status address is propagated from source Ingress to HTTPProxy", func() {
		// Create Ingress that Contour watches for LB status and set its status.
		sourceIngress := &networking_v1.Ingress{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      "lb-status-source",
				Namespace: "projectcontour",
			},
			Spec: networking_v1.IngressSpec{
				IngressClassName: ptr.To("not-controlled-by-contour"),
				DefaultBackend: &networking_v1.IngressBackend{
					Service: &networking_v1.IngressServiceBackend{
						Name: "placeholder",
						Port: networking_v1.ServiceBackendPort{Number: 80},
					},
				},
			},
		}
		require.NoError(f.T(), f.Client.Create(context.TODO(), sourceIngress))
		DeferCleanup(func() {
			_ = f.Client.Delete(context.TODO(), sourceIngress)
		})

		// setSourceStatus updates the Ingress LB status.
		setSourceStatus := func(ingresses []networking_v1.IngressLoadBalancerIngress) {
			require.NoError(f.T(), f.Client.Get(context.TODO(), client.ObjectKeyFromObject(sourceIngress), sourceIngress))
			sourceIngress.Status.LoadBalancer = networking_v1.IngressLoadBalancerStatus{
				Ingress: ingresses,
			}
			require.NoError(f.T(), f.Client.Status().Update(context.TODO(), sourceIngress))
		}

		// Create HTTPProxy and wait for Contour to propagate the LB status.
		f.Fixtures.Echo.Deploy(namespace, "echo")
		p := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "lb-status-test",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "lb-status-test.projectcontour.io",
				},
				Routes: []contour_v1.Route{{
					Services: []contour_v1.Service{{Name: "echo", Port: 80}},
				}},
			},
		}

		proxyHasLBStatus := func(expected []networking_v1.IngressLoadBalancerIngress) bool {
			if err := f.Client.Get(context.TODO(), client.ObjectKeyFromObject(p), p); err != nil {
				return false
			}
			if len(p.Status.LoadBalancer.Ingress) != len(expected) {
				return false
			}
			for i, exp := range expected {
				if p.Status.LoadBalancer.Ingress[i].IP != exp.IP || p.Status.LoadBalancer.Ingress[i].Hostname != exp.Hostname {
					return false
				}
			}
			return true
		}

		// Set initial status and verify it propagates after HTTPProxy is created.
		setSourceStatus([]networking_v1.IngressLoadBalancerIngress{{IP: "1.2.3.4"}})
		require.True(f.T(), f.CreateHTTPProxyAndWaitFor(p, e2e.HTTPProxyValid))
		assert.Eventually(f.T(), func() bool {
			return proxyHasLBStatus([]networking_v1.IngressLoadBalancerIngress{{IP: "1.2.3.4"}})
		}, f.RetryTimeout, f.RetryInterval)

		// Verify updates propagate and that multiple entries including both IP and hostname are handled correctly.
		setSourceStatus([]networking_v1.IngressLoadBalancerIngress{
			{IP: "10.0.0.1"},
			{IP: "10.0.0.2"},
			{Hostname: "lb.example.com"},
		})
		assert.Eventually(f.T(), func() bool {
			return proxyHasLBStatus([]networking_v1.IngressLoadBalancerIngress{
				{IP: "10.0.0.1"},
				{IP: "10.0.0.2"},
				{Hostname: "lb.example.com"},
			})
		}, f.RetryTimeout, f.RetryInterval)
	})
}
