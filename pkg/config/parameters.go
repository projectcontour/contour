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

package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/util/validation"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
)

// ServerType is the name of a xDS server implementation.
type ServerType string

const (
	ContourServerType ServerType = "contour"
	EnvoyServerType   ServerType = "envoy"
)

// Validate the xDS server type.
func (s ServerType) Validate() error {
	switch s {
	case ContourServerType, EnvoyServerType:
		return nil
	default:
		return fmt.Errorf("invalid xDS server type %q", s)
	}
}

// Validate ensures that GatewayRef namespace/name is specified.
func (g *GatewayParameters) Validate() error {
	if g != nil && (g.GatewayRef.Namespace == "" || g.GatewayRef.Name == "") {
		return fmt.Errorf("invalid Gateway parameters specified: gateway ref namespace and name must be provided")
	}

	return nil
}

// ResourceVersion is a version of an xDS server.
type ResourceVersion string

const XDSv3 ResourceVersion = "v3"

// Validate the xDS server versions.
func (s ResourceVersion) Validate() error {
	switch s {
	case XDSv3:
		return nil
	default:
		return fmt.Errorf("invalid xDS version %q", s)
	}
}

// ClusterDNSFamilyType is the Ip family to use for resolving DNS
// names in an Envoy cluster configuration.
type ClusterDNSFamilyType string

func (c ClusterDNSFamilyType) Validate() error {
	switch c {
	case AutoClusterDNSFamily, IPv4ClusterDNSFamily, IPv6ClusterDNSFamily, AllClusterDNSFamily:
		return nil
	default:
		return fmt.Errorf("invalid cluster DNS lookup family %q", c)
	}
}

const (
	AutoClusterDNSFamily ClusterDNSFamilyType = "auto"
	IPv4ClusterDNSFamily ClusterDNSFamilyType = "v4"
	IPv6ClusterDNSFamily ClusterDNSFamilyType = "v6"
	AllClusterDNSFamily  ClusterDNSFamilyType = "all"
)

// ServerHeaderTransformation defines the action to be applied to the Server header on the response path
type ServerHeaderTransformationType string

func (s ServerHeaderTransformationType) Validate() error {
	switch s {
	case OverwriteServerHeader, AppendIfAbsentServerHeader, PassThroughServerHeader:
		return nil
	default:
		return fmt.Errorf("invalid server header transformation %q", s)
	}
}

const (
	OverwriteServerHeader      ServerHeaderTransformationType = "overwrite"
	AppendIfAbsentServerHeader ServerHeaderTransformationType = "append_if_absent"
	PassThroughServerHeader    ServerHeaderTransformationType = "pass_through"
)

// AccessLogType is the name of a supported access logging mechanism.
type AccessLogType string

func (a AccessLogType) Validate() error {
	return contour_v1alpha1.AccessLogType(a).Validate()
}

const (
	EnvoyAccessLog AccessLogType = "envoy"
	JSONAccessLog  AccessLogType = "json"
)

type AccessLogFields []string

func (a AccessLogFields) Validate() error {
	return contour_v1alpha1.AccessLogJSONFields(a).Validate()
}

func (a AccessLogFields) AsFieldMap() map[string]string {
	return contour_v1alpha1.AccessLogJSONFields(a).AsFieldMap()
}

// AccessLogFormatterExtensions returns a list of formatter extension names required by the access log format.
func (p Parameters) AccessLogFormatterExtensions() []string {
	el := &contour_v1alpha1.EnvoyLogging{
		AccessLogFormat:       contour_v1alpha1.AccessLogType(p.AccessLogFormat),
		AccessLogFormatString: p.AccessLogFormatString,
		AccessLogJSONFields:   contour_v1alpha1.AccessLogJSONFields(p.AccessLogFields),
		AccessLogLevel:        contour_v1alpha1.AccessLogLevel(p.AccessLogLevel),
	}
	return el.AccessLogFormatterExtensions()
}

// HTTPVersionType is the name of a supported HTTP version.
type HTTPVersionType string

func (h HTTPVersionType) Validate() error {
	switch h {
	case HTTPVersion1, HTTPVersion2:
		return nil
	default:
		return fmt.Errorf("invalid HTTP version %q", h)
	}
}

const (
	HTTPVersion1 HTTPVersionType = "http/1.1"
	HTTPVersion2 HTTPVersionType = "http/2"
)

// NamespacedName defines the namespace/name of the Kubernetes resource referred from the configuration file.
// Used for Contour configuration YAML file parsing, otherwise we could use K8s types.NamespacedName.
type NamespacedName struct {
	Name      string `yaml:"name"`
	Namespace string `yaml:"namespace"`
}

// Validate that both name fields are present, or neither are.
func (n NamespacedName) Validate() error {
	if len(strings.TrimSpace(n.Name)) == 0 && len(strings.TrimSpace(n.Namespace)) == 0 {
		return nil
	}

	if len(strings.TrimSpace(n.Namespace)) == 0 {
		return errors.New("namespace must be defined")
	}

	if len(strings.TrimSpace(n.Name)) == 0 {
		return errors.New("name must be defined")
	}

	return nil
}

// TLSParameters holds configuration file TLS configuration details.
type TLSParameters struct {
	ProtocolParameters `yaml:",inline"`

	// FallbackCertificate defines the namespace/name of the Kubernetes secret to
	// use as fallback when a non-SNI request is received.
	FallbackCertificate NamespacedName `yaml:"fallback-certificate,omitempty"`

	// ClientCertificate defines the namespace/name of the Kubernetes
	// secret containing the client certificate and private key
	// to be used when establishing TLS connection to upstream
	// cluster.
	ClientCertificate NamespacedName `yaml:"envoy-client-certificate,omitempty"`
}

