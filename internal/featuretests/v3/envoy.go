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

// envoy helpers

import (
	"path"
	"time"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_filter_http_ext_authz_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	envoy_filter_http_jwt_authn_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/jwt_authn/v3"
	envoy_filter_network_http_connection_manager_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoy_filter_network_tcp_proxy_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	envoy_transport_socket_tls_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoy_upstream_http_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	envoy_type_v3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	core_v1 "k8s.io/api/core/v1"

	contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/dag"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/protobuf"
	xdscache_v3 "github.com/projectcontour/contour/internal/xdscache/v3"
)

// DefaultCluster returns a copy of the default Cluster, with each
// Cluster given in the parameter slice merged on top. This makes it
// relatively fluent to compose Clusters by tweaking a few fields.
func DefaultCluster(clusters ...*envoy_config_cluster_v3.Cluster) *envoy_config_cluster_v3.Cluster {
	// NOTE: Keep this in sync with envoy.defaultCluster().
	defaults := &envoy_config_cluster_v3.Cluster{
		ConnectTimeout: durationpb.New(2 * time.Second),
		LbPolicy:       envoy_config_cluster_v3.Cluster_ROUND_ROBIN,
		CommonLbConfig: envoy_v3.ClusterCommonLBConfig(),
	}

	for _, c := range clusters {
		proto.Merge(defaults, c)
	}

	return defaults
}

func clusterWithHealthCheck(name, servicename, statName, healthCheckPath string, expectedStatuses []*envoy_type_v3.Int64Range) *envoy_config_cluster_v3.Cluster {
	c := cluster(name, servicename, statName)
	c.HealthChecks = []*envoy_config_core_v3.HealthCheck{{
		Timeout:            durationpb.New(2 * time.Second),
		Interval:           durationpb.New(10 * time.Second),
		UnhealthyThreshold: wrapperspb.UInt32(3),
		HealthyThreshold:   wrapperspb.UInt32(2),
		HealthChecker: &envoy_config_core_v3.HealthCheck_HttpHealthCheck_{
			HttpHealthCheck: &envoy_config_core_v3.HealthCheck_HttpHealthCheck{
				Host:             "contour-envoy-healthcheck",
				Path:             healthCheckPath,
				ExpectedStatuses: expectedStatuses,
			},
		},
	}}
	c.IgnoreHealthOnHostRemoval = true
	return c
}

func externalNameCluster(name, servicename, statName, externalName string, port int) *envoy_config_cluster_v3.Cluster {
	return DefaultCluster(&envoy_config_cluster_v3.Cluster{
		Name:                 name,
		ClusterDiscoveryType: envoy_v3.ClusterDiscoveryType(envoy_config_cluster_v3.Cluster_STRICT_DNS),
		AltStatName:          statName,
		LoadAssignment: &envoy_config_endpoint_v3.ClusterLoadAssignment{
			ClusterName: servicename,
			Endpoints: envoy_v3.Endpoints(
				envoy_v3.SocketAddress(externalName, port),
			),
		},
	})
}

func routeCluster(cluster string, opts ...func(*envoy_config_route_v3.Route_Route)) *envoy_config_route_v3.Route_Route {
	r := &envoy_config_route_v3.Route_Route{
		Route: &envoy_config_route_v3.RouteAction{
			ClusterSpecifier: &envoy_config_route_v3.RouteAction_Cluster{
				Cluster: cluster,
			},
		},
	}

	for _, o := range opts {
		o(r)
	}

	return r
}

func routePrefix(prefix string) *envoy_config_route_v3.RouteMatch {
	return envoy_v3.RouteMatch(&dag.Route{
		PathMatchCondition: &dag.PrefixMatchCondition{
			Prefix: prefix,
		},
	})
}

func routePrefixWithHeaderConditions(prefix string, headers ...dag.HeaderMatchCondition) *envoy_config_route_v3.RouteMatch {
	return envoy_v3.RouteMatch(&dag.Route{
		PathMatchCondition: &dag.PrefixMatchCondition{
			Prefix: prefix,
		},
		HeaderMatchConditions: headers,
	})
}

func routePrefixWithQueryParameterConditions(prefix string, queryParams ...dag.QueryParamMatchCondition) *envoy_config_route_v3.RouteMatch {
	return envoy_v3.RouteMatch(&dag.Route{
		PathMatchCondition: &dag.PrefixMatchCondition{
			Prefix: prefix,
		},
		QueryParamMatchConditions: queryParams,
	})
}

