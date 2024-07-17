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
	"sort"
	"sync"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	resource "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"google.golang.org/protobuf/proto"

	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/envoy"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/protobuf"
	"github.com/projectcontour/contour/internal/sorter"
)

// ClusterCache manages the contents of the gRPC CDS cache.
type ClusterCache struct {
	mu     sync.Mutex
	values map[string]*envoy_config_cluster_v3.Cluster

	envoyGen *envoy_v3.EnvoyGen
}

func NewClusterCache(envoyGen *envoy_v3.EnvoyGen) *ClusterCache {
	return &ClusterCache{
		values:   make(map[string]*envoy_config_cluster_v3.Cluster),
		envoyGen: envoyGen,
	}
}

// Update replaces the contents of the cache with the supplied map.
func (c *ClusterCache) Update(v map[string]*envoy_config_cluster_v3.Cluster) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.values = v
}

// Contents returns a copy of the cache's contents.
func (c *ClusterCache) Contents() []proto.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	var values []*envoy_config_cluster_v3.Cluster
	for _, v := range c.values {
		values = append(values, v)
	}
	sort.Stable(sorter.For(values))
	return protobuf.AsMessages(values)
}

func (*ClusterCache) TypeURL() string { return resource.ClusterType }

func (c *ClusterCache) OnChange(root *dag.DAG) {
	clusters := map[string]*envoy_config_cluster_v3.Cluster{}

	for _, cluster := range root.GetClusters() {
		name := envoy.Clustername(cluster)
		if _, ok := clusters[name]; !ok {
			clusters[name] = c.envoyGen.Cluster(cluster)
		}
	}

	for name, ec := range root.GetExtensionClusters() {
		if _, ok := clusters[name]; !ok {
			clusters[name] = c.envoyGen.ExtensionCluster(ec)
		}
	}

	for _, cluster := range root.GetDNSNameClusters() {
		name := envoy.DNSNameClusterName(cluster)
		if _, ok := clusters[name]; !ok {
			clusters[name] = c.envoyGen.DNSNameCluster(cluster)
		}
	}

	c.Update(clusters)
}
