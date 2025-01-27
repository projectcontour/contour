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

package e2e

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/require"
	apps_v1 "k8s.io/api/apps/v1"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/pkg/config"
)

const (
	// EchoServerImage is the image to use as a backend fixture.
	EchoServerImage = "gcr.io/k8s-staging-ingressconformance/echoserver:v20221109-7ee2f3e"

	// GRPCServerImage is the image to use for tests that require a gRPC server.
	GRPCServerImage = "ghcr.io/projectcontour/yages:v0.1.0"
)

// Fixtures holds references to all of the E2E fixtures helpers.
type Fixtures struct {
	// Echo provides helpers for working with the ingress-conformance-echo
	// test fixture.
	Echo *Echo

	// EchoSecure provides helpers for working with the TLS-secured
	// ingress-conformance-echo-tls test fixture.
	EchoSecure *EchoSecure

	// GRPC provides helpers for working with a gRPC echo server test
	// fixture.
	GRPC *GRPC
}

// Echo manages the ingress-conformance-echo fixture.
type Echo struct {
	client     client.Client
	t          ginkgo.GinkgoTInterface
	kubeConfig string
}

// Deploy runs DeployN with a default of 1 replica.
func (e *Echo) Deploy(ns, name string) (func(), *apps_v1.Deployment) {
	return e.DeployN(ns, name, 1)
}

