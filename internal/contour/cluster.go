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

package contour

import (
	"sync"

	"strconv"
	"strings"
	"time"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/gogo/protobuf/proto"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoy_api_v2_core4 "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	google_protobuf "github.com/gogo/protobuf/types"
	ingressroutev1 "github.com/heptio/contour/apis/contour/v1beta1"
	"github.com/heptio/contour/internal/dag"
)

const (
	// Default healthcheck / lb algorithm values
	hcTimeout            = int64(2)
	hcInterval           = int64(10)
	hcUnhealthyThreshold = uint32(3)
	hcHealthyThreshold   = uint32(2)
	hcHost               = "contour-envoy-heathcheck"
)

// ClusterCache manages the contents of the gRPC CDS cache.
type ClusterCache struct {
	clusterCache
}

type clusterCache struct {
	mu      sync.Mutex
	values  map[string]*v2.Cluster
	waiters []chan int
	last    int
}

// Register registers ch to receive a value when Notify is called.
// The value of last is the count of the times Notify has been called on this Cache.
// It functions of a sequence counter, if the value of last supplied to Register
// is less than the Cache's internal counter, then the caller has missed at least
// one notification and will fire immediately.
//
// Sends by the broadcaster to ch must not block, therefor ch must have a capacity
// of at least 1.
func (c *clusterCache) Register(ch chan int, last int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if last < c.last {
		// notify this channel immediately
		ch <- c.last
		return
	}
	c.waiters = append(c.waiters, ch)
}

// Update replaces the contents of the cache with the supplied map.
func (c *clusterCache) Update(v map[string]*v2.Cluster) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.values = v
	c.notify()
}

// notify notifies all registered waiters that an event has occured.
func (c *clusterCache) notify() {
	c.last++

	for _, ch := range c.waiters {
		ch <- c.last
	}
	c.waiters = c.waiters[:0]
}

// Values returns a slice of the value stored in the cache.
func (c *clusterCache) Values(filter func(string) bool) []proto.Message {
	c.mu.Lock()
	values := make([]proto.Message, 0, len(c.values))
	for _, v := range c.values {
		if filter(v.Name) {
			values = append(values, v)
		}
	}
	c.mu.Unlock()
	return values
}

// clusterVisitor walks a *dag.DAG and produces a map of *v2.Clusters.
type clusterVisitor struct {
	*ClusterCache
	*dag.DAG

	clusters map[string]*v2.Cluster
}

func (v *clusterVisitor) Visit() map[string]*v2.Cluster {
	v.clusters = make(map[string]*v2.Cluster)
	v.DAG.Visit(v.visit)
	return v.clusters
}

func (v *clusterVisitor) visit(vertex dag.Vertex) {

	if service, ok := vertex.(*dag.Service); ok {
		v.edscluster(service)
	}
	// recurse into children of v
	vertex.Visit(v.visit)
}

func (v *clusterVisitor) edscluster(svc *dag.Service) {
	name := hashname(60, svc.Namespace(), svc.Name(), strconv.Itoa(int(svc.Port)))
	if _, ok := v.clusters[name]; ok {
		// already created this cluster via another edge. skip it.
		return
	}

	c := &v2.Cluster{
		Name:             name,
		Type:             v2.Cluster_EDS,
		EdsClusterConfig: edsconfig("contour", servicename(svc.Namespace(), svc.Name(), svc.ServicePort.Name)),
		ConnectTimeout:   250 * time.Millisecond,
		LbPolicy:         edslbstrategy(svc.LoadBalancerStrategy),
	}

	// Set HealthCheck if requested
	if svc.HealthCheck != nil {
		c.HealthChecks = edshealthcheck(svc.HealthCheck)
	}

	/*
		thresholds := &cluster.CircuitBreakers_Thresholds{
			MaxConnections:     parseAnnotationUInt32(svc.Annotations, annotationMaxConnections),
			MaxPendingRequests: parseAnnotationUInt32(svc.Annotations, annotationMaxPendingRequests),
			MaxRequests:        parseAnnotationUInt32(svc.Annotations, annotationMaxRequests),
			MaxRetries:         parseAnnotationUInt32(svc.Annotations, annotationMaxRetries),
		}
		if thresholds.MaxConnections != nil || thresholds.MaxPendingRequests != nil ||
			thresholds.MaxRequests != nil || thresholds.MaxRetries != nil {
			c.CircuitBreakers = &cluster.CircuitBreakers{
				Thresholds: []*cluster.CircuitBreakers_Thresholds{
					thresholds,
				},
			}
		}
	*/

	switch svc.Protocol {
	case "h2":
		c.Http2ProtocolOptions = &core.Http2ProtocolOptions{}
		c.TlsContext = &auth.UpstreamTlsContext{
			CommonTlsContext: &auth.CommonTlsContext{
				AlpnProtocols: []string{"h2"},
			},
		}
	case "h2c":
		c.Http2ProtocolOptions = &core.Http2ProtocolOptions{}
	}
	v.clusters[c.Name] = c
}

