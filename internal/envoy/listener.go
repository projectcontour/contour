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

package envoy

import (
	"fmt"
	"log"
	"sort"
	"time"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_api_v2_auth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	envoy_api_v2_core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoy_api_v2_listener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	accesslog "github.com/envoyproxy/go-control-plane/envoy/config/filter/accesslog/v2"
	lua "github.com/envoyproxy/go-control-plane/envoy/config/filter/http/lua/v2"
	http "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	tcp "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/tcp_proxy/v2"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/protobuf"
	"github.com/projectcontour/contour/internal/sorter"
)

type HTTPVersionType = http.HttpConnectionManager_CodecType

const (
	HTTPVersionAuto HTTPVersionType = http.HttpConnectionManager_AUTO
	HTTPVersion1    HTTPVersionType = http.HttpConnectionManager_HTTP1
	HTTPVersion2    HTTPVersionType = http.HttpConnectionManager_HTTP2
	HTTPVersion3    HTTPVersionType = http.HttpConnectionManager_HTTP3
)

// TLSInspector returns a new TLS inspector listener filter.
func TLSInspector() *envoy_api_v2_listener.ListenerFilter {
	return &envoy_api_v2_listener.ListenerFilter{
		Name: wellknown.TlsInspector,
	}
}

// ProxyProtocol returns a new Proxy Protocol listener filter.
func ProxyProtocol() *envoy_api_v2_listener.ListenerFilter {
	return &envoy_api_v2_listener.ListenerFilter{
		Name: wellknown.ProxyProtocol,
	}
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

// Listener returns a new v2.Listener for the supplied address, port, and filters.
func Listener(name, address string, port int, lf []*envoy_api_v2_listener.ListenerFilter, filters ...*envoy_api_v2_listener.Filter) *v2.Listener {
	l := &v2.Listener{
		Name:            name,
		Address:         SocketAddress(address, port),
		ListenerFilters: lf,
		SocketOptions:   TCPKeepaliveSocketOptions(),
	}
	if len(filters) > 0 {
		l.FilterChains = append(
			l.FilterChains,
			&envoy_api_v2_listener.FilterChain{
				Filters: filters,
			},
		)
	}
	return l
}

type httpConnectionManagerBuilder struct {
	routeConfigName       string
	metricsPrefix         string
	accessLoggers         []*accesslog.AccessLog
	requestTimeout        time.Duration
	connectionIdleTimeout time.Duration
	streamIdleTimeout     time.Duration
	maxConnectionDuration time.Duration
	drainTimeout          time.Duration
	filters               []*http.HttpFilter
	codec                 HTTPVersionType // Note the zero value is AUTO, which is the default we want.
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

// RequestTimeout sets the active request timeout on the connection
// manager. If not specified or set to 0, this timeout is disabled.
func (b *httpConnectionManagerBuilder) RequestTimeout(timeout time.Duration) *httpConnectionManagerBuilder {
	b.requestTimeout = timeout
	return b
}

// ConnectionIdleTimeout sets the idle timeout on the connection
// manager. If not specified or set to 0, this timeout is disabled.
func (b *httpConnectionManagerBuilder) ConnectionIdleTimeout(timeout time.Duration) *httpConnectionManagerBuilder {
	b.connectionIdleTimeout = timeout
	return b
}

// StreamIdleTimeout sets the stream idle timeout on the connection
// manager. If not specified or set to 0, this timeout is disabled.
func (b *httpConnectionManagerBuilder) StreamIdleTimeout(timeout time.Duration) *httpConnectionManagerBuilder {
	b.streamIdleTimeout = timeout
	return b
}

// MaxConnectionDuration sets the max connection duration on the connection
// manager. If not specified or set to 0, this timeout is disabled.
func (b *httpConnectionManagerBuilder) MaxConnectionDuration(timeout time.Duration) *httpConnectionManagerBuilder {
	b.maxConnectionDuration = timeout
	return b
}

// DrainTimeout sets the drain timeout on the connection
// manager. If not specified or set to 0, this timeout is disabled.
func (b *httpConnectionManagerBuilder) DrainTimeout(timeout time.Duration) *httpConnectionManagerBuilder {
	b.drainTimeout = timeout
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
			Name: wellknown.Router,
		},
	)

	return b
}

func (b *httpConnectionManagerBuilder) AddFilter(f *http.HttpFilter) *httpConnectionManagerBuilder {
	b.filters = append(b.filters, f)
	return b
}

