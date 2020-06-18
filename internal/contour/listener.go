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

package contour

import (
	"path"
	"sort"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_api_v2_auth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	envoy_api_v2_listener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	xds "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/envoy"
	"github.com/projectcontour/contour/internal/protobuf"
	"github.com/projectcontour/contour/internal/sorter"
)

const (
	ENVOY_HTTP_LISTENER            = "ingress_http"
	ENVOY_FALLBACK_ROUTECONFIG     = "ingress_fallbackcert"
	ENVOY_HTTPS_LISTENER           = "ingress_https"
	DEFAULT_HTTP_ACCESS_LOG        = "/dev/stdout"
	DEFAULT_HTTP_LISTENER_ADDRESS  = "0.0.0.0"
	DEFAULT_HTTP_LISTENER_PORT     = 8080
	DEFAULT_HTTPS_ACCESS_LOG       = "/dev/stdout"
	DEFAULT_HTTPS_LISTENER_ADDRESS = DEFAULT_HTTP_LISTENER_ADDRESS
	DEFAULT_HTTPS_LISTENER_PORT    = 8443
	DEFAULT_ACCESS_LOG_TYPE        = "envoy"
)

// Contents returns an array of LDS resources.
func translateListeners(listeners map[string]*v2.Listener, lvc *ListenerConfig) []xds.Resource {
	var values []*v2.Listener
	for _, v := range listeners {
		values = append(values, v)
	}
	for _, v := range lvc.StaticListeners {
		values = append(values, v)
	}

	sort.Stable(sorter.For(values))
	return protobuf.AsMessages(values)
}

type listenerVisitor struct {
	*ListenerConfig

	listeners map[string]*v2.Listener
	http      bool // at least one dag.VirtualHost encountered
}

func visitListeners(root dag.Vertex, lvc *ListenerConfig) []xds.Resource {
	lv := listenerVisitor{
		ListenerConfig: lvc,
		listeners: map[string]*v2.Listener{
			ENVOY_HTTPS_LISTENER: envoy.Listener(
				ENVOY_HTTPS_LISTENER,
				lvc.httpsAddress(),
				lvc.httpsPort(),
				secureProxyProtocol(lvc.UseProxyProto),
			),
		},
	}

	lv.visit(root)

	if lv.http {
		// Add a listener if there are vhosts bound to http.
		cm := envoy.HTTPConnectionManagerBuilder().
			Codec(envoy.CodecForVersions(lv.DefaultHTTPVersions...)).
			DefaultFilters().
			RouteConfigName(ENVOY_HTTP_LISTENER).
			MetricsPrefix(ENVOY_HTTP_LISTENER).
			AccessLoggers(lvc.newInsecureAccessLog()).
			RequestTimeout(lvc.RequestTimeout).
			ConnectionIdleTimeout(lvc.ConnectionIdleTimeout).
			StreamIdleTimeout(lvc.StreamIdleTimeout).
			MaxConnectionDuration(lvc.MaxConnectionDuration).
			ConnectionShutdownGracePeriod(lvc.ConnectionShutdownGracePeriod).
			Get()

		lv.listeners[ENVOY_HTTP_LISTENER] = envoy.Listener(
			ENVOY_HTTP_LISTENER,
			lvc.httpAddress(),
			lvc.httpPort(),
			proxyProtocol(lvc.UseProxyProto),
			cm,
		)
	}

	// Remove the https listener if there are no vhosts bound to it.
	if len(lv.listeners[ENVOY_HTTPS_LISTENER].FilterChains) == 0 {
		delete(lv.listeners, ENVOY_HTTPS_LISTENER)
	} else {
		// there's some https listeners, we need to sort the filter chains
		// to ensure that the LDS entries are identical.
		sort.Stable(sorter.For(lv.listeners[ENVOY_HTTPS_LISTENER].FilterChains))
	}

	return translateListeners(lv.listeners, lvc)
}

func proxyProtocol(useProxy bool) []*envoy_api_v2_listener.ListenerFilter {
	if useProxy {
		return envoy.ListenerFilters(
			envoy.ProxyProtocol(),
		)
	}
	return nil
}

func secureProxyProtocol(useProxy bool) []*envoy_api_v2_listener.ListenerFilter {
	return append(proxyProtocol(useProxy), envoy.TLSInspector())
}

