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
	opintstr "github.com/projectcontour/contour/internal/provisioner/intstr"
	"github.com/projectcontour/contour/internal/provisioner/labels"
	"github.com/projectcontour/contour/internal/provisioner/model"
	"github.com/projectcontour/contour/internal/provisioner/objects"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
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

	var (
		getter  objects.ObjectGetter
		updater objects.ObjectUpdater
		desired client.Object
	)

	switch contour.Spec.EnvoyWorkloadType {
	// If a Deployment was specified, provision a Deployment.
	case model.WorkloadTypeDeployment:
		desired = desiredDeployment(contour, contourImage, envoyImage)

		updater = func(ctx context.Context, cli client.Client, contour *model.Contour, currentObj, desiredObj client.Object) error {
			current := currentObj.(*appsv1.Deployment)
			desired := desiredObj.(*appsv1.Deployment)

			differ := equality.DeploymentSelectorsDiffer(current, desired)
			if differ {
				return EnsureDataPlaneDeleted(ctx, cli, contour)
			}

			return updateDeploymentIfNeeded(ctx, cli, contour, current, desired)
		}

		getter = currentDeployment

	// The default workload type is a DaemonSet.
	default:
		desired = DesiredDaemonSet(contour, contourImage, envoyImage)

		updater = func(ctx context.Context, cli client.Client, contour *model.Contour, currentObj, desiredObj client.Object) error {
			current := currentObj.(*appsv1.DaemonSet)
			desired := desiredObj.(*appsv1.DaemonSet)

			differ := equality.DaemonSetSelectorsDiffer(current, desired)
			if differ {
				return EnsureDataPlaneDeleted(ctx, cli, contour)
			}

			return updateDaemonSetIfNeeded(ctx, cli, contour, current, desired)
		}

		getter = CurrentDaemonSet
	}

	return objects.EnsureObject(ctx, cli, contour, desired, getter, updater)

}

// EnsureDataPlaneDeleted ensures the daemonset or deployment for the provided contour is deleted
// if Contour owner labels exist.
func EnsureDataPlaneDeleted(ctx context.Context, cli client.Client, contour *model.Contour) error {
	// Need to try deleting both the DaemonSet and the Deployment because
	// we don't know which one was actually created, since we're not yet
	// using finalizers so the Gateway spec is unavailable to us at deletion
	// time.
	if err := objects.EnsureObjectDeleted(ctx, cli, contour, contour.EnvoyDataPlaneName(), CurrentDaemonSet); err != nil {
		return err
	}

	return objects.EnsureObjectDeleted(ctx, cli, contour, contour.EnvoyDataPlaneName(), currentDeployment)
}

