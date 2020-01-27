// Copyright © 2019 VMware
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
	"testing"
	"time"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_api_v2_auth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	envoy_api_v2_core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoy_api_v2_listener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	envoy_api_v2_route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	envoy_config_v2_tcpproxy "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/tcp_proxy/v2"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/proto"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/envoy"
	"github.com/projectcontour/contour/internal/protobuf"
	v1 "k8s.io/api/core/v1"
)

// DefaultCluster returns a copy of c, updated with default values.
func DefaultCluster(c *v2.Cluster) *v2.Cluster {
	// NOTE: Keep this in sync with envoy.defaultCluster().
	defaults := &v2.Cluster{
		ConnectTimeout: protobuf.Duration(250 * time.Millisecond),
		LbPolicy:       v2.Cluster_ROUND_ROBIN,
		CommonLbConfig: envoy.ClusterCommonLBConfig(),
	}

	proto.Merge(defaults, c)
	return defaults
}

func externalNameCluster(name, servicename, statName, externalName string, port int) *v2.Cluster {
	return DefaultCluster(&v2.Cluster{
		Name:                 name,
		ClusterDiscoveryType: envoy.ClusterDiscoveryType(v2.Cluster_STRICT_DNS),
		AltStatName:          statName,
		LoadAssignment: &v2.ClusterLoadAssignment{
			ClusterName: servicename,
			Endpoints: envoy.Endpoints(
				envoy.SocketAddress(externalName, port),
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

func routePrefix(prefix string, headers ...dag.HeaderCondition) *envoy_api_v2_route.RouteMatch {
	return envoy.RouteMatch(&dag.Route{
		PathCondition: &dag.PrefixCondition{
			Prefix: prefix,
		},
		HeaderConditions: headers,
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
		Action: envoy.UpgradeHTTPS(),
	}
}

func cluster(name, servicename, statName string) *v2.Cluster {
	return DefaultCluster(&v2.Cluster{
		Name:                 name,
		ClusterDiscoveryType: envoy.ClusterDiscoveryType(v2.Cluster_EDS),
		AltStatName:          statName,
		EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
			EdsConfig:   envoy.ConfigSource("contour"),
			ServiceName: servicename,
		},
	})
}

func tlsCluster(c *v2.Cluster, ca []byte, subjectName string, sni string, alpnProtocols ...string) *v2.Cluster {
	c.TransportSocket = envoy.UpstreamTLSTransportSocket(
		envoy.UpstreamTLSContext(ca, subjectName, sni, alpnProtocols...),
	)
	return c
}

func h2cCluster(c *v2.Cluster) *v2.Cluster {
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

func filterchaintls(domain string, secret *v1.Secret, filter *envoy_api_v2_listener.Filter, alpn ...string) []*envoy_api_v2_listener.FilterChain {
	return []*envoy_api_v2_listener.FilterChain{
		envoy.FilterChainTLS(
			domain,
			&dag.Secret{Object: secret},
			envoy.Filters(filter),
			envoy_api_v2_auth.TlsParameters_TLSv1_1,
			alpn...,
		),
	}
}

func tcpproxy(t *testing.T, statPrefix, cluster string) *envoy_api_v2_listener.Filter {
	return &envoy_api_v2_listener.Filter{
		Name: wellknown.TCPProxy,
		ConfigType: &envoy_api_v2_listener.Filter_TypedConfig{
			TypedConfig: toAny(t, &envoy_config_v2_tcpproxy.TcpProxy{
				StatPrefix: statPrefix,
				ClusterSpecifier: &envoy_config_v2_tcpproxy.TcpProxy_Cluster{
					Cluster: cluster,
				},
				AccessLog:   envoy.FileAccessLogEnvoy("/dev/stdout"),
				IdleTimeout: protobuf.Duration(9001 * time.Second),
			}),
		},
	}
}

func staticListener() *v2.Listener {
	return envoy.StatsListener("0.0.0.0", 8002)
}
