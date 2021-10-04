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

// ContourConfigurationSpec represents a configuration of a Contour controller.
// It contains most of all the options that can be customized, the
// other remaining options being command line flags.
type ContourConfigurationSpec struct {
	// XDSServer contains parameters for the xDS server.
	// +optional
	// +kubebuilder:default={type: "contour", address: "0.0.0.0", port: 8001, tls: { caFile: "/certs/ca.crt", certFile: "/certs/tls.crt", keyFile: "/certs/tls.key", insecure: false}}
	XDSServer XDSServerConfig `json:"xdsServer"`

	// Ingress contains parameters for ingress options.
	// +optional
	Ingress *IngressConfig `json:"ingress,omitempty"`

	// Debug contains parameters to enable debug logging
	// and debug interfaces inside Contour.
	// +optional
	// +kubebuilder:default={logLevel: "info", kubernetesLogLevel: 0}
	Debug DebugConfig `json:"debug"`

	// Health contains parameters to configure endpoints which Contour
	// exposes to respond to Kubernetes health checks.
	// +optional
	// +kubebuilder:default={address: "0.0.0.0", port: 8000}
	Health HealthConfig `json:"health"`

	// Envoy contains parameters for Envoy as well
	// as how to optionally configure a managed Envoy fleet.
	// +optional
	// +kubebuilder:default={listener: {useProxyProtocol: false, disableAllowChunkedLength: false, connectionBalancer: "", tls: { minimumProtocolVersion: "1.2", cipherSuites: "[ECDHE-ECDSA-AES128-GCM-SHA256|ECDHE-ECDSA-CHACHA20-POLY1305]";"[ECDHE-RSA-AES128-GCM-SHA256|ECDHE-RSA-CHACHA20-POLY1305]";"ECDHE-ECDSA-AES256-GCM-SHA384";"ECDHE-RSA-AES256-GCM-SHA384" }}, service: {name: "envoy", namespace: "projectcontour"}, http: {address: "0.0.0.0", port: 8080, accessLog: "/dev/stdout"}, https: {address: "0.0.0.0", port: 8443, accessLog: "/dev/stdout"}, metrics: {address: "0.0.0.0", port: 8002}, logging: { accessLogFormat: "envoy"}, defaultHTTPVersions: "HTTP/1.1";"HTTP/2", cluster: {dnsLookupFamily: "auto"}, network: { adminPort: 9001}}
	Envoy EnvoyConfig `json:"envoy"`

	// Gateway contains parameters for the gateway-api Gateway that Contour
	// is configured to serve traffic.
	// +optional
	Gateway *GatewayConfig `json:"gateway,omitempty"`

	// HTTPProxy defines parameters on HTTPProxy.
	// +optional
	// +kubebuilder:default={disablePermitInsecure: false}
	HTTPProxy HTTPProxyConfig `json:"httpproxy"`

	// LeaderElection contains leader election parameters.
	// +optional
	// +kubebuilder:default={leaseDuration: "15s", renewDeadline: "10s", retryPeriod: "2s", disableLeaderElection: false, configmap: {name: "leader-elect", namespace: "projectcontour"}}
	LeaderElection LeaderElectionConfig `json:"leaderElection"`

	// EnableExternalNameService allows processing of ExternalNameServices
	// Defaults to disabled for security reasons.
	// +optional
	// +kubebuilder:default=false
	EnableExternalNameService bool `json:"enableExternalNameService"`

	// RateLimitService optionally holds properties of the Rate Limit Service
	// to be used for global rate limiting.
	// +optional
	RateLimitService *RateLimitServiceConfig `json:"rateLimitService,omitempty"`

	// Policy specifies default policy applied if not overridden by the user
	// +optional
	Policy *PolicyConfig `json:"policy,omitempty"`

	// Metrics defines the endpoints Contour uses to serve to metrics.
	// +optional
	// +kubebuilder:default={address: "0.0.0.0", port: 8000}
	Metrics MetricsConfig `json:"metrics"`
}

