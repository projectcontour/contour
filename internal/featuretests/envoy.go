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

package featuretests

// envoy helpers

import (
	"path"
	"time"

	envoy_api_v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_api_v2_auth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	envoy_api_v2_core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoy_api_v2_listener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	envoy_api_v2_route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	envoy_config_filter_http_ext_authz_v2 "github.com/envoyproxy/go-control-plane/envoy/config/filter/http/ext_authz/v2"
	http "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	envoy_config_v2_tcpproxy "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/tcp_proxy/v2"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/projectcontour/contour/internal/contour"
	"github.com/projectcontour/contour/internal/dag"
	envoyv2 "github.com/projectcontour/contour/internal/envoy/v2"
	"github.com/projectcontour/contour/internal/protobuf"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DefaultCluster returns a copy of the default Cluster, with each
// Cluster given in the parameter slice merged on top. This makes it
// relatively fluent to compose Clusters by tweaking a few fields.
func DefaultCluster(clusters ...*envoy_api_v2.Cluster) *envoy_api_v2.Cluster {
	// NOTE: Keep this in sync with envoy.defaultCluster().
	defaults := &envoy_api_v2.Cluster{
		ConnectTimeout: protobuf.Duration(250 * time.Millisecond),
		LbPolicy:       envoy_api_v2.Cluster_ROUND_ROBIN,
		CommonLbConfig: envoyv2.ClusterCommonLBConfig(),
	}

	for _, c := range clusters {
		proto.Merge(defaults, c)
	}

	return defaults
}

func clusterWithHealthCheck(name, servicename, statName, healthCheckPath string, drainConnOnHostRemoval bool) *envoy_api_v2.Cluster {
	c := cluster(name, servicename, statName)
	c.HealthChecks = []*envoy_api_v2_core.HealthCheck{{
		Timeout:            protobuf.Duration(2 * time.Second),
		Interval:           protobuf.Duration(10 * time.Second),
		UnhealthyThreshold: protobuf.UInt32(3),
		HealthyThreshold:   protobuf.UInt32(2),
		HealthChecker: &envoy_api_v2_core.HealthCheck_HttpHealthCheck_{
			HttpHealthCheck: &envoy_api_v2_core.HealthCheck_HttpHealthCheck{
				Host: "contour-envoy-healthcheck",
				Path: healthCheckPath,
			},
		},
	}}
	c.DrainConnectionsOnHostRemoval = drainConnOnHostRemoval
	return c
}

func externalNameCluster(name, servicename, statName, externalName string, port int) *envoy_api_v2.Cluster {
	return DefaultCluster(&envoy_api_v2.Cluster{
		Name:                 name,
		ClusterDiscoveryType: envoyv2.ClusterDiscoveryType(envoy_api_v2.Cluster_STRICT_DNS),
		AltStatName:          statName,
		LoadAssignment: &envoy_api_v2.ClusterLoadAssignment{
			ClusterName: servicename,
			Endpoints: envoyv2.Endpoints(
				envoyv2.SocketAddress(externalName, port),
			),
		},
	})
}

func routeCluster(cluster string) *envoy_api_v2_route.Route_Route {
	return &envoy_api_v2_route.Route_Route{
		Route: &envoy_api_v2_route.RouteAction{
			ClusterSpecifier: &envoy_api_v2_route.RouteAction_Cluster{
				Cluster: cluster,
			},
		},
	}
}

func routePrefix(prefix string, headers ...dag.HeaderMatchCondition) *envoy_api_v2_route.RouteMatch {
	return envoyv2.RouteMatch(&dag.Route{
		PathMatchCondition: &dag.PrefixMatchCondition{
			Prefix: prefix,
		},
		HeaderMatchConditions: headers,
	})
}

func routeHostRewrite(cluster, newHostName string) *envoy_api_v2_route.Route_Route {
	return &envoy_api_v2_route.Route_Route{
		Route: &envoy_api_v2_route.RouteAction{
			ClusterSpecifier:     &envoy_api_v2_route.RouteAction_Cluster{Cluster: cluster},
			HostRewriteSpecifier: &envoy_api_v2_route.RouteAction_HostRewrite{HostRewrite: newHostName},
		},
	}
}

func upgradeHTTPS(match *envoy_api_v2_route.RouteMatch) *envoy_api_v2_route.Route {
	return &envoy_api_v2_route.Route{
		Match:  match,
		Action: envoyv2.UpgradeHTTPS(),
	}
}

func cluster(name, servicename, statName string) *envoy_api_v2.Cluster {
	return DefaultCluster(&envoy_api_v2.Cluster{
		Name:                 name,
		ClusterDiscoveryType: envoyv2.ClusterDiscoveryType(envoy_api_v2.Cluster_EDS),
		AltStatName:          statName,
		EdsClusterConfig: &envoy_api_v2.Cluster_EdsClusterConfig{
			EdsConfig:   envoyv2.ConfigSource("contour"),
			ServiceName: servicename,
		},
	})
}

func tlsCluster(c *envoy_api_v2.Cluster, ca []byte, subjectName string, sni string, alpnProtocols ...string) *envoy_api_v2.Cluster {
	c.TransportSocket = envoyv2.UpstreamTLSTransportSocket(
		envoyv2.UpstreamTLSContext(
			&dag.PeerValidationContext{
				CACertificate: &dag.Secret{Object: &v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret",
						Namespace: "default",
					},
					Type: "kubernetes.io/tls",
					Data: map[string][]byte{dag.CACertificateKey: ca},
				}},
				SubjectName: subjectName},
			sni,
			alpnProtocols...,
		),
	)
	return c
}

