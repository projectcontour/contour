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
	contour_api_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/gatewayapi"
	"github.com/projectcontour/contour/internal/k8s"
	"github.com/projectcontour/contour/internal/ref"
	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayapi_v1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayapi_v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

var f = e2e.NewFramework(true)

func TestProvisioner(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Gateway provisioner tests")
}

var _ = BeforeSuite(func() {
	require.NoError(f.T(), f.Provisioner.EnsureResourcesForInclusterProvisioner())

	gc := &gatewayapi_v1beta1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "contour",
		},
		Spec: gatewayapi_v1beta1.GatewayClassSpec{
			ControllerName: gatewayapi_v1beta1.GatewayController("projectcontour.io/gateway-controller"),
		},
	}

	runtimeSettings := contourDeploymentRuntimeSettings()
	// This will be non-nil if we are in an ipv6 cluster since we need to
	// set listen addresses correctly. In that case, we can forgo the
	// coverage of a GatewayClass without parameters set. For a regular
	// cluster we will still have a basic GatewayClass without any
	// parameters to ensure that case is covered.
	if runtimeSettings != nil {
		params := &contour_api_v1alpha1.ContourDeployment{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "projectcontour",
				Name:      "basic-contour",
			},
			Spec: contour_api_v1alpha1.ContourDeploymentSpec{
				RuntimeSettings: runtimeSettings,
			},
		}
		require.NoError(f.T(), f.Client.Create(context.Background(), params))

		gc.Spec.ParametersRef = &gatewayapi_v1beta1.ParametersReference{
			Group:     "projectcontour.io",
			Kind:      "ContourDeployment",
			Namespace: ref.To(gatewayapi_v1beta1.Namespace(params.Namespace)),
			Name:      params.Name,
		}
	}

	_, ok := f.CreateGatewayClassAndWaitFor(gc, e2e.GatewayClassAccepted)
	require.True(f.T(), ok)

	paramsEnvoyDeployment := &contour_api_v1alpha1.ContourDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "projectcontour",
			Name:      "contour-with-envoy-deployment",
		},
		Spec: contour_api_v1alpha1.ContourDeploymentSpec{
			Envoy: &contour_api_v1alpha1.EnvoySettings{
				WorkloadType: contour_api_v1alpha1.WorkloadTypeDeployment,
			},
			RuntimeSettings: runtimeSettings,
		},
	}
	require.NoError(f.T(), f.Client.Create(context.Background(), paramsEnvoyDeployment))

	gcWithEnvoyDeployment := &gatewayapi_v1beta1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "contour-with-envoy-deployment",
		},
		Spec: gatewayapi_v1beta1.GatewayClassSpec{
			ControllerName: gatewayapi_v1beta1.GatewayController("projectcontour.io/gateway-controller"),
			ParametersRef: &gatewayapi_v1beta1.ParametersReference{
				Group:     "projectcontour.io",
				Kind:      "ContourDeployment",
				Namespace: ref.To(gatewayapi_v1beta1.Namespace(paramsEnvoyDeployment.Namespace)),
				Name:      paramsEnvoyDeployment.Name,
			},
		},
	}
	_, ok = f.CreateGatewayClassAndWaitFor(gcWithEnvoyDeployment, e2e.GatewayClassAccepted)
	require.True(f.T(), ok)
})

var _ = AfterSuite(func() {
	// Delete resources individually instead of deleting the entire contour
	// namespace as a performance optimization, because deleting non-empty
	// namespaces can take up to a couple minutes to complete.
	require.NoError(f.T(), f.Provisioner.DeleteResourcesForInclusterProvisioner())

	for _, name := range []string{"contour", "contour-with-envoy-deployment"} {
		gc := &gatewayapi_v1beta1.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		}
		require.NoError(f.T(), f.DeleteGatewayClass(gc, false))
	}

	// No need to delete the ContourDeployment resource explicitly, it was
	// in the projectcontour namespace which has already been deleted.
})

