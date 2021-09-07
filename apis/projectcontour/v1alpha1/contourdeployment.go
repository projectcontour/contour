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
	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ContourDeploymentSpec defines the parameters of how a Contour
// instance should be configured.
type ContourDeploymentSpec struct {
	// Replicas is the desired number of Contour replicas. If unset,
	// defaults to 2.
	//
	// +kubebuilder:default=2
	// +kubebuilder:validation:Minimum=0
	Replicas int32 `json:"replicas,omitempty"`

	// Config is the config that the instances of Contour are to utilize.
	Config ContourConfigurationSpec `json:"config"`
}

// ContourDeploymentStatus defines the observed state of a ContourDeployment resource.
type ContourDeploymentStatus struct {
	// Conditions contains the current status of the Contour resource.
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
	Conditions []contour_api_v1.DetailedCondition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
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
