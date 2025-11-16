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

package v3

import (
	"sort"
	"sync"

	envoy_config_accesslog_v3 "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_filter_network_http_connection_manager_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoy_transport_socket_tls_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	resource "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"k8s.io/apimachinery/pkg/types"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/contour"
	"github.com/projectcontour/contour/internal/contourconfig"
	"github.com/projectcontour/contour/internal/dag"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/protobuf"
	"github.com/projectcontour/contour/internal/sorter"
	"github.com/projectcontour/contour/internal/timeout"
	"github.com/projectcontour/contour/pkg/config"
)

// nolint:revive
const (
	ENVOY_HTTP_LISTENER        = "ingress_http"
	ENVOY_HTTPS_LISTENER       = "ingress_https"
	ENVOY_FALLBACK_ROUTECONFIG = "ingress_fallbackcert"
	DEFAULT_HTTP_ACCESS_LOG    = "/dev/stdout"
	DEFAULT_HTTPS_ACCESS_LOG   = "/dev/stdout"
)

type Listener struct {
	Name    string
	Address string
	Port    int
}

// ListenerConfig holds configuration parameters for building Envoy Listeners.
type ListenerConfig struct {
	// Envoy's HTTP (non TLS) access log path.
	// If not set, defaults to DEFAULT_HTTP_ACCESS_LOG.
	HTTPAccessLog string

	// Envoy's HTTPS (TLS) access log path.
	// If not set, defaults to DEFAULT_HTTPS_ACCESS_LOG.
	HTTPSAccessLog string

	// UseProxyProto configures all listeners to expect a PROXY
	// V1 or V2 preamble.
	// If not set, defaults to false.
	UseProxyProto bool

	// MinimumTLSVersion defines the minimum TLS protocol version the proxy should accept.
	MinimumTLSVersion string

	// MaximumTLSVersion defines the maximum TLS protocol version the proxy should accept.
	MaximumTLSVersion string

	// CipherSuites defines the ciphers Envoy TLS listeners will accept when
	// negotiating TLS 1.2.
	CipherSuites []string

	// DefaultHTTPVersions defines the default set of HTTP
	// versions the proxy should accept. If not specified, all
	// supported versions are accepted. This is applied to both
	// HTTP and HTTPS listeners but has practical effect only for
	// HTTPS, because we don't support h2c.
	DefaultHTTPVersions []envoy_v3.HTTPVersionType

	// AccessLogType defines if Envoy logs should be output as Envoy's default or JSON.
	// Valid values: 'envoy', 'json'
	// If not set, defaults to 'envoy'
	AccessLogType contour_v1alpha1.AccessLogType

	// AccessLogJSONFields sets the fields that should be shown in JSON logs.
	// Valid entries are the keys from internal/envoy/accesslog.go:jsonheaders
	// Defaults to a particular set of fields.
	AccessLogJSONFields contour_v1alpha1.AccessLogJSONFields

	// AccessLogFormatString sets the format string to be used for text based access logs.
	// Defaults to empty to defer to Envoy's default log format.
	AccessLogFormatString string

	// AccessLogFormatterExtensions defines the Envoy extensions to enable for access log.
	AccessLogFormatterExtensions []string

	// AccessLogLevel defines the logging level for access log.
	AccessLogLevel contour_v1alpha1.AccessLogLevel

	// Timeouts holds Listener timeout settings.
	Timeouts contourconfig.Timeouts

	// AllowChunkedLength enables setting allow_chunked_length on the HTTP1 options for all
	// listeners.
	AllowChunkedLength bool

	// MergeSlashes toggles Envoy's non-standard merge_slashes path transformation option for all listeners.
	MergeSlashes bool

	// ServerHeaderTransformation defines the action to be applied to the Server header on the response path.
	ServerHeaderTransformation contour_v1alpha1.ServerHeaderTransformationType

	// XffNumTrustedHops sets the number of additional ingress proxy hops from the
	// right side of the x-forwarded-for HTTP header to trust.
	XffNumTrustedHops uint32

	// ConnectionBalancer
	// The validated value is 'exact'.
	// If no configuration is specified, Envoy will not attempt to balance active connections between worker threads
	// If specified, the listener will use the exact connection balancer.
	ConnectionBalancer string

	// MaxRequestsPerConnection defines the max number of requests per connection before which the connection is closed.
	// if not specified there is no limit set.
	MaxRequestsPerConnection *uint32

	HTTP2MaxConcurrentStreams *uint32

	// PerConnectionBufferLimitBytes defines the soft limit on size of the listener’s new connection read and write buffers
	// If unspecified, an implementation defined default is applied (1MiB).
	PerConnectionBufferLimitBytes *uint32

	// RateLimitConfig optionally configures the global Rate Limit Service to be
	// used.
	RateLimitConfig *RateLimitConfig

	// GlobalExternalAuthConfig optionally configures the global external authorization Service to be
	// used.
	GlobalExternalAuthConfig *GlobalExternalAuthConfig

	// TracingConfig optionally configures the tracing collector Service to be
	// used.
	TracingConfig *TracingConfig

	// SocketOptions configures socket options HTTP and HTTPS listeners.
	SocketOptions *contour_v1alpha1.SocketOptions

	// MaxConnectionsToAcceptPerSocketEvent defines how many new connections to accept per socket event loop iteration.
	MaxConnectionsToAcceptPerSocketEvent *uint32
}

