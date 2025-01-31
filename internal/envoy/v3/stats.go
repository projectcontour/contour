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
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_filter_http_router_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/router/v3"
	envoy_filter_network_http_connection_manager_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoy_transport_socket_tls_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoy_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"google.golang.org/protobuf/types/known/wrapperspb"

	contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/protobuf"
)

const (
	metricsServerCertSDSName = "metrics-tls-certificate"
	metricsCaBundleSDSName   = "metrics-ca-certificate"
)

// StatsListeners returns an array of *envoy_config_listener_v3.Listeners,
// either single HTTP listener or HTTP and HTTPS listeners depending on config.
// The listeners are configured to serve:
//   - prometheus metrics on /stats (either over HTTP or HTTPS)
//   - readiness probe on /ready (always over HTTP)
func (e *EnvoyGen) StatsListeners(metrics contour_v1alpha1.MetricsConfig, health contour_v1alpha1.HealthConfig) []*envoy_config_listener_v3.Listener {
	var listeners []*envoy_config_listener_v3.Listener

	switch {
	// Create HTTPS listener for metrics and HTTP listener for health.
	case metrics.TLS != nil:
		listeners = []*envoy_config_listener_v3.Listener{{
			Name:          "stats",
			Address:       SocketAddress(metrics.Address, metrics.Port),
			SocketOptions: NewSocketOptions().TCPKeepalive().Build(),
			FilterChains: filterChain("stats",
				DownstreamTLSTransportSocket(
					e.downstreamTLSContext(metrics.TLS.CAFile != "")), routeForAdminInterface("/stats", "/stats/prometheus")),
		}, {
			Name:          "health",
			Address:       SocketAddress(health.Address, health.Port),
			SocketOptions: NewSocketOptions().TCPKeepalive().Build(),
			FilterChains:  filterChain("stats", nil, routeForAdminInterface("/ready")),
		}}

	// Create combined HTTP listener for metrics and health.
	case (metrics.Address == health.Address) &&
		(metrics.Port == health.Port):
		listeners = []*envoy_config_listener_v3.Listener{{
			Name:          "stats-health",
			Address:       SocketAddress(metrics.Address, metrics.Port),
			SocketOptions: NewSocketOptions().TCPKeepalive().Build(),
			FilterChains: filterChain("stats", nil, routeForAdminInterface(
				"/ready",
				"/stats",
				"/stats/prometheus",
			)),
		}}

	// Create separate HTTP listeners for metrics and health.
	default:
		listeners = []*envoy_config_listener_v3.Listener{{
			Name:          "stats",
			Address:       SocketAddress(metrics.Address, metrics.Port),
			SocketOptions: NewSocketOptions().TCPKeepalive().Build(),
			FilterChains:  filterChain("stats", nil, routeForAdminInterface("/stats", "/stats/prometheus")),
		}, {
			Name:          "health",
			Address:       SocketAddress(health.Address, health.Port),
			SocketOptions: NewSocketOptions().TCPKeepalive().Build(),
			FilterChains:  filterChain("stats", nil, routeForAdminInterface("/ready")),
		}}
	}

	return listeners
}

// AdminListener returns a *envoy_config_listener_v3.Listener configured to serve Envoy
// debug routes from the admin webpage.
func AdminListener(port int) *envoy_config_listener_v3.Listener {
	return &envoy_config_listener_v3.Listener{
		Name:    "envoy-admin",
		Address: SocketAddress("127.0.0.1", port),
		FilterChains: filterChain("envoy-admin", nil,
			routeForAdminInterface(
				"/certs",
				"/clusters",
				"/listeners",
				"/config_dump",
				"/memory",
				"/ready",
				"/runtime",
				"/server_info",
				"/stats",
				"/stats/prometheus",
				"/stats/recentlookups",
			),
		),
		SocketOptions: NewSocketOptions().TCPKeepalive().Build(),
	}
}

// filterChain returns a filter chain used by static listeners.
func filterChain(statsPrefix string, transportSocket *envoy_config_core_v3.TransportSocket, routes *envoy_filter_network_http_connection_manager_v3.HttpConnectionManager_RouteConfig) []*envoy_config_listener_v3.FilterChain {
	return []*envoy_config_listener_v3.FilterChain{{
		Filters: []*envoy_config_listener_v3.Filter{{
			Name: wellknown.HTTPConnectionManager,
			ConfigType: &envoy_config_listener_v3.Filter_TypedConfig{
				TypedConfig: protobuf.MustMarshalAny(&envoy_filter_network_http_connection_manager_v3.HttpConnectionManager{
					StatPrefix:     statsPrefix,
					RouteSpecifier: routes,
					HttpFilters: []*envoy_filter_network_http_connection_manager_v3.HttpFilter{{
						Name: wellknown.Router,
						ConfigType: &envoy_filter_network_http_connection_manager_v3.HttpFilter_TypedConfig{
							TypedConfig: protobuf.MustMarshalAny(&envoy_filter_http_router_v3.Router{}),
						},
					}},
					NormalizePath: wrapperspb.Bool(true),
				}),
			},
		}},
		TransportSocket: transportSocket,
	}}
}

