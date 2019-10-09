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
	"time"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_api_v2_auth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	envoy_api_v2_core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoy_api_v2_listener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	envoy_api_v2_route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/envoy"
	"github.com/projectcontour/contour/internal/protobuf"
	v1 "k8s.io/api/core/v1"
)

func routeCluster(cluster string) *envoy_api_v2_route.Route_Route {
	return &envoy_api_v2_route.Route_Route{
		Route: &envoy_api_v2_route.RouteAction{
			ClusterSpecifier: &envoy_api_v2_route.RouteAction_Cluster{
				Cluster: cluster,
			},
		},
	}
}

func cluster(name, servicename, statName string) *v2.Cluster {
	return &v2.Cluster{
		Name:                 name,
		ClusterDiscoveryType: envoy.ClusterDiscoveryType(v2.Cluster_EDS),
		AltStatName:          statName,
		EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
			EdsConfig:   envoy.ConfigSource("contour"),
			ServiceName: servicename,
		},
		ConnectTimeout: protobuf.Duration(250 * time.Millisecond),
		LbPolicy:       v2.Cluster_ROUND_ROBIN,
		CommonLbConfig: envoy.ClusterCommonLBConfig(),
	}
}

func tlsCluster(c *v2.Cluster, ca []byte, subjectName string, alpnProtocols ...string) *v2.Cluster {
	c.TlsContext = envoy.UpstreamTLSContext(ca, subjectName, alpnProtocols...)
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
	route.Route.RequestMirrorPolicy = &envoy_api_v2_route.RouteAction_RequestMirrorPolicy{
		Cluster: mirror,
	}
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

func filterchaintls(domain string, secret *v1.Secret, filter *envoy_api_v2_listener.Filter, alpn ...string) []*envoy_api_v2_listener.FilterChain {
	return []*envoy_api_v2_listener.FilterChain{
		envoy.FilterChainTLS(
			domain,
			&dag.Secret{Object: secret},
			[]*envoy_api_v2_listener.Filter{
				filter,
			},
			envoy_api_v2_auth.TlsParameters_TLSv1_1,
			alpn...,
		),
	}
}

func staticListener() *v2.Listener {
	return envoy.StatsListener("0.0.0.0", 8002)
}