func routeSegmentPrefix(prefix string) *envoy_config_route_v3.RouteMatch {
	return &envoy_config_route_v3.RouteMatch{
		PathSpecifier: &envoy_config_route_v3.RouteMatch_PathSeparatedPrefix{
			PathSeparatedPrefix: prefix,
		},
	}
}

func routeHostRewrite(cluster, newHostName string) *envoy_config_route_v3.Route_Route {
	return &envoy_config_route_v3.Route_Route{
		Route: &envoy_config_route_v3.RouteAction{
			ClusterSpecifier:     &envoy_config_route_v3.RouteAction_Cluster{Cluster: cluster},
			HostRewriteSpecifier: &envoy_config_route_v3.RouteAction_HostRewriteLiteral{HostRewriteLiteral: newHostName},
		},
	}
}

func routeHostRewriteHeader(cluster, hostnameHeader string) *envoy_config_route_v3.Route_Route {
	return &envoy_config_route_v3.Route_Route{
		Route: &envoy_config_route_v3.RouteAction{
			ClusterSpecifier:     &envoy_config_route_v3.RouteAction_Cluster{Cluster: cluster},
			HostRewriteSpecifier: &envoy_config_route_v3.RouteAction_HostRewriteHeader{HostRewriteHeader: hostnameHeader},
		},
	}
}

func upgradeHTTPS(match *envoy_config_route_v3.RouteMatch) *envoy_config_route_v3.Route {
	return &envoy_config_route_v3.Route{
		Match:                match,
		Action:               envoy_v3.UpgradeHTTPS(),
		TypedPerFilterConfig: envoy_v3.DisabledExtAuthConfig(),
	}
}

func cluster(name, servicename, statName string) *envoy_config_cluster_v3.Cluster {
	return DefaultCluster(&envoy_config_cluster_v3.Cluster{
		Name:                 name,
		ClusterDiscoveryType: envoy_v3.ClusterDiscoveryType(envoy_config_cluster_v3.Cluster_EDS),
		AltStatName:          statName,
		EdsClusterConfig: &envoy_config_cluster_v3.Cluster_EdsClusterConfig{
			EdsConfig: envoy_v3.NewEnvoyGen(envoy_v3.EnvoyGenOpt{
				XDSClusterName: envoy_v3.DefaultXDSClusterName,
			}).GetConfigSource(),
			ServiceName: servicename,
		},
	})
}

func tlsCluster(c *envoy_config_cluster_v3.Cluster, ca *core_v1.Secret, subjectName, sni string, clientSecret *core_v1.Secret, upstreamTLS *dag.UpstreamTLS, alpnProtocols ...string) *envoy_config_cluster_v3.Cluster {
	var secret *dag.Secret
	if clientSecret != nil {
		secret = &dag.Secret{Object: clientSecret}
	}

	// Secret for validation is optional.
	var s []*dag.Secret
	if ca != nil {
		s = []*dag.Secret{{Object: ca}}
	}

	c.TransportSocket = envoy_v3.UpstreamTLSTransportSocket(
		envoy_v3.NewEnvoyGen(envoy_v3.EnvoyGenOpt{
			XDSClusterName: envoy_v3.DefaultXDSClusterName,
		}).UpstreamTLSContext(
			&dag.PeerValidationContext{
				CACertificates: s,
				SubjectNames:   []string{subjectName},
			},
			sni,
			secret,
			upstreamTLS,
			alpnProtocols...,
		),
	)
	return c
}

func tlsClusterWithoutValidation(c *envoy_config_cluster_v3.Cluster, sni string, clientSecret *core_v1.Secret, upstreamTLS *dag.UpstreamTLS, alpnProtocols ...string) *envoy_config_cluster_v3.Cluster {
	var secret *dag.Secret
	if clientSecret != nil {
		secret = &dag.Secret{Object: clientSecret}
	}

	c.TransportSocket = envoy_v3.UpstreamTLSTransportSocket(
		envoy_v3.NewEnvoyGen(envoy_v3.EnvoyGenOpt{
			XDSClusterName: envoy_v3.DefaultXDSClusterName,
		}).UpstreamTLSContext(
			nil,
			sni,
			secret,
			upstreamTLS,
			alpnProtocols...,
		),
	)
	return c
}

