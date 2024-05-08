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

package provisioner

import (
	"context"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayapi_v1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/gatewayapi"
	"github.com/projectcontour/contour/internal/k8s"
	"github.com/projectcontour/contour/test/e2e"
)

var f = e2e.NewFramework(true)

func TestProvisioner(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Gateway provisioner tests")
}

var _ = BeforeSuite(func() {
	require.NoError(f.T(), f.Provisioner.EnsureResourcesForInclusterProvisioner())

	gc := &gatewayapi_v1.GatewayClass{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "contour",
		},
		Spec: gatewayapi_v1.GatewayClassSpec{
			ControllerName: gatewayapi_v1.GatewayController("projectcontour.io/gateway-controller"),
		},
	}

	runtimeSettings := contourDeploymentRuntimeSettings()
	// This will be non-nil if we are in an ipv6 cluster since we need to
	// set listen addresses correctly. In that case, we can forgo the
	// coverage of a GatewayClass without parameters set. For a regular
	// cluster we will still have a basic GatewayClass without any
	// parameters to ensure that case is covered.
	if runtimeSettings != nil {
		params := &contour_v1alpha1.ContourDeployment{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: "projectcontour",
				Name:      "basic-contour",
			},
			Spec: contour_v1alpha1.ContourDeploymentSpec{
				RuntimeSettings: runtimeSettings,
			},
		}
		require.NoError(f.T(), f.Client.Create(context.Background(), params))

		gc.Spec.ParametersRef = &gatewayapi_v1.ParametersReference{
			Group:     "projectcontour.io",
			Kind:      "ContourDeployment",
			Namespace: ptr.To(gatewayapi_v1.Namespace(params.Namespace)),
			Name:      params.Name,
		}
	}

	require.True(f.T(), f.CreateGatewayClassAndWaitFor(gc, e2e.GatewayClassAccepted))

	paramsEnvoyDeployment := &contour_v1alpha1.ContourDeployment{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "projectcontour",
			Name:      "contour-with-envoy-deployment",
		},
		Spec: contour_v1alpha1.ContourDeploymentSpec{
			Envoy: &contour_v1alpha1.EnvoySettings{
				WorkloadType: contour_v1alpha1.WorkloadTypeDeployment,
			},
			RuntimeSettings: runtimeSettings,
		},
	}
	require.NoError(f.T(), f.Client.Create(context.Background(), paramsEnvoyDeployment))

	gcWithEnvoyDeployment := &gatewayapi_v1.GatewayClass{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "contour-with-envoy-deployment",
		},
		Spec: gatewayapi_v1.GatewayClassSpec{
			ControllerName: gatewayapi_v1.GatewayController("projectcontour.io/gateway-controller"),
			ParametersRef: &gatewayapi_v1.ParametersReference{
				Group:     "projectcontour.io",
				Kind:      "ContourDeployment",
				Namespace: ptr.To(gatewayapi_v1.Namespace(paramsEnvoyDeployment.Namespace)),
				Name:      paramsEnvoyDeployment.Name,
			},
		},
	}
	require.True(f.T(), f.CreateGatewayClassAndWaitFor(gcWithEnvoyDeployment, e2e.GatewayClassAccepted))
})

var _ = AfterSuite(func() {
	// Delete resources individually instead of deleting the entire contour
	// namespace as a performance optimization, because deleting non-empty
	// namespaces can take up to a couple minutes to complete.
	require.NoError(f.T(), f.Provisioner.DeleteResourcesForInclusterProvisioner())

	for _, name := range []string{"contour", "contour-with-envoy-deployment"} {
		gc := &gatewayapi_v1.GatewayClass{
			ObjectMeta: meta_v1.ObjectMeta{
				Name: name,
			},
		}
		require.NoError(f.T(), f.DeleteGatewayClass(gc, false))
	}

	// No need to delete the ContourDeployment resource explicitly, it was
	// in the projectcontour namespace which has already been deleted.
})

