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
	"net"
	"strings"
	"time"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_upstream_http_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	envoy_type_v3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"k8s.io/apimachinery/pkg/types"

	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/envoy"
	"github.com/projectcontour/contour/internal/protobuf"
	"github.com/projectcontour/contour/internal/timeout"
	"github.com/projectcontour/contour/internal/xds"
)

func clusterDefaults() *envoy_config_cluster_v3.Cluster {
	return &envoy_config_cluster_v3.Cluster{
		ConnectTimeout: durationpb.New(2 * time.Second),
		CommonLbConfig: ClusterCommonLBConfig(),
		LbPolicy:       lbPolicy(dag.LoadBalancerPolicyRoundRobin),
	}
}

// Cluster creates new envoy_config_cluster_v3.Cluster from dag.Cluster.
func (e *EnvoyGen) Cluster(c *dag.Cluster) *envoy_config_cluster_v3.Cluster {
	service := c.Upstream
	cluster := clusterDefaults()

	cluster.Name = envoy.Clustername(c)
	cluster.AltStatName = envoy.AltStatName(service)
	cluster.LbPolicy = lbPolicy(c.LoadBalancerPolicy)
	cluster.HealthChecks = edshealthcheck(c)
	cluster.DnsLookupFamily = parseDNSLookupFamily(c.DNSLookupFamily)

	if c.PerConnectionBufferLimitBytes != nil {
		cluster.PerConnectionBufferLimitBytes = protobuf.UInt32OrNil(*c.PerConnectionBufferLimitBytes)
	}

	switch len(service.ExternalName) {
	case 0:
		// external name not set, cluster will be discovered via EDS
		cluster.ClusterDiscoveryType = ClusterDiscoveryType(envoy_config_cluster_v3.Cluster_EDS)
		cluster.EdsClusterConfig = e.edsconfig(service)
	default:
		// external name set, use hard coded DNS name
		// external name set to LOGICAL_DNS when user selects the ALL loookup family
		clusterDiscoveryType := ClusterDiscoveryType(envoy_config_cluster_v3.Cluster_STRICT_DNS)
		if cluster.DnsLookupFamily == envoy_config_cluster_v3.Cluster_ALL {
			clusterDiscoveryType = ClusterDiscoveryType(envoy_config_cluster_v3.Cluster_LOGICAL_DNS)
		}

		cluster.ClusterDiscoveryType = clusterDiscoveryType
		cluster.LoadAssignment = externalNameClusterLoadAssignment(service)
	}

	// Drain connections immediately if using healthchecks and the endpoint is known to be removed
	if c.HTTPHealthCheckPolicy != nil || c.TCPHealthCheckPolicy != nil {
		cluster.IgnoreHealthOnHostRemoval = true
	}

	applyCircuitBreakers(cluster, service.CircuitBreakers)

	httpVersion := HTTPVersionAuto
	switch c.Protocol {
	case "tls":
		cluster.TransportSocket = UpstreamTLSTransportSocket(
			e.UpstreamTLSContext(
				c.UpstreamValidation,
				c.SNI,
				c.ClientCertificate,
				c.UpstreamTLS,
			),
		)
	case "h2":
		httpVersion = HTTPVersion2
		cluster.TransportSocket = UpstreamTLSTransportSocket(
			e.UpstreamTLSContext(
				c.UpstreamValidation,
				c.SNI,
				c.ClientCertificate,
				c.UpstreamTLS,
				"h2",
			),
		)
	case "h2c":
		httpVersion = HTTPVersion2
	}

	if c.TimeoutPolicy.ConnectTimeout > time.Duration(0) {
		cluster.ConnectTimeout = durationpb.New(c.TimeoutPolicy.ConnectTimeout)
	}

	cluster.TypedExtensionProtocolOptions = protocolOptions(httpVersion, c.TimeoutPolicy.IdleConnectionTimeout, c.MaxRequestsPerConnection)

	if c.SlowStartConfig != nil {
		switch cluster.LbPolicy {
		case envoy_config_cluster_v3.Cluster_LEAST_REQUEST:
			cluster.LbConfig = &envoy_config_cluster_v3.Cluster_LeastRequestLbConfig_{
				LeastRequestLbConfig: &envoy_config_cluster_v3.Cluster_LeastRequestLbConfig{
					SlowStartConfig: slowStartConfig(c.SlowStartConfig),
				},
			}
		case envoy_config_cluster_v3.Cluster_ROUND_ROBIN:
			cluster.LbConfig = &envoy_config_cluster_v3.Cluster_RoundRobinLbConfig_{
				RoundRobinLbConfig: &envoy_config_cluster_v3.Cluster_RoundRobinLbConfig{
					SlowStartConfig: slowStartConfig(c.SlowStartConfig),
				},
			}
		default:
			// Slow start is only supported for round robin and weighted least request.
		}
	}

	return cluster
}