// ProtocolParameters holds configuration details for TLS protocol specifics.
type ProtocolParameters struct {
	MinimumProtocolVersion string `yaml:"minimum-protocol-version"`
	MaximumProtocolVersion string `yaml:"maximum-protocol-version"`

	// CipherSuites defines the TLS ciphers to be supported by Envoy TLS
	// listeners when negotiating TLS 1.2. Ciphers are validated against the
	// set that Envoy supports by default. This parameter should only be used
	// by advanced users. Note that these will be ignored when TLS 1.3 is in
	// use.
	CipherSuites TLSCiphers `yaml:"cipher-suites,omitempty"`
}

// Validate TLS fallback certificate, client certificate, and cipher suites
func (t TLSParameters) Validate() error {
	// Check TLS secret names.
	if err := t.FallbackCertificate.Validate(); err != nil {
		return fmt.Errorf("invalid TLS fallback certificate: %w", err)
	}

	if err := t.ClientCertificate.Validate(); err != nil {
		return fmt.Errorf("invalid TLS client certificate: %w", err)
	}

	if err := t.ProtocolParameters.Validate(); err != nil {
		return fmt.Errorf("invalid TLS Protocol Parameters: %w", err)
	}

	return nil
}

// Validate TLS protocol versions and cipher suites
func (t ProtocolParameters) Validate() error {
	if err := t.CipherSuites.Validate(); err != nil {
		return fmt.Errorf("invalid TLS cipher suites: %w", err)
	}

	return contour_v1alpha1.ValidateTLSProtocolVersions(t.MinimumProtocolVersion, t.MaximumProtocolVersion)
}

// ServerParameters holds the configuration for the Contour xDS server.
type ServerParameters struct {
	// Defines the XDSServer to use for `contour serve`.
	// Defaults to "envoy"
	// Deprecated: this field will be removed in a future release when
	// the `contour` xDS server implementation is removed.
	XDSServerType ServerType `yaml:"xds-server-type,omitempty"`
}

// GatewayParameters holds the configuration for Gateway API controllers.
type GatewayParameters struct {
	// GatewayRef defines the specific Gateway that this Contour
	// instance corresponds to.
	GatewayRef NamespacedName `yaml:"gatewayRef"`
}

// TimeoutParameters holds various configurable proxy timeout values.
type TimeoutParameters struct {
	// RequestTimeout sets the client request timeout globally for Contour. Note that
	// this is a timeout for the entire request, not an idle timeout. Omit or set to
	// "infinity" to disable the timeout entirely.
	//
	// See https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/network/http_connection_manager/v3/http_connection_manager.proto#envoy-v3-api-field-extensions-filters-network-http-connection-manager-v3-httpconnectionmanager-request-timeout
	// for more information.
	RequestTimeout string `yaml:"request-timeout,omitempty"`

	// ConnectionIdleTimeout defines how long the proxy should wait while there are
	// no active requests (for HTTP/1.1) or streams (for HTTP/2) before terminating
	// an HTTP connection. Set to "infinity" to disable the timeout entirely.
	//
	// See https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/core/v3/protocol.proto#envoy-v3-api-field-config-core-v3-httpprotocoloptions-idle-timeout
	// for more information.
	ConnectionIdleTimeout string `yaml:"connection-idle-timeout,omitempty"`

	// StreamIdleTimeout defines how long the proxy should wait while there is no
	// request activity (for HTTP/1.1) or stream activity (for HTTP/2) before
	// terminating the HTTP request or stream. Set to "infinity" to disable the
	// timeout entirely.
	//
	// See https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/network/http_connection_manager/v3/http_connection_manager.proto#envoy-v3-api-field-extensions-filters-network-http-connection-manager-v3-httpconnectionmanager-stream-idle-timeout
	// for more information.
	StreamIdleTimeout string `yaml:"stream-idle-timeout,omitempty"`

	// MaxConnectionDuration defines the maximum period of time after an HTTP connection
	// has been established from the client to the proxy before it is closed by the proxy,
	// regardless of whether there has been activity or not. Omit or set to "infinity" for
	// no max duration.
	//
	// See https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/core/v3/protocol.proto#envoy-v3-api-field-config-core-v3-httpprotocoloptions-max-connection-duration
	// for more information.
	MaxConnectionDuration string `yaml:"max-connection-duration,omitempty"`

	// DelayedCloseTimeout defines how long envoy will wait, once connection
	// close processing has been initiated, for the downstream peer to close
	// the connection before Envoy closes the socket associated with the connection.
	//
	// Setting this timeout to 'infinity' will disable it, equivalent to setting it to '0'
	// in Envoy. Leaving it unset will result in the Envoy default value being used.
	//
	// See https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/network/http_connection_manager/v3/http_connection_manager.proto#envoy-v3-api-field-extensions-filters-network-http-connection-manager-v3-httpconnectionmanager-delayed-close-timeout
	// for more information.
	DelayedCloseTimeout string `yaml:"delayed-close-timeout,omitempty"`

	// ConnectionShutdownGracePeriod defines how long the proxy will wait between sending an
	// initial GOAWAY frame and a second, final GOAWAY frame when terminating an HTTP/2 connection.
	// During this grace period, the proxy will continue to respond to new streams. After the final
	// GOAWAY frame has been sent, the proxy will refuse new streams.
	//
	// See https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/network/http_connection_manager/v3/http_connection_manager.proto#envoy-v3-api-field-extensions-filters-network-http-connection-manager-v3-httpconnectionmanager-drain-timeout
	// for more information.
	ConnectionShutdownGracePeriod string `yaml:"connection-shutdown-grace-period,omitempty"`

	// ConnectTimeout defines how long the proxy should wait when establishing connection to upstream service.
	// If not set, a default value of 2 seconds will be used.
	//
	// See https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/cluster/v3/cluster.proto#envoy-v3-api-field-config-cluster-v3-cluster-connect-timeout
	// for more information.
	// +optional
	ConnectTimeout string `yaml:"connect-timeout,omitempty"`
}