type ExtensionServiceConfig struct {
	ExtensionService types.NamespacedName
	Timeout          timeout.Setting
	SNI              string
}

type TracingConfig struct {
	ExtensionServiceConfig

	ServiceName string

	OverallSampling float64

	MaxPathTagLength uint32

	CustomTags []*CustomTag
}

type CustomTag struct {
	// TagName is the unique name of the custom tag.
	TagName string

	// Literal is a static custom tag value.
	Literal string

	// EnvironmentName indicates that the label value is obtained
	// from the environment variable.
	EnvironmentName string

	// RequestHeaderName indicates which request header
	// the label value is obtained from.
	RequestHeaderName string
}

type RateLimitConfig struct {
	ExtensionServiceConfig
	Domain                      string
	FailOpen                    bool
	EnableXRateLimitHeaders     bool
	EnableResourceExhaustedCode bool
}

type GlobalExternalAuthConfig struct {
	ExtensionServiceConfig
	FailOpen                        bool
	Context                         map[string]string
	ServiceAPIType                  contour_v1.AuthorizationServiceAPIType
	HTTPAllowedAuthorizationHeaders []contour_v1.HTTPAuthorizationServerAllowedHeaders
	HTTPAllowedUpstreamHeaders      []contour_v1.HTTPAuthorizationServerAllowedHeaders
	HTTPPathPrefix                  string
	WithRequestBody                 *dag.AuthorizationServerBufferSettings
}

// httpAccessLog returns the access log for the HTTP (non TLS)
// listener or DEFAULT_HTTP_ACCESS_LOG if not configured.
func (lvc *ListenerConfig) httpAccessLog() string {
	if lvc.HTTPAccessLog != "" {
		return lvc.HTTPAccessLog
	}
	return DEFAULT_HTTP_ACCESS_LOG
}

// httpsAccessLog returns the access log for the HTTPS (TLS)
// listener or DEFAULT_HTTPS_ACCESS_LOG if not configured.
func (lvc *ListenerConfig) httpsAccessLog() string {
	if lvc.HTTPSAccessLog != "" {
		return lvc.HTTPSAccessLog
	}
	return DEFAULT_HTTPS_ACCESS_LOG
}

// accesslogType returns the access log type that should be configured
// across all listener types or DEFAULT_ACCESS_LOG_TYPE if not configured.
func (lvc *ListenerConfig) accesslogType() string {
	if lvc.AccessLogType != "" {
		return string(lvc.AccessLogType)
	}
	return string(config.DEFAULT_ACCESS_LOG_TYPE)
}

// accesslogFields returns the access log fields that should be configured
// for Envoy, or a default set if not configured.
func (lvc *ListenerConfig) accesslogFields() contour_v1alpha1.AccessLogJSONFields {
	if lvc.AccessLogJSONFields != nil {
		return lvc.AccessLogJSONFields
	}
	return contour_v1alpha1.DefaultAccessLogJSONFields
}