// XDSServerType is the type of xDS server implementation.
type XDSServerType string

const ContourServerType XDSServerType = "contour"
const EnvoyServerType XDSServerType = "envoy"

// XDSServerConfig holds the config for the Contour xDS server.
type XDSServerConfig struct {
	// Defines the XDSServer to use for `contour serve`.
	// +kubebuilder:validation:Enum=contour;envoy
	Type XDSServerType `json:"type"`

	// Defines the xDS gRPC API address which Contour will serve.
	// +kubebuilder:validation:MinLength=1
	Address string `json:"address"`

	// Defines the xDS gRPC API port which Contour will serve.
	Port int `json:"port"`

	// TLS holds TLS file config details.
	// +optional
	TLS *TLS `json:"tls,omitempty"`
}

// GatewayConfig holds the config for Gateway API controllers.
type GatewayConfig struct {
	// ControllerName is used to determine whether Contour should reconcile a
	// GatewayClass. The string takes the form of "projectcontour.io/<namespace>/contour".
	// If unset, the gatewayclass controller will not be started.
	// +kubebuilder:default="projectcontour.io/projectcontour/contour"
	ControllerName string `json:"controllerName"`
}

// TLS holds TLS file config details.
type TLS struct {
	// CA filename.
	// +optional
	CAFile string `json:"caFile,omitempty"`

	// Client certificate filename.
	// +optional
	CertFile string `json:"certFile,omitempty"`

	// Client key filename.
	// +optional
	KeyFile string `json:"keyFile,omitempty"`

	// Allow serving the xDS gRPC API without TLS.
	Insecure bool `json:"insecure"`
}

// IngressConfig defines ingress specific config items.
type IngressConfig struct {
	// Ingress Class Name Contour should use.
	// +optional
	ClassName *string `json:"className,omitempty"`

	// Address to set in Ingress object status.
	// +optional
	StatusAddress *string `json:"statusAddress,omitempty"`
}

// HealthConfig defines the endpoints Contour will serve to enable health checks.
type HealthConfig struct {
	// Defines the Contour health address interface.
	// +kubebuilder:validation:MinLength=1
	Address string `json:"address"`

	// Defines the Contour health port.
	Port int `json:"port"`
}

// MetricsConfig defines the metrics endpoint.
type MetricsConfig struct {
	// Defines the metrics address interface.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	Address string `json:"address"`

	// Defines the metrics port.
	Port int `json:"port"`
}

// HTTPVersionType is the name of a supported HTTP version.
// +kubebuilder:validation:Enum="HTTP/1.1";"HTTP/2"
type HTTPVersionType string

const HTTPVersion1 HTTPVersionType = "HTTP/1.1"
const HTTPVersion2 HTTPVersionType = "HTTP/2"

