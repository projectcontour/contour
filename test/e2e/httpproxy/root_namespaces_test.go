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
	. "github.com/onsi/ginkgo"
	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func testRootNamespaces(namespaces []string) e2e.NamespacedTestBody {
	return func(testNS string) {
		Specify("root HTTPProxies outside of root namespaces are not configured", func() {
			for _, ns := range namespaces {
				deployEchoServer(f.T(), f.Client, ns, "echo")
				p := &contourv1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
						Name:      "root-proxy",
					},
					Spec: contourv1.HTTPProxySpec{
						VirtualHost: &contourv1.VirtualHost{
							Fqdn: "root-proxy-" + ns + ".projectcontour.io",
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
				f.CreateHTTPProxyAndWaitFor(p, httpProxyValid)

				res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
					Host:      p.Spec.VirtualHost.Fqdn,
					Condition: e2e.HasStatusCode(200),
				})
				require.NotNil(f.T(), res)
				require.Truef(f.T(), ok, "expected 200 response code, got %d", res.StatusCode)
			}

			deployEchoServer(f.T(), f.Client, testNS, "echo")
			p := &contourv1.HTTPProxy{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNS,
					Name:      "root-proxy",
				},
				Spec: contourv1.HTTPProxySpec{
					VirtualHost: &contourv1.VirtualHost{
						Fqdn: "root-proxy-" + testNS + ".projectcontour.io",
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
			f.CreateHTTPProxyAndWaitFor(p, httpProxyRootNotAllowedInNS)
		})
	}
}

func httpProxyRootNotAllowedInNS(proxy *contourv1.HTTPProxy) bool {
	if proxy == nil {
		return false
	}

	if len(proxy.Status.Conditions) == 0 {
		return false
	}

	validCond := proxy.Status.GetConditionFor("Valid")
	if validCond.Status != "False" {
		return false
	}
	subCond, found := validCond.GetError("RootNamespaceError")
	if !found {
		return false
	}
	return subCond.Status == "True"
}