func h2cCluster(c *envoy_config_cluster_v3.Cluster) *envoy_config_cluster_v3.Cluster {
	c.TypedExtensionProtocolOptions = map[string]*anypb.Any{
		"envoy.extensions.upstreams.http.v3.HttpProtocolOptions": protobuf.MustMarshalAny(
			&envoy_upstream_http_v3.HttpProtocolOptions{
				UpstreamProtocolOptions: &envoy_upstream_http_v3.HttpProtocolOptions_ExplicitHttpConfig_{
					ExplicitHttpConfig: &envoy_upstream_http_v3.HttpProtocolOptions_ExplicitHttpConfig{
						ProtocolConfig: &envoy_upstream_http_v3.HttpProtocolOptions_ExplicitHttpConfig_Http2ProtocolOptions{},
					},
				},
			}),
	}
	return c
}

func withConnectionTimeout(c *envoy_config_cluster_v3.Cluster, timeout time.Duration, httpVersion envoy_v3.HTTPVersionType) *envoy_config_cluster_v3.Cluster {
	var config *envoy_upstream_http_v3.HttpProtocolOptions_ExplicitHttpConfig

	switch httpVersion {
	// Default protocol version in Envoy is HTTP1.1.
	case envoy_v3.HTTPVersion1, envoy_v3.HTTPVersionAuto:
		config = &envoy_upstream_http_v3.HttpProtocolOptions_ExplicitHttpConfig{
			ProtocolConfig: &envoy_upstream_http_v3.HttpProtocolOptions_ExplicitHttpConfig_HttpProtocolOptions{},
		}
	case envoy_v3.HTTPVersion2:
		config = &envoy_upstream_http_v3.HttpProtocolOptions_ExplicitHttpConfig{
			ProtocolConfig: &envoy_upstream_http_v3.HttpProtocolOptions_ExplicitHttpConfig_Http2ProtocolOptions{},
		}

	case envoy_v3.HTTPVersion3:
		config = &envoy_upstream_http_v3.HttpProtocolOptions_ExplicitHttpConfig{
			ProtocolConfig: &envoy_upstream_http_v3.HttpProtocolOptions_ExplicitHttpConfig_Http3ProtocolOptions{},
		}
	}

	c.TypedExtensionProtocolOptions = map[string]*anypb.Any{
		"envoy.extensions.upstreams.http.v3.HttpProtocolOptions": protobuf.MustMarshalAny(
			&envoy_upstream_http_v3.HttpProtocolOptions{
				CommonHttpProtocolOptions: &envoy_config_core_v3.HttpProtocolOptions{
					IdleTimeout: durationpb.New(timeout),
				},
				UpstreamProtocolOptions: &envoy_upstream_http_v3.HttpProtocolOptions_ExplicitHttpConfig_{
					ExplicitHttpConfig: config,
				},
			}),
	}
	return c
}

func withResponseTimeout(route *envoy_config_route_v3.Route_Route, timeout time.Duration) *envoy_config_route_v3.Route_Route {
	route.Route.Timeout = durationpb.New(timeout)
	return route
}

func withIdleTimeout(route *envoy_config_route_v3.Route_Route, timeout time.Duration) *envoy_config_route_v3.Route_Route {
	route.Route.IdleTimeout = durationpb.New(timeout)
	return route
}

func withMirrorPolicy(route *envoy_config_route_v3.Route_Route, mirror string, weight uint32) *envoy_config_route_v3.Route_Route {
	route.Route.RequestMirrorPolicies = []*envoy_config_route_v3.RouteAction_RequestMirrorPolicy{{
		Cluster: mirror,
		RuntimeFraction: &envoy_config_core_v3.RuntimeFractionalPercent{
			DefaultValue: &envoy_type_v3.FractionalPercent{
				Numerator:   weight,
				Denominator: envoy_type_v3.FractionalPercent_HUNDRED,
			},
		},
	}}
	return route
}

func withPrefixRewrite(route *envoy_config_route_v3.Route_Route, replacement string) *envoy_config_route_v3.Route_Route {
	route.Route.PrefixRewrite = replacement
	return route
}

