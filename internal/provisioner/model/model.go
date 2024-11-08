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

package model

import (
	apps_v1 "k8s.io/api/apps/v1"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
)

const (
	// ContourOwningGatewayNameLabel is the Contour-defined owner reference label applied
	// to generated resources. The value should be the name of the Gateway.
	ContourOwningGatewayNameLabel = "projectcontour.io/owning-gateway-name"

	// GatewayAPIOwningGatewayNameLabel is the Gateway API-defined owner reference label applied
	// to generated resources. The value should be the name of the Gateway.
	GatewayAPIOwningGatewayNameLabel = "gateway.networking.k8s.io/gateway-name"
)

// Default returns a default instance of a Contour
// for the given namespace/name.
func Default(namespace, name string) *Contour {
	return &Contour{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Spec: ContourSpec{
			ContourReplicas:       2,
			EnvoyWorkloadType:     WorkloadTypeDaemonSet,
			EnvoyReplicas:         2, // ignored if not provisioning Envoy as a deployment.
			EnvoyLogLevel:         contour_v1alpha1.InfoLog,
			EnvoyBaseID:           0,
			EnvoyMaxHeapSizeBytes: 0,
			NetworkPublishing: NetworkPublishing{
				Envoy: EnvoyNetworkPublishing{
					Type:                  LoadBalancerServicePublishingType,
					ExternalTrafficPolicy: core_v1.ServiceExternalTrafficPolicyTypeLocal,
					IPFamilyPolicy:        core_v1.IPFamilyPolicySingleStack,
				},
			},
			EnvoyDaemonSetUpdateStrategy: apps_v1.DaemonSetUpdateStrategy{
				Type: apps_v1.RollingUpdateDaemonSetStrategyType,
				RollingUpdate: &apps_v1.RollingUpdateDaemonSet{
					MaxUnavailable: ptr.To(intstr.FromString("10%")),
				},
			},
			EnvoyDeploymentStrategy: apps_v1.DeploymentStrategy{
				Type: apps_v1.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &apps_v1.RollingUpdateDeployment{
					MaxSurge: ptr.To(intstr.FromString("10%")),
				},
			},
			ContourDeploymentStrategy: apps_v1.DeploymentStrategy{
				Type: apps_v1.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &apps_v1.RollingUpdateDeployment{
					MaxSurge:       ptr.To(intstr.FromString("50%")),
					MaxUnavailable: ptr.To(intstr.FromString("25%")),
				},
			},
			ResourceLabels:        map[string]string{},
			ResourceAnnotations:   map[string]string{},
			EnvoyPodAnnotations:   map[string]string{},
			ContourPodAnnotations: map[string]string{},
		},
	}
}

// Contour is the representation of an instance of Contour + Envoy.
type Contour struct {
	meta_v1.ObjectMeta `json:"metadata,omitempty"`

	// Spec defines the desired state of Contour.
	Spec ContourSpec `json:"spec,omitempty"`
}

// ContourNodeSelectorExists returns true if a nodeSelector is specified for Contour.
func (c *Contour) ContourNodeSelectorExists() bool {
	if c.Spec.NodePlacement != nil &&
		c.Spec.NodePlacement.Contour != nil &&
		c.Spec.NodePlacement.Contour.NodeSelector != nil {
		return true
	}

	return false
}

// ContourTolerationsExist returns true if tolerations are set for Contour.
func (c *Contour) ContourTolerationsExist() bool {
	if c.Spec.NodePlacement != nil &&
		c.Spec.NodePlacement.Contour != nil &&
		len(c.Spec.NodePlacement.Contour.Tolerations) > 0 {
		return true
	}

	return false
}

// EnvoyNodeSelectorExists returns true if a nodeSelector is specified for Envoy.
func (c *Contour) EnvoyNodeSelectorExists() bool {
	if c.Spec.NodePlacement != nil &&
		c.Spec.NodePlacement.Envoy != nil &&
		c.Spec.NodePlacement.Envoy.NodeSelector != nil {
		return true
	}

	return false
}