// Validate the timeout parameters.
func (t TimeoutParameters) Validate() error {
	// We can't use `timeout.Parse` for validation here because
	// that would make an exported package depend on an internal
	// package.
	v := func(str string) error {
		switch str {
		case "", "infinity", "infinite":
			return nil
		default:
			_, err := time.ParseDuration(str)
			return err
		}
	}

	if err := v(t.RequestTimeout); err != nil {
		return fmt.Errorf("invalid request timeout %q: %w", t.RequestTimeout, err)
	}

	if err := v(t.ConnectionIdleTimeout); err != nil {
		return fmt.Errorf("connection idle timeout %q: %w", t.ConnectionIdleTimeout, err)
	}

	if err := v(t.StreamIdleTimeout); err != nil {
		return fmt.Errorf("stream idle timeout %q: %w", t.StreamIdleTimeout, err)
	}

	if err := v(t.MaxConnectionDuration); err != nil {
		return fmt.Errorf("max connection duration %q: %w", t.MaxConnectionDuration, err)
	}

	if err := v(t.DelayedCloseTimeout); err != nil {
		return fmt.Errorf("delayed close timeout %q: %w", t.DelayedCloseTimeout, err)
	}

	if err := v(t.ConnectionShutdownGracePeriod); err != nil {
		return fmt.Errorf("connection shutdown grace period %q: %w", t.ConnectionShutdownGracePeriod, err)
	}

	// ConnectTimeout is normally implicitly set to 2s in Defaults().
	// ConnectTimeout cannot be "infinite" so use time.ParseDuration() directly instead of v().
	if t.ConnectTimeout != "" {
		if _, err := time.ParseDuration(t.ConnectTimeout); err != nil {
			return fmt.Errorf("connect timeout %q: %w", t.ConnectTimeout, err)
		}
	}

	return nil
}

type HeadersPolicy struct {
	Set    map[string]string `yaml:"set,omitempty"`
	Remove []string          `yaml:"remove,omitempty"`
}

func (h HeadersPolicy) Validate() error {
	for key := range h.Set {
		if msgs := validation.IsHTTPHeaderName(key); len(msgs) != 0 {
			return fmt.Errorf("invalid header name %q: %v", key, msgs)
		}
	}
	for _, val := range h.Remove {
		if msgs := validation.IsHTTPHeaderName(val); len(msgs) != 0 {
			return fmt.Errorf("invalid header name %q: %v", val, msgs)
		}
	}
	return nil
}

// PolicyParameters holds default policy used if not explicitly set by the user
type PolicyParameters struct {
	// RequestHeadersPolicy defines the request headers set/removed on all routes
	RequestHeadersPolicy HeadersPolicy `yaml:"request-headers,omitempty"`

	// ResponseHeadersPolicy defines the response headers set/removed on all routes
	ResponseHeadersPolicy HeadersPolicy `yaml:"response-headers,omitempty"`

	// ApplyToIngress determines if the Policies will apply to ingress objects
	ApplyToIngress bool `yaml:"applyToIngress,omitempty"`
}

// Validate the header parameters.
func (h PolicyParameters) Validate() error {
	if err := h.RequestHeadersPolicy.Validate(); err != nil {
		return err
	}
	return h.ResponseHeadersPolicy.Validate()
}

// ClusterParameters holds various configurable cluster values.
type ClusterParameters struct {
	// DNSLookupFamily defines how external names are looked up
	// When configured as V4, the DNS resolver will only perform a lookup
	// for addresses in the IPv4 family. If V6 is configured, the DNS resolver
	// will only perform a lookup for addresses in the IPv6 family.
	// If AUTO is configured, the DNS resolver will first perform a lookup
	// for addresses in the IPv6 family and fallback to a lookup for addresses
	// in the IPv4 family. If ALL is specified, the DNS resolver will perform a lookup for
	// both IPv4 and IPv6 families, and return all resolved addresses.
	// When this is used, Happy Eyeballs will be enabled for upstream connections.
	// Refer to Happy Eyeballs Support for more information.
	// Note: This only applies to externalName clusters.
	//
	// See https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/cluster/v3/cluster.proto.html#envoy-v3-api-enum-config-cluster-v3-cluster-dnslookupfamily
	// for more information.
	DNSLookupFamily ClusterDNSFamilyType `yaml:"dns-lookup-family"`

	// Defines the maximum requests for upstream connections. If not specified, there is no limit.
	// see https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/core/v3/protocol.proto#envoy-v3-api-msg-config-core-v3-httpprotocoloptions
	// for more information.
	//
	// +optional
	MaxRequestsPerConnection *uint32 `yaml:"max-requests-per-connection,omitempty"`

	// Defines the soft limit on size of the cluster’s new connection read and write buffers
	// see https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/cluster/v3/cluster.proto#envoy-v3-api-field-config-cluster-v3-cluster-per-connection-buffer-limit-bytes
	// for more information.
	//
	// +optional
	PerConnectionBufferLimitBytes *uint32 `yaml:"per-connection-buffer-limit-bytes,omitempty"`

	// GlobalCircuitBreakerDefaults holds configurable global defaults for the circuit breakers.
	//
	// +optional
	GlobalCircuitBreakerDefaults *contour_v1alpha1.CircuitBreakers `yaml:"circuit-breakers,omitempty"`

	// UpstreamTLS contains the TLS policy parameters for upstream connections
	UpstreamTLS ProtocolParameters `yaml:"upstream-tls,omitempty"`
}

