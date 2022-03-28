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

	operatorv1alpha1 "github.com/projectcontour/contour-operator/api/v1alpha1"
	objcontour "github.com/projectcontour/contour-operator/internal/objects/contour"
	objcfg "github.com/projectcontour/contour-operator/internal/objects/sharedconfig"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
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

func TestDesiredDeployment(t *testing.T) {
	name := "deploy-test"
	cfg := objcontour.Config{
		Name:        name,
		Namespace:   fmt.Sprintf("%s-ns", name),
		SpecNs:      "projectcontour",
		RemoveNs:    false,
		NetworkType: operatorv1alpha1.LoadBalancerServicePublishingType,
	}
	cntr := objcontour.New(cfg)
	icName := "test-ic"
	cntr.Spec.IngressClassName = &icName
	// Change the default ports to test Envoy service port args.
	insecurePort := objcfg.EnvoyInsecureContainerPort
	securePort := objcfg.EnvoySecureContainerPort
	for i, p := range cntr.Spec.NetworkPublishing.Envoy.ContainerPorts {
		if p.Name == "http" && p.PortNumber == insecurePort {
			cntr.Spec.NetworkPublishing.Envoy.ContainerPorts[i].PortNumber = int32(8081)
		}
		if p.Name == "https" && p.PortNumber == securePort {
			cntr.Spec.NetworkPublishing.Envoy.ContainerPorts[i].PortNumber = int32(8444)
		}
	}

	testContourImage := "ghcr.io/projectcontour/contour:test"
	deploy := DesiredDeployment(cntr, testContourImage)

	container := checkDeploymentHasContainer(t, deploy, contourContainerName, true)
	checkContainerHasImage(t, container, testContourImage)
	checkDeploymentHasEnvVar(t, deploy, contourNsEnvVar)
	checkDeploymentHasEnvVar(t, deploy, contourPodEnvVar)
	checkDeploymentHasLabels(t, deploy, deploy.Labels)

	for _, port := range container.Ports {
		if port.Name == "http" && port.ContainerPort != insecurePort {
			arg := fmt.Sprintf("--envoy-service-http-port=%d", port.ContainerPort)
			checkContainerHasArg(t, container, arg)
		}
		if port.Name == "https" && port.ContainerPort != securePort {
			arg := fmt.Sprintf("--envoy-service-https-port=%d", port.ContainerPort)
			checkContainerHasArg(t, container, arg)
		}
	}

	arg := fmt.Sprintf("--ingress-class-name=%s", *cntr.Spec.IngressClassName)
	checkContainerHasArg(t, container, arg)
	checkDeploymentHasNodeSelector(t, deploy, nil)
	checkDeploymentHasTolerations(t, deploy, nil)
}

func TestNodePlacementDeployment(t *testing.T) {
	name := "selector-test"
	selectors := map[string]string{"node-role": "contour"}
	tolerations := []corev1.Toleration{
		{
			Operator: corev1.TolerationOpExists,
			Key:      "node-role",
			Value:    "contour",
			Effect:   corev1.TaintEffectNoSchedule,
		},
	}
	cfg := objcontour.Config{
		Name:        name,
		Namespace:   fmt.Sprintf("%s-ns", name),
		SpecNs:      "projectcontour",
		RemoveNs:    false,
		NetworkType: operatorv1alpha1.LoadBalancerServicePublishingType,
	}
	cntr := objcontour.New(cfg)
	cntr.Spec.NodePlacement = &operatorv1alpha1.NodePlacement{
		Contour: &operatorv1alpha1.ContourNodePlacement{
			NodeSelector: selectors,
			Tolerations:  tolerations,
		},
	}

	deploy := DesiredDeployment(cntr, "ghcr.io/projectcontour/contour:test")

	checkDeploymentHasNodeSelector(t, deploy, selectors)
	checkDeploymentHasTolerations(t, deploy, tolerations)
}
