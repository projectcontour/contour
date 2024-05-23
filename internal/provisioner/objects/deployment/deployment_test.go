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

package deployment

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	apps_v1 "k8s.io/api/apps/v1"
	core_v1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/provisioner/model"
)

func checkDeploymentHasEnvVar(t *testing.T, deploy *apps_v1.Deployment, name string) {
	t.Helper()

	for _, envVar := range deploy.Spec.Template.Spec.Containers[0].Env {
		if envVar.Name == name {
			return
		}
	}
	t.Errorf("deployment is missing environment variable %q", name)
}

func checkDeploymentHasContainer(t *testing.T, deploy *apps_v1.Deployment, name string, expect bool) *core_v1.Container {
	t.Helper()

	if deploy.Spec.Template.Spec.Containers == nil {
		t.Error("deployment has no containers")
	}

	for _, container := range deploy.Spec.Template.Spec.Containers {
		if container.Name == name {
			if expect {
				return &container
			}
			t.Errorf("deployment has unexpected %q container", name)
		}
	}
	if expect {
		t.Errorf("deployment has no %q container", name)
	}
	return nil
}

func checkDeploymentHasLabels(t *testing.T, deploy *apps_v1.Deployment, expected map[string]string) {
	t.Helper()

	if apiequality.Semantic.DeepEqual(deploy.Labels, expected) {
		return
	}

	t.Errorf("deployment has unexpected %q labels", deploy.Labels)
}