func (v *listenerVisitor) visit(vertex dag.Vertex) {
	max := func(a, b envoy_api_v2_auth.TlsParameters_TlsProtocol) envoy_api_v2_auth.TlsParameters_TlsProtocol {
		if a > b {
			return a
		}
		return b
	}

	switch vh := vertex.(type) {
	case *dag.VirtualHost:
		// we only create on http listener so record the fact
		// that we need to then double back at the end and add
		// the listener properly.
		v.http = true
	case *dag.SecureVirtualHost:
		var alpnProtos []string
		var filters []*envoy_api_v2_listener.Filter

		if vh.TCPProxy == nil {
			// Create a uniquely named HTTP connection manager for
			// this vhost, so that the SNI name the client requests
			// only grants access to that host. See RFC 6066 for
			// security advice. Note that we still use the generic
			// metrics prefix to keep compatibility with previous
			// Contour versions since the metrics prefix will be
			// coded into monitoring dashboards.
			filters = envoy.Filters(
				envoy.HTTPConnectionManagerBuilder().
					Codec(envoy.CodecForVersions(v.DefaultHTTPVersions...)).
					AddFilter(envoy.FilterMisdirectedRequests(vh.VirtualHost.Name)).
					DefaultFilters().
					RouteConfigName(path.Join("https", vh.VirtualHost.Name)).
					MetricsPrefix(ENVOY_HTTPS_LISTENER).
					AccessLoggers(v.ListenerConfig.newSecureAccessLog()).
					RequestTimeout(v.ListenerConfig.RequestTimeout).
					ConnectionIdleTimeout(v.ListenerConfig.ConnectionIdleTimeout).
					StreamIdleTimeout(v.ListenerConfig.StreamIdleTimeout).
					MaxConnectionDuration(v.ListenerConfig.MaxConnectionDuration).
					ConnectionShutdownGracePeriod(v.ListenerConfig.ConnectionShutdownGracePeriod).
					Get(),
			)

			alpnProtos = envoy.ProtoNamesForVersions(v.DefaultHTTPVersions...)
		} else {
			filters = envoy.Filters(
				envoy.TCPProxy(ENVOY_HTTPS_LISTENER,
					vh.TCPProxy,
					v.ListenerConfig.newSecureAccessLog()),
			)

			// Do not offer ALPN for TCP proxying, since
			// the protocols will be provided by the TCP
			// backend in its ServerHello.
		}

		var downstreamTLS *envoy_api_v2_auth.DownstreamTlsContext

		// Secret is provided when TLS is terminated and nil when TLS passthrough is used.
		if vh.Secret != nil {
			// Choose the higher of the configured or requested TLS version.
			vers := max(v.ListenerConfig.minTLSVersion(), vh.MinTLSVersion)

			downstreamTLS = envoy.DownstreamTLSContext(
				vh.Secret,
				vers,
				vh.DownstreamValidation,
				alpnProtos...)
		}

		v.listeners[ENVOY_HTTPS_LISTENER].FilterChains = append(v.listeners[ENVOY_HTTPS_LISTENER].FilterChains,
			envoy.FilterChainTLS(vh.VirtualHost.Name, downstreamTLS, filters))

		// If this VirtualHost has enabled the fallback certificate then set a default
		// FilterChain which will allow routes with this vhost to accept non-SNI TLS requests.
		// Note that we don't add the misdirected requests filter on this chain because at this
		// point we don't actually know the full set of server names that will be bound to the
		// filter chain through the ENVOY_FALLBACK_ROUTECONFIG route configuration.
		if vh.FallbackCertificate != nil && !envoy.ContainsFallbackFilterChain(v.listeners[ENVOY_HTTPS_LISTENER].FilterChains) {
			// Construct the downstreamTLSContext passing the configured fallbackCertificate. The TLS minProtocolVersion will use
			// the value defined in the Contour Configuration file if defined.
			downstreamTLS = envoy.DownstreamTLSContext(
				vh.FallbackCertificate,
				v.ListenerConfig.minTLSVersion(),
				vh.DownstreamValidation,
				alpnProtos...)

			// Default filter chain
			filters = envoy.Filters(
				envoy.HTTPConnectionManagerBuilder().
					DefaultFilters().
					RouteConfigName(ENVOY_FALLBACK_ROUTECONFIG).
					MetricsPrefix(ENVOY_HTTPS_LISTENER).
					AccessLoggers(v.ListenerConfig.newSecureAccessLog()).
					RequestTimeout(v.ListenerConfig.RequestTimeout).
					ConnectionIdleTimeout(v.ListenerConfig.ConnectionIdleTimeout).
					StreamIdleTimeout(v.ListenerConfig.StreamIdleTimeout).
					MaxConnectionDuration(v.ListenerConfig.MaxConnectionDuration).
					ConnectionShutdownGracePeriod(v.ListenerConfig.ConnectionShutdownGracePeriod).
					Get(),
			)

			v.listeners[ENVOY_HTTPS_LISTENER].FilterChains = append(v.listeners[ENVOY_HTTPS_LISTENER].FilterChains,
				envoy.FilterChainTLSFallback(downstreamTLS, filters))
		}

	default:
		// recurse
		vertex.Visit(v.visit)
	}
}