var _ = Describe("Gateway provisioner", func() {
	f.NamespacedTest("provisioner-gatewayclass-params", func(namespace string) {
		Specify("GatewayClass parameters are handled correctly", func() {
			// Create GatewayClass with a reference to a nonexistent ContourDeployment,
			// it should be set to "Accepted: false" since the ref is invalid.
			gatewayClass := &gatewayapi_v1beta1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "contour-with-params",
				},
				Spec: gatewayapi_v1beta1.GatewayClassSpec{
					ControllerName: gatewayapi_v1beta1.GatewayController("projectcontour.io/gateway-controller"),
					ParametersRef: &gatewayapi_v1beta1.ParametersReference{
						Group:     "projectcontour.io",
						Kind:      "ContourDeployment",
						Namespace: ref.To(gatewayapi_v1beta1.Namespace(namespace)),
						Name:      "contour-params",
					},
				},
			}
			_, ok := f.CreateGatewayClassAndWaitFor(gatewayClass, e2e.GatewayClassNotAccepted)
			require.True(f.T(), ok)

			// Create a Gateway using that GatewayClass, it should not be accepted
			// since the GatewayClass is not accepted.
			gateway := &gatewayapi_v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "http",
					Namespace: namespace,
				},
				Spec: gatewayapi_v1beta1.GatewaySpec{
					GatewayClassName: gatewayapi_v1beta1.ObjectName("contour-with-params"),
					Listeners: []gatewayapi_v1beta1.Listener{
						{
							Name:     "http",
							Protocol: gatewayapi_v1.HTTPProtocolType,
							Port:     gatewayapi_v1beta1.PortNumber(80),
							AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
								Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
									From: ref.To(gatewayapi_v1.NamespacesFromSame),
								},
							},
						},
					},
				},
			}
			require.NoError(f.T(), f.Client.Create(context.Background(), gateway))

			require.Never(f.T(), func() bool {
				gw := &gatewayapi_v1beta1.Gateway{}
				if err := f.Client.Get(context.Background(), k8s.NamespacedNameOf(gateway), gw); err != nil {
					return false
				}

				return e2e.GatewayAccepted(gw)
			}, 10*time.Second, time.Second)

			// Now create the ContourDeployment to match the parametersRef.
			params := &contour_api_v1alpha1.ContourDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      "contour-params",
				},
				Spec: contour_api_v1alpha1.ContourDeploymentSpec{
					RuntimeSettings: contourDeploymentRuntimeSettings(),
				},
			}
			require.NoError(f.T(), f.Client.Create(context.Background(), params))

			// Now the GatewayClass should be accepted.
			require.Eventually(f.T(), func() bool {
				gc := &gatewayapi_v1beta1.GatewayClass{}
				if err := f.Client.Get(context.Background(), k8s.NamespacedNameOf(gatewayClass), gc); err != nil {
					return false
				}

				return e2e.GatewayClassAccepted(gc)
			}, time.Minute, time.Second)

			// And now the Gateway should be accepted.
			require.Eventually(f.T(), func() bool {
				gw := &gatewayapi_v1beta1.Gateway{}
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
			gateway := &gatewayapi_v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "http",
					Namespace: namespace,
				},
				Spec: gatewayapi_v1beta1.GatewaySpec{
					GatewayClassName: gatewayapi_v1beta1.ObjectName("contour-with-envoy-deployment"),
					Listeners: []gatewayapi_v1beta1.Listener{
						{
							Name:     "http",
							Protocol: gatewayapi_v1.HTTPProtocolType,
							Port:     gatewayapi_v1beta1.PortNumber(80),
							AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
								Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
									From: ref.To(gatewayapi_v1.NamespacesFromSame),
								},
							},
						},
					},
				},
			}

			gateway, ok := f.CreateGatewayAndWaitFor(gateway, func(gw *gatewayapi_v1beta1.Gateway) bool {
				return e2e.GatewayProgrammed(gw) && e2e.GatewayHasAddress(gw)
			})
			require.True(f.T(), ok)

			f.Fixtures.Echo.Deploy(namespace, "echo")

			route := &gatewayapi_v1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      "httproute-1",
				},
				Spec: gatewayapi_v1beta1.HTTPRouteSpec{
					Hostnames: []gatewayapi_v1beta1.Hostname{"provisioner.projectcontour.io"},
					CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1beta1.ParentReference{
							gatewayapi.GatewayParentRef("", gateway.Name),
						},
					},
					Rules: []gatewayapi_v1beta1.HTTPRouteRule{
						{
							Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/prefix"),
							BackendRefs: gatewayapi.HTTPBackendRef("echo", 80, 1),
						},
					},
				},
			}
			_, ok = f.CreateHTTPRouteAndWaitFor(route, e2e.HTTPRouteAccepted)
			require.True(f.T(), ok)

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

			gateway := &gatewayapi_v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "many-listeners",
					Namespace: namespace,
				},
				Spec: gatewayapi_v1beta1.GatewaySpec{
					GatewayClassName: "contour",
					Listeners: []gatewayapi_v1beta1.Listener{
						{
							Name:     "http-1",
							Protocol: gatewayapi_v1.HTTPProtocolType,
							Port:     80,
							Hostname: ref.To(gatewayapi_v1beta1.Hostname("http-1.provisioner.projectcontour.io")),
						},
						{
							Name:     "http-2",
							Protocol: gatewayapi_v1.HTTPProtocolType,
							Port:     81,
							Hostname: ref.To(gatewayapi_v1beta1.Hostname("http-2.provisioner.projectcontour.io")),
						},
						{
							Name:     "https-1",
							Protocol: gatewayapi_v1.HTTPSProtocolType,
							Port:     443,
							Hostname: ref.To(gatewayapi_v1beta1.Hostname("https-1.provisioner.projectcontour.io")),
							TLS: &gatewayapi_v1beta1.GatewayTLSConfig{
								Mode: ref.To(gatewayapi_v1.TLSModeTerminate),
								CertificateRefs: []gatewayapi_v1beta1.SecretObjectReference{
									{Name: "https-1-cert"},
								},
							},
						},
						{
							Name:     "https-2",
							Protocol: gatewayapi_v1.HTTPSProtocolType,
							Port:     444,
							Hostname: ref.To(gatewayapi_v1beta1.Hostname("https-2.provisioner.projectcontour.io")),
							TLS: &gatewayapi_v1beta1.GatewayTLSConfig{
								Mode: ref.To(gatewayapi_v1.TLSModeTerminate),
								CertificateRefs: []gatewayapi_v1beta1.SecretObjectReference{
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

			gateway, ok := f.CreateGatewayAndWaitFor(gateway, func(gw *gatewayapi_v1beta1.Gateway) bool {
				if !(e2e.GatewayProgrammed(gw) && e2e.GatewayHasAddress(gw)) {
					return false
				}

				for _, listener := range gw.Spec.Listeners {
					if !e2e.ListenerAccepted(gw, listener.Name) {
						return false
					}
				}

				return true
			})
			require.True(f.T(), ok)

			f.Fixtures.Echo.Deploy(namespace, "echo")

			// This HTTPRoute will attach to all of the HTTP and HTTPS Listeners.
			httpRoute := &gatewayapi_v1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      "httproute-1",
				},
				Spec: gatewayapi_v1beta1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1beta1.ParentReference{
							gatewayapi.GatewayParentRef("", gateway.Name),
						},
					},
					Rules: []gatewayapi_v1beta1.HTTPRouteRule{
						{
							Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
							BackendRefs: gatewayapi.HTTPBackendRef("echo", 80, 1),
						},
					},
				},
			}
			_, ok = f.CreateHTTPRouteAndWaitFor(httpRoute, e2e.HTTPRouteAccepted)
			require.True(f.T(), ok)

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
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      "tcproute-1",
				},
				Spec: gatewayapi_v1alpha2.TCPRouteSpec{
					CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1alpha2.ParentReference{
							{
								Namespace: ref.To(gatewayapi_v1beta1.Namespace(gateway.Namespace)),
								Name:      gatewayapi_v1beta1.ObjectName(gateway.Name),
							},
						},
					},
					Rules: []gatewayapi_v1alpha2.TCPRouteRule{
						{
							BackendRefs: gatewayapi.TLSRouteBackendRef("echo", 80, ref.To(int32(1))),
						},
					},
				},
			}
			_, ok = f.CreateTCPRouteAndWaitFor(tcpRoute, e2e.TCPRouteAccepted)
			require.True(f.T(), ok)

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
})

func contourDeploymentRuntimeSettings() *contour_api_v1alpha1.ContourConfigurationSpec {
	if os.Getenv("IPV6_CLUSTER") != "true" {
		return nil
	}

	return &contour_api_v1alpha1.ContourConfigurationSpec{
		XDSServer: &contour_api_v1alpha1.XDSServerConfig{
			Address: "::",
		},
		Debug: &contour_api_v1alpha1.DebugConfig{
			Address: "::1",
		},
		Health: &contour_api_v1alpha1.HealthConfig{
			Address: "::",
		},
		Metrics: &contour_api_v1alpha1.MetricsConfig{
			Address: "::",
		},
		Envoy: &contour_api_v1alpha1.EnvoyConfig{
			HTTPListener: &contour_api_v1alpha1.EnvoyListener{
				Address: "::",
			},
			HTTPSListener: &contour_api_v1alpha1.EnvoyListener{
				Address: "::",
			},
			Health: &contour_api_v1alpha1.HealthConfig{
				Address: "::",
			},
			Metrics: &contour_api_v1alpha1.MetricsConfig{
				Address: "::",
			},
		},
	}
}
