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
	envoy_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	http "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoy_tls_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	contour_api_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/protobuf"
)

const metricsServerCertSDSName = "metrics-tls-certificate"
const metricsCaBundleSDSName = "metrics-ca-certificate"

// StatsListeners returns an array of *envoy_listener_v3.Listeners,
// either single HTTP listener or HTTP and HTTPS listeners depending on config.
// The listeners are configured to serve:
//   - prometheus metrics on /stats (either over HTTP or HTTPS)
//   - readiness probe on /ready (always over HTTP)
func StatsListeners(metrics *contour_api_v1alpha1.MetricsConfig, health *contour_api_v1alpha1.HealthConfig) []*envoy_listener_v3.Listener {
	var listeners []*envoy_listener_v3.Listener

	switch {
	// Create HTTPS listener for metrics and HTTP listener for health.
	case metrics.TLS != nil:
		listeners = []*envoy_listener_v3.Listener{{
			Name:          "stats",
			Address:       SocketAddress(metrics.Address, metrics.Port),
			SocketOptions: TCPKeepaliveSocketOptions(),
			FilterChains: filterChain("stats",
				DownstreamTLSTransportSocket(
					downstreamTLSContext(metrics.TLS.CAFile != "")), routeForAdminInterface("/stats")),
		}, {
			Name:          "health",
			Address:       SocketAddress(health.Address, health.Port),
			SocketOptions: TCPKeepaliveSocketOptions(),
			FilterChains:  filterChain("stats", nil, routeForAdminInterface("/ready")),
		}}

	// Create combined HTTP listener for metrics and health.
	case (metrics.Address == health.Address) &&
		(metrics.Port == health.Port):
		listeners = []*envoy_listener_v3.Listener{{
			Name:          "stats-health",
			Address:       SocketAddress(metrics.Address, metrics.Port),
			SocketOptions: TCPKeepaliveSocketOptions(),
			FilterChains:  filterChain("stats", nil, routeForAdminInterface("/ready", "/stats")),
		}}

	// Create separate HTTP listeners for metrics and health.
	default:
		listeners = []*envoy_listener_v3.Listener{{
			Name:          "stats",
			Address:       SocketAddress(metrics.Address, metrics.Port),
			SocketOptions: TCPKeepaliveSocketOptions(),
			FilterChains:  filterChain("stats", nil, routeForAdminInterface("/stats")),
		}, {
			Name:          "health",
			Address:       SocketAddress(health.Address, health.Port),
			SocketOptions: TCPKeepaliveSocketOptions(),
			FilterChains:  filterChain("stats", nil, routeForAdminInterface("/ready")),
		}}
	}

	return listeners
}

// AdminListener returns a *envoy_listener_v3.Listener configured to serve Envoy
// debug routes from the admin webpage.
func AdminListener(port int) *envoy_listener_v3.Listener {
	return &envoy_listener_v3.Listener{
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
		SocketOptions: TCPKeepaliveSocketOptions(),
	}
}

// filterChain returns a filter chain used by static listeners.
func filterChain(statsPrefix string, transportSocket *envoy_core_v3.TransportSocket, routes *http.HttpConnectionManager_RouteConfig) []*envoy_listener_v3.FilterChain {
	return []*envoy_listener_v3.FilterChain{{
		Filters: []*envoy_listener_v3.Filter{{
			Name: wellknown.HTTPConnectionManager,
			ConfigType: &envoy_listener_v3.Filter_TypedConfig{
				TypedConfig: protobuf.MustMarshalAny(&http.HttpConnectionManager{
					StatPrefix:     statsPrefix,
					RouteSpecifier: routes,
					HttpFilters: []*http.HttpFilter{{
						Name: wellknown.Router,
					}},
					NormalizePath: protobuf.Bool(true),
				}),
			},
		}},
		TransportSocket: transportSocket,
	}}
}

// routeForAdminInterface creates static RouteConfig that forwards requested prefixes to Envoy admin interface.
func routeForAdminInterface(prefixes ...string) *http.HttpConnectionManager_RouteConfig {
	config := &http.HttpConnectionManager_RouteConfig{
		RouteConfig: &envoy_route_v3.RouteConfiguration{
			VirtualHosts: []*envoy_route_v3.VirtualHost{{
				Name:    "backend",
				Domains: []string{"*"},
			}},
		},
	}

	for _, prefix := range prefixes {
		config.RouteConfig.VirtualHosts[0].Routes = append(config.RouteConfig.VirtualHosts[0].Routes,
			&envoy_route_v3.Route{
				Match: &envoy_route_v3.RouteMatch{
					PathSpecifier: &envoy_route_v3.RouteMatch_Prefix{
						Prefix: prefix,
					},
				},
				Action: &envoy_route_v3.Route_Route{
					Route: &envoy_route_v3.RouteAction{
						ClusterSpecifier: &envoy_route_v3.RouteAction_Cluster{
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
func downstreamTLSContext(clientValidation bool) *envoy_tls_v3.DownstreamTlsContext {
	context := &envoy_tls_v3.DownstreamTlsContext{
		CommonTlsContext: &envoy_tls_v3.CommonTlsContext{
			TlsParams: &envoy_tls_v3.TlsParameters{
				TlsMinimumProtocolVersion: envoy_tls_v3.TlsParameters_TLSv1_3,
				TlsMaximumProtocolVersion: envoy_tls_v3.TlsParameters_TLSv1_3,
			},
			TlsCertificateSdsSecretConfigs: []*envoy_tls_v3.SdsSecretConfig{{
				Name:      metricsServerCertSDSName,
				SdsConfig: ConfigSource("contour"),
			}},
		},
	}

	if clientValidation {
		context.CommonTlsContext.ValidationContextType = &envoy_tls_v3.CommonTlsContext_ValidationContextSdsSecretConfig{
			ValidationContextSdsSecretConfig: &envoy_tls_v3.SdsSecretConfig{
				Name:      metricsCaBundleSDSName,
				SdsConfig: ConfigSource("contour"),
			},
		}
		context.RequireClientCertificate = protobuf.Bool(true)
	}

	return context
}

// StatsSecrets returns SDS secrets that refer to local file paths in Envoy container.
func StatsSecrets(metricsTLS *contour_api_v1alpha1.MetricsTLS) []*envoy_tls_v3.Secret {
	secrets := []*envoy_tls_v3.Secret{}

	if metricsTLS != nil {
		if metricsTLS.CertFile != "" && metricsTLS.KeyFile != "" {
			secrets = append(secrets, &envoy_tls_v3.Secret{
				Name: metricsServerCertSDSName,
				Type: &envoy_tls_v3.Secret_TlsCertificate{
					TlsCertificate: &envoy_tls_v3.TlsCertificate{
						CertificateChain: &envoy_core_v3.DataSource{
							Specifier: &envoy_core_v3.DataSource_Filename{
								Filename: metricsTLS.CertFile,
							},
						},
						PrivateKey: &envoy_core_v3.DataSource{
							Specifier: &envoy_core_v3.DataSource_Filename{
								Filename: metricsTLS.KeyFile,
							},
						},
					},
				},
			})
		}
		if metricsTLS.CAFile != "" {
			secrets = append(secrets, &envoy_tls_v3.Secret{
				Name: metricsCaBundleSDSName,
				Type: &envoy_tls_v3.Secret_ValidationContext{
					ValidationContext: &envoy_tls_v3.CertificateValidationContext{
						TrustedCa: &envoy_core_v3.DataSource{
							Specifier: &envoy_core_v3.DataSource_Filename{
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
