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
	"context"
	"fmt"
	"path/filepath"

	"github.com/projectcontour/contour/internal/provisioner/equality"
	"github.com/projectcontour/contour/internal/provisioner/labels"
	"github.com/projectcontour/contour/internal/provisioner/model"
	"github.com/projectcontour/contour/internal/provisioner/objects"
	"github.com/projectcontour/contour/internal/ref"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// EnvoyContainerName is the name of the Envoy container.
	EnvoyContainerName = "envoy"
	// ShutdownContainerName is the name of the Shutdown Manager container.
	ShutdownContainerName = "shutdown-manager"
	// envoyInitContainerName is the name of the Envoy init container.
	envoyInitContainerName = "envoy-initconfig"
	// envoyNsEnvVar is the name of the contour namespace environment variable.
	envoyNsEnvVar = "CONTOUR_NAMESPACE"
	// envoyPodEnvVar is the name of the Envoy pod name environment variable.
	envoyPodEnvVar = "ENVOY_POD_NAME"
	// envoyCertsVolName is the name of the contour certificates volume.
	envoyCertsVolName = "envoycert"
	// envoyCertsVolMntDir is the directory name of the Envoy certificates volume.
	envoyCertsVolMntDir = "certs"
	// envoyCfgVolName is the name of the Envoy configuration volume.
	envoyCfgVolName = "envoy-config"
	// envoyCfgVolMntDir is the directory name of the Envoy configuration volume.
	envoyCfgVolMntDir = "config"
	// envoyAdminVolName is the name of the Envoy admin volume.
	envoyAdminVolName = "envoy-admin"
	// envoyAdminVolMntDir is the directory name of the Envoy admin volume.
	envoyAdminVolMntDir = "admin"
	// envoyCfgFileName is the name of the Envoy configuration file.
	envoyCfgFileName = "envoy.json"
	// xdsResourceVersion is the version of the Envoy xdS resource types.
	xdsResourceVersion = "v3"
)

// EnsureDataPlane ensures an Envoy data plane (daemonset or deployment) exists for the given contour.
func EnsureDataPlane(ctx context.Context, cli client.Client, contour *model.Contour, contourImage, envoyImage string) error {

	switch contour.Spec.EnvoyWorkloadType {
	// If a Deployment was specified, provision a Deployment.
	case model.WorkloadTypeDeployment:
		desired := desiredDeployment(contour, contourImage, envoyImage)

		updater := func(ctx context.Context, cli client.Client, current, desired *appsv1.Deployment) error {
			differ := equality.DeploymentSelectorsDiffer(current, desired)
			if differ {
				return EnsureDataPlaneDeleted(ctx, cli, contour)
			}

			return updateDeploymentIfNeeded(ctx, cli, contour, current, desired)
		}

		return objects.EnsureObject(ctx, cli, desired, updater, &appsv1.Deployment{})

	// The default workload type is a DaemonSet.
	default:
		desired := DesiredDaemonSet(contour, contourImage, envoyImage)

		updater := func(ctx context.Context, cli client.Client, current, desired *appsv1.DaemonSet) error {
			differ := equality.DaemonSetSelectorsDiffer(current, desired)
			if differ {
				return EnsureDataPlaneDeleted(ctx, cli, contour)
			}

			return updateDaemonSetIfNeeded(ctx, cli, contour, current, desired)
		}

		return objects.EnsureObject(ctx, cli, desired, updater, &appsv1.DaemonSet{})
	}
}

// EnsureDataPlaneDeleted ensures the daemonset or deployment for the provided contour is deleted
// if Contour owner labels exist.
func EnsureDataPlaneDeleted(ctx context.Context, cli client.Client, contour *model.Contour) error {
	// Need to try deleting both the DaemonSet and the Deployment because
	// we don't know which one was actually created, since we're not yet
	// using finalizers so the Gateway spec is unavailable to us at deletion
	// time.

	dsObj := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: contour.Namespace,
			Name:      contour.EnvoyDataPlaneName(),
		},
	}

	if err := objects.EnsureObjectDeleted(ctx, cli, dsObj, contour); err != nil {
		return err
	}

	deployObj := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: contour.Namespace,
			Name:      contour.EnvoyDataPlaneName(),
		},
	}

	return objects.EnsureObjectDeleted(ctx, cli, deployObj, contour)
}