// DeployN creates the ingress-conformance-echo fixture, specifically
// the deployment and service, in the given namespace and with the given name, or
// fails the test if it encounters an error. Number of replicas of the deployment
// can be configured. Namespace is defaulted to "default"
// and name is defaulted to "ingress-conformance-echo" if not provided. Returns
// a cleanup function.
func (e *Echo) DeployN(ns, name string, replicas int32) (func(), *apps_v1.Deployment) {
	ns = valOrDefault(ns, "default")
	name = valOrDefault(name, "ingress-conformance-echo")

	deployment := &apps_v1.Deployment{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: ns,
			Name:      name,
		},
		Spec: apps_v1.DeploymentSpec{
			Replicas: ptr.To(replicas),
			Selector: &meta_v1.LabelSelector{
				MatchLabels: map[string]string{"app.kubernetes.io/name": name},
			},
			Template: core_v1.PodTemplateSpec{
				ObjectMeta: meta_v1.ObjectMeta{
					Labels: map[string]string{"app.kubernetes.io/name": name},
				},
				Spec: core_v1.PodSpec{
					TopologySpreadConstraints: []core_v1.TopologySpreadConstraint{
						{
							// Attempt to spread pods across different nodes if possible.
							TopologyKey:       "kubernetes.io/hostname",
							MaxSkew:           1,
							WhenUnsatisfiable: core_v1.ScheduleAnyway,
							LabelSelector: &meta_v1.LabelSelector{
								MatchLabels: map[string]string{"app.kubernetes.io/name": name},
							},
						},
					},
					Containers: []core_v1.Container{
						{
							Name:  "conformance-echo",
							Image: EchoServerImage,
							Env: []core_v1.EnvVar{
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
									ValueFrom: &core_v1.EnvVarSource{
										FieldRef: &core_v1.ObjectFieldSelector{
											FieldPath: "metadata.name",
										},
									},
								},
								{
									Name: "NAMESPACE",
									ValueFrom: &core_v1.EnvVarSource{
										FieldRef: &core_v1.ObjectFieldSelector{
											FieldPath: "metadata.namespace",
										},
									},
								},
							},
							Ports: []core_v1.ContainerPort{
								{
									Name:          "http-api",
									ContainerPort: 3000,
								},
							},
							ReadinessProbe: &core_v1.Probe{
								ProbeHandler: core_v1.ProbeHandler{
									HTTPGet: &core_v1.HTTPGetAction{
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

	service := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: ns,
			Name:      name,
		},
		Spec: core_v1.ServiceSpec{
			Ports: []core_v1.ServicePort{
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
	}, deployment
}

func (e *Echo) ScaleAndWaitDeployment(name, ns string, replicas int32) {
	deployment := &apps_v1.Deployment{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
	}

	updateAndWaitFor(e.t, e.client, deployment,
		func(d *apps_v1.Deployment) {
			d.Spec.Replicas = ptr.To(replicas)
		},
		func(d *apps_v1.Deployment) bool {
			if d.Status.Replicas == replicas && d.Status.ReadyReplicas == replicas {
				return true
			}
			return false
		}, time.Second, time.Second*10)
}

func (e *Echo) ListPodIPs(ns, name string) ([]string, error) {
	ns = valOrDefault(ns, "default")
	name = valOrDefault(name, "ingress-conformance-echo")

	pods := new(core_v1.PodList)
	podListOptions := &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{"app.kubernetes.io/name": name}),
		Namespace:     ns,
	}
	if err := e.client.List(context.TODO(), pods, podListOptions); err != nil {
		return nil, err
	}

	podIPs := make([]string, 0)
	for _, pod := range pods.Items {
		podIPs = append(podIPs, pod.Status.PodIP)
	}

	return podIPs, nil
}

// DumpEchoLogs returns logs of the "conformance-echo" container in
// the Echo pod in the given namespace and with the given name.
// Namespace is defaulted to "default" and name is defaulted to
// "ingress-conformance-echo" if not provided.
func (e *Echo) DumpEchoLogs(ns, name string) ([][]byte, error) {
	ns = valOrDefault(ns, "default")
	name = valOrDefault(name, "ingress-conformance-echo")

	var logs [][]byte

	config, err := clientcmd.BuildConfigFromFlags("", e.kubeConfig)
	if err != nil {
		return nil, err
	}
	coreClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	pods := new(core_v1.PodList)
	podListOptions := &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{"app.kubernetes.io/name": name}),
		Namespace:     ns,
	}
	if err := e.client.List(context.TODO(), pods, podListOptions); err != nil {
		return nil, err
	}

	podLogOptions := &core_v1.PodLogOptions{
		Container: "conformance-echo",
	}
	for _, pod := range pods.Items {
		if pod.Status.Phase == core_v1.PodFailed {
			continue
		}

		req := coreClient.CoreV1().Pods(ns).GetLogs(pod.Name, podLogOptions)
		logStream, err := req.Stream(context.TODO())
		if err != nil {
			continue
		}
		defer logStream.Close()
		logBytes, err := io.ReadAll(logStream)
		if err != nil {
			continue
		}
		logs = append(logs, logBytes)
	}

	return logs, nil
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
func (e *EchoSecure) Deploy(ns, name string, preApplyHook func(deployment *apps_v1.Deployment, service *core_v1.Service)) func() {
	ns = valOrDefault(ns, "default")
	name = valOrDefault(name, "ingress-conformance-echo-tls")

	deployment := &apps_v1.Deployment{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: ns,
			Name:      name,
		},
		Spec: apps_v1.DeploymentSpec{
			Selector: &meta_v1.LabelSelector{
				MatchLabels: map[string]string{"app.kubernetes.io/name": name},
			},
			Template: core_v1.PodTemplateSpec{
				ObjectMeta: meta_v1.ObjectMeta{
					Labels: map[string]string{"app.kubernetes.io/name": name},
				},
				Spec: core_v1.PodSpec{
					Containers: []core_v1.Container{
						{
							Name:  "conformance-echo",
							Image: EchoServerImage,
							Env: []core_v1.EnvVar{
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
									ValueFrom: &core_v1.EnvVarSource{
										FieldRef: &core_v1.ObjectFieldSelector{
											FieldPath: "metadata.name",
										},
									},
								},
								{
									Name: "NAMESPACE",
									ValueFrom: &core_v1.EnvVarSource{
										FieldRef: &core_v1.ObjectFieldSelector{
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
							Ports: []core_v1.ContainerPort{
								{
									Name:          "http-api",
									ContainerPort: 3000,
								},
								{
									Name:          "https-api",
									ContainerPort: 8443,
								},
							},
							ReadinessProbe: &core_v1.Probe{
								ProbeHandler: core_v1.ProbeHandler{
									HTTPGet: &core_v1.HTTPGetAction{
										Path: "/health",
										Port: intstr.FromInt(3000),
									},
								},
							},
							VolumeMounts: []core_v1.VolumeMount{
								{
									MountPath: "/run/secrets/certs",
									Name:      "backend-server-cert",
									ReadOnly:  true,
								},
							},
						},
					},
					Volumes: []core_v1.Volume{
						{
							Name: "backend-server-cert",
							VolumeSource: core_v1.VolumeSource{
								Secret: &core_v1.SecretVolumeSource{
									SecretName: "backend-server-cert",
								},
							},
						},
					},
				},
			},
		},
	}

	service := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: ns,
			Name:      name,
			Annotations: map[string]string{
				"projectcontour.io/upstream-protocol.tls": "443",
			},
		},
		Spec: core_v1.ServiceSpec{
			Ports: []core_v1.ServicePort{
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

	if preApplyHook != nil {
		preApplyHook(deployment, service)
	}

	require.NoError(e.t, e.client.Create(context.TODO(), deployment))
	require.NoError(e.t, e.client.Create(context.TODO(), service))

	return func() {
		require.NoError(e.t, e.client.Delete(context.TODO(), service))
		require.NoError(e.t, e.client.Delete(context.TODO(), deployment))
	}
}

type GRPC struct {
	client client.Client
	t      ginkgo.GinkgoTInterface
}

func (g *GRPC) Deploy(ns, name string) func() {
	ns = valOrDefault(ns, "default")
	name = valOrDefault(name, "grpc-echo")

	deployment := &apps_v1.Deployment{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: ns,
			Name:      name,
		},
		Spec: apps_v1.DeploymentSpec{
			Replicas: ptr.To(int32(1)),
			Selector: &meta_v1.LabelSelector{
				MatchLabels: map[string]string{"app.kubernetes.io/name": name},
			},
			Template: core_v1.PodTemplateSpec{
				ObjectMeta: meta_v1.ObjectMeta{
					Labels: map[string]string{"app.kubernetes.io/name": name},
				},
				Spec: core_v1.PodSpec{
					TopologySpreadConstraints: []core_v1.TopologySpreadConstraint{
						{
							// Attempt to spread pods across different nodes if possible.
							TopologyKey:       "kubernetes.io/hostname",
							MaxSkew:           1,
							WhenUnsatisfiable: core_v1.ScheduleAnyway,
							LabelSelector: &meta_v1.LabelSelector{
								MatchLabels: map[string]string{"app.kubernetes.io/name": name},
							},
						},
					},
					Containers: []core_v1.Container{
						{
							Name:            "grpc-echo",
							Image:           GRPCServerImage,
							ImagePullPolicy: core_v1.PullIfNotPresent,
							Env: []core_v1.EnvVar{
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
									ValueFrom: &core_v1.EnvVarSource{
										FieldRef: &core_v1.ObjectFieldSelector{
											FieldPath: "metadata.name",
										},
									},
								},
								{
									Name: "NAMESPACE",
									ValueFrom: &core_v1.EnvVarSource{
										FieldRef: &core_v1.ObjectFieldSelector{
											FieldPath: "metadata.namespace",
										},
									},
								},
							},
							Ports: []core_v1.ContainerPort{
								{
									Name:          "grpc",
									ContainerPort: 9000,
								},
							},
							ReadinessProbe: &core_v1.Probe{
								ProbeHandler: core_v1.ProbeHandler{
									Exec: &core_v1.ExecAction{
										Command: []string{"/grpc-health-probe", "-addr=localhost:9000"},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	require.NoError(g.t, g.client.Create(context.TODO(), deployment))

	service := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: ns,
			Name:      name,
		},
		Spec: core_v1.ServiceSpec{
			Ports: []core_v1.ServicePort{
				{
					Name:       "grpc",
					Port:       9000,
					TargetPort: intstr.FromString("grpc"),
				},
			},
			Selector: map[string]string{"app.kubernetes.io/name": name},
		},
	}
	require.NoError(g.t, g.client.Create(context.TODO(), service))

	return func() {
		require.NoError(g.t, g.client.Delete(context.TODO(), service))
		require.NoError(g.t, g.client.Delete(context.TODO(), deployment))
	}
}

// DefaultContourConfigFileParams returns a default configuration in a config
// file params object.
func DefaultContourConfigFileParams() *config.Parameters {
	return &config.Parameters{}
}

// DefaultContourConfiguration returns a default ContourConfiguration object.
func DefaultContourConfiguration() *contour_v1alpha1.ContourConfiguration {
	return &contour_v1alpha1.ContourConfiguration{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "ingress",
			Namespace: "projectcontour",
		},
		Spec: contour_v1alpha1.ContourConfigurationSpec{
			XDSServer: &contour_v1alpha1.XDSServerConfig{
				Address: listenAllAddress(),
				Port:    8001,
				TLS: &contour_v1alpha1.TLS{
					CAFile:   "/certs/ca.crt",
					CertFile: "/certs/tls.crt",
					KeyFile:  "/certs/tls.key",
					Insecure: ptr.To(false),
				},
			},
			Debug: &contour_v1alpha1.DebugConfig{
				Address: localAddress(),
				Port:    6060,
			},
			Health: &contour_v1alpha1.HealthConfig{
				Address: listenAllAddress(),
				Port:    8000,
			},
			FeatureFlags: UseFeatureFlagsFromEnv(),
			Envoy: &contour_v1alpha1.EnvoyConfig{
				DefaultHTTPVersions: []contour_v1alpha1.HTTPVersionType{
					"HTTP/1.1", "HTTP/2",
				},
				Listener: &contour_v1alpha1.EnvoyListenerConfig{
					UseProxyProto:             ptr.To(false),
					DisableAllowChunkedLength: ptr.To(false),
					ConnectionBalancer:        "",
					TLS: &contour_v1alpha1.EnvoyTLS{
						MinimumProtocolVersion: "1.2",
						CipherSuites: []string{
							"[ECDHE-ECDSA-AES128-GCM-SHA256|ECDHE-ECDSA-CHACHA20-POLY1305]",
							"[ECDHE-RSA-AES128-GCM-SHA256|ECDHE-RSA-CHACHA20-POLY1305]",
							"ECDHE-ECDSA-AES256-GCM-SHA384",
							"ECDHE-RSA-AES256-GCM-SHA384",
						},
					},
				},
				Service: &contour_v1alpha1.NamespacedName{
					Name:      "envoy",
					Namespace: "projectcontour",
				},
				HTTPListener: &contour_v1alpha1.EnvoyListener{
					Address:   listenAllAddress(),
					Port:      8080,
					AccessLog: "/dev/stdout",
				},
				HTTPSListener: &contour_v1alpha1.EnvoyListener{
					Address:   listenAllAddress(),
					Port:      8443,
					AccessLog: "/dev/stdout",
				},
				Health: &contour_v1alpha1.HealthConfig{
					Address: listenAllAddress(),
					Port:    8002,
				},
				Metrics: &contour_v1alpha1.MetricsConfig{
					Address: listenAllAddress(),
					Port:    8002,
				},
				Logging: &contour_v1alpha1.EnvoyLogging{
					AccessLogFormat: contour_v1alpha1.EnvoyAccessLog,
				},
				Cluster: &contour_v1alpha1.ClusterParameters{
					DNSLookupFamily: contour_v1alpha1.AutoClusterDNSFamily,
				},
				Network: &contour_v1alpha1.NetworkParameters{
					EnvoyAdminPort: ptr.To(9001),
				},
			},
			HTTPProxy: &contour_v1alpha1.HTTPProxyConfig{
				DisablePermitInsecure: ptr.To(false),
			},
			EnableExternalNameService: ptr.To(false),
			Metrics: &contour_v1alpha1.MetricsConfig{
				Address: listenAllAddress(),
				Port:    8000,
			},
		},
	}
}

func UseFeatureFlagsFromEnv() []string {
	flags := make([]string, 0)
	_, found := os.LookupEnv("CONTOUR_E2E_USE_ENDPOINTS")
	if found {
		flags = append(flags, "useEndpointSlices=false")
	}
	return flags
}

func valOrDefault(val, defaultVal string) string {
	if val != "" {
		return val
	}
	return defaultVal
}