// EnvoyTolerationsExist returns true if tolerations are set for Envoy.
func (c *Contour) EnvoyTolerationsExist() bool {
	if c.Spec.NodePlacement != nil &&
		c.Spec.NodePlacement.Envoy != nil &&
		len(c.Spec.NodePlacement.Envoy.Tolerations) > 0 {
		return true
	}

	return false
}

func (c *Contour) WatchAllNamespaces() bool {
	return len(c.Spec.WatchNamespaces) == 0
}

// ContourSpec defines the desired state of Contour.
type ContourSpec struct {
	// ContourReplicas is the desired number of Contour replicas. If unset,
	// defaults to 2.
	ContourReplicas int32

	// EnvoyReplicas is the desired number of Envoy replicas. If WorkloadType
	// is not "Deployment", this field is ignored. Otherwise, if unset,
	// defaults to 2.
	EnvoyReplicas int32

	// NetworkPublishing defines the schema for publishing Contour to a network.
	//
	// See each field for additional details.
	NetworkPublishing NetworkPublishing

	// GatewayControllerName is used to determine which GatewayClass
	// Contour reconciles. The string takes the form of
	// "projectcontour.io/<namespace>/contour". If unset, Contour will not
	// reconcile Gateway API resources.
	GatewayControllerName *string

	// IngressClassName is the name of the IngressClass used by Contour. If unset,
	// Contour will process all ingress objects without an ingress class annotation
	// or ingress objects with an annotation matching ingress-class=contour. When
	// specified, Contour will only process ingress objects that match the provided
	// class.
	//
	// For additional IngressClass details, refer to:
	//   https://projectcontour.io/docs/main/config/annotations/#ingress-class
	IngressClassName *string

	// ContourLogLevel sets the log level for Contour
	// Allowed values are "info", "debug".
	ContourLogLevel contour_v1alpha1.LogLevel

	// NodePlacement enables scheduling of Contour and Envoy pods onto specific nodes.
	//
	// See each field for additional details.
	NodePlacement *NodePlacement

	// EnableExternalNameService enables ExternalName Services.
	// ExternalName Services are disabled by default due to CVE-2021-XXXXX
	// You can re-enable them by setting this setting to "true".
	// This is not recommended without understanding the security implications.
	// Please see the advisory at https://github.com/projectcontour/contour/security/advisories/GHSA-5ph6-qq5x-7jwc for the details.
	EnableExternalNameService *bool

	// RuntimeSettings is any user-defined ContourConfigurationSpec to use when provisioning.
	RuntimeSettings *contour_v1alpha1.ContourConfigurationSpec

	// EnvoyWorkloadType is the way to deploy Envoy, either "DaemonSet" or "Deployment".
	EnvoyWorkloadType WorkloadType

	// KubernetesLogLevel Enable Kubernetes client debug logging with log level. If unset,
	// defaults to 0.
	KubernetesLogLevel uint8

	// An update strategy to replace existing Envoy DaemonSet pods with new pods.
	// when envoy be running as a `Deployment`,it's must be nil
	// +optional
	EnvoyDaemonSetUpdateStrategy apps_v1.DaemonSetUpdateStrategy

	// The deployment strategy to use to replace existing Envoy pods with new ones.
	// when envoy be running as a `DaemonSet`,it's must be nil
	EnvoyDeploymentStrategy apps_v1.DeploymentStrategy

	// The deployment strategy to use to replace existing Contour pods with new ones.
	// when envoy be running as a `DaemonSet`,it's must be nil
	ContourDeploymentStrategy apps_v1.DeploymentStrategy

	// ResourceLabels is a set of labels to add to the provisioned resources.
	ResourceLabels map[string]string

	// ResourceAnnotations is a set of annotations to add to the provisioned resources.
	ResourceAnnotations map[string]string

	// EnvoyExtraVolumes holds the extra volumes to add to envoy's pod.
	EnvoyExtraVolumes []core_v1.Volume

	// EnvoyExtraVolumeMounts holds the extra volume mounts to add to envoy's pod(normally used with envoyExtraVolumes).
	EnvoyExtraVolumeMounts []core_v1.VolumeMount

	// EnvoyPodAnnotations holds the annotations that will be add to the envoy‘s pod.
	EnvoyPodAnnotations map[string]string

	// ContourPodAnnotations holds the annotations that will be add to the contour's pod.
	ContourPodAnnotations map[string]string

	// Compute Resources required by envoy container.
	EnvoyResources core_v1.ResourceRequirements

	// Compute Resources required by contour container.
	ContourResources core_v1.ResourceRequirements

	// EnvoyLogLevel sets the log level for Envoy
	// Allowed values are "trace", "debug", "info", "warn", "error", "critical", "off".
	EnvoyLogLevel contour_v1alpha1.LogLevel

	// The base ID to use when allocating shared memory regions.
	// if Envoy needs to be run multiple times on the same machine, each running Envoy will need a unique base ID
	// so that the shared memory regions do not conflict.
	// defaults to 0.
	EnvoyBaseID int32

	// MaximumHeapSizeBytes defines how much memory the overload manager controls Envoy to allocate at most.
	// If the value is 0, the overload manager is disabled.
	// defaults to 0.
	EnvoyMaxHeapSizeBytes uint64

	// WatchNamespaces is an array of namespaces. Setting it will instruct the contour instance
	// to only watch these set of namespaces
	// default is nil, contour will watch resource of all namespaces
	WatchNamespaces []contour_v1.Namespace

	// DisabledFeatures defines an array of resources that will be ignored by
	// contour reconciler.
	DisabledFeatures []contour_v1.Feature
}