// Get returns a new http.HttpConnectionManager filter, constructed
// from the builder settings.
//
// See https://www.envoyproxy.io/docs/envoy/latest/api-v2/config/filter/network/http_connection_manager/v2/http_connection_manager.proto.html
func (b *httpConnectionManagerBuilder) Get() *envoy_api_v2_listener.Filter {
	cm := &http.HttpConnectionManager{
		CodecType: b.codec,
		RouteSpecifier: &http.HttpConnectionManager_Rds{
			Rds: &http.Rds{
				RouteConfigName: b.routeConfigName,
				ConfigSource:    ConfigSource("contour"),
			},
		},
		HttpFilters: b.filters,
		CommonHttpProtocolOptions: &envoy_api_v2_core.HttpProtocolOptions{
			IdleTimeout: protobuf.Duration(b.connectionIdleTimeout),
		},
		HttpProtocolOptions: &envoy_api_v2_core.Http1ProtocolOptions{
			// Enable support for HTTP/1.0 requests that carry
			// a Host: header. See #537.
			AcceptHttp_10: true,
		},
		UseRemoteAddress: protobuf.Bool(true),
		NormalizePath:    protobuf.Bool(true),
		RequestTimeout:   protobuf.Duration(b.requestTimeout),

		// issue #1487 pass through X-Request-Id if provided.
		PreserveExternalRequestId: true,
		MergeSlashes:              true,

		StreamIdleTimeout: protobuf.Duration(b.streamIdleTimeout),
		DrainTimeout:      protobuf.Duration(b.drainTimeout),
	}

	// This timeout is disabled in Envoy by NOT providing a value, rather than explicitly passing a 0.
	// From the contour perspective, a 0 value still disables the timeout, in order to be consistent
	// with other timeouts.
	if b.maxConnectionDuration > 0 {
		cm.CommonHttpProtocolOptions.MaxConnectionDuration = protobuf.Duration(b.maxConnectionDuration)
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

	return &envoy_api_v2_listener.Filter{
		Name: wellknown.HTTPConnectionManager,
		ConfigType: &envoy_api_v2_listener.Filter_TypedConfig{
			TypedConfig: protobuf.MustMarshalAny(cm),
		},
	}
}

// HTTPConnectionManager creates a new HTTP Connection Manager filter
// for the supplied route, access log, and client request timeout.
func HTTPConnectionManager(routename string, accesslogger []*accesslog.AccessLog, requestTimeout time.Duration) *envoy_api_v2_listener.Filter {
	return HTTPConnectionManagerBuilder().
		RouteConfigName(routename).
		MetricsPrefix(routename).
		AccessLoggers(accesslogger).
		RequestTimeout(requestTimeout).
		DefaultFilters().
		Get()
}

func HTTPConnectionManagerBuilder() *httpConnectionManagerBuilder {
	return &httpConnectionManagerBuilder{}
}

// TCPProxy creates a new TCPProxy filter.
func TCPProxy(statPrefix string, proxy *dag.TCPProxy, accesslogger []*accesslog.AccessLog) *envoy_api_v2_listener.Filter {
	// Set the idle timeout in seconds for connections through a TCP Proxy type filter.
	// The value of two and a half hours for reasons documented at
	// https://github.com/projectcontour/contour/issues/1074
	// Set to 9001 because now it's OVER NINE THOUSAND.
	idleTimeout := protobuf.Duration(9001 * time.Second)

	switch len(proxy.Clusters) {
	case 1:
		return &envoy_api_v2_listener.Filter{
			Name: wellknown.TCPProxy,
			ConfigType: &envoy_api_v2_listener.Filter_TypedConfig{
				TypedConfig: protobuf.MustMarshalAny(&tcp.TcpProxy{
					StatPrefix: statPrefix,
					ClusterSpecifier: &tcp.TcpProxy_Cluster{
						Cluster: Clustername(proxy.Clusters[0]),
					},
					AccessLog:   accesslogger,
					IdleTimeout: idleTimeout,
				}),
			},
		}
	default:
		var clusters []*tcp.TcpProxy_WeightedCluster_ClusterWeight
		for _, c := range proxy.Clusters {
			weight := c.Weight
			if weight == 0 {
				weight = 1
			}
			clusters = append(clusters, &tcp.TcpProxy_WeightedCluster_ClusterWeight{
				Name:   Clustername(c),
				Weight: weight,
			})
		}
		sort.Stable(sorter.For(clusters))
		return &envoy_api_v2_listener.Filter{
			Name: wellknown.TCPProxy,
			ConfigType: &envoy_api_v2_listener.Filter_TypedConfig{
				TypedConfig: protobuf.MustMarshalAny(&tcp.TcpProxy{
					StatPrefix: statPrefix,
					ClusterSpecifier: &tcp.TcpProxy_WeightedClusters{
						WeightedClusters: &tcp.TcpProxy_WeightedCluster{
							Clusters: clusters,
						},
					},
					AccessLog:   accesslogger,
					IdleTimeout: idleTimeout,
				}),
			},
		}
	}
}

// SocketAddress creates a new TCP envoy_api_v2_core.Address.
func SocketAddress(address string, port int) *envoy_api_v2_core.Address {
	if address == "::" {
		return &envoy_api_v2_core.Address{
			Address: &envoy_api_v2_core.Address_SocketAddress{
				SocketAddress: &envoy_api_v2_core.SocketAddress{
					Protocol:   envoy_api_v2_core.SocketAddress_TCP,
					Address:    address,
					Ipv4Compat: true,
					PortSpecifier: &envoy_api_v2_core.SocketAddress_PortValue{
						PortValue: uint32(port),
					},
				},
			},
		}
	}
	return &envoy_api_v2_core.Address{
		Address: &envoy_api_v2_core.Address_SocketAddress{
			SocketAddress: &envoy_api_v2_core.SocketAddress{
				Protocol: envoy_api_v2_core.SocketAddress_TCP,
				Address:  address,
				PortSpecifier: &envoy_api_v2_core.SocketAddress_PortValue{
					PortValue: uint32(port),
				},
			},
		},
	}
}

// Filters returns a []*envoy_api_v2_listener.Filter for the supplied filters.
func Filters(filters ...*envoy_api_v2_listener.Filter) []*envoy_api_v2_listener.Filter {
	if len(filters) == 0 {
		return nil
	}
	return filters
}

// FilterChain retruns a *envoy_api_v2_listener.FilterChain for the supplied filters.
func FilterChain(filters ...*envoy_api_v2_listener.Filter) *envoy_api_v2_listener.FilterChain {
	return &envoy_api_v2_listener.FilterChain{
		Filters: filters,
	}
}

// FilterChains returns a []*envoy_api_v2_listener.FilterChain for the supplied filters.
func FilterChains(filters ...*envoy_api_v2_listener.Filter) []*envoy_api_v2_listener.FilterChain {
	if len(filters) == 0 {
		return nil
	}
	return []*envoy_api_v2_listener.FilterChain{
		FilterChain(filters...),
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
	local host = headers:get(":authority")
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
			TypedConfig: protobuf.MustMarshalAny(&lua.Lua{
				InlineCode: fmt.Sprintf(code, fqdn),
			}),
		},
	}
}

