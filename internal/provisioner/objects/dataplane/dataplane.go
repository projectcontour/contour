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

	apps_v1 "k8s.io/api/apps/v1"
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/projectcontour/contour/internal/provisioner/equality"
	"github.com/projectcontour/contour/internal/provisioner/labels"
	"github.com/projectcontour/contour/internal/provisioner/model"
	"github.com/projectcontour/contour/internal/provisioner/objects"
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

// the default resource requirements for container: envoy-initconfig & shutdown-manager, the default value is come from:
// ref: https://projectcontour.io/docs/1.25/deploy-options/#setting-resource-requests-and-limits
var defContainerResources = core_v1.ResourceRequirements{
	Requests: core_v1.ResourceList{
		core_v1.ResourceCPU:    resource.MustParse("25m"),
		core_v1.ResourceMemory: resource.MustParse("50Mi"),
	},
	Limits: core_v1.ResourceList{
		core_v1.ResourceCPU:    resource.MustParse("50m"),
		core_v1.ResourceMemory: resource.MustParse("100Mi"),
	},
}

// EnsureDataPlane ensures an Envoy data plane (daemonset or deployment) exists for the given contour.
func EnsureDataPlane(ctx context.Context, cli client.Client, contour *model.Contour, contourImage, envoyImage string) error {
	switch contour.Spec.EnvoyWorkloadType {
	// If a Deployment was specified, provision a Deployment.
	case model.WorkloadTypeDeployment:
		desired := desiredDeployment(contour, contourImage, envoyImage)

		updater := func(ctx context.Context, cli client.Client, current, desired *apps_v1.Deployment) error {
			differ := equality.DeploymentSelectorsDiffer(current, desired)
			if differ {
				return EnsureDataPlaneDeleted(ctx, cli, contour)
			}

			return updateDeploymentIfNeeded(ctx, cli, contour, current, desired)
		}

		return objects.EnsureObject(ctx, cli, desired, updater, &apps_v1.Deployment{})

	// The default workload type is a DaemonSet.
	default:
		desired := DesiredDaemonSet(contour, contourImage, envoyImage)

		updater := func(ctx context.Context, cli client.Client, current, desired *apps_v1.DaemonSet) error {
			differ := equality.DaemonSetSelectorsDiffer(current, desired)
			if differ {
				return EnsureDataPlaneDeleted(ctx, cli, contour)
			}

			return updateDaemonSetIfNeeded(ctx, cli, contour, current, desired)
		}

		return objects.EnsureObject(ctx, cli, desired, updater, &apps_v1.DaemonSet{})
	}
}

// EnsureDataPlaneDeleted ensures the daemonset or deployment for the provided contour is deleted
// if Contour owner labels exist.
func EnsureDataPlaneDeleted(ctx context.Context, cli client.Client, contour *model.Contour) error {
	// Need to try deleting both the DaemonSet and the Deployment because
	// we don't know which one was actually created, since we're not yet
	// using finalizers so the Gateway spec is unavailable to us at deletion
	// time.

	dsObj := &apps_v1.DaemonSet{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: contour.Namespace,
			Name:      contour.EnvoyDataPlaneName(),
		},
	}

	if err := objects.EnsureObjectDeleted(ctx, cli, dsObj, contour); err != nil {
		return err
	}

	deployObj := &apps_v1.Deployment{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: contour.Namespace,
			Name:      contour.EnvoyDataPlaneName(),
		},
	}

	return objects.EnsureObjectDeleted(ctx, cli, deployObj, contour)
}

