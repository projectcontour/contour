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
	"errors"
	"time"

	apps_v1 "k8s.io/api/apps/v1"
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/projectcontour/contour/internal/k8s"
)

func WaitForContourDeploymentUpdated(deployment *apps_v1.Deployment, cli client.Client, image string) error {
	// List pods with app label "contour" and check that pods are updated
	// with expected container image and in ready state.
	// We do this instead of checking Deployment status as it is possible
	// for it not to have been updated yet and replicas not yet been shut
	// down.

	if len(deployment.Spec.Template.Spec.Containers) != 1 {
		return errors.New("invalid Contour Deployment containers spec")
	}
	labelSelectAppContour := labels.SelectorFromSet(deployment.Spec.Selector.MatchLabels)
	updatedPods := func(ctx context.Context) (bool, error) {
		updatedPods := getPodsUpdatedWithContourImage(ctx, labelSelectAppContour, deployment.Namespace, image, cli)

		return updatedPods == int(*deployment.Spec.Replicas), nil
	}
	return wait.PollUntilContextTimeout(context.Background(), time.Millisecond*50, time.Minute*3, true, updatedPods)
}

func WaitForEnvoyDaemonSetUpdated(daemonset *apps_v1.DaemonSet, cli client.Client, image string) error {
	labelSelectAppEnvoy := labels.SelectorFromSet(daemonset.Spec.Selector.MatchLabels)

	updatedPods := func(ctx context.Context) (bool, error) {
		ds := &apps_v1.DaemonSet{}
		if err := cli.Get(ctx, k8s.NamespacedNameOf(daemonset), ds); err != nil {
			return false, err
		}

		updatedPods := int(ds.Status.DesiredNumberScheduled)
		if len(daemonset.Spec.Template.Spec.Containers) > 1 {
			updatedPods = getPodsUpdatedWithContourImage(ctx, labelSelectAppEnvoy, daemonset.Namespace, image, cli)
		}
		return updatedPods == int(ds.Status.DesiredNumberScheduled) &&
			ds.Status.NumberReady > 0, nil
	}
	return wait.PollUntilContextTimeout(context.Background(), time.Millisecond*50, time.Minute*3, true, updatedPods)
}

func WaitForEnvoyDeploymentUpdated(deployment *apps_v1.Deployment, cli client.Client, image string) error {
	labelSelectAppEnvoy := labels.SelectorFromSet(deployment.Spec.Selector.MatchLabels)

	updatedPods := func(ctx context.Context) (bool, error) {
		dp := new(apps_v1.Deployment)
		if err := cli.Get(ctx, client.ObjectKeyFromObject(deployment), dp); err != nil {
			return false, err
		}
		updatedPods := int(dp.Status.UpdatedReplicas)
		if len(dp.Spec.Template.Spec.Containers) > 1 {
			updatedPods = getPodsUpdatedWithContourImage(ctx, labelSelectAppEnvoy, deployment.Namespace, image, cli)
		}
		return updatedPods == int(*deployment.Spec.Replicas) &&
			int(dp.Status.ReadyReplicas) == updatedPods &&
			int(dp.Status.UnavailableReplicas) == 0, nil
	}
	return wait.PollUntilContextTimeout(context.Background(), time.Millisecond*50, time.Minute*3, true, updatedPods)
}

func getPodsUpdatedWithContourImage(ctx context.Context, labelSelector labels.Selector, namespace, image string, cli client.Client) int {
	pods := new(core_v1.PodList)
	opts := &client.ListOptions{
		LabelSelector: labelSelector,
		Namespace:     namespace,
	}
	if err := cli.List(ctx, pods, opts); err != nil {
		return 0
	}
	updatedPods := 0
	for _, pod := range pods.Items {
		updated := false
		for _, container := range pod.Spec.Containers {
			if container.Image == image {
				updated = true
			}
		}
		if !updated {
			continue
		}

		for _, cond := range pod.Status.Conditions {
			if cond.Type == core_v1.PodReady && cond.Status == core_v1.ConditionTrue {
				updatedPods++
			}
		}
	}
	return updatedPods
}
