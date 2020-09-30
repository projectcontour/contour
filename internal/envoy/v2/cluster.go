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

package v2

import (
	"strings"
	"time"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_cluster "github.com/envoyproxy/go-control-plane/envoy/api/v2/cluster"
	envoy_api_v2_core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoy_type "github.com/envoyproxy/go-control-plane/envoy/type"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/envoy"
	"github.com/projectcontour/contour/internal/protobuf"
	"github.com/projectcontour/contour/internal/xds"
	"k8s.io/apimachinery/pkg/types"
)

func clusterDefaults() *v2.Cluster {
	return &v2.Cluster{
		ConnectTimeout: protobuf.Duration(250 * time.Millisecond),
		CommonLbConfig: ClusterCommonLBConfig(),
		LbPolicy:       lbPolicy(""),
	}
}

// Cluster creates new v2.Cluster from dag.Cluster.
func Cluster(c *dag.Cluster) *v2.Cluster {
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
		cluster.ClusterDiscoveryType = ClusterDiscoveryType(v2.Cluster_EDS)
		cluster.EdsClusterConfig = edsconfig("contour", service)
	default:
		// external name set, use hard coded DNS name
		cluster.ClusterDiscoveryType = ClusterDiscoveryType(v2.Cluster_STRICT_DNS)
		cluster.LoadAssignment = StaticClusterLoadAssignment(service)
	}

	// Drain connections immediately if using healthchecks and the endpoint is known to be removed
	if c.HTTPHealthCheckPolicy != nil || c.TCPHealthCheckPolicy != nil {
		cluster.DrainConnectionsOnHostRemoval = true
	}

	if envoy.AnyPositive(service.MaxConnections, service.MaxPendingRequests, service.MaxRequests, service.MaxRetries) {
		cluster.CircuitBreakers = &envoy_cluster.CircuitBreakers{
			Thresholds: []*envoy_cluster.CircuitBreakers_Thresholds{{
				MaxConnections:     protobuf.UInt32OrNil(service.MaxConnections),
				MaxPendingRequests: protobuf.UInt32OrNil(service.MaxPendingRequests),
				MaxRequests:        protobuf.UInt32OrNil(service.MaxRequests),
				MaxRetries:         protobuf.UInt32OrNil(service.MaxRetries),
			}},
		}
	}

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
		cluster.Http2ProtocolOptions = &envoy_api_v2_core.Http2ProtocolOptions{}
		cluster.TransportSocket = UpstreamTLSTransportSocket(
			UpstreamTLSContext(
				c.UpstreamValidation,
				c.SNI,
				c.ClientCertificate,
				"h2",
			),
		)
	case "h2c":
		cluster.Http2ProtocolOptions = &envoy_api_v2_core.Http2ProtocolOptions{}
	}

	return cluster
}

// ExtensionCluster builds a v2.Cluster struct for the given extension service.
func ExtensionCluster(ext *dag.ExtensionCluster) *v2.Cluster {
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
	cluster.ClusterDiscoveryType = ClusterDiscoveryType(v2.Cluster_EDS)
	cluster.EdsClusterConfig = &v2.Cluster_EdsClusterConfig{
		EdsConfig:   ConfigSource("contour"),
		ServiceName: ext.Upstream.ClusterName,
	}

	// TODO(jpeach): Externalname service support in https://github.com/projectcontour/contour/issues/2875

	switch ext.Protocol {
	case "h2":
		cluster.Http2ProtocolOptions = &envoy_api_v2_core.Http2ProtocolOptions{}
		cluster.TransportSocket = UpstreamTLSTransportSocket(
			UpstreamTLSContext(
				ext.UpstreamValidation,
				ext.SNI,
				ext.ClientCertificate,
				"h2",
			),
		)
	case "h2c":
		cluster.Http2ProtocolOptions = &envoy_api_v2_core.Http2ProtocolOptions{}
	}

	return cluster
}