func withRetryPolicy(route *envoy_config_route_v3.Route_Route, retryOn string, numRetries uint32, perTryTimeout time.Duration) *envoy_config_route_v3.Route_Route {
	route.Route.RetryPolicy = &envoy_config_route_v3.RetryPolicy{
		RetryOn: retryOn,
	}
	if numRetries > 0 {
		route.Route.RetryPolicy.NumRetries = wrapperspb.UInt32(numRetries)
	}
	if perTryTimeout > 0 {
		route.Route.RetryPolicy.PerTryTimeout = durationpb.New(perTryTimeout)
	}
	return route
}

func withWebsocket(route *envoy_config_route_v3.Route_Route) *envoy_config_route_v3.Route_Route {
	route.Route.UpgradeConfigs = append(route.Route.UpgradeConfigs,
		&envoy_config_route_v3.RouteAction_UpgradeConfig{
			UpgradeType: "websocket",
		},
	)
	return route
}

func withSessionAffinity(route *envoy_config_route_v3.Route_Route) *envoy_config_route_v3.Route_Route {
	route.Route.HashPolicy = append(route.Route.HashPolicy, &envoy_config_route_v3.RouteAction_HashPolicy{
		PolicySpecifier: &envoy_config_route_v3.RouteAction_HashPolicy_Cookie_{
			Cookie: &envoy_config_route_v3.RouteAction_HashPolicy_Cookie{
				Name: "X-Contour-Session-Affinity",
				Ttl:  durationpb.New(0),
				Path: "/",
			},
		},
	})
	return route
}

type hashPolicySpecifier struct {
	headerName    string
	terminal      bool
	hashSourceIP  bool
	parameterName string
}

func withRequestHashPolicySpecifiers(route *envoy_config_route_v3.Route_Route, policies ...hashPolicySpecifier) *envoy_config_route_v3.Route_Route {
	for _, p := range policies {
		hp := &envoy_config_route_v3.RouteAction_HashPolicy{
			Terminal: p.terminal,
		}
		if p.hashSourceIP {
			hp.PolicySpecifier = &envoy_config_route_v3.RouteAction_HashPolicy_ConnectionProperties_{
				ConnectionProperties: &envoy_config_route_v3.RouteAction_HashPolicy_ConnectionProperties{
					SourceIp: true,
				},
			}
		}
		if len(p.headerName) > 0 {
			hp.PolicySpecifier = &envoy_config_route_v3.RouteAction_HashPolicy_Header_{
				Header: &envoy_config_route_v3.RouteAction_HashPolicy_Header{
					HeaderName: p.headerName,
				},
			}
		}
		if len(p.parameterName) > 0 {
			hp.PolicySpecifier = &envoy_config_route_v3.RouteAction_HashPolicy_QueryParameter_{
				QueryParameter: &envoy_config_route_v3.RouteAction_HashPolicy_QueryParameter{
					Name: p.parameterName,
				},
			}
		}
		route.Route.HashPolicy = append(route.Route.HashPolicy, hp)
	}
	return route
}

func withRedirect() *envoy_config_route_v3.Route_Redirect {
	return &envoy_config_route_v3.Route_Redirect{
		Redirect: &envoy_config_route_v3.RedirectAction{
			SchemeRewriteSpecifier: &envoy_config_route_v3.RedirectAction_HttpsRedirect{
				HttpsRedirect: true,
			},
		},
	}
}

func withFilterConfig(name string, message proto.Message) map[string]*anypb.Any {
	return map[string]*anypb.Any{
		name: protobuf.MustMarshalAny(message),
	}
}

type weightedCluster struct {
	name   string
	weight uint32
}

func routeWeightedCluster(clusters ...weightedCluster) *envoy_config_route_v3.Route_Route {
	return &envoy_config_route_v3.Route_Route{
		Route: &envoy_config_route_v3.RouteAction{
			ClusterSpecifier: &envoy_config_route_v3.RouteAction_WeightedClusters{
				WeightedClusters: weightedClusters(clusters),
			},
		},
	}
}

func weightedClusters(clusters []weightedCluster) *envoy_config_route_v3.WeightedCluster {
	var wc envoy_config_route_v3.WeightedCluster
	for _, c := range clusters {
		wc.Clusters = append(wc.Clusters, &envoy_config_route_v3.WeightedCluster_ClusterWeight{
			Name:   c.name,
			Weight: wrapperspb.UInt32(c.weight),
		})
	}
	return &wc
}

