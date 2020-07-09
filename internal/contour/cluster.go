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

package contour

import (
	"sort"
	"sync"

	resource "github.com/envoyproxy/go-control-plane/pkg/resource/v2"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/golang/protobuf/proto"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/envoy"
	"github.com/projectcontour/contour/internal/protobuf"
	"github.com/projectcontour/contour/internal/sorter"
)

// ClusterCache manages the contents of the gRPC CDS cache.
type ClusterCache struct {
	mu     sync.Mutex
	values map[string]*v2.Cluster
	Cond
}

// Update replaces the contents of the cache with the supplied map.
func (c *ClusterCache) Update(v map[string]*v2.Cluster) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.values = v
	c.Cond.Notify()
}

// Contents returns a copy of the cache's contents.
func (c *ClusterCache) Contents() []proto.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	var values []*v2.Cluster
	for _, v := range c.values {
		values = append(values, v)
	}
	sort.Stable(sorter.For(values))
	return protobuf.AsMessages(values)
}

func (c *ClusterCache) Query(names []string) []proto.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	var values []*v2.Cluster
	for _, n := range names {
		// if the cluster is not registered we cannot return
		// a blank cluster because each cluster has a required
		// discovery type; DNS, EDS, etc. We cannot determine the
		// correct value for this property from the cluster's name
		// provided by the query so we must not return a blank cluster.
		if v, ok := c.values[n]; ok {
			values = append(values, v)
		}
	}
	sort.Stable(sorter.For(values))
	return protobuf.AsMessages(values)
}

func (*ClusterCache) TypeURL() string { return resource.ClusterType }

type clusterVisitor struct {
	clusters map[string]*v2.Cluster
}

// visitCluster produces a map of *v2.Clusters.
func visitClusters(root dag.Vertex) map[string]*v2.Cluster {
	cv := clusterVisitor{
		clusters: make(map[string]*v2.Cluster),
	}
	cv.visit(root)
	return cv.clusters
}

func (v *clusterVisitor) visit(vertex dag.Vertex) {
	if cluster, ok := vertex.(*dag.Cluster); ok {
		name := envoy.Clustername(cluster)
		if _, ok := v.clusters[name]; !ok {
			c := envoy.Cluster(cluster)
			v.clusters[c.Name] = c
		}
	}

	// recurse into children of v
	vertex.Visit(v.visit)
}
