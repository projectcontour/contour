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

package e2e

import (
	"context"

	contour_api_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"

	"github.com/onsi/ginkgo"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Fixtures holds references to all of the E2E fixtures helpers.
type Fixtures struct {
	// Echo provides helpers for working with the ingress-conformance-echo
	// test fixture.
	Echo *Echo

	// EchoSecure provides helpers for working with the TLS-secured
	// ingress-conformance-echo-tls test fixture.
	EchoSecure *EchoSecure
}

// Echo manages the ingress-conformance-echo fixture.
type Echo struct {
	client client.Client
	t      ginkgo.GinkgoTInterface
}

// Deploy creates the ingress-conformance-echo fixture, specifically
// the deployment and service, in the given namespace and with the given name, or
// fails the test if it encounters an error. Namespace is defaulted to "default"
// and name is defaulted to "ingress-conformance-echo" if not provided. Returns
// a cleanup function.
func (e *Echo) Deploy(ns, name string) func() {
	valOrDefault := func(val, defaultVal string) string {
		if val != "" {
			return val
		}
		return defaultVal
	}

	ns = valOrDefault(ns, "default")
	name = valOrDefault(name, "ingress-conformance-echo")

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      name,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app.kubernetes.io/name": name},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app.kubernetes.io/name": name},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "conformance-echo",
							Image: "gcr.io/k8s-staging-ingressconformance/echoserver@sha256:dc59c3e517399b436fa9db58f16506bd37f3cd831a7298eaf01bd5762ec514e1",
							Env: []corev1.EnvVar{
								{
									Name:  "INGRESS_NAME",
									Value: name,
								},
								{
									Name:  "SERVICE_NAME",
									Value: name,
								},
								{
									Name: "POD_NAME",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "metadata.name",
										},
									},
								},
								{
									Name: "NAMESPACE",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "metadata.namespace",
										},
									},
								},
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "http-api",
									ContainerPort: 3000,
								},
							},
							ReadinessProbe: &corev1.Probe{
								Handler: corev1.Handler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/health",
										Port: intstr.FromInt(3000),
									},
								},
							},
						},
					},
				},
			},
		},
	}
	require.NoError(e.t, e.client.Create(context.TODO(), deployment))

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      name,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       80,
					TargetPort: intstr.FromString("http-api"),
				},
			},
			Selector: map[string]string{"app.kubernetes.io/name": name},
		},
	}
	require.NoError(e.t, e.client.Create(context.TODO(), service))

	return func() {
		require.NoError(e.t, e.client.Delete(context.TODO(), service))
		require.NoError(e.t, e.client.Delete(context.TODO(), deployment))
	}
}

// EchoSecure manages the TLS-secured ingress-conformance-echo fixture.
type EchoSecure struct {
	client client.Client
	t      ginkgo.GinkgoTInterface
}

// Deploy creates the TLS-secured ingress-conformance-echo-tls fixture, specifically
// the deployment and service, in the given namespace and with the given name, or
// fails the test if it encounters an error. Namespace is defaulted to "default"
// and name is defaulted to "ingress-conformance-echo-tls" if not provided. Returns
// a cleanup function.
func (e *EchoSecure) Deploy(ns, name string) func() {
	valOrDefault := func(val, defaultVal string) string {
		if val != "" {
			return val
		}
		return defaultVal
	}

	ns = valOrDefault(ns, "default")
	name = valOrDefault(name, "ingress-conformance-echo-tls")

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      name,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app.kubernetes.io/name": name},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app.kubernetes.io/name": name},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "conformance-echo",
							Image: "gcr.io/k8s-staging-ingressconformance/echoserver@sha256:dc59c3e517399b436fa9db58f16506bd37f3cd831a7298eaf01bd5762ec514e1",
							Env: []corev1.EnvVar{
								{
									Name:  "INGRESS_NAME",
									Value: name,
								},
								{
									Name:  "SERVICE_NAME",
									Value: name,
								},
								{
									Name: "POD_NAME",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "metadata.name",
										},
									},
								},
								{
									Name: "NAMESPACE",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "metadata.namespace",
										},
									},
								},
								{
									Name:  "TLS_SERVER_CERT",
									Value: "/run/secrets/certs/tls.crt",
								},
								{
									Name:  "TLS_SERVER_PRIVKEY",
									Value: "/run/secrets/certs/tls.key",
								},
								{
									Name:  "TLS_CLIENT_CACERTS",
									Value: "/run/secrets/certs/ca.crt",
								},
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "http-api",
									ContainerPort: 3000,
								},
								{
									Name:          "https-api",
									ContainerPort: 8443,
								},
							},
							ReadinessProbe: &corev1.Probe{
								Handler: corev1.Handler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/health",
										Port: intstr.FromInt(3000),
									},
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									MountPath: "/run/secrets/certs",
									Name:      "backend-server-cert",
									ReadOnly:  true,
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "backend-server-cert",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: "backend-server-cert",
								},
							},
						},
					},
				},
			},
		},
	}
	require.NoError(e.t, e.client.Create(context.TODO(), deployment))

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      name,
			Annotations: map[string]string{
				"projectcontour.io/upstream-protocol.tls": "443",
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       80,
					TargetPort: intstr.FromString("http-api"),
				},
				{
					Name:       "https",
					Port:       443,
					TargetPort: intstr.FromString("https-api"),
				},
			},
			Selector: map[string]string{"app.kubernetes.io/name": name},
		},
	}
	require.NoError(e.t, e.client.Create(context.TODO(), service))

	return func() {
		require.NoError(e.t, e.client.Delete(context.TODO(), service))
		require.NoError(e.t, e.client.Delete(context.TODO(), deployment))
	}
}

