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

package v1alpha1

import (
	apps_v1 "k8s.io/api/apps/v1"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
)

// LogLevel is the logging levels available.
type LogLevel string

const (

	// TraceLog sets the log level for Envoy to `trace`.
	TraceLog LogLevel = "trace"
	// DebugLog sets the log level for Contour/Envoy to `debug`.
	DebugLog LogLevel = "debug"
	// InfoLog sets the log level for Contour/Envoy to `info`.
	InfoLog LogLevel = "info"
	// WarnLog sets the log level for Envoy to `warn`.
	WarnLog LogLevel = "warn"
	// ErrorLog sets the log level for Envoy to `error`.
	ErrorLog LogLevel = "error"
	// CriticalLog sets the log level for Envoy to `critical`.
	CriticalLog LogLevel = "critical"
	// OffLog disable logging for Envoy.
	OffLog LogLevel = "off"
)

// ContourDeploymentSpec specifies options for how a Contour
// instance should be provisioned.
type ContourDeploymentSpec struct {
	// Contour specifies deployment-time settings for the Contour
	// part of the installation, i.e. the xDS server/control plane
	// and associated resources, including things like replica count
	// for the Deployment, and node placement constraints for the pods.
	//
	// +optional
	Contour *ContourSettings `json:"contour,omitempty"`

	// Envoy specifies deployment-time settings for the Envoy
	// part of the installation, i.e. the xDS client/data plane
	// and associated resources, including things like the workload
	// type to use (DaemonSet or Deployment), node placement constraints
	// for the pods, and various options for the Envoy service.
	//
	// +optional
	Envoy *EnvoySettings `json:"envoy,omitempty"`

	// RuntimeSettings is a ContourConfiguration spec to be used when
	// provisioning a Contour instance that will influence aspects of
	// the Contour instance's runtime behavior.
	//
	// +optional
	RuntimeSettings *ContourConfigurationSpec `json:"runtimeSettings,omitempty"`

	// ResourceLabels is a set of labels to add to the provisioned Contour resources.
	//
	// Deprecated: use Gateway.Spec.Infrastructure.Labels instead. This field will be
	// removed in a future release.
	// +optional
	ResourceLabels map[string]string `json:"resourceLabels,omitempty"`
}

// ContourSettings contains settings for the Contour part of the installation,
// i.e. the xDS server/control plane and associated resources.
type ContourSettings struct {
	// Deprecated: Use `DeploymentSettings.Replicas` instead.
	//
	// Replicas is the desired number of Contour replicas. If if unset,
	// defaults to 2.
	//
	// if both `DeploymentSettings.Replicas` and this one is set, use `DeploymentSettings.Replicas`.
	//
	// +kubebuilder:validation:Minimum=0
	// +optional
	Replicas int32 `json:"replicas,omitempty"`

	// NodePlacement describes node scheduling configuration of Contour pods.
	//
	// +optional
	NodePlacement *NodePlacement `json:"nodePlacement,omitempty"`

	// KubernetesLogLevel Enable Kubernetes client debug logging with log level. If unset,
	// defaults to 0.
	//
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=9
	// +optional
	KubernetesLogLevel uint8 `json:"kubernetesLogLevel,omitempty"`

	// LogLevel sets the log level for Contour
	// Allowed values are "info", "debug".
	//
	// +optional
	LogLevel LogLevel `json:"logLevel,omitempty"`

	// Compute Resources required by contour container.
	// Cannot be updated.
	// More info: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	// +optional
	Resources core_v1.ResourceRequirements `json:"resources,omitempty"`

	// Deployment describes the settings for running contour as a `Deployment`.
	// +optional
	Deployment *DeploymentSettings `json:"deployment,omitempty"`

	// PodAnnotations defines annotations to add to the Contour pods.
	// the annotations for Prometheus will be appended or overwritten with predefined value.
	// +optional
	PodAnnotations map[string]string `json:"podAnnotations,omitempty"`

	// WatchNamespaces is an array of namespaces. Setting it will instruct the contour instance
	// to only watch this subset of namespaces.
	// +optional
	// +kubebuilder:validation:Type=array
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=42
	WatchNamespaces []contour_v1.Namespace `json:"watchNamespaces,omitempty"`

	// DisabledFeatures defines an array of resources that will be ignored by
	// contour reconciler.
	// +optional
	// +kubebuilder:validation:Type=array
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=42
	DisabledFeatures []contour_v1.Feature `json:"disabledFeatures,omitempty"`
}

// DeploymentSettings contains settings for Deployment resources.
type DeploymentSettings struct {
	// Replicas is the desired number of replicas.
	//
	// +kubebuilder:validation:Minimum=0
	Replicas int32 `json:"replicas,omitempty"`

	// Strategy describes the deployment strategy to use to replace existing pods with new pods.
	// +optional
	Strategy *apps_v1.DeploymentStrategy `json:"strategy,omitempty"`
}

