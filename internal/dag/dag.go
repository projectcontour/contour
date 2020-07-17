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

// Package dag provides a data model, in the form of a directed acyclic graph,
// of the relationship between Kubernetes Ingress, Service, and Secret objects.
package dag

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	envoy_api_v2_auth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	"github.com/projectcontour/contour/internal/k8s"
	v1 "k8s.io/api/core/v1"
)

// A DAG represents a directed acylic graph of objects representing the relationship
// between Kubernetes Ingress objects, the backend Services, and Secret objects.
// The DAG models these relationships as Roots and Vertices.
type DAG struct {
	// roots are the roots of this dag
	roots []Vertex

	// status computed while building this dag.
	statuses map[k8s.FullName]Status
}

// Visit calls fn on each root of this DAG.
func (d *DAG) Visit(fn func(Vertex)) {
	for _, r := range d.roots {
		fn(r)
	}
}

// Statuses returns a slice of Status objects associated with
// the computation of this DAG.
func (d *DAG) Statuses() map[k8s.FullName]Status {
	return d.statuses
}

type MatchCondition interface {
	fmt.Stringer
}

// PrefixMatchCondition matches the start of a URL.
type PrefixMatchCondition struct {
	Prefix string
}

func (pc *PrefixMatchCondition) String() string {
	return "prefix: " + pc.Prefix
}

// RegexMatchCondition matches the URL by regular expression.
type RegexMatchCondition struct {
	Regex string
}

func (rc *RegexMatchCondition) String() string {
	return "regex: " + rc.Regex
}

// HeaderMatchCondition matches request headers by MatchType
type HeaderMatchCondition struct {
	Name      string
	Value     string
	MatchType string
	Invert    bool
}

func (hc *HeaderMatchCondition) String() string {
	details := strings.Join([]string{
		"name=" + hc.Name,
		"value=" + hc.Value,
		"matchtype=", hc.MatchType,
		"invert=", strconv.FormatBool(hc.Invert),
	}, "&")

	return "header: " + details
}

// Route defines the properties of a route to a Cluster.
type Route struct {

	// PathMatchCondition specifies a MatchCondition to match on the request path.
	// Must not be nil.
	PathMatchCondition MatchCondition

	// HeaderMatchConditions specifies a set of additional Conditions to
	// match on the request headers.
	HeaderMatchConditions []HeaderMatchCondition

	Clusters []*Cluster

	// Should this route generate a 301 upgrade if accessed
	// over HTTP?
	HTTPSUpgrade bool

	// Is this a websocket route?
	// TODO(dfc) this should go on the service
	Websocket bool

	// TimeoutPolicy defines the timeout request/idle
	TimeoutPolicy *TimeoutPolicy

	// RetryPolicy defines the retry / number / timeout options for a route
	RetryPolicy *RetryPolicy

	// Indicates that during forwarding, the matched prefix (or path) should be swapped with this value
	PrefixRewrite string

	// Mirror Policy defines the mirroring policy for this Route.
	MirrorPolicy *MirrorPolicy

	// RequestHeadersPolicy defines how headers are managed during forwarding
	RequestHeadersPolicy *HeadersPolicy

	// ResponseHeadersPolicy defines how headers are managed during forwarding
	ResponseHeadersPolicy *HeadersPolicy
}

// HasPathPrefix returns whether this route has a PrefixPathCondition.
func (r *Route) HasPathPrefix() bool {
	_, ok := r.PathMatchCondition.(*PrefixMatchCondition)
	return ok
}

// HasPathRegex returns whether this route has a RegexPathCondition.
func (r *Route) HasPathRegex() bool {
	_, ok := r.PathMatchCondition.(*RegexMatchCondition)
	return ok
}

// TimeoutPolicy defines the timeout policy for a route.
type TimeoutPolicy struct {
	// ResponseTimeout is the timeout applied to the response
	// from the backend server.
	// A timeout of zero implies "use envoy's default"
	// A timeout of -1 represents "infinity"
	ResponseTimeout time.Duration

	// IdleTimeout is the timeout applied to idle connections.
	IdleTimeout time.Duration
}

// RetryPolicy defines the retry / number / timeout options
type RetryPolicy struct {
	// RetryOn specifies the conditions under which retry takes place.
	// If empty, retries will not be performed.
	RetryOn string

	// RetriableStatusCodes specifies the HTTP status codes under which retry takes place.
	RetriableStatusCodes []uint32

	// NumRetries specifies the allowed number of retries.
	// Ignored if RetryOn is blank, or defaults to 1 if RetryOn is set.
	NumRetries uint32

	// PerTryTimeout specifies the timeout per retry attempt.
	// Ignored if RetryOn is blank.
	PerTryTimeout time.Duration
}