// StaticClusterLoadAssignment creates a *v2.ClusterLoadAssignment pointing to the external DNS address of the service
func StaticClusterLoadAssignment(service *dag.Service) *v2.ClusterLoadAssignment {
	addr := SocketAddress(service.ExternalName, int(service.Weighted.ServicePort.Port))
	return &v2.ClusterLoadAssignment{
		Endpoints: Endpoints(addr),
		ClusterName: xds.ClusterLoadAssignmentName(
			types.NamespacedName{Name: service.Weighted.ServiceName, Namespace: service.Weighted.ServiceNamespace},
			service.Weighted.ServicePort.Name,
		),
	}
}

func edsconfig(cluster string, service *dag.Service) *v2.Cluster_EdsClusterConfig {
	return &v2.Cluster_EdsClusterConfig{
		EdsConfig: ConfigSource(cluster),
		ServiceName: xds.ClusterLoadAssignmentName(
			types.NamespacedName{Name: service.Weighted.ServiceName, Namespace: service.Weighted.ServiceNamespace},
			service.Weighted.ServicePort.Name,
		),
	}
}

func lbPolicy(strategy string) v2.Cluster_LbPolicy {
	switch strategy {
	case "WeightedLeastRequest":
		return v2.Cluster_LEAST_REQUEST
	case "Random":
		return v2.Cluster_RANDOM
	case "Cookie":
		return v2.Cluster_RING_HASH
	default:
		return v2.Cluster_ROUND_ROBIN
	}
}

func edshealthcheck(c *dag.Cluster) []*envoy_api_v2_core.HealthCheck {
	if c.HTTPHealthCheckPolicy == nil && c.TCPHealthCheckPolicy == nil {
		return nil
	}

	if c.HTTPHealthCheckPolicy != nil {
		return []*envoy_api_v2_core.HealthCheck{
			httpHealthCheck(c),
		}
	}

	return []*envoy_api_v2_core.HealthCheck{
		tcpHealthCheck(c),
	}
}

// ClusterCommonLBConfig creates a *v2.Cluster_CommonLbConfig with HealthyPanicThreshold disabled.
func ClusterCommonLBConfig() *v2.Cluster_CommonLbConfig {
	return &v2.Cluster_CommonLbConfig{
		HealthyPanicThreshold: &envoy_type.Percent{ // Disable HealthyPanicThreshold
			Value: 0,
		},
	}
}

// ConfigSource returns a *envoy_api_v2_core.ConfigSource for cluster.
func ConfigSource(cluster string) *envoy_api_v2_core.ConfigSource {
	return &envoy_api_v2_core.ConfigSource{
		ConfigSourceSpecifier: &envoy_api_v2_core.ConfigSource_ApiConfigSource{
			ApiConfigSource: &envoy_api_v2_core.ApiConfigSource{
				ApiType: envoy_api_v2_core.ApiConfigSource_GRPC,
				GrpcServices: []*envoy_api_v2_core.GrpcService{{
					TargetSpecifier: &envoy_api_v2_core.GrpcService_EnvoyGrpc_{
						EnvoyGrpc: &envoy_api_v2_core.GrpcService_EnvoyGrpc{
							ClusterName: cluster,
						},
					},
				}},
			},
		},
	}
}

// ClusterDiscoveryType returns the type of a ClusterDiscovery as a Cluster_type.
func ClusterDiscoveryType(t v2.Cluster_DiscoveryType) *v2.Cluster_Type {
	return &v2.Cluster_Type{Type: t}
}

// parseDNSLookupFamily parses the dnsLookupFamily string into a v2.Cluster_DnsLookupFamily
func parseDNSLookupFamily(value string) v2.Cluster_DnsLookupFamily {

	switch value {
	case "v4":
		return v2.Cluster_V4_ONLY
	case "v6":
		return v2.Cluster_V6_ONLY
	}
	return v2.Cluster_AUTO
}
