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
	VirtualHost *VirtualHost `json:"virtualhost"`
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
	TLS *TLS `json:"tls"`
}

// TLS describes tls properties. The CNI names that will be matched on
// are described in fqdn and aliases, the tls.secretName secret must contain a
// matching certificate
type TLS struct {
	// required, the name of a secret in the current namespace
	SecretName string `json:"secretName"`
	// Minimum TLS version this vhost should negotiate
	MinimumProtocolVersion string `json:"minimumProtocolVersion"`
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
	Weight int `json:"weight"`
	// HealthCheck defines optional healthchecks on the upstream service
	HealthCheck *HealthCheck `json:"healthCheck"`
	// LB Algorithm to apply (see https://github.com/heptio/contour/blob/master/design/ingressroute-design.md#load-balancing)
	Strategy string `json:"strategy"`
}

// Delegate allows for passing delgating VHosts to other IngressRoutes
type Delegate struct {
	// Name of the IngressRoute
	Name string `json:"name"`
	// Namespace of the IngressRoute
	Namespace string `json:"namespace"`
}

// HealthCheck defines optional healthchecks on the upstream service
type HealthCheck struct {
	// HTTP endpoint used to perform health checks on upstream service
	Path string `json:"path"`
	// The value of the host header in the HTTP health check request.
	// If left empty (default value), the name "contour-envoy-healthcheck"
	// will be used.
	Host string `json:"host"`
	// The interval (seconds) between health checks
	IntervalSeconds int64 `json:"intervalSeconds"`
	// The time to wait (seconds) for a health check response
	TimeoutSeconds int64 `json:"timeoutSeconds"`
	// The number of unhealthy health checks required before a host is marked unhealthy
	UnhealthyThresholdCount uint32 `json:"unhealthyThresholdCount"`
	// The number of healthy health checks required before a host is marked healthy
	HealthyThresholdCount uint32 `json:"healthyThresholdCount"`
}

// Status reports the current state of the IngressRoute
type Status struct {
	CurrentStatus string `json:"currentStatus"`
	Description   string `json:"description"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// IngressRoute is an Ingress CRD specificiation
type IngressRoute struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   IngressRouteSpec `json:"spec"`
	Status `json:"status"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// IngressRouteList is a list of IngressRoutes
type IngressRouteList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []IngressRoute `json:"items"`
}