func NamespacesToStrings(ns []contour_v1.Namespace) []string {
	res := make([]string, len(ns))
	for i, n := range ns {
		res[i] = string(n)
	}
	return res
}

func FeaturesToStrings(fs []contour_v1.Feature) []string {
	res := make([]string, len(fs))
	for i := range fs {
		res[i] = string(fs[i])
	}
	return res
}

// WorkloadType is the type of Kubernetes workload to use for a component.
type WorkloadType = contour_v1alpha1.WorkloadType

const (
	// A Kubernetes DaemonSet.
	WorkloadTypeDaemonSet = contour_v1alpha1.WorkloadTypeDaemonSet

	// A Kubernetes Deployment.
	WorkloadTypeDeployment = contour_v1alpha1.WorkloadTypeDeployment
)

// NodePlacement describes node scheduling configuration of Contour and Envoy pods.
type NodePlacement struct {
	// Contour describes node scheduling configuration of Contour pods.
	Contour *ContourNodePlacement

	// Envoy describes node scheduling configuration of Envoy pods.
	Envoy *EnvoyNodePlacement
}

// ContourNodePlacement describes node scheduling configuration for Contour pods.
// If nodeSelector and tolerations are specified, the scheduler will use both to
// determine where to place the Contour pod(s).
type ContourNodePlacement struct {
	// NodeSelector is the simplest recommended form of node selection constraint
	// and specifies a map of key-value pairs. For the Contour pod to be eligible
	// to run on a node, the node must have each of the indicated key-value pairs
	// as labels (it can have additional labels as well).
	//
	// If unset, the Contour pod(s) will be scheduled to any available node.
	NodeSelector map[string]string

	// Tolerations work with taints to ensure that Envoy pods are not scheduled
	// onto inappropriate nodes. One or more taints are applied to a node; this
	// marks that the node should not accept any pods that do not tolerate the
	// taints.
	//
	// The default is an empty list.
	//
	// See https://kubernetes.io/docs/concepts/configuration/taint-and-toleration/
	// for additional details.
	Tolerations []core_v1.Toleration
}