func (lvc *ListenerConfig) newInsecureAccessLog() []*envoy_config_accesslog_v3.AccessLog {
	switch lvc.accesslogType() {
	case string(config.JSONAccessLog):
		return envoy_v3.FileAccessLogJSON(lvc.httpAccessLog(), lvc.accesslogFields(), lvc.AccessLogFormatterExtensions, lvc.AccessLogLevel)
	default:
		return envoy_v3.FileAccessLogEnvoy(lvc.httpAccessLog(), lvc.AccessLogFormatString, lvc.AccessLogFormatterExtensions, lvc.AccessLogLevel)
	}
}

func (lvc *ListenerConfig) newSecureAccessLog() []*envoy_config_accesslog_v3.AccessLog {
	switch lvc.accesslogType() {
	case "json":
		return envoy_v3.FileAccessLogJSON(lvc.httpsAccessLog(), lvc.accesslogFields(), lvc.AccessLogFormatterExtensions, lvc.AccessLogLevel)
	default:
		return envoy_v3.FileAccessLogEnvoy(lvc.httpsAccessLog(), lvc.AccessLogFormatString, lvc.AccessLogFormatterExtensions, lvc.AccessLogLevel)
	}
}

// minTLSVersion returns the requested minimum TLS protocol
// version or envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2 if not configured.
func (lvc *ListenerConfig) minTLSVersion() envoy_transport_socket_tls_v3.TlsParameters_TlsProtocol {
	ver := envoy_v3.ParseTLSVersion(lvc.MinimumTLSVersion)
	if ver > envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2 {
		return ver
	}
	return envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2
}

// maxTLSVersion returns the requested maximum TLS protocol
// version or envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3 if not configured.
func (lvc *ListenerConfig) maxTLSVersion() envoy_transport_socket_tls_v3.TlsParameters_TlsProtocol {
	ver := envoy_v3.ParseTLSVersion(lvc.MaximumTLSVersion)
	if ver >= envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2 {
		return ver
	}
	return envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3
}

// ListenerCache manages the contents of the gRPC LDS cache.
type ListenerCache struct {
	mu           sync.Mutex
	values       map[string]*envoy_config_listener_v3.Listener
	staticValues map[string]*envoy_config_listener_v3.Listener

	Config ListenerConfig
	contour.Cond
}

// NewListenerCache returns an instance of a ListenerCache
func NewListenerCache(
	listenerConfig ListenerConfig,
	metricsConfig contour_v1alpha1.MetricsConfig,
	healthConfig contour_v1alpha1.HealthConfig,
	adminPort int,
) *ListenerCache {
	listenerCache := &ListenerCache{
		Config:       listenerConfig,
		staticValues: map[string]*envoy_config_listener_v3.Listener{},
	}

	for _, l := range envoy_v3.StatsListeners(metricsConfig, healthConfig) {
		listenerCache.staticValues[l.Name] = l
	}

	// If the port is not zero, allow the read-only options from the
	// Envoy admin webpage to be served.
	if adminPort > 0 {
		admin := envoy_v3.AdminListener(adminPort)
		listenerCache.staticValues[admin.Name] = admin
	}

	return listenerCache
}

// Update replaces the contents of the cache with the supplied map.
func (c *ListenerCache) Update(v map[string]*envoy_config_listener_v3.Listener) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.values = v
	c.Cond.Notify()
}

// Contents returns a copy of the cache's contents.
func (c *ListenerCache) Contents() []proto.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	var values []*envoy_config_listener_v3.Listener
	for _, v := range c.values {
		values = append(values, v)
	}
	for _, v := range c.staticValues {
		values = append(values, v)
	}
	sort.Stable(sorter.For(values))
	return protobuf.AsMessages(values)
}

// Query returns the proto.Messages in the ListenerCache that match
// a slice of strings
func (c *ListenerCache) Query(names []string) []proto.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	var values []*envoy_config_listener_v3.Listener
	for _, n := range names {
		v, ok := c.values[n]
		if !ok {
			v, ok = c.staticValues[n]
			if !ok {
				// if the listener is not registered in
				// dynamic or static values then skip it
				// as there is no way to return a blank
				// listener because the listener address
				// field is required.
				continue
			}
		}
		values = append(values, v)
	}
	sort.Stable(sorter.For(values))
	return protobuf.AsMessages(values)
}

