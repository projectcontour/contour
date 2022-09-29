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

package equality_test

import (
	"testing"

	"github.com/projectcontour/contour/internal/provisioner/equality"
	"github.com/projectcontour/contour/internal/provisioner/model"
	"github.com/projectcontour/contour/internal/provisioner/objects/dataplane"
	"github.com/projectcontour/contour/internal/provisioner/objects/deployment"
	"github.com/projectcontour/contour/internal/provisioner/objects/service"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var (
	testName  = "test"
	testNs    = testName + "-ns"
	testImage = "test-image:main"
	cntr      = &model.Contour{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testName,
			Namespace: testNs,
		},
	}
)

func TestDaemonSetConfigChanged(t *testing.T) {
	testCases := []struct {
		description string
		mutate      func(ds *appsv1.DaemonSet)
		expect      bool
	}{
		{
			description: "if nothing changes",
			mutate:      func(_ *appsv1.DaemonSet) {},
			expect:      false,
		},
		{
			description: "if labels are changed",
			mutate: func(ds *appsv1.DaemonSet) {
				ds.Labels = map[string]string{}
			},
			expect: true,
		},
		{
			description: "if selector is changed",
			mutate: func(ds *appsv1.DaemonSet) {
				ds.Spec.Selector = &metav1.LabelSelector{}
			},
			expect: true,
		},
		{
			description: "if the container image is changed",
			mutate: func(ds *appsv1.DaemonSet) {
				ds.Spec.Template.Spec.Containers[0].Image = "foo:latest"
			},
			expect: true,
		},
		{
			description: "if a volume is changed",
			mutate: func(ds *appsv1.DaemonSet) {
				ds.Spec.Template.Spec.Volumes = []corev1.Volume{
					{
						Name: "foo",
						VolumeSource: corev1.VolumeSource{
							HostPath: &corev1.HostPathVolumeSource{
								Path: "/foo",
							},
						},
					}}
			},
			expect: true,
		},
		{
			description: "if container commands are changed",
			mutate: func(ds *appsv1.DaemonSet) {
				ds.Spec.Template.Spec.Containers[0].Command = []string{"foo"}
			},
			expect: true,
		},
		{
			description: "if container args are changed",
			mutate: func(ds *appsv1.DaemonSet) {
				ds.Spec.Template.Spec.Containers[0].Args = []string{"foo", "bar", "baz"}
			},
			expect: true,
		},
		{
			description: "if probe values are set to default values",
			mutate: func(ds *appsv1.DaemonSet) {
				for i, c := range ds.Spec.Template.Spec.Containers {
					if c.Name == dataplane.ShutdownContainerName {
						ds.Spec.Template.Spec.Containers[i].LivenessProbe.ProbeHandler.HTTPGet.Scheme = "HTTP"
						ds.Spec.Template.Spec.Containers[i].LivenessProbe.TimeoutSeconds = int32(1)
						ds.Spec.Template.Spec.Containers[i].LivenessProbe.PeriodSeconds = int32(10)
						ds.Spec.Template.Spec.Containers[i].LivenessProbe.SuccessThreshold = int32(1)
						ds.Spec.Template.Spec.Containers[i].LivenessProbe.FailureThreshold = int32(3)
					}
					if c.Name == dataplane.EnvoyContainerName {
						ds.Spec.Template.Spec.Containers[i].ReadinessProbe.TimeoutSeconds = int32(1)
						// ReadinessProbe InitialDelaySeconds and PeriodSeconds are not set as defaults,
						// so they are omitted.
						ds.Spec.Template.Spec.Containers[i].ReadinessProbe.SuccessThreshold = int32(1)
						ds.Spec.Template.Spec.Containers[i].ReadinessProbe.FailureThreshold = int32(3)
					}
				}
			},
			expect: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			original := dataplane.DesiredDaemonSet(cntr, testImage, testImage)

			mutated := original.DeepCopy()
			tc.mutate(mutated)
			if updated, changed := equality.DaemonsetConfigChanged(original, mutated); changed != tc.expect {
				t.Errorf("expect daemonsetConfigChanged to be %t, got %t", tc.expect, changed)
			} else if changed {
				if _, changedAgain := equality.DaemonsetConfigChanged(mutated, updated); changedAgain {
					t.Error("daemonsetConfigChanged does not behave as a fixed point function")
				}
			}
		})
	}
}