// EnvoyNodePlacement describes node scheduling configuration for Envoy pods.
// If nodeSelector and tolerations are specified, the scheduler will use both
// to determine where to place the Envoy pod(s).
type EnvoyNodePlacement struct {
	// NodeSelector is the simplest recommended form of node selection constraint
	// and specifies a map of key-value pairs. For the Envoy pod to be eligible to
	// run on a node, the node must have each of the indicated key-value pairs as
	// labels (it can have additional labels as well).
	//
	// If unset, the Envoy pod(s) will be scheduled to any available node.
	NodeSelector map[string]string

	// Tolerations work with taints to ensure that Envoy pods are not scheduled
	// onto inappropriate nodes. One or more taints are applied to a node; this
	// marks that the node should not accept any pods that do not tolerate the taints.
	//
	// The default is an empty list.
	//
	// See https://kubernetes.io/docs/concepts/configuration/taint-and-toleration/
	// for additional details.
	Tolerations []core_v1.Toleration
}

// NamespaceSpec defines the schema of a Contour namespace.
type NamespaceSpec struct {
	// Name is the name of the namespace to run Contour and dependent
	// resources. If unset, defaults to "projectcontour".
	Name string

	// RemoveOnDeletion will remove the namespace when the Contour is
	// deleted. If set to True, deletion will not occur if any of the
	// following conditions exist:
	//
	// 1. The Contour namespace is "default", "kube-system" or the
	//    contour-operator's namespace.
	//
	// 2. Another Contour exists in the namespace.
	//
	// 3. The namespace does not contain the Contour owning label.
	RemoveOnDeletion bool
}

// NetworkPublishing defines the schema for publishing Contour to a network.
type NetworkPublishing struct {
	// Envoy provides the schema for publishing the network endpoints of Envoy.
	//
	// If unset, defaults to:
	//   type: LoadBalancerService
	//   containerPorts:
	//   - name: http
	//     portNumber: 8080
	//   - name: https
	//     portNumber: 8443
	Envoy EnvoyNetworkPublishing
}

// EnvoyNetworkPublishing defines the schema to publish Envoy to a network.
type EnvoyNetworkPublishing struct {
	// Type is the type of publishing strategy to use. Valid values are:
	//
	// * LoadBalancerService
	//
	// In this configuration, network endpoints for Envoy use container networking.
	// A Kubernetes LoadBalancer Service is created to publish Envoy network
	// endpoints. The Service uses port 80 to publish Envoy's HTTP network endpoint
	// and port 443 to publish Envoy's HTTPS network endpoint.
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
	Type NetworkPublishingType

	// LoadBalancer holds parameters for the load balancer. Present only if type is
	// LoadBalancerService.
	//
	// If unspecified, defaults to an external Classic AWS ELB.
	LoadBalancer LoadBalancerStrategy

	// Ports is a list of ports to expose on the Envoy service and
	// workload.
	Ports []Port

	// ServiceAnnotations is a set of annotations to add to the provisioned Envoy service.
	ServiceAnnotations map[string]string

	// IPFamilyPolicy represents the dual-stack-ness requested or required by
	// this Service. If there is no value provided, then this field will be set
	// to SingleStack.
	IPFamilyPolicy core_v1.IPFamilyPolicy

	// ExternalTrafficPolicy describes how nodes distribute service traffic they
	// receive on one of the Service's "externally-facing" addresses (NodePorts, ExternalIPs,
	// and LoadBalancer IPs).
	//
	// If unset, defaults to "Local".
	ExternalTrafficPolicy core_v1.ServiceExternalTrafficPolicyType
}

type NetworkPublishingType = contour_v1alpha1.NetworkPublishingType