func (*ListenerCache) TypeURL() string { return resource.ListenerType }

func (c *ListenerCache) OnChange(root *dag.DAG) {
	cfg := c.Config
	listeners := map[string]*envoy_config_listener_v3.Listener{}

	socketOptions := envoy_v3.NewSocketOptions().TCPKeepalive()
	if cfg.SocketOptions != nil {
		socketOptions = socketOptions.TOS(cfg.SocketOptions.TOS).TrafficClass(cfg.SocketOptions.TrafficClass)
	}

	for _, listener := range root.Listeners {
		// A Listener-level TCPProxy proxies all traffic for
		// the Listener port, i.e. no filter chain match.
		if listener.TCPProxy != nil {
			listeners[listener.Name] = envoy_v3.Listener(
				listener.Name,
				listener.Address,
				listener.Port,
				cfg.PerConnectionBufferLimitBytes,
				socketOptions,
				nil,
				envoy_v3.TCPProxy(listener.Name, listener.TCPProxy, cfg.newInsecureAccessLog()),
			)

			continue
		}
		// If there are non-TLS vhosts bound to the listener,
		// add a listener with a single filter chain.
		// Note: Ensure the filter chain order matches with the filter chain
		// order for the HTTPS virtualhosts.
		if len(listener.VirtualHosts) > 0 {
			cm := envoy_v3.HTTPConnectionManagerBuilder().
				Codec(envoy_v3.CodecForVersions(cfg.DefaultHTTPVersions...)).
				DefaultFilters().
				RouteConfigName(httpRouteConfigName(listener)).
				MetricsPrefix(listener.Name).
				AccessLoggers(cfg.newInsecureAccessLog()).
				RequestTimeout(cfg.Timeouts.Request).
				ConnectionIdleTimeout(cfg.Timeouts.ConnectionIdle).
				StreamIdleTimeout(cfg.Timeouts.StreamIdle).
				DelayedCloseTimeout(cfg.Timeouts.DelayedClose).
				MaxConnectionDuration(cfg.Timeouts.MaxConnectionDuration).
				ConnectionShutdownGracePeriod(cfg.Timeouts.ConnectionShutdownGracePeriod).
				AllowChunkedLength(cfg.AllowChunkedLength).
				MergeSlashes(cfg.MergeSlashes).
				ServerHeaderTransformation(cfg.ServerHeaderTransformation).
				NumTrustedHops(cfg.XffNumTrustedHops).
				MaxRequestsPerConnection(cfg.MaxRequestsPerConnection).
				HTTP2MaxConcurrentStreams(cfg.HTTP2MaxConcurrentStreams).
				AddFilter(httpGlobalExternalAuthConfig(cfg.GlobalExternalAuthConfig)).
				Tracing(envoy_v3.TracingConfig(envoyTracingConfig(cfg.TracingConfig))).
				AddFilter(envoy_v3.GlobalRateLimitFilter(envoyGlobalRateLimitConfig(cfg.RateLimitConfig))).
				EnableWebsockets(listener.EnableWebsockets).
				Get()

			listeners[listener.Name] = envoy_v3.Listener(
				listener.Name,
				listener.Address,
				listener.Port,
				cfg.PerConnectionBufferLimitBytes,
				socketOptions,
				proxyProtocol(cfg.UseProxyProto),
				cm,
			)
		}

		// If there are TLS vhosts, add a listener to which we
		// will attach a filter chain per vhost matching on SNI,
		// plus possibly one fallback cert filter chain.
		if len(listener.SecureVirtualHosts) > 0 {
			listeners[listener.Name] = envoy_v3.Listener(
				listener.Name,
				listener.Address,
				listener.Port,
				cfg.PerConnectionBufferLimitBytes,
				socketOptions,
				secureProxyProtocol(cfg.UseProxyProto),
			)
		}

		for _, vh := range listener.SecureVirtualHosts {
			var alpnProtos []string
			var filters []*envoy_config_listener_v3.Filter

			var forwardClientCertificate *dag.ClientCertificateDetails
			if vh.DownstreamValidation != nil {
				forwardClientCertificate = vh.DownstreamValidation.ForwardClientCertificate
			}

			if vh.TCPProxy == nil {
				var authzFilter *envoy_filter_network_http_connection_manager_v3.HttpFilter

				if vh.ExternalAuthorization != nil {
					authzFilter = envoy_v3.FilterExternalAuthz(vh.ExternalAuthorization)
				}

				// Create a uniquely named HTTP connection manager for
				// this vhost, so that the SNI name the client requests
				// only grants access to that host. See RFC 6066 for
				// security advice. Note that we still use the generic
				// metrics prefix to keep compatibility with previous
				// Contour versions since the metrics prefix will be
				// coded into monitoring dashboards.
				cm := envoy_v3.HTTPConnectionManagerBuilder().
					Codec(envoy_v3.CodecForVersions(cfg.DefaultHTTPVersions...)).
					AddFilter(envoy_v3.FilterMisdirectedRequests(vh.VirtualHost.Name)).
					DefaultFilters().
					AddFilter(envoy_v3.FilterJWTAuthN(vh.JWTProviders)).
					AddFilter(authzFilter).
					RouteConfigName(httpsRouteConfigName(listener, vh.VirtualHost.Name)).
					MetricsPrefix(listener.Name).
					AccessLoggers(cfg.newSecureAccessLog()).
					RequestTimeout(cfg.Timeouts.Request).
					ConnectionIdleTimeout(cfg.Timeouts.ConnectionIdle).
					StreamIdleTimeout(cfg.Timeouts.StreamIdle).
					DelayedCloseTimeout(cfg.Timeouts.DelayedClose).
					MaxConnectionDuration(cfg.Timeouts.MaxConnectionDuration).
					ConnectionShutdownGracePeriod(cfg.Timeouts.ConnectionShutdownGracePeriod).
					AllowChunkedLength(cfg.AllowChunkedLength).
					MergeSlashes(cfg.MergeSlashes).
					ServerHeaderTransformation(cfg.ServerHeaderTransformation).
					NumTrustedHops(cfg.XffNumTrustedHops).
					Tracing(envoy_v3.TracingConfig(envoyTracingConfig(cfg.TracingConfig))).
					AddFilter(envoy_v3.GlobalRateLimitFilter(envoyGlobalRateLimitConfig(cfg.RateLimitConfig))).
					ForwardClientCertificate(forwardClientCertificate).
					MaxRequestsPerConnection(cfg.MaxRequestsPerConnection).
					HTTP2MaxConcurrentStreams(cfg.HTTP2MaxConcurrentStreams).
					EnableWebsockets(listener.EnableWebsockets).
					Get()

				filters = envoy_v3.Filters(cm)

				if len(vh.HTTPVersions) != 0 {
					alpnProtos = vh.HTTPVersions
				} else {
					alpnProtos = envoy_v3.ProtoNamesForVersions(cfg.DefaultHTTPVersions...)
				}
			} else {
				filters = envoy_v3.Filters(envoy_v3.TCPProxy(listener.Name, vh.TCPProxy, cfg.newSecureAccessLog()))

				// Do not offer ALPN for TCP proxying, since
				// the protocols will be provided by the TCP
				// backend in its ServerHello.
			}

			var downstreamTLS *envoy_transport_socket_tls_v3.DownstreamTlsContext

			// Secret is provided when TLS is terminated and nil when TLS passthrough is used.
			if vh.Secret != nil {
				// Choose the higher of the configured or requested TLS version.
				minVer := max(cfg.minTLSVersion(), envoy_v3.ParseTLSVersion(vh.MinTLSVersion))

				// Choose the lower of the configured or requested TLS version.
				maxVer := min(cfg.maxTLSVersion(), envoy_v3.ParseTLSVersion(vh.MaxTLSVersion))
				if maxVer == envoy_transport_socket_tls_v3.TlsParameters_TLS_AUTO {
					maxVer = cfg.maxTLSVersion()
				}

				downstreamTLS = envoy_v3.DownstreamTLSContext(
					vh.Secret,
					minVer,
					maxVer,
					cfg.CipherSuites,
					vh.DownstreamValidation,
					alpnProtos...)
			}

			listeners[listener.Name].FilterChains = append(listeners[listener.Name].FilterChains, envoy_v3.FilterChainTLS(vh.VirtualHost.Name, downstreamTLS, filters))

			// If this VirtualHost has enabled the fallback certificate then set a default
			// FilterChain which will allow routes with this vhost to accept non-SNI TLS requests.
			// Note that we don't add the misdirected requests filter on this chain because at this
			// point we don't actually know the full set of server names that will be bound to the
			// filter chain through the ENVOY_FALLBACK_ROUTECONFIG route configuration.
			if vh.FallbackCertificate != nil && !envoy_v3.ContainsFallbackFilterChain(listeners[listener.Name].FilterChains) {
				// Construct the downstreamTLSContext passing the configured fallbackCertificate. The TLS min/max ProtocolVersion will use
				// the value defined in the Contour Configuration file if defined.
				downstreamTLS = envoy_v3.DownstreamTLSContext(
					vh.FallbackCertificate,
					cfg.minTLSVersion(),
					cfg.maxTLSVersion(),
					cfg.CipherSuites,
					vh.DownstreamValidation,
					alpnProtos...,
				)

				var authzFilter *envoy_filter_network_http_connection_manager_v3.HttpFilter
				if vh.ExternalAuthorization != nil {
					authzFilter = envoy_v3.FilterExternalAuthz(vh.ExternalAuthorization)
				}

				cm := envoy_v3.HTTPConnectionManagerBuilder().
					DefaultFilters().
					AddFilter(authzFilter).
					RouteConfigName(fallbackCertRouteConfigName(listener)).
					MetricsPrefix(listener.Name).
					AccessLoggers(cfg.newSecureAccessLog()).
					RequestTimeout(cfg.Timeouts.Request).
					ConnectionIdleTimeout(cfg.Timeouts.ConnectionIdle).
					StreamIdleTimeout(cfg.Timeouts.StreamIdle).
					DelayedCloseTimeout(cfg.Timeouts.DelayedClose).
					MaxConnectionDuration(cfg.Timeouts.MaxConnectionDuration).
					ConnectionShutdownGracePeriod(cfg.Timeouts.ConnectionShutdownGracePeriod).
					AllowChunkedLength(cfg.AllowChunkedLength).
					MergeSlashes(cfg.MergeSlashes).
					ServerHeaderTransformation(cfg.ServerHeaderTransformation).
					NumTrustedHops(cfg.XffNumTrustedHops).
					Tracing(envoy_v3.TracingConfig(envoyTracingConfig(cfg.TracingConfig))).
					AddFilter(envoy_v3.GlobalRateLimitFilter(envoyGlobalRateLimitConfig(cfg.RateLimitConfig))).
					ForwardClientCertificate(forwardClientCertificate).
					MaxRequestsPerConnection(cfg.MaxRequestsPerConnection).
					HTTP2MaxConcurrentStreams(cfg.HTTP2MaxConcurrentStreams).
					EnableWebsockets(listener.EnableWebsockets).
					Get()

				// Default filter chain
				filters = envoy_v3.Filters(cm)

				listeners[listener.Name].FilterChains = append(listeners[listener.Name].FilterChains, envoy_v3.FilterChainTLSFallback(downstreamTLS, filters))
			}
		}

		// Remove the https listener if there are no vhosts bound to it.
		if listener := listeners[listener.Name]; listener != nil && len(listener.FilterChains) == 0 {
			delete(listeners, listener.Name)
		} else {
			// there's some https listeners, we need to sort the filter chains
			// to ensure that the LDS entries are identical.
			sort.Stable(sorter.For(listener.FilterChains))
		}
	}

	// support more params of envoy listener

	// 1. connection balancer
	if cfg.ConnectionBalancer == "exact" {
		for _, listener := range listeners {
			listener.ConnectionBalanceConfig = &envoy_config_listener_v3.Listener_ConnectionBalanceConfig{
				BalanceType: &envoy_config_listener_v3.Listener_ConnectionBalanceConfig_ExactBalance_{
					ExactBalance: &envoy_config_listener_v3.Listener_ConnectionBalanceConfig_ExactBalance{},
				},
			}
		}
	}

	// 2. max_connections_to_accept_per_socket_event
	if cfg.MaxConnectionsToAcceptPerSocketEvent != nil {
		for _, listener := range listeners {
			listener.MaxConnectionsToAcceptPerSocketEvent = wrapperspb.UInt32(*cfg.MaxConnectionsToAcceptPerSocketEvent)
		}
	}

	c.Update(listeners)
}