// ExtensionCluster builds a envoy_config_cluster_v3.Cluster struct for the given extension service.
func (e *EnvoyGen) ExtensionCluster(ext *dag.ExtensionCluster) *envoy_config_cluster_v3.Cluster {
	cluster := clusterDefaults()

	// The Envoy cluster name has already been set.
	cluster.Name = ext.Name

	// The AltStatName was added to make a more readable alternative
	// to the cluster name for metrics (see #827). For extension
	// services, we can have multiple ports, so it doesn't make
	// sense to build this the same way we build it for HTTPProxy
	// service clusters. However, we know the namespaced name for
	// the ExtensionCluster is globally unique, so we can use that
	// to produce a stable, readable name.
	cluster.AltStatName = strings.ReplaceAll(cluster.Name, "/", "_")

	cluster.LbPolicy = lbPolicy(ext.LoadBalancerPolicy)

	// Cluster will be discovered via EDS.
	cluster.ClusterDiscoveryType = ClusterDiscoveryType(envoy_config_cluster_v3.Cluster_EDS)
	cluster.EdsClusterConfig = &envoy_config_cluster_v3.Cluster_EdsClusterConfig{
		EdsConfig:   e.GetConfigSource(),
		ServiceName: ext.Upstream.ClusterName,
	}

	// TODO(jpeach): Externalname service support in https://github.com/projectcontour/contour/issues/2875

	http2Version := HTTPVersionAuto
	switch ext.Protocol {
	case "h2":
		http2Version = HTTPVersion2
		cluster.TransportSocket = UpstreamTLSTransportSocket(
			e.UpstreamTLSContext(
				ext.UpstreamValidation,
				ext.SNI,
				ext.ClientCertificate,
				ext.UpstreamTLS,
				"h2",
			),
		)
	case "h2c":
		http2Version = HTTPVersion2
	}

	if ext.ClusterTimeoutPolicy.ConnectTimeout > time.Duration(0) {
		cluster.ConnectTimeout = durationpb.New(ext.ClusterTimeoutPolicy.ConnectTimeout)
	}
	cluster.TypedExtensionProtocolOptions = protocolOptions(http2Version, ext.ClusterTimeoutPolicy.IdleConnectionTimeout, nil)

	applyCircuitBreakers(cluster, ext.CircuitBreakers)

	return cluster
}

func applyCircuitBreakers(cluster *envoy_config_cluster_v3.Cluster, settings dag.CircuitBreakers) {
	if envoy.AnyPositive(settings.MaxConnections, settings.MaxPendingRequests, settings.MaxRequests, settings.MaxRetries, settings.PerHostMaxConnections) {
		cluster.CircuitBreakers = &envoy_config_cluster_v3.CircuitBreakers{
			Thresholds: []*envoy_config_cluster_v3.CircuitBreakers_Thresholds{{
				MaxConnections:     protobuf.UInt32OrNil(settings.MaxConnections),
				MaxPendingRequests: protobuf.UInt32OrNil(settings.MaxPendingRequests),
				MaxRequests:        protobuf.UInt32OrNil(settings.MaxRequests),
				MaxRetries:         protobuf.UInt32OrNil(settings.MaxRetries),
				TrackRemaining:     true,
			}},
			PerHostThresholds: []*envoy_config_cluster_v3.CircuitBreakers_Thresholds{{
				MaxConnections: protobuf.UInt32OrNil(settings.PerHostMaxConnections),
				TrackRemaining: true,
			}},
		}
	}
}

