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
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	apps_v1 "k8s.io/api/apps/v1"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/provisioner/equality"
	"github.com/projectcontour/contour/internal/provisioner/labels"
	"github.com/projectcontour/contour/internal/provisioner/model"
	"github.com/projectcontour/contour/internal/provisioner/objects"
)

const (
	// contourContainerName is the name of the Contour container.
	contourContainerName = "contour"
	// contourNsEnvVar is the name of the contour namespace environment variable.
	contourNsEnvVar = "CONTOUR_NAMESPACE"
	// contourPodEnvVar is the name of the contour pod name environment variable.
	contourPodEnvVar = "POD_NAME"
	// contourCertsVolName is the name of the contour certificates volume.
	contourCertsVolName = "contourcert"
	// contourCertsVolMntDir is the directory name of the contour certificates volume.
	contourCertsVolMntDir = "certs"
	// metricsPort is the network port number of Contour's metrics service.
	metricsPort = 8000
	// debugPort is the network port number of Contour's debug service.
	debugPort = 6060
)

// EnsureDeployment ensures a deployment using image exists for the given contour.
func EnsureDeployment(ctx context.Context, cli client.Client, contour *model.Contour, image string) error {
	desired := DesiredDeployment(contour, image)

	updater := func(ctx context.Context, cli client.Client, current, desired *apps_v1.Deployment) error {
		differ := equality.DeploymentSelectorsDiffer(current, desired)
		if differ {
			return EnsureDeploymentDeleted(ctx, cli, contour)
		}

		return updateDeploymentIfNeeded(ctx, cli, contour, current, desired)
	}

	return objects.EnsureObject(ctx, cli, desired, updater, &apps_v1.Deployment{})
}

// EnsureDeploymentDeleted ensures the deployment for the provided contour
// is deleted if Contour owner labels exist.
func EnsureDeploymentDeleted(ctx context.Context, cli client.Client, contour *model.Contour) error {
	obj := &apps_v1.Deployment{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: contour.Namespace,
			Name:      contour.ContourDeploymentName(),
		},
	}

	return objects.EnsureObjectDeleted(ctx, cli, obj, contour)
}