func TestDeploymentConfigChanged(t *testing.T) {
	testCases := []struct {
		description string
		mutate      func(deployment *appsv1.Deployment)
		expect      bool
	}{
		{
			description: "if nothing changes",
			mutate:      func(_ *appsv1.Deployment) {},
			expect:      false,
		},
		{
			description: "if replicas is changed",
			mutate: func(deploy *appsv1.Deployment) {
				deploy.Spec.Replicas = nil
			},
			expect: true,
		},
		{
			description: "if selector is changed",
			mutate: func(deploy *appsv1.Deployment) {
				deploy.Spec.Selector = &metav1.LabelSelector{}
			},
			expect: true,
		},
		{
			description: "if the container image is changed",
			mutate: func(deploy *appsv1.Deployment) {
				deploy.Spec.Template.Spec.Containers[0].Image = "foo:latest"
			},
			expect: true,
		},
		{
			description: "if a volume is changed",
			mutate: func(deploy *appsv1.Deployment) {
				deploy.Spec.Template.Spec.Volumes = []corev1.Volume{
					{
						Name: "foo",
						VolumeSource: corev1.VolumeSource{
							HostPath: &corev1.HostPathVolumeSource{
								Path: "/foo",
							},
						},
					}}
			},
			expect: true,
		},
		{
			description: "if container commands are changed",
			mutate: func(deploy *appsv1.Deployment) {
				deploy.Spec.Template.Spec.Containers[0].Command = []string{"foo"}
			},
			expect: true,
		},
		{
			description: "if container args are changed",
			mutate: func(deploy *appsv1.Deployment) {
				deploy.Spec.Template.Spec.Containers[0].Args = []string{"foo", "bar", "baz"}
			},
			expect: true,
		},
		{
			description: "if probe values are set to default values",
			mutate: func(deployment *appsv1.Deployment) {
				deployment.Spec.Template.Spec.Containers[0].LivenessProbe.ProbeHandler.HTTPGet.Scheme = "HTTP"
				deployment.Spec.Template.Spec.Containers[0].LivenessProbe.TimeoutSeconds = int32(1)
				deployment.Spec.Template.Spec.Containers[0].LivenessProbe.PeriodSeconds = int32(10)
				deployment.Spec.Template.Spec.Containers[0].LivenessProbe.SuccessThreshold = int32(1)
				deployment.Spec.Template.Spec.Containers[0].LivenessProbe.FailureThreshold = int32(3)
				deployment.Spec.Template.Spec.Containers[0].ReadinessProbe.TimeoutSeconds = int32(1)
				// ReadinessProbe InitialDelaySeconds and PeriodSeconds are not set at defaults,
				// so they are omitted.
				deployment.Spec.Template.Spec.Containers[0].ReadinessProbe.SuccessThreshold = int32(1)
				deployment.Spec.Template.Spec.Containers[0].ReadinessProbe.FailureThreshold = int32(3)
			},
			expect: false,
		},
	}

	for _, tc := range testCases {
		original := deployment.DesiredDeployment(cntr, testImage)
		mutated := original.DeepCopy()
		tc.mutate(mutated)
		if updated, changed := equality.DeploymentConfigChanged(original, mutated); changed != tc.expect {
			t.Errorf("%s, expect deploymentConfigChanged to be %t, got %t", tc.description, tc.expect, changed)
		} else if changed {
			if _, changedAgain := equality.DeploymentConfigChanged(updated, mutated); changedAgain {
				t.Errorf("%s, deploymentConfigChanged does not behave as a fixed point function", tc.description)
			}
		}
	}
}