// EnvoyConfig defines how Envoy is to be Configured from Contour.
type EnvoyConfig struct {
	// Listener hold various configurable Envoy listener values.
	Listener EnvoyListenerConfig `json:"listener"`

	// Service holds Envoy service parameters for setting Ingress status.
	// +kubebuilder:default={name: "envoy", namespace: "projectcontour"}
	Service NamespacedName `json:"service"`

	// Defines the HTTP Listener for Envoy.
	// +kubebuilder:default={address: "0.0.0.0", port: 8080, accessLog: "/dev/stdout"}
	HTTPListener EnvoyListener `json:"http"`

	// Defines the HTTP Listener for Envoy.
	// +kubebuilder:default={address: "0.0.0.0", port: 8443, accessLog: "/dev/stdout"}
	HTTPSListener EnvoyListener `json:"https"`

	// Metrics defines the endpoints Envoy use to serve to metrics.
	// +kubebuilder:default={address: "0.0.0.0", port: 8002}
	Metrics MetricsConfig `json:"metrics"`

	// ClientCertificate defines the namespace/name of the Kubernetes
	// secret containing the client certificate and private key
	// to be used when establishing TLS connection to upstream
	// cluster.
	// +optional
	ClientCertificate *NamespacedName `json:"clientCertificate,omitempty"`

	// Logging defines how Envoy's logs can be configured.
	Logging EnvoyLogging `json:"logging"`

	// DefaultHTTPVersions defines the default set of HTTPS
	// versions the proxy should accept. HTTP versions are
	// strings of the form "HTTP/xx". Supported versions are
	// "HTTP/1.1" and "HTTP/2".
	DefaultHTTPVersions []HTTPVersionType `json:"defaultHTTPVersions"`

	// Timeouts holds various configurable timeouts that can
	// be set in the config file.
	// +optional
	Timeouts *TimeoutParameters `json:"timeouts,omitempty"`

	// Cluster holds various configurable Envoy cluster values that can
	// be set in the config file.
	Cluster ClusterParameters `json:"cluster"`

	// Network holds various configurable Envoy network values.
	Network NetworkParameters `json:"network"`
}

// LogLevel is the logging levels available.
type LogLevel string

const InfoLog LogLevel = "info"
const DebugLog LogLevel = "debug"

// DebugConfig contains Contour specific troubleshooting options.
type DebugConfig struct {
	// Defines the Contour debug address interface.
	// +optional
	Address string `json:"address"`

	// Defines the Contour debug address port.
	// +optional
	Port int `json:"port"`

	// DebugLogLevel defines the log level which Contour will
	// use when outputting log information.
	// +kubebuilder:validation:Enum=info;debug
	DebugLogLevel LogLevel `json:"logLevel"`

	// KubernetesDebugLogLevel defines the log level which Contour will
	// use when outputting Kubernetes specific log information.
	//
	// Details: https://github.com/kubernetes/community/blob/master/contributors/devel/sig-instrumentation/logging.md
	// +optional
	// +kubebuilder:default=0
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=9
	KubernetesDebugLogLevel uint `json:"kubernetesLogLevel"`
}

// EnvoyListenerConfig hold various configurable Envoy listener values.
type EnvoyListenerConfig struct {
	// Use PROXY protocol for all listeners.
	UseProxyProto bool `json:"useProxyProtocol"`

	// DisableAllowChunkedLength disables the RFC-compliant Envoy behavior to
	// strip the "Content-Length" header if "Transfer-Encoding: chunked" is
	// also set. This is an emergency off-switch to revert back to Envoy's
	// default behavior in case of failures. Please file an issue if failures
	// are encountered.
	// See: https://github.com/projectcontour/contour/issues/3221
	DisableAllowChunkedLength bool `json:"disableAllowChunkedLength"`

	// ConnectionBalancer. If the value is exact, the listener will use the exact connection balancer
	// See https://www.envoyproxy.io/docs/envoy/latest/api-v2/api/v2/listener.proto#envoy-api-msg-listener-connectionbalanceconfig
	// for more information.
	// +kubebuilder:validation:Enum="";"exact"
	ConnectionBalancer string `json:"connectionBalancer"`

	// TLS holds various configurable Envoy TLS listener values.
	TLS EnvoyTLS `json:"tls"`
}

// +kubebuilder:validation:Enum="[ECDHE-ECDSA-AES128-GCM-SHA256|ECDHE-ECDSA-CHACHA20-POLY1305]";"[ECDHE-RSA-AES128-GCM-SHA256|ECDHE-RSA-CHACHA20-POLY1305]";"ECDHE-ECDSA-AES128-GCM-SHA256";"ECDHE-RSA-AES128-GCM-SHA256";"ECDHE-ECDSA-AES128-SHA";"ECDHE-RSA-AES128-SHA";"AES128-GCM-SHA256";"AES128-SHA";"ECDHE-ECDSA-AES256-GCM-SHA384";"ECDHE-RSA-AES256-GCM-SHA384";"ECDHE-ECDSA-AES256-SHA";"ECDHE-RSA-AES256-SHA";"AES256-GCM-SHA384";"AES256-SHA"
type TLSCipherType string