const (
	// LoadBalancerServicePublishingType publishes a network endpoint using a Kubernetes
	// LoadBalancer Service.
	LoadBalancerServicePublishingType NetworkPublishingType = contour_v1alpha1.LoadBalancerServicePublishingType

	// NodePortServicePublishingType publishes a network endpoint using a Kubernetes
	// NodePort Service.
	NodePortServicePublishingType NetworkPublishingType = contour_v1alpha1.NodePortServicePublishingType

	// ClusterIPServicePublishingType publishes a network endpoint using a Kubernetes
	// ClusterIP Service.
	ClusterIPServicePublishingType NetworkPublishingType = contour_v1alpha1.ClusterIPServicePublishingType
)

// LoadBalancerStrategy holds parameters for a load balancer.
type LoadBalancerStrategy struct {
	// Scope indicates the scope at which the load balancer is exposed.
	// Possible values are "External" and "Internal".
	Scope LoadBalancerScope

	// ProviderParameters contains load balancer information specific to
	// the underlying infrastructure provider.
	ProviderParameters ProviderLoadBalancerParameters

	// LoadBalancerIP is the IP (or hostname) to request
	// for the LoadBalancer service.
	LoadBalancerIP string
}

// LoadBalancerScope is the scope at which a load balancer is exposed.
// +kubebuilder:validation:Enum=Internal;External
type LoadBalancerScope string

var (
	// InternalLoadBalancer is a load balancer that is exposed only on the
	// cluster's private network.
	InternalLoadBalancer LoadBalancerScope = "Internal"

	// ExternalLoadBalancer is a load balancer that is exposed on the
	// cluster's public network (which is typically on the Internet).
	ExternalLoadBalancer LoadBalancerScope = "External"
)

// ProviderLoadBalancerParameters holds desired load balancer information
// specific to the underlying infrastructure provider.
type ProviderLoadBalancerParameters struct {
	// Type is the underlying infrastructure provider for the load balancer.
	// Allowed values are "AWS", "Azure", and "GCP".
	Type LoadBalancerProviderType

	// AWS provides configuration settings that are specific to AWS
	// load balancers.
	//
	// If empty, defaults will be applied. See specific aws fields for
	// details about their defaults.
	AWS *AWSLoadBalancerParameters

	// Azure provides configuration settings that are specific to Azure
	// load balancers.
	//
	// If empty, defaults will be applied. See specific azure fields for
	// details about their defaults.
	Azure *AzureLoadBalancerParameters

	// GCP provides configuration settings that are specific to GCP
	// load balancers.
	//
	// If empty, defaults will be applied. See specific gcp fields for
	// details about their defaults.
	GCP *GCPLoadBalancerParameters
}

// LoadBalancerProviderType is the underlying infrastructure provider for the
// load balancer. Allowed values are "AWS", "Azure", and "GCP".
type LoadBalancerProviderType string

const (
	AWSLoadBalancerProvider   LoadBalancerProviderType = "AWS"
	AzureLoadBalancerProvider LoadBalancerProviderType = "Azure"
	GCPLoadBalancerProvider   LoadBalancerProviderType = "GCP"
)

// AWSLoadBalancerParameters provides configuration settings that are specific to
// AWS load balancers.
type AWSLoadBalancerParameters struct {
	// Type is the type of AWS load balancer to manage.
	//
	// Valid values are:
	//
	// * "Classic": A Classic load balancer makes routing decisions at either the
	//   transport layer (TCP/SSL) or the application layer (HTTP/HTTPS). See
	//   the following for additional details:
	//
	//     https://docs.aws.amazon.com/AmazonECS/latest/developerguide/load-balancer-types.html#clb
	//
	// * "NLB": A Network load balancer makes routing decisions at the transport
	//   layer (TCP/SSL). See the following for additional details:
	//
	//     https://docs.aws.amazon.com/AmazonECS/latest/developerguide/load-balancer-types.html#nlb
	//
	// If unset, defaults to "Classic".
	Type AWSLoadBalancerType

	// AllocationIDs is a list of Allocation IDs of Elastic IP addresses that are
	// to be assigned to the Network Load Balancer. Works only with type NLB.
	// If you are using Amazon EKS 1.16 or later, you can assign Elastic IP addresses
	// to Network Load Balancer with AllocationIDs. The number of Allocation IDs
	// must match the number of subnets used for the load balancer.
	//
	// Example: "eipalloc-<xxxxxxxxxxxxxxxxx>"
	//
	// See: https://docs.aws.amazon.com/eks/latest/userguide/load-balancing.html
	AllocationIDs []string
}

