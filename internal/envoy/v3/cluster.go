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

	envoy_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoy_extensions_upstream_http_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	envoy_type "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/envoy"
	"github.com/projectcontour/contour/internal/protobuf"
	"github.com/projectcontour/contour/internal/timeout"
	"github.com/projectcontour/contour/internal/xds"
	"k8s.io/apimachinery/pkg/types"
)

func clusterDefaults() *envoy_cluster_v3.Cluster {
	return &envoy_cluster_v3.Cluster{
		ConnectTimeout: protobuf.Duration(2 * time.Second),
		CommonLbConfig: ClusterCommonLBConfig(),
		LbPolicy:       lbPolicy(dag.LoadBalancerPolicyRoundRobin),
	}
}

// Cluster creates new envoy_cluster_v3.Cluster from dag.Cluster.
func Cluster(c *dag.Cluster) *envoy_cluster_v3.Cluster {
	service := c.Upstream
	cluster := clusterDefaults()

	cluster.Name = envoy.Clustername(c)
	cluster.AltStatName = envoy.AltStatName(service)
	cluster.LbPolicy = lbPolicy(c.LoadBalancerPolicy)
	cluster.HealthChecks = edshealthcheck(c)
	cluster.DnsLookupFamily = parseDNSLookupFamily(c.DNSLookupFamily)

	switch len(service.ExternalName) {
	case 0:
		// external name not set, cluster will be discovered via EDS
		cluster.ClusterDiscoveryType = ClusterDiscoveryType(envoy_cluster_v3.Cluster_EDS)
		cluster.EdsClusterConfig = edsconfig("contour", service)
	default:
		// external name set, use hard coded DNS name
		cluster.ClusterDiscoveryType = ClusterDiscoveryType(envoy_cluster_v3.Cluster_STRICT_DNS)
		cluster.LoadAssignment = StaticClusterLoadAssignment(service)
	}

	// Drain connections immediately if using healthchecks and the endpoint is known to be removed
	if c.HTTPHealthCheckPolicy != nil || c.TCPHealthCheckPolicy != nil {
		cluster.IgnoreHealthOnHostRemoval = true
	}

	if envoy.AnyPositive(service.MaxConnections, service.MaxPendingRequests, service.MaxRequests, service.MaxRetries) {
		cluster.CircuitBreakers = &envoy_cluster_v3.CircuitBreakers{
			Thresholds: []*envoy_cluster_v3.CircuitBreakers_Thresholds{{
				MaxConnections:     protobuf.UInt32OrNil(service.MaxConnections),
				MaxPendingRequests: protobuf.UInt32OrNil(service.MaxPendingRequests),
				MaxRequests:        protobuf.UInt32OrNil(service.MaxRequests),
				MaxRetries:         protobuf.UInt32OrNil(service.MaxRetries),
			}},
		}
	}

	httpVersion := HTTPVersionAuto
	switch c.Protocol {
	case "tls":
		cluster.TransportSocket = UpstreamTLSTransportSocket(
			UpstreamTLSContext(
				c.UpstreamValidation,
				c.SNI,
				c.ClientCertificate,
			),
		)
	case "h2":
		httpVersion = HTTPVersion2
		cluster.TransportSocket = UpstreamTLSTransportSocket(
			UpstreamTLSContext(
				c.UpstreamValidation,
				c.SNI,
				c.ClientCertificate,
				"h2",
			),
		)
	case "h2c":
		httpVersion = HTTPVersion2
	}

	if c.TimeoutPolicy.ConnectTimeout > time.Duration(0) {
		cluster.ConnectTimeout = protobuf.Duration(c.TimeoutPolicy.ConnectTimeout)
	}

	cluster.TypedExtensionProtocolOptions = protocolOptions(httpVersion, c.TimeoutPolicy.IdleConnectionTimeout)

	return cluster
}

