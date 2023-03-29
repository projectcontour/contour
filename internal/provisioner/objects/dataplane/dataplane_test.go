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

package dataplane

import (
	"fmt"
	"testing"

	"github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/provisioner/model"
	"github.com/stretchr/testify/assert"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"
)

func checkDaemonSetHasEnvVar(t *testing.T, ds *appsv1.DaemonSet, container, name string) {
	t.Helper()

	if container == envoyInitContainerName {
		for i, c := range ds.Spec.Template.Spec.InitContainers {
			if c.Name == container {
				for _, envVar := range ds.Spec.Template.Spec.InitContainers[i].Env {
					if envVar.Name == name {
						return
					}
				}
			}
		}
	} else {
		for i, c := range ds.Spec.Template.Spec.Containers {
			if c.Name == container {
				for _, envVar := range ds.Spec.Template.Spec.Containers[i].Env {
					if envVar.Name == name {
						return
					}
				}
			}
		}
	}

	t.Errorf("daemonset is missing environment variable %q", name)
}

func checkDaemonSetHasContainer(t *testing.T, ds *appsv1.DaemonSet, name string, expect bool) *corev1.Container {
	t.Helper()

	if ds.Spec.Template.Spec.Containers == nil {
		t.Error("daemonset has no containers")
	}

	if name == envoyInitContainerName {
		for _, container := range ds.Spec.Template.Spec.InitContainers {
			if container.Name == name {
				if expect {
					return &container
				}
				t.Errorf("daemonset has unexpected %q init container", name)
			}
		}
	} else {
		for _, container := range ds.Spec.Template.Spec.Containers {
			if container.Name == name {
				if expect {
					return &container
				}
				t.Errorf("daemonset has unexpected %q container", name)
			}
		}
	}
	if expect {
		t.Errorf("daemonset has no %q container", name)
	}
	return nil
}

func checkDaemonSetHasLabels(t *testing.T, ds *appsv1.DaemonSet, expected map[string]string) {
	t.Helper()

	if apiequality.Semantic.DeepEqual(ds.Labels, expected) {
		return
	}

	t.Errorf("daemonset has unexpected %q labels", ds.Labels)
}

func checkDaemonSetHasPodAnnotations(t *testing.T, ds *appsv1.DaemonSet, expected map[string]string) {
	t.Helper()

	if apiequality.Semantic.DeepEqual(ds.Spec.Template.ObjectMeta.Annotations, expected) {
		return
	}

	t.Errorf("daemonset has unexpected %q pod annotations", ds.Spec.Template.Annotations)
}

func checkContainerHasPort(t *testing.T, ds *appsv1.DaemonSet, port int32) {
	t.Helper()

	for _, c := range ds.Spec.Template.Spec.Containers {
		for _, p := range c.Ports {
			if p.ContainerPort == port {
				return
			}
		}
	}
	t.Errorf("container is missing containerPort %q", port)
}

func checkContainerHasImage(t *testing.T, container *corev1.Container, image string) {
	t.Helper()

	if container.Image == image {
		return
	}
	t.Errorf("container is missing image %q", image)
}

func checkDaemonSetHasNodeSelector(t *testing.T, ds *appsv1.DaemonSet, expected map[string]string) {
	t.Helper()

	if apiequality.Semantic.DeepEqual(ds.Spec.Template.Spec.NodeSelector, expected) {
		return
	}
	t.Errorf("deployment has unexpected node selector %q", expected)
}

func checkDaemonSetHasVolume(t *testing.T, ds *appsv1.DaemonSet, vol corev1.Volume, volMount corev1.VolumeMount) {
	t.Helper()

	hasVol := false
	hasVolMount := false

	for _, v := range ds.Spec.Template.Spec.Volumes {
		if v.Name == vol.Name {
			hasVol = true
			if !apiequality.Semantic.DeepEqual(v, vol) {
				t.Errorf("daemonset has unexpected volume %q", vol)
			}
		}
	}

	for _, v := range ds.Spec.Template.Spec.Containers[0].VolumeMounts {
		if v.Name == volMount.Name {
			hasVolMount = true
			if !apiequality.Semantic.DeepEqual(v, volMount) {
				t.Errorf("daemonset has unexpected volume mount %q", vol)
			}
		}
	}

	if !(hasVol && hasVolMount) {
		t.Errorf("daemonset has not found volume or volumeMount")
	}

}

