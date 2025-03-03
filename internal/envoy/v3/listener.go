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
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	envoy_config_accesslog_v3 "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_compression_brotli_compressor_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/compression/brotli/compressor/v3"
	envoy_compression_gzip_compressor_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/compression/gzip/compressor/v3"
	envoy_compression_zstd_compressor_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/compression/zstd/compressor/v3"
	envoy_filter_http_compressor_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/compressor/v3"
	envoy_filter_http_cors_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/cors/v3"
	envoy_filter_http_ext_authz_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	envoy_filter_http_grpc_stats_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/grpc_stats/v3"
	envoy_filter_http_grpc_web_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/grpc_web/v3"
	envoy_filter_http_jwt_authn_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/jwt_authn/v3"
	envoy_filter_http_local_ratelimit_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/local_ratelimit/v3"
	envoy_filter_http_lua_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/lua/v3"
	envoy_filter_http_rbac_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/rbac/v3"
	envoy_filter_http_router_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/router/v3"
	envoy_filter_listener_proxy_protocol_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/listener/proxy_protocol/v3"
	envoy_filter_listener_tls_inspector_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/listener/tls_inspector/v3"
	envoy_filter_network_http_connection_manager_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoy_filter_network_tcp_proxy_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	envoy_transport_socket_tls_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoy_type_v3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/envoy"
	"github.com/projectcontour/contour/internal/protobuf"
	"github.com/projectcontour/contour/internal/sorter"
	"github.com/projectcontour/contour/internal/timeout"
)

type HTTPVersionType = envoy_filter_network_http_connection_manager_v3.HttpConnectionManager_CodecType