func httpGlobalExternalAuthConfig(config *GlobalExternalAuthConfig) *envoy_filter_network_http_connection_manager_v3.HttpFilter {
	if config == nil {
		return nil
	}

	return envoy_v3.FilterExternalAuthz(&dag.ExternalAuthorization{
		AuthorizationService: &dag.ExtensionCluster{
			Name: dag.ExtensionClusterName(config.ExtensionServiceConfig.ExtensionService),
			SNI:  config.ExtensionServiceConfig.SNI,
		},
		ServiceAPIType:                     config.ServiceAPIType,
		HTTPAllowedAuthorizationHeaders:    config.HTTPAllowedAuthorizationHeaders,
		HTTPAllowedUpstreamHeaders:         config.HTTPAllowedUpstreamHeaders,
		HTTPPathPrefix:                     config.HTTPPathPrefix,
		AuthorizationFailOpen:              config.FailOpen,
		AuthorizationResponseTimeout:       config.ExtensionServiceConfig.Timeout,
		AuthorizationServerWithRequestBody: config.WithRequestBody,
	})
}

func envoyGlobalRateLimitConfig(config *RateLimitConfig) *envoy_v3.GlobalRateLimitConfig {
	if config == nil {
		return nil
	}

	return &envoy_v3.GlobalRateLimitConfig{
		ExtensionService:            config.ExtensionServiceConfig.ExtensionService,
		SNI:                         config.ExtensionServiceConfig.SNI,
		FailOpen:                    config.FailOpen,
		Timeout:                     config.ExtensionServiceConfig.Timeout,
		Domain:                      config.Domain,
		EnableXRateLimitHeaders:     config.EnableXRateLimitHeaders,
		EnableResourceExhaustedCode: config.EnableResourceExhaustedCode,
	}
}

