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

	"github.com/projectcontour/contour/internal/provisioner/model"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
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

func checkDaemonSetHasTolerations(t *testing.T, ds *appsv1.DaemonSet, expected []corev1.Toleration) {
	t.Helper()

	if apiequality.Semantic.DeepEqual(ds.Spec.Template.Spec.Tolerations, expected) {
		return
	}
	t.Errorf("deployment has unexpected tolerations %v", expected)
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

func TestDesiredDaemonSet(t *testing.T) {
	name := "ds-test"
	cntr := model.Default(fmt.Sprintf("%s-ns", name), name)

	testContourImage := "ghcr.io/projectcontour/contour:test"
	testEnvoyImage := "docker.io/envoyproxy/envoy:test"
	ds := DesiredDaemonSet(cntr, testContourImage, testEnvoyImage)
	container := checkDaemonSetHasContainer(t, ds, EnvoyContainerName, true)
	checkContainerHasImage(t, container, testEnvoyImage)
	container = checkDaemonSetHasContainer(t, ds, ShutdownContainerName, true)
	checkContainerHasImage(t, container, testContourImage)
	container = checkDaemonSetHasContainer(t, ds, envoyInitContainerName, true)
	checkContainerHasImage(t, container, testContourImage)
	checkDaemonSetHasEnvVar(t, ds, EnvoyContainerName, envoyNsEnvVar)
	checkDaemonSetHasEnvVar(t, ds, EnvoyContainerName, envoyPodEnvVar)
	checkDaemonSetHasEnvVar(t, ds, envoyInitContainerName, envoyNsEnvVar)
	checkDaemonSetHasLabels(t, ds, ds.Labels)
	for _, port := range cntr.Spec.NetworkPublishing.Envoy.ContainerPorts {
		checkContainerHasPort(t, ds, port.PortNumber)
	}
	checkDaemonSetHasNodeSelector(t, ds, nil)
	checkDaemonSetHasTolerations(t, ds, nil)
	checkDaemonSecurityContext(t, ds)
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