func edslbstrategy(lbStrategy string) v2.Cluster_LbPolicy {
	switch lbStrategy {
	case "RoundRobin":
		return v2.Cluster_ROUND_ROBIN
	case "WeightedLeastRequest":
		return v2.Cluster_LEAST_REQUEST
	case "RingHash":
		return v2.Cluster_RING_HASH
	case "Maglev":
		return v2.Cluster_MAGLEV
	case "Random":
		return v2.Cluster_RANDOM
	}

	// Default
	return v2.Cluster_ROUND_ROBIN
}

func edshealthcheck(hc *ingressroutev1.HealthCheck) []*envoy_api_v2_core4.HealthCheck {
	timeout := hcTimeout
	interval := hcInterval
	unhealthyThreshold := hcUnhealthyThreshold
	healthyThreshold := hcHealthyThreshold
	host := hcHost

	if hc.TimeoutSeconds != 0 {
		timeout = hc.TimeoutSeconds
	}
	if hc.IntervalSeconds != 0 {
		interval = hc.IntervalSeconds
	}
	if hc.UnhealthyThresholdCount != 0 {
		unhealthyThreshold = hc.UnhealthyThresholdCount
	}
	if hc.HealthyThresholdCount != 0 {
		healthyThreshold = hc.HealthyThresholdCount
	}
	if hc.Host != "" {
		host = hc.Host
	}

	return []*envoy_api_v2_core4.HealthCheck{{
		Timeout: &google_protobuf.Duration{
			Seconds: timeout,
		},
		Interval: &google_protobuf.Duration{
			Seconds: int64(interval),
		},
		UnhealthyThreshold: &google_protobuf.UInt32Value{
			Value: unhealthyThreshold,
		},
		HealthyThreshold: &google_protobuf.UInt32Value{
			Value: healthyThreshold,
		},
		HealthChecker: &envoy_api_v2_core4.HealthCheck_HttpHealthCheck_{
			HttpHealthCheck: &envoy_api_v2_core4.HealthCheck_HttpHealthCheck{
				Path: hc.Path,
				Host: host,
			},
		},
	}}

}

func edsconfig(source, name string) *v2.Cluster_EdsClusterConfig {
	return &v2.Cluster_EdsClusterConfig{
		EdsConfig:   apiconfigsource(source), // hard coded by initconfig
		ServiceName: name,
	}
}

func apiconfigsource(clusters ...string) *core.ConfigSource {
	return &core.ConfigSource{
		ConfigSourceSpecifier: &core.ConfigSource_ApiConfigSource{
			ApiConfigSource: &core.ApiConfigSource{
				ApiType:      core.ApiConfigSource_GRPC,
				ClusterNames: clusters,
			},
		},
	}
}

// servicename returns a fixed name for this service and portname
func servicename(namespace, name, portname string) string {
	sn := []string{
		namespace,
		name,
		portname,
	}
	if portname == "" {
		sn = sn[:2]
	}
	return strings.Join(sn, "/")
}