// DesiredDeployment returns the desired deployment for the provided contour using
// image as Contour's container image.
func DesiredDeployment(contour *model.Contour, image string) *apps_v1.Deployment {
	xdsPort := objects.XDSPort
	args := []string{
		"serve",
		"--incluster",
		"--xds-address=0.0.0.0",
		fmt.Sprintf("--xds-port=%d", xdsPort),
		fmt.Sprintf("--contour-cafile=%s", filepath.Join("/", contourCertsVolMntDir, "ca.crt")),
		fmt.Sprintf("--contour-cert-file=%s", filepath.Join("/", contourCertsVolMntDir, "tls.crt")),
		fmt.Sprintf("--contour-key-file=%s", filepath.Join("/", contourCertsVolMntDir, "tls.key")),
		fmt.Sprintf("--contour-config-name=%s", contour.ContourConfigurationName()),
		fmt.Sprintf("--leader-election-resource-name=%s", contour.LeaderElectionLeaseName()),
		fmt.Sprintf("--envoy-service-name=%s", contour.EnvoyServiceName()),
		fmt.Sprintf("--kubernetes-debug=%d", contour.Spec.KubernetesLogLevel),
	}

	if contour.Spec.ContourLogLevel == contour_v1alpha1.DebugLog {
		args = append(args, "--debug")
	}

	if !contour.WatchAllNamespaces() {
		ns := model.NamespacesToStrings(contour.Spec.WatchNamespaces)
		if !slices.Contains(ns, contour.Namespace) {
			ns = append(ns, contour.Namespace)
		}
		args = append(args, fmt.Sprintf("--watch-namespaces=%s", strings.Join(ns, ",")))
	}

	if len(contour.Spec.DisabledFeatures) > 0 {
		for _, f := range contour.Spec.DisabledFeatures {
			args = append(args, fmt.Sprintf("--disable-feature=%s", string(f)))
		}
	}

	// Pass the insecure/secure flags to Contour if using non-default ports.
	for _, port := range contour.Spec.NetworkPublishing.Envoy.Ports {
		switch {
		case port.Name == "http" && port.ContainerPort != objects.EnvoyInsecureContainerPort:
			args = append(args, fmt.Sprintf("--envoy-service-http-port=%d", port.ContainerPort))
		case port.Name == "https" && port.ContainerPort != objects.EnvoySecureContainerPort:
			args = append(args, fmt.Sprintf("--envoy-service-https-port=%d", port.ContainerPort))
		}
	}
	if contour.Spec.IngressClassName != nil {
		args = append(args, fmt.Sprintf("--ingress-class-name=%s", *contour.Spec.IngressClassName))
	}
	container := core_v1.Container{
		Name:            contourContainerName,
		Image:           image,
		ImagePullPolicy: core_v1.PullIfNotPresent,
		Command:         []string{"contour"},
		Args:            args,
		Env: []core_v1.EnvVar{
			{
				Name: contourNsEnvVar,
				ValueFrom: &core_v1.EnvVarSource{
					FieldRef: &core_v1.ObjectFieldSelector{
						APIVersion: "v1",
						FieldPath:  "metadata.namespace",
					},
				},
			},
			{
				Name: contourPodEnvVar,
				ValueFrom: &core_v1.EnvVarSource{
					FieldRef: &core_v1.ObjectFieldSelector{
						APIVersion: "v1",
						FieldPath:  "metadata.name",
					},
				},
			},
		},
		Ports: []core_v1.ContainerPort{
			{
				Name:          "xds",
				ContainerPort: xdsPort,
				Protocol:      "TCP",
			},
			{
				Name:          "metrics",
				ContainerPort: metricsPort,
				Protocol:      "TCP",
			},
			{
				Name:          "debug",
				ContainerPort: debugPort,
				Protocol:      "TCP",
			},
		},
		LivenessProbe: &core_v1.Probe{
			ProbeHandler: core_v1.ProbeHandler{
				HTTPGet: &core_v1.HTTPGetAction{
					Scheme: core_v1.URISchemeHTTP,
					Path:   "/healthz",
					Port:   intstr.IntOrString{IntVal: int32(metricsPort)},
				},
			},
			TimeoutSeconds:   int32(1),
			PeriodSeconds:    int32(10),
			SuccessThreshold: int32(1),
			FailureThreshold: int32(3),
		},
		ReadinessProbe: &core_v1.Probe{
			ProbeHandler: core_v1.ProbeHandler{
				TCPSocket: &core_v1.TCPSocketAction{
					Port: intstr.IntOrString{
						IntVal: xdsPort,
					},
				},
			},
			TimeoutSeconds:   int32(1),
			PeriodSeconds:    int32(10),
			SuccessThreshold: int32(1),
			FailureThreshold: int32(3),
		},
		TerminationMessagePath:   "/dev/termination-log",
		TerminationMessagePolicy: core_v1.TerminationMessageReadFile,
		VolumeMounts: []core_v1.VolumeMount{
			{
				Name:      contourCertsVolName,
				MountPath: filepath.Join("/", contourCertsVolMntDir),
				ReadOnly:  true,
			},
		},
		Resources: contour.Spec.ContourResources,
	}
	deploy := &apps_v1.Deployment{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace:   contour.Namespace,
			Name:        contour.ContourDeploymentName(),
			Labels:      contour.WorkloadLabels(),
			Annotations: contour.CommonAnnotations(),
		},
		Spec: apps_v1.DeploymentSpec{
			ProgressDeadlineSeconds: ptr.To(int32(600)),
			Replicas:                ptr.To(contour.Spec.ContourReplicas),
			RevisionHistoryLimit:    ptr.To(int32(10)),
			// Ensure the deployment adopts only its own pods.
			Selector: ContourDeploymentPodSelector(contour),
			Strategy: contour.Spec.ContourDeploymentStrategy,
			Template: core_v1.PodTemplateSpec{
				ObjectMeta: meta_v1.ObjectMeta{
					Annotations: contourPodAnnotations(contour),
					Labels:      contourPodLabels(contour),
				},
				Spec: core_v1.PodSpec{
					// TODO [danehans]: Readdress anti-affinity when https://github.com/projectcontour/contour/issues/2997
					// is resolved.
					Affinity: &core_v1.Affinity{
						PodAntiAffinity: &core_v1.PodAntiAffinity{
							PreferredDuringSchedulingIgnoredDuringExecution: []core_v1.WeightedPodAffinityTerm{
								{
									Weight: int32(100),
									PodAffinityTerm: core_v1.PodAffinityTerm{
										TopologyKey: "kubernetes.io/hostname",
										LabelSelector: &meta_v1.LabelSelector{
											MatchLabels: ContourDeploymentPodSelector(contour).MatchLabels,
										},
									},
								},
							},
						},
					},
					Containers: []core_v1.Container{container},
					Volumes: []core_v1.Volume{
						{
							Name: contourCertsVolName,
							VolumeSource: core_v1.VolumeSource{
								Secret: &core_v1.SecretVolumeSource{
									DefaultMode: ptr.To(int32(420)),
									SecretName:  contour.ContourCertsSecretName(),
								},
							},
						},
					},
					DNSPolicy:                     core_v1.DNSClusterFirst,
					ServiceAccountName:            contour.ContourRBACNames().ServiceAccount,
					RestartPolicy:                 core_v1.RestartPolicyAlways,
					SchedulerName:                 "default-scheduler",
					SecurityContext:               objects.NewUnprivilegedPodSecurity(),
					TerminationGracePeriodSeconds: ptr.To(int64(30)),
				},
			},
		},
	}

	if contour.ContourNodeSelectorExists() {
		deploy.Spec.Template.Spec.NodeSelector = contour.Spec.NodePlacement.Contour.NodeSelector
	}

	if contour.ContourTolerationsExist() {
		deploy.Spec.Template.Spec.Tolerations = contour.Spec.NodePlacement.Contour.Tolerations
	}

	return deploy
}

