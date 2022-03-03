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

package job

import (
	"fmt"
	"testing"

	"github.com/projectcontour/contour/internal/provisioner/model"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
)

func checkJobHasEnvVar(t *testing.T, job *batchv1.Job, name string) {
	t.Helper()

	for _, envVar := range job.Spec.Template.Spec.Containers[0].Env {
		if envVar.Name == name {
			return
		}
	}
	t.Errorf("job is missing environment variable %q", name)
}

func checkJobHasContainer(t *testing.T, job *batchv1.Job, name string) *corev1.Container {
	t.Helper()

	for _, container := range job.Spec.Template.Spec.Containers {
		if container.Name == name {
			return &container
		}
	}
	t.Errorf("job is missing container %q", name)
	return nil
}

func checkContainerHasImage(t *testing.T, container *corev1.Container, image string) {
	t.Helper()

	if container.Image == image {
		return
	}
	t.Errorf("container is missing image %q", image)
}

func TestDesiredJob(t *testing.T) {
	name := "job-test"
	cfg := model.Config{
		Name:        name,
		Namespace:   fmt.Sprintf("%s-ns", name),
		NetworkType: model.LoadBalancerServicePublishingType,
	}
	cntr := model.New(cfg)
	testContourImage := "ghcr.io/projectcontour/contour:test"
	job := DesiredJob(cntr, testContourImage)
	container := checkJobHasContainer(t, job, jobContainerName)
	checkContainerHasImage(t, container, testContourImage)
	checkJobHasEnvVar(t, job, jobNsEnvVar)
}