// FilterChainTLS returns a TLS enabled envoy_api_v2_listener.FilterChain.
func FilterChainTLS(domain string, downstream *envoy_api_v2_auth.DownstreamTlsContext, filters []*envoy_api_v2_listener.Filter) *envoy_api_v2_listener.FilterChain {
	fc := &envoy_api_v2_listener.FilterChain{
		Filters: filters,
		FilterChainMatch: &envoy_api_v2_listener.FilterChainMatch{
			ServerNames: []string{domain},
		},
	}
	// Attach TLS data to this listener if provided.
	if downstream != nil {
		fc.TransportSocket = DownstreamTLSTransportSocket(downstream)

	}
	return fc
}

// FilterChainTLSFallback returns a TLS enabled envoy_api_v2_listener.FilterChain conifgured for FallbackCertificate.
func FilterChainTLSFallback(downstream *envoy_api_v2_auth.DownstreamTlsContext, filters []*envoy_api_v2_listener.Filter) *envoy_api_v2_listener.FilterChain {
	fc := &envoy_api_v2_listener.FilterChain{
		Name:    "fallback-certificate",
		Filters: filters,
		FilterChainMatch: &envoy_api_v2_listener.FilterChainMatch{
			TransportProtocol: "tls",
		},
	}
	// Attach TLS data to this listener if provided.
	if downstream != nil {
		fc.TransportSocket = DownstreamTLSTransportSocket(downstream)
	}
	return fc
}

// ListenerFilters returns a []*envoy_api_v2_listener.ListenerFilter for the supplied listener filters.
func ListenerFilters(filters ...*envoy_api_v2_listener.ListenerFilter) []*envoy_api_v2_listener.ListenerFilter {
	return filters
}

func ContainsFallbackFilterChain(filterchains []*envoy_api_v2_listener.FilterChain) bool {
	for _, fc := range filterchains {
		if fc.Name == "fallback-certificate" {
			return true
		}
	}
	return false
}