var _ = Describe("Gateway provisioner", func() {
	Specify("GatewayClass status condition SupportedVersion is set to True", func() {
		// This test will fail if we bump the Gateway API module and CRDs but
		// forget to update the supported version we check for.
		require.Eventually(f.T(), func() bool {
			gc := &gatewayapi_v1.GatewayClass{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: "contour",
				},
			}
			if err := f.Client.Get(context.TODO(), client.ObjectKeyFromObject(gc), gc); err != nil {
				return false
			}
			for _, cond := range gc.Status.Conditions {
				if cond.Type == string(gatewayapi_v1.GatewayClassConditionStatusSupportedVersion) &&
					cond.Status == meta_v1.ConditionTrue {
					return true
				}
			}
			return false
		}, f.RetryTimeout, f.RetryInterval)
	})

	f.NamespacedTest("provisioner-gatewayclass-params", func(namespace string) {
		Specify("GatewayClass parameters are handled correctly", func() {
			// Create GatewayClass with a reference to a nonexistent ContourDeployment,
			// it should be set to "Accepted: false" since the ref is invalid.
			gatewayClass := &gatewayapi_v1.GatewayClass{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: "contour-with-params",
				},
				Spec: gatewayapi_v1.GatewayClassSpec{
					ControllerName: gatewayapi_v1.GatewayController("projectcontour.io/gateway-controller"),
					ParametersRef: &gatewayapi_v1.ParametersReference{
						Group:     "projectcontour.io",
						Kind:      "ContourDeployment",
						Namespace: ptr.To(gatewayapi_v1.Namespace(namespace)),
						Name:      "contour-params",
					},
				},
			}
			require.True(f.T(), f.CreateGatewayClassAndWaitFor(gatewayClass, e2e.GatewayClassNotAccepted))

			// Create a Gateway using that GatewayClass, it should not be accepted
			// since the GatewayClass is not accepted.
			gateway := &gatewayapi_v1.Gateway{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "http",
					Namespace: namespace,
				},
				Spec: gatewayapi_v1.GatewaySpec{
					GatewayClassName: gatewayapi_v1.ObjectName("contour-with-params"),
					Listeners: []gatewayapi_v1.Listener{
						{
							Name:     "http",
							Protocol: gatewayapi_v1.HTTPProtocolType,
							Port:     gatewayapi_v1.PortNumber(80),
							AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
								Namespaces: &gatewayapi_v1.RouteNamespaces{
									From: ptr.To(gatewayapi_v1.NamespacesFromSame),
								},
							},
						},
					},
				},
			}
			require.NoError(f.T(), f.Client.Create(context.Background(), gateway))

			require.Never(f.T(), func() bool {
				gw := &gatewayapi_v1.Gateway{}
				if err := f.Client.Get(context.Background(), k8s.NamespacedNameOf(gateway), gw); err != nil {
					return false
				}

				return e2e.GatewayAccepted(gw)
			}, 10*time.Second, time.Second)

			// Now create the ContourDeployment to match the parametersRef.
			params := &contour_v1alpha1.ContourDeployment{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: namespace,
					Name:      "contour-params",
				},
				Spec: contour_v1alpha1.ContourDeploymentSpec{
					RuntimeSettings: contourDeploymentRuntimeSettings(),
				},
			}
			require.NoError(f.T(), f.Client.Create(context.Background(), params))

			// Now the GatewayClass should be accepted.
			require.Eventually(f.T(), func() bool {
				gc := &gatewayapi_v1.GatewayClass{}
				if err := f.Client.Get(context.Background(), k8s.NamespacedNameOf(gatewayClass), gc); err != nil {
					return false
				}

				return e2e.GatewayClassAccepted(gc)
			}, time.Minute, time.Second)

			// And now the Gateway should be accepted.
			require.Eventually(f.T(), func() bool {
				gw := &gatewayapi_v1.Gateway{}
				if err := f.Client.Get(context.Background(), k8s.NamespacedNameOf(gateway), gw); err != nil {
					return false
				}

				return e2e.GatewayAccepted(gw)
			}, time.Minute, time.Second)

			require.NoError(f.T(), f.DeleteGatewayClass(gatewayClass, false))
		})
	})

	f.NamespacedTest("gateway-with-envoy-deployment", func(namespace string) {
		Specify("A gateway with Envoy as a deployment can be provisioned and routes traffic correctly", func() {
			gateway := &gatewayapi_v1.Gateway{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "http",
					Namespace: namespace,
				},
				Spec: gatewayapi_v1.GatewaySpec{
					GatewayClassName: gatewayapi_v1.ObjectName("contour-with-envoy-deployment"),
					Listeners: []gatewayapi_v1.Listener{
						{
							Name:     "http",
							Protocol: gatewayapi_v1.HTTPProtocolType,
							Port:     gatewayapi_v1.PortNumber(80),
							AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
								Namespaces: &gatewayapi_v1.RouteNamespaces{
									From: ptr.To(gatewayapi_v1.NamespacesFromSame),
								},
							},
						},
					},
				},
			}

			require.True(f.T(), f.CreateGatewayAndWaitFor(gateway, func(gw *gatewayapi_v1.Gateway) bool {
				return e2e.GatewayProgrammed(gw) && e2e.GatewayHasAddress(gw)
			}))

			f.Fixtures.Echo.Deploy(namespace, "echo")

			route := &gatewayapi_v1.HTTPRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: namespace,
					Name:      "httproute-1",
				},
				Spec: gatewayapi_v1.HTTPRouteSpec{
					Hostnames: []gatewayapi_v1.Hostname{"provisioner.projectcontour.io"},
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{
							gatewayapi.GatewayParentRef("", gateway.Name),
						},
					},
					Rules: []gatewayapi_v1.HTTPRouteRule{
						{
							Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/prefix"),
							BackendRefs: gatewayapi.HTTPBackendRef("echo", 80, 1),
						},
					},
				},
			}
			require.True(f.T(), f.CreateHTTPRouteAndWaitFor(route, e2e.HTTPRouteAccepted))

			res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
				OverrideURL: "http://" + net.JoinHostPort(gateway.Status.Addresses[0].Value, "80"),
				Host:        string(route.Spec.Hostnames[0]),
				Path:        "/prefix/match",
				Condition:   e2e.HasStatusCode(200),
			})
			require.NotNil(f.T(), res)
			require.Truef(f.T(), ok, "expected 200 response code, got %d", res.StatusCode)

			body := f.GetEchoResponseBody(res.Body)
			assert.Equal(f.T(), namespace, body.Namespace)
			assert.Equal(f.T(), "echo", body.Service)
		})
	})

	f.NamespacedTest("gateway-with-many-listeners", func(namespace string) {
		Specify("A gateway with many Listeners for different protocols can be provisioned and routes correctly", func() {
			f.Certs.CreateSelfSignedCert(namespace, "https-1-cert", "https-1-cert", "https-1.provisioner.projectcontour.io")
			f.Certs.CreateSelfSignedCert(namespace, "https-2-cert", "https-2-cert", "https-2.provisioner.projectcontour.io")

			gateway := &gatewayapi_v1.Gateway{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "many-listeners",
					Namespace: namespace,
				},
				Spec: gatewayapi_v1.GatewaySpec{
					GatewayClassName: "contour",
					Listeners: []gatewayapi_v1.Listener{
						{
							Name:     "http-1",
							Protocol: gatewayapi_v1.HTTPProtocolType,
							Port:     80,
							Hostname: ptr.To(gatewayapi_v1.Hostname("http-1.provisioner.projectcontour.io")),
						},
						{
							Name:     "http-2",
							Protocol: gatewayapi_v1.HTTPProtocolType,
							Port:     81,
							Hostname: ptr.To(gatewayapi_v1.Hostname("http-2.provisioner.projectcontour.io")),
						},
						{
							Name:     "https-1",
							Protocol: gatewayapi_v1.HTTPSProtocolType,
							Port:     443,
							Hostname: ptr.To(gatewayapi_v1.Hostname("https-1.provisioner.projectcontour.io")),
							TLS: &gatewayapi_v1.GatewayTLSConfig{
								Mode: ptr.To(gatewayapi_v1.TLSModeTerminate),
								CertificateRefs: []gatewayapi_v1.SecretObjectReference{
									{Name: "https-1-cert"},
								},
							},
						},
						{
							Name:     "https-2",
							Protocol: gatewayapi_v1.HTTPSProtocolType,
							Port:     444,
							Hostname: ptr.To(gatewayapi_v1.Hostname("https-2.provisioner.projectcontour.io")),
							TLS: &gatewayapi_v1.GatewayTLSConfig{
								Mode: ptr.To(gatewayapi_v1.TLSModeTerminate),
								CertificateRefs: []gatewayapi_v1.SecretObjectReference{
									{Name: "https-2-cert"},
								},
							},
						},
						{
							Name:     "tcp-1",
							Protocol: gatewayapi_v1.TCPProtocolType,
							Port:     7777,
						},
						{
							Name:     "tcp-2",
							Protocol: gatewayapi_v1.TCPProtocolType,
							Port:     8888,
						},
					},
				},
			}

			require.True(f.T(), f.CreateGatewayAndWaitFor(gateway, func(gw *gatewayapi_v1.Gateway) bool {
				if !(e2e.GatewayProgrammed(gw) && e2e.GatewayHasAddress(gw)) {
					return false
				}

				for _, listener := range gw.Spec.Listeners {
					if !e2e.ListenerAccepted(gw, listener.Name) {
						return false
					}
				}

				return true
			}))

			f.Fixtures.Echo.Deploy(namespace, "echo")

			// This HTTPRoute will attach to all of the HTTP and HTTPS Listeners.
			httpRoute := &gatewayapi_v1.HTTPRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: namespace,
					Name:      "httproute-1",
				},
				Spec: gatewayapi_v1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{
							gatewayapi.GatewayParentRef("", gateway.Name),
						},
					},
					Rules: []gatewayapi_v1.HTTPRouteRule{
						{
							Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
							BackendRefs: gatewayapi.HTTPBackendRef("echo", 80, 1),
						},
					},
				},
			}
			require.True(f.T(), f.CreateHTTPRouteAndWaitFor(httpRoute, e2e.HTTPRouteAccepted))

			for _, tc := range []struct {
				name   string
				scheme string
				port   string
			}{
				{name: "http-1", scheme: "http", port: "80"},
				{name: "http-2", scheme: "http", port: "81"},
				{name: "https-1", scheme: "https", port: "443"},
				{name: "https-2", scheme: "https", port: "444"},
			} {
				var res *e2e.HTTPResponse
				var ok bool

				switch tc.scheme {
				case "http":
					res, ok = f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
						OverrideURL: fmt.Sprintf("%s://%s", tc.scheme, net.JoinHostPort(gateway.Status.Addresses[0].Value, tc.port)),
						Host:        fmt.Sprintf("%s.provisioner.projectcontour.io", tc.name),
						Condition:   e2e.HasStatusCode(200),
					})
				case "https":
					res, ok = f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
						OverrideURL: fmt.Sprintf("%s://%s", tc.scheme, net.JoinHostPort(gateway.Status.Addresses[0].Value, tc.port)),
						Host:        fmt.Sprintf("%s.provisioner.projectcontour.io", tc.name),
						Condition:   e2e.HasStatusCode(200),
					})
				default:
					f.T().Fatal("invalid scheme")
				}

				require.NotNil(f.T(), res)
				require.Truef(f.T(), ok, "expected 200 response code, got %d", res.StatusCode)

				body := f.GetEchoResponseBody(res.Body)
				assert.Equal(f.T(), namespace, body.Namespace)
				assert.Equal(f.T(), "echo", body.Service)
			}

			// This TCPRoute will attach to both TCP Listeners.
			tcpRoute := &gatewayapi_v1alpha2.TCPRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: namespace,
					Name:      "tcproute-1",
				},
				Spec: gatewayapi_v1alpha2.TCPRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{
							{
								Namespace: ptr.To(gatewayapi_v1.Namespace(gateway.Namespace)),
								Name:      gatewayapi_v1.ObjectName(gateway.Name),
							},
						},
					},
					Rules: []gatewayapi_v1alpha2.TCPRouteRule{
						{
							BackendRefs: gatewayapi.TLSRouteBackendRef("echo", 80, ptr.To(int32(1))),
						},
					},
				},
			}
			require.True(f.T(), f.CreateTCPRouteAndWaitFor(tcpRoute, e2e.TCPRouteAccepted))

			for _, tc := range []struct {
				name string
				port string
			}{
				{name: "tcp-1", port: "7777"},
				{name: "tcp-2", port: "8888"},
			} {
				res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
					OverrideURL: "http://" + net.JoinHostPort(gateway.Status.Addresses[0].Value, tc.port),
					Host:        fmt.Sprintf("%s.provisioner.projectcontour.io", tc.name),
					Condition:   e2e.HasStatusCode(200),
				})
				require.NotNil(f.T(), res)
				require.Truef(f.T(), ok, "expected 200 response code, got %d", res.StatusCode)

				body := f.GetEchoResponseBody(res.Body)
				assert.Equal(f.T(), namespace, body.Namespace)
				assert.Equal(f.T(), "echo", body.Service)

				// Envoy is expected to add the "server: envoy" and
				// "x-envoy-upstream-service-time" HTTP headers when
				// proxying HTTP; this ensures we are proxying TCP only.
				assert.Equal(f.T(), "", res.Headers.Get("server"))
				assert.Equal(f.T(), "", res.Headers.Get("x-envoy-upstream-service-time"))
			}
		})
	})
	f.NamespacedTest("gateway-with-envoy-in-watch-namespaces", func(namespace string) {
		objectTestName := "contour-params-with-watch-namespaces"
		BeforeEach(func() {
			By("create gatewayclass that reference contourDeployment with watchNamespace value")
			gatewayClass := &gatewayapi_v1.GatewayClass{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: objectTestName,
				},
				Spec: gatewayapi_v1.GatewayClassSpec{
					ControllerName: gatewayapi_v1.GatewayController("projectcontour.io/gateway-controller"),
					ParametersRef: &gatewayapi_v1.ParametersReference{
						Group:     "projectcontour.io",
						Kind:      "ContourDeployment",
						Namespace: ptr.To(gatewayapi_v1.Namespace(namespace)),
						Name:      objectTestName,
					},
				},
			}
			require.True(f.T(), f.CreateGatewayClassAndWaitFor(gatewayClass, e2e.GatewayClassNotAccepted))

			// Now create the ContourDeployment to match the parametersRef.
			params := &contour_v1alpha1.ContourDeployment{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: namespace,
					Name:      objectTestName,
				},
				Spec: contour_v1alpha1.ContourDeploymentSpec{
					RuntimeSettings: contourDeploymentRuntimeSettings(),
					Contour: &contour_v1alpha1.ContourSettings{
						WatchNamespaces: []contour_v1.Namespace{"testns-1", "testns-2"},
					},
				},
			}
			require.NoError(f.T(), f.Client.Create(context.Background(), params))

			// Now the GatewayClass should be accepted.
			require.Eventually(f.T(), func() bool {
				gc := &gatewayapi_v1.GatewayClass{}
				if err := f.Client.Get(context.Background(), k8s.NamespacedNameOf(gatewayClass), gc); err != nil {
					return false
				}

				return e2e.GatewayClassAccepted(gc)
			}, time.Minute, time.Second)
		})
		AfterEach(func() {
			require.NoError(f.T(), f.DeleteGatewayClass(&gatewayapi_v1.GatewayClass{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: objectTestName,
				},
			}, false))
		})
		Specify("A gateway can be provisioned that only reconciles routes in a subset of namespaces", func() {
			By("This tests deploy 3 dev namespaces testns-1, testns-2, testns-3")
			By("Deploy gateway that referencing above gatewayclass")
			gateway := &gatewayapi_v1.Gateway{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "http-for-watchnamespaces",
					Namespace: namespace,
				},
				Spec: gatewayapi_v1.GatewaySpec{
					GatewayClassName: gatewayapi_v1.ObjectName(objectTestName),
					Listeners: []gatewayapi_v1.Listener{
						{
							Name:     "http",
							Protocol: gatewayapi_v1.HTTPProtocolType,
							Port:     gatewayapi_v1.PortNumber(80),
							AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
								Namespaces: &gatewayapi_v1.RouteNamespaces{
									// TODO: set to from all for now
									// The correct way would be label the testns-1, testns-2, testns-3, then select by label
									From: ptr.To(gatewayapi_v1.NamespacesFromAll),
								},
							},
						},
					},
				},
			}

			require.True(f.T(), f.CreateGatewayAndWaitFor(gateway, func(gw *gatewayapi_v1.Gateway) bool {
				return e2e.GatewayProgrammed(gw) && e2e.GatewayHasAddress(gw)
			}))

			type testObj struct {
				expectReconcile bool
				namespace       string
			}
			testcases := []testObj{
				{
					expectReconcile: true,
					namespace:       "testns-1",
				},
				{
					expectReconcile: true,
					namespace:       "testns-2",
				},
				{
					expectReconcile: false,
					namespace:       "testns-3",
				},
			}

			By("Deploy workload in target namespaces, check if they get reconciled or not")
			for _, t := range testcases {
				f.Fixtures.Echo.Deploy(t.namespace, "echo")

				route := &gatewayapi_v1.HTTPRoute{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace: t.namespace,
						Name:      "httproute-1",
					},
					Spec: gatewayapi_v1.HTTPRouteSpec{
						Hostnames: []gatewayapi_v1.Hostname{"provisioner.projectcontour.io"},
						CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
							ParentRefs: []gatewayapi_v1.ParentReference{
								gatewayapi.GatewayParentRef("", gateway.Name),
							},
						},
						Rules: []gatewayapi_v1.HTTPRouteRule{
							{
								Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/prefix"),
								BackendRefs: gatewayapi.HTTPBackendRef("echo", 80, 1),
							},
						},
					},
				}

				if t.expectReconcile {
					// set route's parentRef's namespace to the gateway's namespace
					route.Spec.CommonRouteSpec.ParentRefs[0].Namespace = (*gatewayapi_v1.Namespace)(&namespace)
					// set the route's hostnames to custom name with namespace inside
					route.Spec.Hostnames = []gatewayapi_v1.Hostname{gatewayapi_v1.Hostname("provisioner.projectcontour.io." + t.namespace)}

					By(fmt.Sprintf("Expect namespace %s to be watched by contour", t.namespace))
					require.True(f.T(), f.CreateHTTPRouteAndWaitFor(route, e2e.HTTPRouteAccepted))

					res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
						OverrideURL: "http://" + net.JoinHostPort(gateway.Status.Addresses[0].Value, "80"),
						Host:        string(route.Spec.Hostnames[0]),
						Path:        "/prefix/match",
						Condition:   e2e.HasStatusCode(200),
					})
					require.NotNil(f.T(), res)
					require.Truef(f.T(), ok, "expected 200 response code, got %d", res.StatusCode)

					body := f.GetEchoResponseBody(res.Body)
					assert.Equal(f.T(), t.namespace, body.Namespace)
					assert.Equal(f.T(), "echo", body.Service)
				} else {
					// Root proxy in non-watched namespace should fail
					By(fmt.Sprintf("Expect namespace %s not to be watched by contour", t.namespace))
					require.True(f.T(), f.CreateHTTPRouteAndWaitFor(route, e2e.HTTPRouteIgnoredByContour))

					By(fmt.Sprintf("Expect httproute under namespace %s is not accepted for a period of time", t.namespace))
					require.Never(f.T(), func() bool {
						hr := &gatewayapi_v1.HTTPRoute{}
						if err := f.Client.Get(context.Background(), k8s.NamespacedNameOf(route), hr); err != nil {
							return false
						}
						return e2e.HTTPRouteAccepted(hr)
					}, 20*time.Second, time.Second)
				}
			}
		})
	}, "testns-1", "testns-2", "testns-3")
	f.NamespacedTest("gateway-with-envoy-with-disabled-features", func(namespace string) {
		objectTestName := "contour-params-with-disabled-features"
		BeforeEach(func() {
			By("create gatewayclass that reference contourDeployment with disabled-features value")
			gatewayClass := &gatewayapi_v1.GatewayClass{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: objectTestName,
				},
				Spec: gatewayapi_v1.GatewayClassSpec{
					ControllerName: gatewayapi_v1.GatewayController("projectcontour.io/gateway-controller"),
					ParametersRef: &gatewayapi_v1.ParametersReference{
						Group:     "projectcontour.io",
						Kind:      "ContourDeployment",
						Namespace: ptr.To(gatewayapi_v1.Namespace(namespace)),
						Name:      objectTestName,
					},
				},
			}
			require.True(f.T(), f.CreateGatewayClassAndWaitFor(gatewayClass, e2e.GatewayClassNotAccepted))

			// Now create the ContourDeployment to match the parametersRef.
			params := &contour_v1alpha1.ContourDeployment{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: namespace,
					Name:      objectTestName,
				},
				Spec: contour_v1alpha1.ContourDeploymentSpec{
					RuntimeSettings: contourDeploymentRuntimeSettings(),
					Contour: &contour_v1alpha1.ContourSettings{
						DisabledFeatures: []contour_v1.Feature{"tlsroutes"},
					},
				},
			}
			require.NoError(f.T(), f.Client.Create(context.Background(), params))

			// Now the GatewayClass should be accepted.
			require.Eventually(f.T(), func() bool {
				gc := &gatewayapi_v1.GatewayClass{}
				if err := f.Client.Get(context.Background(), k8s.NamespacedNameOf(gatewayClass), gc); err != nil {
					return false
				}

				return e2e.GatewayClassAccepted(gc)
			}, time.Minute, time.Second)
		})
		AfterEach(func() {
			require.NoError(f.T(), f.DeleteGatewayClass(&gatewayapi_v1.GatewayClass{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: objectTestName,
				},
			}, false))
		})
		Specify("A gateway can be provisioned that ignore CRDs in disabledFeatures", func() {
			By("Deploy gateway that referencing above gatewayclass")
			gateway := &gatewayapi_v1.Gateway{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "tlsroute",
					Namespace: namespace,
				},
				Spec: gatewayapi_v1.GatewaySpec{
					GatewayClassName: gatewayapi_v1.ObjectName(objectTestName),
					Listeners: []gatewayapi_v1.Listener{
						{
							Name:     "https",
							Protocol: gatewayapi_v1.TLSProtocolType,
							Port:     gatewayapi_v1.PortNumber(443),
							TLS: &gatewayapi_v1.GatewayTLSConfig{
								Mode: ptr.To(gatewayapi_v1.TLSModePassthrough),
							},
							AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
								Namespaces: &gatewayapi_v1.RouteNamespaces{
									From: ptr.To(gatewayapi_v1.NamespacesFromSame),
								},
							},
						},
					},
				},
			}

			require.True(f.T(), f.CreateGatewayAndWaitFor(gateway, func(gw *gatewayapi_v1.Gateway) bool {
				return e2e.GatewayProgrammed(gw) && e2e.GatewayHasAddress(gw)
			}))

			By("Skip reconciling the TLSRoute if disabledFeatures includes it")
			f.Fixtures.EchoSecure.Deploy(namespace, "echo-secure", nil)
			route := &gatewayapi_v1alpha2.TLSRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: namespace,
					Name:      "tlsroute-1",
				},
				Spec: gatewayapi_v1alpha2.TLSRouteSpec{
					Hostnames: []gatewayapi_v1.Hostname{"provisioner.projectcontour.io"},
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{
							{
								Namespace: ptr.To(gatewayapi_v1.Namespace(gateway.Namespace)),
								Name:      gatewayapi_v1.ObjectName(gateway.Name),
							},
						},
					},
					Rules: []gatewayapi_v1alpha2.TLSRouteRule{
						{
							BackendRefs: gatewayapi.TLSRouteBackendRef("echo-secure", 443, ptr.To(int32(1))),
						},
					},
				},
			}
			require.True(f.T(), f.CreateTLSRouteAndWaitFor(route, e2e.TLSRouteIgnoredByContour))

			By("Expect tlsroute not to be accepted")
			require.Never(f.T(), func() bool {
				tr := &gatewayapi_v1alpha2.TLSRoute{}
				if err := f.Client.Get(context.Background(), k8s.NamespacedNameOf(route), tr); err != nil {
					return false
				}
				return e2e.TLSRouteAccepted(tr)
			}, 20*time.Second, time.Second)
		})
	})
})

func contourDeploymentRuntimeSettings() *contour_v1alpha1.ContourConfigurationSpec {
	if os.Getenv("IPV6_CLUSTER") != "true" {
		return nil
	}

	return &contour_v1alpha1.ContourConfigurationSpec{
		XDSServer: &contour_v1alpha1.XDSServerConfig{
			Address: "::",
		},
		Debug: &contour_v1alpha1.DebugConfig{
			Address: "::1",
		},
		Health: &contour_v1alpha1.HealthConfig{
			Address: "::",
		},
		Metrics: &contour_v1alpha1.MetricsConfig{
			Address: "::",
		},
		Envoy: &contour_v1alpha1.EnvoyConfig{
			HTTPListener: &contour_v1alpha1.EnvoyListener{
				Address: "::",
			},
			HTTPSListener: &contour_v1alpha1.EnvoyListener{
				Address: "::",
			},
			Health: &contour_v1alpha1.HealthConfig{
				Address: "::",
			},
			Metrics: &contour_v1alpha1.MetricsConfig{
				Address: "::",
			},
		},
	}
}