func h2cCluster(c *envoy_api_v2.Cluster) *envoy_api_v2.Cluster {
	c.Http2ProtocolOptions = &envoy_api_v2_core.Http2ProtocolOptions{}
	return c
}

func withResponseTimeout(route *envoy_api_v2_route.Route_Route, timeout time.Duration) *envoy_api_v2_route.Route_Route {
	route.Route.Timeout = protobuf.Duration(timeout)
	return route
}

func withIdleTimeout(route *envoy_api_v2_route.Route_Route, timeout time.Duration) *envoy_api_v2_route.Route_Route {
	route.Route.IdleTimeout = protobuf.Duration(timeout)
	return route
}

func withMirrorPolicy(route *envoy_api_v2_route.Route_Route, mirror string) *envoy_api_v2_route.Route_Route {
	route.Route.RequestMirrorPolicies = []*envoy_api_v2_route.RouteAction_RequestMirrorPolicy{{
		Cluster: mirror,
	}}
	return route
}

func withPrefixRewrite(route *envoy_api_v2_route.Route_Route, replacement string) *envoy_api_v2_route.Route_Route {
	route.Route.PrefixRewrite = replacement
	return route
}

func withRetryPolicy(route *envoy_api_v2_route.Route_Route, retryOn string, numRetries uint32, perTryTimeout time.Duration) *envoy_api_v2_route.Route_Route {
	route.Route.RetryPolicy = &envoy_api_v2_route.RetryPolicy{
		RetryOn: retryOn,
	}
	if numRetries > 0 {
		route.Route.RetryPolicy.NumRetries = protobuf.UInt32(numRetries)
	}
	if perTryTimeout > 0 {
		route.Route.RetryPolicy.PerTryTimeout = protobuf.Duration(perTryTimeout)
	}
	return route
}

func withWebsocket(route *envoy_api_v2_route.Route_Route) *envoy_api_v2_route.Route_Route {
	route.Route.UpgradeConfigs = append(route.Route.UpgradeConfigs,
		&envoy_api_v2_route.RouteAction_UpgradeConfig{
			UpgradeType: "websocket",
		},
	)
	return route
}

func withSessionAffinity(route *envoy_api_v2_route.Route_Route) *envoy_api_v2_route.Route_Route {
	route.Route.HashPolicy = append(route.Route.HashPolicy, &envoy_api_v2_route.RouteAction_HashPolicy{
		PolicySpecifier: &envoy_api_v2_route.RouteAction_HashPolicy_Cookie_{
			Cookie: &envoy_api_v2_route.RouteAction_HashPolicy_Cookie{
				Name: "X-Contour-Session-Affinity",
				Ttl:  protobuf.Duration(0),
				Path: "/",
			},
		},
	})
	return route
}

func withRedirect() *envoy_api_v2_route.Route_Redirect {
	return &envoy_api_v2_route.Route_Redirect{
		Redirect: &envoy_api_v2_route.RedirectAction{
			SchemeRewriteSpecifier: &envoy_api_v2_route.RedirectAction_HttpsRedirect{
				HttpsRedirect: true,
			},
		},
	}
}

func withFilterConfig(name string, message proto.Message) map[string]*any.Any {
	return map[string]*any.Any{
		name: protobuf.MustMarshalAny(message),
	}
}

type weightedCluster struct {
	name   string
	weight uint32
}

func routeWeightedCluster(clusters ...weightedCluster) *envoy_api_v2_route.Route_Route {
	return &envoy_api_v2_route.Route_Route{
		Route: &envoy_api_v2_route.RouteAction{
			ClusterSpecifier: &envoy_api_v2_route.RouteAction_WeightedClusters{
				WeightedClusters: weightedClusters(clusters),
			},
		},
	}
}

func weightedClusters(clusters []weightedCluster) *envoy_api_v2_route.WeightedCluster {
	var wc envoy_api_v2_route.WeightedCluster
	var total uint32
	for _, c := range clusters {
		total += c.weight
		wc.Clusters = append(wc.Clusters, &envoy_api_v2_route.WeightedCluster_ClusterWeight{
			Name:   c.name,
			Weight: protobuf.UInt32(c.weight),
		})
	}
	wc.TotalWeight = protobuf.UInt32(total)
	return &wc
}