// appendFilterChains is a helper to turn variadic FilterChain arguments into the corresponding  slice.
func appendFilterChains(chains ...*envoy_config_listener_v3.FilterChain) []*envoy_config_listener_v3.FilterChain {
	return chains
}

// filterchaintls returns a FilterChain wrapping the given virtual host.
func filterchaintls(domain string, secret *core_v1.Secret, filter *envoy_config_listener_v3.Filter, peerValidationContext *dag.PeerValidationContext, alpn ...string) *envoy_config_listener_v3.FilterChain {
	return envoy_v3.FilterChainTLS(
		domain,
		envoy_v3.NewEnvoyGen(envoy_v3.EnvoyGenOpt{
			XDSClusterName: envoy_v3.DefaultXDSClusterName,
		}).DownstreamTLSContext(
			&dag.Secret{Object: secret},
			envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2,
			envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3,
			nil,
			peerValidationContext,
			alpn...),
		envoy_v3.Filters(filter),
	)
}

// filterchaintlsfallback returns a FilterChain for the given TLS fallback certificate.
func filterchaintlsfallback(fallbackSecret *core_v1.Secret, peerValidationContext *dag.PeerValidationContext, alpn ...string) *envoy_config_listener_v3.FilterChain {
	envoyGen := envoy_v3.NewEnvoyGen(envoy_v3.EnvoyGenOpt{
		XDSClusterName: envoy_v3.DefaultXDSClusterName,
	})
	return envoy_v3.FilterChainTLSFallback(
		envoyGen.DownstreamTLSContext(
			&dag.Secret{Object: fallbackSecret},
			envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2,
			envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3,
			nil,
			peerValidationContext,
			alpn...),
		envoy_v3.Filters(
			envoyGen.HTTPConnectionManagerBuilder().
				DefaultFilters().
				RouteConfigName(xdscache_v3.ENVOY_FALLBACK_ROUTECONFIG).
				MetricsPrefix(xdscache_v3.ENVOY_HTTPS_LISTENER).
				AccessLoggers(envoy_v3.FileAccessLogEnvoy("/dev/stdout", "", nil, contour_v1alpha1.LogLevelInfo)).
				Get(),
		),
	)
}

// filterchaintlsfallbackauthz does same thing as filterchaintlsfallback but inserts a
// `ext_authz` filter with the specified configuration into the filter chain.
func filterchaintlsfallbackauthz(fallbackSecret *core_v1.Secret, authz *envoy_filter_http_ext_authz_v3.ExtAuthz, peerValidationContext *dag.PeerValidationContext, alpn ...string) *envoy_config_listener_v3.FilterChain {
	envoyGen := envoy_v3.NewEnvoyGen(envoy_v3.EnvoyGenOpt{
		XDSClusterName: envoy_v3.DefaultXDSClusterName,
	})
	return envoy_v3.FilterChainTLSFallback(
		envoyGen.DownstreamTLSContext(
			&dag.Secret{Object: fallbackSecret},
			envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2,
			envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3,
			nil,
			peerValidationContext,
			alpn...),
		envoy_v3.Filters(
			envoyGen.HTTPConnectionManagerBuilder().
				DefaultFilters().
				AddFilter(&envoy_filter_network_http_connection_manager_v3.HttpFilter{
					Name: envoy_v3.ExtAuthzFilterName,
					ConfigType: &envoy_filter_network_http_connection_manager_v3.HttpFilter_TypedConfig{
						TypedConfig: protobuf.MustMarshalAny(authz),
					},
				}).
				RouteConfigName(xdscache_v3.ENVOY_FALLBACK_ROUTECONFIG).
				MetricsPrefix(xdscache_v3.ENVOY_HTTPS_LISTENER).
				AccessLoggers(envoy_v3.FileAccessLogEnvoy("/dev/stdout", "", nil, contour_v1alpha1.LogLevelInfo)).
				Get(),
		),
	)
}

func httpsFilterFor(vhost string) *envoy_config_listener_v3.Filter {
	envoyGen := envoy_v3.NewEnvoyGen(envoy_v3.EnvoyGenOpt{
		XDSClusterName: envoy_v3.DefaultXDSClusterName,
	})
	return envoyGen.HTTPConnectionManagerBuilder().
		AddFilter(envoy_v3.FilterMisdirectedRequests(vhost)).
		DefaultFilters().
		RouteConfigName(path.Join("https", vhost)).
		MetricsPrefix(xdscache_v3.ENVOY_HTTPS_LISTENER).
		AccessLoggers(envoy_v3.FileAccessLogEnvoy("/dev/stdout", "", nil, contour_v1alpha1.LogLevelInfo)).
		Get()
}

