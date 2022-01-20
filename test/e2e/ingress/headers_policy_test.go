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

package ingress

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func testGlobalHeadersPolicy(applyToIngress bool) e2e.NamespacedTestBody {
	return func(namespace string) {
		var text string
		if applyToIngress {
			text = "global headers policy is applied to ingress objects"
		} else {
			text = "global headers policy is not applied to ingress objects"
		}

		Specify(text, func() {
			t := f.T()

			f.Fixtures.Echo.Deploy(namespace, "echo")

			i := &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      "global-headers-policy",
				},
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{
						{
							Host: "global-headers-policy.ingress.projectcontour.io",
							IngressRuleValue: networkingv1.IngressRuleValue{
								HTTP: &networkingv1.HTTPIngressRuleValue{
									Paths: []networkingv1.HTTPIngressPath{
										{
											PathType: e2e.IngressPathTypePtr(networkingv1.PathTypePrefix),
											Path:     "/",
											Backend: networkingv1.IngressBackend{
												Service: &networkingv1.IngressServiceBackend{
													Name: "echo",
													Port: networkingv1.ServiceBackendPort{
														Number: 80,
													},
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
			require.NoError(f.T(), f.Client.Create(context.Background(), i))

			res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
				Host:      i.Spec.Rules[0].Host,
				Condition: e2e.HasStatusCode(200),
			})
			require.NotNil(t, res, "request never succeeded")
			require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)

			if applyToIngress {
				assert.Equal(t, "foo", f.GetEchoResponseBody(res.Body).RequestHeaders.Get("X-Contour-GlobalRequestHeader"))
				assert.Equal(t, "bar", res.Headers.Get("X-Contour-GlobalResponseHeader"))
			} else {
				assert.Equal(t, "", f.GetEchoResponseBody(res.Body).RequestHeaders.Get("X-Contour-GlobalRequestHeader"))
				assert.Equal(t, "", res.Headers.Get("X-Contour-GlobalResponseHeader"))
			}
		})
	}
}