// routeForAdminInterface creates static RouteConfig that forwards requested paths to Envoy admin interface.
func routeForAdminInterface(paths ...string) *envoy_filter_network_http_connection_manager_v3.HttpConnectionManager_RouteConfig {
	config := &envoy_filter_network_http_connection_manager_v3.HttpConnectionManager_RouteConfig{
		RouteConfig: &envoy_config_route_v3.RouteConfiguration{
			VirtualHosts: []*envoy_config_route_v3.VirtualHost{{
				Name:    "backend",
				Domains: []string{"*"},
			}},
		},
	}

	for _, p := range paths {
		config.RouteConfig.VirtualHosts[0].Routes = append(config.RouteConfig.VirtualHosts[0].Routes,
			&envoy_config_route_v3.Route{
				Match: &envoy_config_route_v3.RouteMatch{
					PathSpecifier: &envoy_config_route_v3.RouteMatch_Path{
						Path: p,
					},
					Headers: []*envoy_config_route_v3.HeaderMatcher{
						{
							Name: ":method",
							HeaderMatchSpecifier: &envoy_config_route_v3.HeaderMatcher_StringMatch{
								StringMatch: &envoy_matcher_v3.StringMatcher{
									IgnoreCase: true,
									MatchPattern: &envoy_matcher_v3.StringMatcher_Exact{
										Exact: "GET",
									},
								},
							},
						},
					},
				},
				Action: &envoy_config_route_v3.Route_Route{
					Route: &envoy_config_route_v3.RouteAction{
						ClusterSpecifier: &envoy_config_route_v3.RouteAction_Cluster{
							Cluster: "envoy-admin",
						},
					},
				},
			},
		)
	}
	return config
}

// downstreamTLSContext creates TLS context when HTTPS is used to protect Envoy stats endpoint.
// Certificates and key are hardcoded to the SDS secrets which are returned by StatsSecrets.
func (e *EnvoyGen) downstreamTLSContext(clientValidation bool) *envoy_transport_socket_tls_v3.DownstreamTlsContext {
	context := &envoy_transport_socket_tls_v3.DownstreamTlsContext{
		CommonTlsContext: &envoy_transport_socket_tls_v3.CommonTlsContext{
			TlsParams: &envoy_transport_socket_tls_v3.TlsParameters{
				TlsMinimumProtocolVersion: envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3,
				TlsMaximumProtocolVersion: envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3,
			},
			TlsCertificateSdsSecretConfigs: []*envoy_transport_socket_tls_v3.SdsSecretConfig{{
				Name:      metricsServerCertSDSName,
				SdsConfig: e.GetConfigSource(),
			}},
		},
	}

	if clientValidation {
		context.CommonTlsContext.ValidationContextType = &envoy_transport_socket_tls_v3.CommonTlsContext_ValidationContextSdsSecretConfig{
			ValidationContextSdsSecretConfig: &envoy_transport_socket_tls_v3.SdsSecretConfig{
				Name:      metricsCaBundleSDSName,
				SdsConfig: e.GetConfigSource(),
			},
		}
		context.RequireClientCertificate = wrapperspb.Bool(true)
	}

	return context
}

// StatsSecrets returns SDS secrets that refer to local file paths in Envoy container.
func StatsSecrets(metricsTLS *contour_v1alpha1.MetricsTLS) []*envoy_transport_socket_tls_v3.Secret {
	secrets := []*envoy_transport_socket_tls_v3.Secret{}

	if metricsTLS != nil {
		if metricsTLS.CertFile != "" && metricsTLS.KeyFile != "" {
			secrets = append(secrets, &envoy_transport_socket_tls_v3.Secret{
				Name: metricsServerCertSDSName,
				Type: &envoy_transport_socket_tls_v3.Secret_TlsCertificate{
					TlsCertificate: &envoy_transport_socket_tls_v3.TlsCertificate{
						CertificateChain: &envoy_config_core_v3.DataSource{
							Specifier: &envoy_config_core_v3.DataSource_Filename{
								Filename: metricsTLS.CertFile,
							},
						},
						PrivateKey: &envoy_config_core_v3.DataSource{
							Specifier: &envoy_config_core_v3.DataSource_Filename{
								Filename: metricsTLS.KeyFile,
							},
						},
					},
				},
			})
		}
		if metricsTLS.CAFile != "" {
			secrets = append(secrets, &envoy_transport_socket_tls_v3.Secret{
				Name: metricsCaBundleSDSName,
				Type: &envoy_transport_socket_tls_v3.Secret_ValidationContext{
					ValidationContext: &envoy_transport_socket_tls_v3.CertificateValidationContext{
						TrustedCa: &envoy_config_core_v3.DataSource{
							Specifier: &envoy_config_core_v3.DataSource_Filename{
								Filename: metricsTLS.CAFile,
							},
						},
					},
				},
			})
		}
	}

	return secrets
}