func (p *ClusterParameters) Validate() error {
	if p == nil {
		return nil
	}

	if p.MaxRequestsPerConnection != nil && *p.MaxRequestsPerConnection < 1 {
		return fmt.Errorf("invalid max connections per request value %q set on cluster, minimum value is 1", *p.MaxRequestsPerConnection)
	}

	if p.PerConnectionBufferLimitBytes != nil && *p.PerConnectionBufferLimitBytes < 1 {
		return fmt.Errorf("invalid per connections buffer limit bytes value %q set on cluster, minimum value is 1", *p.PerConnectionBufferLimitBytes)
	}

	if err := p.UpstreamTLS.Validate(); err != nil {
		return err
	}

	return nil
}

// NetworkParameters hold various configurable network values.
type NetworkParameters struct {
	// XffNumTrustedHops defines the number of additional ingress proxy hops from the
	// right side of the x-forwarded-for HTTP header to trust when determining the origin
	// client’s IP address.
	//
	// See https://www.envoyproxy.io/docs/envoy/v1.17.0/api-v3/extensions/filters/network/http_connection_manager/v3/http_connection_manager.proto?highlight=xff_num_trusted_hops
	// for more information.
	XffNumTrustedHops uint32 `yaml:"num-trusted-hops,omitempty"`

	// Configure the port used to access the Envoy Admin interface.
	// If configured to port "0" then the admin interface is disabled.
	EnvoyAdminPort int `yaml:"admin-port,omitempty"`
}

// ListenerParameters hold various configurable listener values.
type ListenerParameters struct {
	// ConnectionBalancer. If the value is exact, the listener will use the exact connection balancer
	// See https://www.envoyproxy.io/docs/envoy/latest/api-v2/api/v2/listener.proto#envoy-api-msg-listener-connectionbalanceconfig
	// for more information.
	ConnectionBalancer string `yaml:"connection-balancer"`

	// Defines the maximum requests for downstream connections. If not specified, there is no limit.
	// see https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/core/v3/protocol.proto#envoy-v3-api-msg-config-core-v3-httpprotocoloptions
	// for more information.
	//
	// +optional
	MaxRequestsPerConnection *uint32 `yaml:"max-requests-per-connection,omitempty"`

	// Defines the soft limit on size of the listener’s new connection read and write buffers
	// see https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/listener/v3/listener.proto#envoy-v3-api-field-config-listener-v3-listener-per-connection-buffer-limit-bytes
	// for more information.
	//
	// +optional
	PerConnectionBufferLimitBytes *uint32 `yaml:"per-connection-buffer-limit-bytes,omitempty"`

	// SocketOptions is used to set socket options for listeners.
	SocketOptions SocketOptions `yaml:"socket-options"`

	// Defines the limit on number of HTTP requests that Envoy will process from a single
	// connection in a single I/O cycle. Requests over this limit are processed in subsequent
	// I/O cycles. Can be used as a mitigation for CVE-2023-44487 when abusive traffic is
	// detected. Configures the http.max_requests_per_io_cycle Envoy runtime setting. The default
	// value when this is not set is no limit.
	MaxRequestsPerIOCycle *uint32 `yaml:"max-requests-per-io-cycle,omitempty"`

	// Defines the value for SETTINGS_MAX_CONCURRENT_STREAMS Envoy will advertise in the
	// SETTINGS frame in HTTP/2 connections and the limit for concurrent streams allowed
	// for a peer on a single HTTP/2 connection. It is recommended to not set this lower
	// than 100 but this field can be used to bound resource usage by HTTP/2 connections
	// and mitigate attacks like CVE-2023-44487. The default value when this is not set is
	// unlimited.
	HTTP2MaxConcurrentStreams *uint32 `yaml:"http2-max-concurrent-streams,omitempty"`

	// Defines the limit on number of active connections to a listener. The limit is applied
	// per listener. The default value when this is not set is unlimited.
	//
	// +optional
	MaxConnectionsPerListener *uint32 `yaml:"max-connections-per-listener,omitempty"`

	// MaxConnectionsToAcceptPerSocketEvent defines the maximum number of connections
	// Envoy will accept from the kernel per socket event.
	// See: https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/listener/v3/listener.proto
	MaxConnectionsToAcceptPerSocketEvent *uint32 `yaml:"max-connections-to-accept-per-socket-event,omitempty"`
}

func (p *ListenerParameters) Validate() error {
	if p == nil {
		return nil
	}

	if p.ConnectionBalancer != "" && p.ConnectionBalancer != "exact" {
		return fmt.Errorf("invalid listener connection balancer value %q, only 'exact' connection balancing is supported for now", p.ConnectionBalancer)
	}

	if p.MaxRequestsPerConnection != nil && *p.MaxRequestsPerConnection < 1 {
		return fmt.Errorf("invalid max connections per request value %q set on listener, minimum value is 1", *p.MaxRequestsPerConnection)
	}

	if p.PerConnectionBufferLimitBytes != nil && *p.PerConnectionBufferLimitBytes < 1 {
		return fmt.Errorf("invalid per connections buffer limit bytes value %q set on listener, minimum value is 1", *p.PerConnectionBufferLimitBytes)
	}

	if p.MaxRequestsPerIOCycle != nil && *p.MaxRequestsPerIOCycle < 1 {
		return fmt.Errorf("invalid max connections per IO cycle value %q set on listener, minimum value is 1", *p.MaxRequestsPerIOCycle)
	}

	if p.HTTP2MaxConcurrentStreams != nil && *p.HTTP2MaxConcurrentStreams < 1 {
		return fmt.Errorf("invalid max HTTP/2 concurrent streams value %q set on listener, minimum value is 1", *p.HTTP2MaxConcurrentStreams)
	}

	if p.MaxConnectionsPerListener != nil && *p.MaxConnectionsPerListener < 1 {
		return fmt.Errorf("invalid max connections per listener value %q set on listener, minimum value is 1", *p.MaxConnectionsPerListener)
	}

	if p.MaxConnectionsToAcceptPerSocketEvent != nil && *p.MaxConnectionsToAcceptPerSocketEvent == 0 {
		return fmt.Errorf("max-connections-to-accept-per-socket-event must be greater than 0")
	}

	return p.SocketOptions.Validate()
}

