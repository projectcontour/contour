// Copyright © 2018 Heptio
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

package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// IngressRouteSpec defines the spec of the CRD
type IngressRouteSpec struct {
	// Virtualhost appears at most once. If it is present, the object is considered
	// to be a "root".
	VirtualHost `json:"virtualhost"`
	// Routes are the ingress routes
	Routes []Route `json:"routes"`
}

// VirtualHost appears at most once. If it is present, the object is considered
// to be a "root".
type VirtualHost struct {
	// The fully qualified domain name of the root of the ingress tree
	// all leaves of the DAG rooted at this object relate to the fqdn (and its aliases)
	Fqdn string `json:"fqdn"`
	// A set of aliases for the domain, these may be alternative fqdns which are considered
	// aliases of the primary fqdn
	Aliases []string `json:"aliases"`
	// If present describes tls properties. The CNI names that will be matched on
	// are described in fqdn and aliases, the tls.secretName secret must contain a
	// matching certificate
	TLS `json:"tls"`
}

// TLS describes tls properties. The CNI names that will be matched on
// are described in fqdn and aliases, the tls.secretName secret must contain a
// matching certificate
type TLS struct {
	// required, the name of a secret in the current namespace
	SecretName string `json:"secretName"`
}

// Route contains the set of routes for a virtual host
type Route struct {
	// Match defines the prefix match
	Match string `json:"match"`
	// Service are the services to proxy traffic
	Services []Service `json:"services"`
	Delegate `json:"delegate"`
}

// Service defines an upstream to proxy traffic to
type Service struct {
	// Name is the name of Kubernetes service to proxy traffic.
	// Names defined here will be used to look up corresponding endpoints which contain the ips to route.
	Name string `json:"name"`
	// Port (defined as Integer) to proxy traffic to since a service can have multiple defined
	Port int `json:"port"`
	// Weight defines percentage of traffic to balance traffic
	Weight *int `json:"weight"`
}

// Delegate allows for passing delgating VHosts to other IngressRoutes
type Delegate struct {
	// Name of the IngressRoute
	Name string `json:"name"`
	// Namespace of the IngressRoute
	Namespace string `json:"namespace"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// IngressRoute is an Ingress CRD specificiation
type IngressRoute struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec IngressRouteSpec `json:"spec"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// IngressRouteList is a list of IngressRoutes
type IngressRouteList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []IngressRoute `json:"items"`
}
