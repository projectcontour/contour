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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Status defines the status properties of the CRD
type Status struct {
	// Defines the current status
	CurrentStatus string `json:"currentStatus"`
	// LastProcessTime records the last tiem the object was processed
	LastProcessTime string `json:"lastProcessTime"`
	// Errors are a list of errors encountered wih config
	Errors []string `json:"errors"`
}

// RouteSpec defines the spec of the CRD
type RouteSpec struct {
	// Strategy defines the load balancer algorithm to apply
	Strategy string `json:"strategy"`
	//LBHealthCheck defines the check checks to apply to upstreams
	LBHealthCheck `json:"lbHealthCheck"`
	// Host is the DNS domain name of the host
	Host string `json:"host"`
	// Routes are the ingress routes
	Routes []IngressRoute `json:"routes"`
}

// LBHealthCheck defines the health check params for the upstream
type LBHealthCheck struct {
	// HTTP endpoint used to perform health checks on upstream service (e.g. /healthz).
	// It expects a 200 response if the host is healthy. The upstream host can return 503
	// if it wants to immediately notify downstream hosts to no longer forward traffic to it
	Path string `json:"path"`
	// IntervalSeconds defines the interval (seconds) between health checks
	IntervalSeconds int `json:"intervalSeconds"`
	// TimeoutSeconds defines the time to wait (seconds) for a health check response
	TimeoutSeconds int `json:"timeoutSeconds"`
	// UnHealthyThresholdCount defines the number of unhealthy health checks required before
	// a host is marked unhealthy
	UnhealthyThresholdCount int `json:"unhealthyThresholdCount"`
}

// IngressRoute defines a single route object
type IngressRoute struct {
	// PathPrefix defines the prefix to apply to the path
	PathPrefix string `json:"pathPrefix"`
	// Strategy defines the load balancer algorithm to apply
	Strategy string `json:"strategy"`
	// Upstreams are the services to proxy traffic
	Upstreams []Upstream `json:"upstreams"`
	//LBHealthCheck defines the check checks to apply to upstreams
	LBHealthCheck `json:"lbHealthCheck"`
}

// Upstream defines an upstream to proxy traffic to
type Upstream struct {
	// ServiceName is the name of Kubernetes service to proxy traffic.
	// Names defined here will be used to look up corresponding endpoints which contain the ips to route.
	ServiceName string `json:"serviceName"`
	// ServicePort (defined as Integer) to proxy traffic to since a service can have multiple defined
	ServicePort int `json:"servicePort"`
	// Weight defines percentage of traffic to balance traffic
	Weight *int `json:"weight"`
	//LBHealthCheck defines the check checks to apply to upstreams
	LBHealthCheck `json:"lbHealthCheck"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Route is an Ingress CRD specificiation
type Route struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Status Status    `json:"status"`
	Spec   RouteSpec `json:"spec"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RouteList is a list of Routes.
type RouteList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []Route `json:"items"`
}