// SocketOptions defines configurable socket options for Envoy listeners.
type SocketOptions struct {
	// Defines the value for IPv4 TOS field (including 6 bit DSCP field) for IP packets originating from Envoy listeners.
	// Single value is applied to all listeners.
	// The value must be in the range 0-255, 0 means socket option is not set.
	// If listeners are bound to IPv6-only addresses, setting this option will cause an error.
	TOS int32 `yaml:"tos"`

	// Defines the value for IPv6 Traffic Class field (including 6 bit DSCP field) for IP packets originating from the Envoy listeners.
	// Single value is applied to all listeners.
	// The value must be in the range 0-255, 0 means socket option is not set.
	// If listeners are bound to IPv4-only addresses, setting this option will cause an error.
	TrafficClass int32 `yaml:"traffic-class"`
}

func (p *SocketOptions) Validate() error {
	if p == nil {
		return nil
	}

	if p.TOS < 0 || p.TOS > 255 {
		return fmt.Errorf("invalid listener IPv4 TOS value %d, must be in the range 0-255", p.TOS)
	}

	if p.TrafficClass < 0 || p.TrafficClass > 255 {
		return fmt.Errorf("invalid listener IPv6 TrafficClass value %d, must be in the range 0-255", p.TrafficClass)
	}

	return nil
}

// Parameters contains the configuration file parameters for the
// Contour ingress controller.
type Parameters struct {
	// Enable debug logging
	Debug bool

	// Kubernetes client parameters.
	InCluster       bool    `yaml:"incluster,omitempty"`
	Kubeconfig      string  `yaml:"kubeconfig,omitempty"`
	KubeClientQPS   float32 `yaml:"kubernetesClientQPS,omitempty"`
	KubeClientBurst int     `yaml:"kubernetesClientBurst,omitempty"`

	// Server contains parameters for the xDS server.
	Server ServerParameters `yaml:"server,omitempty"`

	// GatewayConfig contains parameters for the gateway-api Gateway that Contour
	// is configured to serve traffic.
	GatewayConfig *GatewayParameters `yaml:"gateway,omitempty"`

	// Address to be placed in status.loadbalancer field of Ingress objects.
	// May be either a literal IP address or a host name.
	// The value will be placed directly into the relevant field inside the status.loadBalancer struct.
	IngressStatusAddress string `yaml:"ingress-status-address,omitempty"`

	// AccessLogFormat sets the global access log format.
	// Valid options are 'envoy' or 'json'
	AccessLogFormat AccessLogType `yaml:"accesslog-format,omitempty"`

	// AccessLogFormatString sets the access log format when format is set to `envoy`.
	// When empty, Envoy's default format is used.
	AccessLogFormatString string `yaml:"accesslog-format-string,omitempty"`

	// AccessLogFields sets the fields that JSON logging will
	// output when AccessLogFormat is json.
	AccessLogFields AccessLogFields `yaml:"json-fields,omitempty"`

	// AccessLogLevel sets the verbosity level of the access log.
	AccessLogLevel AccessLogLevel `yaml:"accesslog-level,omitempty"`

	// TLS contains TLS policy parameters.
	TLS TLSParameters `yaml:"tls,omitempty"`

	// DisablePermitInsecure disables the use of the
	// permitInsecure field in HTTPProxy.
	DisablePermitInsecure bool `yaml:"disablePermitInsecure,omitempty"`

	// DisableAllowChunkedLength disables the RFC-compliant Envoy behavior to
	// strip the "Content-Length" header if "Transfer-Encoding: chunked" is
	// also set. This is an emergency off-switch to revert back to Envoy's
	// default behavior in case of failures. Please file an issue if failures
	// are encountered.
	// See: https://github.com/projectcontour/contour/issues/3221
	DisableAllowChunkedLength bool `yaml:"disableAllowChunkedLength,omitempty"`

	// DisableMergeSlashes disables Envoy's non-standard merge_slashes path transformation option
	// which strips duplicate slashes from request URL paths.
	DisableMergeSlashes bool `yaml:"disableMergeSlashes,omitempty"`

	// Defines the action to be applied to the Server header on the response path.
	// When configured as overwrite, overwrites any Server header with "envoy".
	// When configured as append_if_absent, if a Server header is present, pass it through, otherwise set it to "envoy".
	// When configured as pass_through, pass through the value of the Server header, and do not append a header if none is present.
	//
	// Contour's default is overwrite.
	ServerHeaderTransformation ServerHeaderTransformationType `yaml:"serverHeaderTransformation,omitempty"`

	// EnableExternalNameService allows processing of ExternalNameServices
	// Defaults to disabled for security reasons.
	// TODO(youngnick): put a link to the issue and CVE here.
	EnableExternalNameService bool `yaml:"enableExternalNameService,omitempty"`

	// Timeouts holds various configurable timeouts that can
	// be set in the config file.
	Timeouts TimeoutParameters `yaml:"timeouts,omitempty"`

	// Policy specifies default policy applied if not overridden by the user
	Policy PolicyParameters `yaml:"policy,omitempty"`

	// Namespace of the envoy service to inspect for Ingress status details.
	EnvoyServiceNamespace string `yaml:"envoy-service-namespace,omitempty"`

	// Name of the envoy service to inspect for Ingress status details.
	EnvoyServiceName string `yaml:"envoy-service-name,omitempty"`

	// DefaultHTTPVersions defines the default set of HTTPS
	// versions the proxy should accept. HTTP versions are
	// strings of the form "HTTP/xx". Supported versions are
	// "HTTP/1.1" and "HTTP/2".
	//
	// If this field not specified, all supported versions are accepted.
	DefaultHTTPVersions []HTTPVersionType `yaml:"default-http-versions"`

	// Cluster holds various configurable Envoy cluster values that can
	// be set in the config file.
	Cluster ClusterParameters `yaml:"cluster,omitempty"`

	// Network holds various configurable Envoy network values.
	Network NetworkParameters `yaml:"network,omitempty"`

	// Listener holds various configurable Envoy Listener values.
	Listener ListenerParameters `yaml:"listener,omitempty"`

	// RateLimitService optionally holds properties of the Rate Limit Service
	// to be used for global rate limiting.
	RateLimitService RateLimitService `yaml:"rateLimitService,omitempty"`

	// GlobalExternalAuthorization optionally holds properties of the global external authorization configuration.
	GlobalExternalAuthorization GlobalExternalAuthorization `yaml:"globalExtAuth,omitempty"`

	// MetricsParameters holds configurable parameters for Contour and Envoy metrics.
	Metrics MetricsParameters `yaml:"metrics,omitempty"`

	// Tracing holds the relevant configuration for exporting trace data to OpenTelemetry.
	Tracing *Tracing `yaml:"tracing,omitempty"`

	// FeatureFlags defines toggle to enable new contour features.
	// available toggles are
	// useEndpointSlices - configures contour to fetch endpoint data
	// from k8s endpoint slices. defaults to true,
	// if false then reading endpoint data from the k8s endpoints.
	FeatureFlags []string `yaml:"featureFlags,omitempty"`
}