// updateDeploymentIfNeeded updates a Deployment if current does not match desired,
// using contour to verify the existence of owner labels.
func updateDeploymentIfNeeded(ctx context.Context, cli client.Client, contour *model.Contour, current, desired *apps_v1.Deployment) error {
	if labels.AnyExist(current, model.OwnerLabels(contour)) {
		deploy, updated := equality.DeploymentConfigChanged(current, desired)
		if updated {
			if err := cli.Update(ctx, deploy); err != nil {
				return fmt.Errorf("failed to update deployment %s/%s: %w", deploy.Namespace, deploy.Name, err)
			}
		}
	}
	return nil
}

// ContourDeploymentPodSelector returns a label selector using "app: contour" as the
// key/value pair.
func ContourDeploymentPodSelector(contour *model.Contour) *meta_v1.LabelSelector {
	return &meta_v1.LabelSelector{
		MatchLabels: map[string]string{
			"app": contour.ContourDeploymentName(),
		},
	}
}

// contourPodLabels returns the labels for contour's pods, there are pod selector &
// app labels
func contourPodLabels(contour *model.Contour) map[string]string {
	labels := ContourDeploymentPodSelector(contour).MatchLabels
	for k, v := range contour.WorkloadLabels() {
		labels[k] = v
	}
	return labels
}

// contourPodAnnotations returns the annotations for contour's pods
func contourPodAnnotations(contour *model.Contour) map[string]string {
	annotations := map[string]string{}
	for k, v := range contour.Spec.ContourPodAnnotations {
		annotations[k] = v
	}

	// Annotations specified on the Gateway take precedence
	// over annotations specified on the GatewayClass/its parameters.
	for k, v := range contour.CommonAnnotations() {
		annotations[k] = v
	}

	return annotations
}