func checkDaemonSetHasResourceRequirements(t *testing.T, ds *appsv1.DaemonSet, expected corev1.ResourceRequirements) {
	t.Helper()

	if apiequality.Semantic.DeepEqual(ds.Spec.Template.Spec.Containers[1].Resources, expected) {
		return
	}
	t.Errorf("daemonset has unexpected resource requirements %v", expected)
}
func checkDaemonSetHasUpdateStrategy(t *testing.T, ds *appsv1.DaemonSet, expected appsv1.DaemonSetUpdateStrategy) {
	t.Helper()

	if apiequality.Semantic.DeepEqual(ds.Spec.UpdateStrategy, expected) {
		return
	}
	t.Errorf("daemonset has unexpected update strategy %q", expected)
}

func checkDeploymentHasStrategy(t *testing.T, ds *appsv1.Deployment, expected appsv1.DeploymentStrategy) {
	t.Helper()

	if apiequality.Semantic.DeepEqual(ds.Spec.Strategy, expected) {
		return
	}
	t.Errorf("deployment has unexpected strategy %q", expected)
}

func checkDaemonSetHasTolerations(t *testing.T, ds *appsv1.DaemonSet, expected []corev1.Toleration) {
	t.Helper()

	if apiequality.Semantic.DeepEqual(ds.Spec.Template.Spec.Tolerations, expected) {
		return
	}
	t.Errorf("daemonset has unexpected tolerations %v", expected)
}

func checkDaemonSecurityContext(t *testing.T, ds *appsv1.DaemonSet) {
	t.Helper()

	user := int64(65534)
	group := int64(65534)
	nonRoot := true
	expected := &corev1.PodSecurityContext{
		RunAsUser:    &user,
		RunAsGroup:   &group,
		RunAsNonRoot: &nonRoot,
	}
	if apiequality.Semantic.DeepEqual(ds.Spec.Template.Spec.SecurityContext, expected) {
		return
	}
	t.Errorf("deployment has unexpected SecurityContext %v", expected)
}

func checkContainerHasArg(t *testing.T, container *corev1.Container, arg string) {
	t.Helper()

	for _, a := range container.Args {
		if a == arg {
			return
		}
	}
	t.Errorf("container is missing argument %q", arg)
}

func checkContainerHasReadinessPort(t *testing.T, container *corev1.Container, port int32) {
	t.Helper()

	if container.ReadinessProbe != nil &&
		container.ReadinessProbe.HTTPGet != nil &&
		container.ReadinessProbe.HTTPGet.Port.IntVal == port {
		return
	}
	t.Errorf("container has unexpected readiness port %d", port)
}

func checkDaemonSetHasMetricsPort(t *testing.T, ds *appsv1.DaemonSet, port int32) {
	t.Helper()

	if ds.Spec.Template.ObjectMeta.Annotations["prometheus.io/port"] == fmt.Sprint(port) {
		return
	}
	t.Errorf("container has unexpected metrics port %d", port)
}