// Tracing defines properties for exporting trace data to OpenTelemetry.
type Tracing struct {
	// IncludePodDetail defines a flag.
	// If it is true, contour will add the pod name and namespace to the span of the trace.
	// the default is true.
	// Note: The Envoy pods MUST have the HOSTNAME and CONTOUR_NAMESPACE environment variables set for this to work properly.
	IncludePodDetail *bool `yaml:"includePodDetail,omitempty"`

	// ServiceName defines the name for the service
	// contour's default is contour.
	ServiceName *string `yaml:"serviceName,omitempty"`

	// OverallSampling defines the sampling rate of trace data.
	// the default value is 100.
	OverallSampling *string `yaml:"overallSampling,omitempty"`

	// MaxPathTagLength defines maximum length of the request path
	// to extract and include in the HttpUrl tag.
	// the default value is 256.
	MaxPathTagLength *uint32 `yaml:"maxPathTagLength,omitempty"`

	// CustomTags defines a list of custom tags with unique tag name.
	CustomTags []CustomTag `yaml:"customTags,omitempty"`

	// ExtensionService identifies the extension service defining the otel-collector,
	// formatted as <namespace>/<name>.
	ExtensionService string `yaml:"extensionService"`
}

// CustomTag defines custom tags with unique tag name
// to create tags for the active span.
type CustomTag struct {
	// TagName is the unique name of the custom tag.
	TagName string `yaml:"tagName"`

	// Literal is a static custom tag value.
	// Precisely one of Literal, RequestHeaderName must be set.
	Literal string `yaml:"literal,omitempty"`

	// RequestHeaderName indicates which request header
	// the label value is obtained from.
	// Precisely one of Literal, RequestHeaderName must be set.
	RequestHeaderName string `yaml:"requestHeaderName,omitempty"`
}

// GlobalExternalAuthorization defines properties of global external authorization.
type GlobalExternalAuthorization struct {
	// ExtensionService identifies the extension service defining the RLS,
	// formatted as <namespace>/<name>.
	ExtensionService string `yaml:"extensionService,omitempty"`
	// ServiceAPIType defines the external authorization service API type.
	// It indicates the protocol implemented by the external server, specifying whether it's a raw HTTP authorization server
	// or a gRPC authorization server.
	//
	// +optional
	ServiceAPIType contour_v1.AuthorizationServiceAPIType `json:"serviceAPIType,omitempty"`
	// HttpAuthorizationServerSettings defines configurations for interacting with an external HTTP authorization server.
	//
	// +optional
	HTTPServerSettings *contour_v1.HTTPAuthorizationServerSettings `json:"httpSettings,omitempty"`
	// AuthPolicy sets a default authorization policy for client requests.
	// This policy will be used unless overridden by individual routes.
	//
	// +optional
	AuthPolicy *GlobalAuthorizationPolicy `yaml:"authPolicy,omitempty"`
	// ResponseTimeout configures maximum time to wait for a check response from the authorization server.
	// Timeout durations are expressed in the Go [Duration format](https://godoc.org/time#ParseDuration).
	// Valid time units are "ns", "us" (or "µs"), "ms", "s", "m", "h".
	// The string "infinity" is also a valid input and specifies no timeout.
	//
	// +optional
	ResponseTimeout string `yaml:"responseTimeout,omitempty"`
	// If FailOpen is true, the client request is forwarded to the upstream service
	// even if the authorization server fails to respond. This field should not be
	// set in most cases. It is intended for use only while migrating applications
	// from internal authorization to Contour external authorization.
	//
	// +optional
	FailOpen bool `yaml:"failOpen,omitempty"`
	// WithRequestBody specifies configuration for sending the client request's body to authorization server.
	// +optional
	WithRequestBody *GlobalAuthorizationServerBufferSettings `yaml:"withRequestBody,omitempty"`
}

