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
	"context"
	"fmt"
	"time"

	"github.com/projectcontour/contour/internal/provisioner/equality"
	labels "github.com/projectcontour/contour/internal/provisioner/labels"
	"github.com/projectcontour/contour/internal/provisioner/model"
	objutil "github.com/projectcontour/contour/internal/provisioner/objects"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	jobContainerName = "contour"
	jobNsEnvVar      = "CONTOUR_NAMESPACE"
)

func certgenJobName(contourImage string) string {
	// [TODO] danehans: Remove and use contour.Name + "-certgen" when
	// https://github.com/projectcontour/contour/issues/2122 is fixed.
	return "contour-certgen-" + objutil.TagFromImage(contourImage)
}

// EnsureJob ensures that a Job exists for the given contour.
// TODO [danehans]: The real dependency is whether the TLS secrets are present.
// The method should first check for the secrets, then use certgen as a secret
// generating strategy.
func EnsureJob(ctx context.Context, cli client.Client, contour *model.Contour, image string) error {
	desired := DesiredJob(contour, image)
	current, err := currentJob(ctx, cli, contour, image)
	if err != nil {
		if errors.IsNotFound(err) {
			return createJob(ctx, cli, desired)
		}
		return fmt.Errorf("failed to get job %s/%s: %w", desired.Namespace, desired.Name, err)
	}
	if err := recreateJobIfNeeded(ctx, cli, contour, current, desired); err != nil {
		return fmt.Errorf("failed to recreate job %s/%s: %w", desired.Namespace, desired.Name, err)
	}
	return nil
}

// EnsureJobDeleted ensures the Job for the provided contour is deleted if
// Contour owner labels exist.
func EnsureJobDeleted(ctx context.Context, cli client.Client, contour *model.Contour, image string) error {
	job, err := currentJob(ctx, cli, contour, image)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}
	if labels.Exist(job, model.OwnerLabels(contour)) {
		if err := cli.Delete(ctx, job); err != nil {
			if !errors.IsNotFound(err) {
				return err
			}
			return nil
		}
	}
	return nil
}

// currentJob returns the current Job resource named name for the provided contour.
func currentJob(ctx context.Context, cli client.Client, contour *model.Contour, image string) (*batchv1.Job, error) {
	current := &batchv1.Job{}
	key := types.NamespacedName{
		Namespace: contour.Spec.Namespace.Name,
		Name:      certgenJobName(image),
	}
	err := cli.Get(ctx, key, current)
	if err != nil {
		return nil, err
	}
	return current, nil
}

// DesiredJob generates the desired Job resource using image for the given contour.
func DesiredJob(contour *model.Contour, image string) *batchv1.Job {
	env := corev1.EnvVar{
		Name: jobNsEnvVar,
		ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{
				APIVersion: "v1",
				FieldPath:  "metadata.namespace",
			},
		},
	}
	container := corev1.Container{
		Name:            jobContainerName,
		Image:           image,
		ImagePullPolicy: corev1.PullAlways,
		Command: []string{
			"contour",
			"certgen",
			"--kube",
			"--incluster",
			"--overwrite",
			"--secrets-format=compact",
			fmt.Sprintf("--namespace=$(%s)", jobNsEnvVar),
		},
		Env:                      []corev1.EnvVar{env},
		TerminationMessagePath:   "/dev/termination-log",
		TerminationMessagePolicy: "File",
	}
	spec := corev1.PodSpec{
		Containers:                    []corev1.Container{container},
		DeprecatedServiceAccount:      objutil.CertGenRbacName,
		ServiceAccountName:            objutil.CertGenRbacName,
		SecurityContext:               objutil.NewUnprivilegedPodSecurity(),
		RestartPolicy:                 corev1.RestartPolicyNever,
		DNSPolicy:                     corev1.DNSClusterFirst,
		SchedulerName:                 "default-scheduler",
		TerminationGracePeriodSeconds: pointer.Int64Ptr(int64(30)),
	}
	// TODO [danehans] certgen needs to be updated to match these labels.
	// See https://github.com/projectcontour/contour/issues/1821 for details.
	labels := map[string]string{
		"app.kubernetes.io/name":       "contour-certgen",
		"app.kubernetes.io/instance":   contour.Name,
		"app.kubernetes.io/component":  "ingress-controller",
		"app.kubernetes.io/part-of":    "project-contour",
		"app.kubernetes.io/managed-by": "contour-operator",
	}
	// Add owner labels
	for k, v := range model.OwnerLabels(contour) {
		labels[k] = v
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      certgenJobName(image),
			Namespace: contour.Spec.Namespace.Name,
			Labels:    labels,
		},
		Spec: batchv1.JobSpec{
			Parallelism:  pointer.Int32Ptr(int32(1)),
			Completions:  pointer.Int32Ptr(int32(1)),
			BackoffLimit: pointer.Int32Ptr(int32(1)),
			// Make job eligible to for immediate deletion (feature gate dependent).
			TTLSecondsAfterFinished: pointer.Int32Ptr(int32(0)),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: model.OwningSelector(contour).MatchLabels,
				},
				Spec: spec,
			},
		},
	}
	return job
}

// recreateJobIfNeeded recreates a Job if current doesn't match desired,
// using contour to verify the existence of owner labels.
func recreateJobIfNeeded(ctx context.Context, cli client.Client, contour *model.Contour, current, desired *batchv1.Job) error {
	if labels.Exist(current, model.OwnerLabels(contour)) {
		updated, changed := equality.JobConfigChanged(current, desired)
		if !changed {
			return nil
		}
		if err := cli.Delete(ctx, updated); err != nil {
			if !errors.IsNotFound(err) {
				return err
			}
		}
		// Retry is needed since the object may still be getting deleted.
		if err := retryJobCreate(ctx, cli, updated, time.Second*3); err != nil {
			return err
		}
	}
	return nil
}

// createJob creates a Job resource for the provided job.
func createJob(ctx context.Context, cli client.Client, job *batchv1.Job) error {
	if err := cli.Create(ctx, job); err != nil {
		return fmt.Errorf("failed to create job %s/%s: %w", job.Namespace, job.Name, err)
	}
	return nil
}

// retryJobCreate tries creating the provided Job, retrying every second
// until timeout is reached.
func retryJobCreate(ctx context.Context, cli client.Client, job *batchv1.Job, timeout time.Duration) error {
	err := wait.PollImmediate(1*time.Second, timeout, func() (bool, error) {
		if err := cli.Create(ctx, job); err != nil {
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("failed to create job %s/%s: %w", job.Namespace, job.Name, err)
	}
	return nil
}