func desiredContainers(contour *model.Contour, contourImage, envoyImage string) ([]corev1.Container, []corev1.Container) {
	var ports []corev1.ContainerPort
	for _, port := range contour.Spec.NetworkPublishing.Envoy.Ports {
		p := corev1.ContainerPort{
			Name:          port.Name,
			ContainerPort: port.ContainerPort,
			Protocol:      corev1.ProtocolTCP,
		}
		ports = append(ports, p)
	}

	healthPort := 8002
	if contour.Spec.RuntimeSettings != nil &&
		contour.Spec.RuntimeSettings.Envoy != nil &&
		contour.Spec.RuntimeSettings.Envoy.Health != nil &&
		contour.Spec.RuntimeSettings.Envoy.Health.Port > 0 {
		healthPort = contour.Spec.RuntimeSettings.Envoy.Health.Port
	}

	containers := []corev1.Container{
		{
			Name:            ShutdownContainerName,
			Image:           contourImage,
			ImagePullPolicy: corev1.PullIfNotPresent,
			Command: []string{
				"/bin/contour",
			},
			Args: []string{
				"envoy",
				"shutdown-manager",
			},
			Lifecycle: &corev1.Lifecycle{
				PreStop: &corev1.LifecycleHandler{
					Exec: &corev1.ExecAction{
						Command: []string{"/bin/contour", "envoy", "shutdown"},
					},
				},
			},
			TerminationMessagePolicy: corev1.TerminationMessageReadFile,
			TerminationMessagePath:   "/dev/termination-log",
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      envoyAdminVolName,
					MountPath: filepath.Join("/", envoyAdminVolMntDir),
				},
			},
		},
		{
			Name:            EnvoyContainerName,
			Image:           envoyImage,
			ImagePullPolicy: corev1.PullIfNotPresent,
			Command: []string{
				"envoy",
			},
			Args: []string{
				"-c",
				filepath.Join("/", envoyCfgVolMntDir, envoyCfgFileName),
				fmt.Sprintf("--service-cluster $(%s)", envoyNsEnvVar),
				fmt.Sprintf("--service-node $(%s)", envoyPodEnvVar),
				fmt.Sprintf("--log-level %s", contour.Spec.EnvoyLogLevel),
			},
			Env: []corev1.EnvVar{
				{
					Name: envoyNsEnvVar,
					ValueFrom: &corev1.EnvVarSource{
						FieldRef: &corev1.ObjectFieldSelector{
							APIVersion: "v1",
							FieldPath:  "metadata.namespace",
						},
					},
				},
				{
					Name: envoyPodEnvVar,
					ValueFrom: &corev1.EnvVarSource{
						FieldRef: &corev1.ObjectFieldSelector{
							APIVersion: "v1",
							FieldPath:  "metadata.name",
						},
					},
				},
			},
			ReadinessProbe: &corev1.Probe{
				FailureThreshold: int32(3),
				ProbeHandler: corev1.ProbeHandler{
					HTTPGet: &corev1.HTTPGetAction{
						Scheme: corev1.URISchemeHTTP,
						Path:   "/ready",
						Port:   intstr.IntOrString{IntVal: int32(healthPort)},
					},
				},
				InitialDelaySeconds: int32(3),
				PeriodSeconds:       int32(4),
				SuccessThreshold:    int32(1),
				TimeoutSeconds:      int32(1),
			},
			Ports: ports,
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      envoyCertsVolName,
					MountPath: filepath.Join("/", envoyCertsVolMntDir),
					ReadOnly:  true,
				},
				{
					Name:      envoyCfgVolName,
					MountPath: filepath.Join("/", envoyCfgVolMntDir),
					ReadOnly:  true,
				},
				{
					Name:      envoyAdminVolName,
					MountPath: filepath.Join("/", envoyAdminVolMntDir),
				},
			},
			Lifecycle: &corev1.Lifecycle{
				PreStop: &corev1.LifecycleHandler{
					HTTPGet: &corev1.HTTPGetAction{
						Path:   "/shutdown",
						Port:   intstr.FromInt(8090),
						Scheme: "HTTP",
					},
				},
			},
			TerminationMessagePolicy: corev1.TerminationMessageReadFile,
			TerminationMessagePath:   "/dev/termination-log",
			Resources:                contour.Spec.EnvoyResources,
		},
	}

	initContainers := []corev1.Container{
		{
			Name:            envoyInitContainerName,
			Image:           contourImage,
			ImagePullPolicy: corev1.PullIfNotPresent,
			Command: []string{
				"contour",
			},
			Args: []string{
				"bootstrap",
				filepath.Join("/", envoyCfgVolMntDir, envoyCfgFileName),
				fmt.Sprintf("--xds-address=%s", contour.ContourServiceName()),
				fmt.Sprintf("--xds-port=%d", objects.XDSPort),
				fmt.Sprintf("--xds-resource-version=%s", xdsResourceVersion),
				fmt.Sprintf("--resources-dir=%s", filepath.Join("/", envoyCfgVolMntDir, "resources")),
				fmt.Sprintf("--envoy-cafile=%s", filepath.Join("/", envoyCertsVolMntDir, "ca.crt")),
				fmt.Sprintf("--envoy-cert-file=%s", filepath.Join("/", envoyCertsVolMntDir, "tls.crt")),
				fmt.Sprintf("--envoy-key-file=%s", filepath.Join("/", envoyCertsVolMntDir, "tls.key")),
			},
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      envoyCertsVolName,
					MountPath: filepath.Join("/", envoyCertsVolMntDir),
					ReadOnly:  true,
				},
				{
					Name:      envoyCfgVolName,
					MountPath: filepath.Join("/", envoyCfgVolMntDir),
					ReadOnly:  false,
				},
			},
			Env: []corev1.EnvVar{
				{
					Name: envoyNsEnvVar,
					ValueFrom: &corev1.EnvVarSource{
						FieldRef: &corev1.ObjectFieldSelector{
							APIVersion: "v1",
							FieldPath:  "metadata.namespace",
						},
					},
				},
			},
			TerminationMessagePolicy: corev1.TerminationMessageReadFile,
			TerminationMessagePath:   "/dev/termination-log",
		},
	}

	for j := range containers {
		containers[j].VolumeMounts = append(containers[j].VolumeMounts, contour.Spec.EnvoyExtraVolumeMounts...)
	}
	return initContainers, containers
}