var ContourConfigration = &contour_api_v1alpha1.ContourConfiguration{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "ingress",
		Namespace: "projectcontour",
	},
	Spec: contour_api_v1alpha1.ContourConfigurationSpec{
		Debug: contour_api_v1alpha1.DebugConfig{
			Address:                 "127.0.0.1",
			Port:                    6060,
			DebugLogLevel:           contour_api_v1alpha1.InfoLog,
			KubernetesDebugLogLevel: 0,
		},
		Health: contour_api_v1alpha1.HealthConfig{
			Address: "0.0.0.0",
			Port:    8000,
		},
		Envoy: contour_api_v1alpha1.EnvoyConfig{
			DefaultHTTPVersions: []contour_api_v1alpha1.HTTPVersionType{
				"HTTP/1.1", "HTTP/2",
			},
			Listener: contour_api_v1alpha1.EnvoyListenerConfig{
				UseProxyProto:             false,
				DisableAllowChunkedLength: false,
				ConnectionBalancer:        "",
				TLS: contour_api_v1alpha1.EnvoyTLS{
					MinimumProtocolVersion: "1.2",
					CipherSuites: []contour_api_v1alpha1.TLSCipherType{
						"[ECDHE-ECDSA-AES128-GCM-SHA256|ECDHE-ECDSA-CHACHA20-POLY1305]",
						"[ECDHE-RSA-AES128-GCM-SHA256|ECDHE-RSA-CHACHA20-POLY1305]",
						"ECDHE-ECDSA-AES256-GCM-SHA384",
						"ECDHE-RSA-AES256-GCM-SHA384",
					},
				},
			},
			Service: contour_api_v1alpha1.NamespacedName{
				Name:      "envoy",
				Namespace: "projectcontour",
			},
			HTTPListener: contour_api_v1alpha1.EnvoyListener{
				Address:   "0.0.0.0",
				Port:      8080,
				AccessLog: "/dev/stdout",
			},
			HTTPSListener: contour_api_v1alpha1.EnvoyListener{
				Address:   "0.0.0.0",
				Port:      8443,
				AccessLog: "/dev/stdout",
			},
			Metrics: contour_api_v1alpha1.MetricsConfig{
				Address: "0.0.0.0",
				Port:    8002,
			},
			Logging: contour_api_v1alpha1.EnvoyLogging{
				AccessLogFormat: contour_api_v1alpha1.EnvoyAccessLog,
			},
			Cluster: contour_api_v1alpha1.ClusterParameters{
				DNSLookupFamily: contour_api_v1alpha1.AutoClusterDNSFamily,
			},
			Network: contour_api_v1alpha1.NetworkParameters{
				EnvoyAdminPort: 9001,
			},
		},
		HTTPProxy: contour_api_v1alpha1.HTTPProxyConfig{
			DisablePermitInsecure: false,
		},
		EnableExternalNameService: false,
		Metrics: contour_api_v1alpha1.MetricsConfig{
			Address: "0.0.0.0",
			Port:    8000,
		},
	},
}