func TestClusterIpServiceChanged(t *testing.T) {
	testCases := []struct {
		description string
		mutate      func(service *corev1.Service)
		expect      bool
	}{
		{
			description: "if nothing changed",
			mutate:      func(_ *corev1.Service) {},
			expect:      false,
		},
		{
			description: "if the port number changed",
			mutate: func(svc *corev1.Service) {
				svc.Spec.Ports[0].Port = int32(1234)
			},
			expect: true,
		},
		{
			description: "if the target port number changed",
			mutate: func(svc *corev1.Service) {
				intStrPort := intstr.IntOrString{IntVal: int32(1234)}
				svc.Spec.Ports[0].TargetPort = intStrPort
			},
			expect: true,
		},
		{
			description: "if the port name changed",
			mutate: func(svc *corev1.Service) {
				svc.Spec.Ports[0].Name = "foo"
			},
			expect: true,
		},
		{
			description: "if the port protocol changed",
			mutate: func(svc *corev1.Service) {
				svc.Spec.Ports[0].Protocol = corev1.ProtocolUDP
			},
			expect: true,
		},
		{
			description: "if ports are added",
			mutate: func(svc *corev1.Service) {
				port := corev1.ServicePort{
					Name:       "foo",
					Protocol:   corev1.ProtocolUDP,
					Port:       int32(1234),
					TargetPort: intstr.IntOrString{IntVal: int32(1234)},
				}
				svc.Spec.Ports = append(svc.Spec.Ports, port)
			},
			expect: true,
		},
		{
			description: "if ports are removed",
			mutate: func(svc *corev1.Service) {
				svc.Spec.Ports = []corev1.ServicePort{}
			},
			expect: true,
		},
		{
			description: "if the cluster IP changed",
			mutate: func(svc *corev1.Service) {
				svc.Spec.ClusterIP = "1.2.3.4"
			},
			expect: false,
		},
		{
			description: "if selector changed",
			mutate: func(svc *corev1.Service) {
				svc.Spec.Selector = map[string]string{"foo": "bar"}
			},
			expect: true,
		},
		{
			description: "if service type changed",
			mutate: func(svc *corev1.Service) {
				svc.Spec.Type = corev1.ServiceTypeLoadBalancer
			},
			expect: true,
		},
		{
			description: "if session affinity changed",
			mutate: func(svc *corev1.Service) {
				svc.Spec.SessionAffinity = corev1.ServiceAffinityClientIP
			},
			expect: true,
		},
	}

	for _, tc := range testCases {
		expected := service.DesiredContourService(cntr)

		mutated := expected.DeepCopy()
		tc.mutate(mutated)
		if updated, changed := equality.ClusterIPServiceChanged(mutated, expected); changed != tc.expect {
			t.Errorf("%s, expect ClusterIpServiceChanged to be %t, got %t", tc.description, tc.expect, changed)
		} else if changed {
			if _, changedAgain := equality.ClusterIPServiceChanged(updated, expected); changedAgain {
				t.Errorf("%s, ClusterIpServiceChanged does not behave as a fixed point function", tc.description)
			}
		}
	}
}

func TestLoadBalancerServiceChanged(t *testing.T) {
	testCases := []struct {
		description string
		mutate      func(service *corev1.Service)
		expect      bool
	}{
		{
			description: "if nothing changed",
			mutate:      func(_ *corev1.Service) {},
			expect:      false,
		},
		{
			description: "if the port number changed",
			mutate: func(svc *corev1.Service) {
				svc.Spec.Ports[0].Port = int32(1234)
			},
			expect: true,
		},
		{
			description: "if the target port number changed",
			mutate: func(svc *corev1.Service) {
				intStrPort := intstr.IntOrString{IntVal: int32(1234)}
				svc.Spec.Ports[0].TargetPort = intStrPort
			},
			expect: true,
		},
		{
			description: "if the port name changed",
			mutate: func(svc *corev1.Service) {
				svc.Spec.Ports[0].Name = "foo"
			},
			expect: true,
		},
		{
			description: "if the port protocol changed",
			mutate: func(svc *corev1.Service) {
				svc.Spec.Ports[0].Protocol = corev1.ProtocolUDP
			},
			expect: true,
		},
		{
			description: "if ports are added",
			mutate: func(svc *corev1.Service) {
				port := corev1.ServicePort{
					Name:       "foo",
					Protocol:   corev1.ProtocolUDP,
					Port:       int32(1234),
					TargetPort: intstr.IntOrString{IntVal: int32(1234)},
				}
				svc.Spec.Ports = append(svc.Spec.Ports, port)
			},
			expect: true,
		},
		{
			description: "if ports are removed",
			mutate: func(svc *corev1.Service) {
				svc.Spec.Ports = []corev1.ServicePort{}
			},
			expect: true,
		},
		{
			description: "if the cluster IP changed",
			mutate: func(svc *corev1.Service) {
				svc.Spec.ClusterIP = "1.2.3.4"
			},
			expect: false,
		},
		{
			description: "if selector changed",
			mutate: func(svc *corev1.Service) {
				svc.Spec.Selector = map[string]string{"foo": "bar"}
			},
			expect: true,
		},
		{
			description: "if service type changed",
			mutate: func(svc *corev1.Service) {
				svc.Spec.Type = corev1.ServiceTypeClusterIP
			},
			expect: true,
		},
		{
			description: "if session affinity changed",
			mutate: func(svc *corev1.Service) {
				svc.Spec.SessionAffinity = corev1.ServiceAffinityClientIP
			},
			expect: true,
		},
		{
			description: "if external traffic policy changed",
			mutate: func(svc *corev1.Service) {
				svc.Spec.ExternalTrafficPolicy = corev1.ServiceExternalTrafficPolicyTypeCluster
			},
			expect: true,
		},
		{
			description: "if annotations have changed",
			mutate: func(svc *corev1.Service) {
				svc.Annotations = map[string]string{}
			},
			expect: true,
		},
		{
			description: "if load balancer IP changed",
			mutate: func(svc *corev1.Service) {
				svc.Spec.LoadBalancerIP = "5.6.7.8"
			},
			expect: true,
		},
	}

	for _, tc := range testCases {

		cntr.Spec.NetworkPublishing.Envoy.Type = model.LoadBalancerServicePublishingType
		cntr.Spec.NetworkPublishing.Envoy.LoadBalancer.Scope = model.ExternalLoadBalancer
		cntr.Spec.NetworkPublishing.Envoy.LoadBalancer.ProviderParameters.Type = model.AWSLoadBalancerProvider
		if tc.description == "if load balancer IP changed" {
			loadBalancerIP := "1.2.3.4"
			cntr = &model.Contour{
				Spec: model.ContourSpec{
					NetworkPublishing: model.NetworkPublishing{
						Envoy: model.EnvoyNetworkPublishing{
							Type: model.LoadBalancerServicePublishingType,
							LoadBalancer: model.LoadBalancerStrategy{
								ProviderParameters: model.ProviderLoadBalancerParameters{
									GCP: &model.GCPLoadBalancerParameters{},
								},
							},
						},
					},
				},
			}
			cntr.Spec.NetworkPublishing.Envoy.LoadBalancer.ProviderParameters.Type = model.GCPLoadBalancerProvider
			cntr.Spec.NetworkPublishing.Envoy.LoadBalancer.ProviderParameters.GCP.Address = &loadBalancerIP
		}
		cntr.Spec.NetworkPublishing.Envoy.ServicePorts = []model.ServicePort{
			{
				Name:       "http",
				PortNumber: service.EnvoyServiceHTTPPort,
			},
			{
				Name:       "https",
				PortNumber: service.EnvoyServiceHTTPPort,
			},
		}
		cntr.Spec.NetworkPublishing.Envoy.ContainerPorts = []model.ContainerPort{
			{
				Name:       "http",
				PortNumber: service.EnvoyServiceHTTPPort,
			},
			{
				Name:       "https",
				PortNumber: service.EnvoyServiceHTTPPort,
			},
			{
				Name:       "https",
				PortNumber: service.EnvoyServiceHTTPSPort,
			},
		}
		expected := service.DesiredEnvoyService(cntr)

		mutated := expected.DeepCopy()
		tc.mutate(mutated)
		if updated, changed := equality.LoadBalancerServiceChanged(mutated, expected); changed != tc.expect {
			t.Errorf("%s, expect LoadBalancerServiceChanged to be %t, got %t", tc.description, tc.expect, changed)
		} else if changed {
			if _, changedAgain := equality.LoadBalancerServiceChanged(updated, expected); changedAgain {
				t.Errorf("%s, LoadBalancerServiceChanged does not behave as a fixed point function", tc.description)
			}
		}
	}
}