func httpFilterForGateway() *envoy_config_listener_v3.Filter {
	envoyGen := envoy_v3.NewEnvoyGen(envoy_v3.EnvoyGenOpt{
		XDSClusterName: envoy_v3.DefaultXDSClusterName,
	})
	return envoyGen.HTTPConnectionManagerBuilder().
		DefaultFilters().
		RouteConfigName("http-80").
		AccessLoggers(envoy_v3.FileAccessLogEnvoy("/dev/stdout", "", nil, contour_v1alpha1.LogLevelInfo)).
		EnableWebsockets(true).
		Get()
}

func httpsFilterForGateway(listener, vhost string) *envoy_config_listener_v3.Filter {
	envoyGen := envoy_v3.NewEnvoyGen(envoy_v3.EnvoyGenOpt{
		XDSClusterName: envoy_v3.DefaultXDSClusterName,
	})
	return envoyGen.HTTPConnectionManagerBuilder().
		AddFilter(envoy_v3.FilterMisdirectedRequests(vhost)).
		DefaultFilters().
		RouteConfigName(path.Join(listener, vhost)).
		MetricsPrefix(listener).
		AccessLoggers(envoy_v3.FileAccessLogEnvoy("/dev/stdout", "", nil, contour_v1alpha1.LogLevelInfo)).
		EnableWebsockets(true).
		Get()
}

// httpsFilterWithXfccFor does the same as httpsFilterFor but enable
// client certs details forwarding
func httpsFilterWithXfccFor(vhost string, d *dag.ClientCertificateDetails) *envoy_config_listener_v3.Filter {
	envoyGen := envoy_v3.NewEnvoyGen(envoy_v3.EnvoyGenOpt{
		XDSClusterName: envoy_v3.DefaultXDSClusterName,
	})
	return envoyGen.HTTPConnectionManagerBuilder().
		AddFilter(envoy_v3.FilterMisdirectedRequests(vhost)).
		DefaultFilters().
		RouteConfigName(path.Join("https", vhost)).
		MetricsPrefix(xdscache_v3.ENVOY_HTTPS_LISTENER).
		AccessLoggers(envoy_v3.FileAccessLogEnvoy("/dev/stdout", "", nil, contour_v1alpha1.LogLevelInfo)).
		ForwardClientCertificate(d).
		Get()
}

// authzFilterFor does the same as httpsFilterFor but inserts a
// `ext_authz` filter with the specified configuration into the
// filter chain.
func authzFilterFor(
	vhost string,
	authz *envoy_filter_http_ext_authz_v3.ExtAuthz,
) *envoy_config_listener_v3.Filter {
	envoyGen := envoy_v3.NewEnvoyGen(envoy_v3.EnvoyGenOpt{
		XDSClusterName: envoy_v3.DefaultXDSClusterName,
	})
	return envoyGen.HTTPConnectionManagerBuilder().
		AddFilter(envoy_v3.FilterMisdirectedRequests(vhost)).
		DefaultFilters().
		AddFilter(&envoy_filter_network_http_connection_manager_v3.HttpFilter{
			Name: envoy_v3.ExtAuthzFilterName,
			ConfigType: &envoy_filter_network_http_connection_manager_v3.HttpFilter_TypedConfig{
				TypedConfig: protobuf.MustMarshalAny(authz),
			},
		}).
		RouteConfigName(path.Join("https", vhost)).
		MetricsPrefix(xdscache_v3.ENVOY_HTTPS_LISTENER).
		AccessLoggers(envoy_v3.FileAccessLogEnvoy("/dev/stdout", "", nil, contour_v1alpha1.LogLevelInfo)).
		Get()
}

