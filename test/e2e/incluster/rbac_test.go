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

package incluster

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	networking_v1 "k8s.io/api/networking/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/test/e2e"
)

func testProjectcontourResourcesRBAC(namespace string) {
	Specify("Contour ClusterRole is set up to allow access to projectcontour.io API group resources and resource status", func() {
		f.Fixtures.Echo.Deploy(namespace, "echo")

		otherNS := "another-" + namespace
		f.CreateNamespace(otherNS)
		defer f.DeleteNamespace(otherNS, false)
		f.Certs.CreateSelfSignedCert(otherNS, "delegated-cert", "delegated-cert", "rbac-test.projectcontour.io")

		// HTTPProxy and TLSCertificateDelegation
		t := &contour_v1.TLSCertificateDelegation{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: otherNS,
				Name:      "rbac",
			},
			Spec: contour_v1.TLSCertificateDelegationSpec{
				Delegations: []contour_v1.CertificateDelegation{
					{
						SecretName:       "delegated-cert",
						TargetNamespaces: []string{namespace},
					},
				},
			},
		}
		require.NoError(f.T(), f.Client.Create(context.TODO(), t))

		p := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "rbac",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "rbac-test.projectcontour.io",
					TLS: &contour_v1.TLS{
						SecretName: otherNS + "/delegated-cert",
					},
				},
				Routes: []contour_v1.Route{
					{
						Services: []contour_v1.Service{
							{Name: "invalid-service", Port: 80},
						},
					},
				},
			},
		}
		require.True(f.T(), f.CreateHTTPProxyAndWaitFor(p, e2e.HTTPProxyInvalid))

		// Update HTTPProxy to valid service.
		require.NoError(f.T(), retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			if err := f.Client.Get(context.TODO(), client.ObjectKeyFromObject(p), p); err != nil {
				return err
			}

			p.Spec.Routes[0].Services[0].Name = "echo"
			return f.Client.Update(context.TODO(), p)
		}))

		assert.Eventually(f.T(), func() bool {
			if err := f.Client.Get(context.TODO(), client.ObjectKeyFromObject(p), p); err != nil {
				return false
			}
			return e2e.HTTPProxyValid(p)
		}, time.Second*5, time.Millisecond*20)

		// Check request to app works.
		res, ok := f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
			Host:      "rbac-test.projectcontour.io",
			Condition: e2e.HasStatusCode(200),
		})
		assert.Truef(f.T(), ok, "expected %d response code, got %d", 200, res.StatusCode)

		// ExtensionService
		e := &contour_v1alpha1.ExtensionService{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "rbac",
			},
			Spec: contour_v1alpha1.ExtensionServiceSpec{
				Services: []contour_v1alpha1.ExtensionServiceTarget{
					{Name: "invalid-service", Port: 80},
				},
			},
		}
		require.NoError(f.T(), f.Client.Create(context.TODO(), e))
		assert.Eventually(f.T(), func() bool {
			if err := f.Client.Get(context.TODO(), client.ObjectKeyFromObject(e), e); err != nil {
				return false
			}
			return e2e.DetailedConditionInvalid(e.Status.Conditions)
		}, time.Second*5, time.Millisecond*20)
	})
}

func testIngressResourceRBAC(namespace string) {
	Specify("Contour ClusterRole is set up to allow access to Ingress v1 resources and resource status", func() {
		f.Fixtures.Echo.Deploy(namespace, "echo")

		i := &networking_v1.Ingress{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "rbac",
			},
			Spec: networking_v1.IngressSpec{
				Rules: []networking_v1.IngressRule{
					{
						Host: "rbac-test-ingress.projectcontour.io",
						IngressRuleValue: networking_v1.IngressRuleValue{
							HTTP: &networking_v1.HTTPIngressRuleValue{
								Paths: []networking_v1.HTTPIngressPath{
									{
										PathType: ptr.To(networking_v1.PathTypePrefix),
										Path:     "/",
										Backend: networking_v1.IngressBackend{
											Service: &networking_v1.IngressServiceBackend{
												Name: "echo",
												Port: networking_v1.ServiceBackendPort{Number: 80},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}
		require.NoError(f.T(), f.Client.Create(context.TODO(), i))

		// TODO: In kind, the Envoy service does not get a load
		// balancer address and currently the default
		// deployment does not yet utilize the
		// --ingress-status-address flag.
		//
		// Make sure Contour has updated load balancer status
		// assert.Eventually(f.T(), func() bool {
		// 	if err := f.Client.Get(context.TODO(), client.ObjectKeyFromObject(i), i); err != nil {
		// 		return false
		// 	}
		// 	return len(i.Status.LoadBalancer.Ingress) > 0
		// }, time.Second*5, time.Millisecond*20)

		// Check Contour has read Ingress resource and
		// programmed a route.
		res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      "rbac-test-ingress.projectcontour.io",
			Path:      "/",
			Condition: e2e.HasStatusCode(200),
		})
		require.NotNil(f.T(), res, "request never succeeded")
		require.Truef(f.T(), ok, "expected 200 response code, got %d", res.StatusCode)
	})
}
