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
	"fmt"
	"log"
	"strings"
	"time"

	accesslog "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v3"
	envoy_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_lua_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/lua/v3"
	http "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoy_tls_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/projectcontour/contour/internal/envoy"
	"github.com/projectcontour/contour/internal/protobuf"
	"github.com/projectcontour/contour/internal/timeout"
)

type HTTPVersionType = http.HttpConnectionManager_CodecType

const (
	HTTPVersionAuto HTTPVersionType = http.HttpConnectionManager_AUTO
	HTTPVersion1    HTTPVersionType = http.HttpConnectionManager_HTTP1
	HTTPVersion2    HTTPVersionType = http.HttpConnectionManager_HTTP2
	HTTPVersion3    HTTPVersionType = http.HttpConnectionManager_HTTP3
)

// ProtoNamesForVersions returns the slice of ALPN protocol names for the give HTTP versions.
func ProtoNamesForVersions(versions ...HTTPVersionType) []string {
	protocols := map[HTTPVersionType]string{
		HTTPVersion1: "http/1.1",
		HTTPVersion2: "h2",
		HTTPVersion3: "",
	}
	defaultVersions := []string{"h2", "http/1.1"}
	wantedVersions := map[HTTPVersionType]struct{}{}

	if versions == nil {
		return defaultVersions
	}

	for _, v := range versions {
		wantedVersions[v] = struct{}{}
	}

	var alpn []string

	// Check for versions in preference order.
	for _, v := range []HTTPVersionType{HTTPVersionAuto, HTTPVersion2, HTTPVersion1} {
		if _, ok := wantedVersions[v]; ok {
			if v == HTTPVersionAuto {
				return defaultVersions
			}

			log.Printf("wanted %d -> %s", v, protocols[v])
			alpn = append(alpn, protocols[v])
		}
	}

	return alpn
}

// CodecForVersions determines a single Envoy HTTP codec constant
// that support all the given HTTP protocol versions.
func CodecForVersions(versions ...HTTPVersionType) HTTPVersionType {
	switch len(versions) {
	case 1:
		return versions[0]
	case 0:
		// Default is to autodetect.
		return HTTPVersionAuto
	default:
		// If more than one version is allowed, autodetect and let ALPN sort it out.
		return HTTPVersionAuto
	}
}

type httpConnectionManagerBuilder struct {
	routeConfigName               string
	metricsPrefix                 string
	accessLoggers                 []*accesslog.AccessLog
	requestTimeout                timeout.Setting
	connectionIdleTimeout         timeout.Setting
	streamIdleTimeout             timeout.Setting
	maxConnectionDuration         timeout.Setting
	connectionShutdownGracePeriod timeout.Setting
	filters                       []*http.HttpFilter
	codec                         HTTPVersionType // Note the zero value is AUTO, which is the default we want.
}

// RouteConfigName sets the name of the RDS element that contains
// the routing table for this manager.
func (b *httpConnectionManagerBuilder) RouteConfigName(name string) *httpConnectionManagerBuilder {
	b.routeConfigName = name
	return b
}

// MetricsPrefix sets the prefix used for emitting metrics from the
// connection manager. Note that this prefix is externally visible in
// monitoring tools, so it is subject to compatibility concerns.
//
// See https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_conn_man/stats#config-http-conn-man-stats
func (b *httpConnectionManagerBuilder) MetricsPrefix(prefix string) *httpConnectionManagerBuilder {
	b.metricsPrefix = prefix
	return b
}

// Codec sets the HTTP codec for the manager. The default is AUTO.
func (b *httpConnectionManagerBuilder) Codec(codecType HTTPVersionType) *httpConnectionManagerBuilder {
	b.codec = codecType
	return b
}

// AccessLoggers sets the access logging configuration.
func (b *httpConnectionManagerBuilder) AccessLoggers(loggers []*accesslog.AccessLog) *httpConnectionManagerBuilder {
	b.accessLoggers = loggers
	return b
}

// RequestTimeout sets the active request timeout on the connection manager.
func (b *httpConnectionManagerBuilder) RequestTimeout(timeout timeout.Setting) *httpConnectionManagerBuilder {
	b.requestTimeout = timeout
	return b
}

// ConnectionIdleTimeout sets the idle timeout on the connection manager.
func (b *httpConnectionManagerBuilder) ConnectionIdleTimeout(timeout timeout.Setting) *httpConnectionManagerBuilder {
	b.connectionIdleTimeout = timeout
	return b
}

// StreamIdleTimeout sets the stream idle timeout on the connection manager.
func (b *httpConnectionManagerBuilder) StreamIdleTimeout(timeout timeout.Setting) *httpConnectionManagerBuilder {
	b.streamIdleTimeout = timeout
	return b
}

// MaxConnectionDuration sets the max connection duration on the connection manager.
func (b *httpConnectionManagerBuilder) MaxConnectionDuration(timeout timeout.Setting) *httpConnectionManagerBuilder {
	b.maxConnectionDuration = timeout
	return b
}

