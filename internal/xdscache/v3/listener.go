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

	envoy_accesslog_v3 "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v3"
	envoy_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	http "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoy_tls_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	resource "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	contour_api_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/contour"
	"github.com/projectcontour/contour/internal/contourconfig"
	"github.com/projectcontour/contour/internal/dag"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/protobuf"
	"github.com/projectcontour/contour/internal/sorter"
	"github.com/projectcontour/contour/internal/timeout"
	"github.com/projectcontour/contour/pkg/config"
	"google.golang.org/protobuf/proto"
	"k8s.io/apimachinery/pkg/types"
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
	AccessLogType contour_api_v1alpha1.AccessLogType

	// AccessLogJSONFields sets the fields that should be shown in JSON logs.
	// Valid entries are the keys from internal/envoy/accesslog.go:jsonheaders
	// Defaults to a particular set of fields.
	AccessLogJSONFields contour_api_v1alpha1.AccessLogJSONFields

	// AccessLogFormatString sets the format string to be used for text based access logs.
	// Defaults to empty to defer to Envoy's default log format.
	AccessLogFormatString string

	// AccessLogFormatterExtensions defines the Envoy extensions to enable for access log.
	AccessLogFormatterExtensions []string

	// AccessLogLevel defines the logging level for access log.
	AccessLogLevel contour_api_v1alpha1.AccessLogLevel

	// Timeouts holds Listener timeout settings.
	Timeouts contourconfig.Timeouts

	// AllowChunkedLength enables setting allow_chunked_length on the HTTP1 options for all
	// listeners.
	AllowChunkedLength bool

	// MergeSlashes toggles Envoy's non-standard merge_slashes path transformation option for all listeners.
	MergeSlashes bool

	// ServerHeaderTransformation defines the action to be applied to the Server header on the response path.
	ServerHeaderTransformation contour_api_v1alpha1.ServerHeaderTransformationType

	// XffNumTrustedHops sets the number of additional ingress proxy hops from the
	// right side of the x-forwarded-for HTTP header to trust.
	XffNumTrustedHops uint32

	// ConnectionBalancer
	// The validated value is 'exact'.
	// If no configuration is specified, Envoy will not attempt to balance active connections between worker threads
	// If specified, the listener will use the exact connection balancer.
	ConnectionBalancer string
	// RateLimitConfig optionally configures the global Rate Limit Service to be
	// used.
	RateLimitConfig *RateLimitConfig

	// GlobalExternalAuthConfig optionally configures the global external authorization Service to be
	// used.
	GlobalExternalAuthConfig *GlobalExternalAuthConfig
}

type RateLimitConfig struct {
	ExtensionService            types.NamespacedName
	SNI                         string
	Domain                      string
	Timeout                     timeout.Setting
	FailOpen                    bool
	EnableXRateLimitHeaders     bool
	EnableResourceExhaustedCode bool
}