// DaemonSetSettings contains settings for DaemonSet resources.
type DaemonSetSettings struct {
	// Strategy describes the deployment strategy to use to replace existing DaemonSet pods with new pods.
	// +optional
	UpdateStrategy *apps_v1.DaemonSetUpdateStrategy `json:"updateStrategy,omitempty"`
}

// EnvoySettings contains settings for the Envoy part of the installation,
// i.e. the xDS client/data plane and associated resources.
type EnvoySettings struct {
	// WorkloadType is the type of workload to install Envoy
	// as. Choices are DaemonSet and Deployment. If unset, defaults
	// to DaemonSet.
	//
	// +optional
	WorkloadType WorkloadType `json:"workloadType,omitempty"`

	// Deprecated: Use `DeploymentSettings.Replicas` instead.
	//
	// Replicas is the desired number of Envoy replicas. If WorkloadType
	// is not "Deployment", this field is ignored. Otherwise, if unset,
	// defaults to 2.
	//
	// if both `DeploymentSettings.Replicas` and this one is set, use `DeploymentSettings.Replicas`.
	//
	// +kubebuilder:validation:Minimum=0
	// +optional
	Replicas int32 `json:"replicas,omitempty"`

	// NetworkPublishing defines how to expose Envoy to a network.
	//
	// +optional.
	NetworkPublishing *NetworkPublishing `json:"networkPublishing,omitempty"`

	// NodePlacement describes node scheduling configuration of Envoy pods.
	//
	// +optional
	NodePlacement *NodePlacement `json:"nodePlacement,omitempty"`

	// ExtraVolumes holds the extra volumes to add.
	// +optional
	ExtraVolumes []core_v1.Volume `json:"extraVolumes,omitempty"`

	// ExtraVolumeMounts holds the extra volume mounts to add (normally used with extraVolumes).
	// +optional
	ExtraVolumeMounts []core_v1.VolumeMount `json:"extraVolumeMounts,omitempty"`

	// PodAnnotations defines annotations to add to the Envoy pods.
	// the annotations for Prometheus will be appended or overwritten with predefined value.
	// +optional
	PodAnnotations map[string]string `json:"podAnnotations,omitempty"`

	// Compute Resources required by envoy container.
	// Cannot be updated.
	// More info: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	// +optional
	Resources core_v1.ResourceRequirements `json:"resources,omitempty"`

	// LogLevel sets the log level for Envoy.
	// Allowed values are "trace", "debug", "info", "warn", "error", "critical", "off".
	//
	// +optional
	LogLevel LogLevel `json:"logLevel,omitempty"`

	// DaemonSet describes the settings for running envoy as a `DaemonSet`.
	// if `WorkloadType` is `Deployment`,it's must be nil
	// +optional
	DaemonSet *DaemonSetSettings `json:"daemonSet,omitempty"`

	// Deployment describes the settings for running envoy as a `Deployment`.
	// if `WorkloadType` is `DaemonSet`,it's must be nil
	// +optional
	Deployment *DeploymentSettings `json:"deployment,omitempty"`

	// The base ID to use when allocating shared memory regions.
	// if Envoy needs to be run multiple times on the same machine, each running Envoy will need a unique base ID
	// so that the shared memory regions do not conflict.
	// defaults to 0.
	//
	// +kubebuilder:validation:Minimum=0
	// +optional
	BaseID int32 `json:"baseID,omitempty"`

	// OverloadMaxHeapSize defines the maximum heap memory of the envoy controlled by the overload manager.
	// When the value is greater than 0, the overload manager is enabled,
	// and when envoy reaches 95% of the maximum heap size, it performs a shrink heap operation,
	// When it reaches 98% of the maximum heap size, Envoy Will stop accepting requests.
	// More info: https://projectcontour.io/docs/main/config/overload-manager/
	//
	// +optional
	OverloadMaxHeapSize uint64 `json:"overloadMaxHeapSize,omitempty"`
}

// WorkloadType is the type of Kubernetes workload to use for a component.
type WorkloadType string

const (
	// A Kubernetes daemonset.
	WorkloadTypeDaemonSet = "DaemonSet"

	// A Kubernetes deployment.
	WorkloadTypeDeployment = "Deployment"
)