// MirrorPolicy defines the mirroring policy for a route.
type MirrorPolicy struct {
	Cluster *Cluster
}

// HeadersPolicy defines how headers are managed during forwarding
type HeadersPolicy struct {
	// HostRewrite defines if a host should be rewritten on upstream requests
	HostRewrite string

	Set    map[string]string
	Remove []string
}

type HeaderValue struct {
	// Name represents a key of a header
	Key string
	// Value represents the value of a header specified by a key
	Value string
}

// PeerValidationContext defines how to validate the certificate on the upstream service.
type PeerValidationContext struct {
	// CACertificate holds a reference to the Secret containing the CA to be used to
	// verify the upstream connection.
	CACertificate *Secret
	// SubjectName holds an optional subject name which Envoy will check against the
	// certificate presented by the upstream.
	SubjectName string
}

// GetCACertificate returns the CA certificate from PeerValidationContext.
func (pvc *PeerValidationContext) GetCACertificate() []byte {
	if pvc == nil || pvc.CACertificate == nil {
		// No validation required.
		return nil
	}
	return pvc.CACertificate.Object.Data[CACertificateKey]
}

// GetSubjectName returns the SubjectName from PeerValidationContext.
func (pvc *PeerValidationContext) GetSubjectName() string {
	if pvc == nil {
		// No validation required.
		return ""
	}
	return pvc.SubjectName
}

func (r *Route) Visit(f func(Vertex)) {
	for _, c := range r.Clusters {
		f(c)
	}
	// Allow any mirror clusters to also be visited so that
	// they are also added to CDS.
	if r.MirrorPolicy != nil && r.MirrorPolicy.Cluster != nil {
		f(r.MirrorPolicy.Cluster)
	}
}

// A VirtualHost represents a named L4/L7 service.
type VirtualHost struct {
	// Name is the fully qualified domain name of a network host,
	// as defined by RFC 3986.
	Name string

	routes map[string]*Route
}

func (v *VirtualHost) addRoute(route *Route) {
	if v.routes == nil {
		v.routes = make(map[string]*Route)
	}
	v.routes[conditionsToString(route)] = route
}

func conditionsToString(r *Route) string {
	s := []string{r.PathMatchCondition.String()}
	for _, cond := range r.HeaderMatchConditions {
		s = append(s, cond.String())
	}
	return strings.Join(s, ",")
}

func (v *VirtualHost) Visit(f func(Vertex)) {
	for _, r := range v.routes {
		f(r)
	}
}

func (v *VirtualHost) Valid() bool {
	// A VirtualHost is valid if it has at least one route.
	return len(v.routes) > 0
}

// A SecureVirtualHost represents a HTTP host protected by TLS.
type SecureVirtualHost struct {
	VirtualHost

	// TLS minimum protocol version. Defaults to envoy_api_v2_auth.TlsParameters_TLS_AUTO
	MinTLSVersion envoy_api_v2_auth.TlsParameters_TlsProtocol

	// The cert and key for this host.
	Secret *Secret

	// FallbackCertificate
	FallbackCertificate *Secret

	// Service to TCP proxy all incoming connections.
	*TCPProxy

	// DownstreamValidation defines how to verify the client's certificate.
	DownstreamValidation *PeerValidationContext
}

func (s *SecureVirtualHost) Visit(f func(Vertex)) {
	s.VirtualHost.Visit(f)
	if s.TCPProxy != nil {
		f(s.TCPProxy)
	}
	if s.Secret != nil {
		f(s.Secret) // secret is not required if vhost is using tls passthrough
	}
}

func (s *SecureVirtualHost) Valid() bool {
	// A SecureVirtualHost is valid if either
	// 1. it has a secret and at least one route.
	// 2. it has a tcpproxy, because the tcpproxy backend may negotiate TLS itself.
	return (s.Secret != nil && len(s.routes) > 0) || s.TCPProxy != nil
}

type Visitable interface {
	Visit(func(Vertex))
}

type Vertex interface {
	Visitable
}

// A Listener represents a TCP socket that accepts
// incoming connections.
type Listener struct {

	// Address is the TCP address to listen on.
	// If blank 0.0.0.0, or ::/0 for IPv6, is assumed.
	Address string

	// Port is the TCP port to listen on.
	Port int

	VirtualHosts []Vertex
}