type GlobalExternalAuthConfig struct {
	ExtensionService types.NamespacedName
	FailOpen         bool
	SNI              string
	Timeout          timeout.Setting
	Context          map[string]string
	WithRequestBody  *dag.AuthorizationServerBufferSettings
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
func (lvc *ListenerConfig) accesslogFields() contour_api_v1alpha1.AccessLogJSONFields {
	if lvc.AccessLogJSONFields != nil {
		return lvc.AccessLogJSONFields
	}
	return contour_api_v1alpha1.DefaultAccessLogJSONFields
}

func (lvc *ListenerConfig) newInsecureAccessLog() []*envoy_accesslog_v3.AccessLog {
	switch lvc.accesslogType() {
	case string(config.JSONAccessLog):
		return envoy_v3.FileAccessLogJSON(lvc.httpAccessLog(), lvc.accesslogFields(), lvc.AccessLogFormatterExtensions, lvc.AccessLogLevel)
	default:
		return envoy_v3.FileAccessLogEnvoy(lvc.httpAccessLog(), lvc.AccessLogFormatString, lvc.AccessLogFormatterExtensions, lvc.AccessLogLevel)
	}
}

func (lvc *ListenerConfig) newSecureAccessLog() []*envoy_accesslog_v3.AccessLog {
	switch lvc.accesslogType() {
	case "json":
		return envoy_v3.FileAccessLogJSON(lvc.httpsAccessLog(), lvc.accesslogFields(), lvc.AccessLogFormatterExtensions, lvc.AccessLogLevel)
	default:
		return envoy_v3.FileAccessLogEnvoy(lvc.httpsAccessLog(), lvc.AccessLogFormatString, lvc.AccessLogFormatterExtensions, lvc.AccessLogLevel)
	}
}

// minTLSVersion returns the requested minimum TLS protocol
// version or envoy_tls_v3.TlsParameters_TLSv1_2 if not configured.
func (lvc *ListenerConfig) minTLSVersion() envoy_tls_v3.TlsParameters_TlsProtocol {
	minTLSVersion := envoy_v3.ParseTLSVersion(lvc.MinimumTLSVersion)
	if minTLSVersion > envoy_tls_v3.TlsParameters_TLSv1_2 {
		return minTLSVersion
	}
	return envoy_tls_v3.TlsParameters_TLSv1_2
}

// ListenerCache manages the contents of the gRPC LDS cache.
type ListenerCache struct {
	mu           sync.Mutex
	values       map[string]*envoy_listener_v3.Listener
	staticValues map[string]*envoy_listener_v3.Listener

	Config ListenerConfig
	contour.Cond
}

// NewListenerCache returns an instance of a ListenerCache
func NewListenerCache(
	listenerConfig ListenerConfig,
	metricsConfig contour_api_v1alpha1.MetricsConfig,
	healthConfig contour_api_v1alpha1.HealthConfig,
	adminPort int,
) *ListenerCache {
	listenerCache := &ListenerCache{
		Config:       listenerConfig,
		staticValues: map[string]*envoy_listener_v3.Listener{},
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
func (c *ListenerCache) Update(v map[string]*envoy_listener_v3.Listener) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.values = v
	c.Cond.Notify()
}

// Contents returns a copy of the cache's contents.
func (c *ListenerCache) Contents() []proto.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	var values []*envoy_listener_v3.Listener
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
	var values []*envoy_listener_v3.Listener
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
	listeners := map[string]*envoy_listener_v3.Listener{}

	max := func(a, b envoy_tls_v3.TlsParameters_TlsProtocol) envoy_tls_v3.TlsParameters_TlsProtocol {
		if a > b {
			return a
		}
		return b
	}

	for _, listener := range root.Listeners {
		// If there are non-TLS vhosts bound to the listener,
		// add a listener with a single filter chain.
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
				AddFilter(envoy_v3.GlobalRateLimitFilter(envoyGlobalRateLimitConfig(cfg.RateLimitConfig))).
				AddFilter(httpGlobalExternalAuthConfig(cfg.GlobalExternalAuthConfig)).
				Get()

			listeners[listener.Name] = envoy_v3.Listener(
				listener.Name,
				listener.Address,
				listener.Port,
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
				secureProxyProtocol(cfg.UseProxyProto),
			)
		}

		for _, vh := range listener.SecureVirtualHosts {
			var alpnProtos []string
			var filters []*envoy_listener_v3.Filter

			var forwardClientCertificate *dag.ClientCertificateDetails
			if vh.DownstreamValidation != nil {
				forwardClientCertificate = vh.DownstreamValidation.ForwardClientCertificate
			}

			if vh.TCPProxy == nil {
				var authFilter *http.HttpFilter

				if vh.ExternalAuthorization != nil {
					authFilter = envoy_v3.FilterExternalAuthz(vh.ExternalAuthorization)
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
					AddFilter(authFilter).
					AddFilter(envoy_v3.FilterJWTAuth(vh.JWTProviders)).
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
					AddFilter(envoy_v3.GlobalRateLimitFilter(envoyGlobalRateLimitConfig(cfg.RateLimitConfig))).
					ForwardClientCertificate(forwardClientCertificate).
					Get()

				filters = envoy_v3.Filters(cm)

				alpnProtos = envoy_v3.ProtoNamesForVersions(cfg.DefaultHTTPVersions...)
			} else {
				filters = envoy_v3.Filters(envoy_v3.TCPProxy(listener.Name, vh.TCPProxy, cfg.newSecureAccessLog()))

				// Do not offer ALPN for TCP proxying, since
				// the protocols will be provided by the TCP
				// backend in its ServerHello.
			}

			var downstreamTLS *envoy_tls_v3.DownstreamTlsContext

			// Secret is provided when TLS is terminated and nil when TLS passthrough is used.
			if vh.Secret != nil {
				// Choose the higher of the configured or requested TLS version.
				vers := max(cfg.minTLSVersion(), envoy_v3.ParseTLSVersion(vh.MinTLSVersion))

				downstreamTLS = envoy_v3.DownstreamTLSContext(
					vh.Secret,
					vers,
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
				// Construct the downstreamTLSContext passing the configured fallbackCertificate. The TLS minProtocolVersion will use
				// the value defined in the Contour Configuration file if defined.
				downstreamTLS = envoy_v3.DownstreamTLSContext(
					vh.FallbackCertificate,
					cfg.minTLSVersion(),
					cfg.CipherSuites,
					vh.DownstreamValidation,
					alpnProtos...,
				)

				cm := envoy_v3.HTTPConnectionManagerBuilder().
					DefaultFilters().
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
					AddFilter(envoy_v3.GlobalRateLimitFilter(envoyGlobalRateLimitConfig(cfg.RateLimitConfig))).
					ForwardClientCertificate(forwardClientCertificate).
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
			listener.ConnectionBalanceConfig = &envoy_listener_v3.Listener_ConnectionBalanceConfig{
				BalanceType: &envoy_listener_v3.Listener_ConnectionBalanceConfig_ExactBalance_{
					ExactBalance: &envoy_listener_v3.Listener_ConnectionBalanceConfig_ExactBalance{},
				},
			}
		}
	}

	c.Update(listeners)
}

func httpGlobalExternalAuthConfig(config *GlobalExternalAuthConfig) *http.HttpFilter {
	if config == nil {
		return nil
	}

	return envoy_v3.FilterExternalAuthz(&dag.ExternalAuthorization{
		AuthorizationService: &dag.ExtensionCluster{
			Name: dag.ExtensionClusterName(config.ExtensionService),
			SNI:  config.SNI,
		},
		AuthorizationFailOpen:              config.FailOpen,
		AuthorizationResponseTimeout:       config.Timeout,
		AuthorizationServerWithRequestBody: config.WithRequestBody,
	})

}

func envoyGlobalRateLimitConfig(config *RateLimitConfig) *envoy_v3.GlobalRateLimitConfig {
	if config == nil {
		return nil
	}

	return &envoy_v3.GlobalRateLimitConfig{
		ExtensionService:            config.ExtensionService,
		SNI:                         config.SNI,
		FailOpen:                    config.FailOpen,
		Timeout:                     config.Timeout,
		Domain:                      config.Domain,
		EnableXRateLimitHeaders:     config.EnableXRateLimitHeaders,
		EnableResourceExhaustedCode: config.EnableResourceExhaustedCode,
	}
}

func proxyProtocol(useProxy bool) []*envoy_listener_v3.ListenerFilter {
	if useProxy {
		return envoy_v3.ListenerFilters(
			envoy_v3.ProxyProtocol(),
		)
	}
	return nil
}

func secureProxyProtocol(useProxy bool) []*envoy_listener_v3.ListenerFilter {
	return append(proxyProtocol(useProxy), envoy_v3.TLSInspector())
}