func desiredContainers(contour *model.Contour, contourImage, envoyImage string) ([]core_v1.Container, []core_v1.Container) {
	var (
		metricsPort = objects.EnvoyMetricsPort
		healthPort  = objects.EnvoyHealthPort
	)

	if contour.Spec.RuntimeSettings != nil &&
		contour.Spec.RuntimeSettings.Envoy != nil {

		if contour.Spec.RuntimeSettings.Envoy.Metrics != nil &&
			contour.Spec.RuntimeSettings.Envoy.Metrics.Port > 0 {
			metricsPort = int32(contour.Spec.RuntimeSettings.Envoy.Metrics.Port) //nolint:gosec // disable G115
		}

		if contour.Spec.RuntimeSettings.Envoy.Health != nil &&
			contour.Spec.RuntimeSettings.Envoy.Health.Port > 0 {
			healthPort = int32(contour.Spec.RuntimeSettings.Envoy.Health.Port) //nolint:gosec // disable G115
		}
	}

	ports := []core_v1.ContainerPort{{
		Name:          "metrics",
		ContainerPort: metricsPort,
		Protocol:      core_v1.ProtocolTCP,
	}}

	containers := []core_v1.Container{
		{
			Name:            ShutdownContainerName,
			Image:           contourImage,
			ImagePullPolicy: core_v1.PullIfNotPresent,
			Command: []string{
				"/bin/contour",
			},
			Args: []string{
				"envoy",
				"shutdown-manager",
			},
			Lifecycle: &core_v1.Lifecycle{
				PreStop: &core_v1.LifecycleHandler{
					Exec: &core_v1.ExecAction{
						Command: []string{"/bin/contour", "envoy", "shutdown"},
					},
				},
			},
			TerminationMessagePolicy: core_v1.TerminationMessageReadFile,
			TerminationMessagePath:   "/dev/termination-log",
			VolumeMounts: []core_v1.VolumeMount{
				{
					Name:      envoyAdminVolName,
					MountPath: filepath.Join("/", envoyAdminVolMntDir),
				},
			},

			Resources: defContainerResources,
		},
		{
			Name:            EnvoyContainerName,
			Image:           envoyImage,
			ImagePullPolicy: core_v1.PullIfNotPresent,
			Command: []string{
				"envoy",
			},
			Args: []string{
				"-c",
				filepath.Join("/", envoyCfgVolMntDir, envoyCfgFileName),
				fmt.Sprintf("--service-cluster $(%s)", envoyNsEnvVar),
				fmt.Sprintf("--service-node $(%s)", envoyPodEnvVar),
				fmt.Sprintf("--log-level %s", contour.Spec.EnvoyLogLevel),
				fmt.Sprintf("--base-id %d", contour.Spec.EnvoyBaseID),
			},
			Env: []core_v1.EnvVar{
				{
					Name: envoyNsEnvVar,
					ValueFrom: &core_v1.EnvVarSource{
						FieldRef: &core_v1.ObjectFieldSelector{
							APIVersion: "v1",
							FieldPath:  "metadata.namespace",
						},
					},
				},
				{
					Name: envoyPodEnvVar,
					ValueFrom: &core_v1.EnvVarSource{
						FieldRef: &core_v1.ObjectFieldSelector{
							APIVersion: "v1",
							FieldPath:  "metadata.name",
						},
					},
				},
			},
			ReadinessProbe: &core_v1.Probe{
				FailureThreshold: int32(3),
				ProbeHandler: core_v1.ProbeHandler{
					HTTPGet: &core_v1.HTTPGetAction{
						Scheme: core_v1.URISchemeHTTP,
						Path:   "/ready",
						Port:   intstr.IntOrString{IntVal: healthPort},
					},
				},
				InitialDelaySeconds: int32(3),
				PeriodSeconds:       int32(4),
				SuccessThreshold:    int32(1),
				TimeoutSeconds:      int32(1),
			},
			Ports: ports,
			VolumeMounts: []core_v1.VolumeMount{
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
			Lifecycle: &core_v1.Lifecycle{
				PreStop: &core_v1.LifecycleHandler{
					HTTPGet: &core_v1.HTTPGetAction{
						Path:   "/shutdown",
						Port:   intstr.FromInt(8090),
						Scheme: "HTTP",
					},
				},
			},
			TerminationMessagePolicy: core_v1.TerminationMessageReadFile,
			TerminationMessagePath:   "/dev/termination-log",
			Resources:                contour.Spec.EnvoyResources,
		},
	}

	initContainers := []core_v1.Container{
		{
			Name:            envoyInitContainerName,
			Image:           contourImage,
			ImagePullPolicy: core_v1.PullIfNotPresent,
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
				fmt.Sprintf("--overload-max-heap=%d", contour.Spec.EnvoyMaxHeapSizeBytes),
			},
			VolumeMounts: []core_v1.VolumeMount{
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
			Env: []core_v1.EnvVar{
				{
					Name: envoyNsEnvVar,
					ValueFrom: &core_v1.EnvVarSource{
						FieldRef: &core_v1.ObjectFieldSelector{
							APIVersion: "v1",
							FieldPath:  "metadata.namespace",
						},
					},
				},
			},
			TerminationMessagePolicy: core_v1.TerminationMessageReadFile,
			TerminationMessagePath:   "/dev/termination-log",

			Resources: defContainerResources,
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
func DesiredDaemonSet(contour *model.Contour, contourImage, envoyImage string) *apps_v1.DaemonSet {
	initContainers, containers := desiredContainers(contour, contourImage, envoyImage)

	ds := &apps_v1.DaemonSet{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace:   contour.Namespace,
			Name:        contour.EnvoyDataPlaneName(),
			Labels:      contour.WorkloadLabels(),
			Annotations: contour.CommonAnnotations(),
		},
		Spec: apps_v1.DaemonSetSpec{
			RevisionHistoryLimit: ptr.To(int32(10)),
			// Ensure the deamonset adopts only its own pods.
			Selector:       EnvoyPodSelector(contour),
			UpdateStrategy: contour.Spec.EnvoyDaemonSetUpdateStrategy,
			Template: core_v1.PodTemplateSpec{
				ObjectMeta: meta_v1.ObjectMeta{
					Annotations: envoyPodAnnotations(contour),
					Labels:      envoyPodLabels(contour),
				},
				Spec: core_v1.PodSpec{
					Containers:     containers,
					InitContainers: initContainers,
					Volumes: []core_v1.Volume{
						{
							Name: envoyCertsVolName,
							VolumeSource: core_v1.VolumeSource{
								Secret: &core_v1.SecretVolumeSource{
									DefaultMode: ptr.To(int32(420)),
									SecretName:  contour.EnvoyCertsSecretName(),
								},
							},
						},
						{
							Name: envoyCfgVolName,
							VolumeSource: core_v1.VolumeSource{
								EmptyDir: &core_v1.EmptyDirVolumeSource{},
							},
						},
						{
							Name: envoyAdminVolName,
							VolumeSource: core_v1.VolumeSource{
								EmptyDir: &core_v1.EmptyDirVolumeSource{},
							},
						},
					},
					ServiceAccountName:            contour.EnvoyRBACNames().ServiceAccount,
					AutomountServiceAccountToken:  ptr.To(false),
					TerminationGracePeriodSeconds: ptr.To(int64(300)),
					SecurityContext:               objects.NewUnprivilegedPodSecurity(),
					DNSPolicy:                     core_v1.DNSClusterFirst,
					RestartPolicy:                 core_v1.RestartPolicyAlways,
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

func desiredDeployment(contour *model.Contour, contourImage, envoyImage string) *apps_v1.Deployment {
	initContainers, containers := desiredContainers(contour, contourImage, envoyImage)

	deployment := &apps_v1.Deployment{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace:   contour.Namespace,
			Name:        contour.EnvoyDataPlaneName(),
			Labels:      contour.WorkloadLabels(),
			Annotations: contour.CommonAnnotations(),
		},
		Spec: apps_v1.DeploymentSpec{
			Replicas:             ptr.To(contour.Spec.EnvoyReplicas),
			RevisionHistoryLimit: ptr.To(int32(10)),
			// Ensure the deamonset adopts only its own pods.
			Selector: EnvoyPodSelector(contour),
			Strategy: contour.Spec.EnvoyDeploymentStrategy,
			Template: core_v1.PodTemplateSpec{
				ObjectMeta: meta_v1.ObjectMeta{
					Annotations: envoyPodAnnotations(contour),
					Labels:      envoyPodLabels(contour),
				},
				Spec: core_v1.PodSpec{
					Affinity: &core_v1.Affinity{
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
					},
					Containers:     containers,
					InitContainers: initContainers,
					Volumes: []core_v1.Volume{
						{
							Name: envoyCertsVolName,
							VolumeSource: core_v1.VolumeSource{
								Secret: &core_v1.SecretVolumeSource{
									DefaultMode: ptr.To(int32(420)),
									SecretName:  contour.EnvoyCertsSecretName(),
								},
							},
						},
						{
							Name: envoyCfgVolName,
							VolumeSource: core_v1.VolumeSource{
								EmptyDir: &core_v1.EmptyDirVolumeSource{},
							},
						},
						{
							Name: envoyAdminVolName,
							VolumeSource: core_v1.VolumeSource{
								EmptyDir: &core_v1.EmptyDirVolumeSource{},
							},
						},
					},
					ServiceAccountName:            contour.EnvoyRBACNames().ServiceAccount,
					AutomountServiceAccountToken:  ptr.To(false),
					TerminationGracePeriodSeconds: ptr.To(int64(300)),
					SecurityContext:               objects.NewUnprivilegedPodSecurity(),
					DNSPolicy:                     core_v1.DNSClusterFirst,
					RestartPolicy:                 core_v1.RestartPolicyAlways,
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
func updateDaemonSetIfNeeded(ctx context.Context, cli client.Client, contour *model.Contour, current, desired *apps_v1.DaemonSet) error {
	if labels.AnyExist(current, model.OwnerLabels(contour)) {
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
func updateDeploymentIfNeeded(ctx context.Context, cli client.Client, contour *model.Contour, current, desired *apps_v1.Deployment) error {
	if labels.AnyExist(current, model.OwnerLabels(contour)) {
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
func EnvoyPodSelector(contour *model.Contour) *meta_v1.LabelSelector {
	return &meta_v1.LabelSelector{
		MatchLabels: map[string]string{
			"app": contour.EnvoyDataPlaneName(),
		},
	}
}

// envoyPodLabels returns the labels for envoy's pods
func envoyPodLabels(contour *model.Contour) map[string]string {
	labels := EnvoyPodSelector(contour).MatchLabels
	for k, v := range contour.WorkloadLabels() {
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

	// Annotations specified on the Gateway take precedence
	// over annotations specified on the GatewayClass/its parameters.
	for k, v := range contour.CommonAnnotations() {
		annotations[k] = v
	}

	return annotations
}