func (l *Listener) Visit(f func(Vertex)) {
	for _, vh := range l.VirtualHosts {
		f(vh)
	}
}

// TCPProxy represents a cluster of TCP endpoints.
type TCPProxy struct {

	// Clusters is the, possibly weighted, set
	// of upstream services to forward decrypted traffic.
	Clusters []*Cluster
}

func (t *TCPProxy) Visit(f func(Vertex)) {
	for _, s := range t.Clusters {
		f(s)
	}
}

// Service represents a single Kubernetes' Service's Port.
type Service struct {
	Name, Namespace string

	*v1.ServicePort

	// Protocol is the layer 7 protocol of this service
	// One of "", "h2", "h2c", or "tls".
	Protocol string

	// Circuit breaking limits

	// Max connections is maximum number of connections
	// that Envoy will make to the upstream cluster.
	MaxConnections uint32

	// MaxPendingRequests is maximum number of pending
	// requests that Envoy will allow to the upstream cluster.
	MaxPendingRequests uint32

	// MaxRequests is the maximum number of parallel requests that
	// Envoy will make to the upstream cluster.
	MaxRequests uint32

	// MaxRetries is the maximum number of parallel retries that
	// Envoy will allow to the upstream cluster.
	MaxRetries uint32

	// ExternalName is an optional field referencing a dns entry for Service type "ExternalName"
	ExternalName string
}

type servicemeta struct {
	name      string
	namespace string
	port      int32
}

func (s *Service) ToFullName() servicemeta {
	return servicemeta{
		name:      s.Name,
		namespace: s.Namespace,
		port:      s.Port,
	}
}

func (s *Service) Visit(func(Vertex)) {
	// Services are leaves in the DAG.
}

// Cluster holds the connetion specific parameters that apply to
// traffic routed to an upstream service.
type Cluster struct {

	// Upstream is the backend Kubernetes service traffic arriving
	// at this Cluster will be forwarded too.
	Upstream *Service

	// The relative weight of this Cluster compared to its siblings.
	Weight uint32

	// The protocol to use to speak to this cluster.
	Protocol string

	// UpstreamValidation defines how to verify the backend service's certificate
	UpstreamValidation *PeerValidationContext

	// The load balancer type to use when picking a host in the cluster.
	// See https://www.envoyproxy.io/docs/envoy/latest/api-v2/api/v2/cds.proto#envoy-api-enum-cluster-lbpolicy
	LoadBalancerPolicy string

	// Cluster http health check policy
	*HTTPHealthCheckPolicy

	// Cluster tcp health check policy
	*TCPHealthCheckPolicy

	// RequestHeadersPolicy defines how headers are managed during forwarding
	RequestHeadersPolicy *HeadersPolicy

	// ResponseHeadersPolicy defines how headers are managed during forwarding
	ResponseHeadersPolicy *HeadersPolicy

	// SNI is used when a route proxies an upstream using tls.
	// SNI describes how the SNI is set on a Cluster and is configured via RequestHeadersPolicy.Host key.
	// Policies set on service are used before policies set on a route. Otherwise the value of the externalService
	// is used if the route is configured to proxy to an externalService type.
	// If the value is not set, then SNI is not changed.
	SNI string
}

func (c Cluster) Visit(f func(Vertex)) {
	f(c.Upstream)
}

// Secret represents a K8s Secret for TLS usage as a DAG Vertex. A Secret is
// a leaf in the DAG.
type Secret struct {
	Object *v1.Secret
}

func (s *Secret) Name() string       { return s.Object.Name }
func (s *Secret) Namespace() string  { return s.Object.Namespace }
func (s *Secret) Visit(func(Vertex)) {}

// Data returns the contents of the backing secret's map.
func (s *Secret) Data() map[string][]byte {
	return s.Object.Data
}

// Cert returns the secret's tls certificate
func (s *Secret) Cert() []byte {
	return s.Object.Data[v1.TLSCertKey]
}

// PrivateKey returns the secret's tls private key
func (s *Secret) PrivateKey() []byte {
	return s.Object.Data[v1.TLSPrivateKeyKey]
}

// Cluster http health check policy
type HTTPHealthCheckPolicy struct {
	Path               string
	Host               string
	Interval           time.Duration
	Timeout            time.Duration
	UnhealthyThreshold uint32
	HealthyThreshold   uint32
}

// Cluster tcp health check policy
type TCPHealthCheckPolicy struct {
	Interval           time.Duration
	Timeout            time.Duration
	UnhealthyThreshold uint32
	HealthyThreshold   uint32
}
