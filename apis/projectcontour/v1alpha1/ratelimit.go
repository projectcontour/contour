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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

type RateLimitPolicySpec struct {
	// TargetRef identifies an API object to apply policy to.
	TargetRef gatewayapi_v1alpha2.PolicyTargetReference `json:"targetRef"`

	// Override defines policy configuration that should override policy
	// configuration attached below the targeted resource in the hierarchy.
	// +optional
	Override *RateLimitPolicyConfig `json:"override,omitempty"`

	// Default defines default policy configuration for the targeted resource.
	// +optional
	Default *RateLimitPolicyConfig `json:"default,omitempty"`
}

type RateLimitPolicyConfig struct {
	// Local defines local rate limiting parameters, i.e. parameters
	// for rate limiting that occurs within each Envoy pod as requests
	// are handled.
	// +optional
	Local *LocalRateLimitPolicy `json:"local,omitempty"`
}

// LocalRateLimitPolicy defines local rate limiting parameters.
type LocalRateLimitPolicy struct {
	// Requests defines how many requests per unit of time should
	// be allowed before rate limiting occurs.
	// +required
	// +kubebuilder:validation:Minimum=1
	Requests uint32 `json:"requests"`

	// Unit defines the period of time within which requests
	// over the limit will be rate limited. Valid values are
	// "second", "minute" and "hour".
	// +kubebuilder:validation:Enum=second;minute;hour
	// +required
	Unit string `json:"unit"`

	// Burst defines the number of requests above the requests per
	// unit that should be allowed within a short period of time.
	// +optional
	Burst uint32 `json:"burst,omitempty"`

	// ResponseStatusCode is the HTTP status code to use for responses
	// to rate-limited requests. Codes must be in the 400-599 range
	// (inclusive). If not specified, the Envoy default of 429 (Too
	// Many Requests) is used.
	// +optional
	// +kubebuilder:validation:Minimum=400
	// +kubebuilder:validation:Maximum=599
	ResponseStatusCode uint32 `json:"responseStatusCode,omitempty"`

	// ResponseHeadersToAdd is an optional list of response headers to
	// set when a request is rate-limited.
	// +optional
	ResponseHeadersToAdd []HeaderValue `json:"responseHeadersToAdd,omitempty"`
}

// HeaderValue represents a header name/value pair
type HeaderValue struct {
	// Name represents a key of a header
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// Value represents the value of a header specified by a key
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Value string `json:"value"`
}

type RateLimitPolicyStatus struct {
	// Conditions describe the current conditions of the RateLimitPolicy.
	//
	// +optional
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=8
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:resource:scope=Namespaced,shortName=ratelimitpolicy;ratelimitpolicies

// RateLimitPolicy provides a way to apply ...
type RateLimitPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec RateLimitPolicySpec `json:"spec"`

	// +optional
	Status RateLimitPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RateLimitPolicyList contains a list of RateLimitPolicy resources.
type RateLimitPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RateLimitPolicy `json:"items"`
}

type RateLimitFilterSpec struct {
	// Local defines local rate limiting parameters, i.e. parameters
	// for rate limiting that occurs within each Envoy pod as requests
	// are handled.
	// +optional
	Local *LocalRateLimitPolicy `json:"local,omitempty"`
}

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:resource:scope=Namespaced,shortName=ratelimitfilter;ratelimitfilters

type RateLimitFilter struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec RateLimitFilterSpec `json:"spec"`

	// +optional
	Status RateLimitPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RateLimitFilterList contains a list of RateLimitFilter resources.
type RateLimitFilterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RateLimitFilter `json:"items"`
}