// ConnectionShutdownGracePeriod sets the drain timeout on the connection manager.
func (b *httpConnectionManagerBuilder) ConnectionShutdownGracePeriod(timeout timeout.Setting) *httpConnectionManagerBuilder {
	b.connectionShutdownGracePeriod = timeout
	return b
}

func (b *httpConnectionManagerBuilder) DefaultFilters() *httpConnectionManagerBuilder {
	b.filters = append(b.filters,
		&http.HttpFilter{
			Name: wellknown.Gzip,
		},
		&http.HttpFilter{
			Name: wellknown.GRPCWeb,
		},
		&http.HttpFilter{
			Name: wellknown.CORS,
		},
		&http.HttpFilter{
			Name: wellknown.Router,
		},
	)

	return b
}

// AddFilter appends f to the list of filters for this HTTPConnectionManager. f
// may by nil, in which case it is ignored.
func (b *httpConnectionManagerBuilder) AddFilter(f *http.HttpFilter) *httpConnectionManagerBuilder {
	if f == nil {
		return b
	}

	if len(b.filters) > 0 {
		lastIndex := len(b.filters) - 1
		// If the router filter is last, keep it at the end
		// of the filter chain when we add the new filter.
		if b.filters[lastIndex].Name == wellknown.Router {
			b.filters = append(b.filters[:lastIndex], f, b.filters[lastIndex])
			return b
		}
	}

	b.filters = append(b.filters, f)

	return b
}

// Validate runs builtin validation rules against the current builder state.
func (b *httpConnectionManagerBuilder) Validate() error {
	filterNames := map[string]struct{}{}

	for _, f := range b.filters {
		filterNames[f.Name] = struct{}{}
	}

	// If there's no router filter, requests won't be forwarded.
	if _, ok := filterNames[wellknown.Router]; !ok {
		return fmt.Errorf("missing required filter %q", wellknown.Router)
	}

	return nil
}

// Get returns a new http.HttpConnectionManager filter, constructed
// from the builder settings.
//
// See https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/network/http_connection_manager/v3/http_connection_manager.proto.html
func (b *httpConnectionManagerBuilder) Get() *envoy_listener_v3.Filter {
	// For now, failing validation is a programmer error that
	// the caller can't reasonably recover from. A caller that can
	// handle this should validate manually.
	if err := b.Validate(); err != nil {
		panic(err.Error())
	}

	cm := &http.HttpConnectionManager{
		CodecType: b.codec,
		RouteSpecifier: &http.HttpConnectionManager_Rds{
			Rds: &http.Rds{
				RouteConfigName: b.routeConfigName,
				ConfigSource:    ConfigSource("contour"),
			},
		},
		HttpFilters: b.filters,
		CommonHttpProtocolOptions: &envoy_core_v3.HttpProtocolOptions{
			IdleTimeout: envoy.Timeout(b.connectionIdleTimeout),
		},
		HttpProtocolOptions: &envoy_core_v3.Http1ProtocolOptions{
			// Enable support for HTTP/1.0 requests that carry
			// a Host: header. See #537.
			AcceptHttp_10: true,
		},
		UseRemoteAddress: protobuf.Bool(true),
		NormalizePath:    protobuf.Bool(true),

		// issue #1487 pass through X-Request-Id if provided.
		PreserveExternalRequestId: true,
		MergeSlashes:              true,

		RequestTimeout:    envoy.Timeout(b.requestTimeout),
		StreamIdleTimeout: envoy.Timeout(b.streamIdleTimeout),
		DrainTimeout:      envoy.Timeout(b.connectionShutdownGracePeriod),
	}

	// Max connection duration is infinite/disabled by default in Envoy, so if the timeout setting
	// indicates to either disable or use default, don't pass a value at all. Note that unlike other
	// Envoy timeouts, explicitly passing a 0 here *would not* disable the timeout; it needs to be
	// omitted entirely.
	if !b.maxConnectionDuration.IsDisabled() && !b.maxConnectionDuration.UseDefault() {
		cm.CommonHttpProtocolOptions.MaxConnectionDuration = protobuf.Duration(b.maxConnectionDuration.Duration())
	}

	if len(b.accessLoggers) > 0 {
		cm.AccessLog = b.accessLoggers
	}

	// If there's no explicit metrics prefix, default it to the
	// route config name.
	if b.metricsPrefix != "" {
		cm.StatPrefix = b.metricsPrefix
	} else {
		cm.StatPrefix = b.routeConfigName
	}

	return &envoy_listener_v3.Filter{
		Name: wellknown.HTTPConnectionManager,
		ConfigType: &envoy_listener_v3.Filter_TypedConfig{
			TypedConfig: protobuf.MustMarshalAny(cm),
		},
	}
}

// HTTPConnectionManager creates a new HTTP Connection Manager filter
// for the supplied route, access log, and client request timeout.
func HTTPConnectionManager(routename string, accesslogger []*accesslog.AccessLog, requestTimeout time.Duration) *envoy_listener_v3.Filter {
	return HTTPConnectionManagerBuilder().
		RouteConfigName(routename).
		MetricsPrefix(routename).
		AccessLoggers(accesslogger).
		RequestTimeout(timeout.DurationSetting(requestTimeout)).
		DefaultFilters().
		Get()
}

