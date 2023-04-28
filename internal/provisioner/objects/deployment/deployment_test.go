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
	"testing"

	"github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/provisioner/model"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"
)

func checkDeploymentHasEnvVar(t *testing.T, deploy *appsv1.Deployment, name string) {
	t.Helper()

	for _, envVar := range deploy.Spec.Template.Spec.Containers[0].Env {
		if envVar.Name == name {
			return
		}
	}
	t.Errorf("deployment is missing environment variable %q", name)
}

func checkDeploymentHasContainer(t *testing.T, deploy *appsv1.Deployment, name string, expect bool) *corev1.Container {
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

func checkDeploymentHasLabels(t *testing.T, deploy *appsv1.Deployment, expected map[string]string) {
	t.Helper()

	if apiequality.Semantic.DeepEqual(deploy.Labels, expected) {
		return
	}

	t.Errorf("deployment has unexpected %q labels", deploy.Labels)
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

func checkContainerHasImage(t *testing.T, container *corev1.Container, image string) {
	t.Helper()

	if container.Image == image {
		return
	}
	t.Errorf("container is missing image %q", image)
}

func checkDeploymentHasNodeSelector(t *testing.T, deploy *appsv1.Deployment, expected map[string]string) {
	t.Helper()

	if apiequality.Semantic.DeepEqual(deploy.Spec.Template.Spec.NodeSelector, expected) {
		return
	}
	t.Errorf("deployment has unexpected node selector %q", expected)
}

func checkDeploymentHasTolerations(t *testing.T, deploy *appsv1.Deployment, expected []corev1.Toleration) {
	t.Helper()

	if apiequality.Semantic.DeepEqual(deploy.Spec.Template.Spec.Tolerations, expected) {
		return
	}
	t.Errorf("deployment has unexpected tolerations %v", expected)
}

func checkDeploymentHasResourceRequirements(t *testing.T, deploy *appsv1.Deployment, expected corev1.ResourceRequirements) {
	t.Helper()

	if apiequality.Semantic.DeepEqual(deploy.Spec.Template.Spec.Containers[0].Resources, expected) {
		return
	}
	t.Errorf("daemonset has unexpected resource requirements %v", expected)
}

func checkDeploymentHasStrategy(t *testing.T, ds *appsv1.Deployment, expected appsv1.DeploymentStrategy) {
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
	cntr.Spec.ContourResources = resQutoa

	// Change the Kubernetes log level to test --kubernetes-debug.
	cntr.Spec.KubernetesLogLevel = 7

	// Change the Contour log level to test --debug.
	cntr.Spec.ContourLogLevel = v1alpha1.DebugLog

	cntr.Spec.ResourceLabels = map[string]string{
		"key": "value",
	}

	cntr.Spec.DisabledFeatures = "feature1,feature2"

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
	checkDeploymentHasLabels(t, deploy, cntr.AppLabels())

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

	arg = fmt.Sprintf("--disable-feature=%s", cntr.Spec.DisabledFeatures)
	checkContainerHasArg(t, container, arg)

	checkDeploymentHasNodeSelector(t, deploy, nil)
	checkDeploymentHasTolerations(t, deploy, nil)
	checkDeploymentHasResourceRequirements(t, deploy, resQutoa)
	checkDeploymentHasStrategy(t, deploy, cntr.Spec.ContourDeploymentStrategy)
}

func TestNodePlacementDeployment(t *testing.T) {
	name := "selector-test"
	cntr := model.Default(fmt.Sprintf("%s-ns", name), name)

	selectors := map[string]string{"node-role": "contour"}
	tolerations := []corev1.Toleration{
		{
			Operator: corev1.TolerationOpExists,
			Key:      "node-role",
			Value:    "contour",
			Effect:   corev1.TaintEffectNoSchedule,
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