// AWSLoadBalancerType is the type of AWS load balancer to manage.
type AWSLoadBalancerType string

const (
	AWSClassicLoadBalancer AWSLoadBalancerType = "Classic"
	AWSNetworkLoadBalancer AWSLoadBalancerType = "NLB"
)

type AzureLoadBalancerParameters struct {
	// Address is the desired load balancer IP address. If scope is "Internal", address
	// must reside in same virtual network as AKS and must not already be assigned
	// to a resource. If address does not reside in same subnet as AKS, the subnet
	// parameter is also required.
	//
	// Address must already exist (e.g. `az network public-ip create`).
	//
	// See:
	// 	 https://docs.microsoft.com/en-us/azure/aks/static-ip#create-a-service-using-the-static-ip-address
	// 	 https://docs.microsoft.com/en-us/azure/aks/internal-lb#specify-an-ip-address
	Address *string

	// ResourceGroup is the resource group name where the "address" resides. Relevant
	// only if scope is "External".
	//
	// Omit if desired IP is created in same resource group as AKS cluster.
	ResourceGroup *string

	// Subnet is the subnet name where the "address" resides. Relevant only
	// if scope is "Internal" and desired IP does not reside in same subnet as AKS.
	//
	// Omit if desired IP is in same subnet as AKS cluster.
	//
	// See: https://docs.microsoft.com/en-us/azure/aks/internal-lb#specify-an-ip-address
	Subnet *string
}

type GCPLoadBalancerParameters struct {
	// Address is the desired load balancer IP address. If scope is "Internal", the address
	// must reside in same subnet as the GKE cluster or "subnet" has to be provided.
	//
	// See:
	// 	 https://cloud.google.com/kubernetes-engine/docs/tutorials/configuring-domain-name-static-ip#use_a_service
	// 	 https://cloud.google.com/kubernetes-engine/docs/how-to/internal-load-balancing#lb_subnet
	Address *string

	// Subnet is the subnet name where the "address" resides. Relevant only
	// if scope is "Internal" and desired IP does not reside in same subnet as GKE
	// cluster.
	//
	// Omit if desired IP is in same subnet as GKE cluster.
	//
	// See: https://cloud.google.com/kubernetes-engine/docs/how-to/internal-load-balancing#lb_subnet
	Subnet *string
}

type Port struct {
	// Name is the name to use for the port on the Envoy service and workload.
	Name string
	// ServicePort is the port to expose on the Envoy service.
	ServicePort int32
	// ContainerPort is the port to expose on the Envoy container(s).
	ContainerPort int32
	// NodePort is the network port number to expose for the NodePort Service.
	// If unspecified, a port number will be assigned from the cluster's
	// nodeport service range, i.e. --service-node-port-range flag
	// (default: 30000-32767).
	//
	// If specified, the number must:
	//
	// 1. Not be used by another NodePort Service.
	// 2. Be within the cluster's nodeport service range, i.e. --service-node-port-range
	//    flag (default: 30000-32767).
	// 3. Be a valid network port number, i.e. greater than 0 and less than 65536.
	NodePort int32
}

const (
	// ContourAvailableConditionType indicates that the contour is running
	// and available.
	ContourAvailableConditionType = "Available"
)

// OwnerLabels returns owner labels for the provided contour.
func OwnerLabels(contour *Contour) map[string]string {
	return map[string]string{
		ContourOwningGatewayNameLabel:    contour.Name,
		GatewayAPIOwningGatewayNameLabel: contour.Name,
	}
}