func checkPodHasAnnotation(t *testing.T, tmpl core_v1.PodTemplateSpec, key, value string) {
	t.Helper()

	require.Contains(t, tmpl.Annotations, key)
	require.Equal(t, value, tmpl.Annotations[key])
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

func checkContainerHasImage(t *testing.T, container *core_v1.Container, image string) {
	t.Helper()

	if container.Image == image {
		return
	}
	t.Errorf("container is missing image %q", image)
}

func checkDeploymentHasNodeSelector(t *testing.T, deploy *apps_v1.Deployment, expected map[string]string) {
	t.Helper()

	if apiequality.Semantic.DeepEqual(deploy.Spec.Template.Spec.NodeSelector, expected) {
		return
	}
	t.Errorf("deployment has unexpected node selector %q", expected)
}

func checkDeploymentHasTolerations(t *testing.T, deploy *apps_v1.Deployment, expected []core_v1.Toleration) {
	t.Helper()

	if apiequality.Semantic.DeepEqual(deploy.Spec.Template.Spec.Tolerations, expected) {
		return
	}
	t.Errorf("deployment has unexpected tolerations %v", expected)
}

func checkDeploymentHasResourceRequirements(t *testing.T, deploy *apps_v1.Deployment, expected core_v1.ResourceRequirements) {
	t.Helper()

	if apiequality.Semantic.DeepEqual(deploy.Spec.Template.Spec.Containers[0].Resources, expected) {
		return
	}
	t.Errorf("daemonset has unexpected resource requirements %v", expected)
}

func checkDeploymentHasStrategy(t *testing.T, ds *apps_v1.Deployment, expected apps_v1.DeploymentStrategy) {
	t.Helper()

	if apiequality.Semantic.DeepEqual(ds.Spec.Strategy, expected) {
		return
	}
	t.Errorf("deployment has unexpected strategy %q", expected)
}

func TestDesiredDeployment(t *testing.T) {
	name := "deploy-test"
	cntr := model.Default(fmt.Sprintf("%s-ns", name), name)
	icName := "test-ic"
	cntr.Spec.IngressClassName = &icName

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

	cntr.Spec.ContourResources = resQutoa

	// Change the Kubernetes log level to test --kubernetes-debug.
	cntr.Spec.KubernetesLogLevel = 7

	// Change the Contour log level to test --debug.
	cntr.Spec.ContourLogLevel = contour_v1alpha1.DebugLog

	cntr.Spec.ResourceLabels = map[string]string{
		"key": "value",
	}
	cntr.Spec.ContourPodAnnotations = map[string]string{
		"key":                  "value",
		"prometheus.io/scrape": "false",
	}
	cntr.Spec.ResourceAnnotations = map[string]string{
		"other-annotation": "other-val",
	}

	// Use non-default container ports to test that --envoy-service-http(s)-port
	// flags are added.
	cntr.Spec.NetworkPublishing.Envoy.Ports = []model.Port{
		{Name: "http", ServicePort: 80, ContainerPort: 8081},
		{Name: "https", ServicePort: 443, ContainerPort: 8444},
	}

	testContourImage := "ghcr.io/projectcontour/contour:test"
	deploy := DesiredDeployment(cntr, testContourImage)

	container := checkDeploymentHasContainer(t, deploy, contourContainerName, true)
	checkContainerHasImage(t, container, testContourImage)
	checkDeploymentHasEnvVar(t, deploy, contourNsEnvVar)
	checkDeploymentHasEnvVar(t, deploy, contourPodEnvVar)
	checkDeploymentHasLabels(t, deploy, cntr.WorkloadLabels())
	checkPodHasAnnotation(t, deploy.Spec.Template, "key", "value")
	checkPodHasAnnotation(t, deploy.Spec.Template, "prometheus.io/scrape", "false")
	checkPodHasAnnotation(t, deploy.Spec.Template, "other-annotation", "other-val")

	for _, port := range cntr.Spec.NetworkPublishing.Envoy.Ports {
		switch port.Name {
		case "http":
			arg := fmt.Sprintf("--envoy-service-http-port=%d", port.ContainerPort)
			checkContainerHasArg(t, container, arg)
		case "https":
			arg := fmt.Sprintf("--envoy-service-https-port=%d", port.ContainerPort)
			checkContainerHasArg(t, container, arg)
		default:
			t.Errorf("Unexpected port %s", port.Name)
		}
	}

	checkContainerHasArg(t, container, "--debug")

	arg := fmt.Sprintf("--ingress-class-name=%s", *cntr.Spec.IngressClassName)
	checkContainerHasArg(t, container, arg)

	arg = fmt.Sprintf("--kubernetes-debug=%d", cntr.Spec.KubernetesLogLevel)
	checkContainerHasArg(t, container, arg)

	checkDeploymentHasNodeSelector(t, deploy, nil)
	checkDeploymentHasTolerations(t, deploy, nil)
	checkDeploymentHasResourceRequirements(t, deploy, resQutoa)
	checkDeploymentHasStrategy(t, deploy, cntr.Spec.ContourDeploymentStrategy)
}

func TestDesiredDeploymentWhenSettingWatchNamespaces(t *testing.T) {
	testCases := []struct {
		description string
		namespaces  []contour_v1.Namespace
	}{
		{
			description: "several valid namespaces",
			namespaces:  []contour_v1.Namespace{"ns1", "ns2"},
		},
		{
			description: "single valid namespace",
			namespaces:  []contour_v1.Namespace{"ns1"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			name := "deploy-test"
			cntr := model.Default(fmt.Sprintf("%s-ns", name), name)
			icName := "test-ic"
			cntr.Spec.IngressClassName = &icName
			// Change the Contour watch namespaces flag
			cntr.Spec.WatchNamespaces = tc.namespaces
			deploy := DesiredDeployment(cntr, "ghcr.io/projectcontour/contour:test")
			container := checkDeploymentHasContainer(t, deploy, contourContainerName, true)
			arg := fmt.Sprintf("--watch-namespaces=%s", strings.Join(append(model.NamespacesToStrings(tc.namespaces), cntr.Namespace), ","))
			checkContainerHasArg(t, container, arg)
		})
	}
}

func TestNodePlacementDeployment(t *testing.T) {
	name := "selector-test"
	cntr := model.Default(fmt.Sprintf("%s-ns", name), name)

	selectors := map[string]string{"node-role": "contour"}
	tolerations := []core_v1.Toleration{
		{
			Operator: core_v1.TolerationOpExists,
			Key:      "node-role",
			Value:    "contour",
			Effect:   core_v1.TaintEffectNoSchedule,
		},
	}

	cntr.Spec.NodePlacement = &model.NodePlacement{
		Contour: &model.ContourNodePlacement{
			NodeSelector: selectors,
			Tolerations:  tolerations,
		},
	}

	deploy := DesiredDeployment(cntr, "ghcr.io/projectcontour/contour:test")

	checkDeploymentHasNodeSelector(t, deploy, selectors)
	checkDeploymentHasTolerations(t, deploy, tolerations)
}

func TestDesiredDeploymentWhenSettingDisabledFeature(t *testing.T) {
	testCases := []struct {
		description      string
		disabledFeatures []contour_v1.Feature
	}{
		{
			description:      "disable 2 features",
			disabledFeatures: []contour_v1.Feature{"tlsroutes", "grpcroutes"},
		},
		{
			description:      "disable single feature",
			disabledFeatures: []contour_v1.Feature{"tlsroutes"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			name := "deploy-test"
			cntr := model.Default(fmt.Sprintf("%s-ns", name), name)
			icName := "test-ic"
			cntr.Spec.IngressClassName = &icName
			cntr.Spec.DisabledFeatures = tc.disabledFeatures
			// Change the Contour watch namespaces flag
			deploy := DesiredDeployment(cntr, "ghcr.io/projectcontour/contour:test")
			container := checkDeploymentHasContainer(t, deploy, contourContainerName, true)
			for _, f := range tc.disabledFeatures {
				arg := fmt.Sprintf("--disable-feature=%s", string(f))
				checkContainerHasArg(t, container, arg)
			}
		})
	}
}
