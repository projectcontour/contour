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
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/cluster"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoy_type "github.com/envoyproxy/go-control-plane/envoy/type"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	ingressroutev1 "github.com/heptio/contour/apis/contour/v1beta1"
	"github.com/heptio/contour/internal/dag"
)

const (
	// Default healthcheck / lb algorithm values
	hcTimeout            = 2 * time.Second
	hcInterval           = 10 * time.Second
	hcUnhealthyThreshold = 3
	hcHealthyThreshold   = 2
	hcHost               = "contour-envoy-healthcheck"
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

// notify notifies all registered waiters that an event has occurred.
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
	dag.Visitable

	clusters map[string]*v2.Cluster
}

func (v *clusterVisitor) Visit() map[string]*v2.Cluster {
	v.clusters = make(map[string]*v2.Cluster)
	v.Visitable.Visit(v.visit)
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
		CommonLbConfig: &v2.Cluster_CommonLbConfig{
			HealthyPanicThreshold: &envoy_type.Percent{ // Disable HealthyPanicThreshold
				Value: 0,
			},
		},
	}

	// Set HealthCheck if requested
	if svc.HealthCheck != nil {
		c.HealthChecks = edshealthcheck(svc.HealthCheck)
	}

	if svc.MaxConnections > 0 || svc.MaxPendingRequests > 0 || svc.MaxRequests > 0 || svc.MaxRetries > 0 {
		c.CircuitBreakers = &cluster.CircuitBreakers{
			Thresholds: []*cluster.CircuitBreakers_Thresholds{{
				MaxConnections:     uint32OrNil(svc.MaxConnections),
				MaxPendingRequests: uint32OrNil(svc.MaxPendingRequests),
				MaxRequests:        uint32OrNil(svc.MaxRequests),
				MaxRetries:         uint32OrNil(svc.MaxRetries),
			}},
		}
	}

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
	host := hcHost
	if hc.Host != "" {
		host = hc.Host
	}

	// TODO(dfc) why do we need to specify our own default, what is the default
	// that envoy applies if these fields are left nil?
	return []*core.HealthCheck{{
		Timeout:            secondsOrDefault(hc.TimeoutSeconds, hcTimeout),
		Interval:           secondsOrDefault(hc.IntervalSeconds, hcInterval),
		UnhealthyThreshold: countOrDefault(hc.UnhealthyThresholdCount, hcUnhealthyThreshold),
		HealthyThreshold:   countOrDefault(hc.HealthyThresholdCount, hcHealthyThreshold),
		HealthChecker: &core.HealthCheck_HttpHealthCheck_{
			HttpHealthCheck: &core.HealthCheck_HttpHealthCheck{
				Path: hc.Path,
				Host: host,
			},
		},
	}}
}

func secondsOrDefault(seconds int64, def time.Duration) *time.Duration {
	if seconds != 0 {
		t := time.Duration(seconds) * time.Second
		return &t
	}
	return &def
}

func countOrDefault(count, def uint32) *types.UInt32Value {
	if count != 0 {
		return &types.UInt32Value{
			Value: count,
		}
	}
	return &types.UInt32Value{
		Value: def,
	}
}

// uint32OrNil returns a *types.UInt32Value containing the v or nil if v is zero.
func uint32OrNil(v int) *types.UInt32Value {
	switch v {
	case 0:
		return nil
	default:
		return &types.UInt32Value{Value: uint32(v)}
	}
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