// EnvoyTLS describes tls parameters for Envoy listneners.
type EnvoyTLS struct {
	// MinimumProtocolVersion is the minimum TLS version this vhost should
	// negotiate. Valid options are `1.2` (default) and `1.3`.
	// +kubebuilder:validation:Enum="1.2";"1.3"
	MinimumProtocolVersion string `json:"minimumProtocolVersion"`

	// CipherSuites defines the TLS ciphers to be supported by Envoy TLS
	// listeners when negotiating TLS 1.2. Ciphers are validated against the
	// set that Envoy supports by default. This parameter should only be used
	// by advanced users. Note that these will be ignored when TLS 1.3 is in
	// use.
	//
	//See: https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/transport_sockets/tls/v3/common.proto#extensions-transport-sockets-tls-v3-tlsparameters
	// Note: This list is a superset of what is valid for stock Envoy builds and those using BoringSSL FIPS.
	CipherSuites []TLSCipherType `json:"cipherSuites"`
}

// EnvoyListener defines parameters for an Envoy Listener.
type EnvoyListener struct {
	// Defines an Envoy Listener Address.
	// +kubebuilder:validation:MinLength=1
	Address string `json:"address"`

	// Defines an Envoy listener Port.
	Port int `json:"port"`

	// AccessLog defines where Envoy logs are outputted for this listener.
	AccessLog string `json:"accessLog"`
}

// EnvoyLogging defines how Envoy's logs can be configured.
type EnvoyLogging struct {
	// AccessLogFormat sets the global access log format.
	// Valid options are 'envoy' or 'json'
	// +kubebuilder:validation:Enum="envoy";"json"
	AccessLogFormat AccessLogType `json:"accessLogFormat"`

	// AccessLogFormatString sets the access log format when format is set to `envoy`.
	// When empty, Envoy's default format is used.
	// +optional
	AccessLogFormatString *string `json:"accessLogFormatString,omitempty"`

	// AccessLogFields sets the fields that JSON logging will
	// output when AccessLogFormat is json.
	// +optional
	AccessLogFields AccessLogFields `json:"jsonFields,omitempty"`
}

