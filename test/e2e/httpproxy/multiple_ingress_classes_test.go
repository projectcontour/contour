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
	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/require"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/test/e2e"
)

func testMultipleIngressClassesField(namespace string) {
	Specify("multiple ingress class names with a single Contour instance (ingressClassName field)", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "ingress-conformance-echo")

		for _, class := range []string{"contour", "team1"} {
			p := &contour_v1.HTTPProxy{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: namespace,
					Name:      "multiple-ingress-classes" + class + "-httpproxy",
				},
				Spec: contour_v1.HTTPProxySpec{
					VirtualHost: &contour_v1.VirtualHost{
						Fqdn: class + "httpproxy.projectcontour.io",
					},
					Routes: []contour_v1.Route{
						{
							Services: []contour_v1.Service{
								{
									Name: "ingress-conformance-echo",
									Port: 80,
								},
							},
						},
					},
				},
			}
			if class != "" {
				p.Spec.IngressClassName = class
			}

			require.True(f.T(), f.CreateHTTPProxyAndWaitFor(p, e2e.HTTPProxyValid))

			res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
				Host:      p.Spec.VirtualHost.Fqdn,
				Condition: e2e.HasStatusCode(200),
			})
			require.NotNil(t, res, "request never succeeded")
			require.Truef(t, ok, "%s ingress: expected 200 response code, got %d", class, res.StatusCode)
		}
	})
}

func testMultipleIngressClassesAnnotation(namespace string) {
	Specify("multiple ingress class names with a single Contour instance (annotation)", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "ingress-conformance-echo")

		for _, class := range []string{"contour", "team1"} {
			p := &contour_v1.HTTPProxy{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: namespace,
					Name:      "multiple-ingress-classes" + class + "-httpproxy-annotation",
				},
				Spec: contour_v1.HTTPProxySpec{
					VirtualHost: &contour_v1.VirtualHost{
						Fqdn: class + "httpproxy-annotation.projectcontour.io",
					},
					Routes: []contour_v1.Route{
						{
							Services: []contour_v1.Service{
								{
									Name: "ingress-conformance-echo",
									Port: 80,
								},
							},
						},
					},
				},
			}
			if class != "" {
				p.Annotations = map[string]string{
					"kubernetes.io/ingress.class": class,
				}
			}

			require.True(f.T(), f.CreateHTTPProxyAndWaitFor(p, e2e.HTTPProxyValid))

			res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
				Host:      p.Spec.VirtualHost.Fqdn,
				Condition: e2e.HasStatusCode(200),
			})
			require.NotNil(t, res, "request never succeeded")
			require.Truef(t, ok, "%s ingress: expected 200 response code, got %d", class, res.StatusCode)
		}
	})
}
