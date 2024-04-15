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
	"time"

	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/require"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/test/e2e"
)

func testWatchNamespaces(namespaces []string) e2e.NamespacedTestBody {
	return func(nonWatchedNS string) {
		Specify("HTTPProxies outside of watched namespaces are not configured", func() {
			// Proxy in watched namespace should succeed
			for _, ns := range namespaces {
				deployEchoServer(f.T(), f.Client, ns, "echo")
				p := newEchoProxy("proxy", ns)
				require.True(f.T(), f.CreateHTTPProxyAndWaitFor(p, e2e.HTTPProxyValid))

				res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
					Host:      p.Spec.VirtualHost.Fqdn,
					Condition: e2e.HasStatusCode(200),
				})
				require.NotNil(f.T(), res)
				require.Truef(f.T(), ok, "expected 200 response code, got %d", res.StatusCode)
			}

			// Proxy in non-watched namespace should not be handled
			deployEchoServer(f.T(), f.Client, nonWatchedNS, "echo")
			p := newEchoProxy("proxy", nonWatchedNS)
			err := f.CreateHTTPProxy(p)
			require.NoError(f.T(), err, "could not create httpproxy")
			require.Never(f.T(), func() bool {
				res := &contour_v1.HTTPProxy{}
				if err := f.Client.Get(context.TODO(), client.ObjectKeyFromObject(p), res); err != nil {
					return false
				}
				return !e2e.HTTPProxyNotReconciled(res)
			}, 10*time.Second, time.Second, "expected HTTPProxy to have status NotReconciled")
		})
	}
}

func testWatchAndRootNamespaces(rootNamespaces []string, nonRootNamespace string) e2e.NamespacedTestBody {
	return func(nonWatchedNS string) {
		Specify("root HTTPProxies outside of root namespaces are not configured", func() {
			// Root proxy in root namespace should succeed
			for _, ns := range rootNamespaces {
				deployEchoServer(f.T(), f.Client, ns, "echo")
				p := newEchoProxy("root-proxy", ns)
				require.True(f.T(), f.CreateHTTPProxyAndWaitFor(p, e2e.HTTPProxyValid))

				res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
					Host:      p.Spec.VirtualHost.Fqdn,
					Condition: e2e.HasStatusCode(200),
				})
				require.NotNil(f.T(), res)
				require.Truef(f.T(), ok, "expected 200 response code, got %d", res.StatusCode)
			}

			deployEchoServer(f.T(), f.Client, nonRootNamespace, "echo")

			// Root proxy in non-root namespace should fail
			p := newEchoProxy("root-proxy", nonRootNamespace)
			require.True(f.T(), f.CreateHTTPProxyAndWaitFor(p, httpProxyRootNotAllowedInNS), "expected HTTPProxy to have status RootNamespaceError")

			// Leaf proxy in non-root (but watched) namespace should succeed
			lp := &contour_v1.HTTPProxy{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: nonRootNamespace,
					Name:      "leaf-proxy",
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
			p = &contour_v1.HTTPProxy{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: rootNamespaces[0],
					Name:      "root",
				},
				Spec: contour_v1.HTTPProxySpec{
					VirtualHost: &contour_v1.VirtualHost{
						Fqdn: "root-" + rootNamespaces[0] + ".projectcontour.io",
					},
					Includes: []contour_v1.Include{
						{
							Name:      "leaf-proxy",
							Namespace: nonRootNamespace,
						},
					},
				},
			}
			err := f.CreateHTTPProxy(lp)
			require.NoError(f.T(), err, "could not create leaf httpproxy")
			require.True(f.T(), f.CreateHTTPProxyAndWaitFor(p, e2e.HTTPProxyValid))

			res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
				Host:      p.Spec.VirtualHost.Fqdn,
				Condition: e2e.HasStatusCode(200),
			})
			require.NotNil(f.T(), res)
			require.Truef(f.T(), ok, "expected 200 response code, got %d", res.StatusCode)

			// Root proxy in non-watched namespace should fail
			deployEchoServer(f.T(), f.Client, nonWatchedNS, "echo")
			p = newEchoProxy("root-proxy", nonWatchedNS)
			err = f.CreateHTTPProxy(p)
			require.NoError(f.T(), err, "could not create httpproxy")
			require.Never(f.T(), func() bool {
				res := &contour_v1.HTTPProxy{}
				if err := f.Client.Get(context.TODO(), client.ObjectKeyFromObject(p), res); err != nil {
					return false
				}
				return !e2e.HTTPProxyNotReconciled(res)
			}, 10*time.Second, time.Second, "expected HTTPProxy to have status NotReconciled")
		})
	}
}

func testRootNamespaces(namespaces []string) e2e.NamespacedTestBody {
	return func(nonrootNS string) {
		Specify("root HTTPProxies outside of root namespaces are not configured", func() {
			// Root proxy in root namespace should succeed
			for _, ns := range namespaces {
				deployEchoServer(f.T(), f.Client, ns, "echo")
				p := newEchoProxy("root-proxy", ns)
				require.True(f.T(), f.CreateHTTPProxyAndWaitFor(p, e2e.HTTPProxyValid))

				res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
					Host:      p.Spec.VirtualHost.Fqdn,
					Condition: e2e.HasStatusCode(200),
				})
				require.NotNil(f.T(), res)
				require.Truef(f.T(), ok, "expected 200 response code, got %d", res.StatusCode)
			}

			// Root proxy in non-root namespace should fail
			deployEchoServer(f.T(), f.Client, nonrootNS, "echo")
			p := newEchoProxy("root-proxy", nonrootNS)
			require.True(f.T(), f.CreateHTTPProxyAndWaitFor(p, httpProxyRootNotAllowedInNS), "expected HTTPProxy to have status RootNamespaceError")
		})
	}
}

func httpProxyRootNotAllowedInNS(proxy *contour_v1.HTTPProxy) bool {
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

func newEchoProxy(name, namespace string) *contour_v1.HTTPProxy {
	return &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: name + "-" + namespace + ".projectcontour.io",
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
}