// TimeoutParameters holds various configurable proxy timeout values.
type TimeoutParameters struct {
	// RequestTimeout sets the client request timeout globally for Contour. Note that
	// this is a timeout for the entire request, not an idle timeout. Omit or set to
	// "infinity" to disable the timeout entirely.
	//
	// See https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/network/http_connection_manager/v3/http_connection_manager.proto#envoy-v3-api-field-extensions-filters-network-http-connection-manager-v3-httpconnectionmanager-request-timeout
	// for more information.
	// +optional
	RequestTimeout *string `json:"requestTimeout,omitempty"`

	// ConnectionIdleTimeout defines how long the proxy should wait while there are
	// no active requests (for HTTP/1.1) or streams (for HTTP/2) before terminating
	// an HTTP connection. Set to "infinity" to disable the timeout entirely.
	//
	// See https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/core/v3/protocol.proto#envoy-v3-api-field-config-core-v3-httpprotocoloptions-idle-timeout
	// for more information.
	// +optional
	ConnectionIdleTimeout *string `json:"connectionIdleTimeout,omitempty"`

	// StreamIdleTimeout defines how long the proxy should wait while there is no
	// request activity (for HTTP/1.1) or stream activity (for HTTP/2) before
	// terminating the HTTP request or stream. Set to "infinity" to disable the
	// timeout entirely.
	//
	// See https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/network/http_connection_manager/v3/http_connection_manager.proto#envoy-v3-api-field-extensions-filters-network-http-connection-manager-v3-httpconnectionmanager-stream-idle-timeout
	// for more information.
	// +optional
	StreamIdleTimeout *string `json:"streamIdleTimeout,omitempty"`

	// MaxConnectionDuration defines the maximum period of time after an HTTP connection
	// has been established from the client to the proxy before it is closed by the proxy,
	// regardless of whether there has been activity or not. Omit or set to "infinity" for
	// no max duration.
	//
	// See https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/core/v3/protocol.proto#envoy-v3-api-field-config-core-v3-httpprotocoloptions-max-connection-duration
	// for more information.
	// +optional
	MaxConnectionDuration *string `json:"maxConnectionDuration,omitempty"`

	// DelayedCloseTimeout defines how long envoy will wait, once connection
	// close processing has been initiated, for the downstream peer to close
	// the connection before Envoy closes the socket associated with the connection.
	//
	// Setting this timeout to 'infinity' will disable it, equivalent to setting it to '0'
	// in Envoy. Leaving it unset will result in the Envoy default value being used.
	//
	// See https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/network/http_connection_manager/v3/http_connection_manager.proto#envoy-v3-api-field-extensions-filters-network-http-connection-manager-v3-httpconnectionmanager-delayed-close-timeout
	// for more information.
	// +optional
	DelayedCloseTimeout *string `json:"delayedCloseTimeout,omitempty"`

	// ConnectionShutdownGracePeriod defines how long the proxy will wait between sending an
	// initial GOAWAY frame and a second, final GOAWAY frame when terminating an HTTP/2 connection.
	// During this grace period, the proxy will continue to respond to new streams. After the final
	// GOAWAY frame has been sent, the proxy will refuse new streams.
	//
	// See https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/network/http_connection_manager/v3/http_connection_manager.proto#envoy-v3-api-field-extensions-filters-network-http-connection-manager-v3-httpconnectionmanager-drain-timeout
	// for more information.
	// +optional
	ConnectionShutdownGracePeriod *string `json:"connectionShutdownGracePeriod,omitempty"`
}

// ClusterDNSFamilyType is the Ip family to use for resolving DNS
// names in an Envoy cluster config.
type ClusterDNSFamilyType string

const AutoClusterDNSFamily ClusterDNSFamilyType = "auto"
const IPv4ClusterDNSFamily ClusterDNSFamilyType = "v4"
const IPv6ClusterDNSFamily ClusterDNSFamilyType = "v6"

// ClusterParameters holds various configurable cluster values.
type ClusterParameters struct {
	// DNSLookupFamily defines how external names are looked up
	// When configured as V4, the DNS resolver will only perform a lookup
	// for addresses in the IPv4 family. If V6 is configured, the DNS resolver
	// will only perform a lookup for addresses in the IPv6 family.
	// If AUTO is configured, the DNS resolver will first perform a lookup
	// for addresses in the IPv6 family and fallback to a lookup for addresses
	// in the IPv4 family.
	// Note: This only applies to externalName clusters.
	//
	// See https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/cluster/v3/cluster.proto.html#envoy-v3-api-enum-config-cluster-v3-cluster-dnslookupfamily
	// for more information.
	// +kubebuilder:default="auto"
	// +kubebuilder:validation:Enum="auto";"v4";"v6"
	DNSLookupFamily ClusterDNSFamilyType `json:"dnsLookupFamily"`
}

// HTTPProxyConfig defines parameters on HTTPProxy.
type HTTPProxyConfig struct {
	// DisablePermitInsecure disables the use of the
	// permitInsecure field in HTTPProxy.
	DisablePermitInsecure bool `json:"disablePermitInsecure"`

	// Restrict Contour to searching these namespaces for root ingress routes.
	// +optional
	RootNamespaces []string `json:"rootNamespaces,omitempty"`

	// FallbackCertificate defines the namespace/name of the Kubernetes secret to
	// use as fallback when a non-SNI request is received.
	// +optional
	FallbackCertificate *NamespacedName `json:"fallbackCertificate,omitempty"`
}

