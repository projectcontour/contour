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

package envoy

import (
	"crypto/sha1" // nolint:gosec
	"crypto/sha256"
	"fmt"
	"strconv"
	"strings"
	"time"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_cluster "github.com/envoyproxy/go-control-plane/envoy/api/v2/cluster"
	envoy_api_v2_core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoy_type "github.com/envoyproxy/go-control-plane/envoy/type"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/protobuf"
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

	cluster.Name = Clustername(c)
	cluster.AltStatName = altStatName(service)
	cluster.LbPolicy = lbPolicy(c.LoadBalancerPolicy)
	cluster.HealthChecks = edshealthcheck(c)

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

	if anyPositive(service.MaxConnections, service.MaxPendingRequests, service.MaxRequests, service.MaxRetries) {
		cluster.CircuitBreakers = &envoy_cluster.CircuitBreakers{
			Thresholds: []*envoy_cluster.CircuitBreakers_Thresholds{{
				MaxConnections:     u32nil(service.MaxConnections),
				MaxPendingRequests: u32nil(service.MaxPendingRequests),
				MaxRequests:        u32nil(service.MaxRequests),
				MaxRetries:         u32nil(service.MaxRetries),
			}},
		}
	}

	switch c.Protocol {
	case "tls":
		cluster.TransportSocket = UpstreamTLSTransportSocket(
			UpstreamTLSContext(
				c.UpstreamValidation,
				c.SNI,
			),
		)
	case "h2":
		cluster.TransportSocket = UpstreamTLSTransportSocket(
			UpstreamTLSContext(
				c.UpstreamValidation,
				service.ExternalName,
				"h2",
			),
		)
		fallthrough
	case "h2c":
		cluster.Http2ProtocolOptions = &envoy_api_v2_core.Http2ProtocolOptions{}
	}

	return cluster
}

// StaticClusterLoadAssignment creates a *v2.ClusterLoadAssignment pointing to the external DNS address of the service
func StaticClusterLoadAssignment(service *dag.Service) *v2.ClusterLoadAssignment {
	name := []string{
		service.Namespace,
		service.Name,
		service.ServicePort.Name,
	}

	addr := SocketAddress(service.ExternalName, int(service.ServicePort.Port))
	return &v2.ClusterLoadAssignment{
		ClusterName: strings.Join(name, "/"),
		Endpoints:   Endpoints(addr),
	}
}

func edsconfig(cluster string, service *dag.Service) *v2.Cluster_EdsClusterConfig {
	name := []string{
		service.Namespace,
		service.Name,
		service.ServicePort.Name,
	}
	if name[2] == "" {
		name = name[:2]
	}
	return &v2.Cluster_EdsClusterConfig{
		EdsConfig:   ConfigSource(cluster),
		ServiceName: strings.Join(name, "/"),
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
	} else {
		return []*envoy_api_v2_core.HealthCheck{
			tcpHealthCheck(c),
		}
	}
}

// Clustername returns the name of the CDS cluster for this service.
func Clustername(cluster *dag.Cluster) string {
	service := cluster.Upstream
	buf := cluster.LoadBalancerPolicy
	if hc := cluster.HTTPHealthCheckPolicy; hc != nil {
		if hc.Timeout > 0 {
			buf += hc.Timeout.String()
		}
		if hc.Interval > 0 {
			buf += hc.Interval.String()
		}
		if hc.UnhealthyThreshold > 0 {
			buf += strconv.Itoa(int(hc.UnhealthyThreshold))
		}
		if hc.HealthyThreshold > 0 {
			buf += strconv.Itoa(int(hc.HealthyThreshold))
		}
		buf += hc.Path
	}
	if uv := cluster.UpstreamValidation; uv != nil {
		buf += uv.CACertificate.Object.ObjectMeta.Name
		buf += uv.SubjectName
	}

	// This isn't a crypto hash, we just want a unique name.
	hash := sha1.Sum([]byte(buf)) // nolint:gosec

	ns := service.Namespace
	name := service.Name
	return hashname(60, ns, name, strconv.Itoa(int(service.Port)), fmt.Sprintf("%x", hash[:5]))
}

// altStatName generates an alternative stat name for the service
// using format ns_name_port
func altStatName(service *dag.Service) string {
	return strings.Join([]string{service.Namespace, service.Name, strconv.Itoa(int(service.Port))}, "_")
}

// hashname takes a lenth l and a varargs of strings s and returns a string whose length
// which does not exceed l. Internally s is joined with strings.Join(s, "/"). If the
// combined length exceeds l then hashname truncates each element in s, starting from the
// end using a hash derived from the contents of s (not the current element). This process
// continues until the length of s does not exceed l, or all elements have been truncated.
// In which case, the entire string is replaced with a hash not exceeding the length of l.
func hashname(l int, s ...string) string {
	const shorthash = 6 // the length of the shorthash

	r := strings.Join(s, "/")
	if l > len(r) {
		// we're under the limit, nothing to do
		return r
	}
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(r)))
	for n := len(s) - 1; n >= 0; n-- {
		s[n] = truncate(l/len(s), s[n], hash[:shorthash])
		r = strings.Join(s, "/")
		if l > len(r) {
			return r
		}
	}
	// truncated everything, but we're still too long
	// just return the hash truncated to l.
	return hash[:min(len(hash), l)]
}

// truncate truncates s to l length by replacing the
// end of s with -suffix.
func truncate(l int, s, suffix string) string {
	if l >= len(s) {
		// under the limit, nothing to do
		return s
	}
	if l <= len(suffix) {
		// easy case, just return the start of the suffix
		return suffix[:min(l, len(suffix))]
	}
	return s[:l-len(suffix)-1] + "-" + suffix
}

func min(a, b int) int {
	if a > b {
		return b
	}
	return a
}

// anyPositive indicates if any of the values provided are greater than zero.
func anyPositive(first uint32, rest ...uint32) bool {
	if first > 0 {
		return true
	}
	for _, v := range rest {
		if v > 0 {
			return true
		}
	}
	return false
}

// u32nil creates a *types.UInt32Value containing v.
// u33nil returns nil if v is zero.
func u32nil(val uint32) *wrappers.UInt32Value {
	switch val {
	case 0:
		return nil
	default:
		return protobuf.UInt32(val)
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