func TestNodePortServiceChanged(t *testing.T) {
	testCases := []struct {
		description string
		mutate      func(service *corev1.Service)
		expect      bool
	}{
		{
			description: "if nothing changed",
			mutate:      func(_ *corev1.Service) {},
			expect:      false,
		},
		{
			description: "if the nodeport port number changed",
			mutate: func(svc *corev1.Service) {
				svc.Spec.Ports[0].NodePort = int32(1234)
			},
			expect: true,
		},
		{
			description: "if the number of ports changed",
			mutate: func(svc *corev1.Service) {
				svc.Spec.Ports = append(svc.Spec.Ports, svc.Spec.Ports[0])
			},
			expect: true,
		},
	}

	for _, tc := range testCases {
		cntr.Spec.NetworkPublishing.Envoy.Type = model.NodePortServicePublishingType
		cntr.Spec.NetworkPublishing.Envoy.ContainerPorts = []model.ContainerPort{
			{
				Name:       "http",
				PortNumber: service.EnvoyServiceHTTPPort,
			},
			{
				Name:       "https",
				PortNumber: service.EnvoyServiceHTTPSPort,
			},
			{
				Name:       "https",
				PortNumber: service.EnvoyServiceHTTPSPort,
			},
		}
		expected := service.DesiredEnvoyService(cntr)

		mutated := expected.DeepCopy()
		tc.mutate(mutated)
		if updated, changed := equality.NodePortServiceChanged(mutated, expected); changed != tc.expect {
			t.Errorf("%s, expect NodePortServiceChanged to be %t, got %t", tc.description, tc.expect, changed)
		} else if changed {
			if _, changedAgain := equality.NodePortServiceChanged(updated, expected); changedAgain {
				t.Errorf("%s, NodePortServiceChanged does not behave as a fixed point function", tc.description)
			}
		}
	}
}