// LeaderElectionConfig holds the config bits for leader election
// inside the  config file.
type LeaderElectionConfig struct {
	// +optional
	LeaseDuration string `json:"leaseDuration,omitempty"`

	// +optional
	RenewDeadline string `json:"renewDeadline,omitempty"`

	// +optional
	RetryPeriod string `json:"retryPeriod,omitempty"`

	// +optional
	Configmap NamespacedName `json:"configmap,omitempty"`

	DisableLeaderElection bool `json:"disableLeaderElection"`
}

// NetworkParameters hold various configurable network values.
type NetworkParameters struct {
	// XffNumTrustedHops defines the number of additional ingress proxy hops from the
	// right side of the x-forwarded-for HTTP header to trust when determining the origin
	// client’s IP address.
	//
	// See https://www.envoyproxy.io/docs/envoy/v1.17.0/api-v3/extensions/filters/network/http_connection_manager/v3/http_connection_manager.proto?highlight=xff_num_trusted_hops
	// for more information.
	// +optional
	XffNumTrustedHops uint32 `json:"numTrustedHops"`

	// Configure the port used to access the Envoy Admin interface.
	// If configured to port "0" then the admin interface is disabled.
	// +kubebuilder:default=9001
	EnvoyAdminPort int `json:"adminPort"`
}

// RateLimitServiceConfig defines properties of a global Rate Limit Service.
type RateLimitServiceConfig struct {
	// ExtensionService identifies the extension service defining the RLS.
	ExtensionService NamespacedName `json:"extensionService,omitempty"`

	// Domain is passed to the Rate Limit Service.
	Domain string `json:"domain"`

	// FailOpen defines whether to allow requests to proceed when the
	// Rate Limit Service fails to respond with a valid rate limit
	// decision within the timeout defined on the extension service.
	FailOpen bool `json:"failOpen"`

	// EnableXRateLimitHeaders defines whether to include the X-RateLimit
	// headers X-RateLimit-Limit, X-RateLimit-Remaining, and X-RateLimit-Reset
	// (as defined by the IETF Internet-Draft linked below), on responses
	// to clients when the Rate Limit Service is consulted for a request.
	//
	// ref. https://tools.ietf.org/id/draft-polli-ratelimit-headers-03.html
	EnableXRateLimitHeaders bool `json:"enableXRateLimitHeaders"`
}

// PolicyConfig holds default policy used if not explicitly set by the user
type PolicyConfig struct {
	// RequestHeadersPolicy defines the request headers set/removed on all routes
	// +optional
	RequestHeadersPolicy *HeadersPolicy `json:"requestHeaders,omitempty"`

	// ResponseHeadersPolicy defines the response headers set/removed on all routes
	// +optional
	ResponseHeadersPolicy *HeadersPolicy `json:"responseHeaders,omitempty"`

	// ApplyToIngress determines if the Policies will apply to ingress objects
	// +optional
	ApplyToIngress bool `json:"applyToIngress"`
}

type HeadersPolicy struct {
	// +optional
	Set map[string]string `json:"set,omitempty"`

	// +optional
	Remove []string `json:"remove,omitempty"`
}

// NamespacedName defines the namespace/name of the Kubernetes resource referred from the config file.
// Used for Contour config YAML file parsing, otherwise we could use K8s types.NamespacedName.
type NamespacedName struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// ContourConfigurationStatus defines the observed state of a ContourConfiguration resource.
type ContourConfigurationStatus struct {
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
// +kubebuilder:resource:scope=Namespaced,shortName=contourconfig

// ContourConfiguration is the schema for a Contour instance.
type ContourConfiguration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec ContourConfigurationSpec `json:"spec"`

	// +optional
	Status ContourConfigurationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ContourConfigurationList contains a list of Contour configuration resources.
type ContourConfigurationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ContourConfiguration `json:"items"`
}
