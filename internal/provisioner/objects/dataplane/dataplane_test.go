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

	"github.com/stretchr/testify/require"
	apps_v1 "k8s.io/api/apps/v1"
	core_v1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"

	contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/provisioner/model"
	"github.com/projectcontour/contour/internal/provisioner/objects"
)

func checkDaemonSetHasEnvVar(t *testing.T, ds *apps_v1.DaemonSet, container, name string) {
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

func checkDaemonSetHasAnnotation(t *testing.T, ds *apps_v1.DaemonSet, key, value string) {
	t.Helper()

	require.Contains(t, ds.Spec.Template.Annotations, key)
	require.Equal(t, value, ds.Spec.Template.Annotations[key])
}

func checkDaemonSetHasContainer(t *testing.T, ds *apps_v1.DaemonSet, name string, expect bool) *core_v1.Container {
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

func checkDaemonSetHasLabels(t *testing.T, ds *apps_v1.DaemonSet, expected map[string]string) {
	t.Helper()

	if apiequality.Semantic.DeepEqual(ds.Labels, expected) {
		return
	}

	t.Errorf("daemonset has unexpected %q labels", ds.Labels)
}

func checkDaemonSetHasPodAnnotations(t *testing.T, ds *apps_v1.DaemonSet, expected map[string]string) {
	t.Helper()

	if apiequality.Semantic.DeepEqual(ds.Spec.Template.ObjectMeta.Annotations, expected) {
		return
	}

	t.Errorf("daemonset has unexpected %q pod annotations", ds.Spec.Template.Annotations)
}

func checkContainerHasPort(t *testing.T, ds *apps_v1.DaemonSet, port int32) {
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

func checkContainerHasImage(t *testing.T, container *core_v1.Container, image string) {
	t.Helper()

	if container.Image == image {
		return
	}
	t.Errorf("container is missing image %q", image)
}

func checkContainerHaveResourceRequirements(t *testing.T, container *core_v1.Container) {
	t.Helper()

	if apiequality.Semantic.DeepEqual(container.Resources, defContainerResources) {
		return
	}
	t.Errorf("container doesn't have resource requiremetns")
}

func checkDaemonSetHasNodeSelector(t *testing.T, ds *apps_v1.DaemonSet, expected map[string]string) {
	t.Helper()

	if apiequality.Semantic.DeepEqual(ds.Spec.Template.Spec.NodeSelector, expected) {
		return
	}
	t.Errorf("deployment has unexpected node selector %q", expected)
}

func checkDaemonSetHasVolume(t *testing.T, ds *apps_v1.DaemonSet, vol core_v1.Volume, volMount core_v1.VolumeMount) {
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

func checkDaemonSetHasResourceRequirements(t *testing.T, ds *apps_v1.DaemonSet, expected core_v1.ResourceRequirements) {
	t.Helper()

	if apiequality.Semantic.DeepEqual(ds.Spec.Template.Spec.Containers[1].Resources, expected) {
		return
	}
	t.Errorf("daemonset has unexpected resource requirements %v", expected)
}

func checkDaemonSetHasUpdateStrategy(t *testing.T, ds *apps_v1.DaemonSet, expected apps_v1.DaemonSetUpdateStrategy) {
	t.Helper()

	if apiequality.Semantic.DeepEqual(ds.Spec.UpdateStrategy, expected) {
		return
	}
	t.Errorf("daemonset has unexpected update strategy %q", expected)
}

func checkDeploymentHasStrategy(t *testing.T, ds *apps_v1.Deployment, expected apps_v1.DeploymentStrategy) {
	t.Helper()

	if apiequality.Semantic.DeepEqual(ds.Spec.Strategy, expected) {
		return
	}
	t.Errorf("deployment has unexpected strategy %q", expected)
}

func checkDaemonSetHasTolerations(t *testing.T, ds *apps_v1.DaemonSet, expected []core_v1.Toleration) {
	t.Helper()

	if apiequality.Semantic.DeepEqual(ds.Spec.Template.Spec.Tolerations, expected) {
		return
	}
	t.Errorf("daemonset has unexpected tolerations %v", expected)
}

func checkDaemonSecurityContext(t *testing.T, ds *apps_v1.DaemonSet) {
	t.Helper()

	user := int64(65534)
	group := int64(65534)
	nonRoot := true
	expected := &core_v1.PodSecurityContext{
		RunAsUser:    &user,
		RunAsGroup:   &group,
		RunAsNonRoot: &nonRoot,
	}
	if apiequality.Semantic.DeepEqual(ds.Spec.Template.Spec.SecurityContext, expected) {
		return
	}
	t.Errorf("deployment has unexpected SecurityContext %v", expected)
}

func checkContainerHasArg(t *testing.T, container *core_v1.Container, arg string) {
	t.Helper()

	for _, a := range container.Args {
		if a == arg {
			return
		}
	}
	t.Errorf("container is missing argument %q", arg)
}

func checkContainerHasReadinessPort(t *testing.T, container *core_v1.Container, port int32) {
	t.Helper()

	if container.ReadinessProbe != nil &&
		container.ReadinessProbe.HTTPGet != nil &&
		container.ReadinessProbe.HTTPGet.Port.IntVal == port {
		return
	}
	t.Errorf("container has unexpected readiness port %d", port)
}

func checkEnvoyDeploymentHasAffinity(t *testing.T, d *apps_v1.Deployment, contour *model.Contour) {
	t.Helper()
	if apiequality.Semantic.DeepEqual(*d.Spec.Template.Spec.Affinity,
		core_v1.Affinity{
			PodAntiAffinity: &core_v1.PodAntiAffinity{
				PreferredDuringSchedulingIgnoredDuringExecution: []core_v1.WeightedPodAffinityTerm{
					{
						Weight: int32(100),
						PodAffinityTerm: core_v1.PodAffinityTerm{
							LabelSelector: EnvoyPodSelector(contour),
							TopologyKey:   "kubernetes.io/hostname",
						},
					},
				},
			},
		}) {
		return
	}
	t.Errorf("container has unexpected affinity %v", d.Spec.Template.Spec.Affinity)
}

func TestDesiredDaemonSet(t *testing.T) {
	name := "ds-test"
	cntr := model.Default(fmt.Sprintf("%s-ns", name), name)
	cntr.Spec.ResourceLabels = map[string]string{
		"key": "val",
	}
	cntr.Spec.EnvoyPodAnnotations = map[string]string{
		"annotation":           "value",
		"prometheus.io/scrape": "false",
	}
	cntr.Spec.ResourceAnnotations = map[string]string{
		"other-annotation": "other-val",
	}

	volTest := core_v1.Volume{
		Name: "vol-test-mount",
	}
	volTestMount := core_v1.VolumeMount{
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
	testBaseIDArg := "--base-id 1"
	testEnvoyMaxHeapSize := "--overload-max-heap=8000000000"

	resQutoa := core_v1.ResourceRequirements{
		Limits: core_v1.ResourceList{
			core_v1.ResourceCPU:    resource.MustParse("400m"),
			core_v1.ResourceMemory: resource.MustParse("256Mi"),
		},
		Requests: core_v1.ResourceList{
			core_v1.ResourceCPU:    resource.MustParse("100m"),
			core_v1.ResourceMemory: resource.MustParse("25Mi"),
		},
	}
	cntr.Spec.EnvoyResources = resQutoa

	// Change the Envoy log level to test --log-level debug.
	cntr.Spec.EnvoyLogLevel = contour_v1alpha1.DebugLog
	cntr.Spec.RuntimeSettings = &contour_v1alpha1.ContourConfigurationSpec{
		Envoy: &contour_v1alpha1.EnvoyConfig{
			Metrics: &contour_v1alpha1.MetricsConfig{
				Port: int(objects.EnvoyMetricsPort),
			},
		},
	}
	// Change the Envoy base id to test --base-id 1
	cntr.Spec.EnvoyBaseID = 1

	cntr.Spec.EnvoyMaxHeapSizeBytes = 8000000000

	ds := DesiredDaemonSet(cntr, testContourImage, testEnvoyImage)
	container := checkDaemonSetHasContainer(t, ds, EnvoyContainerName, true)
	checkContainerHasArg(t, container, testLogLevelArg)
	checkContainerHasArg(t, container, testBaseIDArg)
	checkContainerHasImage(t, container, testEnvoyImage)
	checkContainerHasReadinessPort(t, container, 8002)

	container = checkDaemonSetHasContainer(t, ds, ShutdownContainerName, true)
	checkContainerHaveResourceRequirements(t, container)

	checkContainerHasImage(t, container, testContourImage)
	container = checkDaemonSetHasContainer(t, ds, envoyInitContainerName, true)
	checkContainerHaveResourceRequirements(t, container)

	checkContainerHasImage(t, container, testContourImage)
	checkContainerHasArg(t, container, testEnvoyMaxHeapSize)

	checkDaemonSetHasEnvVar(t, ds, EnvoyContainerName, envoyNsEnvVar)
	checkDaemonSetHasEnvVar(t, ds, EnvoyContainerName, envoyPodEnvVar)
	checkDaemonSetHasEnvVar(t, ds, envoyInitContainerName, envoyNsEnvVar)
	checkDaemonSetHasLabels(t, ds, cntr.WorkloadLabels())
	checkContainerHasPort(t, ds, int32(cntr.Spec.RuntimeSettings.Envoy.Metrics.Port)) //nolint:gosec // disable G115

	checkDaemonSetHasNodeSelector(t, ds, nil)
	checkDaemonSetHasTolerations(t, ds, nil)
	checkDaemonSecurityContext(t, ds)
	checkDaemonSetHasVolume(t, ds, volTest, volTestMount)
	checkDaemonSetHasPodAnnotations(t, ds, envoyPodAnnotations(cntr))
	checkDaemonSetHasAnnotation(t, ds, "annotation", "value")
	checkDaemonSetHasAnnotation(t, ds, "other-annotation", "other-val")
	checkDaemonSetHasAnnotation(t, ds, "prometheus.io/scrape", "false")

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
	checkEnvoyDeploymentHasAffinity(t, deploy, cntr)
}

func TestNodePlacementDaemonSet(t *testing.T) {
	name := "selector-test"
	cntr := model.Default(fmt.Sprintf("%s-ns", name), name)

	selectors := map[string]string{"node-role": "envoy"}
	tolerations := []core_v1.Toleration{
		{
			Operator: core_v1.TolerationOpExists,
			Key:      "node-role",
			Value:    "envoy",
			Effect:   core_v1.TaintEffectNoSchedule,
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
	metricPort := 9090
	cntr := model.Default(fmt.Sprintf("%s-ns", name), name)
	cntr.Spec.RuntimeSettings = &contour_v1alpha1.ContourConfigurationSpec{
		Envoy: &contour_v1alpha1.EnvoyConfig{
			Health: &contour_v1alpha1.HealthConfig{
				Port: 8020,
			},
			Metrics: &contour_v1alpha1.MetricsConfig{
				Port: metricPort,
			},
		},
	}

	testContourImage := "ghcr.io/projectcontour/contour:test"
	testEnvoyImage := "docker.io/envoyproxy/envoy:test"
	ds := DesiredDaemonSet(cntr, testContourImage, testEnvoyImage)
	checkContainerHasPort(t, ds, int32(metricPort))

	container := checkDaemonSetHasContainer(t, ds, EnvoyContainerName, true)
	checkContainerHasReadinessPort(t, container, 8020)
}