// DNSNameCluster builds a envoy_config_cluster_v3.Cluster for the given *dag.DNSNameCluster.
func (e *EnvoyGen) DNSNameCluster(c *dag.DNSNameCluster) *envoy_config_cluster_v3.Cluster {
	cluster := clusterDefaults()

	cluster.Name = envoy.DNSNameClusterName(c)
	cluster.DnsLookupFamily = parseDNSLookupFamily(c.DNSLookupFamily)

	clusterType := envoy_config_cluster_v3.Cluster_STRICT_DNS
	if cluster.DnsLookupFamily == envoy_config_cluster_v3.Cluster_ALL {
		clusterType = envoy_config_cluster_v3.Cluster_LOGICAL_DNS
	}
	cluster.ClusterDiscoveryType = ClusterDiscoveryType(clusterType)

	var transportSocket *envoy_config_core_v3.TransportSocket
	if c.Scheme == "https" {
		transportSocket = UpstreamTLSTransportSocket(e.UpstreamTLSContext(c.UpstreamValidation, c.Address, nil, c.UpstreamTLS))
	}

	cluster.LoadAssignment = ClusterLoadAssignment(envoy.DNSNameClusterName(c), SocketAddress(c.Address, c.Port))
	cluster.TransportSocket = transportSocket

	return cluster
}

func (e *EnvoyGen) edsconfig(service *dag.Service) *envoy_config_cluster_v3.Cluster_EdsClusterConfig {
	return &envoy_config_cluster_v3.Cluster_EdsClusterConfig{
		EdsConfig: e.GetConfigSource(),
		ServiceName: xds.ClusterLoadAssignmentName(
			types.NamespacedName{Name: service.Weighted.ServiceName, Namespace: service.Weighted.ServiceNamespace},
			service.Weighted.ServicePort.Name,
		),
	}
}

func lbPolicy(strategy string) envoy_config_cluster_v3.Cluster_LbPolicy {
	switch strategy {
	case dag.LoadBalancerPolicyWeightedLeastRequest:
		return envoy_config_cluster_v3.Cluster_LEAST_REQUEST
	case dag.LoadBalancerPolicyRandom:
		return envoy_config_cluster_v3.Cluster_RANDOM
	case dag.LoadBalancerPolicyCookie, dag.LoadBalancerPolicyRequestHash:
		return envoy_config_cluster_v3.Cluster_RING_HASH
	default:
		return envoy_config_cluster_v3.Cluster_ROUND_ROBIN
	}
}

func edshealthcheck(c *dag.Cluster) []*envoy_config_core_v3.HealthCheck {
	if c.HTTPHealthCheckPolicy == nil && c.TCPHealthCheckPolicy == nil {
		return nil
	}

	if c.HTTPHealthCheckPolicy != nil {
		return []*envoy_config_core_v3.HealthCheck{
			httpHealthCheck(c),
		}
	}

	return []*envoy_config_core_v3.HealthCheck{
		tcpHealthCheck(c),
	}
}

// ClusterCommonLBConfig creates a *envoy_config_cluster_v3.Cluster_CommonLbConfig with HealthyPanicThreshold disabled.
func ClusterCommonLBConfig() *envoy_config_cluster_v3.Cluster_CommonLbConfig {
	return &envoy_config_cluster_v3.Cluster_CommonLbConfig{
		HealthyPanicThreshold: &envoy_type_v3.Percent{ // Disable HealthyPanicThreshold
			Value: 0,
		},
	}
}

// ClusterDiscoveryType returns the type of a ClusterDiscovery as a Cluster_type.
func ClusterDiscoveryType(t envoy_config_cluster_v3.Cluster_DiscoveryType) *envoy_config_cluster_v3.Cluster_Type {
	return &envoy_config_cluster_v3.Cluster_Type{Type: t}
}

// clusterDiscoveryTypeForAddress returns the type of a ClusterDiscovery as a Cluster_type.
// If the provided address is an IP, overrides the type to STATIC, otherwise uses the
// passed in type.
func clusterDiscoveryTypeForAddress(address string, t envoy_config_cluster_v3.Cluster_DiscoveryType) *envoy_config_cluster_v3.Cluster_Type {
	clusterType := t
	if net.ParseIP(address) != nil {
		clusterType = envoy_config_cluster_v3.Cluster_STATIC
	}
	return &envoy_config_cluster_v3.Cluster_Type{Type: clusterType}
}