// DesiredDaemonSet returns the desired DaemonSet for the provided contour using
// contourImage as the shutdown-manager/envoy-initconfig container images and
// envoyImage as Envoy's container image.
func DesiredDaemonSet(contour *model.Contour, contourImage, envoyImage string) *appsv1.DaemonSet {
	initContainers, containers := desiredContainers(contour, contourImage, envoyImage)

	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: contour.Namespace,
			Name:      contour.EnvoyDataPlaneName(),
			Labels:    contour.AppLabels(),
		},
		Spec: appsv1.DaemonSetSpec{
			RevisionHistoryLimit: ref.To(int32(10)),
			// Ensure the deamonset adopts only its own pods.
			Selector:       EnvoyPodSelector(contour),
			UpdateStrategy: contour.Spec.EnvoyDaemonSetUpdateStrategy,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					// TODO [danehans]: Remove the prometheus annotations when Contour is updated to
					// show how the Prometheus Operator is used to scrape Contour/Envoy metrics.
					Annotations: envoyPodAnnotations(contour),
					Labels:      envoyPodLabels(contour),
				},
				Spec: corev1.PodSpec{
					Containers:     containers,
					InitContainers: initContainers,
					Volumes: []corev1.Volume{
						{
							Name: envoyCertsVolName,
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									DefaultMode: ref.To(int32(420)),
									SecretName:  contour.EnvoyCertsSecretName(),
								},
							},
						},
						{
							Name: envoyCfgVolName,
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
						{
							Name: envoyAdminVolName,
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
					},
					ServiceAccountName:            contour.EnvoyRBACNames().ServiceAccount,
					AutomountServiceAccountToken:  ref.To(false),
					TerminationGracePeriodSeconds: ref.To(int64(300)),
					SecurityContext:               objects.NewUnprivilegedPodSecurity(),
					DNSPolicy:                     corev1.DNSClusterFirst,
					RestartPolicy:                 corev1.RestartPolicyAlways,
					SchedulerName:                 "default-scheduler",
				},
			},
		},
	}

	ds.Spec.Template.Spec.Volumes = append(ds.Spec.Template.Spec.Volumes, contour.Spec.EnvoyExtraVolumes...)

	if contour.EnvoyNodeSelectorExists() {
		ds.Spec.Template.Spec.NodeSelector = contour.Spec.NodePlacement.Envoy.NodeSelector
	}

	if contour.EnvoyTolerationsExist() {
		ds.Spec.Template.Spec.Tolerations = contour.Spec.NodePlacement.Envoy.Tolerations
	}

	return ds
}