// ExtensionCluster builds a envoy_cluster_v3.Cluster struct for the given extension service.
func ExtensionCluster(ext *dag.ExtensionCluster) *envoy_cluster_v3.Cluster {
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
	cluster.ClusterDiscoveryType = ClusterDiscoveryType(envoy_cluster_v3.Cluster_EDS)
	cluster.EdsClusterConfig = &envoy_cluster_v3.Cluster_EdsClusterConfig{
		EdsConfig:   ConfigSource("contour"),
		ServiceName: ext.Upstream.ClusterName,
	}

	// TODO(jpeach): Externalname service support in https://github.com/projectcontour/contour/issues/2875

	http2Version := HTTPVersionAuto
	switch ext.Protocol {
	case "h2":
		http2Version = HTTPVersion2
		cluster.TransportSocket = UpstreamTLSTransportSocket(
			UpstreamTLSContext(
				ext.UpstreamValidation,
				ext.SNI,
				ext.ClientCertificate,
				"h2",
			),
		)
	case "h2c":
		http2Version = HTTPVersion2
	}

	if ext.ClusterTimeoutPolicy.ConnectTimeout > time.Duration(0) {
		cluster.ConnectTimeout = protobuf.Duration(ext.ClusterTimeoutPolicy.ConnectTimeout)
	}
	cluster.TypedExtensionProtocolOptions = protocolOptions(http2Version, ext.ClusterTimeoutPolicy.IdleConnectionTimeout)

	return cluster
}

// StaticClusterLoadAssignment creates a *envoy_endpoint_v3.ClusterLoadAssignment pointing to the external DNS address of the service
func StaticClusterLoadAssignment(service *dag.Service) *envoy_endpoint_v3.ClusterLoadAssignment {
	addr := SocketAddress(service.ExternalName, int(service.Weighted.ServicePort.Port))
	return &envoy_endpoint_v3.ClusterLoadAssignment{
		Endpoints: Endpoints(addr),
		ClusterName: xds.ClusterLoadAssignmentName(
			types.NamespacedName{Name: service.Weighted.ServiceName, Namespace: service.Weighted.ServiceNamespace},
			service.Weighted.ServicePort.Name,
		),
	}
}

func edsconfig(cluster string, service *dag.Service) *envoy_cluster_v3.Cluster_EdsClusterConfig {
	return &envoy_cluster_v3.Cluster_EdsClusterConfig{
		EdsConfig: ConfigSource(cluster),
		ServiceName: xds.ClusterLoadAssignmentName(
			types.NamespacedName{Name: service.Weighted.ServiceName, Namespace: service.Weighted.ServiceNamespace},
			service.Weighted.ServicePort.Name,
		),
	}
}

func lbPolicy(strategy string) envoy_cluster_v3.Cluster_LbPolicy {
	switch strategy {
	case dag.LoadBalancerPolicyWeightedLeastRequest:
		return envoy_cluster_v3.Cluster_LEAST_REQUEST
	case dag.LoadBalancerPolicyRandom:
		return envoy_cluster_v3.Cluster_RANDOM
	case dag.LoadBalancerPolicyCookie, dag.LoadBalancerPolicyRequestHash:
		return envoy_cluster_v3.Cluster_RING_HASH
	default:
		return envoy_cluster_v3.Cluster_ROUND_ROBIN
	}
}

func edshealthcheck(c *dag.Cluster) []*envoy_core_v3.HealthCheck {
	if c.HTTPHealthCheckPolicy == nil && c.TCPHealthCheckPolicy == nil {
		return nil
	}

	if c.HTTPHealthCheckPolicy != nil {
		return []*envoy_core_v3.HealthCheck{
			httpHealthCheck(c),
		}
	}

	return []*envoy_core_v3.HealthCheck{
		tcpHealthCheck(c),
	}
}

// ClusterCommonLBConfig creates a *envoy_cluster_v3.Cluster_CommonLbConfig with HealthyPanicThreshold disabled.
func ClusterCommonLBConfig() *envoy_cluster_v3.Cluster_CommonLbConfig {
	return &envoy_cluster_v3.Cluster_CommonLbConfig{
		HealthyPanicThreshold: &envoy_type.Percent{ // Disable HealthyPanicThreshold
			Value: 0,
		},
	}
}

// ConfigSource returns a *envoy_core_v3.ConfigSource for cluster.
func ConfigSource(cluster string) *envoy_core_v3.ConfigSource {
	return &envoy_core_v3.ConfigSource{
		ResourceApiVersion: envoy_core_v3.ApiVersion_V3,
		ConfigSourceSpecifier: &envoy_core_v3.ConfigSource_ApiConfigSource{
			ApiConfigSource: &envoy_core_v3.ApiConfigSource{
				ApiType:             envoy_core_v3.ApiConfigSource_GRPC,
				TransportApiVersion: envoy_core_v3.ApiVersion_V3,
				GrpcServices: []*envoy_core_v3.GrpcService{{
					TargetSpecifier: &envoy_core_v3.GrpcService_EnvoyGrpc_{
						EnvoyGrpc: &envoy_core_v3.GrpcService_EnvoyGrpc{
							ClusterName: cluster,
						},
					},
				}},
			},
		},
	}
}