func TestDesiredDaemonSet(t *testing.T) {
	name := "ds-test"
	cntr := model.Default(fmt.Sprintf("%s-ns", name), name)
	cntr.Spec.ResourceLabels = map[string]string{
		"key": "val",
	}
	cntr.Spec.EnvoyPodAnnotations = map[string]string{
		"annotation": "value",
	}

	volTest := corev1.Volume{
		Name: "vol-test-mount",
	}
	volTestMount := corev1.VolumeMount{
		Name: volTest.Name,
	}

	cntr.Spec.EnvoyExtraVolumes = append(cntr.Spec.EnvoyExtraVolumes, volTest)
	cntr.Spec.EnvoyExtraVolumeMounts = append(cntr.Spec.EnvoyExtraVolumeMounts, volTestMount)

	cntr.Spec.NetworkPublishing.Envoy.Ports = []model.Port{
		{Name: "http", ServicePort: 80, ContainerPort: 8080},
		{Name: "https", ServicePort: 443, ContainerPort: 8443},
	}

	testContourImage := "ghcr.io/projectcontour/contour:test"
	testEnvoyImage := "docker.io/envoyproxy/envoy:test"
	testLogLevelArg := "--log-level debug"

	resQutoa := corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("400m"),
			corev1.ResourceMemory: resource.MustParse("256Mi"),
		},
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("100m"),
			corev1.ResourceMemory: resource.MustParse("25Mi"),
		},
	}
	cntr.Spec.EnvoyResources = resQutoa

	// Change the Envoy log level to test --log-level debug.
	cntr.Spec.EnvoyLogLevel = v1alpha1.DebugLog

	ds := DesiredDaemonSet(cntr, testContourImage, testEnvoyImage)
	container := checkDaemonSetHasContainer(t, ds, EnvoyContainerName, true)
	checkContainerHasArg(t, container, testLogLevelArg)
	checkContainerHasImage(t, container, testEnvoyImage)
	checkContainerHasReadinessPort(t, container, 8002)
	assert.Len(t, container.Ports, 2)

	container = checkDaemonSetHasContainer(t, ds, ShutdownContainerName, true)
	checkContainerHasImage(t, container, testContourImage)
	container = checkDaemonSetHasContainer(t, ds, envoyInitContainerName, true)
	checkContainerHasImage(t, container, testContourImage)
	checkDaemonSetHasEnvVar(t, ds, EnvoyContainerName, envoyNsEnvVar)
	checkDaemonSetHasEnvVar(t, ds, EnvoyContainerName, envoyPodEnvVar)
	checkDaemonSetHasEnvVar(t, ds, envoyInitContainerName, envoyNsEnvVar)
	checkDaemonSetHasLabels(t, ds, cntr.AppLabels())
	for _, port := range cntr.Spec.NetworkPublishing.Envoy.Ports {
		checkContainerHasPort(t, ds, port.ContainerPort)
	}
	checkDaemonSetHasNodeSelector(t, ds, nil)
	checkDaemonSetHasTolerations(t, ds, nil)
	checkDaemonSecurityContext(t, ds)
	checkDaemonSetHasVolume(t, ds, volTest, volTestMount)
	checkDaemonSetHasPodAnnotations(t, ds, envoyPodAnnotations(cntr))
	checkDaemonSetHasMetricsPort(t, ds, 8002)

	checkDaemonSetHasResourceRequirements(t, ds, resQutoa)
	checkDaemonSetHasUpdateStrategy(t, ds, cntr.Spec.EnvoyDaemonSetUpdateStrategy)
}

func TestDesiredDeployment(t *testing.T) {
	name := "deploy-test"
	cntr := model.Default(fmt.Sprintf("%s-ns", name), name)

	testContourImage := "ghcr.io/projectcontour/contour:test"
	testEnvoyImage := "docker.io/envoyproxy/envoy:test"
	deploy := desiredDeployment(cntr, testContourImage, testEnvoyImage)
	checkDeploymentHasStrategy(t, deploy, cntr.Spec.EnvoyDeploymentStrategy)

}

func TestNodePlacementDaemonSet(t *testing.T) {
	name := "selector-test"
	cntr := model.Default(fmt.Sprintf("%s-ns", name), name)

	selectors := map[string]string{"node-role": "envoy"}
	tolerations := []corev1.Toleration{
		{
			Operator: corev1.TolerationOpExists,
			Key:      "node-role",
			Value:    "envoy",
			Effect:   corev1.TaintEffectNoSchedule,
		},
	}

	cntr.Spec.NodePlacement = &model.NodePlacement{
		Envoy: &model.EnvoyNodePlacement{
			NodeSelector: selectors,
			Tolerations:  tolerations,
		},
	}

	testContourImage := "ghcr.io/projectcontour/contour:test"
	testEnvoyImage := "docker.io/envoyproxy/envoy:test"
	ds := DesiredDaemonSet(cntr, testContourImage, testEnvoyImage)
	checkDaemonSetHasNodeSelector(t, ds, selectors)
	checkDaemonSetHasTolerations(t, ds, tolerations)
}

func TestEnvoyCustomPorts(t *testing.T) {
	name := "envoy-runtime-ports"
	cntr := model.Default(fmt.Sprintf("%s-ns", name), name)
	cntr.Spec.RuntimeSettings = &v1alpha1.ContourConfigurationSpec{
		Envoy: &v1alpha1.EnvoyConfig{
			Health: &v1alpha1.HealthConfig{
				Port: 8020,
			},
			Metrics: &v1alpha1.MetricsConfig{
				Port: 9090,
			},
		},
	}

	testContourImage := "ghcr.io/projectcontour/contour:test"
	testEnvoyImage := "docker.io/envoyproxy/envoy:test"
	ds := DesiredDaemonSet(cntr, testContourImage, testEnvoyImage)
	checkDaemonSetHasMetricsPort(t, ds, 9090)

	container := checkDaemonSetHasContainer(t, ds, EnvoyContainerName, true)
	checkContainerHasReadinessPort(t, container, 8020)
}