func desiredDeployment(contour *model.Contour, contourImage, envoyImage string) *appsv1.Deployment {
	initContainers, containers := desiredContainers(contour, contourImage, envoyImage)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: contour.Namespace,
			Name:      contour.EnvoyDataPlaneName(),
			Labels:    contour.AppLabels(),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas:             ref.To(contour.Spec.EnvoyReplicas),
			RevisionHistoryLimit: ref.To(int32(10)),
			// Ensure the deamonset adopts only its own pods.
			Selector: EnvoyPodSelector(contour),
			Strategy: contour.Spec.EnvoyDeploymentStrategy,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					// TODO [danehans]: Remove the prometheus annotations when Contour is updated to
					// show how the Prometheus Operator is used to scrape Contour/Envoy metrics.
					Annotations: envoyPodAnnotations(contour),
					Labels:      envoyPodLabels(contour),
				},
				Spec: corev1.PodSpec{
					// TODO anti-affinity
					Affinity:       nil,
					Containers:     containers,
					InitContainers: initContainers,
					Volumes: []corev1.Volume{
						{
							Name: envoyCertsVolName,
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									DefaultMode: ref.To(int32(420)),
									SecretName:  contour.EnvoyCertsSecretName(),
								},
							},
						},
						{
							Name: envoyCfgVolName,
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
						{
							Name: envoyAdminVolName,
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
					},
					ServiceAccountName:            contour.EnvoyRBACNames().ServiceAccount,
					AutomountServiceAccountToken:  ref.To(false),
					TerminationGracePeriodSeconds: ref.To(int64(300)),
					SecurityContext:               objects.NewUnprivilegedPodSecurity(),
					DNSPolicy:                     corev1.DNSClusterFirst,
					RestartPolicy:                 corev1.RestartPolicyAlways,
					SchedulerName:                 "default-scheduler",
				},
			},
		},
	}

	deployment.Spec.Template.Spec.Volumes = append(deployment.Spec.Template.Spec.Volumes, contour.Spec.EnvoyExtraVolumes...)

	if contour.EnvoyNodeSelectorExists() {
		deployment.Spec.Template.Spec.NodeSelector = contour.Spec.NodePlacement.Envoy.NodeSelector
	}

	if contour.EnvoyTolerationsExist() {
		deployment.Spec.Template.Spec.Tolerations = contour.Spec.NodePlacement.Envoy.Tolerations
	}

	return deployment
}

// updateDaemonSetIfNeeded updates a DaemonSet if current does not match desired,
// using contour to verify the existence of owner labels.
func updateDaemonSetIfNeeded(ctx context.Context, cli client.Client, contour *model.Contour, current, desired *appsv1.DaemonSet) error {
	if labels.Exist(current, model.OwnerLabels(contour)) {
		ds, updated := equality.DaemonsetConfigChanged(current, desired)
		if updated {
			if err := cli.Update(ctx, ds); err != nil {
				return fmt.Errorf("failed to update daemonset %s/%s: %w", ds.Namespace, ds.Name, err)
			}
			return nil
		}
	}
	return nil
}

// updateDeploymentIfNeeded updates a Deployment if current does not match desired,
// using contour to verify the existence of owner labels.
func updateDeploymentIfNeeded(ctx context.Context, cli client.Client, contour *model.Contour, current, desired *appsv1.Deployment) error {
	if labels.Exist(current, model.OwnerLabels(contour)) {
		ds, updated := equality.DeploymentConfigChanged(current, desired)
		if updated {
			if err := cli.Update(ctx, ds); err != nil {
				return fmt.Errorf("failed to update deployment %s/%s: %w", ds.Namespace, ds.Name, err)
			}
			return nil
		}
	}
	return nil
}

// EnvoyPodSelector returns a label selector using "app: envoy" as the
// key/value pair.
func EnvoyPodSelector(contour *model.Contour) *metav1.LabelSelector {
	return &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"app": contour.EnvoyDataPlaneName(),
		},
	}
}

// envoyPodLabels returns the labels for envoy's pods
func envoyPodLabels(contour *model.Contour) map[string]string {
	labels := EnvoyPodSelector(contour).MatchLabels
	for k, v := range contour.AppLabels() {
		labels[k] = v
	}
	return labels
}

// envoyPodAnnotations returns the annotations for envoy's pods
func envoyPodAnnotations(contour *model.Contour) map[string]string {
	annotations := map[string]string{}
	for k, v := range contour.Spec.EnvoyPodAnnotations {
		annotations[k] = v
	}

	metricsPort := 8002
	if contour.Spec.RuntimeSettings != nil &&
		contour.Spec.RuntimeSettings.Envoy != nil &&
		contour.Spec.RuntimeSettings.Envoy.Metrics != nil &&
		contour.Spec.RuntimeSettings.Envoy.Metrics.Port > 0 {
		metricsPort = contour.Spec.RuntimeSettings.Envoy.Metrics.Port
	}

	annotations["prometheus.io/scrape"] = "true"
	annotations["prometheus.io/port"] = fmt.Sprint(metricsPort)
	annotations["prometheus.io/path"] = "/stats/prometheus"

	return annotations
}
