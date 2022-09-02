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
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LogLevel is the logging levels available.
type LogLevel string

const (
	// InfoLog sets the log level for Contour to `info`.
	InfoLog LogLevel = "info"

	// DebugLog sets the log level for Contour to `debug`.
	DebugLog LogLevel = "debug"
)

func (l LogLevel) Validate() error {
	switch l {
	case InfoLog, DebugLog:
		return nil
	default:
		return fmt.Errorf("invalid log level %q", l)
	}
}

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
}

// ContourSettings contains settings for the Contour part of the installation,
// i.e. the xDS server/control plane and associated resources.
type ContourSettings struct {
	// Replicas is the desired number of Contour replicas. If unset,
	// defaults to 2.
	//
	// +kubebuilder:validation:Minimum=0
	Replicas int32 `json:"replicas,omitempty"`

	// NodePlacement describes node scheduling configuration of Contour pods.
	//
	// +optional
	NodePlacement *NodePlacement `json:"nodePlacement,omitempty"`

	// LogLevel sets the log level for Contour
	// Allowed values are "info", "debug".
	//
	// +optional
	LogLevel LogLevel `json:"logLevel,omitempty"`
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

	// Replicas is the desired number of Envoy replicas. If WorkloadType
	// is not "Deployment", this field is ignored. Otherwise, if unset,
	// defaults to 2.
	//
	// +kubebuilder:validation:Minimum=0
	Replicas int32 `json:"replicas,omitempty"`

	// NetworkPublishing defines how to expose Envoy to a network.
	//
	// +optional.
	NetworkPublishing *NetworkPublishing `json:"networkPublishing,omitempty"`

	// NodePlacement describes node scheduling configuration of Envoy pods.
	//
	// +optional
	NodePlacement *NodePlacement `json:"nodePlacement,omitempty"`
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
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
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
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=contourdeploy

// ContourDeployment is the schema for a Contour Deployment.
type ContourDeployment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ContourDeploymentSpec   `json:"spec,omitempty"`
	Status ContourDeploymentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ContourDeploymentList contains a list of Contour Deployment resources.
type ContourDeploymentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ContourDeployment `json:"items"`
}