// ClusterDiscoveryType returns the type of a ClusterDiscovery as a Cluster_type.
func ClusterDiscoveryType(t envoy_cluster_v3.Cluster_DiscoveryType) *envoy_cluster_v3.Cluster_Type {
	return &envoy_cluster_v3.Cluster_Type{Type: t}
}

// ClusterDiscoveryTypeForAddress returns the type of a ClusterDiscovery as a Cluster_type.
// If the provided address is an IP, overrides the type to STATIC, otherwise uses the
// passed in type.
func ClusterDiscoveryTypeForAddress(address string, t envoy_cluster_v3.Cluster_DiscoveryType) *envoy_cluster_v3.Cluster_Type {
	clusterType := t
	if net.ParseIP(address) != nil {
		clusterType = envoy_cluster_v3.Cluster_STATIC
	}
	return &envoy_cluster_v3.Cluster_Type{Type: clusterType}
}

// parseDNSLookupFamily parses the dnsLookupFamily string into a envoy_cluster_v3.Cluster_DnsLookupFamily
func parseDNSLookupFamily(value string) envoy_cluster_v3.Cluster_DnsLookupFamily {

	switch value {
	case "v4":
		return envoy_cluster_v3.Cluster_V4_ONLY
	case "v6":
		return envoy_cluster_v3.Cluster_V6_ONLY
	}
	return envoy_cluster_v3.Cluster_AUTO
}

func protocolOptions(explicitHTTPVersion HTTPVersionType, idleConnectionTimeout timeout.Setting) map[string]*any.Any {
	// Keep Envoy defaults by not setting protocol options at all if not necessary.
	if explicitHTTPVersion == HTTPVersionAuto && idleConnectionTimeout.UseDefault() {
		return nil
	}

	options := envoy_extensions_upstream_http_v3.HttpProtocolOptions{}

	switch explicitHTTPVersion {
	// Default protocol version in Envoy is HTTP1.1.
	case HTTPVersion1, HTTPVersionAuto:
		options.UpstreamProtocolOptions = &envoy_extensions_upstream_http_v3.HttpProtocolOptions_ExplicitHttpConfig_{
			ExplicitHttpConfig: &envoy_extensions_upstream_http_v3.HttpProtocolOptions_ExplicitHttpConfig{
				ProtocolConfig: &envoy_extensions_upstream_http_v3.HttpProtocolOptions_ExplicitHttpConfig_HttpProtocolOptions{},
			},
		}
	case HTTPVersion2:
		options.UpstreamProtocolOptions = &envoy_extensions_upstream_http_v3.HttpProtocolOptions_ExplicitHttpConfig_{
			ExplicitHttpConfig: &envoy_extensions_upstream_http_v3.HttpProtocolOptions_ExplicitHttpConfig{
				ProtocolConfig: &envoy_extensions_upstream_http_v3.HttpProtocolOptions_ExplicitHttpConfig_Http2ProtocolOptions{},
			},
		}
	case HTTPVersion3:
		options.UpstreamProtocolOptions = &envoy_extensions_upstream_http_v3.HttpProtocolOptions_ExplicitHttpConfig_{
			ExplicitHttpConfig: &envoy_extensions_upstream_http_v3.HttpProtocolOptions_ExplicitHttpConfig{
				ProtocolConfig: &envoy_extensions_upstream_http_v3.HttpProtocolOptions_ExplicitHttpConfig_Http3ProtocolOptions{},
			},
		}
	}

	if !idleConnectionTimeout.UseDefault() {
		options.CommonHttpProtocolOptions = &envoy_core_v3.HttpProtocolOptions{IdleTimeout: protobuf.Duration(idleConnectionTimeout.Duration())}
	}

	return map[string]*any.Any{
		"envoy.extensions.upstreams.http.v3.HttpProtocolOptions": protobuf.MustMarshalAny(&options),
	}
}