// appendFilterChains is a helper to turn variadic FilterChain arguments into the corresponding  slice.
func appendFilterChains(chains ...*envoy_api_v2_listener.FilterChain) []*envoy_api_v2_listener.FilterChain {
	return chains
}

// filterchaintls returns a FilterChain wrapping the given virtual host.
func filterchaintls(domain string, secret *v1.Secret, filter *envoy_api_v2_listener.Filter, peerValidationContext *dag.PeerValidationContext, alpn ...string) *envoy_api_v2_listener.FilterChain {
	return envoyv2.FilterChainTLS(
		domain,
		envoyv2.DownstreamTLSContext(
			&dag.Secret{Object: secret},
			envoy_api_v2_auth.TlsParameters_TLSv1_1,
			peerValidationContext,
			alpn...),
		envoyv2.Filters(filter),
	)
}

// filterchaintlsfallback returns a FilterChain for the given TLS fallback certificate.
func filterchaintlsfallback(fallbackSecret *v1.Secret, peerValidationContext *dag.PeerValidationContext, alpn ...string) *envoy_api_v2_listener.FilterChain {
	return envoyv2.FilterChainTLSFallback(
		envoyv2.DownstreamTLSContext(
			&dag.Secret{Object: fallbackSecret},
			envoy_api_v2_auth.TlsParameters_TLSv1_1,
			peerValidationContext,
			alpn...),
		envoyv2.Filters(
			envoyv2.HTTPConnectionManagerBuilder().
				DefaultFilters().
				RouteConfigName(contour.ENVOY_FALLBACK_ROUTECONFIG).
				MetricsPrefix(contour.ENVOY_HTTPS_LISTENER).
				AccessLoggers(envoyv2.FileAccessLogEnvoy("/dev/stdout")).
				Get(),
		),
	)
}

func httpsFilterFor(vhost string) *envoy_api_v2_listener.Filter {
	return envoyv2.HTTPConnectionManagerBuilder().
		AddFilter(envoyv2.FilterMisdirectedRequests(vhost)).
		DefaultFilters().
		RouteConfigName(path.Join("https", vhost)).
		MetricsPrefix(contour.ENVOY_HTTPS_LISTENER).
		AccessLoggers(envoyv2.FileAccessLogEnvoy("/dev/stdout")).
		Get()
}

// authzFilterFor does the same as httpsFilterFor but inserts a
// `ext_authz` filter with the specified configuration into the
// filter chain.
func authzFilterFor(
	vhost string,
	authz *envoy_config_filter_http_ext_authz_v2.ExtAuthz,
) *envoy_api_v2_listener.Filter {
	return envoyv2.HTTPConnectionManagerBuilder().
		AddFilter(envoyv2.FilterMisdirectedRequests(vhost)).
		DefaultFilters().
		AddFilter(&http.HttpFilter{
			Name: "envoy.filters.http.ext_authz",
			ConfigType: &http.HttpFilter_TypedConfig{
				TypedConfig: protobuf.MustMarshalAny(authz),
			},
		}).
		RouteConfigName(path.Join("https", vhost)).
		MetricsPrefix(contour.ENVOY_HTTPS_LISTENER).
		AccessLoggers(envoyv2.FileAccessLogEnvoy("/dev/stdout")).
		Get()
}

func tcpproxy(statPrefix, cluster string) *envoy_api_v2_listener.Filter {
	return &envoy_api_v2_listener.Filter{
		Name: wellknown.TCPProxy,
		ConfigType: &envoy_api_v2_listener.Filter_TypedConfig{
			TypedConfig: protobuf.MustMarshalAny(&envoy_config_v2_tcpproxy.TcpProxy{
				StatPrefix: statPrefix,
				ClusterSpecifier: &envoy_config_v2_tcpproxy.TcpProxy_Cluster{
					Cluster: cluster,
				},
				AccessLog:   envoyv2.FileAccessLogEnvoy("/dev/stdout"),
				IdleTimeout: protobuf.Duration(9001 * time.Second),
			}),
		},
	}
}

func staticListener() *envoy_api_v2.Listener {
	return envoyv2.StatsListener("0.0.0.0", 8002)
}

func defaultHTTPListener() *envoy_api_v2.Listener {
	return &envoy_api_v2.Listener{
		Name:    "ingress_http",
		Address: envoyv2.SocketAddress("0.0.0.0", 8080),
		FilterChains: envoyv2.FilterChains(
			envoyv2.HTTPConnectionManager("ingress_http", envoyv2.FileAccessLogEnvoy("/dev/stdout"), 0),
		),
		SocketOptions: envoyv2.TCPKeepaliveSocketOptions(),
	}
}