// GlobalAuthorizationServerBufferSettings enables ExtAuthz filter to buffer client request data and send it as part of authorization request
type GlobalAuthorizationServerBufferSettings struct {
	// MaxRequestBytes sets the maximum size of message body ExtAuthz filter will hold in-memory.
	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=1024
	MaxRequestBytes uint32 `yaml:"maxRequestBytes,omitempty"`
	// If AllowPartialMessage is true, then Envoy will buffer the body until MaxRequestBytes are reached.
	// +optional
	AllowPartialMessage bool `yaml:"allowPartialMessage,omitempty"`
	// If PackAsBytes is true, the body sent to Authorization Server is in raw bytes.
	// +optional
	PackAsBytes bool `yaml:"packAsBytes,omitempty"`
}

// GlobalAuthorizationPolicy modifies how client requests are authenticated.
type GlobalAuthorizationPolicy struct {
	// When true, this field disables client request authentication
	// for the scope of the policy.
	//
	// +optional
	Disabled bool `yaml:"disabled,omitempty"`
	// Context is a set of key/value pairs that are sent to the
	// authentication server in the check request. If a context
	// is provided at an enclosing scope, the entries are merged
	// such that the inner scope overrides matching keys from the
	// outer scope.
	//
	// +optional
	Context map[string]string `yaml:"context,omitempty"`
}

// RateLimitService defines properties of a global Rate Limit Service.
type RateLimitService struct {
	// ExtensionService identifies the extension service defining the RLS,
	// formatted as <namespace>/<name>.
	ExtensionService string `yaml:"extensionService,omitempty"`

	// Domain is passed to the Rate Limit Service.
	Domain string `yaml:"domain,omitempty"`

	// FailOpen defines whether to allow requests to proceed when the
	// Rate Limit Service fails to respond with a valid rate limit
	// decision within the timeout defined on the extension service.
	FailOpen bool `yaml:"failOpen,omitempty"`

	// EnableXRateLimitHeaders defines whether to include the X-RateLimit
	// headers X-RateLimit-Limit, X-RateLimit-Remaining, and X-RateLimit-Reset
	// (as defined by the IETF Internet-Draft linked below), on responses
	// to clients when the Rate Limit Service is consulted for a request.
	//
	// ref. https://tools.ietf.org/id/draft-polli-ratelimit-headers-03.html
	EnableXRateLimitHeaders bool `yaml:"enableXRateLimitHeaders,omitempty"`

	// EnableResourceExhaustedCode enables translating error code 429 to
	// grpc code RESOURCE_EXHAUSTED. When disabled it's translated to UNAVAILABLE
	EnableResourceExhaustedCode bool `yaml:"enableResourceExhaustedCode,omitempty"`

	// DefaultGlobalRateLimitPolicy allows setting a default global rate limit policy for all HTTPProxy
	// HTTPProxy can overwrite this configuration.
	DefaultGlobalRateLimitPolicy *contour_v1.GlobalRateLimitPolicy `yaml:"defaultGlobalRateLimitPolicy,omitempty"`
}

// MetricsParameters defines configuration for metrics server endpoints in both
// Contour and Envoy.
type MetricsParameters struct {
	Contour MetricsServerParameters `yaml:"contour,omitempty"`
	Envoy   MetricsServerParameters `yaml:"envoy,omitempty"`
}

// MetricsServerParameters defines configuration for metrics server.
type MetricsServerParameters struct {
	// Address that metrics server will bind to.
	Address string `yaml:"address,omitempty"`

	// Port that metrics server will bind to.
	Port int `yaml:"port,omitempty"`

	// ServerCert is the file path for server certificate.
	// Optional: required only if HTTPS is used to protect the metrics endpoint.
	ServerCert string `yaml:"server-certificate-path,omitempty"`

	// ServerKey is the file path for the private key which corresponds to the server certificate.
	// Optional: required only if HTTPS is used to protect the metrics endpoint.
	ServerKey string `yaml:"server-key-path,omitempty"`

	// CABundle is the file path for CA certificate(s) used for validating the client certificate.
	// Optional: required only if client certificates shall be validated to protect the metrics endpoint.
	CABundle string `yaml:"ca-certificate-path,omitempty"`
}

// FeatureFlags defines the set of feature flags
// to toggle new contour features.
type FeatureFlags []string

func (p *MetricsParameters) Validate() error {
	if err := p.Contour.Validate(); err != nil {
		return fmt.Errorf("metrics.contour: %v", err)
	}
	if err := p.Envoy.Validate(); err != nil {
		return fmt.Errorf("metrics.envoy: %v", err)
	}

	return nil
}

func (t *Tracing) Validate() error {
	if t == nil {
		return nil
	}

	if t.ExtensionService == "" {
		return errors.New("tracing.extensionService must be defined")
	}

	var customTagNames []string

	for _, customTag := range t.CustomTags {
		var fieldCount int
		if customTag.TagName == "" {
			return errors.New("tracing.customTag.tagName must be defined")
		}

		for _, customTagName := range customTagNames {
			if customTagName == customTag.TagName {
				return fmt.Errorf("tagName %s is duplicate", customTagName)
			}
		}

		if customTag.Literal != "" {
			fieldCount++
		}

		if customTag.RequestHeaderName != "" {
			fieldCount++
		}
		if fieldCount != 1 {
			return errors.New("must set exactly one of Literal or RequestHeaderName")
		}
		customTagNames = append(customTagNames, customTag.TagName)
	}
	return nil
}