func jwtAuthnFilterFor(
	vhost string,
	jwt *envoy_filter_http_jwt_authn_v3.JwtAuthentication,
) *envoy_config_listener_v3.Filter {
	envoyGen := envoy_v3.NewEnvoyGen(envoy_v3.EnvoyGenOpt{
		XDSClusterName: envoy_v3.DefaultXDSClusterName,
	})
	return envoyGen.HTTPConnectionManagerBuilder().
		AddFilter(envoy_v3.FilterMisdirectedRequests(vhost)).
		DefaultFilters().
		AddFilter(&envoy_filter_network_http_connection_manager_v3.HttpFilter{
			Name: envoy_v3.JWTAuthnFilterName,
			ConfigType: &envoy_filter_network_http_connection_manager_v3.HttpFilter_TypedConfig{
				TypedConfig: protobuf.MustMarshalAny(jwt),
			},
		}).
		RouteConfigName(path.Join("https", vhost)).
		MetricsPrefix(xdscache_v3.ENVOY_HTTPS_LISTENER).
		AccessLoggers(envoy_v3.FileAccessLogEnvoy("/dev/stdout", "", nil, contour_v1alpha1.LogLevelInfo)).
		Get()
}

func tcpproxy(statPrefix, cluster string) *envoy_config_listener_v3.Filter {
	return &envoy_config_listener_v3.Filter{
		Name: wellknown.TCPProxy,
		ConfigType: &envoy_config_listener_v3.Filter_TypedConfig{
			TypedConfig: protobuf.MustMarshalAny(&envoy_filter_network_tcp_proxy_v3.TcpProxy{
				StatPrefix: statPrefix,
				ClusterSpecifier: &envoy_filter_network_tcp_proxy_v3.TcpProxy_Cluster{
					Cluster: cluster,
				},
				AccessLog:   envoy_v3.FileAccessLogEnvoy("/dev/stdout", "", nil, contour_v1alpha1.LogLevelInfo),
				IdleTimeout: durationpb.New(9001 * time.Second),
			}),
		},
	}
}

type clusterWeight struct {
	name   string
	weight uint32
}

func tcpproxyWeighted(statPrefix string, clusters ...clusterWeight) *envoy_config_listener_v3.Filter {
	weightedClusters := &envoy_filter_network_tcp_proxy_v3.TcpProxy_WeightedCluster{}
	for _, clusterWeight := range clusters {
		weightedClusters.Clusters = append(weightedClusters.Clusters, &envoy_filter_network_tcp_proxy_v3.TcpProxy_WeightedCluster_ClusterWeight{
			Name:   clusterWeight.name,
			Weight: clusterWeight.weight,
		})
	}

	return &envoy_config_listener_v3.Filter{
		Name: wellknown.TCPProxy,
		ConfigType: &envoy_config_listener_v3.Filter_TypedConfig{
			TypedConfig: protobuf.MustMarshalAny(&envoy_filter_network_tcp_proxy_v3.TcpProxy{
				StatPrefix: statPrefix,
				ClusterSpecifier: &envoy_filter_network_tcp_proxy_v3.TcpProxy_WeightedClusters{
					WeightedClusters: weightedClusters,
				},
				AccessLog:   envoy_v3.FileAccessLogEnvoy("/dev/stdout", "", nil, contour_v1alpha1.LogLevelInfo),
				IdleTimeout: durationpb.New(9001 * time.Second),
			}),
		},
	}
}

func statsListener() *envoy_config_listener_v3.Listener {
	// Single listener with metrics and health endpoints.
	listeners := envoy_v3.NewEnvoyGen(envoy_v3.EnvoyGenOpt{
		XDSClusterName: envoy_v3.DefaultXDSClusterName,
	}).StatsListeners(
		contour_v1alpha1.MetricsConfig{Address: "0.0.0.0", Port: 8002},
		contour_v1alpha1.HealthConfig{Address: "0.0.0.0", Port: 8002})
	return listeners[0]
}

func envoyAdminListener(port int) *envoy_config_listener_v3.Listener {
	return envoy_v3.AdminListener(port)
}

func defaultHTTPListener() *envoy_config_listener_v3.Listener {
	envoyGen := envoy_v3.NewEnvoyGen(envoy_v3.EnvoyGenOpt{
		XDSClusterName: envoy_v3.DefaultXDSClusterName,
	})
	return &envoy_config_listener_v3.Listener{
		Name:    "ingress_http",
		Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
		FilterChains: envoy_v3.FilterChains(
			envoyGen.HTTPConnectionManager("ingress_http", envoy_v3.FileAccessLogEnvoy("/dev/stdout", "", nil, contour_v1alpha1.LogLevelInfo), 0),
		),
		SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
	}
}
