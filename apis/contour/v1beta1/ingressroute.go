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
	VirtualHost *VirtualHost `json:"virtualhost,omitempty"`
	// Routes are the ingress routes. If TCPProxy is present, Routes is ignored.
	Routes []Route `json:"routes"`
	// TCPProxy holds TCP proxy information.
	TCPProxy *TCPProxy `json:"tcpproxy,omitempty"`
}

// VirtualHost appears at most once. If it is present, the object is considered
// to be a "root".
type VirtualHost struct {
	// The fully qualified domain name of the root of the ingress tree
	// all leaves of the DAG rooted at this object relate to the fqdn
	Fqdn string `json:"fqdn"`
	// If present describes tls properties. The CNI names that will be matched on
	// are described in fqdn, the tls.secretName secret must contain a
	// matching certificate
	TLS *TLS `json:"tls,omitempty"`
}

// TLS describes tls properties. The CNI names that will be matched on
// are described in fqdn, the tls.secretName secret must contain a
// matching certificate unless tls.passthrough is set to true.
type TLS struct {
	// required, the name of a secret in the current namespace
	SecretName string `json:"secretName,omitempty"`
	// Minimum TLS version this vhost should negotiate
	MinimumProtocolVersion string `json:"minimumProtocolVersion,omitempty"`
	// If Passthrough is set to true, the SecretName will be ignored
	// and the encrypted handshake will be passed through to the
	// backing cluster.
	Passthrough bool `json:"passthrough,omitempty"`
}

// Route contains the set of routes for a virtual host
type Route struct {
	// Match defines the prefix match
	Match string `json:"match"`
	// Services are the services to proxy traffic
	Services []Service `json:"services,omitempty"`
	// Delegate specifies that this route should be delegated to another IngressRoute
	Delegate *Delegate `json:"delegate,omitempty"`
	// Enables websocket support for the route
	EnableWebsockets bool `json:"enableWebsockets,omitempty"`
	// Allow this path to respond to insecure requests over HTTP which are normally
	// not permitted when a `virtualhost.tls` block is present.
	PermitInsecure bool `json:"permitInsecure,omitempty"`
	// Indicates that during forwarding, the matched prefix (or path) should be swapped with this value
	PrefixRewrite string `json:"prefixRewrite,omitempty"`
	// The timeout policy for this route
	TimeoutPolicy *TimeoutPolicy `json:"timeoutPolicy,omitempty"`
	// // The retry policy for this route
	RetryPolicy *RetryPolicy `json:"retryPolicy,omitempty"`
}

// TCPProxy contains the set of services to proxy TCP connections.
type TCPProxy struct {
	// Services are the services to proxy traffic
	Services []Service `json:"services,omitempty"`
	// Delegate specifies that this tcpproxy should be delegated to another IngressRoute
	Delegate *Delegate `json:"delegate,omitempty"`
}

// Service defines an upstream to proxy traffic to
type Service struct {
	// Name is the name of Kubernetes service to proxy traffic.
	// Names defined here will be used to look up corresponding endpoints which contain the ips to route.
	Name string `json:"name"`
	// Port (defined as Integer) to proxy traffic to since a service can have multiple defined
	Port int `json:"port"`
	// Weight defines percentage of traffic to balance traffic
	Weight int `json:"weight,omitempty"`
	// HealthCheck defines optional healthchecks on the upstream service
	HealthCheck *HealthCheck `json:"healthCheck,omitempty"`
	// LB Algorithm to apply (see https://github.com/heptio/contour/blob/master/design/ingressroute-design.md#load-balancing)
	Strategy string `json:"strategy,omitempty"`
	// UpstreamValidation defines how to verify the backend service's certificate
	UpstreamValidation *UpstreamValidation `json:"validation,omitempty"`
}

// Delegate allows for delegating VHosts to other IngressRoutes
type Delegate struct {
	// Name of the IngressRoute
	Name string `json:"name"`
	// Namespace of the IngressRoute
	Namespace string `json:"namespace,omitempty"`
}

// HealthCheck defines optional healthchecks on the upstream service
type HealthCheck struct {
	// HTTP endpoint used to perform health checks on upstream service
	Path string `json:"path"`
	// The value of the host header in the HTTP health check request.
	// If left empty (default value), the name "contour-envoy-healthcheck"
	// will be used.
	Host string `json:"host,omitempty"`
	// The interval (seconds) between health checks
	IntervalSeconds int64 `json:"intervalSeconds"`
	// The time to wait (seconds) for a health check response
	TimeoutSeconds int64 `json:"timeoutSeconds"`
	// The number of unhealthy health checks required before a host is marked unhealthy
	UnhealthyThresholdCount uint32 `json:"unhealthyThresholdCount"`
	// The number of healthy health checks required before a host is marked healthy
	HealthyThresholdCount uint32 `json:"healthyThresholdCount"`
}

// TimeoutPolicy define the attributes associated with timeout
type TimeoutPolicy struct {
	// Timeout for receiving a response from the server after processing a request from client.
	// If not supplied the timeout duration is undefined.
	Request string `json:"request"`
}

// RetryPolicy define the attributes associated with retrying policy
type RetryPolicy struct {
	// NumRetries is maximum allowed number of retries.
	// If not supplied, the number of retries is zero.
	NumRetries int `json:"count"`
	// PerTryTimeout specifies the timeout per retry attempt.
	// Ignored if NumRetries is not supplied.
	PerTryTimeout string `json:"perTryTimeout,omitempty"`
}

// UpstreamValidation defines how to verify the backend service's certificate
type UpstreamValidation struct {
	// Name of the Kubernetes secret be used to validate the certificate presented by the backend
	CACertificate string `json:"caSecret"`
	// Key which is expected to be present in the 'subjectAltName' of the presented certificate
	SubjectName string `json:"subjectName"`
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
