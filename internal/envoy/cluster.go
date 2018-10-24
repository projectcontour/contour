// Copyright © 2018 Heptio
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
	"crypto/sha1"
	"crypto/sha256"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_cluster "github.com/envoyproxy/go-control-plane/envoy/api/v2/cluster"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoy_type "github.com/envoyproxy/go-control-plane/envoy/type"
	"github.com/gogo/protobuf/types"
	ingressroutev1 "github.com/heptio/contour/apis/contour/v1beta1"
	"github.com/heptio/contour/internal/dag"
)

// Cluster creates new v2.Cluster from service.
func Cluster(service *dag.Service) *v2.Cluster {
	cluster := &v2.Cluster{
		Name:             Clustername(service),
		Type:             v2.Cluster_EDS,
		EdsClusterConfig: edsconfig("contour", service),
		ConnectTimeout:   250 * time.Millisecond,
		LbPolicy:         lbPolicy(service.LoadBalancerStrategy),
		CommonLbConfig:   clusterCommonLBConfig(),
		HealthChecks:     edshealthcheck(service.HealthCheck),
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
	switch service.Protocol {
	case "h2":
		cluster.TlsContext = UpstreamTLSContext()
		fallthrough
	case "h2c":
		cluster.Http2ProtocolOptions = &core.Http2ProtocolOptions{}
	}
	return cluster
}

func edsconfig(cluster string, service *dag.Service) *v2.Cluster_EdsClusterConfig {
	name := []string{
		service.Namespace(),
		service.Name(),
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
	case "RingHash":
		return v2.Cluster_RING_HASH
	case "Maglev":
		return v2.Cluster_MAGLEV
	case "Random":
		return v2.Cluster_RANDOM
	default:
		return v2.Cluster_ROUND_ROBIN
	}
}

func edshealthcheck(hc *ingressroutev1.HealthCheck) []*core.HealthCheck {
	if hc == nil {
		return nil
	}
	return []*core.HealthCheck{
		healthCheck(hc),
	}
}

// Clustername returns the name of the CDS cluster for this service.
func Clustername(service *dag.Service) string {
	buf := service.LoadBalancerStrategy
	if hc := service.HealthCheck; hc != nil {
		if hc.TimeoutSeconds > 0 {
			buf += (time.Duration(hc.TimeoutSeconds) * time.Second).String()
		}
		if hc.IntervalSeconds > 0 {
			buf += (time.Duration(hc.IntervalSeconds) * time.Second).String()
		}
		if hc.UnhealthyThresholdCount > 0 {
			buf += strconv.Itoa(int(hc.UnhealthyThresholdCount))
		}
		if hc.HealthyThresholdCount > 0 {
			buf += strconv.Itoa(int(hc.HealthyThresholdCount))
		}
		buf += hc.Path
	}

	hash := sha1.Sum([]byte(buf))
	ns := service.Namespace()
	name := service.Name()
	return hashname(60, ns, name, strconv.Itoa(int(service.Port)), fmt.Sprintf("%x", hash[:5]))
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
func anyPositive(first int, rest ...int) bool {
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
// u32nil returns nil if v is zero.
func u32nil(val int) *types.UInt32Value {
	switch val {
	case 0:
		return nil
	default:
		return u32(val)
	}
}

func clusterCommonLBConfig() *v2.Cluster_CommonLbConfig {
	return &v2.Cluster_CommonLbConfig{
		HealthyPanicThreshold: &envoy_type.Percent{ // Disable HealthyPanicThreshold
			Value: 0,
		},
	}
}