const (
	HTTPVersionAuto HTTPVersionType = envoy_filter_network_http_connection_manager_v3.HttpConnectionManager_AUTO
	HTTPVersion1    HTTPVersionType = envoy_filter_network_http_connection_manager_v3.HttpConnectionManager_HTTP1
	HTTPVersion2    HTTPVersionType = envoy_filter_network_http_connection_manager_v3.HttpConnectionManager_HTTP2
	HTTPVersion3    HTTPVersionType = envoy_filter_network_http_connection_manager_v3.HttpConnectionManager_HTTP3
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

// TLSInspector returns a new TLS inspector listener filter.
func TLSInspector() *envoy_config_listener_v3.ListenerFilter {
	return &envoy_config_listener_v3.ListenerFilter{
		Name: wellknown.TlsInspector,
		ConfigType: &envoy_config_listener_v3.ListenerFilter_TypedConfig{
			TypedConfig: protobuf.MustMarshalAny(&envoy_filter_listener_tls_inspector_v3.TlsInspector{}),
		},
	}
}

// ProxyProtocol returns a new Proxy Protocol listener filter.
func ProxyProtocol() *envoy_config_listener_v3.ListenerFilter {
	return &envoy_config_listener_v3.ListenerFilter{
		Name: wellknown.ProxyProtocol,
		ConfigType: &envoy_config_listener_v3.ListenerFilter_TypedConfig{
			TypedConfig: protobuf.MustMarshalAny(&envoy_filter_listener_proxy_protocol_v3.ProxyProtocol{}),
		},
	}
}

// Listener returns a new envoy_config_listener_v3.Listener for the supplied address, port, and filters.
func Listener(name, address string, port int, perConnectionBufferLimitBytes *uint32, so *SocketOptions, lf []*envoy_config_listener_v3.ListenerFilter, filters ...*envoy_config_listener_v3.Filter) *envoy_config_listener_v3.Listener {
	l := &envoy_config_listener_v3.Listener{
		Name:            name,
		Address:         SocketAddress(address, port),
		ListenerFilters: lf,
		SocketOptions:   so.Build(),
	}

	if perConnectionBufferLimitBytes != nil {
		l.PerConnectionBufferLimitBytes = wrapperspb.UInt32(*perConnectionBufferLimitBytes)
	}

	if len(filters) > 0 {
		l.FilterChains = append(
			l.FilterChains,
			&envoy_config_listener_v3.FilterChain{
				Filters: filters,
			},
		)
	}
	return l
}

const (
	CORSFilterName            string = "envoy.filters.http.cors"
	LocalRateLimitFilterName  string = "envoy.filters.http.local_ratelimit"
	GlobalRateLimitFilterName string = "envoy.filters.http.ratelimit"
	RBACFilterName            string = "envoy.filters.http.rbac"
	ExtAuthzFilterName        string = "envoy.filters.http.ext_authz"
	JWTAuthnFilterName        string = "envoy.filters.http.jwt_authn"
	LuaFilterName             string = "envoy.filters.http.lua"
	CompressorFilterName      string = "envoy.filters.http.compressor"
	GRPCWebFilterName         string = "envoy.filters.http.grpc_web"
	GRPCStatsFilterName       string = "envoy.filters.http.grpc_stats"
)

type httpConnectionManagerBuilder struct {
	routeConfigName               string
	routeConfigSource             *envoy_config_core_v3.ConfigSource
	metricsPrefix                 string
	accessLoggers                 []*envoy_config_accesslog_v3.AccessLog
	requestTimeout                timeout.Setting
	connectionIdleTimeout         timeout.Setting
	streamIdleTimeout             timeout.Setting
	delayedCloseTimeout           timeout.Setting
	maxConnectionDuration         timeout.Setting
	connectionShutdownGracePeriod timeout.Setting
	filters                       []*envoy_filter_network_http_connection_manager_v3.HttpFilter
	codec                         HTTPVersionType // Note the zero value is AUTO, which is the default we want.
	allowChunkedLength            bool
	mergeSlashes                  bool
	serverHeaderTransformation    envoy_filter_network_http_connection_manager_v3.HttpConnectionManager_ServerHeaderTransformation
	forwardClientCertificate      *dag.ClientCertificateDetails
	numTrustedHops                uint32
	stripTrailingHostDot          bool
	tracingConfig                 *envoy_filter_network_http_connection_manager_v3.HttpConnectionManager_Tracing
	maxRequestsPerConnection      *uint32
	http2MaxConcurrentStreams     *uint32
	enableWebsockets              bool
	compression                   *contour_v1alpha1.EnvoyCompression
}

func (b *httpConnectionManagerBuilder) EnableWebsockets(enable bool) *httpConnectionManagerBuilder {
	b.enableWebsockets = enable
	return b
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
func (b *httpConnectionManagerBuilder) AccessLoggers(loggers []*envoy_config_accesslog_v3.AccessLog) *httpConnectionManagerBuilder {
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

// DelayedCloseTimeout sets the delayed close timeout on the connection manager.
func (b *httpConnectionManagerBuilder) DelayedCloseTimeout(timeout timeout.Setting) *httpConnectionManagerBuilder {
	b.delayedCloseTimeout = timeout
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

func (b *httpConnectionManagerBuilder) AllowChunkedLength(enabled bool) *httpConnectionManagerBuilder {
	b.allowChunkedLength = enabled
	return b
}

// MergeSlashes toggles Envoy's non-standard merge_slashes path transformation option on the connection manager.
func (b *httpConnectionManagerBuilder) MergeSlashes(enabled bool) *httpConnectionManagerBuilder {
	b.mergeSlashes = enabled
	return b
}

// Compression configures the builder to set the compression method applied by DefaultFilters() to the
// given value `compressor`.
// When chaining builder method calls, this method must be called before DefaultFilters().
func (b *httpConnectionManagerBuilder) Compression(compressor *contour_v1alpha1.EnvoyCompression) *httpConnectionManagerBuilder {
	// Enforce that the function must be called in a specific order.
	if len(b.filters) > 0 {
		panic("Compression must be set before adding filters")
	}
	b.compression = compressor
	return b
}

func (b *httpConnectionManagerBuilder) ServerHeaderTransformation(value contour_v1alpha1.ServerHeaderTransformationType) *httpConnectionManagerBuilder {
	switch value {
	case contour_v1alpha1.OverwriteServerHeader:
		b.serverHeaderTransformation = envoy_filter_network_http_connection_manager_v3.HttpConnectionManager_OVERWRITE
	case contour_v1alpha1.AppendIfAbsentServerHeader:
		b.serverHeaderTransformation = envoy_filter_network_http_connection_manager_v3.HttpConnectionManager_APPEND_IF_ABSENT
	case contour_v1alpha1.PassThroughServerHeader:
		b.serverHeaderTransformation = envoy_filter_network_http_connection_manager_v3.HttpConnectionManager_PASS_THROUGH
	}
	return b
}

func (b *httpConnectionManagerBuilder) ForwardClientCertificate(details *dag.ClientCertificateDetails) *httpConnectionManagerBuilder {
	b.forwardClientCertificate = details
	return b
}

func (b *httpConnectionManagerBuilder) NumTrustedHops(num uint32) *httpConnectionManagerBuilder {
	b.numTrustedHops = num
	return b
}

func (b *httpConnectionManagerBuilder) StripTrailingHostDot(strip bool) *httpConnectionManagerBuilder {
	b.stripTrailingHostDot = strip
	return b
}

// MaxRequestsPerConnection sets max requests per connection for the downstream.
func (b *httpConnectionManagerBuilder) MaxRequestsPerConnection(maxRequestsPerConnection *uint32) *httpConnectionManagerBuilder {
	b.maxRequestsPerConnection = maxRequestsPerConnection
	return b
}

func (b *httpConnectionManagerBuilder) HTTP2MaxConcurrentStreams(http2MaxConcurrentStreams *uint32) *httpConnectionManagerBuilder {
	b.http2MaxConcurrentStreams = http2MaxConcurrentStreams
	return b
}

func (b *httpConnectionManagerBuilder) DefaultFilters() *httpConnectionManagerBuilder {
	// Add a default set of ordered http filters.
	// The names are not required to match anything and are
	// identified by the TypeURL of each filter.
	var compressor proto.Message = &envoy_compression_gzip_compressor_v3.Gzip{}
	compressorName := string(contour_v1alpha1.GzipCompression)
	if b.compression != nil {
		switch b.compression.Algorithm {
		case contour_v1alpha1.BrotliCompression:
			compressorName = "brotli"
			compressor = &envoy_compression_brotli_compressor_v3.Brotli{}
		case contour_v1alpha1.DisabledCompression:
			compressor = nil
		case contour_v1alpha1.ZstdCompression:
			compressorName = "zstd"
			compressor = &envoy_compression_zstd_compressor_v3.Zstd{}
		default:
			compressorName = "gzip"
			compressor = &envoy_compression_gzip_compressor_v3.Gzip{}
		}
	}

	if compressor != nil {
		// If compression is enabled add compressor filter
		b.filters = append(b.filters,
			&envoy_filter_network_http_connection_manager_v3.HttpFilter{
				Name: CompressorFilterName,
				ConfigType: &envoy_filter_network_http_connection_manager_v3.HttpFilter_TypedConfig{
					TypedConfig: protobuf.MustMarshalAny(&envoy_filter_http_compressor_v3.Compressor{
						CompressorLibrary: &envoy_config_core_v3.TypedExtensionConfig{
							Name: compressorName,
							TypedConfig: protobuf.MustMarshalAny(
								compressor,
							),
						},
						ResponseDirectionConfig: &envoy_filter_http_compressor_v3.Compressor_ResponseDirectionConfig{
							CommonConfig: &envoy_filter_http_compressor_v3.Compressor_CommonDirectionConfig{
								ContentType: []string{
									// Default content-types https://github.com/envoyproxy/envoy/blob/e74999dbdb12aa4d6b7a5d62d51731ea86bf72be/source/extensions/filters/http/compressor/compressor_filter.cc#L35-L38
									"text/html", "text/plain", "text/css", "application/javascript", "application/x-javascript",
									"text/javascript", "text/x-javascript", "text/ecmascript", "text/js", "text/jscript",
									"text/x-js", "application/ecmascript", "application/x-json", "application/xml",
									"application/json", "image/svg+xml", "text/xml", "application/xhtml+xml",
									// Additional content-types for grpc-web https://github.com/grpc/grpc/blob/master/doc/PROTOCOL-WEB.md#protocol-differences-vs-grpc-over-http2
									"application/grpc-web", "application/grpc-web+proto", "application/grpc-web+json", "application/grpc-web+thrift",
									"application/grpc-web-text", "application/grpc-web-text+proto", "application/grpc-web-text+thrift",
								},
							},
						},
					}),
				},
			})
	}
	b.filters = append(b.filters,
		&envoy_filter_network_http_connection_manager_v3.HttpFilter{
			Name: GRPCWebFilterName,
			ConfigType: &envoy_filter_network_http_connection_manager_v3.HttpFilter_TypedConfig{
				TypedConfig: protobuf.MustMarshalAny(&envoy_filter_http_grpc_web_v3.GrpcWeb{}),
			},
		},
		&envoy_filter_network_http_connection_manager_v3.HttpFilter{
			Name: GRPCStatsFilterName,
			ConfigType: &envoy_filter_network_http_connection_manager_v3.HttpFilter_TypedConfig{
				TypedConfig: protobuf.MustMarshalAny(
					&envoy_filter_http_grpc_stats_v3.FilterConfig{
						EmitFilterState:     true,
						EnableUpstreamStats: true,
					},
				),
			},
		},
		&envoy_filter_network_http_connection_manager_v3.HttpFilter{
			Name: CORSFilterName,
			ConfigType: &envoy_filter_network_http_connection_manager_v3.HttpFilter_TypedConfig{
				TypedConfig: protobuf.MustMarshalAny(&envoy_filter_http_cors_v3.Cors{}),
			},
		},
		&envoy_filter_network_http_connection_manager_v3.HttpFilter{
			Name: LocalRateLimitFilterName,
			ConfigType: &envoy_filter_network_http_connection_manager_v3.HttpFilter_TypedConfig{
				TypedConfig: protobuf.MustMarshalAny(
					&envoy_filter_http_local_ratelimit_v3.LocalRateLimit{
						StatPrefix: "http",
						// since no token bucket is defined here, the filter is disabled
						// globally but can be enabled on a per-vhost/route basis.
					},
				),
			},
		},
		&envoy_filter_network_http_connection_manager_v3.HttpFilter{
			Name: LuaFilterName,
			ConfigType: &envoy_filter_network_http_connection_manager_v3.HttpFilter_TypedConfig{
				TypedConfig: protobuf.MustMarshalAny(&envoy_filter_http_lua_v3.Lua{
					DefaultSourceCode: &envoy_config_core_v3.DataSource{
						Specifier: &envoy_config_core_v3.DataSource_InlineString{
							InlineString: "-- Placeholder for per-Route or per-Cluster overrides.",
						},
					},
				}),
			},
		},
		&envoy_filter_network_http_connection_manager_v3.HttpFilter{
			Name: RBACFilterName,
			ConfigType: &envoy_filter_network_http_connection_manager_v3.HttpFilter_TypedConfig{
				TypedConfig: protobuf.MustMarshalAny(&envoy_filter_http_rbac_v3.RBAC{}),
			},
		},
		&envoy_filter_network_http_connection_manager_v3.HttpFilter{
			Name: "router",
			ConfigType: &envoy_filter_network_http_connection_manager_v3.HttpFilter_TypedConfig{
				TypedConfig: protobuf.MustMarshalAny(&envoy_filter_http_router_v3.Router{}),
			},
		},
	)

	return b
}

// AddFilter appends f to the list of filters for this HTTPConnectionManager. f
// may be nil, in which case it is ignored. Note that Router filters
// (filters with TypeUrl `type.googleapis.com/envoy.extensions.filters.http.router.v3.Router`)
// are specially treated. There may only be one of these filters, and it must be the last.
// AddFilter will ensure that the router filter, if present, is last, and will panic
// if a second Router is added when one is already present.
func (b *httpConnectionManagerBuilder) AddFilter(f *envoy_filter_network_http_connection_manager_v3.HttpFilter) *httpConnectionManagerBuilder {
	if f == nil {
		return b
	}

	b.filters = append(b.filters, f)

	if len(b.filters) == 1 {
		return b
	}

	lastIndex := len(b.filters) - 1
	routerIndex := -1
	for i, filter := range b.filters {
		if filter.GetTypedConfig().MessageIs(&envoy_filter_http_router_v3.Router{}) {
			routerIndex = i
			break
		}
	}

	if routerIndex != -1 {
		// We can't add more than one router entry, and there should be no way to do it.
		// If this happens, it has to be programmer error, so we panic to tell them
		// it needs to be fixed. Note that in hitting this case, it doesn't matter we added
		// the second one earlier, because we're panicking anyway.
		if f.GetTypedConfig().MessageIs(&envoy_filter_http_router_v3.Router{}) && routerIndex != lastIndex {
			panic("Can't add more than one router to a filter chain")
		}
		if routerIndex != lastIndex {
			// Move the router to the end of the filters array.
			routerFilter := b.filters[routerIndex]
			b.filters = append(b.filters[:routerIndex], b.filters[routerIndex+1])
			b.filters = append(b.filters, routerFilter)
		}
	}

	return b
}

func (b *httpConnectionManagerBuilder) Tracing(tracing *envoy_filter_network_http_connection_manager_v3.HttpConnectionManager_Tracing) *httpConnectionManagerBuilder {
	if tracing == nil {
		return b
	}
	b.tracingConfig = tracing
	return b
}

// Validate runs builtin validation rules against the current builder state.
func (b *httpConnectionManagerBuilder) Validate() error {
	// It's not OK for the filters to be empty.
	if len(b.filters) == 0 {
		return errors.New("filter list is empty")
	}

	// If the router filter is not the last, the listener will be rejected by Envoy.
	// More specifically, the last filter must be a terminating filter. The only one
	// of these used by Contour is the router filter, which is set as the one
	// with typeUrl `type.googleapis.com/envoy.extensions.filters.http.router.v3.Router`,
	// which in this case is the one of type Router.
	lastIndex := len(b.filters) - 1
	if !b.filters[lastIndex].GetTypedConfig().MessageIs(&envoy_filter_http_router_v3.Router{}) {
		return errors.New("last filter is not a Router filter")
	}

	return nil
}

// Get returns a new envoy_filter_network_http_connection_manager_v3.HttpConnectionManager filter, constructed
// from the builder settings.
//
// See https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/network/http_connection_manager/v3/http_connection_manager.proto
func (b *httpConnectionManagerBuilder) Get() *envoy_config_listener_v3.Filter {
	// For now, failing validation is a programmer error that
	// the caller can't reasonably recover from. A caller that can
	// handle this should validate manually.
	if err := b.Validate(); err != nil {
		panic(err.Error())
	}

	cm := &envoy_filter_network_http_connection_manager_v3.HttpConnectionManager{
		CodecType: b.codec,
		RouteSpecifier: &envoy_filter_network_http_connection_manager_v3.HttpConnectionManager_Rds{
			Rds: &envoy_filter_network_http_connection_manager_v3.Rds{
				RouteConfigName: b.routeConfigName,
				ConfigSource:    b.routeConfigSource,
			},
		},
		Tracing:     b.tracingConfig,
		HttpFilters: b.filters,
		CommonHttpProtocolOptions: &envoy_config_core_v3.HttpProtocolOptions{
			IdleTimeout: envoy.Timeout(b.connectionIdleTimeout),
		},
		HttpProtocolOptions: &envoy_config_core_v3.Http1ProtocolOptions{
			// Enable support for HTTP/1.0 requests that carry
			// a Host: header. See #537.
			AcceptHttp_10:      true,
			AllowChunkedLength: b.allowChunkedLength,
		},

		UseRemoteAddress:     wrapperspb.Bool(true),
		XffNumTrustedHops:    b.numTrustedHops,
		StripTrailingHostDot: b.stripTrailingHostDot,

		NormalizePath: wrapperspb.Bool(true),

		// issue #1487 pass through X-Request-Id if provided.
		PreserveExternalRequestId:  true,
		MergeSlashes:               b.mergeSlashes,
		ServerHeaderTransformation: b.serverHeaderTransformation,

		RequestTimeout:      envoy.Timeout(b.requestTimeout),
		StreamIdleTimeout:   envoy.Timeout(b.streamIdleTimeout),
		DrainTimeout:        envoy.Timeout(b.connectionShutdownGracePeriod),
		DelayedCloseTimeout: envoy.Timeout(b.delayedCloseTimeout),
	}

	// Max connection duration is infinite/disabled by default in Envoy, so if the timeout setting
	// indicates to either disable or use default, don't pass a value at all. Note that unlike other
	// Envoy timeouts, explicitly passing a 0 here *would not* disable the timeout; it needs to be
	// omitted entirely.
	if !b.maxConnectionDuration.IsDisabled() && !b.maxConnectionDuration.UseDefault() {
		cm.CommonHttpProtocolOptions.MaxConnectionDuration = durationpb.New(b.maxConnectionDuration.Duration())
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
	if b.forwardClientCertificate != nil {
		cm.ForwardClientCertDetails = envoy_filter_network_http_connection_manager_v3.HttpConnectionManager_SANITIZE_SET
		cm.SetCurrentClientCertDetails = &envoy_filter_network_http_connection_manager_v3.HttpConnectionManager_SetCurrentClientCertDetails{
			Subject: wrapperspb.Bool(b.forwardClientCertificate.Subject),
			Cert:    b.forwardClientCertificate.Cert,
			Chain:   b.forwardClientCertificate.Chain,
			Dns:     b.forwardClientCertificate.DNS,
			Uri:     b.forwardClientCertificate.URI,
		}
	}

	// if maxConnectionsPerRequest is defined, set it.
	if b.maxRequestsPerConnection != nil {
		cm.CommonHttpProtocolOptions.MaxRequestsPerConnection = wrapperspb.UInt32(*b.maxRequestsPerConnection)
	}

	if b.http2MaxConcurrentStreams != nil {
		cm.Http2ProtocolOptions = &envoy_config_core_v3.Http2ProtocolOptions{
			MaxConcurrentStreams: wrapperspb.UInt32(*b.http2MaxConcurrentStreams),
		}
	}

	if b.enableWebsockets {
		cm.UpgradeConfigs = append(cm.UpgradeConfigs,
			&envoy_filter_network_http_connection_manager_v3.HttpConnectionManager_UpgradeConfig{
				UpgradeType: "websocket",
			},
		)
	}

	return &envoy_config_listener_v3.Filter{
		Name: wellknown.HTTPConnectionManager,
		ConfigType: &envoy_config_listener_v3.Filter_TypedConfig{
			TypedConfig: protobuf.MustMarshalAny(cm),
		},
	}
}

// HTTPConnectionManager creates a new HTTP Connection Manager filter
// for the supplied route, access log, and client request timeout.
func (e *EnvoyGen) HTTPConnectionManager(routename string, accesslogger []*envoy_config_accesslog_v3.AccessLog, requestTimeout time.Duration) *envoy_config_listener_v3.Filter {
	return e.HTTPConnectionManagerBuilder().
		RouteConfigName(routename).
		MetricsPrefix(routename).
		AccessLoggers(accesslogger).
		RequestTimeout(timeout.DurationSetting(requestTimeout)).
		DefaultFilters().
		Get()
}

// HTTPConnectionManagerBuilder creates a new HTTP connection manager builder.
// nolint:revive
func (e *EnvoyGen) HTTPConnectionManagerBuilder() *httpConnectionManagerBuilder {
	return &httpConnectionManagerBuilder{
		routeConfigSource: e.GetConfigSource(),
	}
}

// TCPProxy creates a new TCPProxy filter.
func TCPProxy(statPrefix string, proxy *dag.TCPProxy, accesslogger []*envoy_config_accesslog_v3.AccessLog) *envoy_config_listener_v3.Filter {
	// Set the idle timeout in seconds for connections through a TCP Proxy type filter.
	// The value of two and a half hours for reasons documented at
	// https://github.com/projectcontour/contour/issues/1074
	// Set to 9001 because now it's OVER NINE THOUSAND.
	tcpProxy := &envoy_filter_network_tcp_proxy_v3.TcpProxy{
		StatPrefix:  statPrefix,
		AccessLog:   accesslogger,
		IdleTimeout: durationpb.New(9001 * time.Second),
	}

	var totalWeight uint32
	var keepClusters []*dag.Cluster

	// Keep clusters with non-zero weights and drop
	// any others that have zero weights since they shouldn't
	// get traffic (note that TCPProxy weighted clusters can't
	// have zero weights, unlike HTTP route weighted clusters,
	// so we can't include them with a zero weight). Also note
	// that if all clusters have zero weights, then we keep them
	// all and evenly weight them below.
	for _, c := range proxy.Clusters {
		if c.Weight > 0 {
			keepClusters = append(keepClusters, c)
			totalWeight += c.Weight
		}
	}

	// If no clusters had non-zero weights, then revert to
	// keeping all of them.
	if totalWeight == 0 {
		keepClusters = proxy.Clusters
	}

	// Set either Cluster or WeightedClusters based on whether
	// there's one or more than one cluster to include.
	switch len(keepClusters) {
	case 1:
		tcpProxy.ClusterSpecifier = &envoy_filter_network_tcp_proxy_v3.TcpProxy_Cluster{
			Cluster: envoy.Clustername(keepClusters[0]),
		}
	default:
		var weightedClusters []*envoy_filter_network_tcp_proxy_v3.TcpProxy_WeightedCluster_ClusterWeight
		for _, c := range keepClusters {
			weight := c.Weight
			// if this cluster has a zero weight then it means
			// all clusters have zero weights, so evenly weight
			// them all by setting their weights to 1.
			if weight == 0 {
				weight = 1
			}

			weightedClusters = append(weightedClusters, &envoy_filter_network_tcp_proxy_v3.TcpProxy_WeightedCluster_ClusterWeight{
				Name:   envoy.Clustername(c),
				Weight: weight,
			})
		}

		sort.Stable(sorter.For(weightedClusters))
		tcpProxy.ClusterSpecifier = &envoy_filter_network_tcp_proxy_v3.TcpProxy_WeightedClusters{
			WeightedClusters: &envoy_filter_network_tcp_proxy_v3.TcpProxy_WeightedCluster{
				Clusters: weightedClusters,
			},
		}
	}

	return &envoy_config_listener_v3.Filter{
		Name: wellknown.TCPProxy,
		ConfigType: &envoy_config_listener_v3.Filter_TypedConfig{
			TypedConfig: protobuf.MustMarshalAny(tcpProxy),
		},
	}
}

// unixSocketAddress creates a new Unix Socket envoy_config_core_v3.Address.
func unixSocketAddress(address string) *envoy_config_core_v3.Address {
	return &envoy_config_core_v3.Address{
		Address: &envoy_config_core_v3.Address_Pipe{
			Pipe: &envoy_config_core_v3.Pipe{
				Path: address,
				Mode: 0o644,
			},
		},
	}
}

// SocketAddress creates a new TCP envoy_config_core_v3.Address.
func SocketAddress(address string, port int) *envoy_config_core_v3.Address {
	portValue := uint32(port) //nolint:gosec // disable G115
	if address == "::" {
		return &envoy_config_core_v3.Address{
			Address: &envoy_config_core_v3.Address_SocketAddress{
				SocketAddress: &envoy_config_core_v3.SocketAddress{
					Protocol:   envoy_config_core_v3.SocketAddress_TCP,
					Address:    address,
					Ipv4Compat: true,
					PortSpecifier: &envoy_config_core_v3.SocketAddress_PortValue{
						PortValue: portValue,
					},
				},
			},
		}
	}
	return &envoy_config_core_v3.Address{
		Address: &envoy_config_core_v3.Address_SocketAddress{
			SocketAddress: &envoy_config_core_v3.SocketAddress{
				Protocol: envoy_config_core_v3.SocketAddress_TCP,
				Address:  address,
				PortSpecifier: &envoy_config_core_v3.SocketAddress_PortValue{
					PortValue: portValue,
				},
			},
		},
	}
}

// Filters returns a []*envoy_config_listener_v3.Filter for the supplied filters.
func Filters(filters ...*envoy_config_listener_v3.Filter) []*envoy_config_listener_v3.Filter {
	if len(filters) == 0 {
		return nil
	}
	return filters
}

// FilterChain returns a *envoy_config_listener_v3.FilterChain for the supplied filters.
func FilterChain(filters ...*envoy_config_listener_v3.Filter) *envoy_config_listener_v3.FilterChain {
	return &envoy_config_listener_v3.FilterChain{
		Filters: filters,
	}
}

// FilterChains returns a []*envoy_config_listener_v3.FilterChain for the supplied filters.
func FilterChains(filters ...*envoy_config_listener_v3.Filter) []*envoy_config_listener_v3.FilterChain {
	if len(filters) == 0 {
		return nil
	}
	return []*envoy_config_listener_v3.FilterChain{
		FilterChain(filters...),
	}
}

func FilterMisdirectedRequests(fqdn string) *envoy_filter_network_http_connection_manager_v3.HttpFilter {
	var target string

	// fqdn can be "*" to match all hostnames or a wildcard prefix
	// e.g. "*.foo"
	if strings.HasPrefix(fqdn, "*") {
		// When we have a wildcard hostname, we will have already matched
		// the filter chain on an SNI that falls under the wildcard so we
		// retrieve that and make sure the :authority header matches.
		// See: https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/lua_filter#requestedservername
		target = "request_handle:streamInfo():requestedServerName()"
	} else {
		// For specific hostnames we know the SNI we need to match the
		// :authority header against so we can simplify the code.
		target = `"` + strings.ToLower(fqdn) + `"`
	}

	code := `
function envoy_on_request(request_handle)
	local headers = request_handle:headers()
	local host = string.lower(headers:get(":authority"))
	local target = %s

	s, e = string.find(host, ":", 1, true)
	if s ~= nil then
		host = string.sub(host, 1, s - 1)
	end

	if host ~= target then
		request_handle:respond(
			{[":status"] = "421"},
			string.format("misdirected request to %%q", host)
		)
	end
end
	`

	return &envoy_filter_network_http_connection_manager_v3.HttpFilter{
		Name: LuaFilterName,
		ConfigType: &envoy_filter_network_http_connection_manager_v3.HttpFilter_TypedConfig{
			TypedConfig: protobuf.MustMarshalAny(&envoy_filter_http_lua_v3.Lua{
				DefaultSourceCode: &envoy_config_core_v3.DataSource{
					Specifier: &envoy_config_core_v3.DataSource_InlineString{
						InlineString: fmt.Sprintf(code, target),
					},
				},
			}),
		},
	}
}

// FilterExternalAuthz returns an `ext_authz` filter configured with the
// requested parameters.
func FilterExternalAuthz(externalAuthorization *dag.ExternalAuthorization) *envoy_filter_network_http_connection_manager_v3.HttpFilter {
	authConfig := envoy_filter_http_ext_authz_v3.ExtAuthz{
		Services: &envoy_filter_http_ext_authz_v3.ExtAuthz_GrpcService{
			GrpcService: grpcService(externalAuthorization.AuthorizationService.Name, externalAuthorization.AuthorizationService.SNI, externalAuthorization.AuthorizationResponseTimeout),
		},
		// Pretty sure we always want this. Why have an
		// external auth service if it is not going to affect
		// routing decisions?
		ClearRouteCache:  true,
		FailureModeAllow: externalAuthorization.AuthorizationFailOpen,
		StatusOnError: &envoy_type_v3.HttpStatus{
			Code: envoy_type_v3.StatusCode_Forbidden,
		},
		MetadataContextNamespaces: []string{},
		IncludePeerCertificate:    true,
		// TODO(jpeach): When we move to the Envoy v4 API, propagate the
		// `transport_api_version` from ExtensionServiceSpec ProtocolVersion.
		TransportApiVersion: envoy_config_core_v3.ApiVersion_V3,
	}

	if externalAuthorization.AuthorizationServerWithRequestBody != nil {
		authConfig.WithRequestBody = &envoy_filter_http_ext_authz_v3.BufferSettings{
			MaxRequestBytes:     externalAuthorization.AuthorizationServerWithRequestBody.MaxRequestBytes,
			AllowPartialMessage: externalAuthorization.AuthorizationServerWithRequestBody.AllowPartialMessage,
			PackAsBytes:         externalAuthorization.AuthorizationServerWithRequestBody.PackAsBytes,
		}
	}

	return &envoy_filter_network_http_connection_manager_v3.HttpFilter{
		Name: ExtAuthzFilterName,
		ConfigType: &envoy_filter_network_http_connection_manager_v3.HttpFilter_TypedConfig{
			TypedConfig: protobuf.MustMarshalAny(&authConfig),
		},
	}
}

// FilterJWTAuthN returns a `jwt_authn` filter configured with the
// requested parameters.
func FilterJWTAuthN(jwtProviders []dag.JWTProvider) *envoy_filter_network_http_connection_manager_v3.HttpFilter {
	if len(jwtProviders) == 0 {
		return nil
	}

	jwtConfig := envoy_filter_http_jwt_authn_v3.JwtAuthentication{
		Providers:      map[string]*envoy_filter_http_jwt_authn_v3.JwtProvider{},
		RequirementMap: map[string]*envoy_filter_http_jwt_authn_v3.JwtRequirement{},
	}

	for _, provider := range jwtProviders {
		provider := provider
		var cacheDuration *durationpb.Duration
		if provider.RemoteJWKS.CacheDuration != nil {
			cacheDuration = durationpb.New(*provider.RemoteJWKS.CacheDuration)
		}

		jwtConfig.Providers[provider.Name] = &envoy_filter_http_jwt_authn_v3.JwtProvider{
			Issuer:    provider.Issuer,
			Audiences: provider.Audiences,
			JwksSourceSpecifier: &envoy_filter_http_jwt_authn_v3.JwtProvider_RemoteJwks{
				RemoteJwks: &envoy_filter_http_jwt_authn_v3.RemoteJwks{
					HttpUri: &envoy_config_core_v3.HttpUri{
						Uri: provider.RemoteJWKS.URI,
						HttpUpstreamType: &envoy_config_core_v3.HttpUri_Cluster{
							Cluster: envoy.DNSNameClusterName(&provider.RemoteJWKS.Cluster),
						},
						Timeout: durationpb.New(provider.RemoteJWKS.Timeout),
					},
					CacheDuration: cacheDuration,
				},
			},
			Forward: provider.ForwardJWT,
		}

		// Set up a requirement map so that per-route filter config can refer
		// to a requirement by name. This is nicer than specifying rules here,
		// because it likely results in less Envoy config overall (don't have
		// to duplicate every route match in the jwt_authn config), and it means
		// we don't have to implement another sorter to sort JWT rules -- the
		// sorting already being done to routes covers it.
		jwtConfig.RequirementMap[provider.Name] = &envoy_filter_http_jwt_authn_v3.JwtRequirement{
			RequiresType: &envoy_filter_http_jwt_authn_v3.JwtRequirement_ProviderName{
				ProviderName: provider.Name,
			},
		}
	}

	return &envoy_filter_network_http_connection_manager_v3.HttpFilter{
		Name: JWTAuthnFilterName,
		ConfigType: &envoy_filter_network_http_connection_manager_v3.HttpFilter_TypedConfig{
			TypedConfig: protobuf.MustMarshalAny(&jwtConfig),
		},
	}
}

// FilterChainTLS returns a TLS enabled envoy_config_listener_v3.FilterChain.
func FilterChainTLS(domain string, downstream *envoy_transport_socket_tls_v3.DownstreamTlsContext, filters []*envoy_config_listener_v3.Filter) *envoy_config_listener_v3.FilterChain {
	fc := &envoy_config_listener_v3.FilterChain{
		Filters: filters,
	}

	// If the domain doesn't have a specific SNI, Envoy can't filter
	// on that, so change the Match to be on TransportProtocol which would
	// match any request over TLS to this listener.
	if domain == "*" {
		fc.FilterChainMatch = &envoy_config_listener_v3.FilterChainMatch{
			TransportProtocol: "tls",
		}
	} else {
		fc.FilterChainMatch = &envoy_config_listener_v3.FilterChainMatch{
			ServerNames: []string{domain},
		}
	}

	// Attach TLS data to this listener if provided.
	if downstream != nil {
		fc.TransportSocket = DownstreamTLSTransportSocket(downstream)
	}
	return fc
}

// FilterChainTLSFallback returns a TLS enabled envoy_config_listener_v3.FilterChain configured for FallbackCertificate.
func FilterChainTLSFallback(downstream *envoy_transport_socket_tls_v3.DownstreamTlsContext, filters []*envoy_config_listener_v3.Filter) *envoy_config_listener_v3.FilterChain {
	fc := &envoy_config_listener_v3.FilterChain{
		Name:    "fallback-certificate",
		Filters: filters,
		FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
			TransportProtocol: "tls",
		},
	}
	// Attach TLS data to this listener if provided.
	if downstream != nil {
		fc.TransportSocket = DownstreamTLSTransportSocket(downstream)
	}
	return fc
}

// grpcService returns a envoy_config_core_v3.GrpcService for the given parameters.
func grpcService(clusterName, sni string, timeout timeout.Setting) *envoy_config_core_v3.GrpcService {
	authority := strings.ReplaceAll(clusterName, "/", ".")
	if sni != "" {
		authority = sni
	}
	return &envoy_config_core_v3.GrpcService{
		TargetSpecifier: &envoy_config_core_v3.GrpcService_EnvoyGrpc_{
			EnvoyGrpc: &envoy_config_core_v3.GrpcService_EnvoyGrpc{
				ClusterName: clusterName,
				Authority:   authority,
			},
		},
		Timeout: envoy.Timeout(timeout),
	}
}

// ListenerFilters returns a []*envoy_config_listener_v3.ListenerFilter for the supplied listener filters.
func ListenerFilters(filters ...*envoy_config_listener_v3.ListenerFilter) []*envoy_config_listener_v3.ListenerFilter {
	return filters
}

func ContainsFallbackFilterChain(filterchains []*envoy_config_listener_v3.FilterChain) bool {
	for _, fc := range filterchains {
		if fc.Name == "fallback-certificate" {
			return true
		}
	}
	return false
}