func (p *MetricsServerParameters) Validate() error {
	// Check that both certificate and key are provided if either one is provided.
	if (p.ServerCert != "") != (p.ServerKey != "") {
		return fmt.Errorf("you must supply at least server-certificate-path and server-key-path or none of them")
	}

	// Optional client certificate validation can be enabled if server certificate (and consequently also key) is also provided.
	if (p.CABundle != "") && (p.ServerCert == "") {
		return fmt.Errorf("you must supply also server-certificate-path and server-key-path if setting ca-certificate-path")
	}

	return nil
}

// HasTLS returns true if parameters have been provided to enable TLS for metrics.
func (p *MetricsServerParameters) HasTLS() bool {
	return p.ServerCert != "" && p.ServerKey != ""
}

type AccessLogLevel string

func (a AccessLogLevel) Validate() error {
	return contour_v1alpha1.AccessLogLevel(a).Validate()
}

const (
	LogLevelInfo     AccessLogLevel = "info" // Default log level.
	LogLevelError    AccessLogLevel = "error"
	LogLevelCritical AccessLogLevel = "critical"
	LogLevelDisabled AccessLogLevel = "disabled"
)

// Validate verifies that the parameter values do not have any syntax errors.
func (p *Parameters) Validate() error {
	if err := p.Cluster.DNSLookupFamily.Validate(); err != nil {
		return err
	}

	if err := p.Server.XDSServerType.Validate(); err != nil {
		return err
	}

	if err := p.GatewayConfig.Validate(); err != nil {
		return err
	}

	if err := p.AccessLogFormat.Validate(); err != nil {
		return err
	}

	if err := p.AccessLogFields.Validate(); err != nil {
		return err
	}

	if err := p.AccessLogLevel.Validate(); err != nil {
		return err
	}

	if err := contour_v1alpha1.AccessLogFormatString(p.AccessLogFormatString).Validate(); err != nil {
		return err
	}

	if err := p.TLS.Validate(); err != nil {
		return err
	}

	if err := p.Timeouts.Validate(); err != nil {
		return err
	}

	if err := p.Policy.Validate(); err != nil {
		return err
	}

	for _, v := range p.DefaultHTTPVersions {
		if err := v.Validate(); err != nil {
			return err
		}
	}

	if err := p.Metrics.Validate(); err != nil {
		return err
	}

	if err := p.Tracing.Validate(); err != nil {
		return err
	}

	if err := p.Cluster.Validate(); err != nil {
		return err
	}

	return p.Listener.Validate()
}

// Defaults returns the default set of parameters.
func Defaults() Parameters {
	contourNamespace := GetenvOr("CONTOUR_NAMESPACE", "projectcontour")

	return Parameters{
		Debug:      false,
		InCluster:  false,
		Kubeconfig: filepath.Join(os.Getenv("HOME"), ".kube", "config"),
		Server: ServerParameters{
			XDSServerType: EnvoyServerType,
		},
		IngressStatusAddress:       "",
		AccessLogFormat:            DEFAULT_ACCESS_LOG_TYPE,
		AccessLogFields:            DefaultFields,
		AccessLogLevel:             LogLevelInfo,
		TLS:                        TLSParameters{},
		DisablePermitInsecure:      false,
		DisableAllowChunkedLength:  false,
		DisableMergeSlashes:        false,
		ServerHeaderTransformation: OverwriteServerHeader,
		Timeouts: TimeoutParameters{
			// This is chosen as a rough default to stop idle connections wasting resources,
			// without stopping slow connections from being terminated too quickly.
			ConnectionIdleTimeout: "60s",
			ConnectTimeout:        "2s",
		},
		Policy: PolicyParameters{
			RequestHeadersPolicy:  HeadersPolicy{},
			ResponseHeadersPolicy: HeadersPolicy{},
			ApplyToIngress:        false,
		},
		EnvoyServiceName:      "envoy",
		EnvoyServiceNamespace: contourNamespace,
		DefaultHTTPVersions:   []HTTPVersionType{},
		Cluster: ClusterParameters{
			DNSLookupFamily: AutoClusterDNSFamily,
		},
		Network: NetworkParameters{
			XffNumTrustedHops: 0,
			EnvoyAdminPort:    9001,
		},
		Listener: ListenerParameters{
			ConnectionBalancer: "",
		},
	}
}

// Parse reads parameters from a YAML input stream. Any parameters
// not specified by the input are according to Defaults().
func Parse(in io.Reader) (*Parameters, error) {
	conf := Defaults()
	decoder := yaml.NewDecoder(in)

	decoder.KnownFields(true)

	if err := decoder.Decode(&conf); err != nil {
		// The YAML decoder will return EOF if there are
		// no YAML nodes in the results. In this case, we just
		// want to succeed and return the defaults.
		if err != io.EOF {
			return nil, fmt.Errorf("failed to parse configuration: %w", err)
		}
	}

	// Force the version string to match the lowercase version
	// constants (assuming that it will match).
	for i, v := range conf.DefaultHTTPVersions {
		conf.DefaultHTTPVersions[i] = HTTPVersionType(strings.ToLower(string(v)))
	}

	return &conf, nil
}

// GetenvOr reads an environment or return a default value
func GetenvOr(key, defaultVal string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}

	return defaultVal
}
