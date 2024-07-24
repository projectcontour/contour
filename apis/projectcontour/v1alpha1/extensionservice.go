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
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
)

// ExtensionProtocolVersion is the version of the GRPC protocol used
// to access extension services. The only version currently supported
// is "v3".
type ExtensionProtocolVersion string

const (
	// SupportProtocolVersion2 requests the "v2" support protocol version.
	//
	// Deprecated: this protocol version is no longer supported and the
	// constant is retained for backwards compatibility only.
	SupportProtocolVersion2 ExtensionProtocolVersion = "v2"

	// SupportProtocolVersion3 requests the "v3" support protocol version.
	SupportProtocolVersion3 ExtensionProtocolVersion = "v3"
)

// ExtensionServiceTarget defines an Kubernetes Service to target with
// extension service traffic.
type ExtensionServiceTarget struct {
	// Name is the name of Kubernetes service that will accept service
	// traffic.
	//
	// +required
	Name string `json:"name"`

	// Port (defined as Integer) to proxy traffic to since a service can have multiple defined.
	//
	// +required
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65536
	// +kubebuilder:validation:ExclusiveMinimum=false
	// +kubebuilder:validation:ExclusiveMaximum=true
	Port int `json:"port"`

	// Weight defines proportion of traffic to balance to the Kubernetes Service.
	//
	// +optional
	Weight uint32 `json:"weight,omitempty"`
}

// ExtensionServiceSpec defines the desired state of an ExtensionService resource.
type ExtensionServiceSpec struct {
	// Services specifies the set of Kubernetes Service resources that
	// receive GRPC extension API requests.
	// If no weights are specified for any of the entries in
	// this array, traffic will be spread evenly across all the
	// services.
	// Otherwise, traffic is balanced proportionally to the
	// Weight field in each entry.
	//
	// +required
	// +kubebuilder:validation:MinItems=1
	Services []ExtensionServiceTarget `json:"services"`

	// UpstreamValidation defines how to verify the backend service's certificate
	// +optional
	UpstreamValidation *contour_v1.UpstreamValidation `json:"validation,omitempty"`

	// Protocol may be used to specify (or override) the protocol used to reach this Service.
	// Values may be h2 or h2c. If omitted, protocol-selection falls back on Service annotations.
	//
	// +optional
	// +kubebuilder:validation:Enum=h2;h2c
	Protocol *string `json:"protocol,omitempty"`

	// The policy for load balancing GRPC service requests. Note that the
	// `Cookie` and `RequestHash` load balancing strategies cannot be used
	// here.
	//
	// +optional
	LoadBalancerPolicy *contour_v1.LoadBalancerPolicy `json:"loadBalancerPolicy,omitempty"`

	// The timeout policy for requests to the services.
	//
	// +optional
	TimeoutPolicy *contour_v1.TimeoutPolicy `json:"timeoutPolicy,omitempty"`

	// This field sets the version of the GRPC protocol that Envoy uses to
	// send requests to the extension service. Since Contour always uses the
	// v3 Envoy API, this is currently fixed at "v3". However, other
	// protocol options will be available in future.
	//
	// +optional
	// +kubebuilder:validation:Enum=v3
	ProtocolVersion ExtensionProtocolVersion `json:"protocolVersion,omitempty"`

	// CircuitBreakerPolicy specifies the circuit breaker budget across the extension service.
	// If defined this overrides the global circuit breaker budget.
	// +optional
	CircuitBreakerPolicy *CircuitBreakers `json:"circuitBreakerPolicy,omitempty"`
}

// ExtensionServiceStatus defines the observed state of an
// ExtensionService resource.
type ExtensionServiceStatus struct {
	// Conditions contains the current status of the ExtensionService resource.
	//
	// Contour will update a single condition, `Valid`, that is in normal-true polarity.
	//
	// Contour will not modify any other Conditions set in this block,
	// in case some other controller wants to add a Condition.
	//
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []contour_v1.DetailedCondition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=extensionservice;extensionservices

// ExtensionService is the schema for the Contour extension services API.
// An ExtensionService resource binds a network service to the Contour
// API so that Contour API features can be implemented by collaborating
// components.
type ExtensionService struct {
	meta_v1.TypeMeta   `json:",inline"`
	meta_v1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ExtensionServiceSpec   `json:"spec,omitempty"`
	Status ExtensionServiceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ExtensionServiceList contains a list of ExtensionService resources.
type ExtensionServiceList struct {
	meta_v1.TypeMeta `json:",inline"`
	meta_v1.ListMeta `json:"metadata,omitempty"`
	Items            []ExtensionService `json:"items"`
}