func envoyTracingConfig(config *TracingConfig) *envoy_v3.EnvoyTracingConfig {
	if config == nil {
		return nil
	}

	return &envoy_v3.EnvoyTracingConfig{
		ExtensionService: config.ExtensionServiceConfig.ExtensionService,
		ServiceName:      config.ServiceName,
		SNI:              config.ExtensionServiceConfig.SNI,
		Timeout:          config.ExtensionServiceConfig.Timeout,
		OverallSampling:  config.OverallSampling,
		MaxPathTagLength: config.MaxPathTagLength,
		CustomTags:       envoyTracingConfigCustomTag(config.CustomTags),
	}
}

func envoyTracingConfigCustomTag(tags []*CustomTag) []*envoy_v3.CustomTag {
	if tags == nil {
		return nil
	}
	customTags := make([]*envoy_v3.CustomTag, len(tags))
	for i, tag := range tags {
		customTags[i] = &envoy_v3.CustomTag{
			TagName:           tag.TagName,
			Literal:           tag.Literal,
			EnvironmentName:   tag.EnvironmentName,
			RequestHeaderName: tag.RequestHeaderName,
		}
	}
	return customTags
}

func proxyProtocol(useProxy bool) []*envoy_config_listener_v3.ListenerFilter {
	if useProxy {
		return envoy_v3.ListenerFilters(
			envoy_v3.ProxyProtocol(),
		)
	}
	return nil
}

func secureProxyProtocol(useProxy bool) []*envoy_config_listener_v3.ListenerFilter {
	return append(proxyProtocol(useProxy), envoy_v3.TLSInspector())
}
