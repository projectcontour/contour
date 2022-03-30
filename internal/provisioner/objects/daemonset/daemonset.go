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

package daemonset

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/projectcontour/contour/internal/provisioner/equality"
	opintstr "github.com/projectcontour/contour/internal/provisioner/intstr"
	"github.com/projectcontour/contour/internal/provisioner/labels"
	"github.com/projectcontour/contour/internal/provisioner/model"
	objutil "github.com/projectcontour/contour/internal/provisioner/objects"
	objcfg "github.com/projectcontour/contour/internal/provisioner/objects/sharedconfig"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
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

// EnsureDaemonSet ensures a DaemonSet exists for the given contour.
func EnsureDaemonSet(ctx context.Context, cli client.Client, contour *model.Contour, contourImage, envoyImage string) error {
	desired := DesiredDaemonSet(contour, contourImage, envoyImage)
	current, err := CurrentDaemonSet(ctx, cli, contour)
	if err != nil {
		if errors.IsNotFound(err) {
			return createDaemonSet(ctx, cli, desired)
		}
		return fmt.Errorf("failed to get daemonset %s/%s: %w", desired.Namespace, desired.Name, err)
	}
	differ := equality.DaemonSetSelectorsDiffer(current, desired)
	if differ {
		return EnsureDaemonSetDeleted(ctx, cli, contour)
	}
	if err := updateDaemonSetIfNeeded(ctx, cli, contour, current, desired); err != nil {
		return fmt.Errorf("failed to update daemonset for contour %s/%s: %w", contour.Namespace, contour.Name, err)
	}
	return nil
}

// EnsureDaemonSetDeleted ensures the DaemonSet for the provided contour is deleted
// if Contour owner labels exist.
func EnsureDaemonSetDeleted(ctx context.Context, cli client.Client, contour *model.Contour) error {
	ds, err := CurrentDaemonSet(ctx, cli, contour)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}
	if labels.Exist(ds, model.OwnerLabels(contour)) {
		if err := cli.Delete(ctx, ds); err != nil {
			if errors.IsNotFound(err) {
				return nil
			}
			return err
		}
	}
	return nil
}

// DesiredDaemonSet returns the desired DaemonSet for the provided contour using
// contourImage as the shutdown-manager/envoy-initconfig container images and
// envoyImage as Envoy's container image.
func DesiredDaemonSet(contour *model.Contour, contourImage, envoyImage string) *appsv1.DaemonSet {
	labels := map[string]string{
		"app.kubernetes.io/name":       "contour",
		"app.kubernetes.io/instance":   contour.Name,
		"app.kubernetes.io/component":  "ingress-controller",
		"app.kubernetes.io/managed-by": "contour-gateway-provisioner",
	}
	// Add owner labels
	for k, v := range model.OwnerLabels(contour) {
		labels[k] = v
	}

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
				fmt.Sprintf("--xds-port=%d", objcfg.XDSPort),
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

	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: contour.Namespace,
			Name:      contour.EnvoyDaemonSetName(),
			Labels:    labels,
		},
		Spec: appsv1.DaemonSetSpec{
			RevisionHistoryLimit: pointer.Int32Ptr(int32(10)),
			// Ensure the deamonset adopts only its own pods.
			Selector: EnvoyDaemonSetPodSelector(contour),
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
					Labels: EnvoyDaemonSetPodSelector(contour).MatchLabels,
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
					SecurityContext:               objutil.NewUnprivilegedPodSecurity(),
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

// CurrentDaemonSet returns the current DaemonSet resource for the provided contour.
func CurrentDaemonSet(ctx context.Context, cli client.Client, contour *model.Contour) (*appsv1.DaemonSet, error) {
	ds := &appsv1.DaemonSet{}
	key := types.NamespacedName{
		Namespace: contour.Namespace,
		Name:      contour.EnvoyDaemonSetName(),
	}
	if err := cli.Get(ctx, key, ds); err != nil {
		return nil, err
	}
	return ds, nil
}

// createDaemonSet creates a DaemonSet resource for the provided ds.
func createDaemonSet(ctx context.Context, cli client.Client, ds *appsv1.DaemonSet) error {
	if err := cli.Create(ctx, ds); err != nil {
		return fmt.Errorf("failed to create daemonset %s/%s: %w", ds.Namespace, ds.Name, err)
	}
	return nil
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

// EnvoyDaemonSetPodSelector returns a label selector using "app: envoy" as the
// key/value pair.
func EnvoyDaemonSetPodSelector(contour *model.Contour) *metav1.LabelSelector {
	return &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"app": contour.EnvoyDaemonSetName(),
		},
	}
}