func HTTPConnectionManagerBuilder() *httpConnectionManagerBuilder {
	return &httpConnectionManagerBuilder{}
}

// TLSInspector returns a new TLS inspector listener filter.
func TLSInspector() *envoy_listener_v3.ListenerFilter {
	return &envoy_listener_v3.ListenerFilter{
		Name: wellknown.TlsInspector,
	}
}

// Filters returns a []*envoy_listener_v3.Filter for the supplied filters.
func Filters(filters ...*envoy_listener_v3.Filter) []*envoy_listener_v3.Filter {
	if len(filters) == 0 {
		return nil
	}
	return filters
}

// SocketAddress creates a new TCP envoy_core_v3.Address.
func SocketAddress(address string, port int) *envoy_core_v3.Address {
	if address == "::" {
		return &envoy_core_v3.Address{
			Address: &envoy_core_v3.Address_SocketAddress{
				SocketAddress: &envoy_core_v3.SocketAddress{
					Protocol:   envoy_core_v3.SocketAddress_TCP,
					Address:    address,
					Ipv4Compat: true,
					PortSpecifier: &envoy_core_v3.SocketAddress_PortValue{
						PortValue: uint32(port),
					},
				},
			},
		}
	}
	return &envoy_core_v3.Address{
		Address: &envoy_core_v3.Address_SocketAddress{
			SocketAddress: &envoy_core_v3.SocketAddress{
				Protocol: envoy_core_v3.SocketAddress_TCP,
				Address:  address,
				PortSpecifier: &envoy_core_v3.SocketAddress_PortValue{
					PortValue: uint32(port),
				},
			},
		},
	}
}

func FilterMisdirectedRequests(fqdn string) *http.HttpFilter {
	// When Envoy matches on the virtual host domain, we configure
	// it to match any port specifier (see envoy.VirtualHost),
	// so the Host header (authority) may contain a port that
	// should be ignored. This means that if we don't have a match,
	// we should try again after stripping the port specifier.

	code := `
function envoy_on_request(request_handle)
	local headers = request_handle:headers()
	local host = string.lower(headers:get(":authority"))
	local target = "%s"

	if host ~= target then
		s, e = string.find(host, ":", 1, true)
		if s ~= nil then
			host = string.sub(host, 1, s - 1)
		end

		if host ~= target then
			request_handle:respond(
				{[":status"] = "421"},
				string.format("misdirected request to %%q", headers:get(":authority"))
			)
		end
	end
end
	`

	return &http.HttpFilter{
		Name: "envoy.filters.http.lua",
		ConfigType: &http.HttpFilter_TypedConfig{
			TypedConfig: protobuf.MustMarshalAny(&envoy_lua_v3.Lua{
				InlineCode: fmt.Sprintf(code, strings.ToLower(fqdn)),
			}),
		},
	}
}

// FilterChain retuns a *envoy_listener_v3.FilterChain for the supplied filters.
func FilterChain(filters ...*envoy_listener_v3.Filter) *envoy_listener_v3.FilterChain {
	return &envoy_listener_v3.FilterChain{
		Filters: filters,
	}
}

// FilterChains returns a []*envoy_listener_v3.FilterChain for the supplied filters.
func FilterChains(filters ...*envoy_listener_v3.Filter) []*envoy_listener_v3.FilterChain {
	if len(filters) == 0 {
		return nil
	}
	return []*envoy_listener_v3.FilterChain{
		FilterChain(filters...),
	}
}

// FilterChainTLS returns a TLS enabled envoy_listener_v3.FilterChain.
func FilterChainTLS(domain string, downstream *envoy_tls_v3.DownstreamTlsContext, filters []*envoy_listener_v3.Filter) *envoy_listener_v3.FilterChain {
	fc := &envoy_listener_v3.FilterChain{
		Filters: filters,
		FilterChainMatch: &envoy_listener_v3.FilterChainMatch{
			ServerNames: []string{domain},
		},
	}
	// Attach TLS data to this listener if provided.
	if downstream != nil {
		fc.TransportSocket = DownstreamTLSTransportSocket(downstream)

	}
	return fc
}

// FilterChainTLSFallback returns a TLS enabled envoy_listener_v3.FilterChain configured for FallbackCertificate.
func FilterChainTLSFallback(downstream *envoy_tls_v3.DownstreamTlsContext, filters []*envoy_listener_v3.Filter) *envoy_listener_v3.FilterChain {
	fc := &envoy_listener_v3.FilterChain{
		Name:    "fallback-certificate",
		Filters: filters,
		FilterChainMatch: &envoy_listener_v3.FilterChainMatch{
			TransportProtocol: "tls",
		},
	}
	// Attach TLS data to this listener if provided.
	if downstream != nil {
		fc.TransportSocket = DownstreamTLSTransportSocket(downstream)
	}
	return fc
}

// ListenerFilters returns a []*envoy_listener_v3.ListenerFilter for the supplied listener filters.
func ListenerFilters(filters ...*envoy_listener_v3.ListenerFilter) []*envoy_listener_v3.ListenerFilter {
	return filters
}
