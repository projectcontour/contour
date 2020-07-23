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
	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ExtensionProtocolVersion is the version of the GRPC protocol used
// to access extension services. The only version currently supported
// is "v2".
type ExtensionProtocolVersion string

// SupportProtocolVersion2 requests the "v2" support protocol version.
const SupportProtocolVersion2 ExtensionProtocolVersion = "v2"

// ExtensionServiceSpec defines the desired state of an ExtensionService resource.
type ExtensionServiceSpec struct {
	// Services specifies the set of Kubernetes Service resources that
	// receive GRPC extension API requests.
	//
	// +required
	// +kubebuilder:validation:MinItems=1
	Services []contourv1.Service `json:"services"`

	// The policy for load balancing GRPC service requests. Note
	// that the `Cookie` load balancing strategy cannot be used here.
	//
	// +optional
	LoadBalancerPolicy *contourv1.LoadBalancerPolicy `json:"loadBalancerPolicy,omitempty"`

	// The timeout policy for requests to the services.
	//
	// +optional
	TimeoutPolicy *contourv1.TimeoutPolicy `json:"timeoutPolicy,omitempty"`

	// This field sets the version of the GRPC protocol that Envoy uses to
	// send requests to the extension service. Since Contour always uses the
	// v2 Envoy API, this is currently fixed at "v2". However, other
	// protocol options will be available in future.
	//
	// +optional
	// +kubebuilder:validation:Enum=v2
	ProtocolVersion ExtensionProtocolVersion `json:"protocolVersion,omitempty"`
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
	Conditions []contourv1.DetailedCondition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
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
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ExtensionServiceSpec   `json:"spec,omitempty"`
	Status ExtensionServiceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ExtensionServiceList contains a list of ExtensionService resources.
type ExtensionServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ExtensionService `json:"items"`
}