func desiredContainers(contour *model.Contour, contourImage, envoyImage string) ([]corev1.Container, []corev1.Container) {
	var ports []corev1.ContainerPort
	for _, port := range contour.Spec.NetworkPublishing.Envoy.ContainerPorts {
		p := corev1.ContainerPort{
			Name:          port.Name,
			ContainerPort: port.PortNumber,
			Protocol:      corev1.ProtocolTCP,
		}
		ports = append(ports, p)
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
			LivenessProbe: &corev1.Probe{
				FailureThreshold: int32(3),
				ProbeHandler: corev1.ProbeHandler{
					HTTPGet: &corev1.HTTPGetAction{
						Scheme: corev1.URISchemeHTTP,
						Path:   "/healthz",
						Port:   intstr.IntOrString{IntVal: int32(8090)},
					},
				},
				InitialDelaySeconds: int32(3),
				PeriodSeconds:       int32(10),
				SuccessThreshold:    int32(1),
				TimeoutSeconds:      int32(1),
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
				"--log-level info",
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
						Port:   intstr.IntOrString{IntVal: int32(8002)},
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
			Labels:    contour.ComponentLabels(),
		},
		Spec: appsv1.DaemonSetSpec{
			RevisionHistoryLimit: pointer.Int32Ptr(int32(10)),
			// Ensure the deamonset adopts only its own pods.
			Selector: EnvoyPodSelector(contour),
			UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
				Type: appsv1.RollingUpdateDaemonSetStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDaemonSet{
					MaxUnavailable: opintstr.PointerTo(intstr.FromString("10%")),
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					// TODO [danehans]: Remove the prometheus annotations when Contour is updated to
					// show how the Prometheus Operator is used to scrape Contour/Envoy metrics.
					Annotations: map[string]string{
						"prometheus.io/scrape": "true",
						"prometheus.io/port":   "8002",
						"prometheus.io/path":   "/stats/prometheus",
					},
					Labels: EnvoyPodSelector(contour).MatchLabels,
				},
				Spec: corev1.PodSpec{
					Containers:     containers,
					InitContainers: initContainers,
					Volumes: []corev1.Volume{
						{
							Name: envoyCertsVolName,
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									DefaultMode: pointer.Int32Ptr(int32(420)),
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
					AutomountServiceAccountToken:  pointer.BoolPtr(false),
					TerminationGracePeriodSeconds: pointer.Int64Ptr(int64(300)),
					SecurityContext:               objects.NewUnprivilegedPodSecurity(),
					DNSPolicy:                     corev1.DNSClusterFirst,
					RestartPolicy:                 corev1.RestartPolicyAlways,
					SchedulerName:                 "default-scheduler",
				},
			},
		},
	}

	if contour.EnvoyNodeSelectorExists() {
		ds.Spec.Template.Spec.NodeSelector = contour.Spec.NodePlacement.Envoy.NodeSelector
	}

	if contour.EnvoyTolerationsExist() {
		ds.Spec.Template.Spec.Tolerations = contour.Spec.NodePlacement.Envoy.Tolerations
	}

	return ds
}

func desiredDeployment(contour *model.Contour, contourImage, envoyImage string) client.Object {
	initContainers, containers := desiredContainers(contour, contourImage, envoyImage)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: contour.Namespace,
			Name:      contour.EnvoyDataPlaneName(),
			Labels:    contour.ComponentLabels(),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas:             pointer.Int32(contour.Spec.EnvoyReplicas),
			RevisionHistoryLimit: pointer.Int32Ptr(int32(10)),
			// Ensure the deamonset adopts only its own pods.
			Selector: EnvoyPodSelector(contour),
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDeployment{
					MaxSurge: opintstr.PointerTo(intstr.FromString("10%")),
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					// TODO [danehans]: Remove the prometheus annotations when Contour is updated to
					// show how the Prometheus Operator is used to scrape Contour/Envoy metrics.
					Annotations: map[string]string{
						"prometheus.io/scrape": "true",
						"prometheus.io/port":   "8002",
						"prometheus.io/path":   "/stats/prometheus",
					},
					Labels: EnvoyPodSelector(contour).MatchLabels,
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
									DefaultMode: pointer.Int32Ptr(int32(420)),
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
					AutomountServiceAccountToken:  pointer.BoolPtr(false),
					TerminationGracePeriodSeconds: pointer.Int64Ptr(int64(300)),
					SecurityContext:               objects.NewUnprivilegedPodSecurity(),
					DNSPolicy:                     corev1.DNSClusterFirst,
					RestartPolicy:                 corev1.RestartPolicyAlways,
					SchedulerName:                 "default-scheduler",
				},
			},
		},
	}

	if contour.EnvoyNodeSelectorExists() {
		deployment.Spec.Template.Spec.NodeSelector = contour.Spec.NodePlacement.Envoy.NodeSelector
	}

	if contour.EnvoyTolerationsExist() {
		deployment.Spec.Template.Spec.Tolerations = contour.Spec.NodePlacement.Envoy.Tolerations
	}

	return deployment
}

// CurrentDaemonSet returns the current DaemonSet resource for the provided contour.
func CurrentDaemonSet(ctx context.Context, cli client.Client, namespace, name string) (client.Object, error) {
	ds := &appsv1.DaemonSet{}
	key := types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}
	if err := cli.Get(ctx, key, ds); err != nil {
		return nil, err
	}
	return ds, nil
}

// currentDeployment returns the current Deployment resource for the provided contour.
func currentDeployment(ctx context.Context, cli client.Client, namespace, name string) (client.Object, error) {
	ds := &appsv1.Deployment{}
	key := types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}
	if err := cli.Get(ctx, key, ds); err != nil {
		return nil, err
	}
	return ds, nil
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