// parseDNSLookupFamily parses the dnsLookupFamily string into a envoy_config_cluster_v3.Cluster_DnsLookupFamily
func parseDNSLookupFamily(value string) envoy_config_cluster_v3.Cluster_DnsLookupFamily {
	switch value {
	case "v4":
		return envoy_config_cluster_v3.Cluster_V4_ONLY
	case "v6":
		return envoy_config_cluster_v3.Cluster_V6_ONLY
	case "all":
		return envoy_config_cluster_v3.Cluster_ALL
	}
	return envoy_config_cluster_v3.Cluster_AUTO
}

func protocolOptions(explicitHTTPVersion HTTPVersionType, idleConnectionTimeout timeout.Setting, maxRequestsPerConnection *uint32) map[string]*anypb.Any {
	// Keep Envoy defaults by not setting protocol options at all if not necessary.
	if explicitHTTPVersion == HTTPVersionAuto && idleConnectionTimeout.UseDefault() && maxRequestsPerConnection == nil {
		return nil
	}

	options := envoy_upstream_http_v3.HttpProtocolOptions{}

	switch explicitHTTPVersion {
	// Default protocol version in Envoy is HTTP1.1.
	case HTTPVersion1, HTTPVersionAuto:
		options.UpstreamProtocolOptions = &envoy_upstream_http_v3.HttpProtocolOptions_ExplicitHttpConfig_{
			ExplicitHttpConfig: &envoy_upstream_http_v3.HttpProtocolOptions_ExplicitHttpConfig{
				ProtocolConfig: &envoy_upstream_http_v3.HttpProtocolOptions_ExplicitHttpConfig_HttpProtocolOptions{},
			},
		}
	case HTTPVersion2:
		options.UpstreamProtocolOptions = &envoy_upstream_http_v3.HttpProtocolOptions_ExplicitHttpConfig_{
			ExplicitHttpConfig: &envoy_upstream_http_v3.HttpProtocolOptions_ExplicitHttpConfig{
				ProtocolConfig: &envoy_upstream_http_v3.HttpProtocolOptions_ExplicitHttpConfig_Http2ProtocolOptions{},
			},
		}
	case HTTPVersion3:
		options.UpstreamProtocolOptions = &envoy_upstream_http_v3.HttpProtocolOptions_ExplicitHttpConfig_{
			ExplicitHttpConfig: &envoy_upstream_http_v3.HttpProtocolOptions_ExplicitHttpConfig{
				ProtocolConfig: &envoy_upstream_http_v3.HttpProtocolOptions_ExplicitHttpConfig_Http3ProtocolOptions{},
			},
		}
	}

	if !idleConnectionTimeout.UseDefault() || maxRequestsPerConnection != nil {
		commonHTTPProtocolOptions := &envoy_config_core_v3.HttpProtocolOptions{}

		if !idleConnectionTimeout.UseDefault() {
			commonHTTPProtocolOptions.IdleTimeout = durationpb.New(idleConnectionTimeout.Duration())
		}

		if maxRequestsPerConnection != nil {
			commonHTTPProtocolOptions.MaxRequestsPerConnection = wrapperspb.UInt32(*maxRequestsPerConnection)
		}

		options.CommonHttpProtocolOptions = commonHTTPProtocolOptions
	}

	return map[string]*anypb.Any{
		"envoy.extensions.upstreams.http.v3.HttpProtocolOptions": protobuf.MustMarshalAny(&options),
	}
}

// slowStartConfig returns the slow start configuration.
func slowStartConfig(slowStartConfig *dag.SlowStartConfig) *envoy_config_cluster_v3.Cluster_SlowStartConfig {
	return &envoy_config_cluster_v3.Cluster_SlowStartConfig{
		SlowStartWindow: durationpb.New(slowStartConfig.Window),
		Aggression: &envoy_config_core_v3.RuntimeDouble{
			DefaultValue: slowStartConfig.Aggression,
			RuntimeKey:   "contour.slowstart.aggression",
		},
		MinWeightPercent: &envoy_type_v3.Percent{
			Value: float64(slowStartConfig.MinWeightPercent),
		},
	}
}
