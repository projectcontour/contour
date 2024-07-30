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
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apps_v1 "k8s.io/api/apps/v1"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/test/e2e"
)

func testExternalAuth(namespace string) {
	Specify("external auth can be configured on an HTTPRoute", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "echo")
		f.Certs.CreateSelfSignedCert(namespace, "echo", "echo", "externalauth.projectcontour.io")

		f.Certs.CreateSelfSignedCert(namespace, "testserver-cert", "testserver-cert", "testserver")

		// auth testserver
		deployment := &apps_v1.Deployment{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "testserver",
				Labels: map[string]string{
					"app.kubernetes.io/name": "testserver",
				},
			},
			Spec: apps_v1.DeploymentSpec{
				Selector: &meta_v1.LabelSelector{
					MatchLabels: map[string]string{"app.kubernetes.io/name": "testserver"},
				},
				Template: core_v1.PodTemplateSpec{
					ObjectMeta: meta_v1.ObjectMeta{
						Labels: map[string]string{"app.kubernetes.io/name": "testserver"},
					},
					Spec: core_v1.PodSpec{
						Containers: []core_v1.Container{
							{
								Name:            "testserver",
								Image:           "ghcr.io/projectcontour/contour-authserver:v4",
								ImagePullPolicy: core_v1.PullIfNotPresent,
								Command: []string{
									"/contour-authserver",
								},
								Args: []string{
									"testserver",
									"--address=:9443",
									"--tls-ca-path=/tls/ca.crt",
									"--tls-cert-path=/tls/tls.crt",
									"--tls-key-path=/tls/tls.key",
								},
								Ports: []core_v1.ContainerPort{
									{
										Name:          "auth",
										ContainerPort: 9443,
										Protocol:      core_v1.ProtocolTCP,
									},
								},
								VolumeMounts: []core_v1.VolumeMount{
									{
										Name:      "tls",
										MountPath: "/tls",
										ReadOnly:  true,
									},
								},
							},
						},
						Volumes: []core_v1.Volume{
							{
								Name: "tls",
								VolumeSource: core_v1.VolumeSource{
									Secret: &core_v1.SecretVolumeSource{
										SecretName: "testserver-cert",
									},
								},
							},
						},
					},
				},
			},
		}
		require.NoError(t, f.Client.Create(context.TODO(), deployment))

		svc := &core_v1.Service{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      "testserver",
				Namespace: namespace,
				Labels: map[string]string{
					"app.kubernetes.io/name": "testserver",
				},
			},
			Spec: core_v1.ServiceSpec{
				Ports: []core_v1.ServicePort{
					{
						Name:       "auth",
						Protocol:   core_v1.ProtocolTCP,
						Port:       9443,
						TargetPort: intstr.FromInt(9443),
					},
				},
				Selector: map[string]string{
					"app.kubernetes.io/name": "testserver",
				},
				Type: core_v1.ServiceTypeClusterIP,
			},
		}
		require.NoError(t, f.Client.Create(context.TODO(), svc))

		extSvc := &contour_v1alpha1.ExtensionService{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      "testserver",
				Namespace: namespace,
			},
			Spec: contour_v1alpha1.ExtensionServiceSpec{
				Services: []contour_v1alpha1.ExtensionServiceTarget{
					{
						Name: "testserver",
						Port: 9443,
					},
				},
			},
		}
		require.NoError(t, f.Client.Create(context.TODO(), extSvc))

		p := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "external-auth",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "externalauth.projectcontour.io",
					TLS: &contour_v1.TLS{
						SecretName: "echo",
					},
					Authorization: &contour_v1.AuthorizationServer{
						ResponseTimeout: "500ms",
						ExtensionServiceRef: contour_v1.ExtensionServiceReference{
							Name:      extSvc.Name,
							Namespace: extSvc.Namespace,
						},
						AuthPolicy: &contour_v1.AuthorizationPolicy{
							Context: map[string]string{
								"hostname": "externalauth.projectcontour.io",
							},
						},
					},
				},
				Routes: []contour_v1.Route{
					{
						Conditions: []contour_v1.MatchCondition{
							{
								Prefix: "/first",
							},
						},
						AuthPolicy: &contour_v1.AuthorizationPolicy{
							Context: map[string]string{
								"target": "first",
							},
						},
						Services: []contour_v1.Service{
							{
								Name: "echo",
								Port: 80,
							},
						},
					},

					{
						Conditions: []contour_v1.MatchCondition{
							{
								Prefix: "/second",
							},
						},
						AuthPolicy: &contour_v1.AuthorizationPolicy{
							Disabled: true,
						},
						Services: []contour_v1.Service{
							{
								Name: "echo",
								Port: 80,
							},
						},
					},
					{
						Conditions: []contour_v1.MatchCondition{
							{Prefix: "/direct-response-auth-enabled"},
						},
						DirectResponsePolicy: &contour_v1.HTTPDirectResponsePolicy{
							StatusCode: http.StatusTeapot,
						},
					},
					{
						Conditions: []contour_v1.MatchCondition{
							{Prefix: "/direct-response-auth-disabled"},
						},
						DirectResponsePolicy: &contour_v1.HTTPDirectResponsePolicy{
							StatusCode: http.StatusTeapot,
						},
						AuthPolicy: &contour_v1.AuthorizationPolicy{
							Disabled: true,
						},
					},

					{
						AuthPolicy: &contour_v1.AuthorizationPolicy{
							Context: map[string]string{
								"target": "default",
							},
						},
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
		require.True(f.T(), f.CreateHTTPProxyAndWaitFor(p, e2e.HTTPProxyValid))

		// By default requests to /first should not be authorized.
		res, ok := f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Path:      "/first",
			Condition: e2e.HasStatusCode(401),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 401 response code, got %d", res.StatusCode)

		// The `testserver` authorization server will accept any request with
		// "allow" in the path, so this request should succeed. We can tell that
		// the authorization server processed it by inspecting the context headers
		// that it injects.
		res, ok = f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Path:      "/first/allow",
			Condition: e2e.HasStatusCode(200),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)

		body := f.GetEchoResponseBody(res.Body)
		assert.Equal(t, "first", body.RequestHeaders.Get("Auth-Context-Target"))
		assert.Equal(t, "externalauth.projectcontour.io", body.RequestHeaders.Get("Auth-Context-Hostname"))

		// THe /second route disables authorization so this request should succeed.
		res, ok = f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Path:      "/second",
			Condition: e2e.HasStatusCode(200),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)

		// The default route should not authorize by default.
		res, ok = f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Path:      "/matches-default-route",
			Condition: e2e.HasStatusCode(401),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 401 response code, got %d", res.StatusCode)

		// The `testserver` authorization server will accept any request with
		// "allow" in the path, so this request should succeed. We can tell that
		// the authorization server processed it by inspecting the context headers
		// that it injects.
		res, ok = f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Path:      "/matches-default-route/allow",
			Condition: e2e.HasStatusCode(200),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)

		body = f.GetEchoResponseBody(res.Body)
		assert.Equal(t, "default", body.RequestHeaders.Get("Auth-Context-Target"))
		assert.Equal(t, "externalauth.projectcontour.io", body.RequestHeaders.Get("Auth-Context-Hostname"))

		// Direct response with external auth enabled should get a 401.
		res, ok = f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Path:      "/direct-response-auth-enabled",
			Condition: e2e.HasStatusCode(401),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 401 response code, got %d", res.StatusCode)

		// Direct response with external auth enabled with "allow" in the path
		// should succeed.
		res, ok = f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Path:      "/direct-response-auth-enabled/allow",
			Condition: e2e.HasStatusCode(http.StatusTeapot),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 418 response code, got %d", res.StatusCode)

		// Direct response with external auth disabled should succeed.
		res, ok = f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Path:      "/direct-response-auth-disabled",
			Condition: e2e.HasStatusCode(http.StatusTeapot),
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 418 response code, got %d", res.StatusCode)
	})
}