// NetworkPublishing defines the schema for publishing to a network.
type NetworkPublishing struct {
	// NetworkPublishingType is the type of publishing strategy to use. Valid values are:
	//
	// * LoadBalancerService
	//
	// In this configuration, network endpoints for Envoy use container networking.
	// A Kubernetes LoadBalancer Service is created to publish Envoy network
	// endpoints.
	//
	// See: https://kubernetes.io/docs/concepts/services-networking/service/#loadbalancer
	//
	// * NodePortService
	//
	// Publishes Envoy network endpoints using a Kubernetes NodePort Service.
	//
	// In this configuration, Envoy network endpoints use container networking. A Kubernetes
	// NodePort Service is created to publish the network endpoints.
	//
	// See: https://kubernetes.io/docs/concepts/services-networking/service/#nodeport
	//
	// NOTE:
	// When provisioning an Envoy `NodePortService`, use Gateway Listeners' port numbers to populate
	// the Service's node port values, there's no way to auto-allocate them.
	//
	// See: https://github.com/projectcontour/contour/issues/4499
	//
	// * ClusterIPService
	//
	// Publishes Envoy network endpoints using a Kubernetes ClusterIP Service.
	//
	// In this configuration, Envoy network endpoints use container networking. A Kubernetes
	// ClusterIP Service is created to publish the network endpoints.
	//
	// See: https://kubernetes.io/docs/concepts/services-networking/service/#publishing-services-service-types
	//
	// If unset, defaults to LoadBalancerService.
	//
	// +optional
	Type NetworkPublishingType `json:"type,omitempty"`

	// ExternalTrafficPolicy describes how nodes distribute service traffic they
	// receive on one of the Service's "externally-facing" addresses (NodePorts, ExternalIPs,
	// and LoadBalancer IPs).
	//
	// If unset, defaults to "Local".
	//
	// +optional
	ExternalTrafficPolicy core_v1.ServiceExternalTrafficPolicyType `json:"externalTrafficPolicy,omitempty"`

	// IPFamilyPolicy represents the dual-stack-ness requested or required by
	// this Service. If there is no value provided, then this field will be set
	// to SingleStack. Services can be "SingleStack" (a single IP family),
	// "PreferDualStack" (two IP families on dual-stack configured clusters or
	// a single IP family on single-stack clusters), or "RequireDualStack"
	// (two IP families on dual-stack configured clusters, otherwise fail).
	//
	// +optional
	IPFamilyPolicy core_v1.IPFamilyPolicy `json:"ipFamilyPolicy,omitempty"`

	// ServiceAnnotations is the annotations to add to
	// the provisioned Envoy service.
	//
	// +optional
	ServiceAnnotations map[string]string `json:"serviceAnnotations,omitempty"`
}

// NetworkPublishingType is a way to publish network endpoints.
type NetworkPublishingType string

const (
	// LoadBalancerServicePublishingType publishes a network endpoint using a Kubernetes
	// LoadBalancer Service.
	LoadBalancerServicePublishingType NetworkPublishingType = "LoadBalancerService"

	// NodePortServicePublishingType publishes a network endpoint using a Kubernetes
	// NodePort Service.
	NodePortServicePublishingType NetworkPublishingType = "NodePortService"

	// ClusterIPServicePublishingType publishes a network endpoint using a Kubernetes
	// ClusterIP Service.
	ClusterIPServicePublishingType NetworkPublishingType = "ClusterIPService"
)

// NodePlacement describes node scheduling configuration for pods.
// If nodeSelector and tolerations are specified, the scheduler will use both to
// determine where to place the pod(s).
type NodePlacement struct {
	// NodeSelector is the simplest recommended form of node selection constraint
	// and specifies a map of key-value pairs. For the pod to be eligible
	// to run on a node, the node must have each of the indicated key-value pairs
	// as labels (it can have additional labels as well).
	//
	// If unset, the pod(s) will be scheduled to any available node.
	//
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// Tolerations work with taints to ensure that pods are not scheduled
	// onto inappropriate nodes. One or more taints are applied to a node; this
	// marks that the node should not accept any pods that do not tolerate the
	// taints.
	//
	// The default is an empty list.
	//
	// See https://kubernetes.io/docs/concepts/configuration/taint-and-toleration/
	// for additional details.
	//
	// +optional
	Tolerations []core_v1.Toleration `json:"tolerations,omitempty"`
}

// ContourDeploymentStatus defines the observed state of a ContourDeployment resource.
type ContourDeploymentStatus struct {
	// Conditions describe the current conditions of the ContourDeployment resource.
	//
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []meta_v1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=contourdeploy

// ContourDeployment is the schema for a Contour Deployment.
type ContourDeployment struct {
	meta_v1.TypeMeta   `json:",inline"`
	meta_v1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ContourDeploymentSpec   `json:"spec,omitempty"`
	Status ContourDeploymentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ContourDeploymentList contains a list of Contour Deployment resources.
type ContourDeploymentList struct {
	meta_v1.TypeMeta `json:",inline"`
	meta_v1.ListMeta `json:"metadata,omitempty"`
	Items            []ContourDeployment `json:"items"`
}
